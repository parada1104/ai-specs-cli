# Explore: gate-cwd-fidelity

## Root cause (confirmed end-to-end)

The Pi/OMP adapters hardcode `cwd: process.cwd()` (main checkout), and the Go gate absolutizes shell-write candidates against that event cwd while never parsing `git -C`/`cd`. The genuine systemic fix belongs in the **gate**, not the adapters:

- The **event cwd** carries the host session's `process.cwd()`, NOT the cwd of the command being gated.
- For `git -C <wt> ...` and compound `cd X && ...`, the writable cwd lives in the **command text**.
- Pi/OMP correctly keep `process.cwd()` events — **pinned by spec** (`openspec/specs/workspace-context` + `tests/test_hooks_render.py::_assert_process_cwd_event`). Changing them breaks a frozen contract and still cannot derive the write cwd.

## Alternative evaluation (against code)

### Alt 1 — runtime cwd-fidelity contract (adapter declares trusted / provides cwd)
- **Viable as component, not alone. Insufficient.** Conflicts with frozen normatives: workspace-context spec + `_assert_process_cwd_event` test require process.cwd-only events. Changing that breaks the spec/test and never closes the root cause. Blast radius: renderer + generated adapters + event parser + frozen spec/test. High regression risk, no real effect on the bug.

### Alt 2 — honest gate degrade (warn/ask, no block-on-guess) — SELECTED
- **Viable, cleanest systemic fix.** Self-contained in the Go gate. `main.go` routes `mode==ask` → `AskMessage` (already exists), `mode==always` → `BlockMessage`. For the degrade: detect when the effective cwd is **untrustworthy** and emit ask/warn instead of block-on-guess.
- Authoritative mechanism: parse the cwd the command determines — `git -C <dir>` and `cd <dir> && ...` (splitPOSIX/splitSegments already tokenize; missing a pass extracting `-C`/`cd`). When event + command don't yield a resolvable existing cwd and the path is relative, `Decide` degrades instead of assuming host process-cwd.
- Zero false positives; keeps block-on-trust; does not cross the workspace-context contract; one Go source + tests covers BOTH hook surfaces (worktree-gate and worktree-gate-shell share the same `worktree-gate.sh` launcher).
- Risk: extra parsing pass (determinism, nested `git -C`/`cd`, `cd -`, chained `&&`). Contained to Go fixtures; does not touch mode interface or exit code.

### Alt 3 — doctor version-drift check (gate vs catalog)
- **Already implemented, orthogonal.** `doctor.py::_check_worktree_gate` does ERROR on retired bash impl / digest mismatch / selftest, and WARN on `binary_version != stamped_gate_version`. Not the cause. Complements hygiene only.

### Plus — wrong exit message (CONFIRMED)
- `message.go::BlockMessage` (and `decide_test.go:126,130`) is byte-identical to legacy `worktree-gate-legacy.sh:527-529`: "Create a dedicated worktree first (e.g. /worktree-new)". In the bug scenario the user is already in a worktree — wrong exit. Must name the actual command cwd instead.

## Surfaces covered

The "9 surfaces" claim was imprecise: the protected-branch cwd-fidelity false positive is specific to the **worktree-flow Go gate**. plan-build-flow (Bash, planning-root logic) and trello-mcp-workflow/tracker (Bash, Tracker-link logic, not protected-branch resolution) share process.cwd() but different logic and do not suffer the same false positive. Fixing the Go gate covers both worktree surfaces (path + shell) in one source.

## Recommendation — SELECTED DIRECTION

Alt 2 (gate) primary, with adapter metadata (Alt 1) as an optional low-value hint. Alt 3 descarded as a path.

1. Implement in the Go gate (one source, two hooks): parse `git -C`/`cd` as the authoritative cwd base for `Decide`. When effective cwd is not recoverable and the path is relative, degrade to ask/warn instead of block-on-guess on host process-cwd. No false positives; keeps block-on-trust; preserves workspace-context contract.
2. Refine proposal.md — the root is not "adapter reports bad cwd" but "the gate infers the effective cwd from an event cwd that neither the adapter nor the command text alone recovers; the command cwd is authoritative; degrade when not recoverable". Also add the wrong-exit correction.
3. Optional (low value): a `cwd_trusted`/`cwd_hint` adapter field as a signal, WITHOUT changing the Pi/OMP event cwd (would break the frozen spec). Keep process.cwd() adapter semantics and resolve truth in the gate.

## Risks

- Parsing `git -C`/`cd` adds a new pass with determinism to prove (nested, `&&`, `cd -`, spaces in paths). Contained to Go gate fixtures; does not touch mode interface or exit code.
- Changing Pi/OMP cwd (if done) breaks workspace-context spec + boundary test — avoided.
- Degrade to warn reduces blocking power on the "untrustworthy" path; must NOT announce self-bypass `WORKTREE_GATE_MODE=off`, and must not turn legitimate false negatives into silent ones.
- `worktree-gate.sh` is bash 3.2 and synchronized; any launcher change must keep the contract. Not changed here (the Go gate does the work).

## Key Learnings
- The root cause is gate-side cwd inference, not adapter reporting: the writable cwd lives in the command text (`git -C`/`cd`), and the event cwd only carries host process-cwd.
- The Pi/OMP event-cwd semantics are frozen by workspace-context spec and its boundary test; changing them is off-limits and cannot fix the derive anyway.
- Alt 2 (honest gate degrade) is the cleanest systemic fix: one Go source covers both worktree hook surfaces with zero false positives and no contract crossing.
- The doctor version-drift check is already implemented and orthogonal — it is not the cause of this bug.
- The block message "Create a dedicated worktree" is a wrong exit in this scenario because the user is already in a worktree; it must name the command cwd.
