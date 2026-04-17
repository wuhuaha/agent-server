# ADR 0045：开源端点能力继续采用 runtime-owned layered fusion

- Status: Accepted
- Date: 2026-04-17

## Context

当前仓库已经形成一条较清晰的服务侧语音主线：

- `preview ASR`
- `server endpoint`
- `stable_prefix`
- `SemanticTurnJudge`
- `SemanticSlotParser`
- speaking-time interruption / playback truth / resume

与此同时，本轮研究确认了当前开源生态的两个现实：

1. 市面上已经开始出现少量联合 `streaming ASR + EOU` 的公开 checkpoint，例如 Hugging Face 上的 `nvidia/parakeet_realtime_eou_120m-v1`。
2. 但当前中文 / FunASR / ModelScope 主生态仍更接近模块化路径：`streaming ASR + VAD + punctuation + runtime endpoint fusion`，而不是成熟、中文优先、可直接替换 turn orchestration 的单模型 semantic endpointing 方案。

本仓库又必须同时满足两个约束：

- 继续保持 `Realtime Session Core` 和 `internal/voice` 的清晰边界
- 不把某个 provider 的特定 endpoint token / API 直接变成仓库的核心架构假设

## Decision

我们决定：

1. 当前仓库的主线继续采用 runtime-owned layered endpoint fusion。
2. `internal/voice` 继续拥有最终 turn-taking 编排权，至少包括：
   - `endpoint candidate`
   - `draft/prewarm`
   - `turn accept`
   - speaking-time interruption policy
   - `slot completeness` 约束
3. 低层声学/VAD/停顿信号继续作为 realtime safety floor。
4. 流式 ASR 稳定性、标点/clause、规则、小模型/小 LLM 语义判断继续作为上层融合证据。
5. 未来若接入一体化 `ASR + EOU` 模型，它们只作为 `StreamingTranscriber` 归一化后的可选 provider hint 进入 `internal/voice`，例如：
   - `provider_eou_seen`
   - `provider_eou_score`
6. 任何 provider-specific 的 endpoint token、模型名字、或推理细节，都不应直接升级为 gateway 协议语义或替代 shared runtime 的 `turn accept` 逻辑。

## Consequences

### Positive

- 当前中文/FunASR 主线不需要为追单模型而中断。
- 新出现的联合 `ASR + EOU` checkpoint 仍可逐步接入做 A/B。
- `EOU != turn accept` 这一关键边界被保留下来，避免命令类场景中过早提交。
- 未来接入中文、多语或更强的一体化模型时，不需要重写 gateway 或 session 协议。

### Negative

- `internal/voice` 内部仍需维护一套多信号融合逻辑，调参工作不会消失。
- 如果未来 provider hint 质量参差不齐，需要额外 tracing 与归一化策略来抑制抖动。
- 相较“单模型直接给答案”的叙事，这条路线在产品宣讲上不够简洁，但工程上更稳。

## Follow-up

- 新增研究文档：`docs/architecture/streaming-asr-and-semantic-endpointing-research-zh-2026-04-17.md`
- 更新 `docs/architecture/overview.md`
- 在 `.codex/project-memory.md` 记录 durable decision
