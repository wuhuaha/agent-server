# `agent-server` 新一代项目框架设计提案（2026-04-08）

## 文档状态

状态：`Proposed`

这是一份面向下一阶段演进的目标框架设计提案，不等同于当前已实现架构。

它的作用不是推翻现有系统，而是回答一个更具体的问题：

在保留当前 `Realtime Session Core` 中心化路线的前提下，如何把 `agent-server` 升级为一个更现代、更 AI-native、更适合语音中控与多端接入的通用 Agent 服务框架。

## 文档目标

本文要解决的不是“接哪个模型”这种短期问题，而是以下更本质的问题：

1. 当前项目下一代的核心架构应该长成什么样
2. 现有 `session / runtime / voice / adapter` 分层如何升级，而不是被替换
3. 什么能力应该进入共享 runtime 主干，什么能力仍应停留在 adapter 或 worker 层
4. 如何让项目既适合 RTOS 设备，又适合 Web/H5、桌面端、消息渠道、未来多模态端
5. 如何让“家庭中控语音助手”从单轮工具型交互，演进到更接近长期伙伴型交互

## 设计立场

### 1. 不推翻现有中心化架构

当前项目最值得保留的部分，是以 `Realtime Session Core` 为中心的整体分层。

因此新框架不是“换底座”，而是“重构能力布局”：

- 保留 `session core`
- 强化 `voice orchestration`
- 强化 `agent workflow runtime`
- 标准化 `skill / tool / memory / policy`
- 让 adapter 真正只做 ingress / egress

### 2. 继续坚持 modular monolith

下一阶段仍然不建议拆成微服务优先架构。

原因：

- 当前项目最复杂的部分是实时状态协作，而不是服务间隔离
- session、voice、workflow、tool、memory 之间的调度耦合仍然很高
- 过早拆服务会放大时延、错误恢复、分布式 tracing、部署复杂度

因此推荐继续保持：

- 一个主 Go 服务进程
- 若干可选 Python 算法 worker
- 若干可选外部存储与能力服务

### 3. 语音能力必须是内建能力，不是外挂模块

对本项目而言，voice 不是边缘能力，而是主链路。

因此：

- 语音 turn control
- interruption
- endpointing
- ASR/TTS streaming
- playout control

都应成为共享 runtime 的主能力，而不是仅由某个 RTOS adapter 或 Web page 临时处理。

### 4. 家居领域能力必须 skill 化，而不是 core 硬编码

项目要更 AI-native，就不能把产品规则不断塞回 executor、gateway、browser。

正确方向是：

- core 负责会话、工作流、工具循环、记忆、策略边界
- domain behavior 通过 skill 进入
- skill 贡献 prompt、tool、policy、memory hint、UI hint

## 设计依据：主流现代框架的有效模式

本提案吸收但不照搬以下方向：

### LiveKit Agents

借鉴点：

- session-first
- turn orchestration
- 实时语音链路中的 interruption / playout / event 流

不直接照搬的原因：

- 本项目已有自己的 realtime contract 与 RTOS fast path
- 当前阶段不宜让 WebRTC 优先替代现有 websocket 核心

### Pipecat

借鉴点：

- processor / pipeline 式语音处理思路
- observer / metrics / transport 组合能力
- 对 interruption、实时多阶段处理的重视

不直接照搬的原因：

- 本项目需要更强的 session-centric 统一模型
- 当前仓库更适合把其理念内化为 `voice orchestration core`

### OpenAI Agents SDK / Realtime / Voice Agents

借鉴点：

- session 与 tracing
- handoff / tool / approval
- 语音 Agent 中 prompt、tool、human-in-the-loop 的组合方式

不直接照搬的原因：

- 本项目必须保持 provider-neutral
- 不能让某一云厂商的 runtime shape 反向定义内部边界

### LangGraph

借鉴点：

- state graph
- durable execution
- interrupt / resume / checkpoint

不直接照搬的原因：

- 当前项目需要的是轻量 workflow runtime，而不是把整个核心迁移到外部 graph 框架

### MCP

借鉴点：

- tool / resource / prompt 的标准化
- 外部能力接入不再依赖私有协议

对本项目意义非常大：

- skill/tool 层下一步应直接为 MCP bridge 预留标准接口

### Home Assistant Voice

借鉴点：

- 家庭控制不等于“把一切都扔给 LLM”
- 家居场景需要 deterministic pipeline、policy、安全边界

对本项目意义：

- 家庭中控场景必须保持“生成式理解 + 确定性执行边界”混合策略

## 新框架的目标形态

目标框架名称建议：

`Session-Centric Voice Agent Framework`

中文可称：

`会话中心型语音 Agent 框架`

核心思想：

- 以 `Session` 作为系统根对象
- 以 `Voice Orchestration` 作为实时交互骨架
- 以 `Agent Workflow Runtime` 作为智能执行骨架
- 以 `Capability Fabric` 与 `Context Fabric` 作为智能扩展层
- 以 adapter 统一承接 RTOS、Web/H5、桌面和外部渠道

## 顶层架构图

```text
Clients / Channels
  RTOS(xiaozhi/native)  Web/H5  Desktop  Feishu/Telegram/...  Future SIP/WebRTC
            |              |        |            |                   |
            +--------------+--------+------------+-------------------+
                                   |
                           Adapter / Gateway Layer
                                   |
                    +--------------+--------------+
                    | Realtime Session Core       |
                    | session state / turn state  |
                    | interrupt / stream contract |
                    +--------------+--------------+
                                   |
              +--------------------+--------------------+
              |                                         |
      Voice Orchestration Core                 Agent Workflow Core
      turn detect / endpointing                planner / tool loop
      asr / tts / playout                      handoff / resume
      listen-think-speak state                 policy-aware execution
              |                                         |
              +--------------------+--------------------+
                                   |
                        Capability & Context Fabric
                  skills / tools / MCP / memory / policy / screen context
                                   |
                      Providers / Workers / External Systems
              ASR / TTS / LLM / DB / vector store / smart-home bus / MCP servers
                                   |
                         Control Plane & Eval Plane
                config / auth / diagnostics / tracing / metrics / replay / evals
```

## 新框架的八个核心层

### 1. Adapter / Gateway Layer

职责：

- 终端接入
- 协议转换
- 音频帧上传与音频帧下发
- session envelope 适配
- 与共享 session core 对接

不应承担的职责：

- 直接调 LLM
- 直接调 ASR/TTS provider
- own 产品规则
- 自己维护独立的多轮对话状态

建议子层：

- `adapter/device/native_realtime_ws`
- `adapter/device/xiaozhi_compat_ws`
- `adapter/channel/web_h5`
- `adapter/channel/desktop_debug`
- `adapter/channel/messaging/*`

### 2. Realtime Session Core

这是系统的中轴。

职责：

- session 生命周期
- turn 状态机
- interruption 接受与传播
- 流式 response 生命周期
- session-close policy 边界
- shared event semantics

建议把当前 session 核心升级为更清晰的两层模型：

#### Session State

- `idle`
- `active`
- `closing`
- `closed`

#### Turn Phase

- `listening`
- `committed`
- `understanding`
- `planning`
- `tool_running`
- `responding_text`
- `responding_audio`
- `interrupted`
- `completed`

原因：

- 当前 `active / thinking / speaking` 过于粗
- 未来要做 tracing、workflow、resume、partial-ASR、playout control，需要更细 phase

### 3. Voice Orchestration Core

这是新框架最重要的新增主层。

它不是简单的 ASR/TTS provider 包装层，而是“语音会话调度核心”。

职责：

- turn detection
- endpointing policy
- VAD / semantic endpointing 协同
- partial ASR 汇聚
- interruption signal 统一处理
- TTS playout pacing
- cancel propagation
- 语音理解元数据标准化

推荐内部组件：

#### `VoiceTurnManager`

负责：

- 开始收听
- 提交 turn
- 处理 partial/final ASR
- 接收 barge-in
- 切换到回复阶段

#### `EndpointingPolicy`

负责：

- client commit
- optional server VAD
- silence timeout
- semantic continuation

应支持多策略并行配置：

- `client_commit_only`
- `client_commit_plus_server_hint`
- `server_vad_assisted`

#### `PlaybackController`

负责：

- TTS 音频分块下发
- 当前播放位置
- 中断时停止当前 playout
- 确保 `responding_text` 与 `responding_audio` 的状态一致

#### `SpeechUnderstandingNormalizer`

负责统一：

- transcript
- language
- emotion / speaking_style
- speaker
- endpoint_reason
- audio_events
- partial hypotheses

### 4. Agent Workflow Core

这是对当前 `Agent Runtime Core` 的升级版命名与职责细化。

当前 runtime 主要拥有：

- prompt assembly
- model call
- tool loop
- memory hook

目标框架建议升级为真正的 workflow runtime。

职责：

- workflow step orchestration
- model-tool loop
- skill handoff
- pause / resume
- clarification policy
- session-close recommendation
- structured action/result emission

推荐内部组件：

#### `InteractionPlanner`

负责判断当前 turn 属于：

- direct answer
- home control
- status query
- multi-step task
- follow-up clarification
- escalation / rejection

#### `ExecutionGraph`

不是完整外部 graph 引擎，而是本项目内部轻量状态图。

应支持：

- step
- branch
- retry
- timeout
- interrupt
- resume

#### `ToolLoopEngine`

负责：

- tool schema 注入
- tool-call proposal 接收
- tool invocation
- tool result reinjection
- tool budget / loop budget

#### `HandoffCoordinator`

负责在 skill 或子 agent 间转交：

- household control skill
- status/knowledge skill
- reminder/memory skill
- diagnostics skill

### 5. Capability Fabric

这是 skill / tool / MCP 的统一能力层。

它解决一个关键问题：

系统如何在不污染 core 的情况下，安全、可扩展地增长“会做的事”。

应包括：

#### `Skill Registry`

每个 skill 至少应描述：

- skill id
- version
- domain
- prompt fragments
- exposed tools
- required context
- policy sensitivity
- output hints

#### `Tool Registry`

负责：

- 统一 tool identity
- provider-safe alias
- schema versioning
- capability discovery

#### `MCP Bridge`

负责：

- 外部 MCP server 接入
- 将 MCP tools/resources/prompts 适配进 runtime
- 隔离远程能力调用和超时/权限问题

#### `Capability Policy`

负责：

- 哪些 skill 可用
- 哪些 tool 允许本 session 使用
- 哪些敏感 skill 需要额外确认或审批

### 6. Context & Memory Fabric

这是对当前 memory 设计的系统化升级。

当前内存后端已经有基础 scope，但还缺“真正的上下文系统”。

目标拆分为四类上下文：

#### A. Conversation Context

内容：

- 当前 turn
- 最近 4 到 8 轮消息
- 当前未完成的 follow-up

#### B. Household Context

内容：

- 房间
- 设备
- 场景
- 当前中控屏页面
- 当前设备卡片
- 当前展示状态

#### C. Identity & Preference Memory

内容：

- 用户称呼
- 偏好温度
- 常用场景
- 提醒偏好
- 语言习惯

#### D. Derived Memory

内容：

- 后台总结出的稳定偏好
- 最近异常模式
- 需要后续提醒的事项

推荐内部组件：

#### `ContextAssembler`

负责：

- 根据 session / user / room / household / screen state 聚合上下文
- 为 workflow、tool、prompt 提供统一上下文对象

#### `MemoryStore`

建议分层：

- `RecentStore`
- `ProfileStore`
- `HouseholdStore`
- `DerivedMemoryStore`

#### `MemoryPolicy`

负责：

- retention
- TTL
- redact
- correction
- delete / revoke

### 7. Policy & Safety Fabric

这是当前项目明显需要补强的一层。

它不应只体现在 prompt 中。

职责：

- 敏感域策略
- clarification policy
- execution gating
- auth-aware capability gating
- safe-response policy

重点场景：

- 门锁
- 安防
- 燃气
- 自动化场景变更
- 远程代控

推荐内部组件：

#### `RiskClassifier`

判断：

- low risk
- medium risk
- high risk

#### `ActionGate`

根据：

- execution mode
- session auth level
- device trust
- skill sensitivity

决定：

- allow
- require confirm
- dry run only
- reject

### 8. Control Plane & Eval Plane

当前 control plane 已有基础，但还不够形成“运行治理面”。

建议明确把运行治理拆成两部分：

#### `Control Plane`

负责：

- config
- healthz
- info/discovery
- device registry
- auth/policy
- debug surfaces

#### `Eval Plane`

负责：

- trace
- metrics
- quality reports
- replay
- regression fixtures
- provider comparison

这层非常关键，因为没有它，后续优化将缺少量化依据。

## 新框架中的关键共享对象

### 1. `SessionContext`

建议字段：

- `session_id`
- `device_id`
- `client_type`
- `transport_kind`
- `auth_context`
- `execution_mode`
- `voice_profile`
- `screen_context`
- `household_context_ref`

### 2. `TurnContext`

建议字段：

- `turn_id`
- `turn_phase`
- `input_modality`
- `speech_metadata`
- `recent_messages`
- `clarification_state`
- `interruption_source`

### 3. `CapabilityDescriptor`

建议字段：

- `kind`: `skill | tool | mcp_tool | resource`
- `id`
- `version`
- `schema`
- `risk_level`
- `required_context`

### 4. `WorkflowStepResult`

建议字段：

- `text_deltas`
- `tool_calls`
- `tool_results`
- `policy_events`
- `memory_writes`
- `session_directives`
- `audio_render_directives`

### 5. `TraceEnvelope`

建议字段：

- `trace_id`
- `session_id`
- `turn_id`
- `phase`
- `provider`
- `latency_ms`
- `status`
- `attributes`

## 多接入方式在新框架中的位置

新框架应明确支持多接入，但保持同一 runtime。

### A. RTOS Native

定位：

- 第一优先级低延迟接入
- binary audio + json control

### B. `xiaozhi` 兼容接入

定位：

- 兼容遗留设备
- 只做协议与行为适配

### C. Web/H5 Direct

定位：

- 调试与未来屏端原生接入
- 优先复用现有 realtime contract

### D. Desktop Debug / Local Labs

定位：

- 研发与评估工具

### E. Messaging / External Channels

定位：

- 文字/图片渠道
- channel skill 仍只做 adapter

### F. Future SIP / WebRTC

定位：

- 更实时的浏览器/电话类接入
- 进入时也应复用同一 session core 和 voice orchestration

## 推荐的包结构演进

以下是建议目标目录，不要求一次性重命名到位，但值得作为未来迁移方向。

```text
internal/
  sessioncore/
    session.go
    turn_state.go
    response_stream.go
    interruption.go

  voicecore/
    turn_manager.go
    endpointing.go
    playback_controller.go
    speech_metadata.go
    asr/
    tts/

  runtime/
    planner.go
    workflow.go
    handoff.go
    executor.go
    prompt/
    providers/

  capability/
    skills/
    tools/
    mcp/
    registry/
    policy/

  context/
    assembler.go
    screen_context.go
    household_context.go
    memory/

  policy/
    risk_classifier.go
    action_gate.go
    execution_mode.go

  observability/
    tracing/
    metrics/
    replay/
    evals/

  adapter/
    device/
      nativews/
      xiaozhi/
    channel/
      webh5/
      desktop/
      messaging/

  controlplane/
    info/
    health/
    config/
    debug/
```

## 为什么这套新框架更适合本项目

### 1. 更适合语音主链路

因为它显式承认：

- 语音 turn orchestration 是核心能力
- voice 不只是 provider wrapper

### 2. 更适合“AI-native + 家庭中控”混合场景

因为它允许：

- 对话理解走生成式
- 领域能力通过 skill 扩展
- 执行边界和安全边界仍可控

### 3. 更适合多接入统一

因为：

- RTOS、Web/H5、桌面、渠道不再各有一套逻辑
- 只是在 adapter 层适配不同 transport 和 UX

### 4. 更适合长期演进

因为：

- durable memory
- workflow/handoff
- MCP
- tracing/evals

都能自然长进现有系统，而不是后补式硬插。

## 与当前实现的映射关系

### 当前已经存在，可直接继承

- `Realtime Session Core`
- `TurnExecutor` 与流式 delta 边界
- `internal/voice` provider 接口
- runtime skill 基础路径
- 多 scope memory 基础
- Web/H5 与 RTOS 共用 realtime contract

### 当前存在，但需要重构升级

- `internal/voice` 需要升级为 `Voice Orchestration Core`
- `internal/agent` 需要升级为 `Agent Workflow Core`
- builtin skill/tool backend 需要升级为 `Capability Fabric`
- in-memory memory 需要升级为 `Context & Memory Fabric`
- debug runner/report 需要升级为 `Eval Plane`

### 当前尚未真正存在

- workflow / handoff runtime
- MCP bridge
- screen context runtime contract
- durable household memory
- policy fabric
- replay/eval/trace plane

## 实施顺序建议

### F0：保持兼容的前提下对齐内部命名与边界

做法：

- 不改现有 wire contract
- 先在内部引入更清晰的 runtime 子层命名
- 把 `voicecore / runtime / capability / context / policy / observability` 作为内部演进目标

### F1：落 `Voice Orchestration Core`

优先项：

- partial ASR
- endpointing policy
- playback controller
- interrupt/cancel chain

### F2：落 `Capability Fabric`

优先项：

- skill registry
- tool registry
- MCP bridge
- capability policy

### F3：落 `Context & Memory Fabric`

优先项：

- context assembler
- durable memory store
- household context
- screen context

### F4：落 `Agent Workflow Core`

优先项：

- interaction planner
- lightweight workflow graph
- handoff coordinator

### F5：落 `Eval Plane`

优先项：

- trace id
- phase metrics
- replayable turn fixtures
- provider A/B comparison

## 不建议的替代方案

### 1. 让 Web/H5 单独拥有一套浏览器 runtime

不建议，因为这会制造第二条 orchestration 主链。

### 2. 让 `xiaozhi` 兼容层继续吸收更多业务逻辑

不建议，因为兼容层应继续保持薄适配。

### 3. 把所有家居语义继续塞进 prompt

不建议，因为这会让 prompt 成为隐式业务代码。

### 4. 为了“多 agent”而过早做复杂分布式架构

不建议，因为当前真正短板仍是共享 runtime 能力密度，而不是服务拆分度。

## 配套文档

当前已配套：

1. [从当前实现迁移到新一代项目框架的分阶段实施方案（2026-04-08）](migration-plan-to-next-framework-zh-2026-04-08.md)
   - 明确从当前实现迁移到该目标框架的阶段性计划

后续仍建议补：

1. `target-runtime-contracts-zh-*.md`
   - 明确 session、turn、workflow、skill、memory 的内部 contract

## 参考资料

以下为设计时重点参考的官方资料入口：

- LiveKit Agents: <https://docs.livekit.io/agents/>
- Pipecat Docs: <https://docs.pipecat.ai/>
- OpenAI Voice Agents Guide: <https://platform.openai.com/docs/guides/voice-agents>
- OpenAI Agents SDK: <https://openai.github.io/openai-agents-js/>
- LangGraph Docs: <https://docs.langchain.com/oss/javascript/langgraph/overview>
- Model Context Protocol: <https://modelcontextprotocol.io/docs/learn/architecture>
- Home Assistant Voice Pipelines: <https://developers.home-assistant.io/docs/voice/pipelines>
- Home Assistant LLM API: <https://developers.home-assistant.io/docs/core/llm/>
- Google ADK Docs: <https://google.github.io/adk-docs/>
- AutoGen Docs: <https://microsoft.github.io/autogen/>

本提案吸收这些框架的有效模式，但不主张直接替换当前项目的核心运行时。
