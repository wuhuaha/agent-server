"""Tk desktop app for testing the agent-server realtime bootstrap protocol."""

from __future__ import annotations

import json
from pathlib import Path
import queue
import tempfile
import tkinter as tk
from tkinter import filedialog, messagebox, ttk

try:
    import winsound
except ImportError:  # pragma: no cover - non-Windows platforms
    winsound = None

from .audio import chunk_pcm_bytes, generate_silence, load_pcm_wav, write_pcm_wav
from .protocol import new_session_id
from .transport import DesktopRealtimeClient


class DesktopClientApp(tk.Tk):
    """Desktop debug console for the bootstrap realtime websocket profile."""

    def __init__(self) -> None:
        super().__init__()
        self.title("agent-server desktop client")
        self.geometry("1480x920")
        self.minsize(1280, 820)

        self.event_queue: queue.Queue[dict[str, object]] = queue.Queue()
        self.client = DesktopRealtimeClient(self.event_queue.put)

        self.http_base_var = tk.StringVar(value="http://127.0.0.1:8080")
        self.ws_path_var = tk.StringVar(value="/v1/realtime/ws")
        self.subprotocol_var = tk.StringVar(value="agent-server.realtime.v0")
        self.protocol_version_var = tk.StringVar(value="rtos-ws-v0")
        self.auth_mode_var = tk.StringVar(value="disabled")
        self.turn_mode_var = tk.StringVar(value="client_wakeup_server_vad")
        self.input_audio_var = tk.StringVar(value="pcm16le / 16000 Hz / mono")
        self.output_audio_var = tk.StringVar(value="pcm16le / 16000 Hz / mono")
        self.timeout_var = tk.StringVar(value="idle=30000 ms | max_session=300000 ms")
        self.max_frame_var = tk.StringVar(value="4096")

        self.session_id_var = tk.StringVar(value=new_session_id())
        self.device_id_var = tk.StringVar(value="desktop-debug-001")
        self.client_type_var = tk.StringVar(value="desktop")
        self.firmware_var = tk.StringVar(value="debug-client-0.1.0")
        self.mode_var = tk.StringVar(value="voice")
        self.wake_reason_var = tk.StringVar(value="manual")
        self.client_can_end_var = tk.BooleanVar(value=True)
        self.server_can_end_var = tk.BooleanVar(value=True)
        self.text_input_var = tk.BooleanVar(value=True)
        self.image_input_var = tk.BooleanVar(value=False)
        self.half_duplex_var = tk.BooleanVar(value=False)
        self.local_wake_word_var = tk.BooleanVar(value=False)

        self.text_input_entry_var = tk.StringVar()
        self.wav_path_var = tk.StringVar()
        self.silence_ms_var = tk.StringVar(value="1000")
        self.frame_ms_var = tk.StringVar(value="20")
        self.commit_reason_var = tk.StringVar(value="end_of_speech")
        self.end_reason_var = tk.StringVar(value="client_stop")

        self.connection_state_var = tk.StringVar(value="Disconnected")
        self.active_session_var = tk.StringVar(value="-")
        self.metrics_var = tk.StringVar(value="TX 0 B | RX 0 B | RX audio 0 B")

        self.response_text_buffer = ""
        self.received_audio = bytearray()
        self.last_saved_audio: Path | None = None
        self.sent_bytes = 0
        self.received_bytes = 0

        self._build_ui()
        self.protocol("WM_DELETE_WINDOW", self._on_close)
        self.after(100, self._drain_events)

    def _build_ui(self) -> None:
        self.columnconfigure(0, weight=2)
        self.columnconfigure(1, weight=3)
        self.rowconfigure(0, weight=1)

        left = ttk.Frame(self, padding=10)
        left.grid(row=0, column=0, sticky="nsew")
        left.columnconfigure(0, weight=1)

        right = ttk.Frame(self, padding=(0, 10, 10, 10))
        right.grid(row=0, column=1, sticky="nsew")
        right.columnconfigure(0, weight=1)
        right.rowconfigure(0, weight=1)

        self._build_connection_panel(left).grid(row=0, column=0, sticky="ew", pady=(0, 10))
        self._build_session_panel(left).grid(row=1, column=0, sticky="ew", pady=(0, 10))
        self._build_text_panel(left).grid(row=2, column=0, sticky="ew", pady=(0, 10))
        self._build_audio_panel(left).grid(row=3, column=0, sticky="ew", pady=(0, 10))
        self._build_raw_panel(left).grid(row=4, column=0, sticky="nsew")
        left.rowconfigure(4, weight=1)

        self._build_tabs(right).grid(row=0, column=0, sticky="nsew")

        status = ttk.Frame(self, padding=(10, 0, 10, 10))
        status.grid(row=1, column=0, columnspan=2, sticky="ew")
        status.columnconfigure(1, weight=1)
        ttk.Label(status, textvariable=self.connection_state_var).grid(row=0, column=0, sticky="w")
        ttk.Label(status, textvariable=self.active_session_var).grid(row=0, column=1, sticky="w", padx=(12, 12))
        ttk.Label(status, textvariable=self.metrics_var).grid(row=0, column=2, sticky="e")

    def _build_connection_panel(self, parent: ttk.Frame) -> ttk.LabelFrame:
        frame = ttk.LabelFrame(parent, text="Connection / Discovery", padding=10)
        frame.columnconfigure(1, weight=1)

        ttk.Label(frame, text="HTTP Base").grid(row=0, column=0, sticky="w")
        ttk.Entry(frame, textvariable=self.http_base_var).grid(row=0, column=1, sticky="ew", padx=(8, 8))
        ttk.Button(frame, text="Discover", command=self._discover).grid(row=0, column=2, padx=(0, 6))
        ttk.Button(frame, text="Connect", command=self._connect).grid(row=0, column=3, padx=(0, 6))
        ttk.Button(frame, text="Disconnect", command=self._disconnect).grid(row=0, column=4)

        rows = [
            ("WS Path", self.ws_path_var),
            ("Subprotocol", self.subprotocol_var),
            ("Protocol", self.protocol_version_var),
            ("Auth", self.auth_mode_var),
            ("Turn Mode", self.turn_mode_var),
            ("Input", self.input_audio_var),
            ("Output", self.output_audio_var),
            ("Timeouts", self.timeout_var),
            ("Max Frame", self.max_frame_var),
        ]
        for index, (label, var) in enumerate(rows, start=1):
            ttk.Label(frame, text=label).grid(row=index, column=0, sticky="nw", pady=2)
            ttk.Entry(frame, textvariable=var).grid(row=index, column=1, columnspan=4, sticky="ew", padx=(8, 0), pady=2)

        return frame

    def _build_session_panel(self, parent: ttk.Frame) -> ttk.LabelFrame:
        frame = ttk.LabelFrame(parent, text="Session", padding=10)
        frame.columnconfigure(1, weight=1)
        frame.columnconfigure(3, weight=1)

        fields = [
            ("Session ID", self.session_id_var, 0, 0),
            ("Device ID", self.device_id_var, 1, 0),
            ("Client Type", self.client_type_var, 2, 0),
            ("Firmware", self.firmware_var, 3, 0),
            ("Mode", self.mode_var, 0, 2),
            ("Wake Reason", self.wake_reason_var, 1, 2),
            ("Commit Reason", self.commit_reason_var, 2, 2),
            ("End Reason", self.end_reason_var, 3, 2),
        ]
        for label, var, row, column in fields:
            ttk.Label(frame, text=label).grid(row=row, column=column, sticky="w", pady=2)
            ttk.Entry(frame, textvariable=var).grid(row=row, column=column + 1, sticky="ew", padx=(8, 10), pady=2)

        ttk.Checkbutton(frame, text="Client Can End", variable=self.client_can_end_var).grid(row=4, column=0, sticky="w", pady=(6, 0))
        ttk.Checkbutton(frame, text="Server Can End", variable=self.server_can_end_var).grid(row=4, column=1, sticky="w", pady=(6, 0))
        ttk.Checkbutton(frame, text="Text Input", variable=self.text_input_var).grid(row=4, column=2, sticky="w", pady=(6, 0))
        ttk.Checkbutton(frame, text="Image Input", variable=self.image_input_var).grid(row=4, column=3, sticky="w", pady=(6, 0))
        ttk.Checkbutton(frame, text="Half Duplex", variable=self.half_duplex_var).grid(row=5, column=0, sticky="w")
        ttk.Checkbutton(frame, text="Local Wake Word", variable=self.local_wake_word_var).grid(row=5, column=1, sticky="w")

        buttons = ttk.Frame(frame)
        buttons.grid(row=6, column=0, columnspan=4, sticky="ew", pady=(8, 0))
        ttk.Button(buttons, text="Refresh Session ID", command=self._refresh_session_id).pack(side=tk.LEFT)
        ttk.Button(buttons, text="Start Session", command=self._start_session).pack(side=tk.LEFT, padx=(8, 0))
        ttk.Button(buttons, text="End Session", command=self._end_session).pack(side=tk.LEFT, padx=(8, 0))
        return frame

    def _build_text_panel(self, parent: ttk.Frame) -> ttk.LabelFrame:
        frame = ttk.LabelFrame(parent, text="Text Debug", padding=10)
        frame.columnconfigure(0, weight=1)
        ttk.Entry(frame, textvariable=self.text_input_entry_var).grid(row=0, column=0, sticky="ew")
        buttons = ttk.Frame(frame)
        buttons.grid(row=0, column=1, padx=(8, 0))
        ttk.Button(buttons, text="Send Text", command=self._send_text).pack(side=tk.LEFT)
        ttk.Button(buttons, text="Send /end", command=lambda: self._send_text("/end")).pack(side=tk.LEFT, padx=(8, 0))
        return frame

    def _build_audio_panel(self, parent: ttk.Frame) -> ttk.LabelFrame:
        frame = ttk.LabelFrame(parent, text="Audio Debug", padding=10)
        frame.columnconfigure(1, weight=1)
        frame.columnconfigure(3, weight=1)

        ttk.Label(frame, text="WAV File").grid(row=0, column=0, sticky="w")
        ttk.Entry(frame, textvariable=self.wav_path_var).grid(row=0, column=1, sticky="ew", padx=(8, 8))
        ttk.Button(frame, text="Browse", command=self._browse_wav).grid(row=0, column=2)

        ttk.Label(frame, text="Silence (ms)").grid(row=1, column=0, sticky="w", pady=(6, 0))
        ttk.Entry(frame, textvariable=self.silence_ms_var, width=12).grid(row=1, column=1, sticky="w", padx=(8, 0), pady=(6, 0))
        ttk.Label(frame, text="Frame (ms)").grid(row=1, column=2, sticky="w", pady=(6, 0))
        ttk.Entry(frame, textvariable=self.frame_ms_var, width=12).grid(row=1, column=3, sticky="w", padx=(8, 0), pady=(6, 0))

        buttons = ttk.Frame(frame)
        buttons.grid(row=2, column=0, columnspan=3, sticky="ew", pady=(10, 0))
        ttk.Button(buttons, text="Stream WAV + Commit", command=self._stream_wav).pack(side=tk.LEFT)
        ttk.Button(buttons, text="Send Silence + Commit", command=self._send_silence).pack(side=tk.LEFT, padx=(8, 0))
        ttk.Button(buttons, text="Commit Only", command=self._commit_turn).pack(side=tk.LEFT, padx=(8, 0))
        ttk.Button(buttons, text="Save RX Audio", command=self._save_received_audio).pack(side=tk.LEFT, padx=(8, 0))
        ttk.Button(buttons, text="Play RX Audio", command=self._play_received_audio).pack(side=tk.LEFT, padx=(8, 0))
        return frame

    def _build_raw_panel(self, parent: ttk.Frame) -> ttk.LabelFrame:
        frame = ttk.LabelFrame(parent, text="Raw JSON Event", padding=10)
        frame.columnconfigure(0, weight=1)
        frame.rowconfigure(0, weight=1)

        self.raw_json_text = tk.Text(frame, wrap="none", height=12)
        self.raw_json_text.grid(row=0, column=0, sticky="nsew")
        self.raw_json_text.insert(
            "1.0",
            json.dumps(
                {
                    "type": "session.update",
                    "payload": {
                        "state": "active",
                    },
                },
                ensure_ascii=False,
                indent=2,
            ),
        )
        ttk.Button(frame, text="Send Raw JSON", command=self._send_raw_json).grid(row=1, column=0, sticky="e", pady=(8, 0))
        return frame

    def _build_tabs(self, parent: ttk.Frame) -> ttk.Notebook:
        notebook = ttk.Notebook(parent)

        response_frame = ttk.Frame(notebook, padding=10)
        response_frame.rowconfigure(0, weight=1)
        response_frame.columnconfigure(0, weight=1)
        self.response_text = tk.Text(response_frame, wrap="word")
        self.response_text.grid(row=0, column=0, sticky="nsew")
        notebook.add(response_frame, text="Response Text")

        log_frame = ttk.Frame(notebook, padding=10)
        log_frame.rowconfigure(0, weight=1)
        log_frame.columnconfigure(0, weight=1)
        self.log_text = tk.Text(log_frame, wrap="word")
        self.log_text.grid(row=0, column=0, sticky="nsew")
        notebook.add(log_frame, text="Event Log")

        json_frame = ttk.Frame(notebook, padding=10)
        json_frame.rowconfigure(0, weight=1)
        json_frame.columnconfigure(0, weight=1)
        self.last_json_text = tk.Text(json_frame, wrap="none")
        self.last_json_text.grid(row=0, column=0, sticky="nsew")
        notebook.add(json_frame, text="Last JSON Event")

        return notebook

    def _discover(self) -> None:
        try:
            info = self.client.discover(self.http_base_var.get().strip())
        except Exception as exc:  # noqa: BLE001
            messagebox.showerror("Discovery failed", str(exc))
            self.log(f"[error] discovery failed: {exc}")
            return
        self._apply_discovery(info)
        self.log("[info] discovery updated from server")

    def _apply_discovery(self, info: object) -> None:
        self.ws_path_var.set(info.ws_path)
        self.subprotocol_var.set(info.subprotocol)
        self.protocol_version_var.set(info.protocol_version)
        self.auth_mode_var.set(info.auth_mode)
        self.turn_mode_var.set(info.turn_mode)
        self.input_audio_var.set(
            f"{info.input_codec} / {info.input_sample_rate_hz} Hz / {'mono' if info.input_channels == 1 else info.input_channels}"
        )
        self.output_audio_var.set(
            f"{info.output_codec} / {info.output_sample_rate_hz} Hz / {'mono' if info.output_channels == 1 else info.output_channels}"
        )
        self.timeout_var.set(f"idle={info.idle_timeout_ms} ms | max_session={info.max_session_ms} ms")
        self.max_frame_var.set(str(info.max_frame_bytes))
        self.text_input_var.set(info.allow_text_input)
        self.image_input_var.set(info.allow_image_input)

    def _connect(self) -> None:
        self.client.connect(
            self.http_base_var.get().strip(),
            self.ws_path_var.get().strip(),
            self.subprotocol_var.get().strip(),
        )

    def _disconnect(self) -> None:
        self.client.disconnect()

    def _refresh_session_id(self) -> None:
        self.session_id_var.set(new_session_id())

    def _ensure_session_id(self) -> str:
        session_id = self.session_id_var.get().strip()
        if not session_id:
            session_id = new_session_id()
            self.session_id_var.set(session_id)
        return session_id

    def _start_session(self) -> None:
        if not self.client.connected:
            messagebox.showwarning("Not connected", "Connect the websocket before starting a session.")
            return

        session_id = self._ensure_session_id()
        payload = {
            "protocol_version": self.protocol_version_var.get().strip(),
            "device": {
                "device_id": self.device_id_var.get().strip(),
                "client_type": self.client_type_var.get().strip(),
                "firmware_version": self.firmware_var.get().strip(),
            },
            "audio": {
                "codec": self._input_codec(),
                "sample_rate_hz": self._input_sample_rate_hz(),
                "channels": self._input_channels(),
            },
            "session": {
                "mode": self.mode_var.get().strip(),
                "wake_reason": self.wake_reason_var.get().strip(),
                "client_can_end": self.client_can_end_var.get(),
                "server_can_end": self.server_can_end_var.get(),
            },
            "capabilities": {
                "text_input": self.text_input_var.get(),
                "image_input": self.image_input_var.get(),
                "half_duplex": self.half_duplex_var.get(),
                "local_wake_word": self.local_wake_word_var.get(),
            },
        }
        self.client.start_session(session_id, payload)
        self.active_session_var.set(f"Session {session_id}")
        self.log(f"[tx] session.start {session_id}")

    def _end_session(self) -> None:
        if not self.client.active_session_id:
            messagebox.showwarning("No active session", "Start a session before ending it.")
            return
        self.client.end_session(self.end_reason_var.get().strip() or "client_stop")

    def _send_text(self, override_text: str | None = None) -> None:
        text = override_text if override_text is not None else self.text_input_entry_var.get().strip()
        if not text:
            return
        if not self.client.active_session_id:
            self._start_session()
            if not self.client.connected:
                return
        self.client.send_text(text)
        self.log(f"[tx] text.in {text}")
        if override_text is None:
            self.text_input_entry_var.set("")

    def _browse_wav(self) -> None:
        path = filedialog.askopenfilename(
            title="Select PCM WAV",
            filetypes=[("WAV files", "*.wav"), ("All files", "*.*")],
        )
        if path:
            self.wav_path_var.set(path)

    def _stream_wav(self) -> None:
        if not self.client.active_session_id:
            self._start_session()
            if not self.client.active_session_id:
                return
        path = self.wav_path_var.get().strip()
        if not path:
            messagebox.showwarning("Missing WAV", "Select a PCM WAV file first.")
            return
        try:
            clip = load_pcm_wav(path, self._input_sample_rate_hz(), self._input_channels())
            chunks = chunk_pcm_bytes(
                clip.frames,
                clip.sample_rate_hz,
                clip.channels,
                frame_ms=self._frame_ms(),
            )
        except Exception as exc:  # noqa: BLE001
            messagebox.showerror("Audio load failed", str(exc))
            return
        self.client.stream_audio_chunks(
            chunks,
            frame_interval_ms=self._frame_ms(),
            auto_commit_reason=self.commit_reason_var.get().strip() or "end_of_speech",
        )
        self.log(f"[tx] streamed WAV {path} in {len(chunks)} chunks")

    def _send_silence(self) -> None:
        if not self.client.active_session_id:
            self._start_session()
            if not self.client.active_session_id:
                return
        duration_ms = int(self.silence_ms_var.get().strip())
        clip = generate_silence(duration_ms, self._input_sample_rate_hz(), self._input_channels())
        chunks = chunk_pcm_bytes(
            clip.frames,
            clip.sample_rate_hz,
            clip.channels,
            frame_ms=self._frame_ms(),
        )
        self.client.stream_audio_chunks(
            chunks,
            frame_interval_ms=self._frame_ms(),
            auto_commit_reason=self.commit_reason_var.get().strip() or "end_of_speech",
        )
        self.log(f"[tx] sent {duration_ms} ms silence in {len(chunks)} chunks")

    def _commit_turn(self) -> None:
        self.client.commit_turn(self.commit_reason_var.get().strip() or "end_of_speech")

    def _send_raw_json(self) -> None:
        raw_text = self.raw_json_text.get("1.0", "end").strip()
        if not raw_text:
            return
        self.client.send_raw_json(raw_text)

    def _save_received_audio(self) -> None:
        if not self.received_audio:
            messagebox.showwarning("No audio", "No received audio is buffered yet.")
            return
        default_name = f"{self.client.active_session_id or 'session'}-rx.wav"
        path = filedialog.asksaveasfilename(
            title="Save received audio",
            defaultextension=".wav",
            initialfile=default_name,
            filetypes=[("WAV files", "*.wav"), ("All files", "*.*")],
        )
        if not path:
            return
        write_pcm_wav(path, bytes(self.received_audio), self._output_sample_rate_hz(), self._output_channels())
        self.last_saved_audio = Path(path)
        self.log(f"[info] saved received audio to {path}")

    def _play_received_audio(self) -> None:
        if winsound is None:
            messagebox.showwarning("Unsupported", "Playback currently uses winsound and is only available on Windows.")
            return
        if not self.received_audio:
            messagebox.showwarning("No audio", "No received audio is buffered yet.")
            return
        path = self.last_saved_audio
        if path is None or not path.exists():
            temp_dir = Path(tempfile.gettempdir())
            path = temp_dir / "agent-server-desktop-client-rx.wav"
            write_pcm_wav(path, bytes(self.received_audio), self._output_sample_rate_hz(), self._output_channels())
        winsound.PlaySound(str(path), winsound.SND_FILENAME | winsound.SND_ASYNC)
        self.log(f"[info] playing {path}")

    def _drain_events(self) -> None:
        try:
            while True:
                event = self.event_queue.get_nowait()
                self._handle_event(event)
        except queue.Empty:
            pass
        self.after(100, self._drain_events)

    def _handle_event(self, event: dict[str, object]) -> None:
        kind = str(event.get("kind", "log"))
        if kind == "log":
            self.log(str(event.get("message", "")))
        elif kind == "error":
            self.log(f"[error] {event.get('message', '')}")
        elif kind == "status":
            connected = bool(event.get("connected", False))
            self.connection_state_var.set("Connected" if connected else "Disconnected")
            session_id = event.get("session_id")
            self.active_session_var.set(f"Session {session_id}" if session_id else "Session -")
            message = str(event.get("message", "")).strip()
            if message:
                self.log(f"[status] {message}")
        elif kind == "discovery":
            payload = event.get("payload", {})
            self.last_json_text.delete("1.0", "end")
            self.last_json_text.insert("1.0", self._pretty_json(payload))
        elif kind == "sent_event":
            payload = event.get("payload", {})
            serialized = json.dumps(payload, ensure_ascii=False)
            self.sent_bytes += len(serialized.encode("utf-8"))
            self.last_json_text.delete("1.0", "end")
            self.last_json_text.insert("1.0", self._pretty_json(payload))
            self.log(f"[tx] {payload.get('type', 'event')}")
        elif kind == "received_event":
            payload = event.get("payload", {})
            serialized = json.dumps(payload, ensure_ascii=False)
            self.received_bytes += len(serialized.encode("utf-8"))
            self.last_json_text.delete("1.0", "end")
            self.last_json_text.insert("1.0", self._pretty_json(payload))
            event_type = str(payload.get("type", "event"))
            session_id = payload.get("session_id")
            if isinstance(session_id, str) and session_id:
                self.session_id_var.set(session_id)
                self.active_session_var.set(f"Session {session_id}")
            if event_type == "response.chunk":
                text = str(payload.get("payload", {}).get("text", ""))
                if text:
                    self.response_text.insert("end", text + "\n")
                    self.response_text.see("end")
            elif event_type == "session.end":
                self.log(f"[rx] session ended: {payload.get('payload', {})}")
            else:
                self.log(f"[rx] {event_type}")
        elif kind == "audio_tx":
            self.sent_bytes += int(event.get("chunk_bytes", 0))
            self.log(f"[tx-audio] chunk {event.get('chunk_index')}/{event.get('total_chunks')} {event.get('chunk_bytes')} bytes")
        elif kind == "audio_rx":
            chunk = event.get("payload", b"")
            if isinstance(chunk, (bytes, bytearray)):
                self.received_audio.extend(chunk)
            self.received_bytes += int(event.get("chunk_bytes", 0))
            self.log(f"[rx-audio] {event.get('chunk_bytes')} bytes")
        self._update_metrics()

    def _input_codec(self) -> str:
        return self.input_audio_var.get().split("/", 1)[0].strip()

    def _input_sample_rate_hz(self) -> int:
        if self.client.discovery:
            return self.client.discovery.input_sample_rate_hz
        return 16000

    def _input_channels(self) -> int:
        if self.client.discovery:
            return self.client.discovery.input_channels
        return 1

    def _output_sample_rate_hz(self) -> int:
        if self.client.discovery:
            return self.client.discovery.output_sample_rate_hz
        return 16000

    def _output_channels(self) -> int:
        if self.client.discovery:
            return self.client.discovery.output_channels
        return 1

    def _frame_ms(self) -> int:
        try:
            return max(1, int(self.frame_ms_var.get().strip()))
        except ValueError:
            return 20

    def _update_metrics(self) -> None:
        self.metrics_var.set(
            f"TX {self.sent_bytes} B | RX {self.received_bytes} B | RX audio {len(self.received_audio)} B"
        )

    def log(self, message: str) -> None:
        self.log_text.insert("end", message + "\n")
        self.log_text.see("end")

    def _pretty_json(self, payload: object) -> str:
        return json.dumps(payload, ensure_ascii=False, indent=2)

    def _on_close(self) -> None:
        try:
            self.client.close()
        finally:
            self.destroy()


def main() -> None:
    app = DesktopClientApp()
    app.mainloop()


if __name__ == "__main__":
    main()
