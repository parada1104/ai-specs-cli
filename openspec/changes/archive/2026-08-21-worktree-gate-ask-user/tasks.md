# Tasks: gate_mode=ask consults the user

Depth: standard

## Tasks

1. **Respecify `gate_mode=ask` message** — update `gate/message.go`
   `AskHint()` to emit a three-option user decision when `ask` blocks a
   protected-branch write, and stop advertising `WORKTREE_GATE_MODE=off` in
   that branch. The message lists the three choices without revealing any
   bypass mechanism.
2. **Keep Go `always`/`off` behavior intact** — `always` keeps its hard block +
   worktree guidance message; `off` keeps its early return. Verify `main.go`
   dispatch is untouched except the `ask` branch output.
3. **Mirror the change in the frozen Bash reference** —
   `hooks/worktree-gate-legacy.sh` lines ~527-533: drop the self-bypass hint in
   `ask` mode and add the three-option guidance.
4. **Update the deliverable hook** — `hooks/worktree-gate.sh` keeps byte-parity
   with the bash reference for the `ask` message.
5. **Add coverage in unit tests** — extend `gate/main_test.go` (or a new
   `message_test.go`) asserting the `ask` message lists a worktree, a feature
   branch, and the protected-branch override, and does not contain
   `WORKTREE_GATE_MODE=off`.
6. **Parity tests** — update `tests/test_worktree_gate_parity.py` so the Go and
   Bash implementations agree on the new `ask` stderr text.
7. **Hook integration test** — extend `tests/test_worktree_gate_hook.py` to
   assert the `ask` mode block message includes the three options.
8. **Recipe skill + catalog docs** — document the new `ask` semantics in the
   `worktree-flow` SKILL.md table and `docs/recipes-catalog.md` config row
   (`gate_mode` help text), and remove any prose telling the agent to
   self-bypass. The skill prose regulates option 3: only an explicit user
   choice may lead the agent to re-run a write against the protected branch,
   and it maps "ask the user" to each harness's native mechanism (pi
   `ask_user_question`, Claude `AskUserQuestion`, opencode/cursor/omp
   interactive prompts).
9. **Changelog entry** — add a `### Changed` note for `gate_mode=ask`
   under Unreleased. **No version bump in this change**: PR #230 already
   claims `worktree-flow 1.5.0 → 1.6.0` for `creation_mode`; the next bump
   stays out of this diff to avoid a version collision.
10. **Run full suite** — `./tests/run.sh` (this repo's `test_command`); fix
    regressions; record GREEN evidence.

## Review workload forecast

- Expected surface: hook script(s), Go message/tests, parity test, recipe
  toml/skill + catalog + changelog.
- Standard review risk: message semantics, not the decision core.
- Adversarial cases: `ask` vs `always` behavioral drift, leaked
  `WORKTREE_GATE_MODE=off` hint, parity between Bash and Go, no regressions to
  `off` and linked-worktree paths.