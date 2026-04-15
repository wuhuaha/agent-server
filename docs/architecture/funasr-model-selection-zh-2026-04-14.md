# FunASR 模型选型研究（2026-04-14）

## 目标

这份笔记回答三个问题：

1. 当前项目实际用了 FunASR 生态里的哪些模型或模块
2. 当前 FunASR 相关模块里，哪些模型更值得优先关注，它们各自强在哪里
3. 如果以“语音 Agent demo 的主观效果”优先，而不是以最小改动优先，本项目最适合的模型组合是什么

这里的“效果优先”，指的是综合考虑以下体验指标后的排序：

- 唤醒词和短指令鲁棒性
- 真流式 partial 的及时性
- turn endpoint 的稳定性
- 中文口语、方言、噪声、远场下的识别质量
- 口语化 TTS 的自然度与首包速度

本文会区分两类结论：

- 来自官方仓库或模型卡的直接信息
- 结合本仓库现状做出的项目内推断

## 当前项目实际上在用什么

先看当前仓库真实状态，而不是泛化讨论。

### 当前已经接入的模型与模块

- Final-ASR 主模型默认仍是 `iic/SenseVoiceSmall`
- FunASR worker 现在已经支持按模块组合以下能力：
  - `online_model`：在线 preview / partial
  - `final_vad_model`：final-ASR 前的 FunASR VAD
  - `final_punc_model`：final-ASR 路径的标点恢复
  - `kws_model + kws_keywords`：可选 worker 内部 KWS
- 当前默认仍然保持保守配置：
  - `online_model=""`
  - `final_vad_model=""`
  - `final_punc_model=""`
  - `kws_enabled=false`
  - `stream_endpoint_vad_provider=energy`
- 当前默认开启 `use_itn=true`
- 当前默认保持 `trust_remote_code=false`
- 当前 Docker 侧默认本地 TTS 模型目录仍是 `iic/CosyVoice-300M-SFT`

### 当前已经具备但默认未启用的模块化能力

这些能力已经有了 worker 侧接入口，但默认仍然关闭，只有在明确配置后才进入主链路：

- FunASR 官方 `fsmn-vad` 作为 final 路径 VAD / endpoint hint 候选
- 独立 `ct-punc` 作为 final 路径标点恢复模块
- 独立 KWS 模型，例如 `fsmn-kws` / `fsmn-kws_mt`

### 当前仍未真正接入或显式透出的模块

- 说话人模块，例如 `cam++`
- 没有把 `SenseVoice` 的 LID、SER、AED 能力显式透出到共享协议或 runtime 决策里

### 当前 preview / endpoint 的真实形态

当前 worker 已经同时支持两种 streaming preview 形态：

- 默认仍是“缓冲音频 + 反复重跑 final-ASR”的 `stream_preview_batch`
- 当 `online_model` 被配置时，会切到真流式 online preview + final-ASR correction 的 `stream_2pass_online_final`

当前 endpoint 声学侧也不再只有一个固定路径，而是：

- 默认 `energy` 尾部能量启发式
- 可选 FunASR 官方 `fsmn-vad`
- 可选外部 `Silero VAD`
- `auto` 模式下优先 `fsmn-vad`，其次 `Silero VAD`，最后回退到 `energy`

当前 KWS 也已经进入 worker 设计，但仍是默认关闭、配置后启用的状态：

- `kws_enabled=false` 时，不影响现有主链路
- `kws_enabled=true` 时，worker 才会执行 KWS 检测、发出 `kws_detected` 类 `audio_events`，并可选移除识别文本中的唤醒词前缀
- 当前工程内已经校准出的可运行 KWS baseline 是 `iic/speech_charctc_kws_phone-xiaoyun`；短别名 `fsmn-kws` 在本机 `FunASR 1.3.1` runtime 下不能直接作为 worker 侧默认模型使用

一句话总结当前现状：

> 本项目已经把 `KWS + VAD + online preview + final-ASR` 的模块化 worker 入口搭起来了，但默认 bring-up 仍然是保守的 `SenseVoiceSmall + stream_preview_batch + energy`；更强的 2pass/KWS/VAD 路径已经具备，只是默认不强推。

## 当前值得重点关注的模型，按模块梳理

## 1. ASR 主识别 / 最终纠错

### `Fun-ASR-Nano-2512`

官方 `Fun-ASR` 仓库把它定位成当前新的高效果开放模型之一。官方给出的直接信息包括：

- 在 11 个开源测试集上相比主流开源模型有领先表现
- 支持 31 种语言
- 支持热词、VAD、低时延实时转写
- 对方言、口音、复杂背景、远场等场景有强化

对本项目的意义：

- 如果你追求最终文本质量，它是目前最值得优先 benchmark 的 final-ASR 候选
- 对“短口语命令 + 噪声 + 唤醒词后紧接命令”这类场景，理论上比当前单纯 `SenseVoiceSmall + energy endpoint` 更有上限
- 但把它用好通常需要真正改 worker，而不是只改一个模型名；尤其是要重做 streaming / 2pass 架构时收益才大

项目内判断：

- 它更像“最终纠错模型”或“高质量主识别模型”
- 不是解决当前 preview 流畅性的唯一钥匙

### `SenseVoiceSmall`

`SenseVoice` 官方 README 直接强调了这些能力：

- 多语言语音理解
- 同时覆盖 ASR、LID、SER、AED
- 低时延
- 中文、粤语表现强
- 对情绪与事件信息比传统 ASR 更敏感

`SenseVoiceSmall` 模型卡还直接展示了它可以和 `vad_model="fsmn-vad"` 组合使用。

对本项目的意义：

- 它很适合作为 voice-agent 的“语音理解核心”，因为它不是只给 transcript
- 如果未来想把用户情绪、说话状态、环境事件带进 turn policy 或 spoken-style policy，它的潜力高于传统纯 ASR 模型
- 当前项目已经用上了它，所以迁移成本最低

项目内判断：

- 如果目标是“在当前架构上尽快做出更自然的 demo”，它依然是非常好的 final-ASR 候选
- 但它不该继续独自承担 wake word、preview、endpoint、final decode 四件事

### `SeACo-Paraformer-large`

FunASR 官方 README 明确把它列为中文 ASR 代表模型之一，并强调：

- 热词定制
- 时间戳
- 中文场景成熟

对本项目的意义：

- 如果后续重点是家居设备名、房间名、别名、品牌名等热词定制，`SeACo-Paraformer-large` 很值得做对照 benchmark
- 它在“可控的定制词表 + 中文命令识别”方向上可能比 `SenseVoiceSmall` 更容易针对业务做强化

项目内判断：

- 当目标偏“命令词/设备词强定制”而不是“情绪/事件理解”时，它是很强的备选 final-ASR

## 2. 真流式 online ASR / preview partial

### `Paraformer-large-online` / `paraformer-zh-streaming`

FunASR 官方 README 和 runtime 文档都把 Paraformer online 系列放在实时转写和 2pass 服务的主链路上。
官方 runtime 文档明确说明 2pass 服务的价值是：

- 先给实时转写
- 说话结束后再做高精度纠错
- 同时支持标点与热词

这和当前项目最需要补的短板高度一致。

对本项目的意义：

- 它比现在的 `stream_preview_batch` 更适合承担 preview / partial 的职责
- 也更适合作为 hidden preview、barge-in 判定、speech planner 的上游信号
- 如果还想保留当前 websocket 合同不变，把 online ASR 仅作为 `internal/voice` 内部 preview 信号，是非常顺的路径

项目内判断：

- 对“实时语音 demo 的流畅性”来说，最该先补的不是继续换 batch ASR，而是把 preview 换成真 streaming online ASR
- `Paraformer-large-online` 是这条链路目前最合适的官方候选

## 3. VAD / endpoint

### `fsmn-vad`

FunASR 官方 model zoo 把 `fsmn-vad` 列成独立模块，参数量很小，定位就是语音活动检测。

对本项目的意义：

- 它是最应该替换当前 `energy` heuristic 的官方候选
- 它适合做 acoustic endpoint 的第一层，把当前声学判断从简单尾能量提升到真正的 VAD 模型
- 它还可以和已有 lexical hold、preview hint、barge-in 策略叠加，而不是互斥替换

项目内判断：

- 对当前项目，“`energy` -> `fsmn-vad`”是模型侧最高 ROI 的短板修补之一
- 即便未来继续保留可选 `Silero VAD`，也应该优先把 `fsmn-vad` 接入成官方主路径，再做 A/B

## 4. 标点 / ITN

### `ct-punc`

FunASR 官方 model zoo 里仍把 `ct-punc` 作为独立标点恢复模块列出。

对本项目的意义：

- 如果 online preview 路径改成 Paraformer streaming，`ct-punc` 能明显改善最终文本可读性和后续 LLM 理解稳定性
- 对家居语音命令这种短句，收益没有 VAD 和 streaming 那么高，但对聊天型场景是增益项

项目内判断：

- 如果 final-ASR 选 `SenseVoiceSmall` 或 `Fun-ASR-Nano-2512`，可以先依赖模型自身文本规范化能力
- 如果主链路改成 `Paraformer online/offline`，则建议尽快把 `ct-punc` 纳入 finalization path

## 5. Wake word / KWS

### `fsmn-kws` / `fsmn-kws_mt`

FunASR 官方 model zoo 仍列出了 `fsmn-kws` 和 `fsmn-kws_mt`。

对本项目的意义：

- 当前项目已经出现“唤醒词前缀 + 短指令”样本劣化，说明单靠 ASR 文本去顺带识别 wake word 不够稳
- KWS 和 ASR 的职责应该拆开：
  - KWS 负责“有没有被唤醒”
  - ASR 负责“用户说了什么”
- 这样会直接减少把唤醒词误识别成普通文本后又影响 endpoint / preview 的连锁误差

项目内判断：

- 当前项目不该继续把唤醒词稳定性押在 `SenseVoiceSmall` 的 transcript 上
- 单唤醒词方向上优先从 `fsmn-kws` 这类独立 KWS 能力开始；落实到当前工程时，先用已校准可运行的 `iic/speech_charctc_kws_phone-xiaoyun`
- 这里关于 `fsmn-kws` 与 `fsmn-kws_mt` 的细粒度排序，当前更多是基于模块定位而不是本仓库实测，需要后续 A/B

2026-04-14 本仓库追加实测 caveat：

- 当前本机 `FunASR 1.3.1` runtime 在 worker preload 阶段直接拒绝了短模型名 `fsmn-kws`，报错是 `fsmn-kws is not registered`
- 当前 worker 已校准的可运行替代是 `iic/speech_charctc_kws_phone-xiaoyun`
- 这个模型在 worker 里不能只靠 `generate(..., keywords=...)` 临时传参；必须在 `AutoModel(...)` 初始化时同时传入 `keywords` 和 `output_dir`，否则会报 `writer` 相关错误
- 这不改变“KWS 方向值得优先投入”的判断，但意味着本项目在真正把 KWS 打开进主链路时，应先以 `iic/speech_charctc_kws_phone-xiaoyun` 这条已验证路径作为工程基线，再继续寻找更通用的 KWS model id

## 6. 说话人 / speaker-aware 能力

### `cam++` 以及 3D-Speaker 的 `ERes2Net` / `ERes2NetV2`

FunASR 官方 model zoo 把 `cam++` 作为代表性说话人模型列出；而 3D-Speaker 官方 README 说明更近的 `ERes2Net` / `ERes2NetV2` 在说话人验证 benchmark 上有更强表现。

对本项目的意义：

- 第一阶段 demo 不是最优先模块
- 但如果后续想做“家庭成员识别”“多说话人设备侧交互”或更稳的回声/串音判别，这块会非常重要

项目内判断：

- 现在不建议优先投入 speaker 模型接入
- 真要做 speaker-aware，效果优先更应直接看 3D-Speaker 里的 `ERes2NetV2`，而不是只停留在 `cam++`

## 7. 情绪 / 音频事件

### `SenseVoiceSmall` 与 `emotion2vec+ large`

`SenseVoice` 本身就包含 SER / AED；`emotion2vec+` 官方 repo 则是更专门的 speech emotion foundation 表征模型。

对本项目的意义：

- 如果后续想让 agent 的打断策略、确认策略、TTS 语气随用户状态变化，这块会有价值
- 但在第一阶段，优先级仍低于 streaming ASR、VAD、KWS、TTS

项目内判断：

- 第一阶段先不把情绪模型放进主链路
- 如果第二阶段开始做“生动性”和“对话状态感知”，优先复用 `SenseVoiceSmall` 已有输出；要做更细分类再看 `emotion2vec+ large`

## 8. TTS（不属于 FunASR，但属于本项目最终模型组合）

### `Fun-CosyVoice3-0.5B-2512`

CosyVoice 官方 README 直接写明当前推荐模型，并强调：

- streaming 首包可到约 150ms
- 在内容一致性、音色相似度、自然度上相比前代更强
- 继续保留多语言、跨语种、zero-shot 等优势

对本项目的意义：

- 如果以最终 demo 效果为优先，它是当前本项目 TTS 侧最值得优先 benchmark 的候选
- 当前项目已有 `cosyvoice_http` 边界，因此模型升级不需要改变公网协议

### `CosyVoice2-0.5B`

它比当前仓库已验证的 `CosyVoice-300M-SFT` 更强，也保留 streaming 和 zero-shot 路径。

项目内判断：

- 如果要一个“比 300M-SFT 明显更强、同时兼容风险较可控”的过渡方案，可以先试 `CosyVoice2-0.5B`
- 如果纯看效果上限，优先 benchmark `Fun-CosyVoice3-0.5B-2512`

## 本项目最适合的模型组合：按推荐顺序给结论

## 方案 A：当前项目主推荐，兼顾效果与可落地性

- Wake word：`iic/speech_charctc_kws_phone-xiaoyun`（当前工程已校准可运行的 worker baseline）
- Acoustic VAD：`fsmn-vad`
- Online preview / partial：`Paraformer-large-online`
- Final-ASR：`SenseVoiceSmall`
- TTS：`Fun-CosyVoice3-0.5B-2512`
  - 若当前 FastAPI runtime 兼容性或显存预算需要更稳过渡，则先 `CosyVoice2-0.5B`

为什么这是当前项目的主推荐：

- 它直接补中了当前最痛的四个短板：wake word、VAD、真 partial、自然 TTS
- `SenseVoiceSmall` 已经在仓库里落地，final-ASR 迁移成本最低
- `Paraformer-large-online + final correction` 很符合 FunASR 官方 2pass 思路，也最符合当前仓库对 hidden preview、speech planner、barge-in 的演进方向
- 这个组合对“流畅性、自然性、生动性”的综合收益，比单纯把 `SenseVoiceSmall` 替换成另一个 batch 模型更大

一句话总结：

> 如果你现在就要把一期 demo 做得更像“能实时对话的语音 agent”，最优先该做的是把单模型伪流式改成 `KWS + VAD + online partial + final correction` 的多模块组合，而不是继续把所有职责压在一个 `SenseVoiceSmall` worker 上。

## 方案 B：最终识别质量更激进的上限方案

- Wake word：`iic/speech_charctc_kws_phone-xiaoyun`（当前工程已校准可运行的 worker baseline）
- Acoustic VAD：`fsmn-vad`
- Online preview / partial：`Paraformer-large-online`
- Final-ASR：`Fun-ASR-Nano-2512`
- TTS：`Fun-CosyVoice3-0.5B-2512`

这个方案什么时候更值得：

- 你已经接受 worker 侧会做一轮更大改造
- 你更在意最终 transcript 质量、方言噪声鲁棒性、远场效果上限
- 你愿意把 `Fun-ASR-Nano-2512` 当成新一代 final-ASR 做重点 benchmark

项目内判断：

- 如果只讨论“最终识别质量上限”，它比方案 A 更有冲击力
- 但对当前仓库而言，它的 integration 风险和改造量也更高
- 所以它更适合作为方案 A 稳住以后再切入的第二阶段 benchmark 目标

## 方案 C：最小改动但仍有明显收益的过渡方案

- 继续保留 `SenseVoiceSmall`
- 把 `energy` endpoint 升级为 `fsmn-vad`
- 加独立 `iic/speech_charctc_kws_phone-xiaoyun` 这条已验证 KWS 路径
- TTS 从 `CosyVoice-300M-SFT` 升级到 `CosyVoice2-0.5B` 或 `Fun-CosyVoice3-0.5B-2512`

这个方案的定位：

- 它不是效果上限最高
- 但它能用最小成本把当前“唤醒词短句 + endpoint + TTS 自然度”这三块先拉起来

## 我对下一步实施顺序的建议

1. 先接 `iic/speech_charctc_kws_phone-xiaoyun` 这条已验证 KWS path
   - 目的：立刻把“唤醒词是否命中”从 ASR transcript 中解耦
2. 再接 `fsmn-vad`
   - 目的：把当前声学 endpoint 从 heuristic 升级成模型
3. 再把 preview 换成真 streaming online ASR
   - 优先看 `Paraformer-large-online`
4. 保留 `SenseVoiceSmall` 作为 final-ASR 做第一轮 2pass
   - 先吃到工程兼容性收益
5. 再 benchmark `Fun-ASR-Nano-2512`
   - 看它是否值得替掉 final-ASR
6. 最后升级 TTS 到 `Fun-CosyVoice3-0.5B-2512`
   - 在 speech planner 已经落地的前提下，收益会非常直接

## 最重要的判断

对于当前 `agent-server`，最不应该继续走的路是：

- 继续把 wake word、endpoint、preview、final-ASR 都压在一个 `SenseVoiceSmall` worker 上
- 继续把声学收尾主要押在 `energy` heuristic 上
- 继续把 preview 做成“缓冲音频反复重跑 batch 模型”

对于当前 `agent-server`，最值得优先投入的模型化升级是：

- 已校准 KWS baseline：`iic/speech_charctc_kws_phone-xiaoyun`
- `fsmn-vad`
- `Paraformer-large-online`
- `SenseVoiceSmall` 或 `Fun-ASR-Nano-2512` 作为 final-ASR
- `Fun-CosyVoice3-0.5B-2512`

## Sources

- FunASR README: https://github.com/modelscope/FunASR
- FunASR runtime README: https://github.com/modelscope/FunASR/tree/main/runtime
- SenseVoice README: https://github.com/FunAudioLLM/SenseVoice
- SenseVoiceSmall model card: https://huggingface.co/FunAudioLLM/SenseVoiceSmall
- Fun-ASR README: https://github.com/FunAudioLLM/Fun-ASR
- CosyVoice README: https://github.com/FunAudioLLM/CosyVoice
- 3D-Speaker README: https://github.com/modelscope/3D-Speaker
- emotion2vec README: https://github.com/ddlBoJack/emotion2vec
