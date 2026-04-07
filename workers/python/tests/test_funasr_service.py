from __future__ import annotations

import os
import unittest
from unittest.mock import patch

from agent_server_workers.funasr_service import FunASREngine, WorkerConfig, _env_bool, build_config


class FunASRServiceTests(unittest.TestCase):
    def test_env_bool(self) -> None:
        os.environ["AGENT_SERVER_FUNASR_TEST_BOOL"] = "true"
        try:
            self.assertTrue(_env_bool("AGENT_SERVER_FUNASR_TEST_BOOL", False))
        finally:
            os.environ.pop("AGENT_SERVER_FUNASR_TEST_BOOL", None)

    def test_extract_raw_text(self) -> None:
        engine = FunASREngine(
            WorkerConfig(
                host="127.0.0.1",
                port=8091,
                model="iic/SenseVoiceSmall",
                device="cpu",
                language="auto",
                trust_remote_code=False,
                disable_update=True,
                batch_size_s=60,
                use_itn=True,
            )
        )
        self.assertEqual(engine._extract_raw_text([{"text": "hello"}]), "hello")
        self.assertEqual(engine._extract_raw_text({"text": "world"}), "world")

    def test_build_config_defaults_trust_remote_code_false(self) -> None:
        with patch.dict(os.environ, {}, clear=True):
            with patch("sys.argv", ["funasr_service.py"]):
                config = build_config()
        self.assertFalse(config.trust_remote_code)


if __name__ == "__main__":
    unittest.main()
