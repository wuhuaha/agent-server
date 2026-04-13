from __future__ import annotations

import os
import unittest
from unittest.mock import patch

from agent_server_workers.funasr_service import FunASREngine, WorkerConfig, _env_bool, build_config


class FunASRServiceTests(unittest.TestCase):
    def make_config(self, **overrides: object) -> WorkerConfig:
        values = {
            "host": "127.0.0.1",
            "port": 8091,
            "model": "iic/SenseVoiceSmall",
            "device": "cpu",
            "language": "auto",
            "trust_remote_code": False,
            "disable_update": True,
            "batch_size_s": 60,
            "use_itn": True,
            "stream_preview_min_audio_ms": 20,
            "stream_preview_min_interval_ms": 0,
            "stream_endpoint_tail_ms": 160,
            "stream_endpoint_mean_abs_threshold": 180,
            "stream_endpoint_vad_provider": "energy",
            "stream_endpoint_vad_threshold": 0.5,
            "stream_endpoint_vad_min_silence_ms": 160,
            "stream_endpoint_vad_speech_pad_ms": 30,
        }
        values.update(overrides)
        return WorkerConfig(**values)

    def test_env_bool(self) -> None:
        os.environ["AGENT_SERVER_FUNASR_TEST_BOOL"] = "true"
        try:
            self.assertTrue(_env_bool("AGENT_SERVER_FUNASR_TEST_BOOL", False))
        finally:
            os.environ.pop("AGENT_SERVER_FUNASR_TEST_BOOL", None)

    def test_extract_raw_text(self) -> None:
        engine = FunASREngine(self.make_config())
        self.assertEqual(engine._extract_raw_text([{"text": "hello"}]), "hello")
        self.assertEqual(engine._extract_raw_text({"text": "world"}), "world")

    def test_build_config_defaults_trust_remote_code_false(self) -> None:
        with patch.dict(os.environ, {}, clear=True):
            with patch("sys.argv", ["funasr_service.py"]):
                config = build_config()
        self.assertFalse(config.trust_remote_code)
        self.assertEqual(config.stream_preview_min_audio_ms, 320)
        self.assertEqual(config.stream_preview_min_interval_ms, 240)
        self.assertEqual(config.stream_endpoint_tail_ms, 160)
        self.assertEqual(config.stream_endpoint_mean_abs_threshold, 180)
        self.assertEqual(config.stream_endpoint_vad_provider, "energy")
        self.assertEqual(config.stream_endpoint_vad_threshold, 0.5)
        self.assertEqual(config.stream_endpoint_vad_min_silence_ms, 160)
        self.assertEqual(config.stream_endpoint_vad_speech_pad_ms, 30)

    def test_stream_lifecycle_tracks_preview_and_final_without_duplicate_partials(self) -> None:
        engine = FunASREngine(self.make_config(language="zh"))
        preview_audio = bytes(640)
        final_audio = bytes(640)
        with patch.object(
            engine,
            "transcribe_pcm",
            side_effect=[
                {"text": "打开", "elapsed_ms": 10, "language": "zh", "mode": "batch"},
                {"text": "打开", "elapsed_ms": 11, "language": "zh", "mode": "batch"},
                {"text": "打开客厅灯", "elapsed_ms": 15, "language": "zh", "mode": "batch"},
            ],
        ) as transcribe:
            started = engine.start_stream(
                session_id="sess-1",
                turn_id="turn-1",
                trace_id="trace-1",
                device_id="device-1",
                codec="pcm16le",
                sample_rate_hz=16000,
                channels=1,
                language="zh",
            )
            stream_id = started["stream_id"]
            self.assertEqual(started["mode"], "stream_preview_batch")

            pushed_once = engine.push_stream_audio(stream_id, preview_audio)
            self.assertEqual(pushed_once["preview_text"], "打开")
            self.assertTrue(pushed_once["preview_changed"])
            self.assertEqual(pushed_once["latest_partial"], "打开")
            self.assertEqual(pushed_once["partials"], ["打开"])
            self.assertEqual(pushed_once["language"], "zh")
            self.assertEqual(pushed_once["preview_endpoint_reason"], "preview_tail_silence")

            pushed_twice = engine.push_stream_audio(stream_id, final_audio)
            self.assertEqual(pushed_twice["preview_text"], "打开")
            self.assertFalse(pushed_twice["preview_changed"])
            self.assertEqual(pushed_twice["partials"], ["打开"])
            self.assertEqual(pushed_twice["preview_endpoint_reason"], "preview_tail_silence")

            finished = engine.finish_stream(stream_id)
            self.assertEqual(finished["text"], "打开客厅灯")
            self.assertEqual(finished["latest_partial"], "打开客厅灯")
            self.assertEqual(finished["partials"], ["打开", "打开客厅灯"])
            self.assertEqual(finished["endpoint_reason"], "stream_finish")
            self.assertEqual(finished["mode"], "stream_preview_batch")
            self.assertEqual(transcribe.call_count, 3)

    def test_preview_endpoint_reason_uses_tail_energy(self) -> None:
        engine = FunASREngine(self.make_config(stream_endpoint_vad_provider="energy"))
        silent = bytes(640)
        noisy = b"".join(int(1200).to_bytes(2, byteorder="little", signed=True) for _ in range(320))

        silent_reason = engine._preview_endpoint_reason(
            {
                "audio_bytes": silent,
                "sample_rate_hz": 16000,
                "channels": 1,
            },
            {"text": "打开客厅灯"},
        )
        noisy_reason = engine._preview_endpoint_reason(
            {
                "audio_bytes": noisy,
                "sample_rate_hz": 16000,
                "channels": 1,
            },
            {"text": "打开客厅灯"},
        )

        self.assertEqual(silent_reason, "preview_tail_silence")
        self.assertEqual(noisy_reason, "")

    def test_preview_endpoint_reason_prefers_silero_when_available(self) -> None:
        engine = FunASREngine(self.make_config(stream_endpoint_vad_provider="silero"))
        with patch.object(
            engine,
            "_preview_endpoint_reason_silero",
            return_value=("preview_silero_vad_silence", True),
        ) as silero, patch.object(engine, "_preview_endpoint_reason_energy", return_value="preview_tail_silence") as energy:
            reason = engine._preview_endpoint_reason(
                {
                    "audio_bytes": bytes(640),
                    "sample_rate_hz": 16000,
                    "channels": 1,
                },
                {"text": "打开客厅灯"},
            )
        self.assertEqual(reason, "preview_silero_vad_silence")
        silero.assert_called_once()
        energy.assert_not_called()

    def test_preview_endpoint_reason_falls_back_to_energy_when_silero_is_unavailable(self) -> None:
        engine = FunASREngine(self.make_config(stream_endpoint_vad_provider="silero"))
        with patch.object(
            engine,
            "_preview_endpoint_reason_silero",
            return_value=("", False),
        ) as silero, patch.object(engine, "_preview_endpoint_reason_energy", return_value="preview_tail_silence") as energy:
            reason = engine._preview_endpoint_reason(
                {
                    "audio_bytes": bytes(640),
                    "sample_rate_hz": 16000,
                    "channels": 1,
                },
                {"text": "打开客厅灯"},
            )
        self.assertEqual(reason, "preview_tail_silence")
        silero.assert_called_once()
        energy.assert_called_once()

    def test_preview_endpoint_reason_does_not_fall_back_when_silero_runs_and_rejects_hint(self) -> None:
        engine = FunASREngine(self.make_config(stream_endpoint_vad_provider="auto"))
        with patch.object(
            engine,
            "_preview_endpoint_reason_silero",
            return_value=("", True),
        ) as silero, patch.object(engine, "_preview_endpoint_reason_energy", return_value="preview_tail_silence") as energy:
            reason = engine._preview_endpoint_reason(
                {
                    "audio_bytes": bytes(640),
                    "sample_rate_hz": 16000,
                    "channels": 1,
                },
                {"text": "打开客厅灯"},
            )
        self.assertEqual(reason, "")
        silero.assert_called_once()
        energy.assert_not_called()

    def test_close_stream_removes_state(self) -> None:
        engine = FunASREngine(self.make_config())
        started = engine.start_stream(
            session_id="sess-2",
            turn_id="turn-2",
            trace_id="trace-2",
            device_id="device-2",
            codec="pcm16le",
            sample_rate_hz=16000,
            channels=1,
            language="auto",
        )
        closed = engine.close_stream(started["stream_id"])
        self.assertEqual(closed["status"], "closed")
        with self.assertRaisesRegex(ValueError, "not found"):
            engine.close_stream(started["stream_id"])


if __name__ == "__main__":
    unittest.main()
