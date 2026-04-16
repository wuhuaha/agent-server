# 当前项目的分层 LLM + FunASR 增强策略研究（2026-04-17）

## 文档定位

- 性质：深度研究结论 + 当前项目的推荐架构方案
- 目标：回答四个问题
  1. 当前服务侧是否应该使用多个不同尺寸的 LLM 处理不同阶段任务
  2. 如何在现有 `semantic judge` 基础上补 `slot completeness`
  3. FunASR 是否应该引入标点、情绪、音频事件等增强模型，以及分别用于哪个阶段
  4. 截至 2026-04-17，Qwen / GLM / DeepSeek / 面壁 MiniCPM / 小米 MiMo / Google Gemini 与 Gemma 等模型里，哪些更适合本项目不同阶段
- 边界：
  - 不改变当前“服务侧主导 turn-taking / interruption”的主方向
  - 不把 `internal/voice` 退化成 provider glue
  - 不把 LLM 直接提升为 realtime accept 的唯一裁判

## 一句话总论

当前项目最合适的方向不是“找一个最强 LLM 干完所有事”，而是建立一条**分层、分时延、分权限**的语音智能链路：

- `Tier 0`：声学/VAD/turn heuristic 负责毫秒级实时安全底座
- `Tier 1`：小模型 LLM 负责语义裁判，补“是否说完、是否接管、是否只是附和”
- `Tier 2`：中小模型 LLM 负责 domain/intent/slot completeness，补“能不能开始 planning / clarify / act candidate”
- `Tier 3`：更强的大模型负责真正的指令理解、工具调用、回复生成与对话风格

与此同时，FunASR 不应只被当作“出一段文本”的 ASR：

- `final_punc_model` 应优先服务 final text、planner 与主 LLM 理解稳定性
- `SenseVoice` 的 `emotion` / `audio_events` 应优先作为运行时元数据输入到 interruption、reply style、TTS style 和 debug
- 这些增强能力都应保持 **runtime-owned metadata**，而不是直接绑死在 wire protocol 或 adapter 侧策略里

## 1. 当前项目事实复核

先确认当前仓库已经具备什么，以及真正还缺什么。

### 1.1 已经具备的能力

- 服务侧 turn-taking、preview、interruption 的主链路已经收敛到 `internal/voice`
- 当前仓库已经接入第一版 `LLM semantic judge`，可对成熟 preview candidate 做保守语义裁决
- 当前 `semantic judge` 已能输出：
  - `utterance_status`
  - `interruption_intent`
  - `confidence`
  - `reason`
- FunASR worker 已支持：
  - `online_model`
  - `final_vad_model`
  - `final_punc_model`
  - `kws_enabled`
- 当前语音结果协议内已可承载归一化后的：
  - `Emotion`
  - `AudioEvents`
  - `EndpointReason`
  - `Partials`

### 1.2 当前最核心的缺口

#### A. semantic judge 仍然过于“窄”

当前第一版 semantic judge 更接近“complete/correction + backchannel/takeover”二分类增强器，还没有真正进入：

- `domain`
- `intent`
- `slot completeness`
- `clarify_needed`
- `actionability`

也就是说，它现在更像一个 **realtime turn referee**，还不是 **spoken semantic parser**。

#### B. 语义裁判仍复用主 Agent LLM 配置

当前代码里 `buildVoiceSemanticJudge(...)` 仍直接复用主 Agent LLM 配置，这意味着：

- 实时控制链与主回复链还没有真正模型解耦
- 无法为 semantic judge 选择更小、更快、更便宜的模型
- 也不利于后续引入单独的 `slot parser` / `clarify parser`

#### C. FunASR 增强能力还没有真正进入“体验编排”

仓库已经具备 `final_punc_model`、`emotion`、`audio_events` 等能力，但它们还没有系统性进入：

- preview / final text 的分层理解
- planner clause segmentation
- interruption scoring
- TTS 风格控制
- 会话级 debug 与体验诊断

## 2. 为什么必须是“多尺寸 LLM 分工”，而不是单模型通吃

## 2.1 外部实践给出的共同结论

### OpenAI：turn detection 已区分 `server_vad` 与 `semantic_vad`

OpenAI Realtime 官方文档已把 turn detection 拆成：

- `server_vad`
- `semantic_vad`

而且 `semantic_vad` 还提供 `eagerness` 这一类“语义收尾倾向”控制项。这个事实非常关键：

- 是否静音，不等于是否该开始处理
- 是否该开始处理，也不等于是否可以立刻 accept

对本项目的含义是：**语义裁判天然应该是一层单独能力，而不是把它混进主回复模型里顺便做。**

### Google / Amazon：endpointing 的上限来自上下文建模，而不是只调静音阈值

Google 的 endpointing 研究强调 ASR 与 endpointing 联合建模可以同时降低延迟与错误截断；Amazon 则明确指出 endpointing 需要上下文、自适应与在线反馈，而不是固定阈值或离线 grid search。

对本项目的含义是：

- 服务侧仍应保留 acoustic floor
- 但必须在其上叠加 semantic/contextual 层
- 最合理的工程形式，就是小模型或轻量结构化模型参与 runtime arbitration

### Apple：LLM 很适合做受约束的语义增强，但不应粗暴吞掉整个语音栈

Apple 的公开研究一方面表明：

- LLM 可以用于 ASR contextualization、device-directed speech detection、多信号融合

另一方面又说明：

- 真正低时延、可落地的语音 agent 仍大量采用 cascade，只是把 cascade 做得更流式、更并行、更智能

`ChipChat` 对本项目尤其有启发：**并不是 end-to-end 才先进，经过重新设计的 cascade 同样可以做到低时延、自然交互。**

## 2.2 单一大模型通吃的主要问题

### A. 时延与抖动不可控

如果让 14B/32B 模型同时负责：

- partial 语义判断
- slot completeness
- clarify 判断
- 主回复

那么实时链路会出现两个问题：

- 高频 preview 请求会把大模型拖成拥塞点
- 主回复与实时裁判争夺同一推理资源，导致说话时延明显抖动

### B. 权限边界模糊

如果同一个模型既负责“该不该 interrupt”又负责“回复什么”，那会让控制策略、对话策略、工具策略混在一起，后续几乎无法做：

- 独立评测
- 灰度升级
- 局部替换
- SLA 分层

### C. 对中文家居/桌面短指令场景并不经济

本项目当前重点是：

- 快速、自然的实时语音交互
- 智能家居 / 桌面助理式短指令与短对话

这种场景里，很多请求其实不需要 32B 级别模型才能完成“是否说完 / 是否缺房间名 / 是否只是附和”的判断。把所有问题都扔给大模型，性价比很低。

## 2.3 推荐的三层 LLM 分工

### `Tier 1`：语义裁判小模型（0.5B ~ 1.5B，最多到 4B）

负责：

- utterance completeness
- backchannel / duck_only / takeover / correction 粗分类
- 早处理门槛里的“语义是否闭合”部分

要求：

- 高 JSON 服从性
- 低首 token 延迟
- 更偏分类，不偏长推理
- 允许较低温度、短输出、严格 schema

它不负责：

- 工具调用
- 多轮复杂规划
- 主回复生成
- 长链 reasoning

### `Tier 2`：slot / domain parser 中模型（3B ~ 8B，必要时 14B）

负责：

- domain
- intent
- required slots
- filled slots
- normalized value
- ambiguity / missing slots
- `clarify_needed` / `draft_ok` / `act_candidate`

要求：

- 对中文口语、家居实体、桌面实体理解稳定
- 结构化抽取和归一化能力强
- 可以 150~350ms 内返回短 JSON
- 最好可做 few-shot domain prompt

它不负责：

- 最终对话回复全文
- 大规模世界知识问答
- 高成本长链推理

### `Tier 3`：主对话 / 工具规划大模型（14B ~ 32B，本地更大或云端更强）

负责：

- 最终用户意图理解
- tool planning / calling
- 回复生成
- 长 context 利用
- 更自然的表达、追问、解释、人格化

要求：

- 中文理解和表达强
- 工具调用与 JSON 服从性稳定
- 对家居/桌面任务可做 prompt grounding
- 在本地 GPU 条件下能维持可接受的首包延迟

## 3. 本项目建议采用的职责分层

## 3.1 不是三段串行，而是“三段异步并行”

正确姿势不是：

`ASR -> 小模型 -> 中模型 -> 大模型 -> TTS`

而是：

1. `Tier 0` 声学底座持续工作
2. `Tier 1` 只在成熟 preview 上触发，且是短 JSON
3. `Tier 2` 在 `Tier 1` 给出“可能已接近完整”后异步补位
4. `Tier 3` 在 `draft_allowed` 以后预热，在 `accept_candidate/accept_now` 后真正接棒

这样才能做到：

- preview 不被主 LLM 阻塞
- slot completeness 不必等 final ASR 才开始
- 主 LLM 有更高概率在 accept 时已经拿到足够好的结构化上下文

## 3.2 当前项目的推荐时延预算

以下是更适合当前 demo 阶段的预算目标：

- `Tier 0` 声学 / heuristic：持续流式，毫秒级
- `Tier 1` semantic judge：单次目标 `<= 120~220ms`
- `Tier 2` slot parser：单次目标 `<= 180~350ms`
- `Tier 3` 主 LLM 首个可播意群：目标 `<= 400~900ms`（强依赖模型与 TTS）

这里最关键的判断是：

- `Tier 1` 必须比主 LLM 明显更快
- `Tier 2` 可以比 `Tier 1` 慢，但必须足够短，不能拖慢 accept 后首轮 planning

## 4. 如何在 semantic judge 上继续补 `slot completeness`

## 4.1 结论先行

可以补，但**不要把 slot completeness 塞进当前 micro semantic judge 一次性做完**。

更合理的方式是：

- `semantic judge` 继续负责“turn-oriented”问题
- 新增一个更偏 `spoken semantic parser` 的结构化层，负责 `domain + intent + slots`
- 两者都属于 `internal/voice` 或 voice-runtime 辅助层消费的元信息，而不是 gateway 自己调用模型

## 4.2 为什么不能直接让第一层 semantic judge 同时做 slot parsing

因为这两类任务的优化方向不同：

### 语义裁判更像“快速保守二分类/多分类”

它关心：

- 现在是不是一句完整的话
- 用户是在附和还是在接管
- 当前是否处于 correction pending

### slot completeness 更像“结构化语义解析”

它关心：

- 这是哪个 domain / intent
- 哪些槽位是必填
- 当前哪些已经填上
- 这些值是否已被归一化、消歧、稳定
- 如果缺槽，缺的是哪一类槽

把两者塞在一个小 prompt 里，常见后果是：

- 输出 schema 过大
- JSON 不稳定
- 小模型 latency 上升明显
- preview 频繁调用的成本过高

## 4.3 建议的 `slot completeness` 结构

建议在当前项目里把它表达为一个**动作可执行性对象**，而不是单纯“抽到没抽到”。

```json
{
  "domain": "smart_home",
  "intent": "device_control",
  "intent_confidence": 0.91,
  "utterance_status": "complete",
  "slot_status": "partial",
  "required_slots": ["action", "target"],
  "filled_slots": {
    "action": {
      "value": "打开",
      "normalized": "turn_on",
      "confidence": 0.98,
      "stable": true
    },
    "target": {
      "value": "客厅灯",
      "normalized": "device:light.living_room.main",
      "confidence": 0.82,
      "stable": true,
      "ambiguous": false
    },
    "attribute": null,
    "value": null,
    "location": {
      "value": "客厅",
      "normalized": "room:living_room",
      "confidence": 0.95,
      "stable": true
    }
  },
  "missing_slots": [],
  "ambiguous_slots": [],
  "actionability": "act_candidate",
  "clarify_needed": false,
  "reason": "required_slots_grounded"
}
```

其中最关键的不是字段多，而是它能为 runtime 提供三个判断：

- `能不能 prewarm`
- `能不能 draft`
- `如果现在 accept，是应该直接执行、先澄清，还是继续等`

## 4.4 面向当前项目的最小域模型

在研究阶段，先不要追求大而全，建议先只做两个 domain：

### A. `smart_home`

最小 intent 集：

- `device_control`
- `scene_control`
- `query_status`
- `set_attribute`

最小槽位集：

- `action`
- `target`
- `location`
- `attribute`
- `value`
- `mode`
- `duration`

### B. `desktop_assistant`

最小 intent 集：

- `open_app`
- `search_web`
- `create_note`
- `window_control`
- `system_control`

最小槽位集：

- `action`
- `target_app`
- `query`
- `window_name`
- `system_setting`
- `value`

## 4.5 与 early processing 的关系

补了 slot completeness 以后，推荐把 runtime 内部阶段进一步解释为：

### `prewarm_allowed`

满足任一：

- semantic judge 认为 utterance 近似闭合
- 或 domain/intent 已较稳定
- 即使槽位不全，也值得预热 prompt / memory / tool catalog

### `draft_allowed`

需要：

- utterance_status 至少不是明显 correction
- domain + intent 基本稳定
- 必填槽位已覆盖主要部分，或者缺的只是低风险可追问槽位

### `accept_candidate`

需要：

- utterance 基本完整
- 必填槽位足够完整且相对稳定
- ambiguity 可接受，或者可在 accept 后立即 clarify
- 同时仍要满足 acoustic/endpoint floor

### `clarify_needed`

这是当前项目最值得补的一层。典型样例：

- “把灯调亮一点”
  - action 有
  - target 缺
  - utterance 完整
  - 不应一直等更多语音，也不应直接执行
  - 正确行为应是 accept 后立即澄清

换句话说，slot completeness 的价值不只是“更准执行”，还包括**更自然地决定什么时候该追问，而不是继续傻等静音或误执行。**

## 4.6 当前最推荐的实现边界

建议后续实现时这样分层：

- `SemanticTurnJudge`
  - 继续做 utterance/interruption 裁判
- `SemanticSlotParser`
  - 新增接口，返回 domain/intent/slot/actionability
- `TurnArbitration`
  - 只引用 parser 的摘要字段，不直接背完整 domain schema
- `internal/agent`
  - 消费 runtime 已归纳好的结构化上下文，不自行反向决定 realtime accept

## 5. FunASR 是否需要引入标点、情绪分类、音频事件等增强

## 5.1 总结论

需要，但要分清阶段与权限：

- 标点：**建议优先接 final path**
- 情绪：**建议优先作为 metadata 使用**
- 音频事件：**建议优先进入 runtime arbitration/debug，而不是直接触发业务动作**

## 5.2 标点恢复 `final_punc_model`

### 建议：应该引入，优先用于 final-ASR 路径

原因不是“让文本好看”，而是它会直接影响后续几个环节：

1. **主 LLM 理解稳定性**
   - 口语命令文本没有停顿边界时，domain/intent/slot 解析更容易飘
2. **speech planner clause segmentation**
   - 没有标点或边界时，TTS 很难做到更自然的意群起播
3. **memory writeback 与会话回放可读性**
   - 最终落库文本没有边界，会影响 recap、continue、search 和 debug

### 当前项目建议

- preview path：不额外跑重标点模型
- final-ASR path：优先启用 `ct-punc`
- planner：优先消费带标点 final text，但仍允许在 preview 阶段用 pause/prosody hint 做轻量切分

### 不建议的做法

- 不要让标点模型决定 accept
- 不要为每一个 preview chunk 跑独立重标点模型

## 5.3 情绪分类 `emotion`

### 建议：应该接，但优先做 metadata，而不是 accept gate

当前项目已经通过 `SenseVoice` 具备情绪输出能力。这是非常有价值的，因为语音 agent 的“生动、自然、人性化”并不只取决于文本内容，还取决于：

- 用户是在平静发问、急促催促，还是明显困惑
- 用户插话是礼貌附和，还是明显不耐烦地打断
- 系统是否应切换更短、更安抚、更确认式的回复风格

### 推荐使用阶段

#### A. interruption / duck_only

情绪不能单独决定是否 hard interrupt，但可以参与打分，例如：

- 急促、提高音强、情绪明显的短语，可提高 takeover 概率
- 平静、短促、低能量的“嗯/好/行”更偏 backchannel

#### B. reply style / TTS style

- 用户明显焦躁：系统回复更短、更直接
- 用户明显困惑：系统回复更解释型
- 用户明显开心或闲聊：系统可更松弛、更有陪伴感

#### C. debug / eval

情绪元数据是后续做体验评估时非常重要的一条观测维度，可以帮助判断：

- 误打断是否集中发生在高情绪波动场景
- 过慢澄清是否集中发生在疑问句/犹豫句

### 当前项目建议

- 第一优先：直接复用 `SenseVoice` 的情绪输出
- 第二优先：若后续发现颗粒度不够，再评估 `emotion2vec+ large`

不建议现在就单独加一条新的重情绪模型链路，因为 demo 阶段更重要的是先把已有情绪元数据真正消费起来。

## 5.4 音频事件 `audio_events`

### 建议：应该接入 runtime 判断，但默认做辅助证据

`audio_events` 对以下问题非常有用：

- 背景音乐 / 环境声导致的误识别
- 咳嗽、笑声、掌声等非命令语音事件
- speaking-time barge-in 的真假性判定
- 场景理解与回复风格控制

### 推荐使用阶段

#### A. endpoint / accept 抑制

如果当前片段主要是：

- laughter
- coughing
- bgm

那就不应轻易提升为 `accept_candidate`

#### B. interruption policy 辅助

如果 speaking 期间出现的不是稳定 speech，而是明显非语义事件，那么：

- 可降低 hard interrupt 倾向
- 保留 duck_only 或 ignore

#### C. session-level scene awareness

例如：

- 检测到持续 bgm，可对 ASR bias、threshold、clarify 策略做轻微调节
- 检测到频繁 laughter/backchannel，可降低误接管概率

## 5.5 当前阶段不建议优先扩展的 FunASR 能力

在当前研究阶段，不建议把下面几类能力放在最前：

- speaker diarization / `cam++`
  - 很有价值，但不是当前实时流畅性主矛盾
- 额外重情绪模型并行链路
  - 在 `SenseVoice` 情绪元数据还没充分用起来前，ROI 不高
- 把 KWS 改成默认开启
  - 当前项目已明确 KWS 应为可配置选项，默认关闭更稳妥

## 6. 当前主流模型全景评估（截至 2026-04-17）

下面按“是否适合当前项目分阶段使用”来评价，而不是只看榜单。

## 6.1 Qwen 系列

### 已确认的官方现状

- 开源 dense / MoE 主系仍以 `Qwen3` 为核心
- 官方研究页已出现：
  - `Qwen3.6-Plus`（API）
  - `Qwen3.5-Omni`（omni-modal）
- `Qwen3` 官方强调同时支持 thinking / non-thinking 两种模式，并且小模型智能密度明显提升

### 对本项目的适配判断

#### 适合做 `Tier 1` 小模型语义裁判

- `Qwen3-0.6B`
- `Qwen3-1.7B`

优点：

- 中文能力好
- 结构化输出潜力高
- 体积小，适合高频短请求

判断：

- `0.6B` 更适合极低时延保守分类
- `1.7B` 更适合第一版稳定上线的 semantic judge

#### 适合做 `Tier 2` slot/domain parser

- `Qwen3-4B`
- `Qwen3-8B`

优点：

- 对中文短指令、结构化抽取、轻规划都有较好平衡
- 能比 1B 级模型更稳地输出 domain/intent/slot JSON

判断：

- 对当前项目，`4B` 是 slot parser 的第一优选
- 如果 GPU 余量足，`8B` 更像高质量版本

#### 适合做 `Tier 3` 主对话 LLM

- `Qwen3-14B`
- `Qwen3-32B`
- 云端上界：`Qwen3.6-Plus` / `Qwen3-Max` 系列

判断：

- 本地优先时，`Qwen3-14B` 是最均衡起点
- 若多 GPU 或量化条件足够，`Qwen3-32B` 是当前本项目最值得冲的本地高质量主 LLM 之一
- 若允许云端 upper-bound benchmark，则 `Qwen3.6-Plus` 与 `Qwen3-Max` 值得作为质量对照

#### 不建议当前作为主路径的 Qwen 方向

- `Qwen3.5-Omni`
  - 很先进，但当前项目主线仍是 ASR/TTS 分治的服务侧 runtime
  - 现阶段更适合作为 end-to-end baseline，而不是直接替换现有可控栈

## 6.2 GLM / Z.ai 系列

### 已确认的官方现状

- 官方 API 文档当前主打 `GLM-5.1`
- 开源本地侧当前更值得关注的是：
  - `GLM-4-9B-0414`
  - `GLM-Z1-9B-0414`
  - `GLM-4-32B-0414`
- `GLM-4.5/4.7` 能力更强，但模型规模明显更大，更偏高性能 agentic/coding

### 对本项目的适配判断

#### 适合做本地备选 `Tier 2`

- `GLM-4-9B-0414`
- `GLM-Z1-9B-0414`

优点：

- 中文能力强
- 9B 尺寸下仍保留较强结构化理解与 reasoning 能力

判断：

- 如果想找 Qwen 之外的 slot parser /中模型备选，GLM 9B 系值得 benchmark
- 但 9B 对 `Tier 1` 语义裁判来说偏重，不是第一选择

#### 适合做本地 `Tier 3` 高质量备选

- `GLM-4-32B-0414`

优点：

- 本地部署友好度相对较好
- 对中文、agentic、分析型任务较强

判断：

- 它是很强的本地主对话备选
- 若目标是家居/桌面助理中文体验，值得与 `Qwen3-32B` 做横向 benchmark

#### 不建议当前优先选择的 GLM 方向

- `GLM-4.5/4.7` 全尺寸版本
  - 太重，更适合高算力 agent/coding 场景，不是当前语音 demo 的 ROI 最优解
- `GLM-4-Voice`
  - 可以作为 end-to-end 语音基线参考，但会偏离当前共享 runtime 的 ASR/TTS 分层主线

## 6.3 DeepSeek 系列

### 已确认的官方现状

- DeepSeek API 当前主模型入口仍是：
  - `deepseek-chat`
  - `deepseek-reasoner`
- 官方更新页显示其背后已升级到 `DeepSeek-V3.2` 系列
- 开源侧可用的高质量本地选择包括：
  - `DeepSeek-R1-Distill-Qwen-7B`
  - `DeepSeek-R1-Distill-Qwen-14B`
  - `DeepSeek-R1-Distill-Qwen-32B`
- `DeepSeek-V3.2-Exp` / `V3.2` 全尺寸虽然强，但对本地实时语音来说过重

### 对本项目的适配判断

#### 适合做云端 upper-bound / 分析型主 LLM

- `deepseek-chat`
- `deepseek-reasoner`

优点：

- 结构化输出、推理、工具调用都强
- 非常适合作为“高质量上界参考”

局限：

- 对本地实时语音主链来说，网络与服务抖动不利于极致实时性

#### 适合做本地主 LLM 备选

- `DeepSeek-R1-Distill-Qwen-14B`
- `DeepSeek-R1-Distill-Qwen-32B`

判断：

- 更适合主 LLM 或复杂 reasoning specialist
- 不适合 `Tier 1` 语义裁判，因为 reasoning 风格往往更慢、更“想太多”

#### 不建议当前做 `Tier 1`

- `DeepSeek-R1` 或其 distill 小模型

原因：

- semantic judge 需要的是快、稳、短 JSON
- 不是长链推理
- reasoning 型模型常常在这类任务上性能并不等于体验最佳

## 6.4 面壁 MiniCPM 系列

### 已确认的官方现状

- `MiniCPM4` / `MiniCPM4.1` 官方定位非常明确：端侧、高效率、低成本
- `MiniCPM4.1-8B` 官方强调：
  - hybrid reasoning
  - trainable sparse attention
  - 推理速度提升
- ModelBest 官方页仍强调其“端侧 ChatGPT moment”定位

### 对本项目的适配判断

#### 很适合做边缘或低成本 `Tier 2`

- `MiniCPM4-4B`
- `MiniCPM4.1-8B`

优点：

- 对端侧/本地部署极其友好
- 推理效率好
- 做结构化 parser / 中等复杂理解很有潜力

判断：

- 如果本项目后续要把一部分语义层下沉到更弱算力机器，MiniCPM 值得重点看
- 当前服务端 GPU 资源充足时，它不一定超过 Qwen，但在性价比与部署灵活性上很强

#### `Tier 1` 是否适合？

- `MiniCPM4-4B` 可以做 `Tier 1/Tier 2` 合并试验
- 但如果是纯 semantic judge，`Qwen3-1.7B` 这类更小模型通常更经济

## 6.5 小米 MiMo 系列

### 已确认的官方现状

- `MiMo-7B` 官方路线非常强调“born for reasoning”
- `MiMo-7B-RL` 官方声称在 reasoning 上表现很强
- 官方 Hugging Face 组织页已经有更大的 `MiMo-V2-Flash` 系列

### 对本项目的适配判断

#### 适合做 reasoning specialist，不适合当前主线第一优选

优点：

- 数学/代码/推理能力强
- 适合作为专门的深度思考模型或复杂任务评测对象

局限：

- 当前项目更关注实时语音交互、短指令、自然响应
- MiMo 的公开定位更偏 reasoning 强化，而非中文实时口语 agent 的综合平衡

判断：

- 可以做后续 specialist 模型评测
- 不建议当前直接拿它做 `Tier 1 semantic judge`
- 也不建议当前优先拿它替代 Qwen / GLM 做主对话模型

## 6.6 Google Gemini / Gemma

### 一个需要先澄清的事实

截至 2026-04-17，我检索 Google 官方开发者与 DeepMind 模型页面时：

- **没有找到官方公开的 `Gemini 4` 开发模型页面**
- 当前官方文档明确可见的是：
  - `Gemini 2.5 Pro`
  - `Gemini 2.5 Flash`
  - `Gemini 2.5 Flash-Lite`
  - `Gemini 3.1 Flash Live Preview`
  - 开源 `Gemma 4`

因此，如果后续讨论“Gemini4”，在本项目文档里应改写成更准确的官方可见名称，而不是继续使用一个未在官方开发文档中确认的命名。

### Gemini 的启发价值

Google Live API 文档里已经把以下概念公开产品化：

- automatic/custom VAD
- proactive audio
- affective dialog
- interruption events

这对本项目的启发非常强：

- 语音 agent 不是只有 STT/TTS
- “是否该回应”“是否带情感跟随”“是否允许不中断地继续听”都属于运行时编排层能力

### Gemma 的项目价值

`Gemma 4` 官方已经把：

- `E2B / E4B`
- `26B / 31B`

清晰区分为 edge 与 workstation 两类。它很适合做：

- 英文或多语 edge judge
- workstation 主模型

但结合本项目中文场景，当前我更倾向于把它视为：

- 很有价值的对照组
- 不是当前中文主线第一推荐

## 7. 针对本项目的推荐模型组合

## 7.1 当前最推荐的本地主线组合（P0：稳妥高 ROI）

### 语音链路

- preview ASR：`paraformer-zh-streaming`
- preview / final VAD：`fsmn-vad`
- final ASR：`SenseVoiceSmall`
- final punctuation：`ct-punc`
- emotion / audio events：直接复用 `SenseVoice` 输出
- KWS：保持可配置，默认关闭

### LLM 链路

- `Tier 1 semantic judge`：`Qwen3-1.7B`
- `Tier 2 slot/domain parser`：`Qwen3-4B`
- `Tier 3 main dialog`：`Qwen3-14B`

### 为什么这是当前第一推荐

- 与当前仓库边界最兼容
- 中文能力、JSON 服从性、部署可行性较平衡
- `SenseVoiceSmall` 已在项目中验证过，同时还能提供 emotion / audio events
- 不会一下子把系统推到过重、难调试的状态

## 7.2 追求更高文本质量的冲高组合（P1：效果优先）

### 语音链路

- preview ASR：`paraformer-zh-streaming`
- preview / final VAD：`fsmn-vad`
- final ASR：`Fun-ASR-Nano-2512` 与 `SenseVoiceSmall` 做 AB
- final punctuation：`ct-punc`
- 如需情绪元数据：优先保留 `SenseVoice` sidecar benchmark，而不是马上上复杂双 final 链路

### LLM 链路

- `Tier 1 semantic judge`：`Qwen3-1.7B` 或 `Qwen3-4B`
- `Tier 2 slot/domain parser`：`Qwen3-8B` 或 `GLM-4-9B-0414`
- `Tier 3 main dialog`：`Qwen3-32B` 或 `GLM-4-32B-0414`

### 适用场景

- GPU 资源更充足
- 目标是尽量拉高中文理解、澄清、自然回复质量
- 可以接受更复杂的模型部署与调优

## 7.3 云端 upper-bound benchmark 组合（P2：研究对照）

- semantic judge：仍建议本地小模型，不建议云端化
- slot parser / main dialog 可对照：
  - `Qwen3.6-Plus` / `Qwen3-Max`
  - `GLM-5.1`
  - `deepseek-chat` / `deepseek-reasoner`
  - `Gemini 2.5 Pro` / `Gemini 2.5 Flash`

用途：

- 做效果上界对照
- 判断本地主模型是否已经达到“足够好”
- 不建议直接替换掉本地实时裁判链

## 8. 对当前项目的具体架构建议

## 8.1 保持现有主架构不变

继续坚持：

- 服务侧主导 turn-taking / interruption
- `internal/voice` 统一拥有 preview、barge-in、playback truth、semantic judging
- adapter 不直接碰模型

## 8.2 新增两个明确的 runtime-owned 结构化层

### A. `SemanticTurnJudge`

继续存在，输出小而快的 turn/interruption 判断。

### B. `SemanticSlotParser`

新增，专门输出：

- domain
- intent
- slots
- missing / ambiguous
- actionability
- clarify_needed

## 8.3 让 FunASR 元数据真正进入 orchestration

建议后续把以下字段明确接入 runtime：

- `speech.emotion`
- `speech.audio_events`
- `speech.endpoint_reason`
- final text 的标点边界

它们的第一用途不是扩协议，而是：

- 改善 interruption / clarify / response style
- 提高 debug 质量
- 为后续体验优化建立可解释信号

## 8.4 不要过早切回 end-to-end omni 主路径

虽然 `Qwen3.5-Omni`、`GLM-4-Voice`、`Gemini Live` 这类路线很先进，但在当前项目阶段：

- shared runtime 的可控性更重要
- 分治链路更利于 debug 与局部替换
- 现阶段更应该把 cascade 做“更智能、更并行、更自然”，而不是直接换成另一个黑盒

## 9. 最终建议清单

### 最值得优先坚持的 5 个判断

1. **坚持多尺寸、多职责 LLM 分工，不走单模型通吃。**
2. **semantic judge 继续保留 advisory 边界，不直接制造最终 accept。**
3. **slot completeness 应作为独立结构化层补进来，而不是塞爆当前 micro judge。**
4. **FunASR 的标点、情绪、音频事件都值得接，但优先作为 runtime metadata 消费。**
5. **当前本项目最优主线组合，仍然是 `2-pass ASR + tiered LLM + runtime-owned orchestration`。**

## 10. 参考资料（官方与一手资料）

### 语音运行时与 turn-taking

- OpenAI Realtime VAD / `semantic_vad`：<https://platform.openai.com/docs/guides/realtime-vad>
- Google：Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Amazon：Adaptive Endpointing with Deep Contextual Multi-Armed Bandits：<https://www.amazon.science/publications/adaptive-endpointing-with-deep-contextual-multi-armed-bandits>
- Amazon：Natural turn-taking：<https://www.amazon.science/blog/change-to-alexa-wake-word-process-adds-natural-turn-taking>
- Apple：ChipChat: Low-Latency Cascaded Conversational Agent in MLX：<https://machinelearning.apple.com/research/chipchat>
- Apple：A Multi-signal Large Language Model for Device-directed Speech Detection：<https://machinelearning.apple.com/research/llm-device-directed-speech-detection>

### 语音语义理解与 contextualization

- Apple：Contextualization of ASR with LLM Using Phonetic Retrieval-Based Augmentation：<https://machinelearning.apple.com/research/asr-contextualization>
- ACL：Joint Online Spoken Language Understanding and Language Modeling With Recurrent Neural Networks：<https://aclanthology.org/W16-3603/>
- arXiv：OneNet: Joint Domain, Intent, Slot Prediction for Spoken Language Understanding：<https://arxiv.org/abs/1801.05149>
- ACL：Zero-Shot Spoken Language Understanding via Large Language Models: A Preliminary Study：<https://aclanthology.org/2024.lrec-main.1554/>

### FunASR / SenseVoice

- FunASR 官方：<https://github.com/modelscope/FunASR>
- Fun-ASR 官方：<https://github.com/FunAudioLLM/Fun-ASR>
- SenseVoice 官方：<https://github.com/FunAudioLLM/SenseVoice>

### 模型官方来源

- Qwen 官方研究页：<https://qwen.ai/research/>
- Qwen3 官方博客：<https://qwenlm.github.io/blog/qwen3/>
- Z.ai / GLM 官方：<https://docs.z.ai/guides/llm/glm-5>
- GLM-4 开源仓库：<https://github.com/zai-org/GLM-4>
- DeepSeek 官方 API 文档：<https://api-docs.deepseek.com/>
- DeepSeek 模型更新：<https://api-docs.deepseek.com/updates/>
- DeepSeek-R1 Distill 官方模型卡：<https://huggingface.co/deepseek-ai/DeepSeek-R1-Distill-Qwen-7B>
- 面壁 MiniCPM 官方仓库：<https://github.com/OpenBMB/MiniCPM>
- 面壁官方主页：<https://modelbest.cn/en/>
- 小米 MiMo 官方仓库：<https://github.com/XiaomiMiMo/MiMo>
- Google Gemini 模型页：<https://ai.google.dev/gemini-api/docs/models/gemini-v2>
- Google Gemini Live API：<https://ai.google.dev/gemini-api/docs/live-guide>
- Google Gemma 4：<https://deepmind.google/models/gemma/gemma-4/>

## 11. 本文与仓库内既有文档的关系

- 现有 `LLM semantic judge` 研究：`docs/architecture/llm-semantic-turn-taking-and-interruption-zh-2026-04-17.md`
- 现有 `slot completeness` 讨论：`docs/architecture/slot-completeness-computable-object-zh-2026-04-16.md`
- 现有 FunASR 模型研究：`docs/architecture/funasr-model-selection-zh-2026-04-14.md`
- 本文是在以上三者之上，补齐“**分层模型架构 + 当前最新模型选型 + 当前项目最终推荐组合**”。
