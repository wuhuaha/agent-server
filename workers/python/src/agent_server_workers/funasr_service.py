"""Local HTTP ASR worker backed by FunASR."""

from __future__ import annotations

import argparse
import base64
from dataclasses import asdict, dataclass, field
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import tempfile
import threading
import time
from typing import Any
from uuid import uuid4
import wave


@dataclass(slots=True)
class WorkerConfig:
    host: str
    port: int
    model: str
    device: str
    language: str
    trust_remote_code: bool
    disable_update: bool
    batch_size_s: int
    use_itn: bool
    stream_preview_min_audio_ms: int
    stream_preview_min_interval_ms: int
    stream_endpoint_tail_ms: int
    stream_endpoint_mean_abs_threshold: int


@dataclass(slots=True)
class StreamState:
    stream_id: str
    session_id: str
    turn_id: str
    trace_id: str
    device_id: str
    codec: str
    sample_rate_hz: int
    channels: int
    language: str
    created_at_monotonic: float
    buffer: bytearray = field(default_factory=bytearray)
    partials: list[str] = field(default_factory=list)
    last_partial_text: str = ""
    last_preview_at_monotonic: float = 0.0


class FunASREngine:
    """Lazy model wrapper so import and model load stay off startup critical path."""

    def __init__(self, config: WorkerConfig) -> None:
        self.config = config
        self._lock = threading.Lock()
        self._model: Any | None = None
        self._postprocess = None
        self._last_error = ""
        self._streams_lock = threading.Lock()
        self._streams: dict[str, StreamState] = {}

    @property
    def last_error(self) -> str:
        return self._last_error

    def health(self) -> dict[str, Any]:
        return {
            "status": "ok" if self._model is not None else "starting",
            "model_loaded": self._model is not None,
            "model": self.config.model,
            "device": self.config.device,
            "language": self.config.language,
            "stream_preview_min_audio_ms": self.config.stream_preview_min_audio_ms,
            "stream_preview_min_interval_ms": self.config.stream_preview_min_interval_ms,
            "stream_endpoint_tail_ms": self.config.stream_endpoint_tail_ms,
            "stream_endpoint_mean_abs_threshold": self.config.stream_endpoint_mean_abs_threshold,
            "last_error": self._last_error,
        }

    def transcribe_pcm(
        self,
        audio_bytes: bytes,
        sample_rate_hz: int,
        channels: int,
        session_id: str = "",
        language: str = "",
    ) -> dict[str, Any]:
        effective_language = language or self.config.language
        if not audio_bytes:
            return {
                "text": "",
                "raw_text": "",
                "segments": [],
                "duration_ms": 0,
                "session_id": session_id,
                "model": self.config.model,
                "device": self.config.device,
                "language": effective_language,
            }
        model, postprocess = self._ensure_model()
        duration_ms = int(len(audio_bytes) / max(channels, 1) / 2 / sample_rate_hz * 1000)
        wav_path = self._write_temp_wav(audio_bytes, sample_rate_hz, channels)
        try:
            with self._lock:
                started_at = time.perf_counter()
                result = model.generate(
                    input=wav_path,
                    cache={},
                    language=effective_language,
                    use_itn=self.config.use_itn,
                    batch_size_s=self.config.batch_size_s,
                )
                elapsed_ms = int((time.perf_counter() - started_at) * 1000)
        finally:
            Path(wav_path).unlink(missing_ok=True)

        raw_text = self._extract_raw_text(result)
        text = postprocess(raw_text) if raw_text and postprocess is not None else raw_text
        segments = [text] if text else []
        return {
            "text": text,
            "raw_text": raw_text,
            "segments": segments,
            "duration_ms": duration_ms,
            "elapsed_ms": elapsed_ms,
            "session_id": session_id,
            "model": self.config.model,
            "device": self.config.device,
            "language": effective_language,
            "mode": "batch",
        }

    def start_stream(
        self,
        *,
        session_id: str,
        turn_id: str,
        trace_id: str,
        device_id: str,
        codec: str,
        sample_rate_hz: int,
        channels: int,
        language: str,
    ) -> dict[str, Any]:
        if codec != "pcm16le":
            raise ValueError("only pcm16le is supported in the current worker")
        stream_id = f"strm_{uuid4().hex[:16]}"
        state = StreamState(
            stream_id=stream_id,
            session_id=session_id,
            turn_id=turn_id,
            trace_id=trace_id,
            device_id=device_id,
            codec=codec,
            sample_rate_hz=sample_rate_hz,
            channels=channels,
            language=language or self.config.language,
            created_at_monotonic=time.monotonic(),
        )
        with self._streams_lock:
            self._streams[stream_id] = state
        return {
            "stream_id": stream_id,
            "session_id": session_id,
            "turn_id": turn_id,
            "trace_id": trace_id,
            "device_id": device_id,
            "mode": "stream_preview_batch",
            "status": "started",
        }

    def push_stream_audio(self, stream_id: str, audio_bytes: bytes) -> dict[str, Any]:
        if not audio_bytes:
            state = self._get_stream(stream_id)
            return self._stream_status_payload(state)

        with self._streams_lock:
            state = self._must_get_stream_locked(stream_id)
            state.buffer.extend(audio_bytes)
            preview_snapshot = self._preview_snapshot_locked(state)

        preview_payload = self._maybe_preview_stream(preview_snapshot)
        preview_changed = False
        with self._streams_lock:
            state = self._must_get_stream_locked(stream_id)
            if preview_payload is not None:
                latest_partial = preview_payload.get("text", "").strip()
                preview_endpoint_reason = self._preview_endpoint_reason(preview_snapshot, preview_payload)
                if preview_endpoint_reason:
                    preview_payload["preview_endpoint_reason"] = preview_endpoint_reason
                state.last_preview_at_monotonic = time.monotonic()
                if latest_partial and latest_partial != state.last_partial_text:
                    state.last_partial_text = latest_partial
                    state.partials.append(latest_partial)
                    preview_changed = True
            return self._stream_status_payload(state, preview_payload, preview_changed)

    def finish_stream(self, stream_id: str) -> dict[str, Any]:
        with self._streams_lock:
            state = self._must_get_stream_locked(stream_id)
            payload = self._stream_snapshot_payload(state)
            self._streams.pop(stream_id, None)

        result = self.transcribe_pcm(
            audio_bytes=payload["audio_bytes"],
            sample_rate_hz=payload["sample_rate_hz"],
            channels=payload["channels"],
            session_id=payload["session_id"],
            language=payload["language"],
        )
        final_text = result.get("text", "").strip()
        partials = list(payload["partials"])
        if final_text and (not partials or partials[-1] != final_text):
            partials.append(final_text)
        result.update(
            {
                "stream_id": stream_id,
                "turn_id": payload["turn_id"],
                "trace_id": payload["trace_id"],
                "latest_partial": partials[-1] if partials else "",
                "partials": partials,
                "endpoint_reason": "stream_finish",
                "mode": "stream_preview_batch",
            }
        )
        return result

    def close_stream(self, stream_id: str) -> dict[str, Any]:
        with self._streams_lock:
            state = self._streams.pop(stream_id, None)
        if state is None:
            raise ValueError(f"stream {stream_id} not found")
        return {
            "stream_id": stream_id,
            "session_id": state.session_id,
            "turn_id": state.turn_id,
            "trace_id": state.trace_id,
            "status": "closed",
        }

    def _maybe_preview_stream(self, snapshot: dict[str, Any]) -> dict[str, Any] | None:
        if snapshot["duration_ms"] < self.config.stream_preview_min_audio_ms:
            return None
        since_last_preview_ms = int((time.monotonic() - snapshot["last_preview_at_monotonic"]) * 1000)
        if snapshot["last_preview_at_monotonic"] > 0 and since_last_preview_ms < self.config.stream_preview_min_interval_ms:
            return None
        result = self.transcribe_pcm(
            audio_bytes=snapshot["audio_bytes"],
            sample_rate_hz=snapshot["sample_rate_hz"],
            channels=snapshot["channels"],
            session_id=snapshot["session_id"],
            language=snapshot["language"],
        )
        result["mode"] = "stream_preview_batch"
        return result

    def _stream_status_payload(
        self,
        state: StreamState,
        preview_payload: dict[str, Any] | None = None,
        preview_changed: bool = False,
    ) -> dict[str, Any]:
        duration_ms = int(len(state.buffer) / max(state.channels, 1) / 2 / state.sample_rate_hz * 1000)
        payload = {
            "stream_id": state.stream_id,
            "session_id": state.session_id,
            "turn_id": state.turn_id,
            "trace_id": state.trace_id,
            "audio_bytes_total": len(state.buffer),
            "duration_ms": duration_ms,
            "latest_partial": state.last_partial_text,
            "partials": list(state.partials),
            "mode": "stream_preview_batch",
            "language": state.language,
        }
        if preview_payload is not None:
            payload["preview_elapsed_ms"] = int(preview_payload.get("elapsed_ms", 0))
            payload["preview_text"] = str(preview_payload.get("text", "")).strip()
            payload["preview_changed"] = preview_changed
            payload["preview_endpoint_reason"] = str(preview_payload.get("preview_endpoint_reason", "")).strip()
        return payload

    def _preview_endpoint_reason(self, snapshot: dict[str, Any], preview_payload: dict[str, Any]) -> str:
        preview_text = str(preview_payload.get("text", "")).strip()
        if not preview_text:
            return ""
        mean_abs = self._tail_mean_abs_pcm16(
            snapshot["audio_bytes"],
            int(snapshot["sample_rate_hz"]),
            int(snapshot["channels"]),
            self.config.stream_endpoint_tail_ms,
        )
        if mean_abs <= self.config.stream_endpoint_mean_abs_threshold:
            return "preview_tail_silence"
        return ""

    def _tail_mean_abs_pcm16(self, audio_bytes: bytes, sample_rate_hz: int, channels: int, tail_ms: int) -> float:
        if not audio_bytes or sample_rate_hz <= 0 or channels <= 0 or tail_ms <= 0:
            return 0.0
        tail_frames = max(int(sample_rate_hz * tail_ms / 1000), 1)
        tail_bytes = tail_frames * channels * 2
        sample = audio_bytes[-tail_bytes:]
        usable = len(sample) - (len(sample) % 2)
        if usable <= 0:
            return 0.0
        total = 0
        count = 0
        for index in range(0, usable, 2):
            value = int.from_bytes(sample[index : index + 2], byteorder="little", signed=True)
            total += abs(value)
            count += 1
        if count == 0:
            return 0.0
        return total / count

    def _preview_snapshot_locked(self, state: StreamState) -> dict[str, Any]:
        return {
            "session_id": state.session_id,
            "turn_id": state.turn_id,
            "trace_id": state.trace_id,
            "sample_rate_hz": state.sample_rate_hz,
            "channels": state.channels,
            "language": state.language,
            "audio_bytes": bytes(state.buffer),
            "duration_ms": int(len(state.buffer) / max(state.channels, 1) / 2 / state.sample_rate_hz * 1000),
            "last_preview_at_monotonic": state.last_preview_at_monotonic,
        }

    def _stream_snapshot_payload(self, state: StreamState) -> dict[str, Any]:
        return {
            "session_id": state.session_id,
            "turn_id": state.turn_id,
            "trace_id": state.trace_id,
            "sample_rate_hz": state.sample_rate_hz,
            "channels": state.channels,
            "language": state.language,
            "audio_bytes": bytes(state.buffer),
            "partials": list(state.partials),
        }

    def _get_stream(self, stream_id: str) -> StreamState:
        with self._streams_lock:
            return self._must_get_stream_locked(stream_id)

    def _must_get_stream_locked(self, stream_id: str) -> StreamState:
        state = self._streams.get(stream_id)
        if state is None:
            raise ValueError(f"stream {stream_id} not found")
        return state

    def _ensure_model(self) -> tuple[Any, Any]:
        if self._model is not None:
            return self._model, self._postprocess

        with self._lock:
            if self._model is not None:
                return self._model, self._postprocess

            from funasr import AutoModel
            from funasr.utils.postprocess_utils import rich_transcription_postprocess

            self._model = AutoModel(
                model=self.config.model,
                trust_remote_code=self.config.trust_remote_code,
                device=self.config.device,
                disable_update=self.config.disable_update,
            )
            self._postprocess = rich_transcription_postprocess
            self._last_error = ""
            return self._model, self._postprocess

    def _write_temp_wav(self, audio_bytes: bytes, sample_rate_hz: int, channels: int) -> str:
        temp = tempfile.NamedTemporaryFile(prefix="agent-server-funasr-", suffix=".wav", delete=False)
        temp.close()
        with wave.open(temp.name, "wb") as wav_file:
            wav_file.setnchannels(channels)
            wav_file.setsampwidth(2)
            wav_file.setframerate(sample_rate_hz)
            wav_file.writeframes(audio_bytes)
        return temp.name

    def _extract_raw_text(self, result: Any) -> str:
        if isinstance(result, list) and result:
            payload = result[0]
            if isinstance(payload, dict):
                return str(payload.get("text", "")).strip()
        if isinstance(result, dict):
            return str(result.get("text", "")).strip()
        return ""


class FunASRRequestHandler(BaseHTTPRequestHandler):
    server_version = "agent-server-funasr/0.1"

    @property
    def engine(self) -> FunASREngine:
        return self.server.engine  # type: ignore[attr-defined]

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/healthz":
            self._write_json(HTTPStatus.OK, self.engine.health())
            return
        if self.path == "/v1/asr/info":
            self._write_json(
                HTTPStatus.OK,
                {
                    **self.engine.health(),
                    "routes": [
                        "/healthz",
                        "/v1/asr/info",
                        "/v1/asr/transcribe",
                        "/v1/asr/stream/start",
                        "/v1/asr/stream/push",
                        "/v1/asr/stream/finish",
                        "/v1/asr/stream/close",
                    ],
                },
            )
            return
        self._write_json(HTTPStatus.NOT_FOUND, {"error": "route not found"})

    def do_POST(self) -> None:  # noqa: N802
        try:
            payload = self._read_json()
            if self.path == "/v1/asr/transcribe":
                codec = str(payload.get("codec", "pcm16le"))
                if codec != "pcm16le":
                    raise ValueError("only pcm16le is supported in the current worker")
                sample_rate_hz = int(payload.get("sample_rate_hz", 16000))
                channels = int(payload.get("channels", 1))
                session_id = str(payload.get("session_id", ""))
                audio_base64 = str(payload.get("audio_base64", ""))
                audio_bytes = base64.b64decode(audio_base64.encode("utf-8"), validate=True)
                result = self.engine.transcribe_pcm(
                    audio_bytes=audio_bytes,
                    sample_rate_hz=sample_rate_hz,
                    channels=channels,
                    session_id=session_id,
                    language=str(payload.get("language", self.engine.config.language)),
                )
                self._write_json(HTTPStatus.OK, result)
                return
            if self.path == "/v1/asr/stream/start":
                result = self.engine.start_stream(
                    session_id=str(payload.get("session_id", "")),
                    turn_id=str(payload.get("turn_id", "")),
                    trace_id=str(payload.get("trace_id", "")),
                    device_id=str(payload.get("device_id", "")),
                    codec=str(payload.get("codec", "pcm16le")),
                    sample_rate_hz=int(payload.get("sample_rate_hz", 16000)),
                    channels=int(payload.get("channels", 1)),
                    language=str(payload.get("language", self.engine.config.language)),
                )
                self._write_json(HTTPStatus.OK, result)
                return
            if self.path == "/v1/asr/stream/push":
                stream_id = str(payload.get("stream_id", ""))
                audio_base64 = str(payload.get("audio_base64", ""))
                audio_bytes = base64.b64decode(audio_base64.encode("utf-8"), validate=True)
                result = self.engine.push_stream_audio(stream_id, audio_bytes)
                self._write_json(HTTPStatus.OK, result)
                return
            if self.path == "/v1/asr/stream/finish":
                result = self.engine.finish_stream(str(payload.get("stream_id", "")))
                self._write_json(HTTPStatus.OK, result)
                return
            if self.path == "/v1/asr/stream/close":
                result = self.engine.close_stream(str(payload.get("stream_id", "")))
                self._write_json(HTTPStatus.OK, result)
                return
            self._write_json(HTTPStatus.NOT_FOUND, {"error": "route not found"})
        except Exception as exc:  # noqa: BLE001
            self.engine._last_error = str(exc)
            self._write_json(HTTPStatus.BAD_REQUEST, {"error": str(exc)})

    def log_message(self, fmt: str, *args: object) -> None:
        print(f"[funasr-worker] {self.address_string()} - {fmt % args}")

    def _read_json(self) -> dict[str, Any]:
        length = int(self.headers.get("Content-Length", "0"))
        if length <= 0:
            raise ValueError("request body is required")
        raw = self.rfile.read(length)
        payload = json.loads(raw)
        if not isinstance(payload, dict):
            raise ValueError("request body must be a JSON object")
        return payload

    def _write_json(self, status: HTTPStatus, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def build_config() -> WorkerConfig:
    parser = argparse.ArgumentParser(description="Run the local FunASR HTTP worker.")
    parser.add_argument("--host", default=os.getenv("AGENT_SERVER_FUNASR_HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.getenv("AGENT_SERVER_FUNASR_PORT", "8091")))
    parser.add_argument("--model", default=os.getenv("AGENT_SERVER_FUNASR_MODEL", "iic/SenseVoiceSmall"))
    parser.add_argument("--device", default=os.getenv("AGENT_SERVER_FUNASR_DEVICE", "cpu"))
    parser.add_argument("--language", default=os.getenv("AGENT_SERVER_FUNASR_LANGUAGE", "auto"))
    parser.add_argument("--batch-size-s", type=int, default=int(os.getenv("AGENT_SERVER_FUNASR_BATCH_SIZE_S", "60")))
    parser.add_argument(
        "--stream-preview-min-audio-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_AUDIO_MS", "320")),
    )
    parser.add_argument(
        "--stream-preview-min-interval-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_INTERVAL_MS", "240")),
    )
    parser.add_argument(
        "--stream-endpoint-tail-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_ENDPOINT_TAIL_MS", "160")),
    )
    parser.add_argument(
        "--stream-endpoint-mean-abs-threshold",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_ENDPOINT_MEAN_ABS_THRESHOLD", "180")),
    )
    parser.add_argument("--use-itn", action="store_true", default=_env_bool("AGENT_SERVER_FUNASR_USE_ITN", True))
    parser.add_argument(
        "--trust-remote-code",
        action="store_true",
        default=_env_bool("AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE", False),
    )
    parser.add_argument(
        "--disable-update",
        action="store_true",
        default=_env_bool("AGENT_SERVER_FUNASR_DISABLE_UPDATE", True),
    )
    args = parser.parse_args()
    return WorkerConfig(
        host=args.host,
        port=args.port,
        model=args.model,
        device=args.device,
        language=args.language,
        trust_remote_code=bool(args.trust_remote_code),
        disable_update=bool(args.disable_update),
        batch_size_s=args.batch_size_s,
        use_itn=bool(args.use_itn),
        stream_preview_min_audio_ms=args.stream_preview_min_audio_ms,
        stream_preview_min_interval_ms=args.stream_preview_min_interval_ms,
        stream_endpoint_tail_ms=args.stream_endpoint_tail_ms,
        stream_endpoint_mean_abs_threshold=args.stream_endpoint_mean_abs_threshold,
    )


def main() -> None:
    config = build_config()
    engine = FunASREngine(config)
    server = ThreadingHTTPServer((config.host, config.port), FunASRRequestHandler)
    server.engine = engine  # type: ignore[attr-defined]
    print(f"[funasr-worker] listening on http://{config.host}:{config.port}")
    print(json.dumps(asdict(config), ensure_ascii=False))
    server.serve_forever()


def _env_bool(name: str, fallback: bool) -> bool:
    value = os.getenv(name)
    if value is None or value == "":
        return fallback
    return value.strip().lower() in {"1", "true", "yes", "on"}


if __name__ == "__main__":
    main()
