# Delta: plan-build-flow phase compatibility (direct)

## ADDED Requirements

### Requirement: Full phase compatibility

Full planning SHALL run the logical phases `explore`, `proposal`, `spec/design`,
and `tasks` in dependency order. A host-advertised executor MAY implement the
current phase, but the recipe SHALL remain provider-neutral. If execution is
unavailable, the current phase SHALL run inline. Malformed, partial, or blocked
executor results SHALL stop and preserve existing state rather than rerunning,
skipping, or accepting an incomplete artifact.

### Requirement: Single preflight authority

Plan-build SHALL consume one session-level preflight decision for execution
mode, artifact store, review budget, delivery strategy, and chain strategy. It
SHALL not recollect or override those values or add duplicate prompts.

### Requirement: Artifact-derived presentation

The final plan SHALL present intent, scope, key decisions, affected areas,
risks, open questions, and recommendations/assumptions from available
artifacts. Interactive questions follow the phase that exposed them; automatic
mode records recommendations as labeled assumptions or decision notes and
blocks unresolved product decisions. Final review SHALL offer accept, adjust,
or stop.
