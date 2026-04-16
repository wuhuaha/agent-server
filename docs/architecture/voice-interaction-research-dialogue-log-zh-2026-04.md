# 语音交互研究对话记录（2026-04）

## 目的

- 用于持续记录本项目在“提升语音交互流畅性、自然性、人性化、智能性”方向上的逐轮讨论。
- 当前阶段以研究、分析、对比、决策准备为主，不急于收敛为最终实施方案。
- 记录内容优先沉淀：问题背景、用户关注点、外部参考、当前系统现状、讨论结论、未决问题。

## 记录约定

- 每一轮对话单独追加一个小节，按时间顺序记录。
- 每轮至少记录：用户诉求、上下文、关键分析、引用资料方向、临时结论、待继续探讨的问题。
- 若形成稳定结论，再拆分沉淀到独立专题文档；若尚未形成结论，则保留为研究日志，不提前固化为方案。

---

## Round 001｜2026-04-16｜研究阶段启动

### 用户诉求

- 接下来围绕“尽可能提升语音交互流畅性、自然性、人性化、智能性”展开详细探讨。
- 暂时不要急于修改现有实现，而是先充分讨论、参考优秀系统与论文，做高质量研究。
- 当前项目处于研究阶段，目标是优先提升交互效果，暂时不要在架构上过度设计。
- 需要新建 md 文档，详细记录后续每一轮的交互内容。

### 当前上下文

- 项目已进入实时语音主链优化阶段。
- 先前代码与日志分析已经得到一个重要发现：系统内部已经存在 preview partial 生成链路，但尚未稳定地下发到客户端，因此用户主观感知到的“STT结果出现很慢”并不完全等同于 ASR 本身慢。
- 已另行形成专题记录：`docs/architecture/preview-partial-fast-path-options-zh-2026-04-16.md`。
- 当前这份文档不替代专题分析，而是作为逐轮研究对话总日志。

### 本轮讨论重点

- 明确后续讨论的评估目标不只是“识别正确率”，而是完整的交互体验质量。
- 重点关注的质量维度包括：
  - 首字/首个有效反馈时延
  - 用户是否感到“系统在听、在理解、在思考”
  - 打断与抢话是否自然
  - 回答是否及时起播、是否像人在组织语言
  - 语气、韵律、停顿、确认感是否自然
  - 对上下文和意图的理解是否稳定
- 初步判断：当前阶段最值得优先研究的，不是更复杂的大架构，而是“让实时链路尽早给用户反馈，并让输入/输出行为更像真人对话”。

### 外部参考方向

后续各轮研究将优先参考以下类型资料：

- FunASR 及其流式/两阶段识别、VAD、endpointing、热词与实时工程实践。
- OpenAI Realtime / speech-to-speech 相关官方实践与公开资料。
- Google/DeepMind 在流式 ASR、端点检测、对话式 turn-taking、增量语音交互方面的论文与工程文章。
- Amazon Alexa 在 endpointing、barge-in、自然 turn-taking、低时延语音交互方面的公开论文。
- Apple 在端侧语音、流式识别、语音自然性和设备体验上的公开研究与工程实践。

### 当前临时结论

- 当前最重要的是先建立“研究—讨论—再决策”的节奏，而不是立刻拍板方案。
- 后续讨论需要把“主观体验目标”拆成若干可分析的层：
  - 感知时延
  - turn-taking
  - interruption / backchannel
  - preview / partial 呈现
  - 增量回复与早起播
  - TTS 自然性与情绪/韵律
  - 对话智能与上下文稳健性
- 在研究阶段，应优先借鉴成熟系统已经验证有效的交互原则，再判断哪些适合当前项目最小代价落地。

### 待继续探讨

- 什么才是“用户真正感受到快”的关键触点？是 ASR 首包、preview partial、thinking cue，还是首段可听语音？
- 在当前项目阶段，哪些做法能显著提升体验，但不要求立刻大规模改协议或重构状态机？
- 哪些优秀系统的做法看起来先进，但对当前项目而言实现代价过高或依赖基础设施过重？

---

## Round 002｜2026-04-16｜节奏校准

### 用户补充

- 可以创建一个子 agent 辅助工作。
- 当前先进行充分探讨再进行决策，暂时不要急于制定方案。
- “无需检查并撤销本轮修改的已有文件内容”，当前主要目标是暂停下来充分讨论，而不是否定已有工作。

### 本轮校准后的工作方式

- 允许使用 1 个子 agent 做外部资料并行检索，但主线程仍以高质量分析与讨论为主。
- 不急于形成实施路线图，不提前把讨论伪装成“已定方案”。
- 后续每一轮优先输出：
  - 当前问题的本质拆解
  - 外部实践对照
  - 对本项目的适用性判断
  - 暂不决策的原因

### 本轮状态说明

- 本研究日志已建立，后续继续在此文档追加。
- 下一轮开始，将进入基于外部资料与当前项目现状的正式深度讨论。


## Round 003｜2026-04-16｜第一轮正式研究讨论

### 用户诉求

- 明确当前阶段先充分探讨，不急于制定方案。
- 允许使用 1 个子 agent 做外部资料并行研究。
- 讨论目标聚焦：流畅性、自然性、人性化、智能性。

### 本轮研究结论

- 现代优秀实时语音系统的共同点，并不是先上最重的原生语音大模型，而是先把“早感知、会让话、能打断、早起播、少机械感”做好。
- 对当前项目最关键的研究判断是：
  - `preview partial` 应从内部信号升级为主链路信号
  - `turn-taking` 不能只靠 VAD 静音阈值
  - 真正的全双工核心是输入轨/输出轨并行 + interruption arbitration
  - 自然感很多来自时机、prosody、prompt policy，而不只是模型本身
- 当前阶段更适合优先借鉴的，是 OpenAI / Google / Amazon / Apple / FunASR 这些系统背后的共同原则，而不是照搬其重基础设施实现。

### 已沉淀的专题材料

- `docs/architecture/realtime-voice-experience-research-map-zh-2026-04-16.md`
- `docs/architecture/preview-partial-fast-path-options-zh-2026-04-16.md`

### 暂不决策项

- 暂不在本轮确定完整实施路线。
- 暂不提前收敛到单一模型路线或大规模架构重构。

### 下一轮优先讨论候选

- `preview partial` 如何成为主链路信号
- `turn-taking / interruption` 如何更像真人
- `早起播 + prosody` 如何兼顾更快与更自然


## Round 004｜2026-04-16｜服务侧主导 turn-taking 的边界讨论

### 用户问题

- 设想：端侧发起会话并建立连接后，端侧 VAD 等能力不再参与主决策，仅用于兜底。
- turn-taking、打断等均由服务侧决策。
- 追问：相对于当前方案是否更有优势？优缺点分别是什么？其他开源实践或厂商通常怎么做？

### 本轮核心判断

- 相对当前 `client_wakeup_client_commit` 兼容主路径，这个方向整体上更先进，也更契合“全双工 + 更自然交互”的目标。
- 但更合理的表述不是“端侧完全退出”，而是：
  - `服务侧拥有会话决策权`
  - `端侧保留本地反射权与兜底权`
- 若把端侧彻底降成纯采音器，会损失：
  - AEC / render reference 优势
  - 本地即时 duck / UI 提示
  - 网络抖动时的异常回退能力

### 外部实践归纳

- OpenAI Realtime：明显偏服务侧主导，默认 `server_vad`，也支持 `semantic_vad` 和手动关闭自动 turn detection。
- Gemini Live：同样偏服务侧主导，默认 `automaticActivityDetection`，关闭后才要求客户端显式发送活动起止事件。
- LiveKit：典型混合派；支持 provider 内建 turn detection、STT endpointing、VAD-only、manual，并明确建议即使使用 STT endpointing 也保留 VAD 以获得更快 interruption responsiveness。
- Pipecat：默认是 VAD + smarter turn analyzer，也支持 server-side VAD 或 manual control，强调要明确主导者。
- Vocode：偏服务端编排，把 interruption / endpointing / conversation speed 明确暴露成可调“对话拨盘”。
- FunASR：更像服务侧语音能力底座，提供 streaming ASR / VAD / KWS / 2pass，但并不直接给出完整 turn-taking policy。
- Google / Amazon：公开论文普遍在把 endpointing 和 partial prefetch 向 ASR-aware、semantic-aware、服务侧 richer context 方向推进。
- Apple：强端侧唤醒 + 多阶段判别，并不是“一个本地 VAD 阈值决定一切”。

### 对当前项目的临时结论

- 当前更适合讨论并逐步靠近的边界是：
  - 端侧负责 wake、AEC、audio front-end、即时软 duck、fallback
  - 服务侧负责 preview、endpoint、accept、barge-in arbitration、response start
- 这个方向比继续长期停留在 `client commit` 主路径上更有前景。
- 但仍需避免“纯服务侧 everything”的极端化设定。

### 本轮专题沉淀

- `docs/architecture/server-driven-turn-taking-vs-client-commit-zh-2026-04-16.md`

### 下一轮适合继续深挖的点

- 建连后若由服务侧主导，端侧究竟还应该保留哪些“最小但必要”的本地反射能力？
- 服务侧如何区分 backchannel / duck / hard interrupt / ignore？
- preview partial 具体应如何参与 turn accept，而不仅是参与展示？


## Round 005｜2026-04-16｜并行深挖端侧最小能力与 interruption 分层

### 用户要求

- 继续深挖上一轮提出的两个方向：
  1. 建连后端侧还应保留哪些最小但必要的本地能力
  2. 服务侧如何区分 `backchannel / duck_only / hard_interrupt / ignore`
- 明确要求创建子 agent，并与主线程并发研究。

### 本轮研究方式

- 主线程重点分析：建连后端侧最小必要能力边界。
- 子 agent 并行方向：interruption 分层策略、外部资料梳理。
- 本轮仍以研究讨论为主，不收敛成实施方案。

### 本轮核心结论

- 建连后若由服务侧主导 turn-taking，端侧最合理的角色不是“继续主裁”，也不是“完全退出”，而是：
  - 音频前端层
  - 反射层
  - 事实回传层
  - 兜底层
- 其中最容易被低估、但很关键的一项是：`playback progress / played-duration / buffer-clear` 这类端侧播放事实回传。若没有这层，服务侧很难真正知道用户听到了多少，也难把 interruption 与 heard-text 做准。
- speaking 期间检测到新语音，不应直接等价为 hard interrupt；更合理的四层策略是：
  - `ignore`
  - `backchannel`
  - `duck_only`
  - `hard_interrupt`
- 更像人的主线不是“第一个 speech_start 立刻闭嘴”，而是“第一个 speech_start 进入 preview/arbitration，随后由更高置信度的部分文本、上下文、输出状态来决定是否真正打断”。
- 子 agent 并行研究补充强调：`duck_only` 是最值得重视的一层，因为它正好位于“完全不停”和“过度敏感硬停”之间；对于当前项目，它很可能是改善自然感的最高杠杆点之一。

### 当前最值得继续咬住的讨论点

- 端侧反射层里，哪些能力是必须保留，哪些只是锦上添花？
- `duck_only` 应持续多久、在什么条件下升级或回退？
- 若未来真的走 server-primary hybrid，哪些状态需要端侧显式回报给服务侧，才能支撑 heard-text 与 interruption 一致性？

### 本轮专题沉淀

- `docs/architecture/server-primary-hybrid-min-device-capabilities-and-interruption-zh-2026-04-16.md`


## Round 006｜2026-04-16｜并行深挖 duck_only、流式+整段识别协同、领域 ASR 提升

### 用户要求

- 子 agent 1：深挖 `duck_only` 的时间窗和升级条件。
- 主线程：深度研究如何充分利用流式 + 整段识别，在语义完备时尽早开始处理，并在后续 VAD / 新补充语音到来时确认或纠正。
- 子 agent 2：深度研究如何提升 ASR 在智能家居 / 桌面助理等特定领域的识别效果。

### 本轮核心结论

- `duck_only` 最重要的意义，是作为“可逆的犹豫层”。它不应无限停留，而应在有限时间窗内收敛为：
  - `hard_interrupt`
  - `backchannel`
  - `ignore`
- 更适合当前项目的 `duck_only` 讨论框架，不是单一阈值，而是分层时间窗：
  - `initial reflex window`
  - `evidence accumulation window`
  - `escalation window`
  - `release window`
  - `false interruption recovery window`
- 流式 + final-ASR 的协同方向是成立的，而且很值得继续沿着它做：
  - 可以在 streaming partial + stable prefix + 语义完备度足够高时，提前做可逆动作
  - 例如 prewarm、draft response、tool planning candidate
  - 但不可逆提交应等待更强确认
- 对当前项目，更值得坚持的表达是：
  - `前通道尽早理解`
  - `后通道负责纠正`
  - `真正不可逆的提交再晚一步`
- 领域 ASR 提升方面，对智能家居 / 桌面助理这类短命令、高实体密度场景，最有 ROI 的顺序通常不是先做重微调，而是：
  - 动态 contextual biasing / 热词
  - alias / 拼音 / 发音 / 缩写层
  - final-pass 强修正 / 复排 / 实体纠错
  - 再往数据构造与 PEFT 走
- 对当前项目最值得借鉴的领域增强边界是：
  - `preview` 路径：保守、快、轻 bias
  - `final` 路径：更强 contextualization、LM rescoring、entity correction

### 当前最值得继续深挖的后续点

- `duck_only` 动态时间窗里，哪些信号应最先进入判定器，哪些只做后验确认？
- `stable prefix + utterance completeness + slot completeness` 是否可以形成一个统一的“早处理门槛”？
- 如果面向智能家居 / 桌面助理建立动态 bias list / alias / entity catalog，最小可行集合应该怎么设计？

### 本轮专题沉淀

- `docs/architecture/duck-only-timing-and-escalation-zh-2026-04-16.md`
- `docs/architecture/streaming-final-asr-semantic-early-processing-zh-2026-04-16.md`
- `docs/architecture/domain-asr-enhancement-for-assistant-zh-2026-04-16.md`


## Round 007｜2026-04-16｜深挖统一“早处理门槛”

### 用户问题

- 深度研究：`stable prefix + utterance completeness + slot completeness` 能不能形成一个统一的“早处理门槛”？
- 用户多次重复强调这一问题，说明当前讨论主焦点已收敛到“如何定义一个足够现代、但不粗糙的 early-processing gate”。

### 本轮核心结论

- 这三者可以形成一个统一的门槛，但不应理解为“一个硬阈值”或“一个总分数”。
- 更合理的表达是：
  - 用一个统一的 `Early Processing Gate / UEPG` 对象承载三类信息
  - 再对不同动作类型设置分层门槛
- 三者分别回答的是不同问题：
  - `stable prefix`：文本是否稳定到值得拿来用
  - `utterance completeness`：语义上是否已经足够成句、足够像说完
  - `slot completeness`：对命令/工具调用来说，参数是否真的齐了
- 因为三者经常失配，所以不适合压成单一线性分数：
  - 高 `stable prefix` 不代表用户已经说完
  - 高 `utterance completeness` 不代表参数已经齐全
  - 对问答类场景，`slot completeness` 甚至可能不是核心约束
- 当前更适合坚持的方向是：
  - `前通道尽早理解`
  - `后通道负责纠正`
  - `真正不可逆的提交再晚一步`

### 对当前项目的临时表达

- 更适合当前 `agent-server` 的不是“统一单分值”，而是“统一门槛对象 + 分层动作决策”：
  - `preview-ready`
  - `draft-ready`
  - `commit-ready`
- 最适合先让这套门槛驱动的，是可逆动作：
  - preview visible
  - prewarm
  - draft response
  - speculative tool planning
  - early TTS first clause candidate
- 不建议一上来用这套门槛直接驱动高风险不可逆动作。

### 本轮专题沉淀

- `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`


## Round 008｜2026-04-16｜并行深挖 duck_only 动态打分与 dynamic bias/entity catalog

### 用户问题

- 并行深入研究两个方向：
  1. `duck_only` 的动态打分函数怎么设计
  2. 智能家居 / 桌面助理的 `dynamic bias list + alias + entity catalog` 最小可行结构怎么设计

### 本轮研究方式

- 主线程重点分析：`duck_only` 动态打分函数。
- 子 agent 方向原计划并行补充 catalog 结构，但在当前会话中子 agent 返回不稳定，因此本轮产出以主线程整合公开资料与现有研究结论为主，同时保持“1 个子 agent 并行”的尝试过程。
- 本轮仍以研究讨论为主，不收敛成实施方案。

### 本轮核心结论

- `duck_only` 更适合由“两段动态打分”驱动，而不是单一总分或单一阈值：
  - `intrusion_prior`：决定是否进入 `duck_only`
  - `takeover_confirmation`：决定最终收敛为 `hard_interrupt / backchannel / ignore`
- 更适合作为先验快速分的信号包括：
  - 近端声学入侵强度
  - overlap 冲突度
  - 本地 reflex hint
  - false-trigger penalty
- 更适合作为后验确认分的信号包括：
  - stable prefix / prefix stability
  - takeover/correction lexicon
  - directedness
  - output conflict / output phase
  - backchannel likelihood / ignore likelihood
- 对智能家居 / 桌面助理的 entity catalog，最小可行结构不应从“大词表”开始，而应从：
  - 轻量 catalog 主键层
  - 受控 alias 层
  - 动态 top-K bias 逻辑
  开始。
- 更适合当前项目的边界是：
  - `preview` 前通道：小规模、强相关、低歧义 bias
  - `final` 后通道：完整 alias 图谱、entity correction、normalization

### 当前最值得继续深挖的后续点

- `intrusion_prior` 与 `takeover_confirmation` 的最小输入字段是否还能进一步压缩？
- `dynamic top-K` 的排序逻辑如何与 `slot completeness` 直接打通？
- 对中文智能家居命令，哪些 alias / common misrecognitions 最值得优先建第一版表？

### 本轮专题沉淀

- `docs/architecture/duck-only-dynamic-scoring-zh-2026-04-16.md`
- `docs/architecture/dynamic-bias-alias-entity-catalog-mvp-zh-2026-04-16.md`


## Round 009｜2026-04-16｜继续收束 slot completeness 的可计算表达

### 用户要求

- 在 `duck_only` 动态打分与 dynamic bias/entity catalog 结构之后继续推进。

### 本轮核心结论

- `slot completeness` 更合理的表达不是“有没有抽到文本”，而是：
  - `Fill × Normalize × Disambiguate × Stable`
- 对命令/工具调用而言，单槽位 completeness 更适合作为一个可分解对象，而不是简单布尔值。
- 对当前项目，更值得先把 `slot completeness` 用于“限制过早提交”，而不是一上来用它激进推动早执行。
- 在智能家居 / 桌面助理场景，前通道最大的价值之一，不只是“先猜对”，而是“先知道哪里还没说完、还不能执行”。

### 与已有主线的关系

- `slot completeness` 应与：
  - `UEPG`
  - `dynamic bias list`
  - `alias`
  - `entity catalog`
  一起看，而不是单独看 ASR 置信度。
- 对实体槽来说，更合适的路径是：
  - `streaming ASR -> alias match -> entity candidate -> canonical resolve -> completeness`

### 本轮专题沉淀

- `docs/architecture/slot-completeness-computable-object-zh-2026-04-16.md`


## Round 010｜2026-04-16｜盘点除已讨论内容外的下一批关键分析点

### 用户问题

- 除了当前已讨论的内容，这个项目还需要分析哪些关键点？

### 本轮核心结论

- 在已经讨论了 turn-taking、duck_only、preview/final-ASR、UEPG、slot completeness、dynamic bias/entity catalog 之后，下一批真正决定体验上限的关键分析点，主要还有：
  - 端到端时延预算与主观体感映射
  - 播放事实回传与 `heard-text` 真相链
  - TTS 起播、断句、韵律与人味儿
  - 真实声学环境下的全双工鲁棒性
  - 歧义澄清、低置信处理与错误恢复
  - 高风险动作的分级提交与确认策略
  - 面向真实使用场景的评测体系与数据闭环
  - GPU 资源调度与多模块并行竞争下的实时性稳定性
- 如果只按当前阶段 ROI 排序，更值得优先继续分析的是：
  - 时延预算
  - 播放事实回传
  - TTS 韵律/断句
  - 评测体系
  - 风险分级提交

### 本轮专题沉淀

- `docs/architecture/remaining-critical-analysis-topics-zh-2026-04-16.md`


## Round 011｜2026-04-16｜并行深挖时延预算与播放事实真相链

### 用户要求

- 在上一轮列出的下一批关键分析点里，优先并行深挖：
  1. 端到端时延预算与主观体感映射
  2. 播放事实回传与 `heard-text` 真相链
- 继续保持研究优先，不急于收敛成实现方案。

### 本轮研究方式

- 主线程重点分析：时延预算、体验里程碑与主观体感之间的映射。
- 并行检索与综合：播放事实回传、truncation、heard-text 对 interruption/memory 的影响。
- 继续结合当前仓库已落地边界与外部官方/论文资料做交叉判断。

### 本轮核心结论

- 对实时语音体验而言，最重要的不是单一 `end_to_end_ms`，而是一张**里程碑时延预算表**。
- 当前最关键的体感节点包括：
  - `speech_start visible`
  - `first preview partial`
  - `endpoint accept`
  - `response.start / first draft`
  - `first audible syllable`
  - `barge-in cutoff`
- `heard-text` 不是一个直接观测量，而是一条真相链的推断结果：
  - `generated -> delivered -> playback_started -> playback_progress/mark -> interrupted/cleared/completed -> heard_text_estimate`
- 当前项目的架构边界方向已经基本正确：
  - playback / preview / heard-text 继续由共享 `voice runtime` 解释和持久化
  - adapter 更适合只上报 transport/playout facts
- 对当前研究阶段，最值得优先补强的不是“完美毫秒级可听性真相”，而是更稳定的 `Tier 1` 播放事实：
  - `playback_started`
  - `segment/mark played`
  - `playback_cleared`
  - `playback_completed`
- 就时延优化的 ROI 来看，当前比“完整答完更快”更重要的，是：
  - 更早给 listening cue
  - 更早下发 preview partial
  - 更早形成 endpoint accept
  - 更早起播首句

### 与当前项目主线的关系

- 这两项研究都在强化同一个核心判断：
  - 当前项目应该继续围绕 **服务侧主导的 turn orchestration + 更强 preview 链路 + 更可信的 playback truth chain** 前进。
- 如果没有时延预算表，后续优化容易陷入“各模块都在加速，但用户体感没明显改善”。
- 如果没有 playback truth chain，后续的 interruption / resume / memory 即使功能上能跑，也会长期不够自然。

### 本轮专题沉淀

- `docs/architecture/latency-budget-and-subjective-feel-zh-2026-04-16.md`
- `docs/architecture/playback-facts-and-heard-text-truth-chain-zh-2026-04-16.md`


## Round 012｜2026-04-16｜收束为正式语音架构方案

### 用户诉求

- 基于最近这一系列研究与当前项目现状，完整分析并整理已有讨论。
- 在深度研究基础上，形成一份尽可能先进、自然、智能、可落地、可持续演进的完整语音架构方案文档。
- 方案文档需要专业、完整、详细、清晰。

### 本轮工作方式

- 回读当前仓库的架构、计划、gap review、近期专题研究与项目记忆。
- 对外部一手资料继续做补充核验，重点对齐：OpenAI Realtime、Google endpointing/prefetching、Amazon endpointing/predictive ASR、LiveKit、Deepgram、AssemblyAI、Twilio、FunASR。
- 不急于变更实现，而是先把“长期正确的语音架构骨架”正式收束为主线文档与 ADR。

### 本轮核心结论

- 当前项目最适合的长期方向已明确为：
  - `server-primary hybrid`
  - `session-centric`
  - `voice-runtime-orchestrated`
- `internal/voice` 的长期角色被正式提升为共享 `Voice Orchestration Runtime`，围绕四条主循环工作：
  - `Input Preview Loop`
  - `Early Processing Loop`
  - `Output Orchestration Loop`
  - `Playback Truth Loop`
- 统一早处理门槛不应是一个分数，而应是分层对象：
  - `prefix_stability`
  - `utterance_completeness`
  - `slot_completeness`
  - `correction_risk`
  - `action_risk`
- interruption 的长期方向也进一步固定：
  - `ignore / backchannel / duck_only / hard_interrupt`
  - 其中 `duck_only` 是关键中间态，而不是日志标签
- `heard-text` 与 playback truth chain 被正式提升为架构级核心，而不是后续再补的小优化
- 协议演进继续坚持：
  - 兼容当前公开 `client_commit` 基线
  - 新能力以 additive / capability 方式逐步公开

### 本轮正式沉淀

- `docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`
- `docs/adr/0032-voice-architecture-converges-on-server-primary-hybrid.md`

### 与主线的关系

- 这轮输出不是单个实现方案，而是为后续所有语音链路优化提供“总蓝图”。
- 后续不论推进 preview、endpoint、duck_only、早起播、playback truth，还是领域 bias / slot completeness，都应回到这份正式架构方案下统一评估。


## Round 013｜2026-04-16｜从蓝图拆出实施路线图与 client 协作协议方案

### 用户诉求

- 并行完成两件事：
  1. 基于正式蓝图，拆出“可执行实施路线图 + 文件级任务清单 + 验收指标”
  2. 从 blueprint 中抽一版“协议与事件方案”，要求明确、具体，便于嵌入式同事同步开发 client

### 本轮工作方式

- 先回读蓝图与当前协议文档，明确哪些属于现有兼容基线，哪些适合做 capability-gated 增量方案。
- 尝试用子 agent 并行推进两条产出；由于子 agent 在当前工作区上下文下未稳定完成最终文档落盘，主线程接管并完成最终沉淀。
- 最终形成一份执行路线图文档与一份 client 协作协议提案文档，并补充主索引、计划与长期记忆。

### 本轮核心结论

- 从蓝图到落地，最合理的实施顺序已被进一步固定为：
  - preview-first 主路径
  - 更早起播与 output orchestration
  - playback truth / heard-text / resume
  - 领域 bias / alias / slot completeness / risk gating
  - 协议公开化与 client 协同毕业
- 面向嵌入式 client 的协议方案也明确收束为：
  - 保留当前兼容基线
  - 通过 discovery + capability 协商逐步开放 `preview-aware` 与 `playback-truth-aware` 两类能力
- 对客户端最重要的新增协作方向包括：
  - `input.speech.start`
  - `input.preview`
  - `input.endpoint`
  - `audio.out.meta`
  - `audio.out.started`
  - `audio.out.mark`
  - `audio.out.cleared`
  - `audio.out.completed`
- 这套协议提案明确强调：
  - client 负责 playback 事实与本地执行
  - server 负责 turn/interruption/truth 的解释与裁决
  - 不让 adapter/client 变成第二编排层

### 本轮正式沉淀

- `docs/architecture/voice-architecture-execution-roadmap-zh-2026-04-16.md`
- `docs/protocols/realtime-voice-client-collaboration-proposal-v0-zh-2026-04-16.md`

### 与主线的关系

- 这轮把“正式蓝图”进一步拆成了两份真正可执行、可协作文档：
  - 一份指导服务端与 worker 的工程推进顺序
  - 一份指导嵌入式与 reference client 的并行协议开发


## Round 014｜2026-04-16｜把 client 协作协议提案下沉到 schema/时序图/状态机

### 用户诉求

- 在上一轮的 client 协作协议提案基础上，继续下沉一层。
- 补充：
  - schema 草案
  - 时序图
  - RTOS client 状态机
- 目标是让嵌入式同事可以直接按文档实现。

### 本轮工作方式

- 回读已有协作协议提案与当前稳定 v0 协议文档，保持兼容边界清晰。
- 不直接升级稳定公共合同，而是补一份 draft schema 与实现导向的文档细化。
- 明确 accepted-turn、preview、playback fact 三者的边界，避免 client 被设计成第二编排层。

### 本轮核心结论

- 嵌入式实现最需要的不是更多抽象，而是三样具体材料：
  - payload shape 草案
  - 时序图
  - client 本地状态机
- accepted-turn 的实现规则进一步固定为：
  - `accept_reason` 是主信号
  - `input.preview / input.endpoint` 只表示观察与候选，不表示 accepted turn
- playback 协作的实现规则也进一步固定为：
  - `audio.out.started / mark / cleared / completed` 是事实回传
  - 不承载 `duck_only / hard_interrupt` 等策略语义
- 通过新增 `schemas/realtime/voice-collaboration-v0-draft.schema.json`，当前 proposal 已经足够让嵌入式同事并行编码，而不需要等待稳定 schema 正式毕业。

### 本轮正式沉淀

- `docs/protocols/realtime-voice-client-collaboration-proposal-v0-zh-2026-04-16.md`（增强）
- `schemas/realtime/voice-collaboration-v0-draft.schema.json`（新增）

### 与主线的关系

- 这一轮把“协议提案”继续往“实现材料”推进了一步。
- 现在嵌入式同事已经有：
  - 事件定义
  - payload 草案
  - 时序图
  - client 状态机
  可以直接开始并行开发。


## Round 015｜2026-04-16｜把协作协议继续下沉到 embedded 实施手册，并开始落服务端

### 用户诉求

- 把 draft schema 继续下沉为：
  - embedded client 字段表
  - 错误码 / 重试策略
  - ACK 时机表
- 同时，不只停留在提案层，而是直接开始按这份协议方案修改服务端。

### 本轮工作方式

- 以 additive、capability-gated 为前提，不推翻当前 v0 基线。
- 先选最小、安全、能开始联调的第一实现切片：
  - discovery 暴露 `voice_collaboration`
  - `session.start.capabilities` 支持协商 `preview_events` 与 `playback_ack.mode`
  - native realtime 路径先公开 preview-aware 事件
  - playback ACK 先接入为“事实回传入口 + 可观测日志”
- 同步把稳定 schema、RTOS 文档、session 文档与 embedded 实施材料一起补齐，避免协议只存在于聊天里。

### 本轮核心结论

- embedded 侧现在真正需要的不只是 proposal，而是一份能直接照着写固件的 implementation guide，因此新增了更具体的字段表、错误码、重试与 ACK 时机文档。
- 协议落地的第一实现切片不需要一步到位做成完整 playback-truth runtime，只要先把下面这几件事做实，就已经能开始并行联调：
  - discovery 暴露 `voice_collaboration.preview_events / playback_ack`
  - `session.start` 可以显式声明 `preview_events=true` 与 `playback_ack.mode=segment_mark_v1`
  - native realtime 在协商成功后发出：
    - `input.speech.start`
    - `input.preview`
    - `input.endpoint`
    - `audio.out.meta`
  - native realtime 接收：
    - `audio.out.started`
    - `audio.out.mark`
    - `audio.out.cleared`
    - `audio.out.completed`
- 当前对 playback ACK 的策略被刻意收敛为“先接入口，再深化语义”：
  - 先把 client 播放事实接进来并记录
  - 下一阶段再让 heard-text / resume / resume-from-anchor 更强地依赖这些事实，而不是立即把现有播放完成路径全部重构掉

### 本轮正式沉淀

- `docs/protocols/realtime-voice-client-implementation-guide-v0-zh-2026-04-16.md`（新增）
- `docs/protocols/realtime-session-v0.md`（增强）
- `docs/protocols/rtos-device-ws-v0.md`（增强）
- `schemas/realtime/session-envelope.schema.json`（增强）
- `schemas/realtime/device-session-start.schema.json`（增强）

### 与主线的关系

- 这一轮把“client 协作协议”从提案真正推进到“可以联调”的阶段：
  - 嵌入式同事拿到 implementation guide 就能直接开始写 client
  - 服务端也已经开始把 preview-aware / playback-truth-aware 的第一批钩子公开出来
- 这让后续主线能够更自然地进入：
  - 端到端联调
  - 更精准的 playback truth
  - 更自然的 interruption / heard-text / resume
