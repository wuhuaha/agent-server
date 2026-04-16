"""Local OpenAI-compatible chat-completions worker for GPU-hosted causal LLMs."""

from __future__ import annotations

import argparse
import importlib.util
import inspect
import json
import os
import re
import threading
import time
import uuid
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Iterator

_THINK_BLOCK_RE = re.compile(r"<think>.*?</think>", re.DOTALL | re.IGNORECASE)


@dataclass(slots=True)
class WorkerConfig:
    host: str
    port: int
    model_id: str
    model_dir: str
    device: str
    torch_dtype: str
    trust_remote_code: bool
    preload_model: bool
    max_new_tokens: int
    temperature: float
    top_p: float
    repetition_penalty: float
    seed: int
    force_no_think: bool


class StreamingThinkFilter:
    """Strip `<think>...</think>` blocks from streamed text chunks."""

    def __init__(self) -> None:
        self._buffer = ""
        self._inside_think = False

    def feed(self, chunk: str) -> str:
        if not chunk:
            return ""
        self._buffer += chunk
        output: list[str] = []
        while self._buffer:
            if self._inside_think:
                end = self._buffer.find("</think>")
                if end == -1:
                    if len(self._buffer) > 32:
                        self._buffer = self._buffer[-32:]
                    return ""
                self._buffer = self._buffer[end + len("</think>") :]
                self._inside_think = False
                continue

            start = self._buffer.find("<think>")
            if start == -1:
                flush_upto = len(self._buffer) - trailing_tag_fragment_len(self._buffer)
                if flush_upto == 0:
                    return ""
                output.append(self._buffer[:flush_upto])
                self._buffer = self._buffer[flush_upto:]
                return "".join(output)

            if start > 0:
                output.append(self._buffer[:start])
            self._buffer = self._buffer[start + len("<think>") :]
            self._inside_think = True
        return "".join(output)

    def finish(self) -> str:
        if self._inside_think:
            return ""
        output = self._buffer.replace("<think>", "")
        self._buffer = ""
        return output


class LocalLLMEngine:
    def __init__(self, config: WorkerConfig) -> None:
        self.config = config
        self._lock = threading.Lock()
        self._loaded = False
        self._loading = False
        self._last_error = ""
        self._tokenizer: Any | None = None
        self._model: Any | None = None
        self._torch: Any | None = None
        self._transformers: Any | None = None

    @property
    def model_ref(self) -> str:
        return self.config.model_dir or self.config.model_id

    def health(self) -> dict[str, Any]:
        ready = self._loaded and self._model is not None and self._tokenizer is not None
        status = "ok" if ready else "starting"
        if self._last_error:
            status = "error"
        return {
            "status": status,
            "ready": ready,
            "model_id": self.config.model_id,
            "model_dir": self.config.model_dir,
            "device": self.config.device,
            "torch_dtype": self.config.torch_dtype,
            "loaded": self._loaded,
            "loading": self._loading,
            "last_error": self._last_error,
            "force_no_think": self.config.force_no_think,
            "max_new_tokens": self.config.max_new_tokens,
            "temperature": self.config.temperature,
            "top_p": self.config.top_p,
            "repetition_penalty": self.config.repetition_penalty,
        }

    def ensure_loaded(self) -> None:
        if self._loaded and self._model is not None and self._tokenizer is not None:
            return
        with self._lock:
            if self._loaded and self._model is not None and self._tokenizer is not None:
                return
            self._loading = True
            self._last_error = ""
            try:
                import torch
                from transformers import AutoModelForCausalLM, AutoTokenizer, TextIteratorStreamer

                self._torch = torch
                self._transformers = {
                    "AutoModelForCausalLM": AutoModelForCausalLM,
                    "AutoTokenizer": AutoTokenizer,
                    "TextIteratorStreamer": TextIteratorStreamer,
                }

                model_ref = self.model_ref
                tokenizer = AutoTokenizer.from_pretrained(
                    model_ref,
                    trust_remote_code=self.config.trust_remote_code,
                )
                model = AutoModelForCausalLM.from_pretrained(model_ref, **build_model_load_kwargs(torch, self.config))
                model.to(self.config.device)
                model.eval()
                self._tokenizer = tokenizer
                self._model = model
                self._loaded = True
            except Exception as exc:  # pragma: no cover - exercised only in live runtime
                self._loaded = False
                self._last_error = str(exc)
                raise
            finally:
                self._loading = False

    def preload(self) -> None:
        try:
            self.ensure_loaded()
        except Exception:
            pass

    def complete(self, payload: dict[str, Any]) -> dict[str, Any]:
        text = self.generate_text(payload)
        created = int(time.time())
        completion_id = new_completion_id()
        model_name = requested_model_name(payload, self.config.model_id)
        return {
            "id": completion_id,
            "object": "chat.completion",
            "created": created,
            "model": model_name,
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": text},
                    "finish_reason": "stop",
                }
            ],
        }

    def stream(self, payload: dict[str, Any]) -> Iterator[str]:
        self.ensure_loaded()
        assert self._model is not None
        assert self._tokenizer is not None
        assert self._torch is not None
        assert self._transformers is not None

        created = int(time.time())
        completion_id = new_completion_id()
        model_name = requested_model_name(payload, self.config.model_id)
        prompt = build_prompt_text(self._tokenizer, payload, self.config.force_no_think)
        encoded = self._tokenizer(prompt, return_tensors="pt")
        encoded = {key: value.to(self.config.device) for key, value in encoded.items()}
        streamer = self._transformers["TextIteratorStreamer"](
            self._tokenizer,
            skip_prompt=True,
            skip_special_tokens=True,
        )
        generation_kwargs = build_generation_kwargs(self._torch, payload, self.config)
        generation_kwargs.update(encoded)
        generation_kwargs["streamer"] = streamer

        worker = threading.Thread(target=self._generate_worker, args=(generation_kwargs,), daemon=True)
        worker.start()

        yield sse_chunk(
            {
                "id": completion_id,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model_name,
                "choices": [{"index": 0, "delta": {"role": "assistant"}, "finish_reason": None}],
            }
        )

        filter_state = StreamingThinkFilter()
        for piece in streamer:
            visible = filter_state.feed(piece)
            if not visible:
                continue
            yield sse_chunk(
                {
                    "id": completion_id,
                    "object": "chat.completion.chunk",
                    "created": created,
                    "model": model_name,
                    "choices": [{"index": 0, "delta": {"content": visible}, "finish_reason": None}],
                }
            )

        tail = filter_state.finish()
        if tail:
            yield sse_chunk(
                {
                    "id": completion_id,
                    "object": "chat.completion.chunk",
                    "created": created,
                    "model": model_name,
                    "choices": [{"index": 0, "delta": {"content": tail}, "finish_reason": None}],
                }
            )

        worker.join()
        if self._last_error:
            raise RuntimeError(self._last_error)
        yield sse_chunk(
            {
                "id": completion_id,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model_name,
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
            }
        )
        yield "data: [DONE]\n\n"

    def generate_text(self, payload: dict[str, Any]) -> str:
        self.ensure_loaded()
        assert self._model is not None
        assert self._tokenizer is not None
        assert self._torch is not None

        prompt = build_prompt_text(self._tokenizer, payload, self.config.force_no_think)
        encoded = self._tokenizer(prompt, return_tensors="pt")
        input_ids = encoded["input_ids"].to(self.config.device)
        attention_mask = encoded.get("attention_mask")
        if attention_mask is not None:
            attention_mask = attention_mask.to(self.config.device)
        generation_kwargs = build_generation_kwargs(self._torch, payload, self.config)
        with self._torch.inference_mode():
            generated = self._model.generate(
                input_ids=input_ids,
                attention_mask=attention_mask,
                **generation_kwargs,
            )
        completion = generated[0][input_ids.shape[-1] :]
        text = self._tokenizer.decode(completion, skip_special_tokens=True)
        return strip_thinking_text(text)

    def _generate_worker(self, generation_kwargs: dict[str, Any]) -> None:
        try:
            assert self._torch is not None
            assert self._model is not None
            with self._torch.inference_mode():
                self._model.generate(**generation_kwargs)
        except Exception as exc:  # pragma: no cover - exercised only in live runtime
            self._last_error = str(exc)


def requested_model_name(payload: dict[str, Any], default: str) -> str:
    model = str(payload.get("model") or "").strip()
    return model or default


def new_completion_id() -> str:
    return "chatcmpl-" + uuid.uuid4().hex


def sse_chunk(payload: dict[str, Any]) -> str:
    return "data: " + json.dumps(payload, ensure_ascii=False) + "\n\n"


def strip_thinking_text(text: str) -> str:
    cleaned = _THINK_BLOCK_RE.sub("", text or "")
    cleaned = cleaned.replace("<think>", "").replace("</think>", "")
    return cleaned.strip()


def trailing_tag_fragment_len(text: str) -> int:
    keep = 0
    for tag in ("<think>", "</think>"):
        for index in range(1, len(tag)):
            if text.endswith(tag[:index]):
                keep = max(keep, index)
    return keep


def flatten_content(content: Any) -> str:
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts: list[str] = []
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                parts.append(str(item.get("text") or ""))
            elif isinstance(item, dict) and "text" in item:
                parts.append(str(item.get("text") or ""))
            elif isinstance(item, str):
                parts.append(item)
        return "\n".join(part for part in parts if part)
    if isinstance(content, dict):
        return str(content.get("text") or content.get("content") or "")
    return str(content or "")


def normalize_messages(messages: list[dict[str, Any]] | None, force_no_think: bool) -> list[dict[str, str]]:
    normalized: list[dict[str, str]] = []
    for raw in messages or []:
        role = str(raw.get("role") or "user").strip().lower() or "user"
        content = flatten_content(raw.get("content"))
        if role == "tool":
            content = "工具调用结果：" + content if content else "工具调用结果为空。"
            role = "user"
        if role not in {"system", "user", "assistant"}:
            role = "user"
        normalized.append({"role": role, "content": content})
    if force_no_think:
        normalized = inject_no_think(normalized)
    return normalized


def inject_no_think(messages: list[dict[str, str]]) -> list[dict[str, str]]:
    if not messages:
        return [{"role": "system", "content": "/no_think"}]
    updated = [dict(message) for message in messages]
    for message in updated:
        if message.get("role") in {"system", "user"}:
            content = message.get("content", "")
            if "/no_think" not in content:
                message["content"] = "/no_think\n" + content if content else "/no_think"
            return updated
    updated.insert(0, {"role": "system", "content": "/no_think"})
    return updated


def build_prompt_text(tokenizer: Any, payload: dict[str, Any], force_no_think: bool) -> str:
    messages = normalize_messages(payload.get("messages") or [], force_no_think)
    if not messages:
        system_prompt = flatten_content(payload.get("system") or payload.get("system_prompt"))
        user_text = flatten_content(payload.get("input") or payload.get("prompt"))
        raw_messages = []
        if system_prompt:
            raw_messages.append({"role": "system", "content": system_prompt})
        raw_messages.append({"role": "user", "content": user_text})
        messages = normalize_messages(raw_messages, force_no_think)

    apply_kwargs: dict[str, Any] = {
        "tokenize": False,
        "add_generation_prompt": True,
    }
    try:
        signature = inspect.signature(tokenizer.apply_chat_template)
        if "enable_thinking" in signature.parameters:
            apply_kwargs["enable_thinking"] = False
    except (TypeError, ValueError):
        pass
    return tokenizer.apply_chat_template(messages, **apply_kwargs)


def resolve_torch_dtype(torch_module: Any, dtype_name: str) -> Any:
    normalized = str(dtype_name or "auto").strip().lower()
    if normalized in {"auto", "float16", "fp16", "half"}:
        return torch_module.float16
    if normalized in {"bfloat16", "bf16"}:
        return torch_module.bfloat16
    if normalized in {"float32", "fp32"}:
        return torch_module.float32
    return torch_module.float16


def has_optional_dependency(module_name: str) -> bool:
    return importlib.util.find_spec(module_name) is not None


def build_model_load_kwargs(torch_module: Any, config: WorkerConfig) -> dict[str, Any]:
    kwargs: dict[str, Any] = {
        "trust_remote_code": config.trust_remote_code,
        "torch_dtype": resolve_torch_dtype(torch_module, config.torch_dtype),
    }
    if has_optional_dependency("accelerate"):
        kwargs["low_cpu_mem_usage"] = True
    return kwargs


def build_generation_kwargs(torch_module: Any, payload: dict[str, Any], config: WorkerConfig) -> dict[str, Any]:
    max_new_tokens = int(payload.get("max_tokens") or config.max_new_tokens)
    max_new_tokens = max(16, min(max_new_tokens, config.max_new_tokens))
    temperature = float(payload.get("temperature") if payload.get("temperature") is not None else config.temperature)
    top_p = float(payload.get("top_p") if payload.get("top_p") is not None else config.top_p)
    do_sample = temperature > 0.0
    kwargs: dict[str, Any] = {
        "max_new_tokens": max_new_tokens,
        "do_sample": do_sample,
        "temperature": max(0.0, temperature),
        "top_p": top_p,
        "repetition_penalty": config.repetition_penalty,
        "pad_token_id": None,
    }
    if config.seed >= 0:
        torch_module.manual_seed(config.seed)
    return kwargs


def _env_bool(name: str, default: bool) -> bool:
    raw = os.getenv(name)
    if raw is None:
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def build_config(argv: list[str] | None = None) -> WorkerConfig:
    parser = argparse.ArgumentParser(description="Local OpenAI-compatible LLM worker")
    parser.add_argument("--host", default=os.getenv("AGENT_SERVER_LOCAL_LLM_HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.getenv("AGENT_SERVER_LOCAL_LLM_PORT", "8012")))
    parser.add_argument("--model-id", default=os.getenv("AGENT_SERVER_LOCAL_LLM_MODEL_ID", "Qwen/Qwen3-4B-Instruct-2507"))
    parser.add_argument("--model-dir", default=os.getenv("AGENT_SERVER_LOCAL_LLM_MODEL_DIR", ""))
    parser.add_argument("--device", default=os.getenv("AGENT_SERVER_LOCAL_LLM_DEVICE", "cuda:0"))
    parser.add_argument("--torch-dtype", default=os.getenv("AGENT_SERVER_LOCAL_LLM_TORCH_DTYPE", "float16"))
    parser.add_argument("--trust-remote-code", action="store_true", default=_env_bool("AGENT_SERVER_LOCAL_LLM_TRUST_REMOTE_CODE", False))
    parser.add_argument("--preload-model", action="store_true", default=_env_bool("AGENT_SERVER_LOCAL_LLM_PRELOAD_MODEL", True))
    parser.add_argument("--max-new-tokens", type=int, default=int(os.getenv("AGENT_SERVER_LOCAL_LLM_MAX_NEW_TOKENS", "192")))
    parser.add_argument("--temperature", type=float, default=float(os.getenv("AGENT_SERVER_LOCAL_LLM_TEMPERATURE", "0.2")))
    parser.add_argument("--top-p", type=float, default=float(os.getenv("AGENT_SERVER_LOCAL_LLM_TOP_P", "0.9")))
    parser.add_argument("--repetition-penalty", type=float, default=float(os.getenv("AGENT_SERVER_LOCAL_LLM_REPETITION_PENALTY", "1.05")))
    parser.add_argument("--seed", type=int, default=int(os.getenv("AGENT_SERVER_LOCAL_LLM_SEED", "7")))
    parser.add_argument("--force-no-think", action="store_true", default=_env_bool("AGENT_SERVER_LOCAL_LLM_FORCE_NO_THINK", True))
    args = parser.parse_args(argv)
    model_dir = args.model_dir.strip()
    if model_dir:
        model_dir = str(Path(model_dir).expanduser())
    return WorkerConfig(
        host=args.host,
        port=args.port,
        model_id=args.model_id.strip() or "Qwen/Qwen3-4B-Instruct-2507",
        model_dir=model_dir,
        device=args.device.strip() or "cuda:0",
        torch_dtype=args.torch_dtype.strip() or "float16",
        trust_remote_code=bool(args.trust_remote_code),
        preload_model=bool(args.preload_model),
        max_new_tokens=max(32, args.max_new_tokens),
        temperature=max(0.0, args.temperature),
        top_p=min(max(args.top_p, 0.1), 1.0),
        repetition_penalty=max(1.0, args.repetition_penalty),
        seed=args.seed,
        force_no_think=bool(args.force_no_think),
    )


def build_app(engine: LocalLLMEngine):
    from fastapi import Body, FastAPI, Header, HTTPException
    from fastapi.responses import JSONResponse, StreamingResponse

    app = FastAPI(title="agent-server local llm worker", version="0.1.0")

    @app.get("/healthz")
    def healthz() -> JSONResponse:
        return JSONResponse(engine.health())

    @app.get("/v1/models")
    def list_models() -> dict[str, Any]:
        return {
            "object": "list",
            "data": [
                {
                    "id": engine.config.model_id,
                    "object": "model",
                    "owned_by": "local",
                }
            ],
        }

    @app.post("/chat/completions")
    @app.post("/v1/chat/completions")
    async def chat_completions(payload: dict[str, Any] = Body(...), authorization: str | None = Header(default=None)):
        del authorization  # Local worker does not validate bearer tokens.
        try:
            if payload.get("stream"):
                return StreamingResponse(engine.stream(payload), media_type="text/event-stream")
            return JSONResponse(engine.complete(payload))
        except Exception as exc:
            raise HTTPException(status_code=500, detail=str(exc)) from exc

    return app


def main(argv: list[str] | None = None) -> int:
    config = build_config(argv)
    engine = LocalLLMEngine(config)
    if config.preload_model:
        thread = threading.Thread(target=engine.preload, daemon=True)
        thread.start()
    app = build_app(engine)
    import uvicorn

    uvicorn.run(app, host=config.host, port=config.port, log_level="info")
    return 0


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
