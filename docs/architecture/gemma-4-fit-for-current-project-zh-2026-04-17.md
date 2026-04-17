# Gemma 4 小模型对当前项目的适配性研究（2026-04-17）

## 文档定位

- 性质：模型选型专题研究
- 目标：回答两个问题
  1. 2026-04-17 当前官方刚发布的 `Gemma 4` 到底是什么，`2B / 4B` 这些小模型能力如何
  2. 对当前 `agent-server` 项目，尤其是实时中文语音 agent 主线，是否适合采用
- 相关文档：
  - `docs/architecture/voice-multi-llm-and-funasr-strategy-zh-2026-04-17.md`
  - `docs/architecture/llm-assisted-semantic-completeness-and-dynamic-vad-zh-2026-04-17.md`
  - `docs/architecture/project-status-and-voice-flow-review-zh-2026-04-17.md`

---

## 1. 先把事实讲清楚：`Gemma 4` 是什么

根据 Google 官方博客，`Gemma 4` 于 **2026-04-02** 正式发布。Google 对它的定位是“目前最强的开放权重 Gemma 家族”，重点强调：

- advanced reasoning
- agentic workflows
- function calling / structured JSON
- multimodal understanding
- 长上下文
- 更适合本地与边缘部署

Google 官方把 `Gemma 4` 家族分成四个尺寸：

- `E2B`
- `E4B`
- `26B A4B`
- `31B`

其中与本题最相关的是小模型：

- `E2B`
- `E4B`

### 1.1 `E2B / E4B` 不是简单“参数越小越弱”的老式 tiny LLM

Google 官方模型卡里明确写到：

- `E2B`：`2.3B effective (5.1B with embeddings)`
- `E4B`：`4.5B effective (8B with embeddings)`
- 两者都有：
  - `128K` context
  - `Text, Image, Audio` 输入
  - 原生 function calling
  - multilingual 能力

这说明它们的设计目标并不是“极简聊天小模型”，而是：

> 面向移动端 / 边缘端 / 本地 agent 场景的小而全多模态模型。

### 1.2 官方 benchmark 显示：`E4B` 明显比 `E2B` 更像“可认真用”的小模型

同一张 Google 官方模型卡表里，`E4B` 相比 `E2B` 在若干通用 benchmark 上有明显提升，例如：

- `MMLU Pro`：`69.4` vs `60.0`
- `AIME 2026 no tools`：`42.5` vs `37.5`
- `LiveCodeBench v6`：`52.0` vs `44.0`
- `MMMLU`：`76.6` vs `67.4`

这说明如果只是看模型能力上限：

- `E2B` 更偏“极致轻量 / 边缘设备 / very fast judge”
- `E4B` 更偏“仍然足够小，但质量已经明显更像一款正式 agent 小模型”

---

## 2. Gemma 4 小模型的主要优点

## 2.1 对 agent 工作流友好

Google 官方对 `Gemma 4` 的描述里，明确强调了：

- native system prompt support
- function calling
- structured JSON output
- agentic workflows

这对当前项目是加分项，因为我们做的不是纯聊天，而是：

- `SemanticTurnJudge`
- `SemanticSlotParser`
- 未来的 tool / clarify / actionability 语义链

这些任务都偏向：

- 结构化输出
- 短 schema
- 可控对话

从官方定位看，Gemma 4 的确不是“只能闲聊”的模型。

## 2.2 小模型就有长上下文

`E2B / E4B` 都提供 `128K` 上下文，这在小模型里比较激进。

对当前项目的直接意义：

- 如果后续想把较长的 `voice.previous.*`、工具上下文、较长设备状态、房间/实体 catalog 摘要、近期多轮对话一起喂给模型，Gemma 4 不会太早吃满上下文。

不过也要注意：

- 对 `Tier 1 semantic judge` 这类实时裁判任务，`128K` 并不是核心优势
- 这类任务真正关键的仍是：延迟、中文口语理解、JSON 稳定性、误判率

## 2.3 小模型原生支持音频输入

Google 官方模型卡明确写到：

- `E2B` / `E4B` 支持 `Audio`
- 音频最长支持 `30 seconds`
- 官方示例里可以直接做 speech transcription / translation

这件事很值得关注，因为它说明：

- `Gemma 4` 的小模型不只是“文本 LLM”，而是已经把一部分音频理解能力带进了边缘模型
- 从长期看，它很适合作为“语音 agent 的多模态补充模型”去研究

但这里一定要分清：

- 这是 **prompt-style multimodal audio understanding**
- 不是当前项目主线所需的 **低延迟 streaming ASR + preview + endpointing**

这一点决定了它是否适合当前主链，后面会详细说。

## 2.4 官方生态支持很强

Google 官方博客明确写到：

- day-one 支持 `Transformers`
- 也支持 `vLLM`、`llama.cpp`、`Ollama` 等

这对当前项目是好消息，因为我们本地 worker 当前就是 `transformers` 路线。也就是说：

- 从“能不能在现有 Python 栈里尝试跑起来”这个角度，Gemma 4 不算难接

---

## 3. Gemma 4 小模型对当前项目的关键短板

## 3.1 它的音频能力很有吸引力，但**不适合直接替换当前实时语音主链**

这是最重要的判断。

Google 官方模型卡里，`E2B / E4B` 的音频示例本质上是：

- 输入一段音频
- 再配一个 prompt
- 输出转写/翻译文本

同时官方明确写到：

- audio 最长 `30 seconds`

这意味着：

1. 它当前公开的音频使用形态，更像是 **多模态单次推理**。
2. 它并没有像当前项目所需那样，明确提供：
   - streaming partial
   - stable prefix
   - endpoint hint
   - server endpoint candidate
   - final-ASR correction
3. 因此它**不适合直接替换当前项目基于 FunASR 的实时流式 ASR 主链**。

对当前项目来说，这一点几乎是决定性的：

- 我们当前最核心的不是“有没有一个能听懂 30 秒音频的模型”
- 而是“能不能更早、更稳地给出 preview / endpoint / interruption evidence”

而 Gemma 4 官方公开能力并没有显示出它已经是这一类 streaming runtime 模型。

### 结论

- **不推荐**把 Gemma 4 E2B/E4B 作为当前 realtime ASR / endpoint 主链替代品
- **推荐**把它当成 text-side semantic judge / slot parser / multimodal sidecar 的候选模型

## 3.2 当前仓库的本地 LLM worker 是 text-only CausalLM 路线，Gemma 4 多模态能力接不进来

当前仓库本地 worker 代码的现实是：

- `workers/python/src/agent_server_workers/local_llm_service.py` 当前走的是：
  - `AutoTokenizer`
  - `AutoModelForCausalLM`
- 而 Gemma 4 官方 Hugging Face 示例则是：
  - 文本模式：`AutoProcessor + AutoModelForCausalLM`
  - 多模态模式：`AutoProcessor + AutoModelForMultimodalLM`

这意味着：

- **如果只拿 Gemma 4 做文本推理，接入门槛不高，但仍需做一次兼容性验证。**
- **如果想吃到它的音频/图像能力，当前 worker 代码路径不够，需要改为 `AutoProcessor` 乃至 `AutoModelForMultimodalLM`。**

对当前项目的实际结论就是：

- 现在就算你切到 Gemma 4，小模型最现实的用途也仍然是 **文本侧小模型任务**
- 它的多模态卖点，当前项目暂时吃不到主要红利

## 3.3 中文是“支持”，但不是它最显著、最明确的强项

Google 官方明确写到：

- out-of-the-box 支持 `35+ languages`
- pre-trained on `140+ languages`

这说明 Gemma 4 的 multilingual 面是强的。

但是，从当前公开的一手资料里，我没有看到像我们现在选 Qwen 路线时那样，对“中文口语 agent、中文短指令、中文家居/桌面助理语义裁判”给出特别明确的优势证明。

因此这里要做一个很重要的工程判断：

- **Gemma 4 的多语能力是加分项**
- **但对当前中文优先项目，它并没有在公开资料里体现出足以压倒 Qwen 路线的确定性优势**

这是一个**基于官方公开资料范围的推断**，不是说 Gemma 4 中文一定差，而是说：

> 对当前中文语音 agent 项目，Gemma 4 目前更像“值得 A/B 的优秀备选”，还不像“可以立刻替代当前 Qwen 主线”的确定性答案。

## 3.4 `thinking` 机制与当前 worker 的默认处理方式并非完全同构

Google 官方 Hugging Face 模型卡明确写到：

- Gemma 4 通过 `enable_thinking` 控制 thinking mode
- 开启时会输出特殊 thought channel
- 关闭 thinking 后，部分变体仍可能出现 thought tag 的行为差异

而当前仓库本地 worker 的现实是：

- 主要处理的是 Qwen 风格的 `<think>...</think>`
- 会尝试在 `apply_chat_template` 上设置 `enable_thinking=False`（若 tokenizer 支持）
- 但默认的 streamed think-filter 仍主要针对 `<think>` 标签

这意味着：

- **Gemma 4 文本接入不是完全零风险的“换个 model_id 就行”。**
- 最好先做一次：
  - non-thinking 模式验证
  - stream 输出验证
  - tool / JSON 输出验证

这不是大问题，但说明它更适合“评测候选”，而不是“直接切主路径”。

---

## 4. 那它到底适不适合当前项目？

## 4.1 适合做什么

### A. 适合做 `Tier 1 semantic judge` 的 A/B 候选

对于当前项目的 `SemanticTurnJudge`：

- 任务短
- 输出结构化
- 更关心：
  - utterance completeness
  - continue / correction
  - takeover / backchannel
  - wait delta

从官方定位看，Gemma 4 E2B/E4B：

- 小
- 支持 agentic / structured JSON
- 支持 non-thinking 模式

所以它**适合拿来做小模型语义裁判的 A/B 候选**。

其中：

- `E2B`：更适合极低成本、极低延迟 judge 实验
- `E4B`：更适合追求更稳一点的语义质量

### B. 适合做 `Tier 2 slot parser` 的候选，但我更偏向 `E4B` 而不是 `E2B`

`slot parser` 比 semantic judge 更需要：

- domain / intent 区分
- slot completeness
- clarify_needed
- actionability

这类任务通常比单纯 complete/incomplete 更吃模型能力。

因此如果要试 Gemma 4：

- `E2B` 可试，但我不会把它当首选
- `E4B` 更像一个合理候选

### C. 适合未来作为 multimodal sidecar 研究对象

例如未来如果我们想研究：

- 图片 + 文本 + 语音上下文的 agent
- 屏幕/UI 语义辅助
- 音频理解 side task

Gemma 4 E2B/E4B 是值得关注的，因为它把：

- audio
- image
- long context
- agentic JSON

放在了比较小的模型尺寸里。

但这是后续方向，不是当前主线的第一优先级。

---

## 4.2 不适合做什么

### A. 不适合当前项目的 realtime streaming ASR 主模型

原因前面已经讲过：

- 官方公开形态不是 streaming preview/endpoint runtime
- 当前项目的主问题不是单次 audio prompt，而是 streaming turn orchestration

### B. 不适合当前项目的首选主对话模型

这是我对当前项目的明确判断：

- **如果只看“本地中文语音 agent server 的当前阶段”，Gemma 4 E2B/E4B 都不应成为主对话 LLM 首选。**

原因：

1. 当前项目是中文优先。
2. 当前项目已经在 Qwen 路线上形成了：
   - `Qwen3-1.7B` 做 semantic judge 的研究结论
   - `Qwen3-4B / 8B` 做 slot parser 的现实路径
3. Gemma 4 小模型的官方公开资料，尚不足以让我认为它在中文主对话上能明显优于当前 Qwen 方案。

### C. 不适合现在就为了它去重写 worker 为 multimodal-first

虽然 Gemma 4 的 audio/image 很诱人，但当前项目主线依然是：

- `FunASR + server endpoint + semantic judge + slot parser + speech planner`

在这个阶段：

- 先把 realtime 中文语音体验做顺
- 比为了追 Gemma 4 多模态而重写 local worker 更重要

---

## 5. 我给当前项目的明确建议

## 5.1 总判断

### 如果你的问题是：Gemma 4 小模型“强不强”？

我的回答是：

- **强，尤其从官方定位看，E2B/E4B 是这一代很有竞争力的小型 agent/multimodal 开放模型。**

### 如果你的问题是：Gemma 4 小模型“适不适合当前项目”？

我的回答是：

- **适合做候选，但不适合直接切成当前主线默认。**

## 5.2 分角色建议

### 角色 1：`Tier 1 semantic judge`

- `Gemma 4 E2B`：可试
- `Gemma 4 E4B`：更值得试
- 推荐度：`中高`
- 适用方式：A/B against `Qwen3-1.7B`

### 角色 2：`Tier 2 slot parser`

- `Gemma 4 E2B`：一般
- `Gemma 4 E4B`：可试
- 推荐度：`中`
- 适用方式：A/B against `Qwen3-4B`

### 角色 3：主对话 LLM

- `Gemma 4 E2B/E4B`：不推荐作为当前默认主模型
- 推荐度：`低`
- 原因：中文优先、项目已有 Qwen 路线、当前核心不是换主聊天模型

### 角色 4：实时 ASR / endpoint 主链

- `Gemma 4 E2B/E4B`：不推荐
- 推荐度：`很低`
- 原因：官方公开形态不匹配当前 streaming ASR runtime 需求

---

## 5.3 如果要在当前项目里试 Gemma 4，我建议的顺序

1. **先只试 `google/gemma-4-E4B-it` 文本模式**
   - `enable_thinking=False`
   - 只做 `SemanticTurnJudge` 或 `SemanticSlotParser` 的 A/B
2. **不要先试音频模式**
   - 因为这会把评测变量混进 ASR 主链，收益不清晰
3. **先比 4 项指标**
   - JSON 结构稳定性
   - 中文短口语完整性判断
   - `continue/correction/backchannel` 区分
   - 延迟抖动
4. **只有文本 A/B 结果明显好，再考虑更深接入**

---

## 6. 最终结论

### 一句话结论

**Gemma 4 小模型很有看点，尤其是 `E4B`；但对当前项目，它更像“值得认真 A/B 的优质备选”，而不是“应该立刻替换当前 Qwen/FunASR 主线的默认答案”。**

### 更落地一点的结论

- `E2B`：适合做超轻量语义裁判实验，但我不建议把它当当前项目的主力
- `E4B`：是当前最值得试的 Gemma 4 小模型变体，可作为 `semantic judge / slot parser` 候选
- **不建议**当前把 Gemma 4 用作：
  - realtime streaming ASR 主链
  - 当前默认主对话 LLM
- **建议**把它作为：
  - 文本侧小模型 A/B 候选
  - 未来 multimodal sidecar 研究对象

---

## 参考链接

- Google 官方博客（发布日期、定位、尺寸、生态）：<https://blog.google/innovation-and-ai/technology/developers-tools/gemma-4/>
- Google 官方 Hugging Face 模型卡 `gemma-4-E4B`：<https://huggingface.co/google/gemma-4-E4B>
- Qwen 官方 Hugging Face `Qwen3-1.7B`：<https://huggingface.co/Qwen/Qwen3-1.7B>
