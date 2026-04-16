# Server-Primary Hybrid：建连后端侧最小能力与 interruption 分层研究（2026-04-16）

## 文档性质

- 本文是研究材料，不是最终实施方案。
- 目标是继续回答两个问题：
  1. 建连后，如果 turn-taking 由服务侧主导，端侧还必须保留哪些最小但必要的能力？
  2. 服务侧如何把 speaking 期间的新输入区分为 `backchannel / duck_only / hard_interrupt / ignore`？
- 当前项目仍处于研究阶段，因此本文优先追求“更像现代优秀实时语音系统”的边界，而不是先做过度架构化设计。

## 先给结论

如果要走 `server decides, client protects` 这条路，那么建连后：

- **端侧不应继续担任 turn accept / final endpoint / hard interrupt 的主裁判**
- 但 **端侧绝不能退化成纯麦克风采集器**

最小但必要的端侧能力，至少包括：

1. 音频前端与硬件近身能力
2. 本地播放控制与急停
3. 播放进度与播放状态回传
4. 轻量 speech hint / 本地 VAD 作为反射层和兜底层
5. 弱网/断网/超时 fallback
6. 手动控制入口

而服务侧真正值得主导的是：

- preview / partial
- endpoint candidate
- turn accept
- interruption arbitration
- response start
- heard-text / truncate 一致性

更进一步地说：

- **如果没有端侧播放急停与播放进度回传，那么即使服务侧做了更聪明的 interruption policy，也很难真正落成“像人”的体验。**

## 一、为什么建连后端侧仍然必须保留一部分能力

### 1. 端侧最接近真实声学现场

无论服务侧多强，端侧都仍然最接近：

- 扬声器参考信号
- 麦克风原始输入
- AEC / NS / AGC 的实时状态
- 当前本地播放缓冲是否还有未播音频
- 用户是否刚刚明显开口

这意味着：

- 服务侧适合做更高层的会话裁决
- 端侧适合做最贴近设备物理现场的反射动作

Apple 的公开研究长期体现的是这个边界：低功耗唤醒、第一阶段 trigger、设备近身前处理都明显偏端侧；但更高层的有效输入判定会逐渐进入多阶段、多信号决策。参考：

- Voice Trigger System for Siri：<https://machinelearning.apple.com/research/voice-trigger>
- Device-directed speech detection：<https://machinelearning.apple.com/research/device-directed>

### 2. 真正的“低延迟打断体感”不能只等服务侧 round-trip

即使服务侧是最终裁判，端侧仍需要在用户明显开口时有本地反射，例如：

- 立刻 soft duck 当前播放
- 暂停继续扩张本地播放缓冲
- 给出“我注意到你在说话”的 UI/灯效反馈

否则，用户会感到：

- 自己明明已经插话
- 设备却还继续说了半拍甚至一整拍

LiveKit 的 turn detection 文档实际上也体现了这个思路：即使使用 STT endpointing，仍建议保留 VAD 来提高 interruption responsiveness。参考：

- LiveKit Turns：<https://docs.livekit.io/agents/logic/turns/>

### 3. 服务侧若不知道“实际播到了哪里”，就很难做好 heard-text 和 interruption 一致性

这是对当前项目尤其关键的一点。

当前仓库已经明确记录：

- `docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md:42` 提到，当前播放进度仍有启发式成分，因为还没有覆盖所有 adapter 的精确 playout acknowledgement
- `docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md:46` 也明确指出后续要提升 playout progress fidelity

这意味着：

- 如果服务侧决定 hard interrupt
- 但并不知道端侧到底已经播了多少
- 那么它对“用户实际听到了什么”的记忆、截断、追问、继续说，都可能失真

OpenAI Realtime 官方文档也体现了类似边界：WebRTC / SIP 这类由服务端掌握缓冲的路径，服务端更清楚播到了哪里；而 WebSocket 路径下，客户端需要显式处理本地播放中断和 truncate 协调。参考：

- OpenAI Realtime conversations：<https://platform.openai.com/docs/guides/realtime-conversations>

### 4. `speech start` 更适合做快速反射信号，而不是 `hard_interrupt` 终判

这一点是本轮并行资料研究里最值得强调的补充。

`speech start` 最多说明：

- 检测到了近端语音样信号
- 当前 speaking 期间可能有人开始说话

但它天然分不清：

- 附和词
- 短笑声 / 咳嗽 / 感叹
- 背景旁谈
- 自播音残留 / echo leakage
- 犹豫开头
- 真正要求夺回话轮的新请求或纠正

因此，更合理的链路不是：

- `speech_start -> 直接 hard_interrupt`

而是：

- `speech_start -> previewing -> duck_only 或继续观察 -> hard_interrupt / backchannel / ignore`

这和 OpenAI、LiveKit、Pipecat、Google/Amazon 的公开实践方向是一致的：声学 onset 适合做快反应，但最终打断判定需要更高层信号。

## 二、建连后端侧“最小但必要”的能力边界

下面不是最终定案，而是更适合当前项目讨论的边界拆法。

### A. 必须保留：音频前端与硬件近身能力

#### 1. AEC / NS / AGC / beamforming / clipping protection

这些能力应主要留在端侧，因为它们依赖：

- 设备播放参考信号
- 麦克风阵列
- 本地声学环境
- 极低延迟前处理

如果把这些也交给服务侧，风险是：

- 自播音误触发 speech start
- 环境噪声被错当用户插话
- 远场识别显著恶化
- interruption 分类被严重污染

#### 2. 播放缓冲、音量、静音、紧急 stop

建连后即便 response timing 由服务侧主导，端侧也必须拥有：

- 立即停播
- 清空未播缓冲
- 立刻 duck / unduck
- 本地 mute / unmute

这不是 turn decision，而是本地执行控制。

### B. 必须保留：端侧反射层

#### 3. 本地轻量 speech hint / VAD

这里的关键是：

- 它不再负责最终 accept
- 但它仍应该负责：
  - 最早期“疑似用户开口”的本地反射
  - 本地 soft duck 触发
  - 灯效 / UI 即时变化
  - 弱网时的 fallback 参考
  - 可选的上行 hint

更直接地说：

- **VAD 不应该继续当主裁判**
- **但它仍然应该当反射层传感器**

如果再进一步压缩成一句工程边界，就是：

- **local VAD = reflex sensor**
- **server arbitration = turn judge**

#### 4. 短时 preroll / 环形缓冲

如果端侧不是持续把所有原始音频上送，而是仍然有轻量 gating，那么它至少应保留短时 preroll，例如最近 200-400ms 的音频。

否则：

- 用户开口的前几个音素很容易被吃掉
- 服务侧 preview / endpointing 的质量会直接下降

这一点在 server-primary 方案里经常被低估。

### C. 必须保留：端侧向服务侧回传事实状态

#### 5. 播放进度 ack / playback head telemetry

这是我认为当前最不该忽略的一项。

如果要让服务侧真正成为 turn / interruption 的主裁判，端侧至少要回传：

- playback started
- playback stopped
- approximate played duration / played samples
- local buffer cleared
- ducking active / inactive

这样服务侧才更可能准确知道：

- 用户到底听到了多少
- 何时应该 truncate
- 中断后下一句该从哪里接
- memory 写回应该落“生成全文”还是“实际听到的 heard text”

对于当前 `agent-server` 的 WebSocket + binary audio 路径，这一点比 WebRTC 托管缓冲模式更重要。

#### 6. 网络与会话健康状态

端侧至少应向服务侧提供或本地维护：

- 当前连接是否抖动
- 最近上行是否堆积
- 最近下行是否堆积
- 当前是否进入本地 fallback 模式

因为这些状态会直接影响：

- 是否还适合做激进 server-endpoint
- 是否适合等待服务侧 interruption 判决
- 是否应退回到保守半双工

### D. 必须保留：端侧兜底与手动 override

#### 7. 手动 stop / push-to-talk / force commit / clear

无论多智能，端侧都必须保留最小手动控制：

- 手动停止播放
- 手动打断
- 手动结束当前输入
- 会话异常时 clear/reset

这与 AssemblyAI 的 `ForceEndpoint`、Gemini Live 关闭自动 activity detection 后让客户端显式发 start/end 的做法，本质上属于同一类兜底思想。参考：

- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>
- Gemini Live API：<https://ai.google.dev/api/live>

### E. 不建议继续由端侧主导的能力

以下能力如果仍长期放在端侧主导，会显著限制体验上限：

- final endpoint / turn accept
- hard interrupt 终裁
- backchannel vs real interruption 的语义分类
- heard-text 的最终真相定义
- response start 的主时机控制

## 三、为什么 interruption 不能等价于 “检测到 speech start”

这是第二个研究重点的核心。

### 1. speaking 期间出现新声音，不一定代表“请你闭嘴”

现实对话里常见的重叠输入包括：

- 嗯
- 对
- 好的
- 然后呢
- 等一下
- 不是，我的意思是
- 对旁边人的说话
- 笑声 / 叹气 / 咳嗽
- 设备自身 TTS 泄露

这些输入里：

- 有些应 `ignore`
- 有些应 `backchannel`
- 有些应 `duck_only`
- 有些才是真正的 `hard_interrupt`

所以 speech start 最多是“进入 preview / arbitration”的入口，而不是最终结论。

### 2. 是否该打断，取决于当前 output 状态

相同一句“嗯”，在不同场景意义完全不同：

- 如果系统刚问完“这样可以吗？”，用户说“嗯”，可能是确认
- 如果系统正在长篇解释，用户说“嗯”，更可能是 backchannel
- 如果用户说“停一下”“不是”“你先别说”，则更像 hard interrupt
- 如果系统正在播最后半句，而用户发出一个短促不确定音，更适合先 duck 再观察

因此 interruption 必须依赖：

- 当前 output 是否在 speaking
- 已播到哪里
- 当前句法是否将近收束
- output 还剩多长

### 3. 是否“面向设备”本身也是变量

Apple 的 device-directed speech detection 与 Amazon 的相关研究都说明：

- 不是所有被麦克风采到的语音都在对设备说话
- 在远场、家庭、多人的环境里，这一点尤其关键

参考：

- Apple Device-directed：<https://machinelearning.apple.com/research/device-directed>
- Amazon Device-Directed Utterance Detection：<https://www.amazon.science/publications/device-directed-utterance-detection>

## 四、把 speaking 期间新输入分成四类，更合理的判断信号有哪些

下面不是最终算法，而是研究阶段最值得讨论的多信号集合。

### A. 声学信号

- onset energy / VAD speech probability
- 发声持续时长
- 与当前 TTS 播放的重叠程度
- 近端/远端能量比
- AEC residual / echo likelihood
- prosody：是短促附和、拉长打断、还是完整句起头
- 说话人方向 / 说话人切换（若设备具备阵列）

这些信号更适合做：

- `ignore` 初筛
- `duck_only` 快速进入
- `hard_interrupt` 的第一阶段候选发现

### B. ASR / partial 信号

- preview partial 是否稳定
- stable prefix 长度
- 识别文本是否只是一两个附和词
- 是否出现明确纠正 / 中止词：
  - 停
  - 等一下
  - 不是
  - 你先别说
  - 错了
- 是否出现完整问句 / 新指令雏形

这些信号特别适合区分：

- `backchannel` vs `hard_interrupt`
- `duck_only` vs `hard_interrupt`

而且从研究阶段角度看，`duck_only` 很可能是最高杠杆的一层：

- 它允许系统在证据还不够硬时先礼貌“让一点路”
- 避免“一听到人声就硬停”的粗糙体验
- 也避免“必须等 final ASR 才敢动”的拖沓体验

Google 的 endpointing 与 prefetching 研究、Deepgram Flux 的 `EagerEndOfTurn / TurnResumed`、AssemblyAI 的 semantic + acoustic turn detection，本质都在支持这一点：仅靠声学门限不够，partial / semantic 必须进入主链路。参考：

- Google endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Google prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Deepgram Flux Agent：<https://developers.deepgram.com/docs/flux/agent>
- Deepgram Eager EOT：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>

### C. 当前 output 与对话上下文信号

- 当前 output 是否刚开始、已中段、还是即将结束
- 当前 output 的意图类型：提问、解释、确认、追问
- 是否已经播出关键内容
- 当前对话中用户是否常打断、是否正在纠错
- 系统是否刚询问确认

这些信号非常适合做：

- `backchannel` 的保守识别
- `hard_interrupt` 的时机判别
- `duck_only` 的持续时间控制

### D. device-directedness / speaker-intent 信号

- 是否在对设备说话
- 是否像对旁边人说话
- 是否只是笑、叹气、背景音

Apple / Amazon 的公开研究都在提醒：

- 这一步缺失时，系统会过度敏感
- 结果就是“什么都当打断”

## 五、四类 interruption policy 的更合理语义边界

### 1. `ignore`

典型情况：

- 很短的非语言噪音
- 低置信背景声
- 明显 echo / self-playback leakage
- 非 device-directed speech

系统行为：

- 不改变当前输出
- 最多只更新极轻量的内部统计

### 2. `backchannel`

典型情况：

- 嗯 / 对 / 好 / 是的
- 很短、低 takeover intent 的附和
- 不包含明确改口、反对、停止、提问意图

系统行为：

- 当前输出继续
- 可以轻微适配 prosody 或后续简短 acknowledge
- 不立即切断当前说话

### 3. `duck_only`

典型情况：

- 用户明显在说话，但意图还不够明确
- partial 尚不稳定
- 可能是想打断，也可能只是附和或想插一句

系统行为：

- 先降低当前播放音量
- 持续 preview 新输入
- 在更高置信度出来前不贸然 hard stop

这一步对“像人”非常重要，因为真人在被别人似乎要插话时，通常会先收小声、放慢、观察，而不是立刻瞬间闭嘴。

从当前项目研究阶段看，`duck_only` 也是最值得优先讨论清楚的一层，因为它恰好位于两种糟糕体验的中间：

- 一边是“完全不停，显得听不见用户”
- 一边是“过度敏感，一有声音就硬打断”

### 4. `hard_interrupt`

典型情况：

- 出现明确 stop/correct/new-request lexical cue
- preview 已稳定成完整新意图
- device-directedness 较高
- 当前 output 尚未接近自然收束，继续说会显得明显不礼貌

系统行为：

- 立刻停止当前输出
- 清理未播缓冲
- 记住已 heard 的边界
- 转入新 turn 的 accept 路径

## 六、对当前 agent-server 最值得借鉴的研究边界

结合当前仓库状态与项目目标，我认为最值得继续沿着以下边界讨论：

### A. 服务侧拥有唯一会话裁决权

尤其是在：

- endpoint candidate
- turn accept
- interruption classification
- response start
- heard-text consistency

这几项上，应尽量避免“双主脑”。

### B. 端侧必须保留“反射 + 事实回传 + 兜底”三类能力

这三类能力缺一不可：

1. 反射：本地 soft duck、UI、轻量 VAD hint
2. 事实回传：playback progress、buffer clear、render state
3. 兜底：manual stop、force commit、fallback half-duplex

### C. interruption 推荐走“两阶段而不是一刀切”

当前研究阶段很适合讨论的，不是上一个特别重的统一分类器，而是：

- 第一阶段：廉价快速 gate
  - 声学 onset
  - 本地 hint
  - speaking 期间检测到新近端语音
- 第二阶段：服务侧 richer arbitration
  - partial / stable prefix
  - lexical cue
  - output progress
  - device-directedness
  - 对话上下文

把它再压缩成更贴近当前项目的渐进链路，可以写成：

- `speech_start`
- `input_state=previewing`
- 必要时先 `duck_only`
- 150-600ms 内等待更稳定的 partial / semantic evidence
- 再决定 `ignore / backchannel / hard_interrupt`

这样既能保住实时性，又能避免“一有声音就停”的笨拙体验。

### D. 当前项目最容易被忽略但最值的一个点：播放进度回传

如果后面真的要把 interruption 与 heard-text 做得像样，**播放进度 ack / playback telemetry 很可能是比再换一个更强模型还高 ROI 的基础能力**。

因为它直接决定：

- 用户实际听到了什么
- 被打断时服务侧应该如何记忆、如何续说、如何修正
- interruption policy 到底有没有在体感上真正生效

## 七、当前仍不建议急于收敛的地方

- 暂不急于把这两部分直接固化成协议定案
- 暂不急于把所有 interruption policy 外显成公网 wire event
- 暂不急于把端侧做得非常重
- 暂不急于直接引入很重的端侧语义模型

## 参考资料

- OpenAI Realtime conversations：<https://platform.openai.com/docs/guides/realtime-conversations>
- OpenAI Realtime VAD：<https://platform.openai.com/docs/guides/realtime-vad>
- OpenAI Realtime model capabilities：<https://platform.openai.com/docs/guides/realtime-model-capabilities>
- Gemini Live API：<https://ai.google.dev/api/live>
- Gemini Live capabilities：<https://ai.google.dev/gemini-api/docs/live-api/capabilities>
- LiveKit Turns：<https://docs.livekit.io/agents/build/turns/>
- Pipecat User Turn Strategies：<https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies>
- Pipecat Interruption Strategies：<https://docs.pipecat.ai/api-reference/server/utilities/turn-management/interruption-strategies>
- Deepgram Flux Agent：<https://developers.deepgram.com/docs/flux/agent>
- Deepgram Eager EOT：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>
- FunASR：<https://github.com/modelscope/FunASR>
- Google unified endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Google prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Google Project Astra：<https://deepmind.google/technologies/project-astra/>
- Amazon endpointing：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
- Amazon contextual acoustic barge-in：<https://www.amazon.science/publications/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems>
- Amazon device-directed detection：<https://www.amazon.science/publications/device-directed-utterance-detection>
- Apple Voice Trigger System for Siri：<https://machinelearning.apple.com/research/voice-trigger>
- Apple Device-directed speech detection：<https://machinelearning.apple.com/research/device-directed>
- Apple Talking Turns：<https://machinelearning.apple.com/research/talking-turns>
