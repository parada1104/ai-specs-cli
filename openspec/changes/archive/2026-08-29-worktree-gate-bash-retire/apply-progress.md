# Apply progress: worktree-gate-bash-retire

Change: `worktree-gate-bash-retire`
Workspace: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/worktree-gate-bash-retire`
Branch: `change/worktree-gate-bash-retire`
Artifact store: openspec
Strict TDD: active (`openspec/config.yaml` `strict_tdd: true`; runner `./tests/run.sh`)
Delivery: single PR with size:exception (user-approved; no chaining)

## Structured status consumed

Parent native status listed change selection as ambiguous, but the apply
prompt named `worktree-gate-bash-retire` and recorded a delivery decision.
`actionContext.mode` = `repo-local`; `allowedEditRoots` = this worktree.
`applyState` from the ambiguous listing was treated as non-binding for the
named change.

## TDD Cycle Evidence

Prior apply session implemented Phases 1–4 in the working tree and timed out
before `./tests/run.sh`, apply-progress persistence, and commits. This
continuation verified GREEN on that tree; per-slice RED logs from the first
session are not recoverable here.

| Slice | RED (intended) | GREEN (this session) | Notes |
|-------|----------------|----------------------|-------|
| 1.1–1.6 `gate_impl=bash` rejected | Dist-config + gate-binary tests assert rejection / no Bash acquire branch | `./tests/run.sh` 1785 OK in 563.784s | Production: `GATE_IMPL_VALUES = ("auto", "go")`; `recipe.toml` enum `["auto", "go"]`; `gate_binary.acquire` has no `bash` early return |
| 2.1–2.4 launcher fail-open | Hook tests: one stderr warning, exit 0, planted `worktree-gate-legacy.sh` never exec'd | Same suite + `bash -n` launcher OK | Fallback #4 deleted; resolution is env → project-local → version-keyed cache → warn + `exit 0` |
| 2.5–2.7 no legacy materialization | Dist-config + harness-phase4: leftover bytes unchanged; no `LEGACY_HOOK_REL` write | Same suite OK | `materialize_legacy_gate` / `LEGACY_HOOK_REL` / call site removed |
| 2.8–2.9 doctor retable | Doctor tests: bash → ERROR; leftover → INFO + `rm` hint; no rollback-lever text | Same suite OK | `_check_worktree_gate` matches design §1.3 |
| 3.1–3.9 Go-only suites + catalog delete | Parity/hook/tokenizer/metrics retarget; corpus adds `mv a b 2>&1` | Same suite OK; `go test ./catalog/recipes/worktree-flow/gate/...` ok (cached) | Catalog `worktree-gate-legacy.sh` deleted |
| 4.1–4.4 docs/CHANGELOG | n/a (docs) | File review | Breaking changelog + auto\|go docs |
| 4.5 dogfood resync | n/a | Deferred until product commits leave a clean tree | Skill: dogfood-verification-isolation |
| 4.6 full validate | n/a | `./tests/validate.sh` in progress (py_compile + bash -n + gofmt -l clean; suite re-running) | |
| 4.7–4.8 spec sweep + scope | n/a | See scenario map below | Out-of-scope surfaces not edited |

## Persisted task checkbox updates

`openspec/changes/worktree-gate-bash-retire/tasks.md`: implementation rows
1.1–4.8 already `- [x]` from the prior session. Parent-owned `5.1` left
`- [ ]` byte-for-byte. Evidence notes appended on this continuation after
GREEN confirmation.

## Files changed (working tree at revival)

| Path | Phase |
|------|-------|
| `catalog/recipes/worktree-flow/recipe.toml` | 1 |
| `lib/_internal/gate_binary.py` | 1 |
| `tests/test_worktree_gate_dist_config.py` | 1 + 2.5 |
| `tests/test_gate_binary_dist.py` | 1 |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | 2 |
| `lib/_internal/recipe-materialize.py` | 1.3 + 2.6 |
| `lib/_internal/doctor.py` | 2 |
| `tests/test_worktree_gate_hook.py` | 2 + 3.2 |
| `tests/test_worktree_gate_harness_phase4.py` | 2 |
| `tests/test_worktree_root_propagation.py` | 2 |
| `tests/test_doctor_worktree_gate.py` | 2 |
| `tests/test_worktree_gate_parity.py` | 3 |
| `tests/test_worktree_gate_tokenizer.py` | 3 |
| `tests/test_worktree_gate_metrics.py` | 3 |
| `tests/fixtures/worktree-gate-tokenizer-corpus.json` | 3 |
| `.github/workflows/release-worktree-gate.yml` | 3 |
| `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` | 3 (deleted) |
| `docs/runtime-hooks.md` | 4 |
| `docs/recipes-catalog.md` | 4 |
| `catalog/recipes/worktree-flow/README.md` | 4 |
| `CHANGELOG.md` | 4 |

## Actuals vs forecast

| | Forecast | Actual (unstaged `git diff --stat` at revival) |
|--|----------|------------------------------------------------|
| Additions | ≈450–600 | 497 |
| Deletions | ≈950–1,250 | 1,225 |
| Total | ~1,400–1,800 | 1,722 |
| Files | — | 21 product files (+ planning artifacts) |

Catalog `worktree-gate-legacy.sh` accounted for 547 deleted lines.

## Spec-scenario map (task 4.7)

| Spec scenario | Coverage |
|---------------|----------|
| `gate_impl = "bash"` rejected at sync with `auto \| go` | `tests/test_worktree_gate_dist_config.py` |
| Acquisition has no Bash skip; offline auto/go degrade without Bash claim | `tests/test_gate_binary_dist.py` |
| Launcher fail-open, one warning, leftover legacy.sh never exec'd | `tests/test_worktree_gate_hook.py` |
| Ordinary sync does not write/classify leftover legacy.sh | `tests/test_worktree_gate_dist_config.py`, `tests/test_worktree_gate_harness_phase4.py` |
| Doctor: bash ERROR, leftover INFO + rm, missing binary ERROR | `tests/test_doctor_worktree_gate.py` |
| Tokenizer corpus includes `mv a b 2>&1` (Go/shlex, not Bash) | `tests/fixtures/worktree-gate-tokenizer-corpus.json`, `tests/test_worktree_gate_tokenizer.py` |
| Parity/metrics Go-only | `tests/test_worktree_gate_parity.py`, `tests/test_worktree_gate_metrics.py` |
| Freshness/launcher MODIFIED scenarios stay green | `tests/test_worktree_gate_harness_phase4.py`, dist-config |

## Scope guard (task 4.8)

Not edited: `lib/_internal/hooks-render.py`, generated wrapper templates,
`catalog/recipes/trello-mcp-workflow/**`, Go gate policy (`decide.go`,
extraction, topology), no auto-deletion of consumer leftovers, no Windows work.

Historical `worktree-gate-legacy.sh:NNN` comments remain in
`catalog/recipes/worktree-flow/gate/*.go` (out of scope).

## Remaining tasks

Parent-owned only:

- [ ] 5.1 Start or reuse bounded review. `<!-- sdd-owner: parent -->`

## Workload / PR boundary

Single PR with size:exception. No chained-PR split. Atomic Phase-3 slice
included in that PR.

## Deviations from design

None. `gate_impl` key retained (`auto \| go`) per design D1.

## Test commands

- `./tests/run.sh` — GREEN. `Ran 1785 tests in 563.784s` / `OK`. Go gate tests `ok ai-specs.dev/worktree-gate (cached)`.
- `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh` — OK
- `./tests/validate.sh` — GREEN. Exit 0 (orchestrator rerun 2026-08-29 after the apply child was interrupted): py_compile, bash -n, gofmt clean, premerge-guardian OK (light + standard), `Ran 1785 tests in 602.921s` / `OK`, plus live sync/init/doctor smoke scenarios green.
- Dogfood `./bin/ai-specs sync` (task 4.5, per dogfood-verification-isolation skill) — evidence captured, project state reverted:
  - Ordinary sync in this worktree reports `hook ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh has no recorded provenance; preserving existing bytes` — no gate rewrite, and NO legacy-gate materialization path fires.
  - Materialized `ai-specs/recipes/worktree-flow/hooks/` contains only `worktree-gate.sh` with `stamped_gate_impl="auto"` and zero `worktree-gate-legacy` references (fallback #4 gone).
  - `ai-specs doctor` reports no retired-value ERROR (no `gate_impl=bash` diagnostics).
  - Project-state output (`ai-specs/.ai-specs.lock`, materialized overrides) was REVERTED after evidence capture — verification output is never committed to the feature branch; this repo's own dogfood migration happens post-release on its own terms.

## Commits

- `158c29c` feat(worktree-flow): retire the Bash worktree gate — product code + suite retarget, 17 files, +427/−1,169 (includes deletion of `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh`, 547 lines).
- `b313fb3` docs(worktree-flow): document the Go-only worktree gate — docs/runtime-hooks.md, docs/recipes-catalog.md, recipe README, CHANGELOG (breaking note for gate_impl=bash).
- Planning artifacts (spec delta, design, tasks, apply-progress) committed as the final chore commit on this branch.

## ActionContext warnings

None.
