# 智能家居 / 桌面助理的 `dynamic bias list + alias + entity catalog` 最小可行结构研究（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标是回答：
  - 面向智能家居 / 桌面助理，`dynamic bias list + alias + entity catalog` 的最小可行结构该怎么设计？
- 当前项目仍处于研究阶段，因此重点是：
  - 先定义一个高 ROI、低复杂度、适合流式 preview + final correction 的最小结构

## 一句话结论

最小可行结构不应从“全量知识库”开始，而应从：

- **一个轻量实体目录**
- **一套受控 alias 层**
- **一条动态 top-K bias 生成逻辑**

开始。

更具体地说：

- `preview` 前通道只吃：小规模、强相关、低歧义的 top-K bias
- `final` 后通道再吃：完整 catalog、alias 图谱、拼音/英文缩写、实体纠错

## 为什么最小可行结构不能一开始就做成“大词表”

### 1. 流式前通道最怕过度 bias

如果一开始把完整智能家居实体库全部砸进 preview 路径，风险是：

- 没说“空调”，也更容易被认成“空调”
- room/device 同音近音更容易误吸附
- preview partial 更不稳定

Google 的 adaptation 文档明确提醒：

- boost 太高会带来 false positive
- 多词短语需谨慎处理

参考：

- <https://cloud.google.com/speech-to-text/docs/adaptation-model>

### 2. 真实高价值上下文通常非常局部

对 assistant 场景，更有价值的不是全量目录，而是：

- 当前房间
- 当前设备群
- 最近提到的实体
- 当前焦点 App / Window / File
- 当前用户常用候选

这意味着：

- dynamic top-K 比 static full-list 更关键

### 3. alias 层的作用比“更多实体”更大

对很多错误，问题不是 catalog 里没有这个实体，而是：

- 用户说的是别名
- 英文缩写
- 拼音近音
- 常见误叫法

因此早期最值得投入的是 alias 层，而不是 catalog 做得无限大。

## 我最推荐的最小可行 catalog 字段设计

## A. 实体主键层

```text
entity_id
canonical_name
entity_type
namespace
```

### 字段说明

- `entity_id`：稳定主键
- `canonical_name`：标准规范名
- `entity_type`：如 room / device / scene / app / file / contact / media / mode
- `namespace`：如 smart_home / desktop / media / personal

这四个字段构成最小身份层。

## B. alias 层

```text
aliases_zh[]
aliases_en[]
abbreviations[]
pinyin_aliases[]
phonetic_aliases[]
common_misrecognitions[]
```

### 字段说明

- `aliases_zh[]`：中文简称、俗称
- `aliases_en[]`：英文名、品牌名
- `abbreviations[]`：缩写，如 `NAS`、`HDMI`、`VS Code`
- `pinyin_aliases[]`：拼音级表达，适合中文实体纠偏
- `phonetic_aliases[]`：更接近发音映射的别名
- `common_misrecognitions[]`：历史高频误识别对

这层是 MVP 的核心，原因是：

- 智能家居 / 桌面助理场景里，大量错误其实是 alias recovery 问题

## C. 上下文与作用域层

```text
room_scope
location_scope
owner_scope
device_group
current_context_tags[]
```

### 字段说明

- `room_scope`：如 客厅 / 主卧 / 书房
- `location_scope`：更广的物理位置
- `owner_scope`：谁的设备、谁的文件等
- `device_group`：灯 / 空调 / 音箱 / 投影 / 窗帘
- `current_context_tags[]`：用于动态 top-K 过滤，例如 active / nearby / recent / focused

这层不要求一开始很重，但至少要有 room / group 这类能直接提升命令执行准确率的作用域信息。

## D. 动作兼容与风险层

```text
action_compatibility[]
risk_level
requires_disambiguation
```

### 字段说明

- `action_compatibility[]`：该实体通常支持哪些动作
  - 例如灯支持：打开/关闭/调亮度
  - App 支持：打开/关闭/切换
- `risk_level`：低/中/高风险
- `requires_disambiguation`：若实体本身高歧义，则要求更高确认门槛

这层对 current project 很重要，因为它能把 ASR catalog 直接接到后续 tool planning / slot completeness 上。

## E. 运行时排序层

```text
frequency_prior
recency_prior
session_boost
preview_bias_weight
final_bias_weight
```

### 字段说明

- `frequency_prior`：长期使用频率
- `recency_prior`：最近使用频率
- `session_boost`：当前轮/当前 session 动态加权
- `preview_bias_weight`：适合前通道的轻 bias 权重
- `final_bias_weight`：适合 final correction 的较强权重

## 更适合当前项目的 MVP 实体结构

如果只保留研究阶段最小可行字段，我建议这样讨论：

```text
EntityCatalogItem {
  entity_id
  canonical_name
  entity_type
  namespace

  aliases_zh[]
  aliases_en[]
  abbreviations[]
  pinyin_aliases[]
  common_misrecognitions[]

  room_scope
  device_group
  action_compatibility[]
  risk_level

  frequency_prior
  recency_prior
  preview_bias_weight
  final_bias_weight
}
```

## 哪些字段适合进 streaming preview top-K bias

### 适合前通道 preview bias 的

- `canonical_name` 的短形式
- `aliases_zh[]` 中最常用、最短、最不歧义的别名
- `abbreviations[]` 中高频且强相关的缩写
- `room_scope`
- `device_group`
- `action_compatibility[]`
- `preview_bias_weight`
- `session_boost / recency_prior`

### 为什么只放这些

因为 preview 路径要优先追求：

- 快
- 稳
- 不过度 hallucination

因此：

- 不应把所有长尾 alias 都压进前通道
- 不应把完整 catalog 全塞给流式路径

## 哪些字段更适合 final correction

- 全量 `aliases_zh[]`
- 全量 `aliases_en[]`
- `pinyin_aliases[]`
- `common_misrecognitions[]`
- 更细粒度 phonetic alias
- 更大的 related entity 候选集合
- 更完整的 room/device/app/file/contact namespace 解析
- 更强的 `final_bias_weight`

### 为什么

因为 final 路径更适合做：

- 强 contextualization
- ambiguity resolution
- retrieval correction
- entity normalization

## dynamic top-K 选择逻辑如何做

我建议不要直接问“哪些实体进 top-K”，而是先做四层筛选。

## 第一步：按 turn type / intent candidate 做粗筛

例如：

- 如果更像智能家居控制
  - 优先设备、房间、场景
- 如果更像桌面操作
  - 优先 App / window / file
- 如果更像 media
  - 优先 media / app / device

## 第二步：按上下文作用域做收缩

优先保留：

- 当前房间内实体
- 当前 session 最近提到的实体
- 当前焦点窗口 / App
- 最近操作过的实体

## 第三步：按兼容度与歧义惩罚打分

研究阶段可讨论成：

```text
entity_rank_score =
  + action_match_score
  + scope_match_score
  + recency_score
  + frequency_score
  + alias_match_score
  - ambiguity_penalty
  - risk_penalty_for_preview
```

### 字段解释

- `action_match_score`：当前动词与实体是否兼容
- `scope_match_score`：room / current focus / session context 是否匹配
- `alias_match_score`：partial 是否和实体 alias 有初步对齐
- `ambiguity_penalty`：若多个相似实体竞争，则降低其 preview 权重
- `risk_penalty_for_preview`：高风险实体不宜在 preview 过强 bias

## 第四步：按通道分别截断

### preview top-K

建议保守：

- 数量更小
- 实体更集中
- alias 更少
- 歧义更低

### final candidate set

可以更大：

- 更完整 catalog
- 更全 alias
- 更强 normalization / correction

## 最常见的错误，以及 catalog 应如何兜住

### 1. 同音近音实体

例如：

- `筒灯 / 同等`
- `幕布 / 目不`
- `米家 / 米加`

catalog 要兜住的方法：

- `pinyin_aliases[]`
- `common_misrecognitions[]`
- 相似实体组的 ambiguity penalty
- final-path retrieval correction

### 2. 房间名 + 设备名组合歧义

例如：

- 客厅灯 / 主卧灯 / 书房灯

catalog 要兜住的方法：

- room_scope
- entity_type + device_group
- action_compatibility
- recent room context

### 3. 中英混说 / 缩写

例如：

- `NAS`
- `HDMI`
- `VS Code`
- `HomePod`

catalog 要兜住的方法：

- `aliases_en[]`
- `abbreviations[]`
- 字母发音 alias
- final-path normalization

### 4. 场景名与设备名混淆

例如：

- `回家模式`
- `影院模式`
- `影院灯`

catalog 要兜住的方法：

- entity_type 区分 scene vs device
- action_compatibility 区分
- requires_disambiguation

### 5. 文件 / App / 窗口名长尾问题

例如桌面助理：

- 应用名
- 文件名
- 项目名
- 窗口标题

catalog 要兜住的方法：

- namespace 区分
- active/focused context tag
- final-path retrieval correction

## 对当前 `agent-server` 最值得借鉴的边界

## 1. 先做“轻 catalog + 动态 top-K”，不要直接做“大而全知识库”

这是研究阶段 ROI 最高的做法。

## 2. preview 路径只用“高相关、低歧义、短 alias”

这样更适合与当前流式 preview + final correction 结构配合。

## 3. final 路径再接完整 alias 图谱与 entity correction

这与当前项目的 2pass 主线高度一致：

- `preview` 保守
- `final` 强纠正

## 4. catalog 应直接为后续 slot completeness 服务

也就是说，catalog 不应只是 ASR 词表，而应能直接支持：

- entity normalization
- ambiguity resolution
- action compatibility 检查
- 可执行性判断

## 参考资料

- Google model adaptation：<https://cloud.google.com/speech-to-text/docs/adaptation-model>
- Google `RecognitionConfig`：<https://cloud.google.com/speech-to-text/docs/reference/rest/v1p1beta1/RecognitionConfig>
- FunASR：<https://github.com/modelscope/FunASR>
- SenseVoice：<https://github.com/FunAudioLLM/SenseVoice>
- Apple Class LM and Word Mapping：<https://machinelearning.apple.com/research/class-lm-and-word-mapping>
- Apple Retrieval-Augmented Correction of Named Entity ASR Errors：<https://machinelearning.apple.com/research/retrieval-asr>
- Apple Contextualization of ASR with LLM Using Phonetic Retrieval-Based Augmentation：<https://machinelearning.apple.com/research/asr-contextualization>
- Amazon contextual LM adaptation：<https://assets.amazon.science/5d/52/5dfaa0244b5f9b80bdefeb10d201/attention-based-contextual-language-model-adaptation-for-speech-recognition.pdf>
- Amazon synthetic-audio contextual biasing：<https://assets.amazon.science/68/89/8ffe4a544946b6d36280603ea2e5/effective-training-of-attention-based-contextual-biasing-adapters-with-synthetic-audio-for-personalised-asr.pdf>
- NVIDIA NeMo ASR customization：<https://docs.nvidia.com/nemo-framework/user-guide/24.07/nemotoolkit/asr/asr_language_modeling_and_customization.html>
- NVIDIA NeMo Word Boosting：<https://docs.nvidia.com/nemo-framework/user-guide/latest/nemotoolkit/asr/asr_customization/word_boosting.html>
- sherpa-onnx hotwords：<https://k2-fsa.github.io/sherpa/onnx/hotwords/index.html>
