# 项目优化路线图（2026-04-04）

## 文档目的

本文基于当前 `agent-server` 代码与协议实现，整理一份面向下一阶段的中文优化路线图。

目标不是泛泛列愿望，而是把当前系统从“已经具备语音接入和基础 Agent 运行时”推进到“更像一个家庭语音伙伴”的方向，并按 `P0 / P1 / P2` 拆成可执行任务。

相关背景研究：

- [语音 Agent 伙伴化研究（2026-04-04）](voice-agent-companion-research-zh-2026-04.md)
- [Voice Agent Companion Research (2026-04-04)](voice-agent-companion-research-2026-04.md)

## 当前判断

当前项目已经具备以下基础：

- 原生 `rtos-ws-v0` 实时会话链路已打通
- `xiaozhi` 兼容适配层已可完成首版 RTOS 语音接入
- `Agent Runtime Core`、`Voice Runtime`、`Gateway` 基本完成边界拆分
- 已具备本地 ASR、流式 TTS、可选云 ASR/TTS、可选云 LLM
- 已有流式 delta、内存后端、工具后端、家庭中控型默认提示词

但当前系统离“更像伙伴”还有明显距离，主要问题集中在：

- runtime 智能化还偏薄，LLM 路径仍接近单轮文本生成
- 记忆仍然是进程内、短期、弱结构化
- ASR 结果只有 transcript，缺少语音理解元数据
- `turn_mode` 命名与实际行为存在偏差
- 会话层有明确的 turn audio 拷贝开销问题
- 家庭实体上下文、房间、设备、场景模型尚未进入 runtime
- 多模态与屏幕上下文只做了预留，未真正打通

## 排序原则

### P0

先修“正确性、成本、语义一致性、观测性”，避免后续所有智能化工作建立在错误抽象上。

### P1

在既有 runtime 边界上补齐真正的 agent 能力，包括工具循环、分层记忆、结构化语音理解、家庭上下文。

### P2

在 P0/P1 稳定后，再推进更强的陪伴感、多模态、个性化、主动交互能力。

## P0：先把基础做实

### P0-1 修复 turn audio 缓冲与 Snapshot 拷贝路径

目标：

降低实时音频路径的内存拷贝和累计开销，避免长句或长轮次下出现 O(n²) 式退化。

执行项：

- 将会话状态快照与 turn audio buffer 解耦
- 音频帧写入时仅更新轻量统计信息，不返回整段累计音频副本
- 在 `commit` 阶段一次性导出本轮音频给 ASR
- 为长轮次和高帧率场景补回归测试与基准测试

主要文件：

- `internal/session/realtime_session.go`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `internal/session/realtime_session_test.go`

验收标准：

- 单轮 30 到 60 秒语音输入时无明显内存拷贝放大
- `go test ./...` 通过
- 新增性能或分配回归测试，能验证修复效果

### P0-2 对齐 `turn_mode`、状态语义和真实行为

目标：

消除“协议声明”和“实际实现”之间的偏差，避免设备侧和后续 channel 侧基于错误语义做集成。

执行项：

- 明确当前模式到底是 `client_commit` 还是 `server_vad`
- 如果短期不实现服务端 VAD，则调整 discovery、配置默认值和文档命名
- 如果要保留 `client_wakeup_server_vad`，则补上最小可用 server-side VAD 流程
- 真正启用或移除 `armed` 状态，避免状态机名义存在、运行时不用

主要文件：

- `internal/app/config.go`
- `internal/gateway/realtime.go`
- `internal/gateway/realtime_ws.go`
- `internal/session/types.go`
- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`

验收标准：

- `GET /v1/realtime` 返回的 `turn_mode` 与实际行为一致
- 文档、代码、测试对状态流的描述一致
- 设备接入方不再需要靠猜测理解 commit 责任边界

### P0-3 拆分“人格”与“执行模式”

目标：

让默认家庭助手人格可以稳定复用，同时把调试阶段仿真执行逻辑从人格层中独立出来，为后续真实设备执行做准备。

执行项：

- 新增独立配置，如 `persona` 与 `execution_mode`
- 将当前“调试阶段、仿真成功式反馈”从默认提示词中拆成 mode policy
- 让 `simulation / dry_run / live_control` 可以在 runtime 层切换
- 保持对 `{{assistant_name}}` 的兼容

主要文件：

- `internal/agent/llm.go`
- `internal/agent/llm_executor.go`
- `internal/app/config.go`
- `docs/architecture/runtime-configuration.md`
- `.env.example`

验收标准：

- 不改人格即可切换仿真态与实控态
- live 模式下不再遗留调试期确认逻辑
- 现有默认提示词行为通过回归测试覆盖

### P0-4 建立基础可观测性与质量评估

目标：

后续所有“更像伙伴”的优化，都必须可以被量化，而不是只靠主观感受。

执行项：

- 增加关键时延和质量指标
- 记录 ASR、LLM、TTS、首字节、首音频、barge-in 成功率
- 为文本轮、语音轮、打断轮增加统一验证脚本或测试夹具
- 输出可归档的 JSON 报告，便于不同 provider 横向比较

建议指标：

- ASR latency
- first token latency
- first audio latency
- full response latency
- empty ASR ratio
- fallback-to-text ratio
- barge-in accepted ratio
- memory hit ratio
- tool call success ratio

主要文件：

- `internal/voice/*`
- `internal/agent/*`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `clients/python-desktop-client/*`
- `scripts/*`

验收标准：

- 至少能导出一份端到端质量报告
- 能稳定比较不同 ASR/TTS/LLM 配置
- 回归前后能量化看到性能与行为变化

### P0-5 建立优化路线与现有计划的映射

目标：

让当前长期里程碑和短期优化执行不冲突，避免“文档有路线图，实际计划还是旧顺序”。

执行项：

- 在 `plan.md` 中补充 P0/P1/P2 优化轨道
- 保持现有 `M1 / M1.5 / M2 / M3` 里程碑不被破坏
- 将当前 Immediate Next Step 调整到 P0 任务执行

主要文件：

- `plan.md`
- `docs/architecture/project-optimization-roadmap-zh-2026-04.md`

验收标准：

- 路线图与正式计划不冲突
- 后续工作可以按优先级直接挂到当前计划中

## P1：把 runtime 真正做成 agent

### P1-1 引入 `StreamingChatModel` 与工具循环

目标：

让 LLM 路径从“一次性文本生成”升级为“可流式输出、可调用工具、可接收工具结果再继续推理”的 runtime agent。

执行项：

- 在 `ChatModel` 之上增加 streaming 接口
- 支持模型侧 text delta
- 支持模型侧 tool call proposal
- 支持 tool result reinjection
- 增加 step budget、防止无限循环
- 保持现有 `response.chunk` 协议兼容

主要文件：

- `internal/agent/contracts.go`
- `internal/agent/llm.go`
- `internal/agent/llm_executor.go`
- `internal/agent/deepseek_chat.go`
- `internal/voice/turn_executor.go`
- `internal/gateway/realtime_ws.go`

验收标准：

- 支持流式首 token 输出
- 支持真实工具循环而不依赖 `/tool ...` 调试命令
- transport 层无需知道 provider 细节

### P1-2 建立“双层上下文”：近期消息窗口 + 长期记忆

目标：

让系统具备真正可用的多轮连续性，而不是只靠一段 memory summary。

执行项：

- 在 `ChatModelRequest` 中增加最近 4 到 8 轮消息窗口
- 将当前 `InMemoryMemoryStore` 保留为短期或摘要层
- 引入更稳定的 durable memory backend 接口
- 细化 scope：`session / device / user / room / household`
- 加入 retention 与 recall policy

主要文件：

- `internal/agent/contracts.go`
- `internal/agent/runtime_backends.go`
- `internal/agent/llm_executor.go`
- 新增 memory backend 文件

验收标准：

- 多轮指代和跟进显著改善
- 家庭共享设备下不再只有 `device_id` 级记忆
- 能区分“近期上下文”和“长期偏好”

### P1-3 将 ASR 结果升级为结构化语音理解结果

目标：

让 runtime 能理解的不只是文字，还包括语气、语言、说话人和事件，从而支持更自然的策略和回复风格。

执行项：

- 扩展 `TranscriptionResult`
- 增加可选字段：`language / emotion / speaker_id / audio_events / endpoint_reason / partials`
- 把这些字段注入 `TurnInput.Metadata`
- 为不同 ASR provider 做能力差异兼容

主要文件：

- `internal/voice/contracts.go`
- `internal/voice/asr_responder.go`
- `internal/voice/http_transcriber.go`
- `internal/voice/iflytek_rtasr.go`
- `internal/agent/contracts.go`

验收标准：

- runtime 可以消费结构化 speech metadata
- 不同 provider 缺字段时不会破坏兼容性
- 后续 prompt、TTS 风格、记忆策略可基于该元数据工作

### P1-4 新增家庭上下文层与确定性控制路由

目标：

让家庭场景的“聪明”来自明确上下文，而不是纯提示词猜测。

执行项：

- 在 runtime 增加 `Household Context` 能力层
- 建模房间、设备、场景、最近已知状态、用户偏好
- 将普通控制请求优先路由到确定性控制路径
- 将模糊表达、问答、解释类请求交给生成式路径
- 为敏感设备建立显式安全策略

主要文件：

- `internal/agent/*`
- `internal/channel/*` 未来接入时复用
- 工具后端相关文件
- 文档与 ADR

验收标准：

- 家居控制不再完全依赖 LLM 文本输出
- 模糊表达也能被映射到更稳定的家庭语义
- 安全相关设备可单独施加策略

### P1-5 补齐关键 RTOS 兼容能力，不追求完整协议等价

目标：

让 RTOS 路径能稳定承载更强 runtime，而不是停留在“只够打通语音链路”。

执行项：

- 为 `xiaozhi` compat 增加最关键但缺失的能力
- 优先补齐：
  - 音频轮次的 `stt` 回显
  - `iot` 设备状态上报
  - 有效的 `mcp` 能力协商
  - 基础 auth/token 校验
- 不以“完全对齐外部参考实现”为第一目标

主要文件：

- `internal/gateway/xiaozhi_ws.go`
- `internal/gateway/xiaozhi_ota.go`
- `docs/protocols/xiaozhi-compat-ws-v0.md`

验收标准：

- 兼容路径可承载更强家庭上下文输入
- 设备状态不再完全脱离 runtime
- 兼容适配仍然保持适配层定位，不反向侵入核心架构

## P2：做出真正的“伙伴感”

### P2-1 引入 context-aware turn detection 与 follow-up listening

目标：

让系统从“工具式一问一答”升级为更自然的对话流。

执行项：

- 区分暂停与真正说完
- 加入 follow-up listening window
- 支持短确认词和快速更正
- 支持更自然的说话中接管与恢复

主要文件：

- `internal/session/*`
- `internal/voice/*`
- `internal/gateway/realtime_ws.go`
- `docs/protocols/realtime-session-v0.md`

验收标准：

- 用户短暂停顿时误触发率下降
- 跟进问题无需重新完整唤醒或新建体验链路
- 打断与接续更加自然

### P2-2 增加更自然的 TTS 风格控制

目标：

让系统的声音表达更像真实助手，而不是单纯播报。

执行项：

- 增加 calm / concise / warm / urgent 等 style hint
- 把语音理解元数据映射到 TTS 风格
- 优化 `time-to-first-audio`
- 优化句子切分和停顿控制

主要文件：

- `internal/voice/*`
- `internal/agent/*`

验收标准：

- 同一文本在不同情境下能有稳定的表达差异
- 首音频时延继续下降
- 回复更短、更稳、更像服务型家庭助手

### P2-3 打通屏幕上下文与图像输入

目标：

让“家庭中控屏上的语音助手”真正具备屏幕和多模态感知，而不仅是语音壳子。

执行项：

- 补齐 `image.in` 或等价输入路径
- 把当前页面、卡片、设备列表等 UI 状态作为 runtime metadata
- 评估后续是否增加 `ui_hint`、`card` 类 delta
- 保持 transport-neutral 设计，不把 UI 逻辑塞回网关

主要文件：

- `internal/agent/contracts.go`
- `internal/gateway/realtime_ws.go`
- `docs/protocols/realtime-session-v0.md`
- 前端或屏幕侧接入代码

验收标准：

- runtime 能知道当前屏幕上下文
- 对话可以围绕屏幕内容继续展开
- 多模态扩展不破坏现有会话核心边界

### P2-4 建立 speaker-aware 的多用户家庭记忆

目标：

解决共享中控设备下“全家共用一个记忆桶”的问题。

执行项：

- 引入 speaker-aware identity 或弱身份聚类
- 区分个人偏好与家庭公共偏好
- 对用户称呼、习惯、提醒做权限与归属边界

主要文件：

- `internal/agent/*`
- `internal/voice/*`
- memory backend 相关文件

验收标准：

- 多成员家庭下的连续性显著提升
- 不会把一个人的偏好错误投射给所有人

### P2-5 增加“有限、克制、可关闭”的主动能力

目标：

让系统逐步具备伙伴感，但不变成高频打扰型助手。

执行项：

- 建立主动提醒和主动建议的触发策略
- 加入频率限制、时段限制、可关闭控制
- 要求所有主动行为都可解释、可回溯

主要文件：

- `internal/agent/*`
- `internal/control/*`
- 配置与策略相关文档

验收标准：

- 主动能力默认克制
- 用户可关闭、可限制
- 不因“更像伙伴”而牺牲可信度

## 暂不建议优先推进的事项

- 不建议优先做完整 `xiaozhi-server` 协议等价复刻
- 不建议优先接入更多同类 ASR/TTS/LLM provider
- 不建议在当前阶段直接重写为端到端 `speech-to-speech`
- 不建议在没有指标体系前进行大规模“体验优化”

## 建议执行顺序

第一阶段：

- P0-1
- P0-2
- P0-3
- P0-4
- P0-5

第二阶段：

- P1-1
- P1-2
- P1-3
- P1-4
- P1-5

第三阶段：

- P2-1
- P2-2
- P2-3
- P2-4
- P2-5

## 一句话总结

当前项目最不缺的是“继续接更多模型”，最缺的是把已经搭好的 runtime 边界真正填实。先把 P0 做扎实，再推进 P1 的 runtime 智能化，最后做 P2 的陪伴感增强，这样最符合当前仓库架构，也最有机会把系统做成可信、自然、长期可演进的家庭语音 Agent。
