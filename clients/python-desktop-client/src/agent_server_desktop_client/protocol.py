"""Protocol helpers for the desktop realtime debug client."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime
import json
from typing import Any
from urllib.parse import urljoin, urlparse, urlunparse
from uuid import uuid4


def utc_timestamp() -> str:
    """Return an RFC3339-like UTC timestamp with second precision."""
    return datetime.now(UTC).isoformat(timespec="seconds").replace("+00:00", "Z")


def new_session_id() -> str:
    """Return a short debug-oriented session identifier."""
    return f"sess_{uuid4().hex[:16]}"


def build_event(
    event_type: str,
    seq: int,
    payload: dict[str, Any] | None = None,
    session_id: str | None = None,
) -> dict[str, Any]:
    """Build a control event using the shared envelope shape."""
    event: dict[str, Any] = {
        "type": event_type,
        "seq": seq,
        "ts": utc_timestamp(),
        "payload": payload or {},
    }
    if session_id:
        event["session_id"] = session_id
    return event


def normalize_raw_event(
    raw_text: str,
    next_seq: int,
    active_session_id: str | None,
) -> dict[str, Any]:
    """Normalize user-provided JSON into a valid outbound event."""
    parsed = json.loads(raw_text)
    if not isinstance(parsed, dict):
        raise ValueError("Raw JSON event must be an object.")
    parsed.setdefault("seq", next_seq)
    parsed.setdefault("ts", utc_timestamp())
    parsed.setdefault("payload", {})
    if not isinstance(parsed["payload"], dict):
        raise ValueError("Event payload must be an object.")
    if active_session_id and "session_id" not in parsed:
        parsed["session_id"] = active_session_id
    if "type" not in parsed:
        raise ValueError("Event object must contain a type field.")
    return parsed


def http_base_to_ws_base(http_base: str) -> str:
    """Convert an HTTP base URL to the matching WS base URL."""
    parsed = urlparse(http_base)
    if parsed.scheme not in {"http", "https"}:
        raise ValueError("HTTP base must use http or https.")
    scheme = "wss" if parsed.scheme == "https" else "ws"
    return urlunparse((scheme, parsed.netloc, parsed.path.rstrip("/"), "", "", ""))


def join_ws_url(base: str, path: str) -> str:
    """Join a base URL and path while preserving ws/wss schemes."""
    normalized_base = base if base.endswith("/") else f"{base}/"
    normalized_path = path[1:] if path.startswith("/") else path
    return urljoin(normalized_base, normalized_path)


@dataclass(slots=True)
class DiscoveryInfo:
    """Normalized view of the realtime discovery endpoint."""

    protocol_version: str
    ws_path: str
    subprotocol: str
    auth_mode: str
    turn_mode: str
    input_codec: str
    input_sample_rate_hz: int
    input_channels: int
    output_codec: str
    output_sample_rate_hz: int
    output_channels: int
    allow_opus: bool
    allow_text_input: bool
    allow_image_input: bool
    idle_timeout_ms: int
    max_session_ms: int
    max_frame_bytes: int
    protocol_doc: str
    device_profile_doc: str

    @classmethod
    def from_dict(cls, payload: dict[str, Any]) -> "DiscoveryInfo":
        """Build a normalized discovery object from endpoint JSON."""
        input_audio = payload.get("input_audio", {})
        output_audio = payload.get("output_audio", {})
        capabilities = payload.get("capabilities", {})
        return cls(
            protocol_version=str(payload.get("protocol_version", "rtos-ws-v0")),
            ws_path=str(payload.get("ws_path", "/v1/realtime/ws")),
            subprotocol=str(payload.get("subprotocol", "agent-server.realtime.v0")),
            auth_mode=str(payload.get("auth_mode", "disabled")),
            turn_mode=str(payload.get("turn_mode", "client_wakeup_server_vad")),
            input_codec=str(input_audio.get("codec", "pcm16le")),
            input_sample_rate_hz=int(input_audio.get("sample_rate_hz", 16000)),
            input_channels=int(input_audio.get("channels", 1)),
            output_codec=str(output_audio.get("codec", "pcm16le")),
            output_sample_rate_hz=int(output_audio.get("sample_rate_hz", 16000)),
            output_channels=int(output_audio.get("channels", 1)),
            allow_opus=bool(capabilities.get("allow_opus", False)),
            allow_text_input=bool(capabilities.get("allow_text_input", True)),
            allow_image_input=bool(capabilities.get("allow_image_input", False)),
            idle_timeout_ms=int(payload.get("idle_timeout_ms", 30000)),
            max_session_ms=int(payload.get("max_session_ms", 300000)),
            max_frame_bytes=int(payload.get("max_frame_bytes", 4096)),
            protocol_doc=str(payload.get("protocol_doc", "")),
            device_profile_doc=str(payload.get("device_profile_doc", "")),
        )
