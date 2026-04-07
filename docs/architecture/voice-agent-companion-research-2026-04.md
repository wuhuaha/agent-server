# Voice Agent Companion Research (2026-04-04)

## Purpose

This note captures a focused review of current open-source voice-agent systems and adjacent agent frameworks to answer one practical question:

How do modern voice agents become more like a reliable companion instead of a thin speech wrapper around tools?

The goal is not to chase every new model. The goal is to identify the technical capabilities that most clearly improve perceived intelligence, continuity, and naturalness for a household voice assistant.

## Scope

This review focuses on:

- open-source or openly documented voice stacks
- realtime speech systems
- multimodal or tool-capable agent runtimes
- home-assistant or always-available companion patterns

It does not assume that end-to-end speech-to-speech models should replace every existing `ASR -> agent -> TTS` pipeline. For home control, reliability and controllability still matter more than novelty.

## Executive Summary

The strongest current systems do not feel more intelligent only because they use a larger LLM. They feel more companion-like because they combine six capabilities into one realtime session system:

1. richer speech understanding than plain transcription
2. context-aware turn taking and interruption handling
3. low-latency expressive spoken output
4. session memory plus long-term preference memory
5. safe, unified tool use across external systems
6. hybrid control policy that keeps deterministic actions separate from open-ended conversation

The open-source ecosystem is converging on this architecture even when projects choose different transports, models, or runtime languages.

## What "Companion-Like" Actually Requires

### 1. Speech Understanding Must Capture More Than Words

Traditional ASR gives text. Modern voice agents need speech understanding metadata:

- language
- emotion
- speaking style
- audio events
- speaker identity or diarization
- wake-word or room context

This is one reason `SenseVoice` matters. It is positioned as a speech foundation model that combines multilingual ASR with emotion recognition and audio event understanding instead of treating ASR as text extraction only.

For a household assistant, this changes behavior in useful ways:

- "turn on the lights" can be routed differently if the speaker sounds urgent
- "what happened" after a loud crash can be interpreted as a situational query, not generic chat
- follow-up prompts can be calmer or shorter when the user sounds tired or frustrated

Relevant references:

- [FunASR](https://github.com/modelscope/FunASR)
- [SenseVoice](https://github.com/FunAudioLLM/SenseVoice)

### 2. Turn Taking Is Now A Core Product Feature

Older voice assistants were effectively push-to-talk systems with silence timeout. Modern realtime frameworks treat turn detection, barge-in, and half-duplex/full-duplex tradeoffs as first-class runtime behavior.

This is visible in:

- `LiveKit Agents` turn detection and session orchestration
- `Pipecat` interruption-focused streaming pipelines
- `TEN` realtime agent graphs
- `Moshi` research toward native spoken dialogue with overlap handling

The user experience impact is large:

- the assistant should not cut in the moment a user pauses to think
- the assistant should stop speaking immediately when the user barges in
- the assistant should support short acknowledgements and quick corrections without forcing a full restart of the turn

Relevant references:

- [LiveKit Agents](https://docs.livekit.io/agents/)
- [Pipecat](https://docs.pipecat.ai/overview/introduction)
- [TEN Framework](https://github.com/TEN-framework/ten-framework)
- [Moshi](https://github.com/kyutai-labs/moshi)

### 3. Spoken Output Quality Is About Timing, Prosody, And Streamability

A companion does not sound like a batch TTS job. The system needs:

- low `time-to-first-audio`
- incremental response streaming
- sentence-aware chunking
- controllable pace, style, and affect
- graceful cancellation and restart

`CosyVoice` is a strong example because it targets streaming speech generation and controllable voice behavior instead of only offline TTS. `Sesame CSM` and other conversational speech models are pushing further toward dialogue-native speech generation, where turn timing and prosody come from dialogue context rather than plain text punctuation.

Relevant references:

- [CosyVoice](https://github.com/FunAudioLLM/CosyVoice)
- [Sesame CSM](https://github.com/SesameAILabs/csm)

### 4. Memory Must Be Layered

Companion-like behavior depends on memory, but not one undifferentiated memory blob.

The most useful practical split is:

- hot session memory: what is happening in the current exchange
- recent episodic memory: what the user asked recently, in this room, on this device
- durable preference memory: names, routines, preferred wording, recurring reminders
- derived memory: summarized patterns extracted offline from interaction history

Open agent runtimes increasingly formalize this split:

- `Pipecat` uses context aggregators and pipeline state
- `LiveKit` supports session-level context and external data injection
- `LangGraph` and `LangMem` focus on durable long-term memory patterns and background consolidation

The important lesson is that memory should not be implemented as "append every transcript forever". Retrieval, summarization, expiry, and editability matter.

Relevant references:

- [Pipecat Context Management](https://docs.pipecat.ai/pipecat/learn/context-management)
- [LiveKit Sessions](https://docs.livekit.io/agents/logic/sessions/)
- [LiveKit External Data](https://docs.livekit.io/agents/logic/external-data/)
- [LangGraph](https://langchain-ai.github.io/langgraph/)
- [LangMem](https://langchain-ai.github.io/langmem/)

### 5. Tool Use Must Be Unified, Not Embedded Ad Hoc Into Each Transport

The strongest agent systems are standardizing around a separate tool layer instead of hardwiring every capability into the voice loop.

This shows up in:

- `LiveKit` tools and handoff model
- `Pipecat` function calling
- `MCP` as a common contract for tools, resources, and prompts
- home platforms that treat device control, search, RAG, and messaging as distinct capabilities

This matters for "companion" behavior because the assistant must be able to:

- answer
- control
- check state
- look things up
- remember
- call external services

without collapsing transport adapters into orchestration code.

Relevant references:

- [LiveKit Tools](https://docs.livekit.io/agents/logic/tools/)
- [Pipecat Function Calling](https://docs.pipecat.ai/pipecat/learn/function-calling)
- [Model Context Protocol](https://modelcontextprotocol.io/docs/getting-started/intro)

### 6. The Best Product Systems Use A Hybrid Policy Model

Home-assistant platforms show a consistent pattern:

- deterministic control paths for common device commands
- generative or LLM-backed interpretation for open-ended conversation
- explicit runtime policy for sensitive domains such as locks, security, and gas

`Home Assistant Assist` is the clearest public example. It treats voice as a pipeline with wake word, STT, intent/conversation routing, and TTS, while newer LLM integrations add personality and richer follow-up behavior without letting every home action become a pure text-generation problem.

This is the right direction for a household agent. A companion should feel natural, but household control still needs predictable execution and traceable policy.

Relevant references:

- [Assist Pipelines](https://developers.home-assistant.io/docs/voice/pipelines/)
- [Home Assistant LLM API](https://developers.home-assistant.io/docs/core/llm/)
- [Create a personality with AI](https://www.home-assistant.io/voice_control/assist_create_open_ai_personality/)
- [AI in Home Assistant](https://www.home-assistant.io/blog/2025/09/11/ai-in-home-assistant/)

## Representative Project Patterns

### FunASR / SenseVoice / CosyVoice

Contribution:

- richer speech-native understanding
- strong practical local deployment story
- streaming speech I/O foundations

What matters most for this repository:

- `SenseVoice` suggests that the ASR boundary should expose more than transcript text
- `CosyVoice` suggests that TTS should be modeled as a stream with controllable expression, not a buffered blob

### LiveKit Agents / Pipecat / TEN

Contribution:

- strong session model
- interruption handling
- tool use inside realtime loops
- transport/runtime separation

What matters most for this repository:

- session and turn management are not a thin wrapper around models
- interruption and low-latency response are part of the runtime contract

### Home Assistant / OpenVoiceOS / Rhasspy

Contribution:

- practical always-on assistant behavior
- wake-word and local-first patterns
- home-context grounding
- follow-up and routine-centric interaction

What matters most for this repository:

- household assistants must mix deterministic control with conversational flexibility
- privacy, local fallback, and device reliability matter more than benchmark novelty

Relevant references:

- [OpenVoiceOS](https://github.com/OpenVoiceOS)
- [ovos-dinkum-listener](https://github.com/OpenVoiceOS/ovos-dinkum-listener)
- [Rhasspy](https://rhasspy.readthedocs.io/)

### Moshi / Conversational Speech Models

Contribution:

- pushes toward dialogue-native spoken interaction
- models overlap, backchanneling, and speech-first timing

What matters most for this repository:

- these systems are promising for future research
- they do not remove the need for controllable policy, tool boundaries, and deterministic home-control safety

## Design Implications For `agent-server`

The current repository direction remains sound:

- keep `Realtime Session Core` transport-neutral
- keep device adapters as adapters
- keep model providers behind runtime interfaces
- keep voice as a built-in runtime capability

The research suggests the next intelligence gains will come less from replacing transports and more from upgrading the runtime contract.

### Recommended Runtime Enrichments

#### 1. Upgrade ASR Output From Plain Text To Structured Speech Understanding

Add optional metadata fields such as:

- detected language
- emotion or speaking style
- audio event labels
- speaker id or confidence
- endpointing metadata

This can remain optional at the schema level while immediately improving policy quality.

#### 2. Add Context-Aware Turn Detection

Current stop conditions should evolve from simple endpointing toward runtime-aware turn control:

- user pause versus turn end
- interruption acceptance during speaking
- short acknowledgement handling
- configurable follow-up listening windows

#### 3. Keep TTS Streaming-First And Add Expression Controls

The runtime should prefer:

- sentence-aware streaming
- early first audio
- interruptible synthesis
- optional style controls for calm, concise, warm, or urgent replies

#### 4. Split Memory Into Short-Term And Durable Layers

The in-process memory backend is a valid bring-up step, but companion behavior will require:

- recent per-session memory
- per-device or per-user preferences
- summarized durable recall
- explicit retention policy and editability

#### 5. Standardize External Capability Access

Continue moving toward one runtime-owned tool layer, ideally compatible with `MCP`-style concepts:

- tools
- resources
- prompts
- server-side capability discovery

#### 6. Route Home Control Through Hybrid Policy

Do not let open-ended conversation policy fully replace deterministic control policy.

Recommended split:

- deterministic control and status actions for normal smart-home commands
- generative interpretation for ambiguity resolution, explanation, and companion dialogue
- explicit safety policy for sensitive actions

## A Practical Roadmap For A More Companion-Like Household Agent

### Near Term

- enrich ASR results with optional speech metadata
- improve barge-in and follow-up listening behavior
- optimize `time-to-first-audio`
- make spoken replies shorter, more state-aware, and less boilerplate
- persist bounded preference memory outside process memory

### Mid Term

- add context-aware turn detection
- add explicit user, room, and device grounding to runtime turns
- add a memory service with summarization and retention policy
- unify smart-home tools and knowledge tools under one runtime capability layer
- add configurable proactive behaviors with rate limits and opt-out controls

### Research Track

- evaluate dialogue-native speech models for specific low-risk paths
- test richer prosody control and conversational TTS
- explore speaker-aware multi-user household memory
- explore multimodal companion loops that combine voice, screen state, and camera context without collapsing privacy boundaries

## Non-Goals And Cautions

- Do not treat every improvement as a model swap problem.
- Do not move orchestration into device adapters.
- Do not let "personality" override safety, clarity, or predictable control behavior.
- Do not retain unlimited raw conversation history without retention and user-control rules.
- Do not assume end-to-end speech-to-speech models are automatically better for household control.

## Bottom Line

As of `2026-04-04`, open-source voice agents are becoming more companion-like by turning speech interaction into a full realtime session system with memory, interruption handling, tool access, and expressive output.

For `agent-server`, the highest-leverage path is not to replace the existing architecture. It is to deepen the current architecture:

- richer speech understanding
- stronger turn control
- streaming expressive output
- layered memory
- runtime-owned tool integration
- hybrid deterministic plus generative policy

That path is aligned with the repository guardrails and is more likely to produce a trustworthy household assistant than a transport-specific or model-specific rewrite.

## Sources

- [FunASR](https://github.com/modelscope/FunASR)
- [SenseVoice](https://github.com/FunAudioLLM/SenseVoice)
- [CosyVoice](https://github.com/FunAudioLLM/CosyVoice)
- [LiveKit Agents](https://docs.livekit.io/agents/)
- [Pipecat](https://docs.pipecat.ai/overview/introduction)
- [TEN Framework](https://github.com/TEN-framework/ten-framework)
- [Moshi](https://github.com/kyutai-labs/moshi)
- [Home Assistant Voice Pipelines](https://developers.home-assistant.io/docs/voice/pipelines/)
- [Home Assistant LLM API](https://developers.home-assistant.io/docs/core/llm/)
- [OpenVoiceOS](https://github.com/OpenVoiceOS)
- [ovos-dinkum-listener](https://github.com/OpenVoiceOS/ovos-dinkum-listener)
- [Rhasspy](https://rhasspy.readthedocs.io/)
- [LangGraph](https://langchain-ai.github.io/langgraph/)
- [LangMem](https://langchain-ai.github.io/langmem/)
- [Model Context Protocol](https://modelcontextprotocol.io/docs/getting-started/intro)
