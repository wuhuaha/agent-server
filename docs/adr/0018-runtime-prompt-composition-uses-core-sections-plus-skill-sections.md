# ADR 0018: Runtime Prompt Composition Uses Core Sections Plus Skill Sections

## Status

Accepted

## Context

The shared runtime already owned persona selection, output constraints, and execution-mode policy, but they were still assembled through one hardcoded system-prompt function.

That shape was workable for bring-up, but it made two things blurry:

1. core runtime policy versus product-domain behavior
2. stable runtime-owned policy versus pluggable skill-owned prompt guidance

After moving household control out of the executor path and into runtime skills, prompt composition needed the same cleanup.

## Decision

Split runtime prompt composition into explicit layers:

1. core prompt sections
   - persona section
   - runtime output-contract section
   - execution-mode policy section
2. runtime-skill prompt sections

The core executor continues to own prompt assembly, but the assembly step now works over composable prompt sections instead of one monolithic hardcoded string.

For the current implementation:

- `BuiltinPromptSectionProvider` provides the core sections
- runtime skills may still append their own prompt fragments separately
- `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT` overrides the persona section only
- execution-mode and output-contract policy remain runtime-owned and cannot be disabled by a persona override

## Consequences

- The runtime keeps clear ownership of global assistant policy.
- Skill-specific behavior can evolve without turning the core prompt builder into a product-rules file.
- Future domain skills can add prompt guidance without modifying the core persona or execution-mode sections.
- The path remains open for more granular policy toggles later without changing transport contracts.

## Rejected Alternatives

### Keep one monolithic hardcoded system prompt

Rejected because it hides the separation between global runtime policy and pluggable domain guidance.

### Let runtime skills fully replace the whole prompt

Rejected because execution-mode policy and output-contract constraints are runtime-owned safety and product invariants, not optional skill behavior.
