# Verify report: worktree-gate-bash-retire

## Verify evidence

- Verdict: PASS
- Command: ./tests/validate.sh
- Exit code: 0
- Date: 2026-08-29
- Commit: c3a867f
- ready_for_archive: true

- **Change**: `worktree-gate-bash-retire`
- **Workspace**: `.worktrees/worktree-gate-bash-retire`
- **Branch**: `change/worktree-gate-bash-retire`
- **HEAD verified**: `4f77415` (validate re-run by verify on `c3a867f` tree)
- **Artifact store**: openspec
- **Strict TDD**: active (`openspec/config.yaml` `strict_tdd: true`)

## Success-criteria mapping

- Criterion 1: PASS — dist/gate-binary/doctor suites reject `bash` and accept only `auto | go` (see R1/R2 table below)
- Criterion 2: PASS — launcher fail-open single warning, planted legacy sentinel never exec'd (`tests/test_worktree_gate_hook.py:990,1013,1031`)
- Criterion 3: PASS — sync materializes no legacy gate; corpus pins `mv a b 2>&1` as Go-only (`harness_phase4.py:412`, tokenizer corpus:525-531)
- Criterion 4: PASS — full suite exit 0, 1785 tests, zero skips, reproduced twice (apply 602.9s, verify 530.1s)

Verification was performed against the tree, not against the apply report's
claims. Every assertion below was re-derived by reading the named files and by
running the suite in this session.

## Structured status and actionContext findings

The parent native status reported `blocked` with
`Change selection is ambiguous: agent-assisted-recipe-config,
retire-decision-matrix, worktree-gate-bash-retire, worktree-gate-go,
worktree-gate-internal-uris` and `changeName: null`. That ambiguity is a
selector-level condition across five open changes, not a defect in this change:
the parent named `worktree-gate-bash-retire` explicitly and acquired attempt
authority (`state: proceed`). All artifacts for the named change resolve on
disk under `openspec/changes/worktree-gate-bash-retire/`, so verification
proceeded on the named change.

`actionContext.mode` = `repo-local`; `allowedEditRoots` =
`/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/worktree-gate-bash-retire`.
Every changed file resolves inside that root. `actionContext.warnings` empty.
Working tree clean (`git status --porcelain` empty) before and after the suite
run; nothing staged.

## Artifact availability

Full artifacts present: proposal, design, spec delta, tasks, apply-progress.
No graceful-degradation path was needed; tasks, spec, and design coherence were
all verified.

## R1 — Spec-scenario map re-derived from the tree

**Verdict: PASS.** Every claim in apply-progress §Spec-scenario map maps to a
real assertion in a currently-passing test. Assertions are behavioral
(sentinel-based, count-based, digest/severity-based) — no tautologies, no
type-only assertions, no smoke-only tests, no ghost loops, no
implementation-detail assertions.

| Spec scenario | Test evidence | Verdict |
|---|---|---|
| Explicit bash configuration is rejected | `tests/test_worktree_gate_dist_config.py:110` `test_gate_impl_bash_rejected_at_sync` — asserts `returncode == 1`, and message contains `bash`, `removed`, `auto \| go`; also asserts `self._legacy_hook(proj).exists()` is False. Reinforced at `:184` (`merge_config` raises `bash.*removed`) and `:403` `test_gate_impl_bash_rejected_instead_of_rollback_rehearsal` (the converted predecessor rollback-rehearsal test). Production: `lib/_internal/recipe-materialize.py:486` error string, `:480` `GATE_IMPL_VALUES = ("auto", "go")`, `:613` guard. | PASS |
| Acquisition has no Bash branch | `tests/test_gate_binary_dist.py` (modified in diff). Production re-derived: `lib/_internal/gate_binary.py` has **zero** `gate_impl == "bash"` branch — the only surviving `bash` tokens are docstrings at `:23` and `:393` stating "There is no Bash fallback"; `_degradation_hint` (`:617-619`) no longer promises auto→Bash. | PASS |
| Launcher fail-open, exactly one warning, planted legacy never exec'd | `tests/test_worktree_gate_hook.py:990` `test_unresolved_binary_fails_open_one_warning_never_execs_legacy` — plants an executable `worktree-gate-legacy.sh` emitting `SENTINEL-LEGACY-MUST-NOT-EXEC` and `exit 2`, asserts `returncode == 0`, `assertNotIn(sentinel)`, and `assertEqual(len([lines with "no usable gate"]), 1)` plus the three recovery tokens (`ai-specs sync`, `--refresh-gates`, `ai-specs doctor`). Also `:1013` `test_launcher_does_not_branch_on_stamped_gate_impl_to_select_bash` (stamped `gate_impl="bash"` still refuses to exec the planted script) and `:1031` (retired `test_legacy_fallback_under_derived_root`, now asserting inertness). Duplicated at harness level: `tests/test_worktree_gate_harness_phase4.py:412` `test_missing_binary_fails_open_without_legacy_exec` with `SENTINEL-HARNESS-LEGACY-MUST-NOT-EXEC` and the same single-warning count. | PASS |
| Existing resolution order preserved above the fail-open floor | `tests/test_worktree_gate_hook.py` `test_worktree_gate_bin_override_wins`, `test_cache_precedence_when_project_local_missing`, `test_project_local_resolves_with_no_fail_open_warning` — each asserts the correct `MARKER:*` and `assertNotIn("no usable gate")`. Production: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh:129-156` `_resolve_binary` implements env → project-local → version-keyed cache and returns 1; `:172-173` is the single warning + `exit 0`. Fallback #4 (`local_legacy=`) is gone. | PASS |
| Ordinary sync never writes/classifies a legacy gate | `tests/test_worktree_gate_dist_config.py:141` `test_ordinary_sync_never_creates_legacy_hook` (cleanup override still materializes; legacy hook absent) and `:139` (`gate_impl=go` path). Production: `materialize_legacy_gate`, `LEGACY_HOOK_REL`, and the call site are absent from the entire tree (grep over `lib/ catalog/ tests/` returns only a test *message string* at `dist_config.py:139`). | PASS |
| Doctor: bash → ERROR, leftover → INFO + rm hint, missing binary → ERROR, no rollback lever | `tests/test_doctor_worktree_gate.py:98` (config bash → exactly 1 ERROR containing `retired`/`auto`/`go`/`sync`, and `assertNotIn("rollback lever")` over the full check blob), `:110` (stamped `gate_impl="bash"` in the launcher → retired ERROR), `:120` (leftover file → exactly 1 INFO containing `leftover` + guidance `rm ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh`, and `assertNotIn("stale")` proving it is not classified as a governed stale asset), `:141`/`:151` (`go` and `auto` without binary → ERROR `failing open`, `assertNotIn("Bash")`, `assertNotIn("rollback lever")`). Production: `lib/_internal/doctor.py:771-780` severity table, `:829-835` leftover INFO, `:837-843` retired ERROR, `:849-855` fail-open ERROR. `_check_worktree_gate` only appends `Check` objects — read-only confirmed. | PASS |
| Tokenizer corpus pins `mv a b 2>&1` against the Go/shlex oracle | `tests/fixtures/worktree-gate-tokenizer-corpus.json:525-531` — `{"cmd": "mv a b 2>&1", "tokens": ["mv","a","b","2>&1"]}` (109 cases total). That is exactly `shlex.split("mv a b 2>&1", posix=True)`, i.e. the Go tokenizer contract, **not** the retired Bash tokenizer behavior. `tests/test_worktree_gate_tokenizer.py` runs a two-sided differential: the local `shlex.split` re-derives every pinned answer (`:60`, `:90-99`) and the Go binary's `--tokenize` JSON diagnostic is compared (`:66-74`, `:121-133`). | PASS |
| Parity / metrics are Go-only | `tests/test_worktree_gate_parity.py` — the `LEGACY`/`materialize_legacy` runner half is deleted; `require_go_binary()` (`:32`) is the only skip source and `GO_BINARY = ROOT/"dist"/"worktree-gate-current"` (`:316`) is the only SUT. `tests/test_worktree_gate_metrics.py` — no Go-vs-Bash git-call comparison; every skip is `"no Go gate binary in dist/"` (`:175, :220, :264, :317, :361`). No suite executes or compares against the retired script anywhere in the tree. | PASS |
| MODIFIED freshness/launcher scenarios stay green | Covered by the unchanged freshness suites plus `tests/test_worktree_gate_harness_phase4.py` and `tests/test_worktree_gate_dist_config.py` (e.g. `test_sentinel_upgrade_replaces_pre_go_gate` at `:195` still exercises unknown-provenance preserve + explicit-refresh upgrade with backup). Whole-suite GREEN under R3. | PASS |

**Skip audit (spec: "no suite MUST skip Go coverage because a Bash reference is
missing").** The full run reported plain `OK` — **zero skipped tests**
(`/tmp/verify-fg.log:7397`; a skip would render as `OK (skipped=N)`). Every
Go-dependent parity/tokenizer/metrics case actually executed against
`dist/worktree-gate-current`. Additionally, every remaining `skipTest` string in
those three suites names the *Go binary* as the missing input, never a Bash
reference.

## R2 — Exit-criteria sweep

**Verdict: PASS.**

| Exit criterion | Evidence |
|---|---|
| `gate_impl = bash` no longer exists in config | `catalog/recipes/worktree-flow/recipe.toml:73` `enum = ["auto", "go"]`; `:74` help_text rewritten ("The bash value has been removed"). `lib/_internal/recipe-materialize.py:480` `GATE_IMPL_VALUES = ("auto", "go")`. |
| ...in gate binary resolution | `lib/_internal/gate_binary.py` — no `bash` branch; `acquire` runs the same path for `auto` and `go`. |
| ...in the launcher | `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` — no `local_legacy`, no fallback #4; header resolution list (`:20-30`) documents 3 steps + fail-open floor. |
| ...in doctor | `lib/_internal/doctor.py` — the rollback-lever INFO and the auto→Bash WARN are gone; the only `bash` occurrences (`:776`, `:842`) are the **required** retired-value ERROR diagnostic. |
| ...in the catalog | `catalog/recipes/worktree-flow/hooks/` contains only `worktree-gate.sh`. `worktree-gate-legacy.sh` is `D` in `git diff --name-status 313c6d2..HEAD` (547 deleted lines). No `legacy` reference in `recipe.toml`. |
| ...in docs | `docs/runtime-hooks.md:110` (`auto \| go` only), `:113-125` 3-step chain + fail-open floor, `:131-135` "Recovery (no Bash rollback path)". `docs/recipes-catalog.md:284-293, :313`. `catalog/recipes/worktree-flow/README.md:70, :90-93`. `CHANGELOG.md:11-14` breaking note. |
| Launcher fails open with one stderr warning | `worktree-gate.sh:172-173` — single `echo ... >&2` then `exit 0`. Count-asserted by two independent suites (see R1). |
| All parity/tokenizer/hook/dist/doctor suites pass Go-only | R3: 1785 tests, 0 failures, 0 skips. |

**Residual `bash` string occurrences are intentional and spec-required**, not
survivals of the retired path: the rejection message
(`recipe-materialize.py:486`), the doctor retired-value ERROR
(`doctor.py:842`), the docs/CHANGELOG breaking notes, and the test names that
assert rejection. No enum member, branch, materialization path, or fallback
step accepts `bash`.

## R3 — Full suite, own evidence

**Verdict: PASS.**

Commands run in this verification session, in this worktree, at `4f77415`:

| Command | Result |
|---|---|
| `./tests/validate.sh` (foreground, exit code captured) | **exit 0** — `Ran 1785 tests in 530.054s` / `OK` (0 failures, 0 errors, **0 skips**). Includes `python3 -m py_compile lib/_internal/*.py tests/*.py`, `bash -n lib/*.sh bin/ai-specs tests/*.sh`, `gofmt -l catalog/recipes/worktree-flow/gate` (no output = clean), `tests/test_vault_fs_mcp.sh` (all `ok -`), `go -C catalog/recipes/worktree-flow/gate test ./...` → `ok ai-specs.dev/worktree-gate (cached)`, then `python3 -m unittest discover -s tests -p 'test_*.py'`. Log: `/tmp/verify-fg.log`. |
| `./tests/validate.sh` (earlier independent run, same tree) | `Ran 1785 tests in 564.515s` / `OK` — reproducible. Log: `/tmp/verify-validate-2537.log`. |
| `git status --porcelain` (before and after) | empty — the suite left no working-tree residue and nothing is staged. |

The apply-progress claim of `Ran 1785 tests ... OK` is **reproduced
independently**, twice, with a captured exit code of 0.

## R4 — Scope guard

**Verdict: PASS.** `git diff --name-only 313c6d2..HEAD` = 25 files: 21 product
files + 4 planning artifacts. Out-of-scope surfaces are untouched:

- `lib/_internal/hooks-render.py` — **not in the diff**.
- Runtime extension / wrapper templates (`.opencode`, `.pi`, `.omp`, Cursor) — **not in the diff**.
- `catalog/recipes/trello-mcp-workflow/**` (tracker-card-gate) — **not in the diff**.
- Go gate policy: `catalog/recipes/worktree-flow/gate/decide.go`, `extract*.go`, `topology.go`, `uri.go`, `tokenize.go` — **not in the diff** (no `gate/*.go` file appears at all).
- No automatic deletion of consumer leftover files (design D2) — confirmed: `recipe-materialize.py` has no leftover-removal path; doctor emits INFO + a *manual* `rm` hint.
- No Windows work.

Diff totals: `25 files changed, 1522 insertions(+), 1225 deletions(-)`
(insertions include the 4 planning artifacts; product-only totals per
apply-progress: +497/−1225, within the forecast band of ~1,400–1,800 total).

## R5 — Dogfood evidence

**Verdict: PASS (documented + orchestrator-confirmed; not re-run).**
`apply-progress.md` §Test commands records the task-4.5 dogfood run performed
under `/Users/robert/proyectos/nnodes/ai-specs-cli/.pi/skills/dogfood-verification-isolation/SKILL.md`:
ordinary sync preserved the existing gate ("no recorded provenance; preserving
existing bytes") and fired **no** legacy-materialization path; the materialized
`ai-specs/recipes/worktree-flow/hooks/` contained only `worktree-gate.sh` with
`stamped_gate_impl="auto"` and zero `worktree-gate-legacy` references; doctor
reported no retired-value ERROR; project state was reverted. The parent
orchestrator independently confirmed the same three facts.

I did **not** re-run the dogfood sync: R1–R4 surfaced no doubt requiring it, and
the isolation skill's cost (mutating this repo's own project state) is not
justified. Independent corroboration from the tree instead:
`find ai-specs -name 'worktree-gate-legacy.sh'` → **no match**;
`ai-specs/recipes/worktree-flow/hooks/` contains only `worktree-gate.sh`;
`git diff --stat 313c6d2..HEAD -- ai-specs/` → **empty**, so no dogfood output
leaked into the branch.

## Task completion status

`openspec/changes/worktree-gate-bash-retire/tasks.md`: 32 implementation tasks
(1.1–4.8) are `- [x]`. Exactly one unchecked line remains, and it is
parent-owned, not implementation scope:

```
265:- [ ] 5.1 Start or reuse bounded review. <!-- sdd-owner: parent -->
```

No unchecked `<!-- sdd-owner: implementation -->` task remains. Archive is not
blocked by implementation scope; it is gated only on the parent completing 5.1.

## Strict TDD compliance

**Compliant, with one honest caveat carried from apply.**

- `apply-progress.md` contains the required `TDD Cycle Evidence` table with a row per slice.
- Every test file named in that table exists and was cross-referenced (see R1); every claimed assertion was located by reading the file.
- GREEN was re-confirmed by my own `./tests/validate.sh` run (exit 0, 1785/1785).
- **Caveat (already self-declared by apply, not a concealment):** the first apply session implemented Phases 1–4 and timed out before persisting per-slice RED logs; those RED transcripts are not recoverable. The continuation session verified GREEN on that tree and said so explicitly. The RED intent is nonetheless *structurally* evidenced: several tests are conversions of predecessor tests that asserted the opposite contract and could only have failed before the production edit — `test_gate_impl_bash_rejected_instead_of_rollback_rehearsal` (was the rollback rehearsal asserting bash *materializes* legacy), `test_legacy_fallback_under_derived_root` (was asserting the legacy fallback *runs*, now asserts it must not), and `test_gate_impl_bash_skips_acquisition` → replaced. I record this as a **WARNING**, not a CRITICAL: the evidence gap is disclosed, the behavioral inversion is provable from the diff, and the GREEN state is independently reproduced.

### Assertion quality audit (changed/created tests)

No tautologies, no ghost loops, no type-only-assertion-only tests, no smoke-only
tests, no CSS/implementation-detail assertions. Positive quality signals:

- Negative control via planted sentinel: an executable legacy script that would `exit 2` and print a unique marker is planted, and the test asserts both `returncode == 0` and marker absence. This proves *non-execution*, not merely *non-failure*.
- Exact-count assertions (`assertEqual(len(warnings), 1)`, `assertEqual(len(errors), 1)`, `assertEqual(len(infos), 1)`) rather than `assertIn`-only, so a regression that duplicates a warning fails.
- Negative-content assertions on doctor output (`assertNotIn("rollback lever")`, `assertNotIn("Bash")`, `assertNotIn("stale")`) pin the retired vocabulary out of existence.
- The tokenizer test is a genuine differential (local `shlex` oracle **and** the Go `--tokenize` diagnostic), so the corpus cannot silently drift into pinning the retired Bash behavior.
- `dist_config` asserts the *positive* companion (`cleanup override must still materialize`) alongside the negative, so "nothing materializes" cannot pass as success.

## Review workload / PR boundary

`tasks.md` §Review Workload Forecast recommended chained PRs (High budget risk,
~1,400–1,800 lines). `apply-progress.md` records the user-approved deviation:
**single PR with `size:exception`, no chaining**, with the atomic Phase-3 slice
included. That exception is explicitly recorded, so the guard is satisfied.

No scope creep: the delivered diff matches the assigned tasks exactly (R4), and
actuals (+497/−1225 product) land inside the forecast band. Reviewer burden is
materially lighter than the raw line count suggests — 547 deleted lines are the
single catalog file deletion, and the largest test diffs are deletions of the
Bash half of parity/metrics.

## Findings

No CRITICAL findings. No blockers.

**WARNING**
1. Strict-TDD RED transcripts for the first apply session are unrecoverable (disclosed in apply-progress; behavioral inversion provable from the diff; GREEN independently reproduced). See "Strict TDD compliance" above.

**SUGGESTION**
2. Stale prose in the launcher: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh:56` still says "warn and fall back (mirrors the legacy reference contract)" and `:127-128` says "Prints the command to exec, or nothing when the legacy path applies (the legacy file carries its own stamped values)". Both describe a path that no longer exists. Dead comments only — no executable consequence, and the header block (`:20-30`) is correct. Worth a one-line cleanup on a future touch.
3. `catalog/recipes/worktree-flow/gate/*.go` retain `worktree-gate-legacy.sh:NNN` provenance citations in comments (e.g. `decide.go:21`, `tokenize.go:33`, `message.go:6`). This is explicitly permitted by task 3.2 ("historical line citations may remain in comments only as prose, never as an executable oracle") and was declared out of scope in apply-progress. No suite reads them as an oracle. Recorded for transparency, not action.
4. `go test` reported `(cached)`. Cache reuse is legitimate here because no `gate/*.go` file changed in this diff (R4), so the cached result describes the current sources. A future verification could force `-count=1` if Go sources are ever in scope.
5. The launcher emits a second, distinct stderr line when `WORKTREE_GATE_BIN` is set but not executable (`worktree-gate.sh:136`) before the fail-open warning at `:172`. This is not a spec violation — the "exactly one warning" scenario is scoped to `WORKTREE_GATE_BIN` unset, and the test counts only `"no usable gate"` lines — but it is the one input shape where stderr carries two gate lines. Pre-existing behavior, intentionally retained.

**INFO**
6. The working-tree `ai-specs/recipes/worktree-flow/README.md` carries post-change dogfood bytes, but `ai-specs/recipes/**` is gitignored (`ai-specs/.gitignore:7`) and the tracked project state (`ai-specs/.ai-specs.lock`) is unchanged. Nothing leaked into the branch.

## Conclusion

**PASS.** The implementation satisfies both MODIFIED requirements and all three
ADDED requirements. The proposal's three exit criteria are met and independently
re-derived from the tree. The suite is green on my own run (exit 0, 1785 tests,
zero skips). Scope is clean. The only outstanding item is the parent-owned task
5.1 (bounded review); archive is not blocked by implementation scope.
