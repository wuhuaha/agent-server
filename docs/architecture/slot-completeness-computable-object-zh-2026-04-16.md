# `slot completeness` 作为可计算对象的研究（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标是把 `slot completeness` 从一个模糊概念，压缩成一个适合当前 `agent-server` 研究阶段讨论的可计算对象。
- 本文与以下文档形成互补：
  - `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
  - `docs/architecture/dynamic-bias-alias-entity-catalog-mvp-zh-2026-04-16.md`

## 一句话结论

`slot completeness` 不应被理解为“这个槽位有没有文本”，而应理解为：

- **有没有被听见**
- **能不能被解析**
- **能不能被映射到真实世界对象/标准值**
- **后续被 final-ASR 改写的风险高不高**

因此，更合理的表达不是：

- `slot_filled = true/false`

而是一个分解式对象：

- `Fill × Normalize × Disambiguate × Stable`

## 最小可计算表达

对单槽位 `s`、时刻 `t`、候选意图 `I`，更适合当前项目讨论的最小形式是：

```text
SC(s,t|I,ctx) = Req(s|I,ctx) * F(s,t) * N(s,t) * D(s,t) * Stab(s,t)
```

其中：

- `Req(s|I,ctx)`：该槽位在当前意图与上下文下是否必填
- `F(s,t)`：是否已经抽到候选值，或候选 posterior 已足够集中
- `N(s,t)`：候选值是否能被该槽类型解析/归一化
- `D(s,t)`：若是实体槽，能否唯一或近唯一映射到 canonical entity / canonical value
- `Stab(s,t)`：该值在后续 correction 下仍成立的概率，也就是“后续被推翻的风险”

## 更直接的工程理解

- `Fill`：有词了没有？
- `Normalize`：能转成结构化值吗？
- `Disambiguate`：到底是哪个对象？
- `Stable`：下一拍会不会被推翻？

## 意图级 completeness

如果要把多个槽位汇总到意图级，可讨论为：

```text
IC(t|I) = min_{s in Required(I,ctx)} SC(s,t|I,ctx)
```

这表示：

- 对需要的槽位来说，最短板决定当前意图能否进入更高 readiness

也可以更保守地拆成向量，而不是单个值：

```text
IC = {
  required_fill_ratio,
  required_normalized_ratio,
  required_disambiguated_ratio,
  required_stable_ratio,
  executable
}
```

## 为什么这比“槽位是否抽出来了”更重要

### 1. 抽到了不等于能执行

例如：

- `把客厅灯调到三十`

系统可能已经抽到了一个值候选 `三十`，但：

- 单位可能没说完
- 是亮度还是温度，还可能未闭合

### 2. 听见了不等于唯一

例如：

- `打开书房灯`

若 catalog 里存在：

- 书房主灯
- 书房筒灯
- 书房氛围灯

那 `Fill=1`，但 `Disambiguate` 未必高。

### 3. 当前对了不等于后面不改

例如：

- `打开卧室灯，不对，客厅灯`

前一刻槽位看起来已填满，但 `Stable` 很低。

## 面向智能家居 / 桌面助理的槽位类型分层

下面是更适合当前项目研究阶段的槽位类型分层。

## `L0` 隐式上下文槽

例如：

- speaker/profile
- room
- 当前设备
- timezone
- 当前桌面焦点对象

特点：

- 多数可预填或从上下文继承
- 不一定通过当前语音显式说出

## `L1` 封闭集动作槽

例如：

- 开 / 关
- 暂停 / 继续
- 打开 / 关闭
- 确认 / 取消

特点：

- 最适合前通道尽早稳定
- 往往是 preview 路径最先能可靠捕捉的部分

## `L2` 结构化参数槽

例如：

- 数字
- 百分比
- 温度
- 亮度
- 音量
- mode
- duration

特点：

- 需要 normalization
- 比动作槽晚稳定，但比开放文本更容易结构化

## `L3` 目录实体槽

例如：

- device instance
- device group
- room name
- scene
- app
- file
- contact

特点：

- 强依赖 alias / catalog / context
- `Disambiguate` 是关键因子

## `L4` 组合约束槽

例如：

- 绝对/相对时间
- recurrence
- trigger condition
- filter
- sort / compare

特点：

- 常见于桌面助理和复杂家居自动化
- 很容易被后续追加语音修正

## `L5` 开放内容槽

例如：

- 消息正文
- 搜索 query
- 笔记内容
- 邮件主题

特点：

- 不适合太早 commit
- 更适合 final 后再确认

## `L6` 高风险权限槽

例如：

- 门锁目标
- 支付对象
- 删除对象
- 收件人
- 安全模式

特点：

- 即使表面上高 completeness，也不应轻易早提交

## 哪些槽位能前通道粗判，哪些必须 final 确认

## 适合前通道粗判的

### `L1` 动作槽

例如：

- 开
- 关
- 暂停
- 继续
- 静音
- 切换

原因：

- 封闭集
- 强语义
- 往往很早稳定

### `L2` 简单结构化槽

例如：

- 亮度 50%
- 音量加 10
- 空调 26 度

但要求：

- normalization 已基本完成
- correction risk 不高

### 一部分 `L3` 低歧义实体槽

例如：

- 客厅灯
- 主卧空调
- 当前前台 App

前提：

- alias 命中唯一
- catalog 唯一
- 风险低

## 更适合前通道预判，但不建议直接 commit 的

- domain / intent routing
- App / file / contact shortlist
- scene / device 候选集合
- 尚未听完整的时间/数值表达
- 高歧义 room/device 组合

## 原则上应等待 final 或显式确认的

- `L5` 开放文本槽
- `L4` 复杂时间与 recurrence 槽
- 高歧义人名 / 联系人 / 文件名
- `L6` 高风险权限槽
- 任何 `D` 低或 `Stab` 低的槽位

## 最关键的一点：前通道不只是“先判对”，也能“先判缺”

例如：

- `把客厅灯调到...`

系统在很早阶段就可能知道：

- `target` 已有
- `value` 尚未完整

因此，前通道最大的价值之一是：

- 不是提前执行
- 而是提前知道“还不能执行”

## 如何与 `dynamic bias list + alias + entity catalog` 协同

这三者的职责最好严格分开：

- `dynamic bias list`：提高“被听见”的概率
- `alias`：把不同说法/发音/简称归并到同一 canonical 对象
- `entity catalog`：决定系统最终要操作哪个 `entity_id`

一句话总结：

- `bias list` 负责召回
- `alias` 负责归并
- `catalog` 负责真值

## 对实体槽来说，`slot completeness` 最好吃 canonical 后验值，而不是 raw ASR 文本

更适合当前项目的链路是：

```text
streaming ASR text
-> alias match
-> entity candidates
-> context-constrained resolve
-> canonical slot value
-> slot completeness score
```

### 为什么

因为：

- bias 命中但 catalog 无实体：`heard != complete`
- alias 映射多个实体：`filled != complete`
- final-ASR 改写表面文本，但 canonical entity 未变：`slot completeness` 可以保持高

## 对实体槽，`Disambiguate` 最好怎么看

对于实体槽位，`D(s,t)` 更适合基于：

- top1-top2 margin
- entropy
- 与当前 intent/action 的兼容度
- 与 room / device state 的兼容度
- 与 recent-turn carry-over 的一致性

而不应只看 raw ASR confidence。

## 与 UEPG 的关系

`slot completeness` 是 `UEPG` 里最适合约束“执行提交”的那一层。

可以把它理解为：

- `preview-ready`：可以不强依赖 slot completeness
- `draft-ready`：开始需要部分 slot completeness
- `commit-ready`：命令型场景下必须有足够高的 slot completeness

## 对当前 `agent-server` 最值得借鉴的边界

## 1. 先做“最小可计算表达”，不要一上来做复杂联合模型

对当前研究阶段，更合适的是：

- 先把 `Req/F/N/D/Stab` 这 5 个因素显式化
- 再逐步让它们接进 UEPG

## 2. 先让它用于“限制过早执行”，再让它用于“推动更早执行”

也就是说：

- 先拿它防止 commit 太早
- 再逐步拿它支持更智能的 draft-ready / commit-ready

## 3. 对命令型场景，`slot completeness` 应比 `utterance completeness` 更偏硬约束

尤其是：

- 实体槽
- 数值槽
- 时间槽
- 高风险权限槽

## 参考资料

- Google Speech Adaptation：<https://docs.cloud.google.com/speech-to-text/docs/v1/adaptation-model>
- Google `SpeechAdaptation` RPC：<https://cloud.google.com/speech-to-text/docs/reference/rpc/google.cloud.speech.v1#speechadaptation>
- FunASR：<https://github.com/modelscope/FunASR>
- FunASR runtime：<https://github.com/modelscope/FunASR/tree/main/runtime>
- Apple `contextualStrings`：<https://developer.apple.com/documentation/speech/sfspeechrecognitionrequest/contextualstrings>
- Apple `SFCustomLanguageModelData`：<https://developer.apple.com/documentation/speech/sfcustomlanguagemodeldata>
- AWS Transcribe Custom Vocabulary：<https://docs.aws.amazon.com/transcribe/latest/dg/custom-vocabulary.html>
- AWS Transcribe Custom Language Models：<https://docs.aws.amazon.com/transcribe/latest/dg/custom-language-models.html>
- NVIDIA NeMo ASR customization：<https://docs.nvidia.com/nemo-framework/user-guide/latest/nemotoolkit/asr/asr_language_modeling_and_customization.html>
- NVIDIA NeMo repo：<https://github.com/NVIDIA/NeMo>
- sherpa-onnx：<https://github.com/k2-fsa/sherpa-onnx>
