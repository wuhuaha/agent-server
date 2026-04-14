"""Local HTTP ASR worker backed by FunASR."""

from __future__ import annotations

import argparse
import ast
from array import array
import base64
from dataclasses import asdict, dataclass, field
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib
import json
import os
from pathlib import Path
import sys
import tempfile
import threading
import time
from typing import Any
from uuid import uuid4
import wave

KWS_PREFIX_SEPARATORS = " \t,.;:!?/\\-_\u3001\u3002\uff0c\uff01\uff1f\uff1a"


@dataclass(slots=True)
class WorkerConfig:
    host: str
    port: int
    model: str
    online_model: str
    final_vad_model: str
    final_punc_model: str
    stream_chunk_size: tuple[int, int, int]
    stream_encoder_chunk_look_back: int
    stream_decoder_chunk_look_back: int
    device: str
    language: str
    trust_remote_code: bool
    disable_update: bool
    batch_size_s: int
    use_itn: bool
    final_merge_vad: bool
    final_merge_length_s: int
    kws_enabled: bool
    kws_model: str
    kws_keywords: tuple[str, ...]
    kws_strip_matched_prefix: bool
    kws_min_audio_ms: int
    kws_min_interval_ms: int
    stream_preview_min_audio_ms: int
    stream_preview_min_interval_ms: int
    stream_endpoint_tail_ms: int
    stream_endpoint_mean_abs_threshold: int
    stream_endpoint_vad_provider: str
    stream_endpoint_vad_threshold: float
    stream_endpoint_vad_min_silence_ms: int
    stream_endpoint_vad_speech_pad_ms: int


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
    online_pending: bytearray = field(default_factory=bytearray)
    online_cache: dict[str, Any] = field(default_factory=dict)
    partials: list[str] = field(default_factory=list)
    last_partial_text: str = ""
    last_preview_at_monotonic: float = 0.0
    last_kws_at_monotonic: float = 0.0
    kws_detected: bool = False
    kws_keyword: str = ""
    kws_score: float = 0.0


class FunASREngine:
    """Lazy model wrapper so import and model load stay off startup critical path."""

    def __init__(self, config: WorkerConfig) -> None:
        self.config = config
        self._final_lock = threading.Lock()
        self._final_model: Any | None = None
        self._final_postprocess = None
        self._online_lock = threading.Lock()
        self._online_model: Any | None = None
        self._online_postprocess = None
        self._kws_lock = threading.Lock()
        self._kws_model: Any | None = None
        self._kws_postprocess = None
        self._fsmn_vad_lock = threading.Lock()
        self._fsmn_vad_model: Any | None = None
        self._fsmn_vad_postprocess = None
        self._fsmn_vad_attempted = False
        self._fsmn_vad_error = ""
        self._last_error = ""
        self._vad_lock = threading.Lock()
        self._silero_vad_runtime: tuple[Any, Any] | None = None
        self._silero_vad_attempted = False
        self._silero_vad_error = ""
        self._streams_lock = threading.Lock()
        self._streams: dict[str, StreamState] = {}

    @property
    def last_error(self) -> str:
        return self._last_error

    def health(self) -> dict[str, Any]:
        endpoint_provider = _normalize_stream_endpoint_vad_provider(self.config.stream_endpoint_vad_provider)
        return {
            "status": "ok" if self._final_model is not None else "starting",
            "model_loaded": self._final_model is not None,
            "model": self.config.model,
            "pipeline_mode": self._stream_mode(),
            "online_model": self.config.online_model,
            "final_vad_model": self.config.final_vad_model,
            "final_punc_model": self.config.final_punc_model,
            "kws_enabled": self.config.kws_enabled,
            "kws_model": self.config.kws_model if self.config.kws_enabled else "",
            "kws_keywords": list(self.config.kws_keywords),
            "device": self.config.device,
            "language": self.config.language,
            "stream_chunk_size": list(self.config.stream_chunk_size),
            "stream_encoder_chunk_look_back": self.config.stream_encoder_chunk_look_back,
            "stream_decoder_chunk_look_back": self.config.stream_decoder_chunk_look_back,
            "stream_preview_min_audio_ms": self.config.stream_preview_min_audio_ms,
            "stream_preview_min_interval_ms": self.config.stream_preview_min_interval_ms,
            "stream_endpoint_tail_ms": self.config.stream_endpoint_tail_ms,
            "stream_endpoint_mean_abs_threshold": self.config.stream_endpoint_mean_abs_threshold,
            "stream_endpoint_vad_provider": endpoint_provider,
            "stream_endpoint_vad_runtime_status": self._stream_endpoint_vad_status(endpoint_provider),
            "stream_endpoint_vad_threshold": self.config.stream_endpoint_vad_threshold,
            "stream_endpoint_vad_min_silence_ms": self.config.stream_endpoint_vad_min_silence_ms,
            "stream_endpoint_vad_speech_pad_ms": self.config.stream_endpoint_vad_speech_pad_ms,
            "stream_endpoint_vad_last_error": self._stream_endpoint_vad_last_error(endpoint_provider),
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
                "elapsed_ms": 0,
                "session_id": session_id,
                "model": self.config.model,
                "device": self.config.device,
                "language": effective_language,
                "mode": "batch",
            }
        duration_ms = self._audio_duration_ms(audio_bytes, sample_rate_hz, channels)
        kws_detection = self._run_kws_detection(
            audio_bytes,
            sample_rate_hz,
            channels,
            state=None,
        )
        result = self._run_final_transcription(
            audio_bytes=audio_bytes,
            sample_rate_hz=sample_rate_hz,
            channels=channels,
            session_id=session_id,
            language=effective_language,
        )
        text = self._apply_kws_text_policy(str(result.get("text", "")).strip(), kws_detection)
        result["text"] = text
        result["segments"] = [text] if text else []
        result["duration_ms"] = duration_ms
        if kws_event := self._kws_audio_event(kws_detection):
            result["audio_events"] = self._append_audio_events(result.get("audio_events"), kws_event)
            result["kws_detected"] = True
            result["kws_keyword"] = kws_detection["keyword"]
            result["kws_score"] = kws_detection["score"]
        return result

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
            "mode": self._stream_mode(),
            "status": "started",
        }

    def push_stream_audio(self, stream_id: str, audio_bytes: bytes) -> dict[str, Any]:
        if not audio_bytes:
            state = self._get_stream(stream_id)
            return self._stream_status_payload(state)

        with self._streams_lock:
            state = self._must_get_stream_locked(stream_id)
            state.buffer.extend(audio_bytes)
            state.online_pending.extend(audio_bytes)
            preview_snapshot = self._preview_snapshot_locked(state)

        self._maybe_update_kws_state(state, preview_snapshot)
        preview_payload = self._maybe_preview_stream(state, preview_snapshot)
        preview_changed = False

        with self._streams_lock:
            state = self._must_get_stream_locked(stream_id)
            if preview_payload is not None:
                latest_partial = self._apply_kws_text_policy(
                    str(preview_payload.get("text", "")).strip(),
                    self._kws_snapshot(state),
                )
                preview_payload["text"] = latest_partial
                preview_endpoint_reason = self._preview_endpoint_reason(preview_snapshot, preview_payload)
                if preview_endpoint_reason:
                    preview_payload["preview_endpoint_reason"] = preview_endpoint_reason
                state.last_preview_at_monotonic = time.monotonic()
                if latest_partial != state.last_partial_text:
                    state.last_partial_text = latest_partial
                    if latest_partial:
                        state.partials.append(latest_partial)
                    preview_changed = True
            return self._stream_status_payload(state, preview_payload, preview_changed)

    def finish_stream(self, stream_id: str) -> dict[str, Any]:
        state = self._get_stream(stream_id)
        final_preview = self._flush_online_preview(state)

        with self._streams_lock:
            state = self._must_get_stream_locked(stream_id)
            if final_preview is not None:
                latest_partial = self._apply_kws_text_policy(
                    str(final_preview.get("text", "")).strip(),
                    self._kws_snapshot(state),
                )
                if latest_partial != state.last_partial_text:
                    state.last_partial_text = latest_partial
                    if latest_partial:
                        state.partials.append(latest_partial)
            payload = self._stream_snapshot_payload(state)
            self._streams.pop(stream_id, None)

        result = self._run_final_transcription(
            audio_bytes=payload["audio_bytes"],
            sample_rate_hz=payload["sample_rate_hz"],
            channels=payload["channels"],
            session_id=payload["session_id"],
            language=payload["language"],
        )
        kws_detection = payload["kws"]
        if not kws_detection["detected"]:
            kws_detection = self._run_kws_detection(
                payload["audio_bytes"],
                payload["sample_rate_hz"],
                payload["channels"],
                state=None,
            )
        final_text = self._apply_kws_text_policy(str(result.get("text", "")).strip(), kws_detection)
        result["text"] = final_text
        result["segments"] = [final_text] if final_text else []
        partials = list(payload["partials"])
        if final_text and (not partials or partials[-1] != final_text):
            partials.append(final_text)
        if kws_event := self._kws_audio_event(kws_detection):
            result["audio_events"] = self._append_audio_events(result.get("audio_events"), kws_event)
            result["kws_detected"] = True
            result["kws_keyword"] = kws_detection["keyword"]
            result["kws_score"] = kws_detection["score"]
        result.update(
            {
                "stream_id": stream_id,
                "turn_id": payload["turn_id"],
                "trace_id": payload["trace_id"],
                "latest_partial": partials[-1] if partials else "",
                "partials": partials,
                "endpoint_reason": "stream_finish",
                "mode": self._stream_mode(),
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

    def _maybe_preview_stream(self, state: StreamState, snapshot: dict[str, Any]) -> dict[str, Any] | None:
        if self._online_preview_enabled():
            return self._maybe_preview_stream_online(state, snapshot)
        return self._maybe_preview_stream_batch(snapshot)

    def _maybe_preview_stream_batch(self, snapshot: dict[str, Any]) -> dict[str, Any] | None:
        if snapshot["duration_ms"] < self.config.stream_preview_min_audio_ms:
            return None
        since_last_preview_ms = int((time.monotonic() - snapshot["last_preview_at_monotonic"]) * 1000)
        if snapshot["last_preview_at_monotonic"] > 0 and since_last_preview_ms < self.config.stream_preview_min_interval_ms:
            return None
        result = self._run_final_transcription(
            audio_bytes=snapshot["audio_bytes"],
            sample_rate_hz=snapshot["sample_rate_hz"],
            channels=snapshot["channels"],
            session_id=snapshot["session_id"],
            language=snapshot["language"],
        )
        result["mode"] = self._stream_mode()
        return result

    def _stream_status_payload(
        self,
        state: StreamState,
        preview_payload: dict[str, Any] | None = None,
        preview_changed: bool = False,
    ) -> dict[str, Any]:
        payload = {
            "stream_id": state.stream_id,
            "session_id": state.session_id,
            "turn_id": state.turn_id,
            "trace_id": state.trace_id,
            "audio_bytes_total": len(state.buffer),
            "duration_ms": self._audio_duration_ms(bytes(state.buffer), state.sample_rate_hz, state.channels),
            "latest_partial": state.last_partial_text,
            "partials": list(state.partials),
            "mode": self._stream_mode(),
            "language": state.language,
            "kws_detected": state.kws_detected,
            "kws_keyword": state.kws_keyword,
            "kws_score": state.kws_score,
        }
        if preview_payload is not None:
            payload["preview_elapsed_ms"] = int(preview_payload.get("elapsed_ms", 0))
            payload["preview_text"] = str(preview_payload.get("text", "")).strip()
            payload["preview_changed"] = preview_changed
            payload["preview_endpoint_reason"] = str(preview_payload.get("preview_endpoint_reason", "")).strip()
        return payload

    def _stream_mode(self) -> str:
        if self._online_preview_enabled():
            return "stream_2pass_online_final"
        return "stream_preview_batch"

    def _online_preview_enabled(self) -> bool:
        return bool(self.config.online_model.strip())

    def _maybe_preview_stream_online(self, state: StreamState, snapshot: dict[str, Any]) -> dict[str, Any] | None:
        if snapshot["duration_ms"] < self.config.stream_preview_min_audio_ms:
            return None
        chunk_bytes = self._stream_chunk_bytes(state.sample_rate_hz, state.channels)
        if chunk_bytes <= 0:
            return None

        preview_payload: dict[str, Any] | None = None
        while True:
            with self._streams_lock:
                if len(state.online_pending) < chunk_bytes:
                    break
                chunk = bytes(state.online_pending[:chunk_bytes])
                del state.online_pending[:chunk_bytes]
            candidate = self._run_online_preview(
                chunk,
                sample_rate_hz=state.sample_rate_hz,
                channels=state.channels,
                language=state.language,
                cache=state.online_cache,
                is_final=False,
            )
            if candidate is not None:
                preview_payload = candidate
        return preview_payload

    def _flush_online_preview(self, state: StreamState) -> dict[str, Any] | None:
        if not self._online_preview_enabled():
            return None
        with self._streams_lock:
            pending = bytes(state.online_pending)
            state.online_pending.clear()
            cache_was_used = bool(state.online_cache)
        if not pending and not cache_was_used:
            return None
        return self._run_online_preview(
            pending,
            sample_rate_hz=state.sample_rate_hz,
            channels=state.channels,
            language=state.language,
            cache=state.online_cache,
            is_final=True,
        )

    def _stream_chunk_bytes(self, sample_rate_hz: int, channels: int) -> int:
        chunk_ms = max(self.config.stream_chunk_size[1], 1) * 60
        frames = max(int(sample_rate_hz * chunk_ms / 1000), 1)
        return frames * max(channels, 1) * 2

    def _run_online_preview(
        self,
        audio_bytes: bytes,
        *,
        sample_rate_hz: int,
        channels: int,
        language: str,
        cache: dict[str, Any],
        is_final: bool,
    ) -> dict[str, Any] | None:
        if not self._online_preview_enabled():
            return None
        audio_input = self._pcm16le_bytes_to_mono_float_array(audio_bytes, channels)
        if audio_input is None:
            return None
        if getattr(audio_input, "size", len(audio_input)) == 0 and not is_final:
            return None

        model, postprocess = self._ensure_online_model()
        started_at = time.perf_counter()
        with self._online_lock:
            result = model.generate(
                input=audio_input,
                cache=cache,
                language=language or self.config.language,
                use_itn=self.config.use_itn,
                chunk_size=list(self.config.stream_chunk_size),
                encoder_chunk_look_back=self.config.stream_encoder_chunk_look_back,
                decoder_chunk_look_back=self.config.stream_decoder_chunk_look_back,
                is_final=is_final,
            )
        elapsed_ms = int((time.perf_counter() - started_at) * 1000)
        raw_text = self._extract_raw_text(result)
        text = postprocess(raw_text) if raw_text and postprocess is not None else raw_text
        return {
            "text": text,
            "raw_text": raw_text,
            "elapsed_ms": elapsed_ms,
            "language": language or self.config.language,
            "mode": self._stream_mode(),
        }

    def _maybe_update_kws_state(self, state: StreamState, snapshot: dict[str, Any]) -> None:
        if not self.config.kws_enabled or state.kws_detected:
            return
        if not self.config.kws_keywords or snapshot["duration_ms"] < self.config.kws_min_audio_ms:
            return
        since_last_ms = int((time.monotonic() - state.last_kws_at_monotonic) * 1000)
        if state.last_kws_at_monotonic > 0 and since_last_ms < self.config.kws_min_interval_ms:
            return
        detection = self._run_kws_detection(
            snapshot["audio_bytes"],
            snapshot["sample_rate_hz"],
            snapshot["channels"],
            state=state,
        )
        state.last_kws_at_monotonic = time.monotonic()
        if detection["detected"]:
            state.kws_detected = True
            state.kws_keyword = detection["keyword"]
            state.kws_score = detection["score"]

    def _kws_snapshot(self, state: StreamState) -> dict[str, Any]:
        return {
            "detected": state.kws_detected,
            "keyword": state.kws_keyword,
            "score": state.kws_score,
        }

    def _run_kws_detection(
        self,
        audio_bytes: bytes,
        sample_rate_hz: int,
        channels: int,
        *,
        state: StreamState | None,
    ) -> dict[str, Any]:
        _ = state
        detected = {"detected": False, "keyword": "", "score": 0.0}
        if not self.config.kws_enabled or not self.config.kws_keywords or not audio_bytes:
            return detected
        model, _ = self._ensure_kws_model()
        wav_path = self._write_temp_wav(audio_bytes, sample_rate_hz, channels)
        try:
            with self._kws_lock:
                result = model.generate(
                    input=wav_path,
                    cache={},
                    keywords=",".join(self.config.kws_keywords),
                )
        finally:
            Path(wav_path).unlink(missing_ok=True)
        return self._parse_kws_result(result)

    def _parse_kws_result(self, result: Any) -> dict[str, Any]:
        payloads: list[dict[str, Any]] = []
        if isinstance(result, list):
            payloads = [item for item in result if isinstance(item, dict)]
        elif isinstance(result, dict):
            payloads = [result]
        for payload in payloads:
            for key in ("text", "text2"):
                parsed = self._parse_kws_text(str(payload.get(key, "")).strip())
                if parsed["detected"]:
                    return parsed
        return {"detected": False, "keyword": "", "score": 0.0}

    def _parse_kws_text(self, text: str) -> dict[str, Any]:
        if not text:
            return {"detected": False, "keyword": "", "score": 0.0}
        if text.lower() == "rejected":
            return {"detected": False, "keyword": "", "score": 0.0}
        if not text.lower().startswith("detected "):
            return {"detected": False, "keyword": "", "score": 0.0}
        parts = text.split()
        keyword = parts[1] if len(parts) > 1 else ""
        score = 0.0
        if len(parts) > 2:
            try:
                score = float(parts[2])
            except ValueError:
                score = 0.0
        return {"detected": keyword != "", "keyword": keyword, "score": score}

    def _apply_kws_text_policy(self, text: str, kws: dict[str, Any] | None) -> str:
        cleaned = text.strip()
        if not cleaned:
            return ""
        if not self.config.kws_enabled or not self.config.kws_strip_matched_prefix:
            return cleaned

        candidates: list[str] = []
        if kws is not None:
            keyword = str(kws.get("keyword", "")).strip()
            if keyword:
                candidates.append(keyword)
        candidates.extend(self.config.kws_keywords)

        seen: set[str] = set()
        for candidate in candidates:
            candidate = candidate.strip()
            if not candidate:
                continue
            folded = candidate.casefold()
            if folded in seen:
                continue
            seen.add(folded)
            if cleaned.casefold().startswith(folded):
                return cleaned[len(candidate) :].lstrip(KWS_PREFIX_SEPARATORS)
        return cleaned

    def _kws_audio_event(self, kws: dict[str, Any]) -> str:
        if not kws.get("detected", False):
            return ""
        keyword = str(kws.get("keyword", "")).strip()
        if keyword == "":
            return "kws_detected"
        return f"kws_detected:{keyword}"

    def _append_audio_events(self, base: Any, event: str) -> list[str]:
        values: list[str] = []
        if isinstance(base, list):
            values = [str(item).strip() for item in base if str(item).strip()]
        if event and event not in values:
            values.append(event)
        return values

    def _preview_endpoint_reason(self, snapshot: dict[str, Any], preview_payload: dict[str, Any]) -> str:
        preview_text = str(preview_payload.get("text", "")).strip()
        if not preview_text:
            return ""
        provider = _normalize_stream_endpoint_vad_provider(self.config.stream_endpoint_vad_provider)
        if provider == "none":
            return ""
        if provider == "auto":
            if self.config.final_vad_model.strip():
                fsmn_reason = self._preview_endpoint_reason_fsmn(snapshot)
                if fsmn_reason:
                    return fsmn_reason
            silero_reason, silero_handled = self._preview_endpoint_reason_silero(snapshot)
            if silero_handled:
                return silero_reason
            return self._preview_endpoint_reason_energy(snapshot)
        if provider == "fsmn_vad":
            if not self.config.final_vad_model.strip():
                return self._preview_endpoint_reason_energy(snapshot)
            return self._preview_endpoint_reason_fsmn(snapshot)
        if provider == "silero":
            silero_reason, silero_handled = self._preview_endpoint_reason_silero(snapshot)
            if silero_handled:
                return silero_reason
            return self._preview_endpoint_reason_energy(snapshot)
        if provider == "energy":
            return self._preview_endpoint_reason_energy(snapshot)
        return ""

    def _preview_endpoint_reason_fsmn(self, snapshot: dict[str, Any]) -> str:
        if not self.config.final_vad_model.strip():
            return ""
        try:
            result = self._run_fsmn_vad(snapshot["audio_bytes"], snapshot["sample_rate_hz"], snapshot["channels"])
        except Exception as exc:  # noqa: BLE001
            self._fsmn_vad_error = str(exc)
            return self._preview_endpoint_reason_energy(snapshot)
        segments = self._extract_vad_segments(result)
        if not segments:
            return ""
        last_segment = segments[-1]
        if not isinstance(last_segment, (list, tuple)) or len(last_segment) < 2:
            return ""
        total_duration_ms = int(snapshot["duration_ms"])
        trailing_silence_ms = total_duration_ms - int(last_segment[1])
        if trailing_silence_ms >= self.config.stream_endpoint_vad_min_silence_ms:
            return "preview_fsmn_vad_silence"
        return ""

    def _extract_vad_segments(self, result: Any) -> list[Any]:
        payload: dict[str, Any] | None = None
        if isinstance(result, list) and result and isinstance(result[0], dict):
            payload = result[0]
        elif isinstance(result, dict):
            payload = result
        if payload is None:
            return []
        value = payload.get("value")
        if isinstance(value, str):
            try:
                value = ast.literal_eval(value)
            except (SyntaxError, ValueError):
                return []
        if isinstance(value, list):
            return value
        return []

    def _preview_endpoint_reason_energy(self, snapshot: dict[str, Any]) -> str:
        mean_abs = self._tail_mean_abs_pcm16(
            snapshot["audio_bytes"],
            int(snapshot["sample_rate_hz"]),
            int(snapshot["channels"]),
            self.config.stream_endpoint_tail_ms,
        )
        if mean_abs <= self.config.stream_endpoint_mean_abs_threshold:
            return "preview_tail_silence"
        return ""

    def _preview_endpoint_reason_silero(self, snapshot: dict[str, Any]) -> tuple[str, bool]:
        sample_rate_hz = int(snapshot.get("sample_rate_hz", 0))
        channels = int(snapshot.get("channels", 0))
        audio_bytes = bytes(snapshot.get("audio_bytes", b""))
        if not audio_bytes or sample_rate_hz <= 0 or channels <= 0:
            return "", True
        if sample_rate_hz not in {8000, 16000}:
            return "", False
        runtime = self._ensure_silero_vad_runtime()
        if runtime is None:
            return "", False
        model, get_speech_timestamps = runtime
        try:
            audio_tensor = self._pcm16le_bytes_to_mono_float_tensor(audio_bytes, channels)
            if audio_tensor is None:
                return "", True
            total_samples = int(audio_tensor.numel())
            if total_samples <= 0:
                return "", True
            timestamps = get_speech_timestamps(
                audio_tensor,
                model,
                threshold=self.config.stream_endpoint_vad_threshold,
                sampling_rate=sample_rate_hz,
                min_silence_duration_ms=self.config.stream_endpoint_vad_min_silence_ms,
                speech_pad_ms=self.config.stream_endpoint_vad_speech_pad_ms,
            )
        except Exception as exc:  # noqa: BLE001
            self._silero_vad_error = str(exc)
            return "", False
        if not timestamps:
            return "", True
        last_end = self._speech_timestamp_end(timestamps[-1])
        if last_end <= 0:
            return "", True
        required_silence_samples = max(
            int(sample_rate_hz * self.config.stream_endpoint_vad_min_silence_ms / 1000),
            0,
        )
        if total_samples - last_end >= required_silence_samples:
            return "preview_silero_vad_silence", True
        return "", True

    def _ensure_silero_vad_runtime(self) -> tuple[Any, Any] | None:
        if self._silero_vad_runtime is not None:
            return self._silero_vad_runtime
        if self._silero_vad_attempted:
            return None
        with self._vad_lock:
            if self._silero_vad_runtime is not None:
                return self._silero_vad_runtime
            if self._silero_vad_attempted:
                return None
            self._silero_vad_attempted = True
            try:
                silero_vad = importlib.import_module("silero_vad")
                load_silero_vad = getattr(silero_vad, "load_silero_vad", None)
                get_speech_timestamps = getattr(silero_vad, "get_speech_timestamps", None)
                if load_silero_vad is None or get_speech_timestamps is None:
                    raise RuntimeError("silero_vad missing load_silero_vad/get_speech_timestamps")
                self._silero_vad_runtime = (load_silero_vad(), get_speech_timestamps)
                self._silero_vad_error = ""
            except Exception as exc:  # noqa: BLE001
                self._silero_vad_runtime = None
                self._silero_vad_error = str(exc)
            return self._silero_vad_runtime

    def _stream_endpoint_vad_status(self, provider: str | None = None) -> str:
        provider = _normalize_stream_endpoint_vad_provider(provider or self.config.stream_endpoint_vad_provider)
        if provider in {"none", "energy"}:
            return "not_requested"
        if provider == "fsmn_vad":
            return self._fsmn_vad_status()
        if provider == "silero":
            return self._silero_vad_status()
        if provider == "auto":
            if self.config.final_vad_model.strip():
                return f"fsmn_vad_{self._fsmn_vad_status()}"
            silero_status = self._silero_vad_status()
            if silero_status == "ready":
                return "silero_ready"
            if silero_status == "fallback_energy":
                return "fallback_energy"
            return "lazy"
        return "not_requested"

    def _stream_endpoint_vad_last_error(self, provider: str | None = None) -> str:
        provider = _normalize_stream_endpoint_vad_provider(provider or self.config.stream_endpoint_vad_provider)
        if provider in {"auto", "fsmn_vad"} and self._fsmn_vad_error:
            return self._fsmn_vad_error
        return self._silero_vad_error

    def _fsmn_vad_status(self) -> str:
        if not self.config.final_vad_model.strip():
            return "not_configured"
        if self._fsmn_vad_model is not None:
            return "ready"
        if self._fsmn_vad_attempted and self._fsmn_vad_error:
            return "fallback_energy"
        return "lazy"

    def _silero_vad_status(self) -> str:
        if self._silero_vad_runtime is not None:
            return "ready"
        if self._silero_vad_attempted:
            return "fallback_energy"
        return "lazy"

    def _pcm16le_bytes_to_mono_float_array(self, audio_bytes: bytes, channels: int) -> Any | None:
        if channels <= 0:
            return None
        try:
            import numpy as np
        except Exception as exc:  # noqa: BLE001
            self._last_error = str(exc)
            return None
        usable = len(audio_bytes) - (len(audio_bytes) % 2)
        if usable <= 0:
            return np.zeros((0,), dtype=np.float32)
        samples = np.frombuffer(audio_bytes[:usable], dtype="<i2").astype(np.float32)
        if channels > 1:
            frame_count = len(samples) // channels
            if frame_count <= 0:
                return np.zeros((0,), dtype=np.float32)
            samples = samples[: frame_count * channels].reshape(frame_count, channels).mean(axis=1)
        return samples / 32768.0

    def _pcm16le_bytes_to_mono_float_tensor(self, audio_bytes: bytes, channels: int) -> Any | None:
        usable = len(audio_bytes) - (len(audio_bytes) % 2)
        if usable <= 0 or channels <= 0:
            return None
        try:
            import torch
        except Exception as exc:  # noqa: BLE001
            self._silero_vad_error = str(exc)
            return None
        samples = array("h")
        samples.frombytes(audio_bytes[:usable])
        if sys.byteorder != "little":
            samples.byteswap()
        mono_samples: list[float]
        if channels == 1:
            mono_samples = [float(sample) for sample in samples]
        else:
            mono_samples = []
            for index in range(0, len(samples), channels):
                frame = samples[index : index + channels]
                if len(frame) == 0:
                    continue
                mono_samples.append(float(sum(frame)) / float(len(frame)))
        if not mono_samples:
            return None
        return torch.tensor(mono_samples, dtype=torch.float32) / 32768.0

    def _speech_timestamp_end(self, payload: Any) -> int:
        if isinstance(payload, dict):
            for key in ("end", "stop"):
                value = payload.get(key)
                if value is not None:
                    return int(value)
            return 0
        if isinstance(payload, (list, tuple)) and len(payload) >= 2:
            return int(payload[1])
        return 0

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
            "duration_ms": self._audio_duration_ms(bytes(state.buffer), state.sample_rate_hz, state.channels),
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
            "kws": self._kws_snapshot(state),
        }

    def _get_stream(self, stream_id: str) -> StreamState:
        with self._streams_lock:
            return self._must_get_stream_locked(stream_id)

    def _must_get_stream_locked(self, stream_id: str) -> StreamState:
        state = self._streams.get(stream_id)
        if state is None:
            raise ValueError(f"stream {stream_id} not found")
        return state

    def _run_final_transcription(
        self,
        *,
        audio_bytes: bytes,
        sample_rate_hz: int,
        channels: int,
        session_id: str,
        language: str,
    ) -> dict[str, Any]:
        effective_language = language or self.config.language
        duration_ms = self._audio_duration_ms(audio_bytes, sample_rate_hz, channels)
        if not audio_bytes:
            return {
                "text": "",
                "raw_text": "",
                "segments": [],
                "duration_ms": duration_ms,
                "elapsed_ms": 0,
                "session_id": session_id,
                "model": self.config.model,
                "device": self.config.device,
                "language": effective_language,
                "mode": "batch",
            }
        model, postprocess = self._ensure_final_model()
        wav_path = self._write_temp_wav(audio_bytes, sample_rate_hz, channels)
        generate_kwargs: dict[str, Any] = {
            "input": wav_path,
            "cache": {},
            "language": effective_language,
            "use_itn": self.config.use_itn,
            "batch_size_s": self.config.batch_size_s,
        }
        if self.config.final_vad_model.strip():
            generate_kwargs["merge_vad"] = self.config.final_merge_vad
            generate_kwargs["merge_length_s"] = self.config.final_merge_length_s
        try:
            started_at = time.perf_counter()
            with self._final_lock:
                result = model.generate(**generate_kwargs)
            elapsed_ms = int((time.perf_counter() - started_at) * 1000)
        finally:
            Path(wav_path).unlink(missing_ok=True)
        raw_text = self._extract_raw_text(result)
        text = postprocess(raw_text) if raw_text and postprocess is not None else raw_text
        return {
            "text": text,
            "raw_text": raw_text,
            "segments": [text] if text else [],
            "duration_ms": duration_ms,
            "elapsed_ms": elapsed_ms,
            "session_id": session_id,
            "model": self.config.model,
            "device": self.config.device,
            "language": effective_language,
            "mode": "batch",
        }

    def _run_fsmn_vad(self, audio_bytes: bytes, sample_rate_hz: int, channels: int) -> Any:
        if not self.config.final_vad_model.strip() or not audio_bytes:
            return []
        model, _ = self._ensure_fsmn_vad_model()
        wav_path = self._write_temp_wav(audio_bytes, sample_rate_hz, channels)
        try:
            with self._fsmn_vad_lock:
                return model.generate(input=wav_path, cache={})
        finally:
            Path(wav_path).unlink(missing_ok=True)

    def _ensure_final_model(self) -> tuple[Any, Any]:
        if self._final_model is not None:
            return self._final_model, self._final_postprocess
        with self._final_lock:
            if self._final_model is not None:
                return self._final_model, self._final_postprocess
            from funasr import AutoModel
            from funasr.utils.postprocess_utils import rich_transcription_postprocess

            kwargs = self._base_model_kwargs(self.config.model)
            if self.config.final_vad_model.strip():
                kwargs["vad_model"] = self.config.final_vad_model.strip()
            if self.config.final_punc_model.strip():
                kwargs["punc_model"] = self.config.final_punc_model.strip()
            try:
                self._final_model = AutoModel(**kwargs)
                self._final_postprocess = rich_transcription_postprocess
                self._last_error = ""
            except Exception as exc:  # noqa: BLE001
                self._last_error = str(exc)
                raise
            return self._final_model, self._final_postprocess

    def _ensure_online_model(self) -> tuple[Any, Any]:
        if self._online_model is not None:
            return self._online_model, self._online_postprocess
        model_name = self.config.online_model.strip()
        if model_name == "":
            raise ValueError("online preview requested but AGENT_SERVER_FUNASR_ONLINE_MODEL is empty")
        with self._online_lock:
            if self._online_model is not None:
                return self._online_model, self._online_postprocess
            from funasr import AutoModel
            from funasr.utils.postprocess_utils import rich_transcription_postprocess

            try:
                self._online_model = AutoModel(**self._base_model_kwargs(model_name))
                self._online_postprocess = rich_transcription_postprocess
                self._last_error = ""
            except Exception as exc:  # noqa: BLE001
                self._last_error = str(exc)
                raise
            return self._online_model, self._online_postprocess

    def _ensure_kws_model(self) -> tuple[Any, Any]:
        if self._kws_model is not None:
            return self._kws_model, self._kws_postprocess
        model_name = self.config.kws_model.strip()
        if model_name == "":
            raise ValueError("KWS is enabled but AGENT_SERVER_FUNASR_KWS_MODEL is empty")
        with self._kws_lock:
            if self._kws_model is not None:
                return self._kws_model, self._kws_postprocess
            from funasr import AutoModel

            try:
                self._kws_model = AutoModel(**self._base_model_kwargs(model_name))
                self._kws_postprocess = None
                self._last_error = ""
            except Exception as exc:  # noqa: BLE001
                self._last_error = str(exc)
                raise
            return self._kws_model, self._kws_postprocess

    def _ensure_fsmn_vad_model(self) -> tuple[Any, Any]:
        if self._fsmn_vad_model is not None:
            return self._fsmn_vad_model, self._fsmn_vad_postprocess
        model_name = self.config.final_vad_model.strip()
        if model_name == "":
            raise ValueError("FSMN VAD requested but AGENT_SERVER_FUNASR_FINAL_VAD_MODEL is empty")
        with self._fsmn_vad_lock:
            if self._fsmn_vad_model is not None:
                return self._fsmn_vad_model, self._fsmn_vad_postprocess
            from funasr import AutoModel

            self._fsmn_vad_attempted = True
            try:
                self._fsmn_vad_model = AutoModel(**self._base_model_kwargs(model_name))
                self._fsmn_vad_postprocess = None
                self._fsmn_vad_error = ""
                self._last_error = ""
            except Exception as exc:  # noqa: BLE001
                self._fsmn_vad_error = str(exc)
                self._last_error = str(exc)
                raise
            return self._fsmn_vad_model, self._fsmn_vad_postprocess

    def _base_model_kwargs(self, model_name: str) -> dict[str, Any]:
        return {
            "model": model_name,
            "trust_remote_code": self.config.trust_remote_code,
            "device": self.config.device,
            "disable_update": self.config.disable_update,
        }

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

    def _audio_duration_ms(self, audio_bytes: bytes, sample_rate_hz: int, channels: int) -> int:
        if sample_rate_hz <= 0 or channels <= 0:
            return 0
        return int(len(audio_bytes) / channels / 2 / sample_rate_hz * 1000)


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
    parser.add_argument("--online-model", default=os.getenv("AGENT_SERVER_FUNASR_ONLINE_MODEL", ""))
    parser.add_argument("--final-vad-model", default=os.getenv("AGENT_SERVER_FUNASR_FINAL_VAD_MODEL", ""))
    parser.add_argument("--final-punc-model", default=os.getenv("AGENT_SERVER_FUNASR_FINAL_PUNC_MODEL", ""))
    parser.add_argument(
        "--stream-chunk-size",
        default=os.getenv("AGENT_SERVER_FUNASR_STREAM_CHUNK_SIZE", "0,10,5"),
    )
    parser.add_argument(
        "--stream-encoder-chunk-look-back",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_ENCODER_CHUNK_LOOK_BACK", "4")),
    )
    parser.add_argument(
        "--stream-decoder-chunk-look-back",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_DECODER_CHUNK_LOOK_BACK", "1")),
    )
    parser.add_argument("--device", default=os.getenv("AGENT_SERVER_FUNASR_DEVICE", "cpu"))
    parser.add_argument("--language", default=os.getenv("AGENT_SERVER_FUNASR_LANGUAGE", "auto"))
    parser.add_argument("--batch-size-s", type=int, default=int(os.getenv("AGENT_SERVER_FUNASR_BATCH_SIZE_S", "60")))
    parser.add_argument(
        "--final-merge-length-s",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_FINAL_MERGE_LENGTH_S", "15")),
    )
    parser.add_argument(
        "--kws-model",
        default=os.getenv("AGENT_SERVER_FUNASR_KWS_MODEL", "fsmn-kws"),
    )
    parser.add_argument(
        "--kws-keywords",
        default=os.getenv("AGENT_SERVER_FUNASR_KWS_KEYWORDS", ""),
    )
    parser.add_argument(
        "--kws-min-audio-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_KWS_MIN_AUDIO_MS", "480")),
    )
    parser.add_argument(
        "--kws-min-interval-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_KWS_MIN_INTERVAL_MS", "400")),
    )
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
    parser.add_argument(
        "--stream-endpoint-vad-provider",
        default=os.getenv("AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER", "energy"),
    )
    parser.add_argument(
        "--stream-endpoint-vad-threshold",
        type=float,
        default=float(os.getenv("AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_THRESHOLD", "0.5")),
    )
    parser.add_argument(
        "--stream-endpoint-vad-min-silence-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_MIN_SILENCE_MS", "160")),
    )
    parser.add_argument(
        "--stream-endpoint-vad-speech-pad-ms",
        type=int,
        default=int(os.getenv("AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_SPEECH_PAD_MS", "30")),
    )
    _add_bool_argument(parser, "use-itn", "AGENT_SERVER_FUNASR_USE_ITN", True)
    _add_bool_argument(parser, "trust-remote-code", "AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE", False)
    _add_bool_argument(parser, "disable-update", "AGENT_SERVER_FUNASR_DISABLE_UPDATE", True)
    _add_bool_argument(parser, "final-merge-vad", "AGENT_SERVER_FUNASR_FINAL_MERGE_VAD", True)
    _add_bool_argument(parser, "kws-enabled", "AGENT_SERVER_FUNASR_KWS_ENABLED", False)
    _add_bool_argument(
        parser,
        "kws-strip-matched-prefix",
        "AGENT_SERVER_FUNASR_KWS_STRIP_MATCHED_PREFIX",
        True,
    )
    args = parser.parse_args()
    return WorkerConfig(
        host=args.host,
        port=args.port,
        model=str(args.model).strip(),
        online_model=str(args.online_model).strip(),
        final_vad_model=str(args.final_vad_model).strip(),
        final_punc_model=str(args.final_punc_model).strip(),
        stream_chunk_size=_parse_chunk_size(args.stream_chunk_size),
        stream_encoder_chunk_look_back=max(int(args.stream_encoder_chunk_look_back), 0),
        stream_decoder_chunk_look_back=max(int(args.stream_decoder_chunk_look_back), 0),
        device=args.device,
        language=args.language,
        trust_remote_code=bool(args.trust_remote_code),
        disable_update=bool(args.disable_update),
        batch_size_s=max(int(args.batch_size_s), 1),
        use_itn=bool(args.use_itn),
        final_merge_vad=bool(args.final_merge_vad),
        final_merge_length_s=max(int(args.final_merge_length_s), 1),
        kws_enabled=bool(args.kws_enabled),
        kws_model=str(args.kws_model).strip(),
        kws_keywords=_parse_keywords(args.kws_keywords),
        kws_strip_matched_prefix=bool(args.kws_strip_matched_prefix),
        kws_min_audio_ms=max(int(args.kws_min_audio_ms), 0),
        kws_min_interval_ms=max(int(args.kws_min_interval_ms), 0),
        stream_preview_min_audio_ms=max(int(args.stream_preview_min_audio_ms), 0),
        stream_preview_min_interval_ms=max(int(args.stream_preview_min_interval_ms), 0),
        stream_endpoint_tail_ms=max(int(args.stream_endpoint_tail_ms), 0),
        stream_endpoint_mean_abs_threshold=max(int(args.stream_endpoint_mean_abs_threshold), 0),
        stream_endpoint_vad_provider=_normalize_stream_endpoint_vad_provider(args.stream_endpoint_vad_provider),
        stream_endpoint_vad_threshold=float(args.stream_endpoint_vad_threshold),
        stream_endpoint_vad_min_silence_ms=max(int(args.stream_endpoint_vad_min_silence_ms), 0),
        stream_endpoint_vad_speech_pad_ms=max(int(args.stream_endpoint_vad_speech_pad_ms), 0),
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


def _add_bool_argument(parser: argparse.ArgumentParser, name: str, env_name: str, fallback: bool) -> None:
    default = _env_bool(env_name, fallback)
    action = getattr(argparse, "BooleanOptionalAction", None)
    if action is not None:
        parser.add_argument(f"--{name}", default=default, action=action)
        return
    dest = name.replace("-", "_")
    parser.add_argument(f"--{name}", dest=dest, action="store_true", default=default)
    parser.add_argument(f"--no-{name}", dest=dest, action="store_false")


def _parse_chunk_size(value: Any) -> tuple[int, int, int]:
    if isinstance(value, (list, tuple)):
        parts = [int(part) for part in value]
    else:
        text = str(value).strip().strip("[]()")
        normalized = text.replace("/", ",").replace(" ", ",")
        parts = [int(part) for part in normalized.split(",") if part.strip()]
    if len(parts) != 3:
        raise ValueError("stream chunk size must contain exactly 3 integers")
    return tuple(max(int(part), 0) for part in parts)  # type: ignore[return-value]


def _parse_keywords(value: Any) -> tuple[str, ...]:
    if isinstance(value, (list, tuple)):
        raw_items = [str(item) for item in value]
    else:
        raw_items = str(value).replace("\u3001", ",").replace("\uff0c", ",").split(",")
    values: list[str] = []
    seen: set[str] = set()
    for item in raw_items:
        keyword = item.strip()
        if keyword == "":
            continue
        folded = keyword.casefold()
        if folded in seen:
            continue
        seen.add(folded)
        values.append(keyword)
    return tuple(values)


def _normalize_stream_endpoint_vad_provider(value: str) -> str:
    candidate = value.strip().lower().replace("-", "_")
    if candidate in {"", "energy"}:
        return "energy"
    if candidate in {"auto", "silero", "none", "fsmn_vad"}:
        return candidate
    return "energy"


if __name__ == "__main__":
    main()
