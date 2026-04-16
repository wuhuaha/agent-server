# 实时语音交互体验研究地图（2026-04-16）

## 文档性质

- 本文是研究阶段讨论材料，不是最终实施方案。
- 目标是为后续“充分讨论后再决策”提供一个共同分析框架。
- 当前优先关注交互效果，不做过度架构化设计。

## 研究目标

围绕四个体验目标建立统一分析框架：

- 流畅性：用户是否很快感到“系统在听、在懂、在回应”。
- 自然性：停顿、打断、衔接、措辞、韵律是否接近真人对话。
- 人性化：是否会在不确定时澄清、是否会用适合语境的语气与节奏。
- 智能性：是否能利用上下文、识别纠正/续问/附和，并减少机械回复。

## 一个核心判断

对实时语音系统而言，用户主观上的“快”并不等于最终 ASR 文本出现得早。

更接近现代优秀系统的拆法是四段：

1. 系统知道你开始说了。
2. 系统大致知道你在说什么。
3. 系统知道你说完了，或足够有把握可以开始准备回复。
4. 系统开始给出可听、可感知、像真人的回应。

如果只优化第 3 和第 4 段，而忽略第 1 和第 2 段，用户仍会觉得“它反应慢”。

## 与当前项目最相关的外部结论

### 1. Preview / partial 应被视为主链路信号

- OpenAI Realtime 官方会话模型把 `speech_started`、进行中的 transcript delta、以及 audio delta 都视为会话主事件，而不是 debug 附属物。
- Google 的 `Low Latency Speech Recognition using End-to-End Prefetching` 指出，可以在 final 识别前依据 partial hypothesis 提前触发下游处理，以换取系统级时延收益。
- 对当前项目而言，这意味着 preview partial 的价值不只是“屏幕上早点出字”，而是：
  - 更早建立用户信心
  - 更早触发 turn 预测
  - 更早做 LLM/TTS prewarm
  - 更早形成端侧/服务侧对同一轮输入的共识

### 2. Turn-taking 不能只靠静音阈值

- Google 的 `Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems` 说明 endpointing 与 ASR 共享信息可以显著降低 endpoint latency，并避免只依赖外置 endpointer 的局限。
- Amazon 的 `Accurate Endpointing with Expected Pause Duration` 进一步指出，关键在于区分句中停顿与句末停顿。
- 对当前项目而言，这意味着：
  - 只靠 VAD stop 很难让中文对话自然
  - endpoint 判断至少应结合停顿、partial 稳定度、句法完成度、语义完成感

### 3. 全双工的本质不是“必须原生语音大模型”

- OpenAI Realtime、Google Project Astra 的公开实践都体现出：输入持续进行、输出持续进行、两者可被仲裁，才是更接近真人对话的核心。
- 这说明当前阶段真正值得追求的是“输入轨/输出轨并存 + interruption arbitration”，而不是先追求完整 end-to-end native-audio 大基建。

### 4. 自然感很多来自时机与韵律，而不只是文本正确

- OpenAI 的 Realtime prompting guide 明确强调：短而清晰的规则、对 unclear audio 的明确策略、以及避免机械重复，会显著影响语音 agent 听起来是否自然。
- Apple 在可控 TTS / prosody 研究中说明，pitch、duration、energy、spectral tilt 等因素对“像人”非常关键。
- 这意味着“自然性”至少有三层：
  - 回复内容像不像人
  - 什么时候开口像不像人
  - 说出来的节奏和情绪像不像人

### 5. 真实系统往往采用多阶段、多策略，而不是单一阈值

- Apple 的 `Voice Trigger System for Siri` 展示了典型多阶段设计：first-pass 高召回、second-pass 高精度、后续再结合 directed-speech / false-trigger mitigation。
- 对当前项目的启发不是照搬其硬件架构，而是采用其原则：
  - 先快速发现可能有效的信号
  - 再用更强、更贵、更稳的策略做确认
  - 不把所有判断压在一个模块或一个阈值上

### 6. “会不会在合适的时候附和/停顿/让出话轮”已成为现代系统的重要评估维度

- Apple 2025 的 `Talking Turns` 指出，很多现有 spoken dialogue systems 在 turn-taking 上仍存在显著不足，例如不会恰当地插入 backchannel、会过于激进地打断、也可能不知道何时开口。
- 这对当前项目非常重要，因为“像人”很大一部分正来自这些微行为，而不是大模型分数本身。

## 当前项目在研究视角下的主要体验瓶颈

结合当前本地分析，可先把问题抽象成以下几个层次：

### A. 感知时延仍偏大

- 系统内部已存在 preview partial，但用户尚不能稳定、及时地感知到。
- 因此主观体验上更像“系统在长时间沉默”。

### B. 输入结束判断仍不够像真人

- 若 turn accept 主要依赖静音或 client commit，会导致：
  - 抢话不自然
  - 等待过长
  - 句中停顿被误判

### C. 输出开始太晚

- 如果必须等完整回复收口后再起播，哪怕 LLM / TTS 本身质量不错，仍会让人觉得“机器人味很重”。

### D. interruption policy 不够细腻

- 若把所有用户发声都当 hard interrupt，系统会显得笨拙。
- 更接近人类对话的系统，需要区分附和、轻微插入、真正改口、真正打断。

### E. 自然性不只是模型问题，也是 policy 问题

- 即使 ASR 和 TTS 都升级，如果对 unclear audio、确认、追问、重复表达没有对话策略，最终听感仍可能机械。

## 当前阶段更值得讨论的方向

从 ROI 看，当前更值得深入讨论的是：

1. 如何把 preview partial 从“内部信号”升级为“用户可感知、编排可利用的主链路信号”。
2. 如何把 endpoint / turn-taking 从单一阈值升级为多信号仲裁，但又不引入过重系统复杂度。
3. 如何把“边生成边说”的起播时机做对，而不是简单追求越早越好。
4. 如何把 interruption policy 做细，让 backchannel、duck、hard interrupt 有差异。
5. 如何通过 prompt policy + prosody policy 提升“像人”和“有温度”的体感。

## 当前阶段暂不急于做的事

- 暂不急于收敛为完整实施路线图。
- 暂不急于把整条链路替换成单一 native-audio 大模型主路径。
- 暂不急于做重型 personalization / 在线自学习闭环。
- 暂不急于为未来远期能力提前设计过深的架构层次。

## 下一轮讨论建议议题

建议下一轮优先深挖下列三者之一：

1. `preview partial` 如何成为主链路信号，而不是只做显示字幕。
2. `turn-taking / interruption` 怎样才能更像真人，而不是纯 VAD 逻辑。
3. `早起播 + prosody` 怎样在“更快”和“更自然”之间取得好平衡。

## 参考资料

- FunASR 官方仓库：<https://github.com/modelscope/FunASR>
- Paraformer：<https://arxiv.org/abs/2206.08317>
- FunAudioLLM：<https://fun-audio-llm.github.io/pdf/FunAudioLLM.pdf>
- OpenAI Realtime conversations：<https://developers.openai.com/api/docs/guides/realtime-conversations>
- OpenAI Realtime prompting guide：<https://cdn.openai.com/API/docs/realtime-prompting-guide.pdf>
- Google endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Google prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Google DeepMind Project Astra：<https://deepmind.google/models/project-astra/>
- Amazon endpointing：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
- Amazon predictive ASR：<https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants>
- Apple Siri Voice Trigger：<https://machinelearning.apple.com/research/voice-trigger>
- Apple Talking Turns：<https://machinelearning.apple.com/research/talking-turns>
- Apple controllable TTS：<https://machinelearning.apple.com/research/controllable-neural-text-to-speech-synthesis>
