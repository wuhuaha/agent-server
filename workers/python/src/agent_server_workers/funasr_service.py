"""Local HTTP ASR worker backed by FunASR."""

from __future__ import annotations

import argparse
import base64
from dataclasses import asdict, dataclass
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import tempfile
import threading
import time
from typing import Any
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


class FunASREngine:
    """Lazy model wrapper so import and model load stay off startup critical path."""

    def __init__(self, config: WorkerConfig) -> None:
        self.config = config
        self._lock = threading.Lock()
        self._model: Any | None = None
        self._postprocess = None
        self._last_error = ""

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
            "last_error": self._last_error,
        }

    def transcribe_pcm(
        self,
        audio_bytes: bytes,
        sample_rate_hz: int,
        channels: int,
        session_id: str = "",
    ) -> dict[str, Any]:
        if not audio_bytes:
            return {
                "text": "",
                "raw_text": "",
                "segments": [],
                "duration_ms": 0,
                "session_id": session_id,
                "model": self.config.model,
                "device": self.config.device,
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
                    language=self.config.language,
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
        }

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
                    "routes": ["/healthz", "/v1/asr/info", "/v1/asr/transcribe"],
                },
            )
            return
        self._write_json(HTTPStatus.NOT_FOUND, {"error": "route not found"})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/v1/asr/transcribe":
            self._write_json(HTTPStatus.NOT_FOUND, {"error": "route not found"})
            return
        try:
            payload = self._read_json()
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
            )
            self._write_json(HTTPStatus.OK, result)
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
