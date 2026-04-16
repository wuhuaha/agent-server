# 面向智能家居 / 桌面助理的领域 ASR 提升研究（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标是回答：在智能家居 / 桌面助理 / agent 指令交互场景下，如何提升 ASR 的“可执行准确率”，而不只是通用 WER/CER。

## 一句话结论

对这类场景，最有 ROI 的顺序通常不是先做重型全量微调，而是：

1. 动态 contextual biasing / 热词与类目约束
2. alias / 发音 / 拼音层
3. 2-pass final rescoring + 后处理纠错
4. 再做数据构造、蒸馏、PEFT/微调

更直白一点：

- **实体恢复能力**，往往比“通用 WER 再降一点”更重要。

## 这类场景为什么难

核心难点通常不是普通话或英语基础声学不够，而是：

- 动态实体太多
- 同音近音太多
- 命令很短
- 上下文很强
- 尾部 slot 很关键
- 中英混说、缩写、品牌名多

因此，很多错误不是“完全没听见”，而是：

- 听到了近似声学形状，但落到了常见词
- 命令主干对了，但设备名或数值槽错了
- 句尾被截断，导致动作不可执行

## 提升领域识别效果的主要路径分层

### A. 热词 / Phrase hint / Word boosting

把设备名、房间名、场景名、品牌名、App 名、联系人名、文件名等，作为 bias list 注入解码。

代表性实践：

- Google `PhraseSet + boost`
- FunASR `hotword`
- NeMo `word boosting`
- sherpa-onnx `hotwords`

优点：

- 快
- 便宜
- 易实验

缺点：

- 易 `over-bias`
- 可能把没说出来的词“脑补”出来

### B. Contextual biasing / Custom class / 语法约束

这比单纯热词更强，因为它不是“堆词表”，而是告诉 ASR：

- 当前最可能出现哪些词类
- 以及这些词类通常如何组合成命令

典型做法：

- `CustomClass`：房间名、设备名、场景名、App 名、联系人名
- `ABNF grammar / command grammar`
- `Class LM / context FST / context graph`

这对智能家居 / 桌面助理尤其重要，因为命令空间高度结构化。

### C. Lexicon / 发音 / 别名 / 拼音层

这层解决的是：ASR 已经听到了接近的声学形状，但落字不对。

典型做法：

- 维护 alias 表
- 英文缩写发音映射
- 拼音 / 近音变体
- 对困难实体做发音驱动映射

对于中文智能家居场景，这层往往收益非常高。

### D. LM rescoring / 2-pass rescoring / N-best 复排

第一遍先快，第二遍再准。

可做：

- shallow fusion / domain LM
- N-best rescoring
- class-aware rescoring
- WFST hotwords / context graph
- final-only 更强 bias

这与当前项目的 streaming preview + final-ASR 路线天然契合。

### E. 后处理纠错 / Entity recovery / Retrieval correction

这层不重新做声学识别，而是做：

- confusion set 纠错
- entity resolver
- retrieval-based correction
- 受限 LLM 纠错

重点不是让 LLM 自由发挥，而是让它在候选实体范围里约束纠错。

### F. 数据构造 / 合成数据 / 蒸馏 / 微调

当上面几层吃到瓶颈，再往训练层走：

- 真日志回流
- TTS 合成命令
- hard negative
- PEFT/LoRA
- teacher-student / streaming-final distillation

### G. Endpoint / ITN / normalization / punctuation

这不只是“附加优化”，对 agent 场景的可执行准确率非常关键。

典型问题：

- endpoint 太早，尾部 slot 被截断
- ITN 不稳，数字 / 时间 / 百分比归一化混乱
- normalization 不一致，导致后续 NLU / tool call 失败

## 哪些适合当前研究阶段快速验证

### 最适合当前阶段快速验证的

- 动态 bias list
- 命令模板 + 类目约束
- alias / 拼音 / 英文缩写映射
- final-only 更强修正
- 后处理实体归一化
- 域内评测集

### 中等投入但很值得做的

- 2-pass 复排
- TTS 合成域内命令数据
- hard negative 数据
- 受限检索纠错
- 轻量 PEFT/LoRA

### 更重、更后置的

- 重训 streaming ASR 主模型
- 自研 contextual bias encoder / class-aware transducer
- 修改 tokenization / pronunciation-aware subword 训练链
- 大规模 personalized / federated adaptation
- 让自由 LLM 直接接管 ASR 纠错主链

## 智能家居 / 桌面助理最常见的错误类型与补救

### 1. 稀有实体 / 长尾设备名

- 错误：房间名、设备名、场景名、品牌名被识成常见词
- 补救：
  - 动态热词/类目
  - alias 表
  - final-pass entity recovery
  - 受限检索纠错

### 2. 同音 / 近音 / 形近实体

- 错误：`筒灯/同等`、`幕布/目不`、`米家/米加`
- 补救：
  - 拼音/发音映射
  - hard negatives
  - pronunciation-driven biasing
  - 相似实体 disambiguation

### 3. 中英混说 / 缩写 / 品牌名

- 错误：`NAS`、`HDMI`、`HomePod`、`VS Code`
- 补救：
  - 中英别名双写
  - 字母发音别名
  - domain lexicon
  - final-pass rewrite

### 4. 数字、时间、百分比、单位

- 错误：`30%`、`6点半`、`二号灯`、`一档/二档`
- 补救：
  - grammar / class token
  - ITN
  - slot validator
  - 面向动作槽位的 normalization

### 5. 短命令、上下文弱、句子太短

- 错误：`开灯`、`锁屏`、`静音`
- 补救：
  - 利用 session context 做 bias
  - 用当前房间 / 当前焦点窗口 / 最近实体增强推断
  - 不只看 WER，要看 execution accuracy

### 6. 尾部 slot 被截断

- 错误：`把客厅灯调到三十...` 被提前收尾
- 补救：
  - endpoint 更保守地保护 slot 尾部
  - final pass 确认
  - 如果值槽不完整，不立即执行

### 7. 用户自我纠正

- 错误：`打开卧室灯，不对，客厅灯`
- 补救：
  - correction-aware rewrite
  - repetition-based recovery
  - 会话级融合前后两句，而不是只看最后一句 ASR

### 8. 过度 bias 造成“幻听”

- 错误：没说“空调”，系统也认成“空调”
- 补救：
  - bias list 做 top-K 裁剪
  - streaming 轻 bias，final 强 bias
  - 对低置信且高风险动作加确认

## 推荐给当前 `agent-server` 的优先研究组合

### P0：最优先

#### 组合 1：流式轻 bias + final 强修正

- 流式 preview：
  - 只注入小规模、强相关、低风险 bias
  - 例如动作词、房间名、当前 session top-K 设备
- final：
  - 再上更强的 hotword / WFST / Ngram LM / entity correction

#### 组合 2：实体库驱动的动态 contextual biasing

每轮实时生成：

- 房间名
- 设备名
- 场景名
- App / 文件 / 联系人
- 最近一轮提到的实体

#### 组合 3：alias / 拼音 / 英文缩写层

给每个实体维护：

- canonical name
- 中文简称
- 常见误叫法
- 拼音 / 近音
- 英文 / 缩写

#### 组合 4：后处理实体归一化 + 受限纠错

- 不让自由 LLM 任意改写
- 更推荐从候选实体库检索并约束纠错
- 高歧义时先澄清，不盲执行

#### 组合 5：专项评测指标

不要只看 CER/WER，还要看：

- entity recall
- slot accuracy
- command execution accuracy
- dangerous false accept rate

### P1：第二批值得做的

- 用 FunASR `2pass-offline + Ngram LM + WFST hotwords` 做 final-pass 强化
- 针对相似实体做 hard-negative 训练 / 评测
- 用 TTS 合成域内命令数据，先做 PEFT 小步微调
- 对高价值 rare entity 做 retrieval-based correction

### P2：更后置、但长期值得看

- SeACo-Paraformer 这类更深层 contextual / hotword 模型
- pronunciation-aware tokenization
- retrieval + constrained LLM 的实体纠错主链
- personalized / continual adaptation

## 我对当前项目最具体的建议

如果当前主线已经在推进 `streaming preview + final confirm`，那么领域增强也应严格 2 段化：

- `preview`：保守、快、轻 bias
- `final`：更强 contextualization、LM rescoring、entity correction

我不建议一开始把完整智能家居实体库重锤进流式前通道，这会明显增加误命中。

更推荐：

- 前通道只用 top-K session context
- 后通道再做完整 catalog 纠错

## 参考资料

- FunASR：<https://github.com/modelscope/FunASR>
- SeACo-Paraformer：<https://arxiv.org/abs/2308.03266>
- SenseVoice：<https://github.com/FunAudioLLM/SenseVoice>
- Google model adaptation：<https://cloud.google.com/speech-to-text/docs/adaptation-model>
- Google `RecognitionConfig`：<https://cloud.google.com/speech-to-text/docs/reference/rest/v1p1beta1/RecognitionConfig>
- Google Deep Context：<https://research.google/pubs/deep-context-end-to-end-contextual-speech-recognition/>
- Amazon contextual LM adaptation：<https://assets.amazon.science/5d/52/5dfaa0244b5f9b80bdefeb10d201/attention-based-contextual-language-model-adaptation-for-speech-recognition.pdf>
- Amazon synthetic-audio contextual biasing：<https://assets.amazon.science/68/89/8ffe4a544946b6d36280603ea2e5/effective-training-of-attention-based-contextual-biasing-adapters-with-synthetic-audio-for-personalised-asr.pdf>
- Apple Class LM and Word Mapping：<https://machinelearning.apple.com/research/class-lm-and-word-mapping>
- Apple pronunciation-driven tokenization：<https://machinelearning.apple.com/research/ctc-based>
- Apple retrieval-augmented ASR correction：<https://machinelearning.apple.com/research/retrieval-asr>
- NVIDIA NeMo ASR customization：<https://docs.nvidia.com/nemo-framework/user-guide/24.07/nemotoolkit/asr/asr_language_modeling_and_customization.html>
- sherpa-onnx hotwords：<https://k2-fsa.github.io/sherpa/onnx/hotwords/index.html>
- Google contextual recovery of out-of-lattice named entities：<https://research.google/pubs/contextual-recovery-of-out-of-lattice-named-entities-in-automatic-speech-recognition/>
