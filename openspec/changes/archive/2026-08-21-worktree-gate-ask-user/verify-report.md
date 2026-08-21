# Verify Report: worktree-gate-ask-user

## Verdict

**PASS** — the candidate passes the gate suite, Go suite, SHA256SUMS trust-root
regeneration, and a full Judgment Day adversarial review (round 1, APPROVED).

## Verify evidence

- Verdict: PASS
- Command: `python3 -m pytest tests/test_worktree_gate_hook.py tests/test_worktree_gate_parity.py tests/test_worktree_gate_release_phase4.py tests/test_worktree_root_propagation.py tests/test_worktree_gate_dist_config.py tests/test_worktree_gate_tokenizer.py tests/test_worktree_flow_recipe.py -q`
- Exit: 0
- Date: 2026-08-21
- Commit: c91dd1a

## Evidence detail

### Go suite

`go test ./...` in `catalog/recipes/worktree-flow/gate/` — PASS. New
`TestAskMessage*`/`TestDefaultTool` plus existing decide/parity/core green.

### Python gate suite (independent re-run)

- **253 passed, 0 failures** in 107.66s (310 subtests passed).
- Covers: hook behavior (ask 3-option message, no `WORKTREE_GATE_MODE=off`),
  Bash parity, release digests, dist config, tokenizer, recipe materialization.

### SHA256SUMS trust root

- Regenerated with canonical go1.24.13 via `scripts/build-gate.sh`.
- All 4 matrix digests match freshly built binaries: `test_committed_digests_match_locally_built_assets` + `test_ci_generated_sums_file_parses_and_matches_build` pass.
- `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` → OK.

### Full repo suite note

`./tests/run.sh` (1865 tests): only the 6 pre-existing environmental failures
documented by PR #230 (Go digest cold-cache + `test_upgrade_notices` loader).
All 6 pass on `development` and are unrelated to this change.

## Judgment Day (adversarial) — APPROVED

Two blind judges (`commandcode/gpt-5.6-luna:high`) against the immutable diff.

- F1 (judge A): "hook deliverable not updated" — **false positive** (wrapper
  delegates to Go binary + legacy; no inline messages).
- F2 (judge B): "env override still functional; option 3 not enforced" —
  **suspect accepted as designed** (human decision: option 3 regulated by skill
  + explicit user choice; operator override preserved).
- Ledger: `openspec/changes/archive/2026-08-21-worktree-gate-ask-user/review-ledger.md`.

## Spec compliance

All requirements from `specs/worktree-flow/gate-ask-mode.md` verified COMPLIANT:
3-option user destinations, self-bypass hint removed (Go + Bash), `always`/`off`
byte-intact, Go↔Bash parity, per-harness question mechanism, skill anti-bypass
rule. See the spec matrix in the candidate commit for the per-requirement test
evidence.