# 语音运行时 slot post-processing 边界收敛

- 日期：2026-04-17
- 状态：当前实现边界说明

## 1. 背景

最近的语音语义链路已经从单纯的 `ASR -> LLM reply`，扩展到：

- `SemanticTurnJudge`
- `SemanticSlotParser`
- `EntityCatalogGrounder`
- recent-context ranking
- ASR hotword / hint 回灌
- value normalization
- risk-aware clarification

这些能力本身方向是对的，但如果继续把智能家居的实体、风险词、目标词散落在 runtime 中，就会让项目从“通用 agent server”滑向“某个 demo 的业务后端”。

因此，这一轮收敛的重点不是回退能力，而是明确：

- 哪些属于 shared runtime 的通用机制
- 哪些只是当前研究阶段的 seed data / seed profile

## 2. 边界原则

### 2.1 runtime 可以拥有机制

`internal/voice` 继续拥有以下机制层能力：

- 在 slot parser 之后做 runtime-owned grounding orchestration
- 记录 session recent context，并用于 runtime 内部 tie-break
- 生成 provider-neutral 的 ASR hints
- 做 slot value normalization
- 做 risk gating / confirm-required policy

这些能力本质上都属于“语音 runtime 如何更早、更稳、更智能地理解输入”。

### 2.2 runtime 不应拥有具体业务词表

`internal/voice` 不应继续直接堆积如下业务语义硬编码：

- 某些具体设备词本身天然高风险
- 某些具体中文动词一出现就直接判高风险
- 某个 demo 的 alias 表被当作全局固定知识库

这些内容应该来自：

- built-in seed profile
- entity catalog annotations
- policy data
- 后续可外置的 config / catalog source

## 3. 本轮实现收敛点

## 3.1 built-in catalog 变成 optional profile

当前 built-in entity catalog 不再被视为 runtime 的永久默认知识库，而是明确为：

- `seed_companion`

这个 profile 只承载当前研究阶段的高频 seed data：

- smart-home demo 高频实体
- desktop-assistant demo 高频实体

接入方式也收敛为：

- `AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE=seed_companion`
- 或显式关闭：`AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE=off`

这样做的目的不是减少能力，而是把“机制”和“示例数据”拆开。

## 3.2 risk gating 只吃抽象注解

当前 risk gating 已收敛为：

- `result.RiskLevel`
- `result.RiskReason`
- `result.RiskConfirmRequired`

也就是说：

- runtime 负责“高风险动作不要直接 act，需要 clarify / confirm”
- 但“什么对象/动作是高风险”不再从运行时代码里的词面列表直接推断

这样后续无论是智能家居、桌面助理，还是别的 domain，都可以把风险定义放到 catalog / policy data，而不需要继续污染 runtime。

## 3.3 ASR hints 保持 provider-neutral

recent-context ranking 的结果现在通过通用 contract 回灌给 ASR：

- `TranscriptionRequest.Hotwords`
- `TranscriptionRequest.HintPhrases`

这层边界的关键点是：

- gateway 不做业务 bias
- transcriber 不做领域逻辑
- runtime 只提供 provider-neutral hints

因此，后续无论底层换 FunASR、云 ASR，还是别的兼容 worker，这条链路都能复用。

## 3.4 value normalization 仍是 seed-domain MVP

当前 `slot value normalization` 仍保留少量 seed-domain 规则，例如：

- 温度
- 百分比
- 少量 mode / delta 值

这部分目前仍是研究期 MVP，原因是：

- 它确实能显著提升 realtime slot completeness 的可用性
- 但它还不适合作为长期的“全域业务语义库”

因此这里的结论是：

- 先保留机制验证收益
- 后续继续向 profile data 或 policy annotations 收敛

## 4. 当前代码映射

- 配置入口：
  - `internal/app/config_voice.go`
- 运行时装配：
  - `internal/app/app.go`
- built-in seed catalog / recent-context ranking / ASR hints：
  - `internal/voice/entity_catalog.go`
- value normalization / abstract risk gating：
  - `internal/voice/slot_value_normalizer.go`
- ASR hints 主链接入：
  - `internal/voice/asr_responder.go`

## 5. 当前结论

这一轮收敛后的边界可以概括为一句话：

> `internal/voice` 保留“如何理解语音”的通用机制，但不再把智能家居等 seed app 的具体业务词表当成 runtime 的永久硬编码。

这保证了三件事可以同时成立：

1. 当前语音 demo 继续向高实时性、高智能性推进
2. 项目仍保持为通用 `ai agent server`
3. 后续可把 catalog / policy / domain data 继续外置，而不需要推翻现有 runtime 主链
