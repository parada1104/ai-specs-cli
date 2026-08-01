# Tasks: compact-sync-output

Depth: **spec + tasks**

Branch: `feat/compact-sync-output`
Worktree: `/Users/robert/proyectos/nnodes/ai-specs-cli-sync-output`
Plan refs: `proposal.md`, `specs/sync-output-verbosity/spec.md`

**Stop for human authorization before any further production code implementation.**

## Tier rationale

Not `tasks-only`: the change adds an observable CLI contract (`-v/--verbose` on two
commands), redefines the default output of the two most central scripts in the repo, and
alters fan-out control flow. A new capability needs a spec.

Not `full`: no architectural fork is open. The capture-and-filter approach is already
chosen and is a local mechanism, not a cross-module design. If the gate decides to
re-litigate the stream-capture strategy (see proposal risk 1), this escalates to `full`
and needs a `design.md`.

## Prior state

Production code was written before any planning existed: 192 uncommitted lines in
`lib/sync.sh` and `lib/sync-agent.sh`, zero tests, no change folder. P1 below therefore
starts from existing-but-unverified code rather than from nothing.

---

## P0 — Planning gate (this session)

- [x] `proposal.md`
- [x] Spec delta: `sync-output-verbosity`
- [x] `tasks.md` (this file)
- [x] Commit the existing uncommitted work so it stops living only in the working tree
- [x] **Human authorization to continue implementation** (given in chat: "si, continuemos con el cambio")

---

## P1 — Fix the fan-out regression (TDD, blocking)

**Goal:** the parent must not run an extra silent sync pass. This is a correctness defect
introduced by the uncommitted code and gates everything else.

- [x] **T1.1** — RED: test that a public root resolving to 2 targets produces exactly 2
  child `sync-agent` invocations and no additional parent materialize/render pass.
- [x] **T1.2** — GREEN: restore the terminal `exit 0` after the fan-out loop in
  `lib/sync-agent.sh` (removed at line 163 of the current working tree).
- [x] **T1.3** — Test that a first-child failure stops the loop, exits non-zero, and names
  the failing target on stderr (guards the existing behavior against the refactor).

---

## P2 — Verbosity contract (TDD)

- [x] **T2.1** — RED: compact mode suppresses `✓`/`·`/`⇢`/`▸` lines and blank lines.
- [x] **T2.2** — RED: compact mode preserves `!`/`✗`/`ℹ` lines byte-identically and on
  their original stream.
- [x] **T2.3** — RED: `--verbose` reproduces the step's full output.
- [x] **T2.4** — RED: a failing step prints full unfiltered stdout+stderr in **both**
  modes and propagates the exit status.
- [x] **T2.5** — GREEN for T2.1–T2.4 against the existing `print_step_output` / `run_step`
  implementation; fix whatever the tests expose.
- [x] **T2.6** — Test `-v` forwarding through fan-out, and that it is absent when the
  parent had no `-v`.
- [x] **T2.7** — Test unknown-flag rejection still exits non-zero on both commands.

---

## P3 — Nested framing and marker hygiene (TDD)

- [x] **T3.1** — RED: header appears exactly once and `✓ sync-agent complete` does not
  appear for fan-out children.
- [x] **T3.2** — RED: the "mcp skipped (no [mcp.*] in manifest)" notice is visible in
  compact mode. Currently it starts with `·` (`sync-agent.sh:446`) and is therefore
  swallowed, while the analogous "skipped AGENTS.md" notice was promoted to `ℹ`. Fix the
  inconsistency.
- [x] **T3.3** — Audit every remaining `·` line and decide per line: noise (keep `·`) or
  notice (promote to `ℹ`). Record the decision in the spec if any line is ambiguous.

---

## P4 — errexit interaction (TDD)

- [x] **T4.1** — Characterize `shopt -s inherit_errexit`: add a test covering a command
  substitution below that line whose inner command fails, asserting the intended
  behavior. This is the highest-risk line in the diff after the `exit 0` removal.
- [x] **T4.2** — Review the ~12 `|| return $?` sites added in `sync_one_agent` for
  correctness under the new errexit mode; confirm no failure is silently swallowed.
- [x] **T4.3** — Decide whether the `sync_one_agent` extraction ships as its own commit
  ahead of the presentation feature, for reviewability (proposal risk 3).

---

## P5 — Docs and close

- [x] **T5.1** — Document both modes and the failure-always-full guarantee in the sync
  docs; note the cross-stream ordering caveat. (README.md CLI table + note)
- [x] **T5.2** — `CHANGELOG.md` entry under `## [Unreleased]`.
- [x] **T5.3** — `./tests/validate.sh` green. (1187 tests OK)
- [x] **T5.4** — Manual check: `ai-specs sync` and `ai-specs sync -v` on this repo, per the
  `dogfood-verification-isolation` skill so dogfood state does not leak into the commit.
  (both modes verified; AGENTS.md regen reverted, only README/CHANGELOG kept)
- [ ] **T5.5** — Verify against this spec, then archive the change folder on the review
  branch before opening the PR.

---


---

## P6 — Verify remediation (post-judge findings)

Remediation after Judgment Day / adversarial review on `f799196`. T5.5
(archive) remains open until this remediation is independently re-verified.

- [x] **F1/H1** — Wrap `flatten-resolved-skills`, `merge-commands`, and subrepo
  `gitignore-render` through `run_step` in `lib/sync-agent.sh` so compact mode
  no longer leaks raw `✓` detail lines. E2E tests cover standalone and
  public-root fan-out (2+ targets), allowing only the intentional top-level
  `✓ sync-agent complete` footer.
- [x] **H2** — Behavioral e2e tests: `ai-specs sync -v` shows detail from parent
  and every fanned-out child; short `-v` forwards through `sync` → `sync-agent`
  fan-out and through `sync-agent`'s own fan-out (beyond T2.6 argv checks).
- [x] **M1** — Classified `· template skipped (exists)` in
  `recipe-materialize.py` as **Noise (keep ·)** (idempotent no-op, same class
  as `symlink ok`; not an ℹ policy notice). Inline comment + compact-filter test.
- [x] **M2** — Test that `ℹ skipped AGENTS.md (brief.render = false)` survives
  compact mode (parallel to the mcp-skipped compact-visibility test).
- [x] **M3/F5** — `print_step_output` / `run_step` in both `lib/sync.sh` and
  `lib/sync-agent.sh` now cat capture files for verbose replay (byte-identical,
  including trailing blank lines). Test proves trailing blanks survive `-v`.
- [x] **L1** — Reworded CHANGELOG fan-out entry so it does not overstate a
  product regression from a prior release (pre-release development fix note
  folded into the compact-output Added bullet).
- [x] **L2** — Added `[-v|--verbose]` to Usage: synopses in `sync.sh` and
  `sync-agent.sh` `--help` text.

## Notes

- Branch is currently 4 commits behind `development` with zero commits of its own; rebase
  before implementation continues.
- `ai-specs/recipes/` is untracked in this worktree (dogfood state). Keep it out of the
  commit; see the `dogfood-verification-isolation` skill.
