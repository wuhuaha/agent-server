# ADR 0041: LLM Semantic Judging Stays Advisory Over Realtime Heuristics

## Status

Accepted

## Context

The shared voice runtime already owns:

- preview-based turn arbitration
- acoustic-first interruption verification
- reversible `duck_only / backchannel / hard_interrupt` policy

However, the current semantic layer was still dominated by hand-written heuristics:

- `looksLexicallyComplete(...)`
- correction-suffix rules
- backchannel token lists
- takeover lexicon and threshold scoring

Those rules are still necessary for realtime safety, but they are not sufficient for the current product goal: a more natural, more intelligent realtime voice agent on a GPU-first host with a local LLM already available.

At the same time, replacing turn-taking or interruption with a free-running LLM-only decision path would be a regression:

- higher latency variance
- weaker explainability
- harder rollback when the model misclassifies a partial utterance
- more pressure on adapters to recover from semantic mistakes

## Decision

Introduce an optional runtime-owned `SemanticTurnJudge` inside `internal/voice` with these constraints:

1. the judge is **advisory**, not authoritative
2. provider access stays behind the shared `agent.ChatModel` boundary
3. websocket and channel adapters do not call the model directly
4. acoustic timing, silence windows, and transport-safe interruption behavior remain in the heuristic core
5. LLM judgement may upgrade or suppress **draft / prewarm / interruption interpretation**, but does not directly short-circuit final accept semantics
6. rollout and A/B policy stay runtime-owned and session-sticky rather than becoming adapter or client configuration

Concretely:

- preview sessions may asynchronously request an LLM semantic judgement for a mature preview candidate
- the returned judgement may mark a preview as semantically complete, correction-like, backchannel-like, or takeover-like
- that judgement is merged back into `InputPreview.Arbitration`
- `EvaluateBargeIn(...)` may use the semantic intent to avoid false hard interrupts on short acknowledgements and to escalate earlier when semantic takeover is clear
- preview sessions may independently stay in `control`, `semantic`, or `sticky_percent` rollout variants, but that decision is made once inside `internal/voice` and then only exposed as additive trace metadata such as variant or enabled state

## Consequences

Positive:

- the current project starts using the local LLM for realtime spoken-interaction policy, not only for post-accept reply generation
- turn-taking and interruption can improve without moving provider logic into adapters
- the model assists where rules are weakest: semantic completeness, correction detection, and takeover-vs-backchannel disambiguation
- the system remains rollback-friendly because the heuristic floor still exists underneath

Tradeoffs:

- preview sessions now own one more asynchronous state path
- extra observability is required to tell whether a semantic promotion came from heuristics or the model
- badly tuned prompts or timeouts could still add unnecessary traffic to the local LLM worker

## Follow-Up

- keep the semantic judge capability-gated and runtime-configurable
- keep semantic rollout default-conservative (`control`) and make A/B activation sticky per preview session
- add trace fields and later eval slices for semantic-judge hit rate, false promotion rate, and interruption disagreement rate
- later extend the same judge toward `slot completeness` and domain-aware ambiguity handling rather than turning it into a free-form policy engine
