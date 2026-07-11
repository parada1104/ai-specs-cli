# Proposal: worktree-flow recipe modes (`always` / `ask` / `off`)

## Intent

`worktree-flow` is enforced strictly today: `worktree-gate.sh` blocks every
Edit/Write/MultiEdit against the main worktree while on a protected branch
(`main`, `development`), with no escape hatch. That protects the trunk but is
the wrong default for every team — one-off config edits, small fixes, and
trunk-first repos all hit the wall even when the user understands the tradeoff.
This change makes the gate *configurable* per project.

## Modes

| Mode | Hook behavior on protected branch | Meant for |
|------|----------------------------------|-----------|
| `always` | `exit 2` — today's strict block | Trunk-protection, CI-bound repos |
| `ask` | `exit 2` with a confirm-to-proceed message; orchestrator mediates confirmation | Mixed workflows wanting a speed bump, not a wall |
| `off` | self-disables early, `exit 0`; skill still documents convention | Trunk-first / scratch repos |

`always` and `off` are deterministic. `ask` is the only interactive mode and is
**agent-mediated** (hook stderr surfaced by host), not a TTY dialog.

## Default

`always`. Strict behavior already ships and is the safest choice; changing the
default would silently relax trunk protection for relying projects. `ask` and
`off` are opt-in via `[recipes.worktree-flow.config]`.

## Scope

### In scope
- New config key `recipes.worktree-flow.config.gate_mode` (`always` | `ask` | `off`, default `always`).
- `worktree-gate.sh` reads the sync-stamped mode (+ `WORKTREE_GATE_MODE` env override) and branches; no runtime manifest lookup.
- `ai-specs sync` stamps the resolved mode into the materialized hook and config snapshot.
- Recipe `README.md` documents the modes, default, and tradeoffs.
- Unit tests per mode + env override (TDD per `openspec/config.yaml`).

### Out of scope
- Per-path allowlists (exempt paths).
- New commands/hooks beyond the existing gate.
- Changes to the cleanup contract (`worktree-cleanup.sh`).
- TTY/GUI prompts — `ask` is agent-mediated.

## Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| `worktree-flow` | **New** | Recipe config schema, gate-mode resolution, enforcement contract for `always` / `ask` / `off` |

No `worktree-flow` spec exists in `openspec/specs/`, so this becomes a new
capability spec; later gate changes become deltas.

## Approach

1. Resolve `gate_mode` deterministically at sync time and stamp it into the materialized `worktree-gate.sh` so the hook has no runtime manifest lookup to fail-open on.
2. Hook dispatch: `off` → `exit 0` at top; `ask` → guidance + `exit 2`; `always` → today's logic unchanged.
3. Keep `WORKTREE_GATE_PROTECTED` (protected-branch override) orthogonal to mode.
4. README + docs show the modes table and the recommended pick per team shape.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` | Modified | Mode dispatch + env override |
| `ai-specs/recipes/worktree-flow/README.md` | Modified | Modes table, default, tradeoffs |
| `ai-specs/recipes/worktree-flow/config.yaml` | Modified | Validate `gate_mode` enum |
| `templates/ai-specs.toml.tmpl`, `docs/ai-specs-toml.md` | Modified | Document `gate_mode` |
| `tests/test_worktree_flow_gate.py` | New | Unit tests for the three modes + env override |
| `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | Unchanged | Cleanup contract untouched |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `ask` message mis-surfaced by some hosts → edits silently fail | Medium | Message names the `WORKTREE_GATE_MODE=off` one-shot bypass; README documents host mediation caveat |
| Teams set `off` and lose trunk protection unintentionally | Low | Default stays `always`; `off` needs explicit opt-in; doctor can WARN when `off` active |
| Env override changes strict mode on machines not stamped by sync | Low | Env override beats stamped value; resolution order documented |

## Rollback Plan

1. Revert `gate_mode` parsing; hook returns to today's strict logic.
2. Manifests without `gate_mode` keep `always` behavior, so revert is non-breaking.
3. Drop the new test module and README section. No data migration concerns.

## Dependencies

- Existing `worktree-flow` recipe v1.0.0.
- Host agent surfaces hook stderr (already required by the existing gate).

## Success Criteria

- [ ] `gate_mode = "always"` (or unset) → gate behaves exactly as today.
- [ ] `gate_mode = "off"` → gate exits `0` immediately for main-worktree writes on protected branches.
- [ ] `gate_mode = "ask"` → gate exits `2` with a confirmation message, and a documented orchestrator step lets the user proceed via the documented env override.
- [ ] `WORKTREE_GATE_MODE` env override beats the manifest value with documented precedence.
- [ ] `ai-specs sync` materializes the resolved mode into `worktree-gate.sh`.
- [ ] README documents all three modes, the default, and recommended picks.
- [ ] `./tests/validate.sh` passes.