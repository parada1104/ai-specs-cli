# Proposal: retire the Bash worktree gate

- **Change slug**: `worktree-gate-bash-retire`
- **Status**: follow-up (not started — entry criteria pending)
- **Depends on**: `worktree-gate-go` (the Go gate becoming the default)

## Entry criterion

Start this change **after one minor release** has shipped with the Go gate as
the default (`gate_impl = "auto"`) and **no field regression** has been
reported against the gate (blocked-write false negatives, silent fail-open,
stamping or acquisition defects). That release cadence guarantees a rollback
window (`gate_impl = "bash"` stays usable) before any removal.

## What changes (draft)

1. Remove the frozen Bash reference `hooks/worktree-gate-legacy.sh` from the
   catalog and stop materializing it.
2. Remove the `bash` value from the `gate_impl` enum (`auto | go`), or retire
   the key entirely once `auto` is the only behavior.
3. Delete the legacy Bash materialization path in `recipe-materialize.py` and
   the launcher's legacy fallback step.
4. Drop the Bash half of the parity corpus (keep the corpus as the Go spec).
5. Update `docs/runtime-hooks.md`, the recipe README, and the catalog.

## Exit criteria

- `gate_impl = bash` no longer exists anywhere (config, docs, launcher).
- The launcher fails open with one stderr warning when no binary resolves.
- All parity, tokenizer, hook, dist and doctor suites pass with the Go binary
  as the only implementation.

## Success Criteria

1. `gate_impl` accepts only `auto | go` — the value `bash` is rejected with an
   actionable error by config validation, acquisition, doctor, and the
   launcher stamp (verified by `tests/test_worktree_gate_dist_config.py`,
   `tests/test_gate_binary_dist.py`, `tests/test_doctor_worktree_gate.py`).
2. The launcher fails open with exactly one stderr warning when no Go binary
   resolves, and never executes a legacy Bash fallback (verified by
   `tests/test_worktree_gate_hook.py` single-warning and planted-sentinel
   scenarios).
3. Ordinary sync never materializes a Bash gate: no legacy reference in the
   catalog, no `materialize_legacy_gate` call path, and the parity corpus
   pins the `mv a b 2>&1` tokenizer regression as Go-only behavior
   (verified by `tests/test_worktree_gate_parity.py`,
   `tests/test_worktree_gate_tokenizer.py`,
   `tests/test_worktree_gate_harness_phase4.py`).
4. Full suite green on the final tree: `./tests/validate.sh` exit 0 with
   1785 tests and zero skips, reproduced independently by apply and verify.
