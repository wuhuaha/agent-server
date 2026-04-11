# 现代 AI Agent / 语音 Agent 框架复核与 `agent-server` 架构优化建议（2026-04-08）

## 文档目的

基于 2026-04-08 对当前主流 AI Agent 与语音 Agent 官方资料的复核，结合本项目现状，回答三个问题：

1. 当前 `agent-server` 的总体架构方向是否还正确
2. 相比最新现代化框架，当前项目的主要差距在哪里
3. 后续应该优先优化哪些方向，而不是盲目替换框架或继续堆接入

本文是对现有中文研究与路线图的补充，重点从“现代框架共性能力”与“本项目现实落点”两个角度做更细的对照。

## 核心结论

结论先行：

- 当前项目不需要推倒重来。
- 当前项目也不应直接替换成某个外部 Agent 框架作为核心运行时。
- 当前最值得优化的，不是“再多接几个模型”，而是把 `voice runtime`、`skill/tool`、`memory`、`observability`、`workflow` 这几层能力补齐。

更具体地说：

1. 当前的中心化分层是正确的
   - `Realtime Session Core` 作为中心
   - `Agent Runtime Core` 负责模型、工具、记忆、策略
   - `Voice Runtime` 独立负责 ASR/TTS/音频输出
   - 设备层和渠道层保持 adapter 角色

2. 当前的主要短板不在边界，而在能力密度
   - 会话边界已经对，但语音会话控制还不够“现代”
   - 技能入口已经有，但还不是完整的 skill registry / MCP 能力层
   - 记忆已有 scope 意识，但还缺 durable、检索、回放和治理
   - 已有流式输出，但缺 trace、eval、state graph、handoff

3. 项目下一阶段最合理的方向
   - 保留现有会话中心架构
   - 在该架构上引入现代 voice-agent runtime 能力
   - 逐步标准化 skill/tool/memory/workflow，而不是重写整个系统

## 研究样本与时间点

本次复核主要参考以下公开官方资料，访问时间统一为 `2026-04-08`：

- LiveKit Agents
- Pipecat
- OpenAI Voice Agents / Realtime / Agents SDK
- LangGraph
- Model Context Protocol（MCP）
- Home Assistant Voice Pipelines / LLM API
- Google ADK
- AutoGen

这些项目覆盖了几个关键方向：

- 实时语音会话编排
- Agent runtime 与 tool loop
- 持久状态与 workflow
- 标准化工具接入
- 家庭语音与常驻式助手落地

## 现代框架的共同演进方向

虽然这些框架实现方式不同，但最近两年的演进方向高度一致。

### 1. Session-first，而不是 request-first

现代语音 Agent 越来越强调“会话”而不是“单次请求”。

典型特征：

- 一个持续的会话对象，而不是每轮重新拼装上下文
- turn、interrupt、playout、cancel、handoff 都围绕 session 展开
- transport 和 provider 被隔离在 session/runtime 之外

对本项目的启示：

- 当前把 `Realtime Session Core` 放在中心是正确路线
- 不应该退回到“每个 adapter 自己拼对话状态”的结构
- 未来无论增加 WebRTC、SIP、桌面端还是消息渠道，都应该继续围绕共享 session core 做

### 2. Duplex voice orchestration 成为主能力，不再只是附属逻辑

主流 voice-agent 框架都不满足于“收音频 -> 转文字 -> 回一段 TTS”。

它们更关注：

- 何时开始监听
- 何时认为用户说完
- 系统说话时是否允许插话
- 抢话后如何停止当前输出
- 新一轮听说切换如何平滑完成

这意味着现代语音系统真正需要的是：

- turn detector
- endpointing policy
- interruption policy
- playout controller
- cancel propagation

对本项目的启示：

- 当前 `audio.in.commit` 驱动的模式适合第一阶段 bring-up，但还偏“对讲机模型”
- 现有 barge-in 逻辑已经具备雏形，但还不是完整的 duplex orchestration
- 语音层后续要从“能打断”升级到“能协调听、想、说”

### 3. Tool / Skill 正在标准化，而不是散落在业务里

主流 Agent 框架都在把工具和外部能力做成统一能力层：

- 模型看到的是稳定工具接口
- transport 不直接调用 provider 或业务系统
- tool schema、tool execution、tool result 回灌都在 runtime 内完成
- 越来越多系统开始接入 MCP，而不是继续堆私有工具协议

对本项目的启示：

- 你希望“设备控制规则通过 skill 加，而不是写死在代码里”，这和行业方向是一致的
- 当前 `runtime skills + tool loop` 已经是正确起点
- 下一步不是再加更多 builtin 判断，而是补齐：
  - skill registry
  - skill 生命周期
  - skill capability metadata
  - MCP bridge

### 4. Memory 正在从“聊天历史”演进为“持久上下文系统”

现代框架越来越少把 memory 等同于“全量历史消息回放”。

更常见的分层是：

- 当前 turn 和最近消息窗口
- 近期情境摘要
- 长期偏好或用户事实
- 后台整理的 stable memory
- 与身份、设备、空间、组织结构绑定的上下文

对家庭场景尤其重要：

- 同一设备可能对应不同家庭成员
- 同一房间内的设备和场景是天然上下文
- 常用设置、称呼、提醒偏好都需要长期记忆

对本项目的启示：

- 当前按 `session / user / device / room / household` 分 scope 的方向是对的
- 但目前仍偏进程内短期记忆，还不够“可持续”
- 接下来应优先做 durable memory backend，而不是继续扩大 prompt 字符串

### 5. Observability / tracing / evals 正在变成基础设施

现代 Agent 系统的共识很明确：

- 没有 trace，就无法解释复杂时延
- 没有指标，就无法比较不同 provider
- 没有 evals，就无法稳定演进 prompt、skill、tool、memory 策略

语音 Agent 尤其依赖以下指标：

- ASR latency
- endpoint latency
- first token latency
- first audio latency
- total response latency
- interruption accepted ratio
- empty-ASR ratio
- tool success ratio
- memory hit ratio

对本项目的启示：

- 当前项目已经开始有 runner report，但还不够系统
- 后面所有“更像伙伴”的工作，都需要靠可观测数据来验证
- 如果没有 tracing/evals，后续会频繁陷入“体感变好了还是变差了”的反复争论

### 6. Workflow / handoff / subagent 正在上移为 runtime 能力

新的 Agent 框架不再把一切都当成单个 LLM 一次性回答。

它们更倾向于：

- 用状态图或工作流来描述多步执行
- 让不同 agent/skill 在共享上下文里协作
- 允许中断、恢复、审批、重试

对本项目的启示：

- 当前项目还主要是“单运行时 + tools”
- 这对 P0/P1 足够，但对更复杂的家庭语音体验还不够
- 后续应该有一个轻量 workflow 层，承载：
  - 家控
  - 状态查询
  - 提醒/记忆
  - 设备诊断
  - 多步骤问答

### 7. 多模态与屏幕上下文正在成为语音 Agent 的差异化能力

如果 Agent 运行在家庭中控屏上，现代系统不会把它当作纯音箱。

更合理的方向是：

- 语音只是输入输出方式之一
- screen state 也是 runtime context
- 页面焦点、当前卡片、设备面板、用户操作轨迹，都可以成为 agent context

对本项目的启示：

- 当前项目做“家庭中控屏语音助手”，天然应该把 screen context 纳入设计
- 这不是前端特例，而应进入共享 runtime context 模型

## 对当前项目的具体判断

### 方向上已经正确的部分

以下部分已经符合现代框架方向，不建议推翻：

#### 1. 会话中心架构

当前架构已明确把 `Realtime Session Core` 作为系统中心，这一点应继续保留。

相关文档：

- [Architecture Overview](overview.md)

#### 2. runtime 与 transport 已经分开

当前 `TurnExecutor` 边界明确，模型 provider 没有泄露到 gateway 或 channel。

这比很多“先写通流程再说”的项目更健康。

#### 3. Voice Runtime 已经是共享层，而不是浏览器私有逻辑

TTS 和 ASR 已经放进共享 `internal/voice`，这条线是正确的。

这意味着：

- RTOS
- Web/H5
- 桌面调试工具
- 未来渠道接入

都可以复用同一条语音输出路径，而不是各写一套。

#### 4. domain behavior 已经开始进入 runtime skill

这解决了一个很关键的问题：

- 设备控制语义不再硬塞进 executor 核心分支
- household 行为开始通过 runtime skill 注入

这与“更 AI-native，而不是更多硬编码”这一目标一致。

#### 5. prompt 结构已经分层

目前已经把：

- persona
- runtime output contract
- execution-mode policy
- skill prompt fragments

拆成组合式结构，这比一个巨大的系统提示词更可维护。

### 当前项目的主要差距

下面这些是“方向正确但能力还不够”的部分。

#### 1. 语音 runtime 仍然偏基础版

当前项目的实时语音路径仍然主要依赖：

- client wakeup
- client commit
- 基础 interrupt
- 基础 streamed text / audio

这已经能工作，但与现代 voice-agent runtime 相比，还缺：

- partial ASR 参与 turn control
- 更细粒度 endpoint reason
- listen / think / speak / playout 分离状态
- 更完整的 cancel chain
- 更成熟的 double-talk / overlap 策略

#### 2. `turn_mode` 仍是 bring-up 友好，而不是最终体验友好

当前的 `client_wakeup_client_commit` 对设备接入方清晰，但它不是长期最佳用户体验形态。

它的优点：

- 协议稳定
- 设备责任清晰
- RTOS bring-up 成本低

它的局限：

- server 无法更深参与 turn detection
- follow-up listen 与语义停顿判断较弱
- 更难做自然的“像伙伴”对话节奏

#### 3. 结构化语音理解还没有真正进入策略层

虽然语音层已经预留 speech metadata 归一化边界，但当前 runtime 还没有充分利用这些信号。

后续真正应该让以下信号进入 runtime 决策：

- detected language
- endpoint reason
- emotion / speaking style
- speaker or speaker confidence
- audio events
- partial hypotheses

#### 4. 记忆仍然不够 durable

当前 memory backend 的问题不在是否存在，而在：

- 仍是 in-process
- 缺少 durable storage
- 缺少 recall ranking
- 缺少 retention / expiry / correction policy
- 缺少 household identity graph

对家庭场景来说，这会限制“连续性”和“熟悉感”。

#### 5. skill 体系仍然偏 builtin，而不是平台化

当前 runtime skill 已经建立方向，但还没有形成完整平台能力。

还需要补的包括：

- skill registry
- provider-neutral tool schema registry
- skill manifest
- enable/disable policy
- skill-level safety policy
- MCP server/client bridge

#### 6. 缺少统一 tracing / evals

当前仓库已有一些质量报告，但还没有形成完整 observability 体系。

缺少后续会直接影响：

- provider 选型
- prompt 调优
- ASR/TTS 切换
- turn-control 策略升级
- skill 行为回归验证

#### 7. 还没有真正的 workflow / handoff 层

现在 runtime 更像：

- 一个 shared executor
- 加 memory
- 加 tools

但现代 agent runtime 更进一步，会把多步过程建模成：

- graph
- workflow
- handoff
- interruptible tasks

这部分不是现在就必须重构，但值得尽快预留。

#### 8. 中控屏场景下的 screen context 仍未进主链路

如果长期目标是“家庭中控屏上的高端语音中控助手”，那页面上下文就不应只是 UI 层状态。

更合适的方式是让 runtime 可接收：

- 当前页面
- 当前房间
- 当前设备卡片
- 最近一次触控操作
- 当前展示的告警或状态

这会显著提升“懂上下文”的主观感受。

## 当前项目是否需要优化

答案是：需要，而且现在就应该开始，但不需要推倒现有架构。

理由很简单：

1. 主架构已正确，重构成本高，收益低
2. 当前差距主要集中在 voice runtime、skill、memory、observability 等可渐进增强的层
3. 这些能力一旦补齐，当前架构完全可以继续承载更复杂的 RTOS、Web、桌面和渠道接入

因此更合理的策略是：

- 保留当前分层
- 强化 runtime 能力
- 标准化扩展边界
- 用指标和 eval 驱动后续演进

## 建议的优化方向

### P0：先补现代 voice-agent 基础设施

#### P0-1 语音会话编排升级

目标：

- 让系统从“可用语音链路”升级到“现代化语音会话 runtime”

执行方向：

- 抽象 `TurnDetector` 或 `EndpointingPolicy`
- 增加 partial ASR 参与 turn-control 的能力
- 明确 `listen / thinking / speaking / playout / interrupted` 状态
- 统一 interrupt、cancel、playout completion 的传播链
- 对 server-side VAD 采用“可选增强”而不是一次性替换当前 contract

#### P0-2 可观测性与评估体系

目标：

- 让语音体验优化有数据依据，而不是只靠体感

执行方向：

- 引入每轮 trace id
- 记录 ASR、LLM、TTS、tool、session 状态转换时间点
- 统一 runner/eval report 输出
- 建立可回归的 turn test fixture

#### P0-3 Skill Registry 与 MCP Bridge

目标：

- 把“技能化扩展”真正做成架构能力

执行方向：

- 为 runtime skill 增加 manifest 和 registry
- skill 贡献 prompt fragment、tool schema、tool executor、policy metadata
- 接入 MCP client/server bridge
- 保证 transport 不直接依赖 skill/provider 实现细节

#### P0-4 Durable Household Memory

目标：

- 让助手具备持续上下文，而不是只记得最近几轮

执行方向：

- 增加 durable memory backend
- 明确 `session / user / device / room / household` 的落盘策略
- 增加 recall ranking 和 retention policy
- 区分 conversation memory、preference memory、environment memory

### P1：把 runtime 从“带工具的单 agent”升级为“可编排 agent runtime”

#### P1-1 轻量 workflow / state graph

优先支持：

- 多步任务
- 设备状态确认
- skill handoff
- 审批或澄清
- 中断后恢复

#### P1-2 Screen Context 进入共享 runtime

优先支持：

- 当前页面或卡片
- 当前房间上下文
- 最近一次 UI 交互
- 当前设备列表或选中对象

#### P1-3 Skill-level Policy

对敏感域单独建策略层，而不是写死在 prompt 里：

- 门锁
- 安防
- 燃气
- 远程控制
- 自动化场景变更

#### P1-4 渠道与设备接入统一化

目标不是多做几种接入，而是让所有接入都更像“同一 runtime 的不同 adapter”。

### P2：继续向“像伙伴”推进

#### P2-1 伴随式语音体验

可探索：

- follow-up listening
- 更自然的短确认
- 更克制的 backchannel
- 语音风格随情境轻度变化

#### P2-2 有边界的主动性

例如：

- 只在高置信、低风险、强相关场景下主动提醒
- 所有主动性都必须可解释、可关闭、可审计

#### P2-3 更前沿的语音形态实验

包括但不限于：

- speech-to-speech
- 更原生的对话韵律
- 重叠说话和更细粒度插话

但这类探索不应早于：

- observability
- skill policy
- durable memory
- 安全策略

## 不建议做的事情

基于这次复核，以下方向不建议优先做：

### 1. 直接把项目替换为某个外部框架

原因：

- 会带来较大迁移成本
- 会破坏已经形成的 RTOS 和 realtime contract
- 真正的短板并不在现有边界定义

### 2. 重新把家控规则硬编码回 executor 或 gateway

原因：

- 这会再次削弱 runtime 的可扩展性
- 会与“skill 化扩展”的方向冲突

### 3. 在没有指标体系前追求端到端 speech-to-speech

原因：

- 很容易牺牲可控性与可调试性
- 不适合作为当前家庭中控项目的第一优先级

### 4. 把 Web/H5 调试页做成第二套 orchestration

原因：

- 当前正确方向是 browser 复用 native realtime contract
- debug surface 应继续属于 control-plane bring-up，而不是新的 runtime

## 建议的近期执行顺序

如果只看未来 6 到 8 周，建议顺序如下：

1. 先做 `voice runtime orchestration + observability`
2. 再做 `skill registry + MCP bridge`
3. 再做 `durable household memory`
4. 然后引入轻量 `workflow / handoff`
5. 最后把 `screen context` 和更强的“伙伴感”能力接入

这个顺序的好处是：

- 先把基础实时体验做稳
- 再把扩展方式做对
- 再把长期智能化能力做深

## 与现有文档的关系

建议与以下文档一起阅读：

- [Architecture Overview](overview.md)
- [Runtime Configuration](runtime-configuration.md)
- [语音 Agent 伙伴化研究（2026-04-04）](voice-agent-companion-research-zh-2026-04.md)
- [项目优化路线图（2026-04-04）](project-optimization-roadmap-zh-2026-04.md)

本文与现有文档的关系如下：

- `voice-agent-companion-research-zh-2026-04.md`
  - 更偏“为什么伙伴感重要、开放生态在怎么做”
- `project-optimization-roadmap-zh-2026-04.md`
  - 更偏“当前项目要按 P0/P1/P2 做哪些事”
- 本文
  - 更偏“结合最新现代框架，对当前项目再复核一次，并明确什么该优化、什么不该推翻”

## 外部参考资料

以下链接为本次复核时重点参考的官方资料入口：

- LiveKit Agents: <https://docs.livekit.io/agents/>
- LiveKit turn detection: <https://docs.livekit.io/agents/v1/build/turn-detection/>
- LiveKit external data: <https://docs.livekit.io/agents/build/external-data/>
- Pipecat Docs: <https://docs.pipecat.ai/>
- OpenAI Voice Agents Guide: <https://platform.openai.com/docs/guides/voice-agents>
- OpenAI Agents SDK: <https://openai.github.io/openai-agents-js/>
- LangGraph Docs: <https://docs.langchain.com/oss/javascript/langgraph/overview>
- Model Context Protocol: <https://modelcontextprotocol.io/docs/learn/architecture>
- Home Assistant Voice Pipelines: <https://developers.home-assistant.io/docs/voice/pipelines>
- Home Assistant LLM API: <https://developers.home-assistant.io/docs/core/llm/>
- Google ADK Docs: <https://google.github.io/adk-docs/>
- AutoGen Docs: <https://microsoft.github.io/autogen/>

以上资料用于提炼架构共性，不代表本项目要直接替换为其中任一框架。
