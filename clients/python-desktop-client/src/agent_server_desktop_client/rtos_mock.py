"""RTOS-oriented reference client for the agent-server realtime wire profile."""

from __future__ import annotations

import argparse
import asyncio
from dataclasses import asdict, dataclass, field
from datetime import UTC, datetime
import json
from pathlib import Path
from typing import Any
from urllib.parse import urljoin
from urllib.request import urlopen
from uuid import uuid4

import websockets

from .audio import chunk_pcm_bytes, generate_silence, load_pcm_wav, write_pcm_wav
from .protocol import DiscoveryInfo, build_event, http_base_to_ws_base, join_ws_url, new_session_id


@dataclass(slots=True)
class MockMetrics:
    response_start_count: int = 0
    response_text_count: int = 0
    tool_call_count: int = 0
    tool_result_count: int = 0
    audio_chunk_count: int = 0
    received_audio_bytes: int = 0


@dataclass(slots=True)
class MockRunResult:
    generated_at: str
    run_id: str
    http_base: str
    protocol_version: str
    ws_path: str
    subprotocol: str
    turn_mode: str
    llm_provider: str
    voice_provider: str
    tts_provider: str
    session_id: str
    ok: bool
    interrupt_sent: bool
    close_reason: str | None
    turn_id: str | None = None
    trace_id: str | None = None
    turn_ids: list[str] = field(default_factory=list)
    trace_ids: list[str] = field(default_factory=list)
    checks: list[str] = field(default_factory=list)
    issues: list[str] = field(default_factory=list)
    artifacts: dict[str, str] = field(default_factory=dict)
    response_texts: list[str] = field(default_factory=list)
    audio_chunks: list[int] = field(default_factory=list)
    received_audio_bytes: int = 0
    events: list[dict[str, Any]] = field(default_factory=list)
    metrics: MockMetrics = field(default_factory=MockMetrics)
    artifact_dir: str | None = None


class RTOSMockClient:
    """Reference client that behaves closer to an RTOS voice endpoint."""

    def __init__(
        self,
        http_base: str,
        device_id: str,
        client_type: str = "rtos-mock",
        firmware_version: str = "rtos-mock-0.1.0",
        timeout_sec: float = 30.0,
    ) -> None:
        self.http_base = http_base.rstrip("/")
        self.device_id = device_id
        self.client_type = client_type
        self.firmware_version = firmware_version
        self.timeout_sec = timeout_sec
        self.generated_at = _utc_timestamp()
        self.run_id = _new_run_id()
        self.discovery = self._discover()
        self.ws_url = join_ws_url(http_base_to_ws_base(self.http_base), self.discovery.ws_path)
        self._seq = 0

    def _discover(self) -> DiscoveryInfo:
        discovery_url = urljoin(f"{self.http_base}/", "v1/realtime")
        with urlopen(discovery_url, timeout=10) as response:
            payload = json.load(response)
        return DiscoveryInfo.from_dict(payload)

    async def run(
        self,
        wav_path: str | None,
        text: str | None,
        silence_ms: int,
        frame_ms: int,
        interrupt_wav_path: str | None,
        interrupt_silence_ms: int,
        send_interrupt_update: bool,
        auto_end: bool,
        save_audio_path: str | None,
        artifact_dir: str | None = None,
    ) -> MockRunResult:
        session_id = new_session_id()
        result = MockRunResult(
            generated_at=self.generated_at,
            run_id=self.run_id,
            http_base=self.http_base,
            protocol_version=self.discovery.protocol_version,
            ws_path=self.discovery.ws_path,
            subprotocol=self.discovery.subprotocol,
            turn_mode=self.discovery.turn_mode,
            llm_provider=self.discovery.llm_provider,
            voice_provider=self.discovery.voice_provider,
            tts_provider=self.discovery.tts_provider,
            session_id=session_id,
            ok=False,
            interrupt_sent=False,
            close_reason=None,
        )
        artifact_root = self._artifact_root(artifact_dir)
        if artifact_root is not None:
            result.artifact_dir = str(artifact_root)

        received_audio = bytearray()
        primary_chunks = self._build_chunks(wav_path, silence_ms, frame_ms)
        interrupt_chunks = (
            self._build_chunks(interrupt_wav_path, interrupt_silence_ms, frame_ms)
            if interrupt_wav_path or interrupt_silence_ms > 0
            else []
        )
        turns_sent = 0
        responses_started = 0

        async with websockets.connect(
            self.ws_url,
            subprotocols=[self.discovery.subprotocol],
            max_size=None,
            ping_interval=20,
            ping_timeout=20,
        ) as ws:
            await self._start_session(ws, session_id)
            if text:
                await self._send_event(ws, "text.in", {"text": text}, session_id=session_id)
                turns_sent += 1
            else:
                await self._send_audio(ws, primary_chunks, frame_ms)
                await self._send_event(ws, "audio.in.commit", {"reason": "end_of_speech"}, session_id=session_id)
                turns_sent += 1

            client_end_sent = False
            while True:
                message = await asyncio.wait_for(ws.recv(), timeout=self.timeout_sec)
                if isinstance(message, bytes):
                    result.audio_chunks.append(len(message))
                    result.metrics.audio_chunk_count += 1
                    received_audio.extend(message)
                    result.events.append({"type": "audio.out.chunk", "bytes": len(message)})
                    if interrupt_chunks and not result.interrupt_sent:
                        if send_interrupt_update:
                            await self._send_event(
                                ws,
                                "session.update",
                                {"interrupt": True},
                                session_id=session_id,
                            )
                        await self._send_audio(ws, interrupt_chunks, frame_ms)
                        await self._send_event(
                            ws,
                            "audio.in.commit",
                            {"reason": "barge_in"},
                            session_id=session_id,
                        )
                        result.interrupt_sent = True
                        turns_sent += 1
                    continue

                event = json.loads(message)
                result.events.append(event)
                event_type = event.get("type")
                payload = event.get("payload", {})
                payload_dict = payload if isinstance(payload, dict) else None

                if event_type == "session.update" and payload_dict is not None:
                    turn_id = payload_dict.get("turn_id")
                    if isinstance(turn_id, str) and turn_id:
                        result.turn_id = turn_id
                        _append_unique(result.turn_ids, turn_id)

                if event_type == "response.start" and payload_dict is not None:
                    responses_started += 1
                    result.metrics.response_start_count += 1
                    turn_id = payload_dict.get("turn_id")
                    trace_id = payload_dict.get("trace_id")
                    if isinstance(turn_id, str) and turn_id:
                        result.turn_id = turn_id
                        _append_unique(result.turn_ids, turn_id)
                    if isinstance(trace_id, str) and trace_id:
                        result.trace_id = trace_id
                        _append_unique(result.trace_ids, trace_id)

                if event_type == "response.chunk" and payload_dict is not None:
                    delta_type = payload_dict.get("delta_type")
                    if delta_type == "tool_call":
                        result.metrics.tool_call_count += 1
                    elif delta_type == "tool_result":
                        result.metrics.tool_result_count += 1
                    text_chunk = payload_dict.get("text")
                    if isinstance(text_chunk, str):
                        result.response_texts.append(text_chunk)

                if event_type == "session.end" and payload_dict is not None:
                    reason = payload_dict.get("reason")
                    if isinstance(reason, str):
                        result.close_reason = reason
                    break

                if auto_end and event_type == "session.update" and payload_dict is not None:
                    state = payload_dict.get("state")
                    if (
                        not client_end_sent
                        and state == "active"
                        and responses_started > 0
                        and responses_started >= turns_sent
                        and (not interrupt_chunks or result.interrupt_sent)
                    ):
                        await self._send_event(
                            ws,
                            "session.end",
                            {"reason": "client_stop", "message": "rtos mock completed"},
                            session_id=session_id,
                        )
                        client_end_sent = True

        result.received_audio_bytes = len(received_audio)
        result.metrics.received_audio_bytes = result.received_audio_bytes
        result.metrics.response_text_count = len(result.response_texts)

        if save_audio_path and received_audio:
            write_pcm_wav(
                save_audio_path,
                bytes(received_audio),
                self.discovery.output_sample_rate_hz,
                self.discovery.output_channels,
            )
            result.artifacts["legacy_received_audio_wav"] = str(Path(save_audio_path))

        self._check(result.metrics.response_start_count > 0, result, "response.start received")
        self._check(bool(result.close_reason), result, "session.end observed")
        self._check(
            any(chunk.strip() for chunk in result.response_texts) or result.metrics.audio_chunk_count > 0,
            result,
            "received non-empty response text or audio",
        )
        if interrupt_chunks:
            self._check(result.interrupt_sent, result, "interrupt turn sent")
            self._check(result.metrics.response_start_count >= 2, result, "multiple responses observed after barge-in")

        self._write_artifacts(artifact_root, result, bytes(received_audio))
        result.ok = len(result.issues) == 0
        return result

    def _build_chunks(self, wav_path: str | None, silence_ms: int, frame_ms: int) -> list[bytes]:
        if wav_path:
            clip = load_pcm_wav(wav_path, self.discovery.input_sample_rate_hz, self.discovery.input_channels)
        else:
            clip = generate_silence(silence_ms, self.discovery.input_sample_rate_hz, self.discovery.input_channels)
        return chunk_pcm_bytes(clip.frames, clip.sample_rate_hz, clip.channels, frame_ms=frame_ms)

    async def _start_session(self, ws: websockets.ClientConnection, session_id: str) -> None:
        await self._send_event(
            ws,
            "session.start",
            {
                "protocol_version": self.discovery.protocol_version,
                "device": {
                    "device_id": self.device_id,
                    "client_type": self.client_type,
                    "firmware_version": self.firmware_version,
                },
                "audio": {
                    "codec": self.discovery.input_codec,
                    "sample_rate_hz": self.discovery.input_sample_rate_hz,
                    "channels": self.discovery.input_channels,
                },
                "session": {
                    "mode": "voice",
                    "wake_reason": "keyword",
                    "client_can_end": True,
                    "server_can_end": True,
                },
                "capabilities": {
                    "text_input": self.discovery.allow_text_input,
                    "image_input": self.discovery.allow_image_input,
                    "half_duplex": False,
                    "local_wake_word": True,
                },
            },
            session_id=session_id,
        )

    async def _send_audio(self, ws: websockets.ClientConnection, chunks: list[bytes], frame_ms: int) -> None:
        frame_interval = max(0.0, frame_ms / 1000)
        for chunk in chunks:
            await ws.send(chunk)
            if frame_interval > 0:
                await asyncio.sleep(frame_interval)

    async def _send_event(
        self,
        ws: websockets.ClientConnection,
        event_type: str,
        payload: dict[str, Any],
        session_id: str | None,
    ) -> None:
        self._seq += 1
        event = build_event(event_type, self._seq, payload, session_id=session_id)
        await ws.send(json.dumps(event, ensure_ascii=False))

    def _check(self, condition: bool, result: MockRunResult, description: str) -> None:
        if condition:
            result.checks.append(description)
            return
        result.issues.append(description)

    def _artifact_root(self, raw_dir: str | None) -> Path | None:
        if not raw_dir:
            return None
        root = Path(raw_dir) / self.run_id
        root.mkdir(parents=True, exist_ok=True)
        return root

    def _write_artifacts(self, artifact_root: Path | None, result: MockRunResult, received_audio: bytes) -> None:
        if artifact_root is None:
            return

        events_path = artifact_root / "events.json"
        response_path = artifact_root / "response.txt"
        summary_path = artifact_root / "run.json"
        result.artifacts["events_json"] = str(events_path)
        result.artifacts["response_txt"] = str(response_path)
        result.artifacts["run_json"] = str(summary_path)

        events_path.write_text(json.dumps(result.events, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        response_path.write_text("".join(result.response_texts), encoding="utf-8")

        if received_audio:
            audio_path = artifact_root / "received-audio.wav"
            write_pcm_wav(
                str(audio_path),
                received_audio,
                self.discovery.output_sample_rate_hz,
                self.discovery.output_channels,
            )
            result.artifacts["received_audio_wav"] = str(audio_path)

        summary_path.write_text(json.dumps(asdict(result), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="RTOS-oriented reference client for agent-server.")
    parser.add_argument("--http-base", default="http://127.0.0.1:8080", help="HTTP base URL for agent-server.")
    parser.add_argument("--device-id", default="rtos-mock-001", help="Device ID for session.start.")
    parser.add_argument("--client-type", default="rtos-mock", help="Client type for session.start.")
    parser.add_argument("--firmware-version", default="rtos-mock-0.1.0", help="Firmware version for session.start.")
    parser.add_argument("--wav", dest="wav_path", default=None, help="Primary PCM16LE 16k mono WAV file.")
    parser.add_argument("--text", default=None, help="Optional text turn instead of primary audio.")
    parser.add_argument("--silence-ms", type=int, default=1000, help="Fallback silence duration for the primary turn.")
    parser.add_argument("--frame-ms", type=int, default=20, help="PCM frame size in milliseconds.")
    parser.add_argument("--interrupt-wav", dest="interrupt_wav_path", default=None, help="Optional WAV clip used for barge-in.")
    parser.add_argument(
        "--interrupt-silence-ms",
        type=int,
        default=0,
        help="Optional silence clip used for barge-in when interrupt-wav is omitted.",
    )
    parser.add_argument(
        "--no-interrupt-update",
        action="store_true",
        help="Do not send session.update {interrupt:true} before the barge-in audio clip.",
    )
    parser.add_argument("--no-auto-end", action="store_true", help="Keep the session open after the final active state.")
    parser.add_argument("--timeout-sec", type=float, default=30.0, help="Per-receive timeout in seconds.")
    parser.add_argument("--output", default=None, help="Optional JSON summary output path.")
    parser.add_argument("--save-rx", default=None, help="Optional legacy path for received PCM16 WAV output.")
    parser.add_argument("--save-rx-dir", default=None, help="Optional directory for replay-friendly RTOS mock artifacts.")
    return parser


async def _run_from_args(args: argparse.Namespace) -> MockRunResult:
    client = RTOSMockClient(
        http_base=args.http_base,
        device_id=args.device_id,
        client_type=args.client_type,
        firmware_version=args.firmware_version,
        timeout_sec=args.timeout_sec,
    )
    return await client.run(
        wav_path=args.wav_path,
        text=args.text,
        silence_ms=args.silence_ms,
        frame_ms=args.frame_ms,
        interrupt_wav_path=args.interrupt_wav_path,
        interrupt_silence_ms=args.interrupt_silence_ms,
        send_interrupt_update=not args.no_interrupt_update,
        auto_end=not args.no_auto_end,
        save_audio_path=args.save_rx,
        artifact_dir=args.save_rx_dir,
    )


def main() -> None:
    parser = _build_parser()
    args = parser.parse_args()
    result = asyncio.run(_run_from_args(args))
    payload = asdict(result)
    rendered = json.dumps(payload, ensure_ascii=False, indent=2)
    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(rendered + "\n", encoding="utf-8")
    print(rendered)


def _utc_timestamp() -> str:
    return datetime.now(UTC).isoformat(timespec="seconds").replace("+00:00", "Z")


def _new_run_id() -> str:
    return f"run_{uuid4().hex[:12]}"


def _append_unique(target: list[str], value: str) -> None:
    if value not in target:
        target.append(value)


if __name__ == "__main__":
    main()
