# 语音架构执行路线图（2026-04-16）

## 文档性质

- 性质：从正式蓝图到工程落地的执行路线图
- 依赖基线：`docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`
- 适用对象：服务端开发、语音算法/worker 开发、嵌入式/客户端开发、测试与评测同学
- 目标：把当前语音蓝图拆成可执行阶段、文件级任务、验收指标与验证命令

## 一句话结论

当前项目最合理的落地顺序，不是“同时把所有语音能力做满”，而是按体验杠杆和架构依赖推进：

1. 先把 `preview -> endpoint accept -> response.start -> first audible` 这条主路径打稳
2. 再把 `duck_only/backchannel + playback truth + resume` 做出真实全双工手感
3. 再把 `dynamic bias / alias / slot completeness / risk gating` 接到领域智能上
4. 最后把评测、协议公开化、客户端能力协同做完整

换句话说，当前阶段最关键的不是再加更多模型，而是让：

- preview 成为主路径信号
- output 更早起播
- interruption 更自然
- heard-text 更可信
- 客户端与服务端围绕同一协议节奏协同

## 目录

1. 当前现状与执行约束
2. 执行原则
3. 阶段总览
4. Phase 0：基线、观测与验证面补齐
5. Phase 1：Preview-first 主路径毕业
6. Phase 2：Output orchestration 与更早起播
7. Phase 3：Playback truth、resume 与真实全双工
8. Phase 4：领域智能与高风险动作 gating
9. Phase 5：协议公开化与客户端协同毕业
10. 跨阶段测试矩阵
11. 里程碑完成定义

## 1. 当前现状与执行约束

## 1.1 已有基础

当前仓库已经具备以下关键基础：

- `internal/session` 已有 `input_state / output_state` 双轨表达
- `internal/voice` 已有 preview、turn detector、barge-in、speech planner、session orchestrator 等主干能力
- `internal/gateway` 已有 native realtime 与 `xiaozhi` 的共享 turn/output 流转基础
- `workers/python` 已有本地 FunASR 2pass 路径与 CosyVoice GPU TTS 路径
- `docs/protocols/realtime-session-v0.md` 与 `docs/protocols/rtos-device-ws-v0.md` 已支持当前兼容基线与 `server_endpoint` candidate 的说明
- 当前 tracing 已能记录 preview / first text / first audio 等关键里程碑

## 1.2 当前真正的缺口

真正阻碍体验继续提升的，不再是“完全没有能力”，而是下面几类能力还没收束成稳定主路径：

- `preview partial` 尚未成为端侧稳定可感知主链路
- `server_endpoint` 还是 candidate，不是完整编排中心
- `duck_only/backchannel` 虽已有运行时路径，但行为深度不足
- output 仍未完全做到“意群一稳定就起播”
- playback truth 仍然偏 heuristic，导致 resume / memory 自然度受限
- 领域 bias / alias / slot completeness 还停留在研究层

## 1.3 执行约束

- 不引入第二套实时协议家族
- 不让 adapter 成为第二语音编排层
- 不破坏 `Realtime Session Core` 为中心的架构
- 优先兼容当前 discovery 与公开 v0 合同
- 继续坚持 `local/open-source first` 的语音主路径
- 优先让真实设备体感提升，而不是只改善离线 benchmark

## 2. 执行原则

### 2.1 优先压体验里程碑，不优先压总时延

每个阶段都要围绕这些里程碑评估：

- `speech_start visible`
- `preview_first_partial`
- `endpoint_accept`
- `response.start`
- `first_audio_chunk`
- `first_audible_playout`
- `barge_in_cutoff`

### 2.2 先做可逆前推，再做不可逆提交

对于 preview、draft、planner、TTS prewarm 等：

- 可以早做
- 允许撤销

对于设备控制、高风险工具调用、memory 强写入：

- 必须等更强 final / completeness / risk gating

### 2.3 先补 runtime 共享能力，再考虑 protocol graduation

- 内部 runtime 路径先成熟
- 然后通过 additive capability 暴露给 client
- 避免协议先行、实现跟不上

### 2.4 端侧协同只拿必要事实，不下放主裁决

端侧重点提供：

- local reflex
- playback control
- playback facts
- fallback

而不是重新拥有 turn accept 主判断。

## 3. 阶段总览

| Phase | 目标 | 主收益 | 主要风险 | 是否依赖 client 协同 |
| --- | --- | --- | --- | --- |
| `P0` | 基线、观测、验证面补齐 | 后续优化可测量 | 只加日志不改体验 | 否 |
| `P1` | Preview-first 主路径毕业 | 更早感知、更自然 accept | false endpoint | 低 |
| `P2` | Output orchestration 与更早起播 | 更快开口、更像真人 | false start / chunking 生硬 | 低到中 |
| `P3` | Playback truth 与 resume | interruption 更自然 | client telemetry 不稳定 | 高 |
| `P4` | 领域智能与风险 gating | 更准、更智能、更安全 | 过度 bias / 错误执行 | 低到中 |
| `P5` | 协议公开化与 client 协同毕业 | RTOS/client 并行开发顺畅 | 兼容性管理复杂 | 高 |

## 4. Phase 0：基线、观测与验证面补齐

## 4.1 目标

- 把后续优化所需的关键指标补齐
- 把真实设备与 runner 场景固定成可回归基线
- 为 preview、playback truth、barge-in、早起播建立统一 artifact 约定

## 4.2 非目标

- 不在本阶段大改 endpoint 策略
- 不在本阶段公开新协议事件
- 不在本阶段接领域 bias 主路径

## 4.3 主要任务

### 服务端文件级任务

- `internal/gateway/preview_trace.go`
  - 补 `preview_stable_prefix_latency_ms`
  - 区分 first partial 与 stable partial
- `internal/gateway/turn_trace.go`
  - 增加 `turn_accept_latency_ms`
  - 区分 `first_audio_chunk` 与 `first_audible_playout` 预留字段
- `internal/gateway/output_flow.go`
  - 为 `playback_id / segment_id / expected_duration_ms` 预留内部记录点
- `internal/voice/session_orchestrator.go`
  - 补齐 delivered/heard/truncated 内部记录粒度
- `internal/voice/barge_in.go`
  - 输出更结构化的 accepted / rejected 决策原因
- `internal/app/config_voice.go`
  - 集中整理与 preview、endpoint、planner、barge-in、playback-truth 相关配置项说明

### worker / client / artifact 任务

- `workers/python/src/agent_server_workers/funasr_service.py`
  - 输出 preview endpoint hints 的更稳定内部调试字段
- `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - 增加场景级指标归档：preview first/stable、accept、first audio、first heard、cutoff
- `docs/codex/live-validation-runbook.md`
  - 增补语音主线路径回归采样与 artifact 目录约定

## 4.4 验收指标

- 所有关键里程碑都能在 runner artifact 或服务端日志中追踪
- 至少有一组固定场景能回归：
  - 短问答
  - 短命令
  - speaking-time interruption
- 不新增公开协议破坏

## 4.5 建议验证

- `go test ./internal/gateway ./internal/voice ./internal/session`
- `make test-go`
- `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest clients.python-desktop-client.tests.test_runner`

## 5. Phase 1：Preview-first 主路径毕业

## 5.1 目标

- 让 `preview partial` 成为真实主链路输入之一，而不只是隐藏调试信号
- 让 `server_endpoint` 从“候选能力”进入“稳定主路径候选”
- 让服务端稳定输出：speech start / partial updated / endpoint candidate / accept reason

## 5.2 非目标

- 不在本阶段做完整 resume
- 不在本阶段引入毫秒级 playback cursor
- 不在本阶段接入复杂领域 bias 策略

## 5.3 主要任务

### turn-taking 与 preview 主链

- `internal/voice/turn_detector.go`
  - 将 acoustic + lexical + semantic completeness 融合为更稳定的 endpoint evidence
  - 引入 incomplete hold / correction risk / continuation risk 的统一解释
- `internal/voice/input_preview*.go`（实际存在的 previewer/session 文件）
  - 明确 stable prefix 语义
  - 区分 raw partial 与 stable partial
- `internal/gateway/realtime_ws.go`
  - 让 preview 观察与 accepted-turn 更新路径统一
  - 兼容当前 `session.update` 语义，不制造 adapter-local accept logic
- `internal/gateway/xiaozhi_ws.go`
  - 保持 compat adapter 只做翻译，不复制 endpoint 逻辑
- `internal/app/config_voice.go`
  - 统一 preview / endpoint / hold / hint 配置
- `internal/control/info.go`
  - discovery 对 `server_endpoint` 能力说明更清楚

### worker 侧

- `workers/python/src/agent_server_workers/funasr_service.py`
  - 稳定输出 online preview + final correction
  - 把 endpoint hint 继续保持为 worker-internal evidence，不向 adapter 泄露 provider 细节
  - KWS 继续保持 default-off

### client / debug

- `clients/web-realtime-client/*`
- `/debug/realtime-h5/*`
  - 让 preview partial 在调试页面上可视化
  - 显示 accept reason / lane state

## 5.4 文件级任务清单

- `internal/voice/turn_detector.go`
- `internal/voice/turn_detector_test.go`
- `internal/voice/session_orchestrator.go`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `internal/gateway/preview_trace.go`
- `internal/app/config_voice.go`
- `internal/control/info.go`
- `workers/python/src/agent_server_workers/funasr_service.py`
- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`
- `schemas/realtime/session-envelope.schema.json`（仅当公开字段/事件真正毕业时）

## 5.5 验收指标

- `preview_first_partial_latency_ms` 稳定可测
- `preview_endpoint_candidate_latency_ms` 稳定可测
- `accept_reason=server_endpoint` 在真实语音场景下稳定出现
- 端侧能在不依赖显式 `audio.in.commit` 的情况下完成基础短命令
- false endpoint 率可控，不显著劣化识别/交互体验

## 5.6 建议验证

- `go test ./internal/gateway ./internal/voice`
- `go test ./internal/session ./internal/gateway ./internal/voice`
- `make test-go-integration`
- 新增或固定 runner 场景：
  - `server-endpoint-preview`
  - `short-command-no-commit`
  - `wake-followed-command`

## 6. Phase 2：Output orchestration 与更早起播

## 6.1 目标

- 让首个稳定意群出现后即可启动 TTS
- 让 `response.start`、`response.chunk(text)`、`audio.out.chunk` 生命周期真实重叠
- 把 `duck_only/backchannel` 从“分类结果”推进成更可信的输出策略

## 6.2 非目标

- 不在本阶段做完整 playback-truth 协议公开
- 不在本阶段追求 token 级超碎片输出

## 6.3 主要任务

### output orchestrator

- `internal/voice/speech_planner.go`
  - clause/sense-group 切分
  - 首句优先策略
  - 短确认语 + 回答骨架策略
- `internal/voice/asr_responder.go`
  - 让 draft / planner / final response 生命周期更清晰
- `internal/voice/synthesis_audio.go`
  - incremental TTS session 管理
  - segment 级 synth 生命周期
- `internal/gateway/turn_flow.go`
  - 更早接入 `TurnResponseFuture` / planner audio
- `internal/gateway/output_flow.go`
  - 管理 output lane 与 ducking / interruption 后的输出状态

### interruption 行为深度

- `internal/voice/barge_in.go`
  - 正式引入两段式：`intrusion_prior` + `takeover_confirmation`
- `internal/voice/session_orchestrator.go`
  - speaking-time preview 与 output directive 的统一协调
- `internal/gateway/realtime_ws.go`
  - 避免把 soft policy 判断重新散落到 gateway 分支逻辑里

## 6.4 文件级任务清单

- `internal/voice/speech_planner.go`
- `internal/voice/speech_planner_test.go`
- `internal/voice/asr_responder.go`
- `internal/voice/asr_responder_test.go`
- `internal/voice/synthesis_audio.go`
- `internal/voice/barge_in.go`
- `internal/voice/session_orchestrator.go`
- `internal/gateway/turn_flow.go`
- `internal/gateway/output_flow.go`
- `internal/gateway/realtime_ws.go`

## 6.5 验收指标

- `first_audio_chunk_latency_ms` 明显前移
- 新增 `first_audible_playout_latency_ms` 后能看到可观前移
- `duck_only` 能明显减少误硬停
- `backchannel` 不默认触发 hard interrupt
- speaking-time preview 与 output overlap 场景稳定

## 6.6 建议验证

- `go test ./internal/voice ./internal/gateway`
- `go test ./internal/voice ./internal/gateway -run 'BargeIn|Planner|Realtime'`
- 新增或固定 runner 场景：
  - `early-audio-short-answer`
  - `speaking-time-brief-interruption`
  - `backchannel-no-hard-stop`

## 7. Phase 3：Playback truth、resume 与真实全双工

## 7.1 目标

- 拿到至少 Tier 1 级 playback facts
- 建立 `generated / delivered / heard / truncated` 的可信区分
- 支撑 interruption 后的 continue / resume / memory truth

## 7.2 非目标

- 不强制第一阶段就做毫秒级 playhead
- 不要求所有 adapter 一次支持到 Tier 3 audibility aware

## 7.3 主要任务

### 服务端

- `internal/voice/session_orchestrator.go`
  - playback fact ingest
  - heard-text derivation
  - resume anchor 生成
- `internal/gateway/output_flow.go`
  - `playback_id / segment_id / response_id` 关联
- `internal/session/realtime_session.go`
  - speaking / interrupted / active return 的状态过渡更精确
- `internal/agent` memory 相关路径
  - 区分 generated text 与 heard text 写回（按当前仓库实际 memory store 路径实现）

### 协议与 client

- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`
- `schemas/realtime/session-envelope.schema.json`
- `clients/web-realtime-client/*`
- `clients/python-desktop-client/*`
- 嵌入式 client
  - 增加 `audio.out.started`
  - 增加 `audio.out.mark`
  - 增加 `audio.out.cleared`
  - 增加 `audio.out.completed`
  - 至少支持 segment-level ack

## 7.4 文件级任务清单

- `internal/voice/session_orchestrator.go`
- `internal/voice/session_orchestrator_test.go`
- `internal/gateway/output_flow.go`
- `internal/session/realtime_session.go`
- `internal/session/realtime_session_test.go`
- `internal/agent/*memory*`
- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`
- `schemas/realtime/session-envelope.schema.json`
- `clients/web-realtime-client/*`
- `clients/python-desktop-client/*`

## 7.5 验收指标

- 服务端能稳定记录 `delivered / heard / interrupted / truncated / playback_completed`
- 至少有一个 client profile 支持 segment-level playback ack
- interruption 后 memory 不再默认写完整未播文本
- resume / continue 场景具备基本自然性

## 7.6 建议验证

- `go test ./internal/session ./internal/gateway ./internal/voice ./internal/agent`
- `make test-go-integration`
- 新增 runner / RTOS mock 场景：
  - `playback-mark-complete`
  - `clear-mid-playback`
  - `interrupt-then-resume`
  - `heard-text-memory-writeback`

## 8. Phase 4：领域智能与高风险动作 gating

## 8.1 目标

- 把研究中的 `dynamic bias + alias + entity catalog + slot completeness` 变成真实运行时能力
- 让智能家居 / 桌面助理语音交互更准、更自然、更安全

## 8.2 非目标

- 不做超大一体化知识库
- 不把所有领域规则硬编码到 gateway 或 executor

## 8.3 主要任务

### 领域识别增强

- `internal/voice` 或 `internal/agent` 间的共享结构
  - dynamic top-K bias context
  - alias resolution
  - entity canonicalization
- `workers/python/src/agent_server_workers/funasr_service.py`
  - preview / final 两条路径不同强度 bias 接入策略

### 风险 gating

- `internal/agent` skill/tool 执行前
  - 接入 `slot completeness`
  - 接入 `action risk`
  - 区分低风险可早回复与高风险必须确认

### skill 层

- `internal/agent` runtime skill / household skill 路径
  - 不直接污染 core executor
  - 通过 runtime-owned prompt/tool/memory 组合接入

## 8.4 文件级任务清单

- `internal/agent/*`
- `internal/voice/*domain*`（若新增）
- `workers/python/src/agent_server_workers/funasr_service.py`
- `docs/architecture/dynamic-bias-alias-entity-catalog-mvp-zh-2026-04-16.md`（回填实施状态）
- `docs/architecture/slot-completeness-computable-object-zh-2026-04-16.md`（回填实施状态）

## 8.5 验收指标

- 智能家居 / 桌面助理常用实体识别准确率明显提升
- 多实体歧义场景下降
- 高风险动作不再因 preview 过早执行
- 澄清语句比“机械报错”更自然

## 8.6 建议验证

- 领域词表回归集
- 多 alias / 拼音近音回归集
- 高风险动作确认回归集
- 真实设备场景验证：
  - 家居设备控制
  - 桌面对象操作

## 9. Phase 5：协议公开化与客户端协同毕业

## 9.1 目标

- 让嵌入式、桌面、Web 调试 client 能围绕统一 capability 协同开发
- 让 preview / playback truth 逐步从内部能力公开为加法协议能力

## 9.2 非目标

- 不废弃当前 v0 兼容基线
- 不制造第二套专用浏览器/专用嵌入式协议

## 9.3 主要任务

- capability-gated 公开：
  - preview events
  - endpoint candidate events
  - playback ack events
- 明确 discovery 协商对象
- 更新 schema、docs、reference clients、RTOS mock
- 推出 capability matrix：
  - baseline only
  - preview aware
  - playback-truth aware
  - full collaboration profile

## 9.4 文件级任务清单

- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`
- `schemas/realtime/session-envelope.schema.json`
- `clients/web-realtime-client/*`
- `clients/python-desktop-client/*`
- `internal/gateway/realtime.go`
- `internal/control/info.go`

## 9.5 验收指标

- 嵌入式 client 可只靠文档并行开发 preview-aware client
- 至少一个 reference client 与一个 RTOS mock 能跑通 capability-gated 新路径
- 老 client 在忽略新字段/新事件时仍能工作

## 9.6 建议验证

- `go test ./internal/control ./internal/gateway ./internal/app`
- `make verify-fast`
- reference client 与 RTOS mock 双端回归

## 10. 跨阶段测试矩阵

| 测试桶 | 关注点 | 推荐归属 |
| --- | --- | --- |
| 短问答 | preview / endpoint / first audible | P1/P2 |
| 短命令 | completeness / accept / action gating | P1/P4 |
| speaking-time interruption | duck_only / hard interrupt / cutoff | P2/P3 |
| playback truth | mark / clear / completed / heard-text | P3 |
| 领域 alias | bias / canonicalization / disambiguation | P4 |
| 高风险动作 | confirm / final authority / no early execute | P4 |
| client capability | additive protocol / fallback compatibility | P5 |

## 11. 里程碑完成定义

### `P1 done`

- preview partial 成为主路径信号
- server endpoint 在真实短命令中稳定可用
- accept reason 与 endpoint candidate 稳定可观测

### `P2 done`

- 首句可明显更早起播
- backchannel / duck_only 不再只是日志分类
- speaking-time preview 与 output overlap 稳定

### `P3 done`

- 至少一个 client profile 支持 playback Tier 1 facts
- heard-text 不再默认等于 generated text
- interruption 后 resume / memory 明显更自然

### `P4 done`

- 家居 / 桌面场景实体识别显著提升
- 高风险动作可通过 completeness + risk gating 控制

### `P5 done`

- 协议、schema、reference client、嵌入式 client 协作路径清晰稳定
- 兼容基线未破坏

## 相关文档

- `docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`
- `docs/architecture/realtime-full-duplex-gap-review-zh-2026-04-15.md`
- `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
- `docs/architecture/latency-budget-and-subjective-feel-zh-2026-04-16.md`
- `docs/architecture/playback-facts-and-heard-text-truth-chain-zh-2026-04-16.md`
- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`
