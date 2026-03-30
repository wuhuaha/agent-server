"""Async websocket transport for the desktop realtime debug client."""

from __future__ import annotations

import asyncio
from concurrent.futures import Future
import json
import threading
from typing import Any, Callable, Iterable
from urllib.parse import urljoin
from urllib.request import urlopen

import websockets
from websockets.exceptions import ConnectionClosed

from .protocol import (
    DiscoveryInfo,
    build_event,
    http_base_to_ws_base,
    join_ws_url,
    normalize_raw_event,
)

EventCallback = Callable[[dict[str, Any]], None]


class DesktopRealtimeClient:
    """Background-loop realtime client for the desktop debug UI."""

    def __init__(self, callback: EventCallback) -> None:
        self._callback = callback
        self._loop = asyncio.new_event_loop()
        self._thread = threading.Thread(target=self._run_loop, name="realtime-client", daemon=True)
        self._thread.start()
        self._socket: websockets.ClientConnection | None = None
        self._reader_task: asyncio.Task[None] | None = None
        self._seq = 0
        self._lock = threading.Lock()
        self.active_session_id: str | None = None
        self.connected = False
        self.discovery: DiscoveryInfo | None = None

    def _run_loop(self) -> None:
        asyncio.set_event_loop(self._loop)
        self._loop.run_forever()

    def close(self) -> None:
        if self._loop.is_closed():
            return
        future = self._schedule(self._disconnect())
        try:
            future.result(timeout=2)
        except Exception:  # noqa: BLE001
            pass
        self._loop.call_soon_threadsafe(self._loop.stop)
        self._thread.join(timeout=2)

    def discover(self, http_base: str) -> DiscoveryInfo:
        discovery_url = urljoin(http_base if http_base.endswith("/") else f"{http_base}/", "v1/realtime")
        self._emit("log", message=f"GET {discovery_url}")
        with urlopen(discovery_url, timeout=10) as response:
            payload = json.load(response)
        info = DiscoveryInfo.from_dict(payload)
        self.discovery = info
        self._emit("discovery", payload=payload)
        self._emit(
            "status",
            connected=self.connected,
            session_id=self.active_session_id,
            message="Discovery completed.",
        )
        return info

    def connect(self, http_base: str, ws_path: str, subprotocol: str) -> None:
        ws_url = join_ws_url(http_base_to_ws_base(http_base), ws_path)
        self._schedule(self._connect(ws_url, subprotocol))

    async def _connect(self, ws_url: str, subprotocol: str) -> None:
        if self._socket is not None:
            await self._disconnect()
        self._emit("log", message=f"Connecting to {ws_url}")
        self._socket = await websockets.connect(
            ws_url,
            subprotocols=[subprotocol],
            max_size=None,
            ping_interval=20,
            ping_timeout=20,
        )
        self.connected = True
        self._emit("status", connected=True, session_id=self.active_session_id, message="WebSocket connected.")
        self._reader_task = asyncio.create_task(self._reader())

    def disconnect(self) -> None:
        self._schedule(self._disconnect())

    async def _disconnect(self) -> None:
        if self._reader_task is not None:
            self._reader_task.cancel()
            self._reader_task = None
        if self._socket is not None:
            await self._socket.close()
            self._socket = None
        self.connected = False
        self.active_session_id = None
        self._emit("status", connected=False, session_id=None, message="WebSocket disconnected.")

    def start_session(self, session_id: str, payload: dict[str, Any]) -> None:
        with self._lock:
            self.active_session_id = session_id
        self.send_event("session.start", payload, session_id=session_id)

    def send_text(self, text: str) -> None:
        self.send_event("text.in", {"text": text}, session_id=self.active_session_id)

    def commit_turn(self, reason: str) -> None:
        self.send_event("audio.in.commit", {"reason": reason}, session_id=self.active_session_id)

    def end_session(self, reason: str, message: str = "") -> None:
        self.send_event(
            "session.end",
            {"reason": reason, "message": message},
            session_id=self.active_session_id,
        )

    def send_raw_json(self, raw_text: str) -> None:
        event = normalize_raw_event(raw_text, self._next_seq(), self.active_session_id)
        self._schedule(self._send_json(event))

    def send_event(
        self,
        event_type: str,
        payload: dict[str, Any],
        session_id: str | None = None,
    ) -> None:
        event = build_event(event_type, self._next_seq(), payload, session_id=session_id)
        self._schedule(self._send_json(event))

    def stream_audio_chunks(
        self,
        chunks: Iterable[bytes],
        frame_interval_ms: int,
        auto_commit_reason: str | None = None,
    ) -> None:
        self._schedule(self._stream_audio(list(chunks), frame_interval_ms, auto_commit_reason))

    async def _stream_audio(
        self,
        chunks: list[bytes],
        frame_interval_ms: int,
        auto_commit_reason: str | None,
    ) -> None:
        socket = self._require_socket()
        interval = max(frame_interval_ms, 0) / 1000
        for index, chunk in enumerate(chunks, start=1):
            await socket.send(chunk)
            self._emit(
                "audio_tx",
                chunk_index=index,
                chunk_bytes=len(chunk),
                total_chunks=len(chunks),
            )
            if interval > 0:
                await asyncio.sleep(interval)
        if auto_commit_reason:
            await self._send_json(
                build_event(
                    "audio.in.commit",
                    self._next_seq(),
                    {"reason": auto_commit_reason},
                    session_id=self.active_session_id,
                )
            )

    async def _send_json(self, event: dict[str, Any]) -> None:
        socket = self._require_socket()
        await socket.send(json.dumps(event, ensure_ascii=False))
        self._emit("sent_event", payload=event)

    async def _reader(self) -> None:
        socket = self._require_socket()
        try:
            async for message in socket:
                if isinstance(message, bytes):
                    self._emit("audio_rx", chunk_bytes=len(message), payload=message)
                    continue
                try:
                    event = json.loads(message)
                except json.JSONDecodeError as exc:
                    self._emit("error", message=f"Invalid JSON from server: {exc}")
                    continue
                session_id = event.get("session_id")
                if isinstance(session_id, str) and session_id:
                    self.active_session_id = session_id
                self._emit("received_event", payload=event)
        except asyncio.CancelledError:
            pass
        except ConnectionClosed as exc:
            self._emit("log", message=f"Connection closed: code={exc.code}, reason={exc.reason}")
        finally:
            self.connected = False
            self._socket = None
            self._emit("status", connected=False, session_id=self.active_session_id, message="WebSocket reader stopped.")

    def _schedule(self, coroutine: Any) -> Future[Any]:
        future = asyncio.run_coroutine_threadsafe(coroutine, self._loop)

        def _done(done_future: Future[Any]) -> None:
            try:
                done_future.result()
            except Exception as exc:  # noqa: BLE001
                self._emit("error", message=str(exc))

        future.add_done_callback(_done)
        return future

    def _require_socket(self) -> websockets.ClientConnection:
        if self._socket is None:
            raise RuntimeError("WebSocket is not connected.")
        return self._socket

    def _next_seq(self) -> int:
        with self._lock:
            self._seq += 1
            return self._seq

    def _emit(self, kind: str, **payload: Any) -> None:
        event = {"kind": kind, **payload}
        self._callback(event)
