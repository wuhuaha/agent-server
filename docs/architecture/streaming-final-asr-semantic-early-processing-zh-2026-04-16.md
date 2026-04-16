# 流式 + 整段识别协同：语义完备即早处理，后续再确认或纠正（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标是回答一个关键问题：
  - 是否可以在流式识别阶段，结合语义理解认为句子已经“足够完备”时，就提前开始处理；
  - 然后在后续 VAD、追加语音、或 final-ASR 到来后，再做确认、修正、或回滚？

## 一句话结论

**可以，而且这基本就是现代实时语音系统正在做的方向。**

但关键不在于“是否提前处理”，而在于要把处理动作分层：

- 可逆动作可以早做
- 不可逆动作必须晚做

更具体地说：

- `preview partial + 语义完备度` 可以提前触发：
  - UI 反馈
  - prewarm
  - 计划生成
  - 工具候选草拟
  - TTS 首句草拟
- 但真正不可逆的动作，例如：
  - 设备控制执行
  - 持久化强结论
  - 高风险工具调用
  仍应等待更强确认，或具备可回滚保护

## 这条思路为什么成立

### 1. Google 公开研究已经证明 partial hypothesis 可以提前驱动下游

Google 的 `Low Latency Speech Recognition using End-to-End Prefetching` 直接讨论了：

- 在 final 识别之前，利用 partial hypothesis 提前触发下游处理
- 系统级延迟可获得约 200ms 收益

来源：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>

### 2. OpenAI / Deepgram / AssemblyAI 都在把“早猜测 + 后确认”做成一等能力

- OpenAI `semantic_vad` 本质上是在“语义上看起来说完了”时更早推进 turn。参考：<https://platform.openai.com/docs/guides/realtime-vad>
- Deepgram Flux 提供 `EagerEndOfTurn` 与 `TurnResumed`，公开承认“先猜测用户可能说完，再在后续恢复或确认”。参考：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- AssemblyAI turn detection 也是 semantic + acoustic 的组合，而不是纯静音。参考：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>

### 3. FunASR 的 2pass 思路非常适合当前项目

当前项目内部已有清晰的 2pass 方向：

- `workers/python/README.md:95` 起：默认 `stream_preview_batch`，可选 `stream_2pass_online_final`
- `docs/architecture/local-funasr-asr.md:18` 起：worker 已支持 `online preview + final-ASR correction`
- `docs/architecture/funasr-model-selection-zh-2026-04-14.md:151` 起：已经明确把 Paraformer online 系列视为实时转写与 2pass 主链路的合适候选

这说明当前项目并不需要从零发明这条路径，而是应该继续把它往“更现代的会话编排”推进。

## 关键不是“提前处理”，而是“提前处理什么”

这是最重要的边界。

### 建议把动作分成四层

#### L0：只做感知反馈

- 点亮“我在听”
- 端侧显示 preview partial
- 输出轨开始 soft duck / previewing

这一层可以最早发生。

#### L1：做可逆预热

- 预热 LLM / tool router
- 预热 TTS session
- 建立候选 intent / slot 草稿
- 生成首句候选，但不正式下发

这一层也可以很早发生，因为它的错误成本很低。

#### L2：做可修正的推理与规划

- 基于 stable prefix 做意图推断
- 提前草拟回复
- 提前规划工具调用参数
- 形成 `turn candidate`

这一层可以在“语义完备度较高”时开始，但仍需允许后续修正或覆盖。

#### L3：做不可逆提交

- 真实设备控制执行
- 向外部系统发送不可撤销调用
- 持久化强结论
- 高风险事务写入

这一层不应仅靠 preview partial 就提交。

## 怎样判断“语义上已足够完备，可以提前处理”

我更推荐从以下四类信号联合判断，而不是单靠 VAD。

### 1. `stable prefix` 是否已形成

不是看“有没有 partial”，而是看：

- stable prefix 长度是否足够
- 尾部是否仍频繁波动
- partial 是否在连续几次更新中保持收敛

### 2. 意图是否闭合

例如：

- `明天天气怎么样`：问句已闭合
- `把客厅灯打开`：命令已闭合
- `把客厅灯调到三十`：可能还未闭合，因为值槽可能缺单位或还有补充
- `帮我打开那个`：明显未闭合，缺实体

### 3. slot 是否完备

对 agent / 助手类场景，很多“句子看似说完”其实只是 slot 没全。

应单独看：

- 动作槽是否稳定
- 实体槽是否稳定
- 数值槽是否稳定
- 时间槽是否稳定
- 是否存在明显需要澄清的空槽

### 4. 是否出现“延迟补充 / 自我纠正”信号

例如：

- 等一下
- 不是
- 我是说
- 还有
- 然后
- 改成

这类信号一旦出现，说明即便前面看似完整，也应降低“已完备”置信度。

## 我更推荐的协同模式：`早理解、晚提交`

### 核心链路

#### 阶段 1：在线流式前通道

职责：

- 快速产出 preview partial
- 尽快识别 stable prefix
- 判断语义完备度
- 尽早预热后续模块

模型更适合：

- 真 streaming online ASR
- 如当前项目讨论中的 `paraformer-zh-streaming`

#### 阶段 2：语义候选生成

基于 preview / stable prefix：

- 形成 `intent candidate`
- 形成 `slot candidate`
- 给出 `utterance completeness score`
- 决定是否开始：
  - prewarm
  - draft response
  - tool planning draft

#### 阶段 3：final-ASR / endpoint / 追加语音确认

职责：

- 用 final-ASR 做 authoritative correction
- 若后续又来一小段补充语音，允许修正：
  - 实体
  - 数值
  - 否定/纠正
  - 句尾补充条件

#### 阶段 4：最终执行 / 正式起播

根据风险等级分层：

- 低风险回复：可以在较早阶段启动 TTS 首句
- 高风险工具调用：应等待更强确认

## 是否可以“语义完备就开始处理，后续再纠正”？

### 可以，但要区分三种后续情况

#### 情况 A：后续只是 silence / 正常句尾

- 直接让 final-ASR 做轻量修正
- 若 candidate 与 final 一致，则顺利进入 commit

#### 情况 B：后续是小补充

例如：

- `把客厅灯打开` -> `...调到三十`
- `明天北京天气` -> `...和上海也一起说`

这时应做：

- 更新 slot candidate
- 取消旧 draft
- 生成新 draft
- 若尚未做不可逆提交，则问题不大

#### 情况 C：后续是纠正或否定

例如：

- `打开卧室灯，不对，客厅灯`
- `提醒我明天八点，不，九点`

这时应：

- 识别 correction cue
- 提升 correction 优先级
- 撤销旧 candidate
- 强覆盖到新 candidate

## 这条路最容易踩的坑

### 1. 把“早处理”误做成“早提交”

如果 preview 刚稳定一点就直接执行工具调用，错误成本会很高。

### 2. 没有稳定前缀机制，导致抖动太大

如果把每次 partial 尾部都直接送进下游，系统会过于神经质。

### 3. 不区分命令型和问答型场景

- 问答型：更适合早生成首句
- 命令型：更适合早形成候选，但晚执行

### 4. 不处理用户自我纠正

如果没有 correction-aware merge，后续一小段补充语音会把整条链路搞乱。

### 5. 将 preview 通道做得过于激进

如果 preview 注入了过重 bias，早处理反而会更早犯错。

## 对当前 `agent-server` 最值得借鉴的边界

### 1. 明确把“早处理动作”分成可逆和不可逆

这是当前项目最该优先固化的讨论边界。

### 2. 继续坚持 2pass，但要从“ASR 两段”升级到“会话编排两段”

不只是：

- online preview
- final correction

还应该变成：

- online preview -> semantic candidate -> prewarm / speculative plan
- final correction -> commit / execute / finalize

### 3. 让 preview partial 成为主链路信号，但只让 stable prefix 深度参与仲裁

也就是说：

- 不要让不稳定尾巴直接主导决策
- 应优先让稳定前缀参与：
  - endpoint
  - duck_only / interruption
  - prewarm
  - early draft

### 4. 对不同风险等级的动作设不同提交门槛

例如：

- 纯文本答复首句起播：门槛可低
- 智能家居控制：门槛更高
- 高风险桌面操作：门槛最高

## 一个适合当前项目的研究阶段表达

如果只用一句最短的话描述我对这条路径的建议，那就是：

- **前通道尽早理解，后通道负责纠正，真正不可逆的提交再晚一步。**

## 参考资料

- FunASR：<https://github.com/modelscope/FunASR>
- 当前项目 FunASR worker 2pass 说明：`workers/python/README.md`
- 当前项目本地 FunASR 架构说明：`docs/architecture/local-funasr-asr.md`
- 当前项目 FunASR 模型选择笔记：`docs/architecture/funasr-model-selection-zh-2026-04-14.md`
- Google Low Latency Speech Recognition using End-to-End Prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Google Unified End-to-End Speech Recognition and Endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- OpenAI Realtime VAD：<https://platform.openai.com/docs/guides/realtime-vad>
- Deepgram Eager End of Turn：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- Deepgram Flux Agent：<https://developers.deepgram.com/docs/flux/agent>
- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>
