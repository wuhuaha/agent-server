# ADR 0042：语音运行时采用分层 LLM 与 FunASR 增强策略

- Status: Accepted
- Date: 2026-04-17

## Context

当前项目已经把 turn-taking、preview、barge-in、playback truth 等实时语音行为逐步收敛到 `internal/voice`。同时，仓库已经具备：

- 第一版 `LLM semantic judge`
- FunASR worker 内部的 `online_model` / `final_vad_model` / `final_punc_model` / `kws_enabled`
- 归一化后的 `emotion` / `audio_events` 元数据承载能力

但当前仍存在三个结构性问题：

1. `semantic judge` 仍复用主 Agent LLM 配置，尚未形成独立的小模型实时裁判层。
2. 运行时还没有显式的 `slot completeness` 结构化层，无法稳定支持 `clarify_needed` 与 `actionability`。
3. FunASR 的标点、情绪、音频事件能力尚未系统性进入 runtime-owned orchestration。

## Decision

我们采用以下长期方向作为语音运行时的正式策略：

1. 继续坚持 cascade 主架构，不把当前主线切回单一 end-to-end omni 模型。
2. 保留 `Tier 0` 声学 / VAD / heuristic 作为实时安全底座。
3. 在 `internal/voice` 内保持并逐步强化独立的 `Tier 1` 小模型语义裁判层，用于：
   - utterance completeness
   - interruption intent
   - correction / backchannel / takeover 判断
4. 在 `internal/voice` 或其紧邻的 shared runtime 能力中补充 `Tier 2` 结构化语义解析层，用于：
   - domain
   - intent
   - slot completeness
   - clarify / actionability
5. 主对话 LLM 继续保持为独立的 `Tier 3` 能力层，不与实时裁判职责混合。
6. FunASR 的 `final_punc_model`、`emotion`、`audio_events` 统一视为 runtime-owned metadata：
   - 可参与 planner、clarify、reply style、interruption scoring 与 debug
   - 不直接把 provider-specific 细节泄露给 gateway 或公共协议

## Consequences

### Positive

- 实时控制链与主回复链解耦，后续可独立替换小模型、中模型和主模型。
- `slot completeness` 与 `clarify_needed` 可以以更清晰的结构落地到 early processing 与 accept orchestration。
- FunASR 不再只是文本来源，而成为服务侧语音体验优化的多信号输入源。
- 更符合 OpenAI `semantic_vad`、Google/Amazon contextual endpointing、Apple ChipChat 与多信号语音理解等外部实践。

### Negative

- 运行时内部模型层次会变多，需要更严格的配置管理、评测与 tracing。
- 在真正代码落地前，当前仓库仍处于“第一版 semantic judge 已落地，但多模型职责尚未完全拆开”的过渡状态。
- 若同时部署多个本地模型，GPU 显存调度与并发限流会成为新约束。

## Follow-up

- 更新 `docs/architecture/overview.md`
- 新增系统性研究文档：`docs/architecture/voice-multi-llm-and-funasr-strategy-zh-2026-04-17.md`
- 在 `.codex/project-memory.md` 记录该决策
- 后续实现中优先拆分：
  - 独立 `semantic judge` 模型配置
  - `SemanticSlotParser`
  - FunASR punctuation / emotion / audio-events 的 runtime 消费路径
