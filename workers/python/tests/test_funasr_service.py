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
            "online_model": "",
            "final_vad_model": "",
            "final_punc_model": "",
            "stream_chunk_size": (0, 10, 5),
            "stream_encoder_chunk_look_back": 4,
            "stream_decoder_chunk_look_back": 1,
            "device": "cpu",
            "language": "auto",
            "trust_remote_code": False,
            "disable_update": True,
            "batch_size_s": 60,
            "use_itn": True,
            "final_merge_vad": True,
            "final_merge_length_s": 15,
            "kws_enabled": False,
            "kws_model": "fsmn-kws",
            "kws_keywords": (),
            "kws_strip_matched_prefix": True,
            "kws_min_audio_ms": 480,
            "kws_min_interval_ms": 400,
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

    def test_build_config_defaults_include_2pass_and_kws_flags(self) -> None:
        with patch.dict(os.environ, {}, clear=True):
            with patch("sys.argv", ["funasr_service.py"]):
                config = build_config()
        self.assertFalse(config.trust_remote_code)
        self.assertEqual(config.online_model, "")
        self.assertEqual(config.final_vad_model, "")
        self.assertEqual(config.final_punc_model, "")
        self.assertEqual(config.stream_chunk_size, (0, 10, 5))
        self.assertEqual(config.stream_encoder_chunk_look_back, 4)
        self.assertEqual(config.stream_decoder_chunk_look_back, 1)
        self.assertTrue(config.final_merge_vad)
        self.assertEqual(config.final_merge_length_s, 15)
        self.assertFalse(config.kws_enabled)
        self.assertEqual(config.kws_model, "fsmn-kws")
        self.assertEqual(config.kws_keywords, ())
        self.assertTrue(config.kws_strip_matched_prefix)
        self.assertEqual(config.kws_min_audio_ms, 480)
        self.assertEqual(config.kws_min_interval_ms, 400)
        self.assertEqual(config.stream_preview_min_audio_ms, 320)
        self.assertEqual(config.stream_preview_min_interval_ms, 240)
        self.assertEqual(config.stream_endpoint_vad_provider, "energy")
        self.assertEqual(config.stream_endpoint_vad_threshold, 0.5)
        self.assertEqual(config.stream_endpoint_vad_min_silence_ms, 160)
        self.assertEqual(config.stream_endpoint_vad_speech_pad_ms, 30)

    def test_build_config_parses_online_kws_and_fsmn_vad_envs(self) -> None:
        env = {
            "AGENT_SERVER_FUNASR_ONLINE_MODEL": "paraformer-zh-streaming",
            "AGENT_SERVER_FUNASR_FINAL_VAD_MODEL": "fsmn-vad",
            "AGENT_SERVER_FUNASR_FINAL_PUNC_MODEL": "ct-punc",
            "AGENT_SERVER_FUNASR_STREAM_CHUNK_SIZE": "0,8,4",
            "AGENT_SERVER_FUNASR_KWS_ENABLED": "true",
            "AGENT_SERVER_FUNASR_KWS_KEYWORDS": "你好小智, 小智同学",
            "AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER": "fsmn-vad",
        }
        with patch.dict(os.environ, env, clear=True):
            with patch("sys.argv", ["funasr_service.py"]):
                config = build_config()
        self.assertEqual(config.online_model, "paraformer-zh-streaming")
        self.assertEqual(config.final_vad_model, "fsmn-vad")
        self.assertEqual(config.final_punc_model, "ct-punc")
        self.assertEqual(config.stream_chunk_size, (0, 8, 4))
        self.assertTrue(config.kws_enabled)
        self.assertEqual(config.kws_keywords, ("你好小智", "小智同学"))
        self.assertEqual(config.stream_endpoint_vad_provider, "fsmn_vad")

    def test_stream_lifecycle_tracks_preview_and_final_without_duplicate_partials(self) -> None:
        engine = FunASREngine(self.make_config(language="zh"))
        preview_audio = bytes(640)
        final_audio = bytes(640)
        with patch.object(
            engine,
            "_run_final_transcription",
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

    def test_stream_lifecycle_uses_online_preview_and_final_asr_in_2pass_mode(self) -> None:
        engine = FunASREngine(
            self.make_config(
                language="zh",
                online_model="paraformer-zh-streaming",
                stream_preview_min_audio_ms=0,
                stream_chunk_size=(0, 1, 0),
            )
        )
        preview_audio = bytes(1920)
        preview_calls: list[bool] = []

        def fake_online_preview(
            audio_bytes: bytes,
            *,
            sample_rate_hz: int,
            channels: int,
            language: str,
            cache: dict[str, object],
            is_final: bool,
        ) -> dict[str, object]:
            self.assertEqual(sample_rate_hz, 16000)
            self.assertEqual(channels, 1)
            self.assertEqual(language, "zh")
            preview_calls.append(is_final)
            cache["seen"] = True
            if is_final:
                self.assertEqual(audio_bytes, b"")
                return {"text": "打开客厅", "elapsed_ms": 9, "language": "zh", "mode": "stream_2pass_online_final"}
            self.assertEqual(audio_bytes, preview_audio)
            return {"text": "打开", "elapsed_ms": 8, "language": "zh", "mode": "stream_2pass_online_final"}

        with patch.object(
            engine,
            "_run_online_preview",
            side_effect=fake_online_preview,
        ) as online_preview, patch.object(
            engine,
            "_run_final_transcription",
            return_value={"text": "打开客厅灯", "elapsed_ms": 16, "language": "zh", "mode": "batch"},
        ) as final_asr:
            started = engine.start_stream(
                session_id="sess-2",
                turn_id="turn-2",
                trace_id="trace-2",
                device_id="device-2",
                codec="pcm16le",
                sample_rate_hz=16000,
                channels=1,
                language="zh",
            )
            stream_id = started["stream_id"]
            self.assertEqual(started["mode"], "stream_2pass_online_final")

            pushed = engine.push_stream_audio(stream_id, preview_audio)
            self.assertEqual(pushed["preview_text"], "打开")
            self.assertTrue(pushed["preview_changed"])
            self.assertEqual(pushed["latest_partial"], "打开")
            self.assertEqual(pushed["mode"], "stream_2pass_online_final")

            finished = engine.finish_stream(stream_id)
            self.assertEqual(finished["text"], "打开客厅灯")
            self.assertEqual(finished["latest_partial"], "打开客厅灯")
            self.assertEqual(finished["partials"], ["打开", "打开客厅", "打开客厅灯"])
            self.assertEqual(finished["mode"], "stream_2pass_online_final")
            self.assertEqual(online_preview.call_count, 2)
            self.assertEqual(preview_calls, [False, True])
            final_asr.assert_called_once()

    def test_transcribe_pcm_applies_kws_prefix_strip_and_audio_event(self) -> None:
        engine = FunASREngine(
            self.make_config(
                language="zh",
                kws_enabled=True,
                kws_keywords=("你好小智",),
            )
        )
        with patch.object(
            engine,
            "_run_kws_detection",
            return_value={"detected": True, "keyword": "你好小智", "score": 0.91},
        ), patch.object(
            engine,
            "_run_final_transcription",
            return_value={
                "text": "你好小智，打开客厅灯",
                "raw_text": "你好小智，打开客厅灯",
                "segments": ["你好小智，打开客厅灯"],
                "duration_ms": 20,
                "elapsed_ms": 5,
                "session_id": "sess-3",
                "model": "iic/SenseVoiceSmall",
                "device": "cpu",
                "language": "zh",
                "mode": "batch",
            },
        ):
            result = engine.transcribe_pcm(
                audio_bytes=bytes(640),
                sample_rate_hz=16000,
                channels=1,
                session_id="sess-3",
                language="zh",
            )
        self.assertEqual(result["text"], "打开客厅灯")
        self.assertEqual(result["segments"], ["打开客厅灯"])
        self.assertEqual(result["audio_events"], ["kws_detected:你好小智"])
        self.assertTrue(result["kws_detected"])
        self.assertEqual(result["kws_keyword"], "你好小智")
        self.assertEqual(result["kws_score"], 0.91)
        self.assertEqual(engine._apply_kws_text_policy("你好小智", {"keyword": "你好小智"}), "")

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

    def test_preview_endpoint_reason_prefers_fsmn_vad_when_configured(self) -> None:
        engine = FunASREngine(
            self.make_config(
                stream_endpoint_vad_provider="auto",
                final_vad_model="fsmn-vad",
                stream_endpoint_vad_min_silence_ms=160,
            )
        )
        with patch.object(engine, "_run_fsmn_vad", return_value=[{"value": [[0, 120]]}]):
            reason = engine._preview_endpoint_reason(
                {
                    "audio_bytes": bytes(6400),
                    "sample_rate_hz": 16000,
                    "channels": 1,
                    "duration_ms": 320,
                },
                {"text": "打开客厅灯"},
            )
        self.assertEqual(reason, "preview_fsmn_vad_silence")

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
            session_id="sess-4",
            turn_id="turn-4",
            trace_id="trace-4",
            device_id="device-4",
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
