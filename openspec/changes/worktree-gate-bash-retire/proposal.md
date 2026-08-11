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
