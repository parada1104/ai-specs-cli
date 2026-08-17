# Proposal: plan-build-flow phase compatibility (direct)

## Intent

Make the bundled plan-build contract explicit about Full logical phases,
optional host-advertised execution, one-session preflight composition, and
artifact-derived plan review without coupling the recipe to a runtime or
external provider.

## Scope

- Full: `explore -> proposal -> spec/design -> tasks`, with spec and design
  parallel only after proposal and tasks waiting for both.
- Missing or unavailable phase execution may fall back inline; malformed,
  partial, or blocked results stop and preserve state.
- Standard and Light remain collapsed as currently defined.
- Presentation, preflight, existing readiness, worktree, verify, archive, and
  PR/topology contracts are documented and tested.

## Out of Scope

- New runtime code, recipe configuration fields, external runtime selection, or
  changes to the paused SDD worktree.
- External orchestration integration, phase subagents, archive/verify/PR gate
  behavior, and live eval framework changes.

## Tracker

- **card_id**: `6a83510b0a6de6bf2ab607ed`
- **url**: https://trello.com/c/5ClJQNEW/82-direct-plan-build-phase-compatibility-and-plan-presentation
