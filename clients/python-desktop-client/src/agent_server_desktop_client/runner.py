"""Headless validation runner for the agent-server desktop client package."""

from __future__ import annotations

import argparse
import asyncio
from dataclasses import asdict, dataclass, field
import json
from pathlib import Path
import time
from typing import Any
from urllib.parse import urljoin
from urllib.request import urlopen

import websockets

from .audio import chunk_pcm_bytes, generate_silence, load_pcm_wav, write_pcm_wav
from .protocol import DiscoveryInfo, build_event, http_base_to_ws_base, join_ws_url, new_session_id


@dataclass(slots=True)
class ScenarioMetrics:
    response_start_latency_ms: int | None = None
    first_text_latency_ms: int | None = None
    first_audio_latency_ms: int | None = None
    response_complete_latency_ms: int | None = None
    session_end_latency_ms: int | None = None
    response_text_count: int = 0
    audio_chunk_count: int = 0
    received_audio_bytes: int = 0


@dataclass(slots=True)
class ScenarioResult:
    name: str
    session_id: str
    ok: bool
    checks: list[str] = field(default_factory=list)
    response_texts: list[str] = field(default_factory=list)
    audio_chunks: list[int] = field(default_factory=list)
    received_audio_bytes: int = 0
    events: list[dict[str, Any]] = field(default_factory=list)
    metrics: ScenarioMetrics = field(default_factory=ScenarioMetrics)


@dataclass(slots=True)
class QualitySummary:
    scenario_count: int
    ok_scenarios: int
    response_start_latency_ms_avg: float | None = None
    first_text_latency_ms_avg: float | None = None
    first_audio_latency_ms_avg: float | None = None
    response_complete_latency_ms_avg: float | None = None
    response_with_audio_ratio: float = 0.0
    non_empty_response_ratio: float = 0.0


@dataclass(slots=True)
class ValidationReport:
    http_base: str
    protocol_version: str
    ws_path: str
    subprotocol: str
    turn_mode: str
    voice_provider: str
    tts_provider: str
    scenarios: list[ScenarioResult]

    @property
    def ok(self) -> bool:
        return all(scenario.ok for scenario in self.scenarios)

    @property
    def quality_summary(self) -> QualitySummary:
        return build_quality_summary(self.scenarios)


class RealtimeScenarioRunner:
    """Headless runner for scripted realtime validation scenarios."""

    def __init__(
        self,
        http_base: str,
        device_id: str,
        client_type: str = "desktop-script",
        firmware_version: str = "script-runner-0.1.0",
        timeout_sec: float = 30.0,
    ) -> None:
        self.http_base = http_base.rstrip("/")
        self.device_id = device_id
        self.client_type = client_type
        self.firmware_version = firmware_version
        self.timeout_sec = timeout_sec
        self.discovery = self._discover()
        self.ws_url = join_ws_url(http_base_to_ws_base(self.http_base), self.discovery.ws_path)
        self._seq = 0

    def _discover(self) -> DiscoveryInfo:
        discovery_url = urljoin(f"{self.http_base}/", "v1/realtime")
        with urlopen(discovery_url, timeout=10) as response:
            payload = json.load(response)
        return DiscoveryInfo.from_dict(payload)

    async def run_text_scenario(self, text: str) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="text", session_id=session_id, ok=False)
            await self._start_session(ws, session_id)
            turn_started_at = time.perf_counter()
            await self._send_text(ws, session_id, text)
            await self._collect_until_active_or_end(ws, result, turn_started_at=turn_started_at)
            await self._end_session(ws, session_id, reason="client_stop", message="text scenario complete")
            result.events.append(await self._recv_json(ws))
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any(text in chunk for chunk in result.response_texts), result, "response includes echoed text input")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.checks) == 3
            return result

    async def run_audio_scenario(
        self,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        save_audio_path: str | None = None,
    ) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="audio", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            if wav_path:
                clip = load_pcm_wav(wav_path, self.discovery.input_sample_rate_hz, self.discovery.input_channels)
            else:
                clip = generate_silence(silence_ms, self.discovery.input_sample_rate_hz, self.discovery.input_channels)
            for chunk in chunk_pcm_bytes(clip.frames, clip.sample_rate_hz, clip.channels, frame_ms=frame_ms):
                await ws.send(chunk)
            turn_started_at = time.perf_counter()
            await self._send_commit(ws, session_id, "end_of_speech")
            await self._collect_until_active_or_end(ws, result, received_audio=received_audio, turn_started_at=turn_started_at)
            await self._end_session(ws, session_id, reason="client_stop", message="audio scenario complete")
            result.events.append(await self._recv_json(ws))
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
            self._check(any(chunk.strip() for chunk in result.response_texts), result, "received non-empty response text")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.checks) == 3
            return result

    async def run_server_end_scenario(self) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="server-end", session_id=session_id, ok=False)
            await self._start_session(ws, session_id)
            turn_started_at = time.perf_counter()
            await self._send_text(ws, session_id, "/end")
            await self._collect_until_active_or_end(ws, result, stop_on_server_end=True, turn_started_at=turn_started_at)
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any("/end" in chunk for chunk in result.response_texts), result, "response includes /end trigger echo")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "server initiated session.end observed")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received before close")
            result.ok = len(result.checks) == 3
            return result

    async def run_full(
        self,
        text: str,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        save_rx_dir: str | None = None,
    ) -> ValidationReport:
        audio_save_path = None
        if save_rx_dir:
            output_dir = Path(save_rx_dir)
            output_dir.mkdir(parents=True, exist_ok=True)
            audio_save_path = str(output_dir / "audio-scenario-rx.wav")
        scenarios = [
            await self.run_text_scenario(text),
            await self.run_audio_scenario(silence_ms, frame_ms, wav_path=wav_path, save_audio_path=audio_save_path),
            await self.run_server_end_scenario(),
        ]
        return ValidationReport(
            http_base=self.http_base,
            protocol_version=self.discovery.protocol_version,
            ws_path=self.discovery.ws_path,
            subprotocol=self.discovery.subprotocol,
            turn_mode=self.discovery.turn_mode,
            voice_provider=self.discovery.voice_provider,
            tts_provider=self.discovery.tts_provider,
            scenarios=scenarios,
        )

    def _connect(self) -> websockets.ClientConnection:
        return websockets.connect(
            self.ws_url,
            subprotocols=[self.discovery.subprotocol],
            max_size=None,
            ping_interval=20,
            ping_timeout=20,
        )

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
                    "wake_reason": "script",
                    "client_can_end": True,
                    "server_can_end": True,
                },
                "capabilities": {
                    "text_input": self.discovery.allow_text_input,
                    "image_input": self.discovery.allow_image_input,
                    "half_duplex": False,
                    "local_wake_word": False,
                },
            },
            session_id=session_id,
        )

    async def _send_text(self, ws: websockets.ClientConnection, session_id: str, text: str) -> None:
        await self._send_event(ws, "text.in", {"text": text}, session_id=session_id)

    async def _send_commit(self, ws: websockets.ClientConnection, session_id: str, reason: str) -> None:
        await self._send_event(ws, "audio.in.commit", {"reason": reason}, session_id=session_id)

    async def _end_session(
        self,
        ws: websockets.ClientConnection,
        session_id: str,
        reason: str,
        message: str,
    ) -> None:
        await self._send_event(
            ws,
            "session.end",
            {"reason": reason, "message": message},
            session_id=session_id,
        )

    async def _send_event(
        self,
        ws: websockets.ClientConnection,
        event_type: str,
        payload: dict[str, Any],
        session_id: str | None = None,
    ) -> None:
        self._seq += 1
        await ws.send(json.dumps(build_event(event_type, self._seq, payload, session_id=session_id), ensure_ascii=False))

    async def _collect_until_active_or_end(
        self,
        ws: websockets.ClientConnection,
        result: ScenarioResult,
        stop_on_server_end: bool = False,
        received_audio: bytearray | None = None,
        turn_started_at: float | None = None,
    ) -> None:
        while True:
            message = await asyncio.wait_for(ws.recv(), timeout=self.timeout_sec)
            received_at = time.perf_counter()
            if isinstance(message, bytes):
                result.audio_chunks.append(len(message))
                result.metrics.audio_chunk_count += 1
                if received_audio is not None:
                    received_audio.extend(message)
                _set_latency(result.metrics, "first_audio_latency_ms", turn_started_at, received_at)
                result.events.append({"type": "audio.out.chunk", "bytes": len(message)})
                continue
            event = json.loads(message)
            result.events.append(event)
            event_type = event.get("type")
            payload = event.get("payload", {})
            if event_type == "response.start":
                _set_latency(result.metrics, "response_start_latency_ms", turn_started_at, received_at)
            if event_type == "response.chunk" and isinstance(payload, dict):
                text = payload.get("text")
                if isinstance(text, str):
                    result.response_texts.append(text)
                    if text.strip():
                        _set_latency(result.metrics, "first_text_latency_ms", turn_started_at, received_at)
            if event_type == "session.end" and stop_on_server_end:
                _set_latency(result.metrics, "session_end_latency_ms", turn_started_at, received_at)
                _set_latency(result.metrics, "response_complete_latency_ms", turn_started_at, received_at)
                return
            if event_type == "session.update" and isinstance(payload, dict):
                if payload.get("state") == "active" and result.response_texts:
                    _set_latency(result.metrics, "response_complete_latency_ms", turn_started_at, received_at)
                    return

    async def _recv_json(self, ws: websockets.ClientConnection) -> dict[str, Any]:
        while True:
            message = await asyncio.wait_for(ws.recv(), timeout=self.timeout_sec)
            if isinstance(message, bytes):
                continue
            return json.loads(message)

    def _check(self, condition: bool, result: ScenarioResult, description: str) -> None:
        if condition:
            result.checks.append(description)


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Scripted validation runner for agent-server realtime bootstrap.")
    parser.add_argument("--http-base", default="http://127.0.0.1:8080", help="HTTP base URL for agent-server.")
    parser.add_argument(
        "--scenario",
        choices=["text", "audio", "server-end", "full"],
        default="full",
        help="Scenario to execute.",
    )
    parser.add_argument("--text", default="hello from scripted desktop client", help="Text payload for text scenario.")
    parser.add_argument("--silence-ms", type=int, default=1000, help="Silence duration for audio scenario.")
    parser.add_argument("--frame-ms", type=int, default=20, help="PCM frame size in milliseconds.")
    parser.add_argument("--wav", dest="wav_path", default=None, help="Optional PCM16LE WAV file for audio scenario.")
    parser.add_argument("--device-id", default="desktop-script-001", help="Device ID used for session.start.")
    parser.add_argument("--client-type", default="desktop-script", help="Client type used for session.start.")
    parser.add_argument("--firmware-version", default="script-runner-0.1.0", help="Firmware version used for session.start.")
    parser.add_argument("--timeout-sec", type=float, default=30.0, help="Per-receive timeout in seconds.")
    parser.add_argument("--output", default=None, help="Optional JSON report output path.")
    parser.add_argument("--save-rx-dir", default=None, help="Optional directory for received audio artifacts.")
    return parser


async def _run_from_args(args: argparse.Namespace) -> ValidationReport:
    runner = RealtimeScenarioRunner(
        http_base=args.http_base,
        device_id=args.device_id,
        client_type=args.client_type,
        firmware_version=args.firmware_version,
        timeout_sec=args.timeout_sec,
    )
    if args.scenario == "text":
        scenarios = [await runner.run_text_scenario(args.text)]
    elif args.scenario == "audio":
        scenarios = [
            await runner.run_audio_scenario(
                args.silence_ms,
                args.frame_ms,
                wav_path=args.wav_path,
                save_audio_path=str(Path(args.save_rx_dir) / "audio-scenario-rx.wav") if args.save_rx_dir else None,
            )
        ]
    elif args.scenario == "server-end":
        scenarios = [await runner.run_server_end_scenario()]
    else:
        return await runner.run_full(
            text=args.text,
            silence_ms=args.silence_ms,
            frame_ms=args.frame_ms,
            wav_path=args.wav_path,
            save_rx_dir=args.save_rx_dir,
        )
    return ValidationReport(
        http_base=runner.http_base,
        protocol_version=runner.discovery.protocol_version,
        ws_path=runner.discovery.ws_path,
        subprotocol=runner.discovery.subprotocol,
        turn_mode=runner.discovery.turn_mode,
        voice_provider=runner.discovery.voice_provider,
        tts_provider=runner.discovery.tts_provider,
        scenarios=scenarios,
    )


def main() -> None:
    parser = _build_parser()
    args = parser.parse_args()
    report = asyncio.run(_run_from_args(args))
    payload = {
        "ok": report.ok,
        "http_base": report.http_base,
        "protocol_version": report.protocol_version,
        "ws_path": report.ws_path,
        "subprotocol": report.subprotocol,
        "turn_mode": report.turn_mode,
        "voice_provider": report.voice_provider,
        "tts_provider": report.tts_provider,
        "quality_summary": asdict(report.quality_summary),
        "scenarios": [asdict(scenario) for scenario in report.scenarios],
    }
    rendered = json.dumps(payload, ensure_ascii=False, indent=2)
    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(rendered + "\n", encoding="utf-8")
    print(rendered)


def _set_latency(metrics: ScenarioMetrics, field_name: str, started_at: float | None, observed_at: float) -> None:
    if started_at is None or getattr(metrics, field_name) is not None:
        return
    setattr(metrics, field_name, int(round((observed_at - started_at) * 1000)))


def build_quality_summary(scenarios: list[ScenarioResult]) -> QualitySummary:
    total = len(scenarios)
    if total == 0:
        return QualitySummary(scenario_count=0, ok_scenarios=0)

    return QualitySummary(
        scenario_count=total,
        ok_scenarios=sum(1 for scenario in scenarios if scenario.ok),
        response_start_latency_ms_avg=_average_optional(
            scenario.metrics.response_start_latency_ms for scenario in scenarios
        ),
        first_text_latency_ms_avg=_average_optional(
            scenario.metrics.first_text_latency_ms for scenario in scenarios
        ),
        first_audio_latency_ms_avg=_average_optional(
            scenario.metrics.first_audio_latency_ms for scenario in scenarios
        ),
        response_complete_latency_ms_avg=_average_optional(
            scenario.metrics.response_complete_latency_ms for scenario in scenarios
        ),
        response_with_audio_ratio=round(
            sum(1 for scenario in scenarios if scenario.metrics.audio_chunk_count > 0) / total,
            3,
        ),
        non_empty_response_ratio=round(
            sum(1 for scenario in scenarios if any(text.strip() for text in scenario.response_texts)) / total,
            3,
        ),
    )


def _average_optional(values: Any) -> float | None:
    filtered = [float(value) for value in values if value is not None]
    if not filtered:
        return None
    return round(sum(filtered) / len(filtered), 1)


if __name__ == "__main__":
    main()
