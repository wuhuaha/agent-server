"""Headless validation runner for the agent-server desktop client package."""

from __future__ import annotations

import argparse
import asyncio
from dataclasses import asdict, dataclass, field
from datetime import UTC, datetime
import json
from pathlib import Path
import time
from typing import Any
from urllib.parse import urljoin
from urllib.request import urlopen
from uuid import uuid4

import websockets

from .audio import chunk_pcm_bytes, generate_silence, load_pcm_wav, write_pcm_wav
from .protocol import DiscoveryInfo, build_event, http_base_to_ws_base, join_ws_url, new_session_id


@dataclass(slots=True)
class ScenarioMetrics:
    thinking_latency_ms: int | None = None
    speaking_latency_ms: int | None = None
    active_return_latency_ms: int | None = None
    response_start_latency_ms: int | None = None
    first_partial_latency_ms: int | None = None
    first_text_latency_ms: int | None = None
    first_audio_latency_ms: int | None = None
    barge_in_cutoff_latency_ms: int | None = None
    response_complete_latency_ms: int | None = None
    playout_complete_latency_ms: int | None = None
    session_end_latency_ms: int | None = None
    response_text_count: int = 0
    response_text_chars: int = 0
    partial_response_count: int = 0
    partial_response_chars: int = 0
    heard_text_chars: int = 0
    response_start_count: int = 0
    tool_call_count: int = 0
    tool_result_count: int = 0
    audio_chunk_count: int = 0
    received_audio_bytes: int = 0


@dataclass(slots=True)
class ScenarioResult:
    name: str
    session_id: str
    ok: bool
    turn_id: str | None = None
    trace_id: str | None = None
    end_reason: str | None = None
    turn_ids: list[str] = field(default_factory=list)
    trace_ids: list[str] = field(default_factory=list)
    checks: list[str] = field(default_factory=list)
    issues: list[str] = field(default_factory=list)
    artifacts: dict[str, str] = field(default_factory=dict)
    response_texts: list[str] = field(default_factory=list)
    audio_chunks: list[int] = field(default_factory=list)
    received_audio_bytes: int = 0
    events: list[dict[str, Any]] = field(default_factory=list)
    metrics: ScenarioMetrics = field(default_factory=ScenarioMetrics)


@dataclass(slots=True)
class QualitySummary:
    scenario_count: int
    ok_scenarios: int
    thinking_latency_ms_avg: float | None = None
    speaking_latency_ms_avg: float | None = None
    active_return_latency_ms_avg: float | None = None
    response_start_latency_ms_avg: float | None = None
    first_partial_latency_ms_avg: float | None = None
    first_text_latency_ms_avg: float | None = None
    first_audio_latency_ms_avg: float | None = None
    barge_in_cutoff_latency_ms_avg: float | None = None
    response_complete_latency_ms_avg: float | None = None
    playout_complete_latency_ms_avg: float | None = None
    issue_scenario_count: int = 0
    received_audio_bytes_total: int = 0
    response_text_chars_total: int = 0
    partial_response_ratio: float = 0.0
    heard_text_chars_total: int = 0
    response_with_audio_ratio: float = 0.0
    non_empty_response_ratio: float = 0.0


@dataclass(slots=True)
class ValidationReport:
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
    scenarios: list[ScenarioResult]
    artifact_dir: str | None = None

    @property
    def ok(self) -> bool:
        return all(scenario.ok for scenario in self.scenarios)

    @property
    def quality_summary(self) -> QualitySummary:
        return build_quality_summary(self.scenarios)


@dataclass(slots=True)
class ObservedInbound:
    kind: str
    received_at: float
    event_type: str | None = None
    payload: dict[str, Any] | None = None


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
        self.run_id = _new_run_id()
        self.generated_at = _utc_timestamp()
        self.discovery = self._discover()
        self.ws_url = join_ws_url(http_base_to_ws_base(self.http_base), self.discovery.ws_path)
        self._seq = 0

    def _discover(self) -> DiscoveryInfo:
        discovery_url = urljoin(f"{self.http_base}/", "v1/realtime")
        with urlopen(discovery_url, timeout=10) as response:
            payload = json.load(response)
        return DiscoveryInfo.from_dict(payload)

    async def run_text_scenario(self, text: str, artifact_root: Path | None = None) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="text", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            turn_started_at = time.perf_counter()
            await self._send_text(ws, session_id, text)
            await self._collect_until_active_or_end(ws, result, received_audio=received_audio, turn_started_at=turn_started_at)
            await self._end_session(ws, session_id, reason="client_stop", message="text scenario complete")
            result.events.append(await self._recv_json(ws))
            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any(text in chunk for chunk in result.response_texts), result, "response includes echoed text input")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.issues) == 0 and len(result.checks) == 3
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_audio_scenario(
        self,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        artifact_root: Path | None = None,
    ) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="audio", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            await self._send_audio(ws, self._build_audio_chunks(wav_path, silence_ms, frame_ms), frame_ms)
            turn_started_at = time.perf_counter()
            await self._send_commit(ws, session_id, "end_of_speech")
            await self._collect_until_active_or_end(ws, result, received_audio=received_audio, turn_started_at=turn_started_at)
            await self._end_session(ws, session_id, reason="client_stop", message="audio scenario complete")
            result.events.append(await self._recv_json(ws))
            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any(chunk.strip() for chunk in result.response_texts), result, "received non-empty response text")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.issues) == 0 and len(result.checks) == 3
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_server_end_scenario(self, artifact_root: Path | None = None) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="server-end", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            turn_started_at = time.perf_counter()
            await self._send_text(ws, session_id, "/end")
            await self._collect_until_active_or_end(
                ws,
                result,
                stop_on_server_end=True,
                received_audio=received_audio,
                turn_started_at=turn_started_at,
            )
            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any("/end" in chunk for chunk in result.response_texts), result, "response includes /end trigger echo")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "server initiated session.end observed")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received before close")
            result.ok = len(result.issues) == 0 and len(result.checks) == 3
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_server_endpoint_preview_scenario(
        self,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        artifact_root: Path | None = None,
    ) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="server-endpoint-preview", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            turn_started_at = time.perf_counter()
            await self._send_audio(ws, self._build_audio_chunks(wav_path, silence_ms, frame_ms), frame_ms)

            timed_out = False
            try:
                await self._collect_until_active_or_end(
                    ws,
                    result,
                    received_audio=received_audio,
                    turn_started_at=turn_started_at,
                )
            except TimeoutError:
                timed_out = True
                result.issues.append("server-endpoint preview produced no response before timeout")

            if not timed_out:
                await self._end_session(ws, session_id, reason="client_stop", message="server-endpoint preview complete")
                result.events.append(await self._recv_json(ws))

            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            self._check(not timed_out, result, "server-endpoint preview returned before client commit")
            self._check(any(chunk.strip() for chunk in result.response_texts), result, "received non-empty response text")
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.issues) == 0 and len(result.checks) == 4
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_tool_scenario(self, artifact_root: Path | None = None) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="tool", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            turn_started_at = time.perf_counter()
            await self._send_text(ws, session_id, "/tool time.now {}")
            await self._collect_until_active_or_end(ws, result, received_audio=received_audio, turn_started_at=turn_started_at)
            await self._end_session(ws, session_id, reason="client_stop", message="tool scenario complete")
            result.events.append(await self._recv_json(ws))
            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any(event.get("type") == "response.start" for event in result.events), result, "response.start received")
            self._check(result.metrics.tool_call_count > 0, result, "tool_call delta observed")
            self._check(result.metrics.tool_result_count > 0, result, "tool_result delta observed")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.issues) == 0 and len(result.checks) == 4
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_timeout_scenario(self, artifact_root: Path | None = None) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="timeout", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            started_at = time.perf_counter()

            if self.discovery.idle_timeout_ms <= 0:
                self._check(False, result, "server advertises idle timeout support")
                self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
                return result

            while True:
                observed = await self._observe_inbound(ws, result, received_audio=received_audio, turn_started_at=started_at)
                if observed.event_type != "session.end":
                    continue
                _set_latency(result.metrics, "session_end_latency_ms", started_at, observed.received_at)
                break

            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "server session.end observed")
            self._check(result.end_reason == "idle_timeout", result, "timeout scenario ended with idle_timeout")
            result.ok = len(result.issues) == 0 and len(result.checks) == 2
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_barge_in_scenario(
        self,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        interrupt_wav_path: str | None = None,
        interrupt_silence_ms: int = 600,
        artifact_root: Path | None = None,
    ) -> ScenarioResult:
        session_id = new_session_id()
        async with self._connect() as ws:
            result = ScenarioResult(name="barge-in", session_id=session_id, ok=False)
            received_audio = bytearray()
            await self._start_session(ws, session_id)
            await self._send_audio(ws, self._build_audio_chunks(wav_path, silence_ms, frame_ms), frame_ms)
            await self._send_commit(ws, session_id, "end_of_speech")

            interrupt_chunks = self._build_audio_chunks(interrupt_wav_path, interrupt_silence_ms, frame_ms)
            current_response_index = 0
            first_response_audio_chunks = 0
            second_response_audio_chunks = 0
            first_response_text_chars = 0
            active_after_interrupt = False
            interrupt_sent = False
            client_end_sent = False
            interrupt_requested_at: float | None = None
            first_turn_started_at = time.perf_counter()

            while True:
                observed = await self._observe_inbound(
                    ws,
                    result,
                    received_audio=received_audio,
                    turn_started_at=first_turn_started_at,
                )
                if observed.kind == "audio":
                    if current_response_index == 1:
                        first_response_audio_chunks += 1
                        if not interrupt_sent:
                            await self._send_event(ws, "session.update", {"interrupt": True}, session_id=session_id)
                            await self._send_audio(ws, interrupt_chunks, frame_ms)
                            await self._send_commit(ws, session_id, "barge_in")
                            interrupt_sent = True
                            interrupt_requested_at = time.perf_counter()
                    elif current_response_index >= 2:
                        second_response_audio_chunks += 1
                    continue

                if observed.event_type == "response.start":
                    current_response_index = result.metrics.response_start_count
                    continue

                if observed.event_type == "response.chunk" and observed.payload is not None:
                    text = observed.payload.get("text")
                    if current_response_index == 1 and isinstance(text, str):
                        first_response_text_chars += len(text)

                if observed.event_type == "session.update" and observed.payload is not None:
                    state = observed.payload.get("state")
                    if state == "active" and interrupt_sent and result.metrics.response_start_count == 1:
                        active_after_interrupt = True
                        _set_latency(result.metrics, "barge_in_cutoff_latency_ms", interrupt_requested_at, observed.received_at)
                    elif state == "active" and interrupt_sent and result.metrics.response_start_count >= 2 and not client_end_sent:
                        _set_latency(result.metrics, "active_return_latency_ms", first_turn_started_at, observed.received_at)
                        _set_latency(result.metrics, "response_complete_latency_ms", first_turn_started_at, observed.received_at)
                        if result.metrics.audio_chunk_count > 0:
                            _set_latency(result.metrics, "playout_complete_latency_ms", first_turn_started_at, observed.received_at)
                        await self._end_session(ws, session_id, reason="client_stop", message="barge-in scenario complete")
                        client_end_sent = True
                        continue

                if observed.event_type == "session.end":
                    break

            result.received_audio_bytes = len(received_audio)
            result.metrics.received_audio_bytes = result.received_audio_bytes
            result.metrics.response_text_count = len(result.response_texts)
            result.metrics.heard_text_chars = first_response_text_chars
            self._check(interrupt_sent, result, "interrupt turn sent after first audio playout")
            self._check(first_response_audio_chunks > 0, result, "first response produced audio before interruption")
            self._check(active_after_interrupt, result, "session returned to active after interruption before second response")
            self._check(result.metrics.response_start_count >= 2, result, "second response started after barge-in")
            self._check(second_response_audio_chunks > 0, result, "second response produced audio")
            self._check(any(event.get("type") == "session.end" for event in result.events), result, "client session.end acknowledged")
            result.ok = len(result.issues) == 0 and len(result.checks) == 6
            self._write_scenario_artifacts(artifact_root, result, bytes(received_audio))
            return result

    async def run_full(
        self,
        text: str,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        save_rx_dir: str | None = None,
    ) -> ValidationReport:
        artifact_root = self._artifact_root(save_rx_dir)
        scenarios = [
            await self.run_text_scenario(text, artifact_root=artifact_root),
            await self.run_audio_scenario(silence_ms, frame_ms, wav_path=wav_path, artifact_root=artifact_root),
            await self.run_server_end_scenario(artifact_root=artifact_root),
        ]
        return ValidationReport(
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
            scenarios=scenarios,
            artifact_dir=str(artifact_root) if artifact_root else None,
        )

    async def run_regression(
        self,
        text: str,
        silence_ms: int,
        frame_ms: int,
        wav_path: str | None = None,
        save_rx_dir: str | None = None,
    ) -> ValidationReport:
        artifact_root = self._artifact_root(save_rx_dir)
        scenarios = [
            await self.run_text_scenario(text, artifact_root=artifact_root),
            await self.run_audio_scenario(silence_ms, frame_ms, wav_path=wav_path, artifact_root=artifact_root),
            await self.run_server_end_scenario(artifact_root=artifact_root),
            await self.run_tool_scenario(artifact_root=artifact_root),
            await self.run_barge_in_scenario(silence_ms, frame_ms, wav_path=wav_path, artifact_root=artifact_root),
            await self.run_timeout_scenario(artifact_root=artifact_root),
        ]
        return ValidationReport(
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
            scenarios=scenarios,
            artifact_dir=str(artifact_root) if artifact_root else None,
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

    def _build_audio_chunks(self, wav_path: str | None, silence_ms: int, frame_ms: int) -> list[bytes]:
        if wav_path:
            clip = load_pcm_wav(wav_path, self.discovery.input_sample_rate_hz, self.discovery.input_channels)
        else:
            clip = generate_silence(silence_ms, self.discovery.input_sample_rate_hz, self.discovery.input_channels)
        return chunk_pcm_bytes(clip.frames, clip.sample_rate_hz, clip.channels, frame_ms=frame_ms)

    async def _send_audio(self, ws: websockets.ClientConnection, chunks: list[bytes], frame_ms: int) -> None:
        frame_interval = max(0.0, frame_ms / 1000)
        for chunk in chunks:
            await ws.send(chunk)
            if frame_interval > 0:
                await asyncio.sleep(frame_interval)

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
            observed = await self._observe_inbound(
                ws,
                result,
                received_audio=received_audio,
                turn_started_at=turn_started_at,
            )
            if observed.event_type == "session.update" and observed.payload is not None:
                state = observed.payload.get("state")
                if state == "active" and (result.response_texts or result.metrics.audio_chunk_count > 0 or result.turn_id):
                    _set_latency(result.metrics, "active_return_latency_ms", turn_started_at, observed.received_at)
                    _set_latency(result.metrics, "response_complete_latency_ms", turn_started_at, observed.received_at)
                    if result.metrics.audio_chunk_count > 0:
                        _set_latency(result.metrics, "playout_complete_latency_ms", turn_started_at, observed.received_at)
                    return
            if observed.event_type == "session.end" and stop_on_server_end:
                _set_latency(result.metrics, "session_end_latency_ms", turn_started_at, observed.received_at)
                _set_latency(result.metrics, "response_complete_latency_ms", turn_started_at, observed.received_at)
                if result.metrics.audio_chunk_count > 0:
                    _set_latency(result.metrics, "playout_complete_latency_ms", turn_started_at, observed.received_at)
                return

    async def _observe_inbound(
        self,
        ws: websockets.ClientConnection,
        result: ScenarioResult,
        received_audio: bytearray | None = None,
        turn_started_at: float | None = None,
    ) -> ObservedInbound:
        message = await asyncio.wait_for(ws.recv(), timeout=self.timeout_sec)
        received_at = time.perf_counter()
        if isinstance(message, bytes):
            result.audio_chunks.append(len(message))
            result.metrics.audio_chunk_count += 1
            if received_audio is not None:
                received_audio.extend(message)
            _set_latency(result.metrics, "first_audio_latency_ms", turn_started_at, received_at)
            result.events.append({"type": "audio.out.chunk", "bytes": len(message)})
            return ObservedInbound(kind="audio", received_at=received_at)

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
            state = payload_dict.get("state")
            if state == "thinking":
                _set_latency(result.metrics, "thinking_latency_ms", turn_started_at, received_at)
            elif state == "speaking":
                _set_latency(result.metrics, "speaking_latency_ms", turn_started_at, received_at)
        elif event_type == "response.start" and payload_dict is not None:
            result.metrics.response_start_count += 1
            turn_id = payload_dict.get("turn_id")
            trace_id = payload_dict.get("trace_id")
            if isinstance(turn_id, str) and turn_id:
                result.turn_id = turn_id
                _append_unique(result.turn_ids, turn_id)
            if isinstance(trace_id, str) and trace_id:
                result.trace_id = trace_id
                _append_unique(result.trace_ids, trace_id)
            _set_latency(result.metrics, "response_start_latency_ms", turn_started_at, received_at)
        elif event_type == "response.chunk" and payload_dict is not None:
            delta_type = payload_dict.get("delta_type")
            if delta_type == "tool_call":
                result.metrics.tool_call_count += 1
            elif delta_type == "tool_result":
                result.metrics.tool_result_count += 1
            elif delta_type in {"speech_partial", "input_partial", "transcription_partial"}:
                result.metrics.partial_response_count += 1
            text = payload_dict.get("text")
            if isinstance(text, str):
                result.response_texts.append(text)
                result.metrics.response_text_chars += len(text)
                if delta_type in {"speech_partial", "input_partial", "transcription_partial"}:
                    result.metrics.partial_response_chars += len(text)
                    if text.strip():
                        _set_latency(result.metrics, "first_partial_latency_ms", turn_started_at, received_at)
                if text.strip():
                    _set_latency(result.metrics, "first_text_latency_ms", turn_started_at, received_at)
        elif event_type == "session.end" and payload_dict is not None:
            reason = payload_dict.get("reason")
            if isinstance(reason, str) and reason:
                result.end_reason = reason

        return ObservedInbound(
            kind="event",
            received_at=received_at,
            event_type=event_type if isinstance(event_type, str) else None,
            payload=payload_dict,
        )

    async def _recv_json(self, ws: websockets.ClientConnection) -> dict[str, Any]:
        while True:
            message = await asyncio.wait_for(ws.recv(), timeout=self.timeout_sec)
            if isinstance(message, bytes):
                continue
            return json.loads(message)

    def _check(self, condition: bool, result: ScenarioResult, description: str) -> None:
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

    def _write_scenario_artifacts(self, artifact_root: Path | None, result: ScenarioResult, received_audio: bytes) -> None:
        if artifact_root is None:
            return
        scenario_dir = artifact_root / result.name
        scenario_dir.mkdir(parents=True, exist_ok=True)

        events_path = scenario_dir / "events.json"
        response_path = scenario_dir / "response.txt"
        scenario_path = scenario_dir / "scenario.json"

        result.artifacts["events_json"] = str(events_path)
        result.artifacts["response_txt"] = str(response_path)
        result.artifacts["scenario_json"] = str(scenario_path)

        events_path.write_text(json.dumps(result.events, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        response_path.write_text("".join(result.response_texts), encoding="utf-8")

        if received_audio:
            audio_path = scenario_dir / "received-audio.wav"
            write_pcm_wav(
                str(audio_path),
                received_audio,
                self.discovery.output_sample_rate_hz,
                self.discovery.output_channels,
            )
            result.artifacts["received_audio_wav"] = str(audio_path)

        scenario_path.write_text(json.dumps(asdict(result), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Scripted validation runner for agent-server realtime bootstrap.")
    parser.add_argument("--http-base", default="http://127.0.0.1:8080", help="HTTP base URL for agent-server.")
    parser.add_argument(
        "--scenario",
        choices=["text", "audio", "server-end", "server-endpoint-preview", "tool", "barge-in", "timeout", "full", "regression"],
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
    parser.add_argument("--save-rx-dir", default=None, help="Optional directory for replay-friendly scenario artifacts.")
    return parser


async def _run_from_args(args: argparse.Namespace) -> ValidationReport:
    runner = RealtimeScenarioRunner(
        http_base=args.http_base,
        device_id=args.device_id,
        client_type=args.client_type,
        firmware_version=args.firmware_version,
        timeout_sec=args.timeout_sec,
    )
    artifact_root = runner._artifact_root(args.save_rx_dir)
    if args.scenario == "text":
        scenarios = [await runner.run_text_scenario(args.text, artifact_root=artifact_root)]
    elif args.scenario == "audio":
        scenarios = [
            await runner.run_audio_scenario(
                args.silence_ms,
                args.frame_ms,
                wav_path=args.wav_path,
                artifact_root=artifact_root,
            )
        ]
    elif args.scenario == "server-end":
        scenarios = [await runner.run_server_end_scenario(artifact_root=artifact_root)]
    elif args.scenario == "server-endpoint-preview":
        scenarios = [
            await runner.run_server_endpoint_preview_scenario(
                args.silence_ms,
                args.frame_ms,
                wav_path=args.wav_path,
                artifact_root=artifact_root,
            )
        ]
    elif args.scenario == "tool":
        scenarios = [await runner.run_tool_scenario(artifact_root=artifact_root)]
    elif args.scenario == "barge-in":
        scenarios = [
            await runner.run_barge_in_scenario(
                args.silence_ms,
                args.frame_ms,
                wav_path=args.wav_path,
                artifact_root=artifact_root,
            )
        ]
    elif args.scenario == "timeout":
        scenarios = [await runner.run_timeout_scenario(artifact_root=artifact_root)]
    elif args.scenario == "regression":
        return await runner.run_regression(
            text=args.text,
            silence_ms=args.silence_ms,
            frame_ms=args.frame_ms,
            wav_path=args.wav_path,
            save_rx_dir=args.save_rx_dir,
        )
    else:
        return await runner.run_full(
            text=args.text,
            silence_ms=args.silence_ms,
            frame_ms=args.frame_ms,
            wav_path=args.wav_path,
            save_rx_dir=args.save_rx_dir,
        )
    return ValidationReport(
        generated_at=runner.generated_at,
        run_id=runner.run_id,
        http_base=runner.http_base,
        protocol_version=runner.discovery.protocol_version,
        ws_path=runner.discovery.ws_path,
        subprotocol=runner.discovery.subprotocol,
        turn_mode=runner.discovery.turn_mode,
        llm_provider=runner.discovery.llm_provider,
        voice_provider=runner.discovery.voice_provider,
        tts_provider=runner.discovery.tts_provider,
        scenarios=scenarios,
        artifact_dir=str(artifact_root) if artifact_root else None,
    )


def main() -> None:
    parser = _build_parser()
    args = parser.parse_args()
    report = asyncio.run(_run_from_args(args))
    payload = {
        "ok": report.ok,
        "generated_at": report.generated_at,
        "run_id": report.run_id,
        "http_base": report.http_base,
        "protocol_version": report.protocol_version,
        "ws_path": report.ws_path,
        "subprotocol": report.subprotocol,
        "turn_mode": report.turn_mode,
        "llm_provider": report.llm_provider,
        "voice_provider": report.voice_provider,
        "tts_provider": report.tts_provider,
        "artifact_dir": report.artifact_dir,
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
        thinking_latency_ms_avg=_average_optional(
            scenario.metrics.thinking_latency_ms for scenario in scenarios
        ),
        speaking_latency_ms_avg=_average_optional(
            scenario.metrics.speaking_latency_ms for scenario in scenarios
        ),
        active_return_latency_ms_avg=_average_optional(
            scenario.metrics.active_return_latency_ms for scenario in scenarios
        ),
        response_start_latency_ms_avg=_average_optional(
            scenario.metrics.response_start_latency_ms for scenario in scenarios
        ),
        first_partial_latency_ms_avg=_average_optional(
            scenario.metrics.first_partial_latency_ms for scenario in scenarios
        ),
        first_text_latency_ms_avg=_average_optional(
            scenario.metrics.first_text_latency_ms for scenario in scenarios
        ),
        first_audio_latency_ms_avg=_average_optional(
            scenario.metrics.first_audio_latency_ms for scenario in scenarios
        ),
        barge_in_cutoff_latency_ms_avg=_average_optional(
            scenario.metrics.barge_in_cutoff_latency_ms for scenario in scenarios
        ),
        response_complete_latency_ms_avg=_average_optional(
            scenario.metrics.response_complete_latency_ms for scenario in scenarios
        ),
        playout_complete_latency_ms_avg=_average_optional(
            scenario.metrics.playout_complete_latency_ms for scenario in scenarios
        ),
        issue_scenario_count=sum(1 for scenario in scenarios if scenario.issues),
        received_audio_bytes_total=sum(scenario.metrics.received_audio_bytes for scenario in scenarios),
        response_text_chars_total=sum(scenario.metrics.response_text_chars for scenario in scenarios),
        partial_response_ratio=round(
            sum(1 for scenario in scenarios if scenario.metrics.partial_response_count > 0) / total,
            3,
        ),
        heard_text_chars_total=sum(scenario.metrics.heard_text_chars for scenario in scenarios),
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


def _utc_timestamp() -> str:
    return datetime.now(UTC).isoformat(timespec="seconds").replace("+00:00", "Z")


def _new_run_id() -> str:
    return f"run_{uuid4().hex[:12]}"


def _append_unique(target: list[str], value: str) -> None:
    if value not in target:
        target.append(value)


if __name__ == "__main__":
    main()
