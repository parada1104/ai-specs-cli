# Design: plan-build-evals-v2

## Overview

Three coordinated surfaces:

1. **Eval harness v2** — runtime-agnostic headless runner with plan/build mode
2. **Pre-merge guardian** — skill + deterministic checks before merge
3. **Worktree cleanup hardening** — leave-cwd + script-first cleanup after merge

## Decisions

### D1 — Plan mode is the harness surface; prompts stay natural

Each runtime is invoked in its **plan** mode with a user-shaped request
("necesito implementar…"). The eval asserts classifier + artifacts + no
production edits. Never inject "haz un plan" / `/plan` into the prompt.

Runtime flags (confirm during apply; adjust if CLI drift):

| Runtime | Plan invocation sketch | Default model |
|---------|------------------------|---------------|
| `claude` | `claude -p --permission-mode …` with plan permission / plan mode | `opus` |
| `opencode` | `opencode run --format json --model opencode-go/<model>` (+ plan mode if available) | `glm-5.2` (alt `deepseek-v4-flash`) |
| `pi` / `omp` | `pi -p` / `omp` with `--model opencode-go/<model>` (+ plan flags) | same as OpenCode |

`EVALS_RUNTIME` selects backend; `EVALS_MODEL` overrides default.

### D2 — Classifier tiers unchanged

Keep Full / Standard / Light. Evals focus on **Standard** (spec + tasks) for
substantial "implement X" prompts, plus one **Light** scenario if cheap.

### D3 — Fixture must seed app + runtime skill discovery

Materialize recipe, copy skill into the runtime discovery path
(`.claude/skills`, `.opencode/skills`, `.pi/skills`, …), seed a small app so
the agent has something to plan against, write minimal `AGENTS.md` brief
fragment from recipe `[provides.brief]`.

### D4 — Pre-merge guardian: skill contract + checkable criteria

No new CLI product command required in v1. Harden:

- `plan-build-flow` skill §7 (PR + archive gates) with explicit **merge**
  hard-stop language
- All three VCS merge skills: before `gh pr merge` / `glab mr merge` /
  Bitbucket merge, verify:
  1. Matching `openspec/changes/<slug>/` is **gone** (moved to `archive/`)
  2. `openspec/changes/archive/<slug>/` contains tier minimum files
  3. Archive commit is on the review branch (pushed)

Add a small **deterministic helper** (shell or python under recipe bin or
`tests/`) that can assert (1)–(2) for unit tests and optional agent use.
Live eval of the guardian is optional; unit tests cover the helper.

### D5 — Worktree cleanup: leave first, script second, force-delete squash

Document and test:

1. Always `cd` to main repo root before `git worktree remove`
2. Prefer `worktree-cleanup.sh --base <integration>` after merge
3. Manual fallback only if script unavailable
4. Keep squash/rebase merge detection (already in script); ensure VCS skills
   call the script rather than only ad-hoc `worktree remove`

### D6 — Spec deltas by capability

| Capability | Delta focus |
|------------|-------------|
| `recipe-evals` | multi-runtime, plan mode, natural prompts, model defaults |
| `plan-build-flow` | merge guardian language; ambient plan-mode preference |
| `vcs-pr-flow` | hard-stop merge without archive; mandatory post-merge cleanup |
| `worktree-flow` | leave-cwd + script-first post-merge cleanup |

## Risks

| Risk | Mitigation |
|------|------------|
| Runtime CLI flags differ / drift | Probe during apply; document actual flags in harness |
| Light models ignore ambient skill | Prefer glm-5.2 / deepseek-v4-flash / opus; N-of-M trials |
| Guardian too strict for Light (tasks-only) | Tier-aware minimum checks |
| Cleanup removes dirty trees | Keep dirty skip; never force without user |

## Alternatives considered

- **Prompt-only "please plan"** — rejected; tests coaching, not ambient skill
- **New merge CLI binary** — deferred; skills + helper sufficient for v1
- **Composer / sonnet defaults** — rejected by user preference
