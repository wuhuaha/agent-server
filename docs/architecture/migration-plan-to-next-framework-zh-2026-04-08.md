# 从当前实现迁移到新一代项目框架的分阶段实施方案（2026-04-08）

## 文档状态

状态：`Proposed`

本文是 [新一代项目框架设计提案](agent-server-next-framework-zh-2026-04-08.md) 的配套迁移方案。

它回答的问题不是“目标架构应该是什么”，而是：

在不破坏当前 RTOS、Web/H5、桌面调试、`xiaozhi` 兼容链路的前提下，如何把现有仓库稳妥迁移到目标框架。

## 文档目标

本文要提供一份可执行的迁移实施方案，覆盖：

1. 迁移原则
2. 迁移阶段划分
3. 每阶段的目标、改动范围、依赖关系、验收标准
4. 如何控制协议兼容、运行风险与回滚成本
5. 如何把当前 `P0 / P1 / P2` 任务与目标框架迁移对齐

## 适用范围

适用于以下现有代码与能力边界：

- 原生 `/v1/realtime/ws`
- `xiaozhi` 兼容接入
- Web/H5 直连调试路径
- Python desktop runner 与 standalone web client
- `internal/session`
- `internal/voice`
- `internal/agent`
- `internal/gateway`
- `internal/control`

## 迁移基本判断

### 1. 当前仓库不需要“大爆炸式重构”

不建议：

- 一次性重命名整个目录树
- 一次性替换现有 runtime
- 一次性升级 public protocol
- 一次性把所有能力变成远程服务

建议：

- 内部逐层替换
- 对外协议尽可能保持稳定
- 先做内核能力重构，再考虑 public contract 升级

### 2. 迁移必须遵守“外部稳定、内部演进”原则

短中期内应尽量保持以下外部接口稳定：

- `GET /v1/realtime`
- `GET /v1/info`
- `/v1/realtime/ws`
- `/xiaozhi/ota/`
- `/xiaozhi/v1/`
- 当前 `response.chunk` 流式事件语义
- 当前 browser/desktop bring-up 工具的基本连通方式

### 3. 迁移优先解决能力密度，而不是命名整洁

不要把大量时间先消耗在：

- 包名美化
- 大规模移动文件
- 不带行为改进的层次重命名

更优先的是：

- voice orchestration
- capability fabric
- durable memory
- workflow core
- eval plane

## 迁移总策略

采用 `6+1` 阶段迁移法：

1. `Stage 0` 迁移前准备与观测基线
2. `Stage 1` Voice Orchestration Core 落地
3. `Stage 2` Capability Fabric 落地
4. `Stage 3` Context & Memory Fabric 落地
5. `Stage 4` Agent Workflow Core 落地
6. `Stage 5` Policy & Safety Fabric 落地
7. `Stage 6` Eval Plane 与整体收口

其中：

- `Stage 0` 是所有阶段的前置门
- `Stage 1-4` 是核心技术迁移主线
- `Stage 5-6` 是可运营、可量产、可长期演进的关键补强

## 外部契约冻结清单

迁移期间默认冻结以下外部契约，除非进入明确的协议版本升级阶段：

### Frozen A：Realtime Public Contract

- `session.start`
- binary audio uplink
- `audio.in.commit`
- `session.update`
- `response.start`
- `response.chunk`
- `session.end`

### Frozen B：RTOS Compatibility Contract

- `xiaozhi` 兼容路径
- 兼容二进制 framing
- 兼容 `stt` echo、`tts.start/stop` 语义

### Frozen C：Bring-up Tooling Contract

- desktop runner 基本报告结构
- standalone web client 的基本连通模型
- built-in Web/H5 debug 页的路径入口

冻结的意思不是“不能增强”，而是：

- 不轻易修改字段名
- 不轻易修改事件顺序依赖
- 不轻易让现有设备和工具失效

## Stage 0：迁移前准备与观测基线

### 目标

在做任何内核迁移前，先把当前系统变成“可量化、可回归、可比较”的状态。

### 要解决的问题

如果没有统一观测基线，后续迁移会出现三个典型问题：

1. 做了很多重构，但不知道性能是否变差
2. 体验退化无法快速定位是 ASR、LLM、TTS 还是 session 状态机
3. 不同 provider 或不同执行模式无法横向比较

### 工作项

#### 0.1 统一 trace id / session id / turn id 贯通

要求：

- 所有 turn 级日志、指标、runner 报告都要具备统一标识
- trace 至少贯通：
  - gateway
  - session
  - voice
  - runtime
  - tool
  - tts playback

#### 0.2 建立 phase-level latency 基线

最少记录：

- turn accepted
- first partial ASR
- final ASR
- runtime start
- first text delta
- first audio chunk
- response completed
- playout completed

#### 0.3 建立标准化回归场景集

建议最少包含：

- text turn
- short voice turn
- long voice turn
- barge-in turn
- empty/noisy audio turn
- tool loop turn
- tts interrupted turn
- session timeout turn

#### 0.4 建立 compare-ready 报告输出

建议新增统一 JSON 报告字段：

- build/runtime metadata
- llm_provider
- asr_provider
- tts_provider
- turn_mode
- quality metrics
- error summary

### 当前推进记录

截至 2026-04-09，`F0` 已经先后落下两段兼容实现：

1. `turn_id / trace_id` 已从 gateway 贯通到 shared voice 与 runtime turn path，并以 additive 方式进入 native realtime `response.start` 与 turn-state `session.update`
2. server 侧已补齐结构化 turn trace 日志，desktop runner 也已补齐 `generated_at`、`run_id`、`llm_provider`、per-scenario issue/artifact 引用，以及 replay-friendly `artifact_dir`

这两段都保持了 native realtime 与 `xiaozhi` 兼容协议的外部契约稳定，没有引入新的必选公网字段。

### 输出物

- 统一 trace/metrics 约定
- 标准回归场景定义
- 一份“迁移前基线报告”

### 验收标准

- 同一场景能稳定比较迁移前后的时延与行为
- 能快速回答“退化发生在哪个 phase”

## Stage 1：落地 Voice Orchestration Core

### 目标

把当前相对薄的 voice runtime 升级为真正的语音会话编排核心。

### 为什么最先做

因为：

- 语音是主链路
- 它直接决定主观体验
- 后续 workflow、memory、policy 都依赖更稳定的 turn phase 模型

### 迁移策略

不是直接替换 `internal/voice`，而是在现有 `internal/voice` 之上逐步抽出以下内核：

- `VoiceTurnManager`
- `EndpointingPolicy`
- `PlaybackController`
- `SpeechUnderstandingNormalizer`

### 工作项

#### 1.1 引入更细的 turn phase 内部模型

建议先内部引入，不立刻修改公网协议：

- `listening`
- `committed`
- `understanding`
- `planning`
- `tool_running`
- `responding_text`
- `responding_audio`
- `interrupted`
- `completed`

当前对外仍可继续映射为：

- `active`
- `thinking`
- `speaking`

#### 1.2 增加 partial ASR 与 endpoint reason

做法：

- 先让 ASR provider 返回 partial / final / endpoint_reason
- 统一通过 `speech.*` metadata 进入 runtime
- 暂不要求所有 adapter 都立即消费 partial

#### 1.3 引入 `EndpointingPolicy`

先支持三档：

- `client_commit_only`
- `client_commit_plus_server_hint`
- `server_vad_assisted`

注意：

- 第一阶段不要求默认切到 server VAD
- 只要求把 server-side endpointing 变成可插拔能力

#### 1.4 引入 `PlaybackController`

要解决的问题：

- TTS 播放与状态机脱节
- 中断时 playout 停止不够统一
- text streaming 和 audio playout 缺少统一边界

#### 1.5 Barge-in 统一中断链

把以下中断信号统一起来：

- inbound audio
- `session.update interrupt=true`
- client end
- timeout
- provider / stream cancel

### 涉及模块

- `internal/voice/*`
- `internal/session/*`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`

### 风险

- 容易误伤 `xiaozhi` 兼容时序
- 容易让 `response.chunk` 与 binary audio 顺序退化

### 控制策略

- 内部状态先扩展，外部事件先保持兼容
- 所有新 phase 先只进入 trace/report，不立即公开为新协议字段

### 验收标准

- 现有设备与调试工具无需修改即可继续使用
- phase 指标可以区分 `responding_text` 和 `responding_audio`
- barge-in 成功率可量化

## Stage 2：落地 Capability Fabric

### 目标

把当前 builtin skill/tool 能力，升级为真正的平台化能力层。

### 为什么第二阶段做

因为当前用户明确希望：

- 设备控制规则不要写死在 core
- 尽量通过 skill 扩展
- 未来还能更灵活地接第三方能力

这正是 `Capability Fabric` 的职责。

### 工作项

#### 2.1 建立 `Skill Registry`

每个 skill 至少定义：

- id
- version
- domain
- prompt fragments
- tools
- policy sensitivity
- required context

#### 2.2 建立 `Tool Registry`

要求：

- 稳定 tool identity
- provider-safe alias
- schema versioning
- capability discovery

#### 2.3 抽出 `Capability Policy`

用于决定：

- 哪些 skill 默认启用
- 哪些 tool 在当前 session 可见
- 哪些敏感能力只允许 `dry_run` 或 `simulation`

#### 2.4 增加 `MCP Bridge`

第一阶段只需做到：

- 能引入外部 MCP tool
- 能把 MCP tool 包装进现有 tool loop
- 有超时、错误、权限边界

不要求第一阶段就把 resources/prompts 全部做满。

### 建议先迁移的内建 skill

优先级建议：

1. `household_control`
2. `status_query`
3. `memory_recall`
4. `device_diagnostics`

### 涉及模块

- `internal/agent/*`
- 新增 `internal/capability/*`
- `internal/app/*`

### 风险

- skill 注册机制设计过重
- tool 身份和 provider alias 再次耦合

### 控制策略

- 先做本地 registry，再补远程插件
- 先做 skill metadata + tool registry，再做更复杂的资源发现

### 验收标准

- household 逻辑完全不需要再新增 executor 分支
- runtime 能按 skill registry 装配 prompt/tool/policy
- MCP tool 能进入正常 tool loop

## Stage 3：落地 Context & Memory Fabric

### 目标

把当前 in-memory、多 scope 的基础 memory，升级为真正的上下文系统。

### 为什么第三阶段做

因为：

- 先把 voice 与 capability 做稳，才能知道 memory 需要喂给谁、怎么喂
- 家庭中控最重要的“熟悉感”来自 context，不只是 prompt

### 工作项

#### 3.1 引入 `ContextAssembler`

输入：

- session
- user
- device
- room
- household
- screen state

输出：

- workflow 可消费的统一上下文对象

#### 3.2 将 memory 拆为四层

- `RecentStore`
- `ProfileStore`
- `HouseholdStore`
- `DerivedMemoryStore`

#### 3.3 补 durable backend

建议第一阶段优先选择：

- SQLite / Postgres 其一作为 durable metadata store
- 可选 vector store 保留到后续阶段

注意：

- 不要一开始就强依赖重型外部向量数据库
- 先解决 durability，再解决 fancy retrieval

#### 3.4 接入 screen context

这一步很重要。

推荐先支持：

- 当前页面 id
- 当前房间
- 当前设备卡片
- 最近一次触控动作

### 涉及模块

- 新增 `internal/context/*`
- `internal/agent/*`
- Web/H5 debug / future screen client context uplink

### 风险

- 上下文过量注入导致 prompt 膨胀
- session/user/household 冲突处理不清晰

### 控制策略

- 上下文聚合必须有 budget
- memory recall 必须有优先级
- screen context 先做轻量字段，不上传大块 UI 结构

### 验收标准

- 多轮连续性显著改善
- runtime 能根据 room/household/screen context 调整推理
- durable memory 重启后可保留关键偏好与事实

## Stage 4：落地 Agent Workflow Core

### 目标

把当前“单 runtime + tool loop”升级为轻量 workflow runtime。

### 为什么第四阶段做

因为只有在 voice、capability、context 三层足够稳定后，workflow 才不会成为空中楼阁。

### 工作项

#### 4.1 引入 `InteractionPlanner`

负责把 turn 分类为：

- direct answer
- direct control
- status query
- multi-step orchestration
- clarification
- rejection / safe fallback

#### 4.2 引入 `ExecutionGraph`

应支持：

- step
- branch
- retry
- timeout
- interrupt
- resume

第一阶段不追求复杂 DSL，优先追求：

- 内部简单、可测试、可追踪

#### 4.3 引入 `HandoffCoordinator`

先支持 skill 级 handoff，而不是多进程 agent。

例如：

- 从 household_control handoff 到 status_query
- 从 diagnostics handoff 到 memory/reminder

#### 4.4 将 workflow 结果与 session/voice 对接

要求：

- workflow step 结果能产出 text delta
- 能产出 tool result
- 能产出 session directive
- 能产出 audio render directive

### 涉及模块

- `internal/agent/*`
- 新增 `internal/runtime/*`
- `internal/session/*`
- `internal/voice/*`

### 风险

- 过早“框架化”，导致实现过重
- workflow 与 tool loop 重复建模

### 控制策略

- 先把 tool loop 看作 workflow 的一种 step
- 先做单 session 内 handoff，不做分布式 agent 编排

### 验收标准

- 至少支持一个多步 household 场景
- 至少支持一个澄清后继续执行场景
- interruption 后可安全停止或恢复当前 workflow

## Stage 5：落地 Policy & Safety Fabric

### 目标

把当前散落在 prompt、skill 说明、产品约定里的安全策略，收敛成系统层能力。

### 为什么这一步不能省

家庭中控天然涉及：

- 门锁
- 安防
- 燃气
- 自动化
- 远程代理控制

如果不做系统级策略，后续 runtime 越强，风险反而越大。

### 工作项

#### 5.1 引入 `RiskClassifier`

按请求和 skill/tool 组合打标签：

- low risk
- medium risk
- high risk

#### 5.2 引入 `ActionGate`

决策：

- allow
- require confirm
- dry_run only
- reject

#### 5.3 与 execution mode 对接

要求：

- `simulation`
- `dry_run`
- `live_control`

不再只是 prompt 行为差异，也应成为系统级执行约束。

#### 5.4 与 auth / trust level 对接

为未来预留：

- authenticated user
- trusted device
- verified household role

### 涉及模块

- 新增 `internal/policy/*`
- `internal/app/*`
- `internal/runtime/*`
- `internal/capability/*`

### 验收标准

- 高风险技能不能仅靠 prompt 绕过
- 执行模式与安全门控能在系统层生效

## Stage 6：落地 Eval Plane 与整体收口

### 目标

把项目从“可研发”推进到“可持续优化、可比较、可运营”。

### 工作项

#### 6.1 建立 replay 机制

可回放对象：

- 文本 turn
- 音频 turn
- tool loop
- interrupted turn

#### 6.2 建立 provider comparison harness

可对比：

- ASR provider
- TTS provider
- LLM provider
- endpointing policy

#### 6.3 建立 nightly / CI 级回归集

至少覆盖：

- realtime contract regression
- latency regression
- skill regression
- memory regression

#### 6.4 形成 release gate

建议发布门槛：

- 核心场景通过率
- latency 不劣化
- barge-in 成功率达标
- 关键高风险策略无回退

### 验收标准

- 任何一次架构重构都能量化验证收益或退化
- provider 与 skill 的变更不再完全依赖人工体验判断

## 与当前 P0/P1/P2 路线的映射

### P0 对应

主要落在：

- `Stage 0`
- `Stage 1`

对应关系：

- 观测性 -> Stage 0
- turn control / barge-in / phase 语义 -> Stage 1

### P1 对应

主要落在：

- `Stage 2`
- `Stage 3`
- `Stage 4`

对应关系：

- skill/tool -> Stage 2
- memory/context -> Stage 3
- workflow/runtime intelligence -> Stage 4

### P2 对应

主要落在：

- `Stage 5`
- `Stage 6`

以及后续基于这些基础能力的体验升级。

## 推荐的里程碑命名

为了避免“P0/P1/P2 是优化优先级，但不是架构迁移阶段”的混淆，建议迁移阶段独立命名：

- `F0` Baseline And Traceability
- `F1` Voice Orchestration
- `F2` Capability Fabric
- `F3` Context And Memory
- `F4` Workflow Core
- `F5` Policy And Safety
- `F6` Eval Plane And Release Gates

## 每阶段的退出门

### F0 Exit Gate

- 有稳定基线报告
- 核心场景可量化比较

### F1 Exit Gate

- turn phase 更细但外部兼容
- interruption/tts/playout 可稳定追踪

### F2 Exit Gate

- skill/tool 不再需要继续往 executor 加分支
- 至少一个 MCP tool 正常工作

### F3 Exit Gate

- durable memory 落地
- screen context 进入 runtime

### F4 Exit Gate

- 至少一个多步 workflow 落地
- interruption/resume 可用

### F5 Exit Gate

- 高风险动作具备系统级 gate

### F6 Exit Gate

- release gate 生效
- 回归评估可自动跑

## 风险清单与对策

### 风险 1：架构文档越来越漂亮，但代码层不收敛

对策：

- 每个阶段结束必须映射到具体目录与 contract
- 每个阶段至少要有一个真实行为提升，而不只是重命名

### 风险 2：迁移过程中打断 RTOS 接入稳定性

对策：

- 所有 public protocol 变更默认冻结
- 内部 phase 扩展先不外泄

### 风险 3：Workflow 过重，拖慢进展

对策：

- 先做轻量 graph
- 先做 skill handoff，不做复杂多 agent 分布式编排

### 风险 4：Memory 系统过重

对策：

- 先 durable，再 fancy
- 先结构化 scope 与 retention，再向量检索

### 风险 5：MCP 引入后能力边界失控

对策：

- 先走 capability policy
- MCP 先接 tool，再逐步放开 resources/prompts

## 推荐的近期执行顺序

如果从现在开始推进，建议顺序如下：

1. `F0` trace/metrics/baseline
2. `F1` voice orchestration
3. `F2` capability fabric
4. `F3` context & memory
5. `F4` workflow core
6. `F5` policy & safety
7. `F6` eval plane

## 文档关系

建议与以下文档配套阅读：

- [Architecture Overview](overview.md)
- [项目优化路线图（2026-04-04）](project-optimization-roadmap-zh-2026-04.md)
- [现代 AI Agent / 语音 Agent 框架复核与架构优化建议（2026-04-08）](modern-ai-agent-framework-review-zh-2026-04-08.md)
- [新一代项目框架设计提案（2026-04-08）](agent-server-next-framework-zh-2026-04-08.md)

关系如下：

- `modern-ai-agent-framework-review-zh-2026-04-08.md`
  - 解释为什么需要优化、应该朝什么方向优化
- `agent-server-next-framework-zh-2026-04-08.md`
  - 给出目标框架长什么样
- 本文
  - 解释如何从当前实现迁移到该目标框架
