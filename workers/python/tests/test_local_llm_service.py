from __future__ import annotations

import os
import unittest
from unittest.mock import patch

from agent_server_workers.local_llm_service import (
    StreamingThinkFilter,
    WorkerConfig,
    build_config,
    build_model_load_kwargs,
    inject_no_think,
    normalize_messages,
    requested_model_name,
    strip_thinking_text,
)


class LocalLLMServiceTests(unittest.TestCase):
    def test_strip_thinking_text_removes_think_blocks(self) -> None:
        self.assertEqual(strip_thinking_text("<think>hidden</think>你好"), "你好")
        self.assertEqual(strip_thinking_text("你好<think>hidden</think>世界"), "你好世界")

    def test_streaming_think_filter_hides_internal_reasoning(self) -> None:
        filter_state = StreamingThinkFilter()
        self.assertEqual(filter_state.feed("<think>abc"), "")
        self.assertEqual(filter_state.feed("def</think>你好"), "你好")
        self.assertEqual(filter_state.feed("，世界"), "，世界")
        self.assertEqual(filter_state.finish(), "")

    def test_normalize_messages_coerces_tool_messages_to_user_text(self) -> None:
        normalized = normalize_messages(
            [
                {"role": "system", "content": "你是小欧管家"},
                {"role": "tool", "content": '{"status":"ok"}'},
            ],
            force_no_think=False,
        )
        self.assertEqual(normalized[1]["role"], "user")
        self.assertIn("工具调用结果", normalized[1]["content"])

    def test_inject_no_think_prefixes_first_system_or_user_message(self) -> None:
        updated = inject_no_think([{"role": "system", "content": "系统提示"}])
        self.assertTrue(updated[0]["content"].startswith("/no_think"))

    def test_requested_model_name_prefers_payload_model(self) -> None:
        self.assertEqual(requested_model_name({"model": "qwen3:8b"}, "fallback"), "qwen3:8b")
        self.assertEqual(requested_model_name({}, "fallback"), "fallback")

    def test_build_config_reads_local_llm_env(self) -> None:
        env = {
            "AGENT_SERVER_LOCAL_LLM_PORT": "8015",
            "AGENT_SERVER_LOCAL_LLM_MODEL_ID": "Qwen/Qwen3-4B-Instruct-2507",
            "AGENT_SERVER_LOCAL_LLM_MODEL_DIR": "/tmp/qwen3-8b",
            "AGENT_SERVER_LOCAL_LLM_FORCE_NO_THINK": "true",
            "AGENT_SERVER_LOCAL_LLM_PRELOAD_MODEL": "false",
        }
        with patch.dict(os.environ, env, clear=True):
            config = build_config([])
        self.assertEqual(config.port, 8015)
        self.assertEqual(config.model_id, "Qwen/Qwen3-4B-Instruct-2507")
        self.assertEqual(config.model_dir, "/tmp/qwen3-8b")
        self.assertTrue(config.force_no_think)
        self.assertFalse(config.preload_model)

    def test_build_model_load_kwargs_skips_low_cpu_flag_without_accelerate(self) -> None:
        config = WorkerConfig(
            host="127.0.0.1",
            port=8012,
            model_id="Qwen/Qwen3-4B-Instruct-2507",
            model_dir="/tmp/model",
            device="cuda:0",
            torch_dtype="float16",
            trust_remote_code=False,
            preload_model=True,
            max_new_tokens=192,
            temperature=0.2,
            top_p=0.9,
            repetition_penalty=1.05,
            seed=7,
            force_no_think=True,
        )
        torch_stub = type("TorchStub", (), {"float16": "fp16", "bfloat16": "bf16", "float32": "fp32"})()
        with patch("agent_server_workers.local_llm_service.importlib.util.find_spec", return_value=None):
            kwargs = build_model_load_kwargs(torch_stub, config)
        self.assertEqual(kwargs["torch_dtype"], "fp16")
        self.assertNotIn("low_cpu_mem_usage", kwargs)


if __name__ == "__main__":
    unittest.main()
