# Tasks: Tracker-only Go Ledger foundation

Design (`design.md`, A1–A11) is authoritative for CLI flags, JSON keys, witness `state` enum, and store paths. Spec naming that differs defers to design until a human revises it. Product locks D1–D19 are not reopened. Planning depth: Full (requested Full, signal Full, decided Full — no conflict).

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~2,000–2,600 (additions+deletions) across ~25 files |
| 400-line budget risk | High |
| Chained PRs recommended | No (technical recommendation was Yes; overridden by accepted `size:exception`) |
| Suggested split | Single PR containing the 7 ordered work units as reviewable commit slices |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

## Size Exception Authorization (recorded delivery decision)

**Authorized**: the user explicitly accepted `size:exception` for this change — one oversized PR is delivered instead of chained PRs. This is a maintainer acceptance, not an inference, and it is the reason `Decision needed before apply` is now `No`.

**Rationale (accepted)**: the tracker foundation must land as one atomic migration — a single authoritative Go grader, the durable binding witness that activates it, and the host cutover that stops the old graders from deciding. Chaining it would ship intermediate states that are worse than a big diff:

1. **Temporary duplicate graders.** The `## Tracker` validity rule exists three times today. A PR boundary between the Go authority (units 1–3) and the host cutover/grader retirement (units 5–6) leaves two or three graders live and able to disagree — the exact failure mode the project's strangler policy forbids.
2. **Inert compatibility surfaces.** A PR that lands the Go `--ledger` mode with no witness producer (unit 4), or the witness with no host that reads it, ships dead code plus a fake activation path that reviewers and `doctor` must later unwind. Both were previously merge-safe only because no host consumed the verdict.
3. **Split trust-root churn.** The four-arch `SHA256SUMS` asset is rebuilt whenever the module changes; chaining would regenerate or re-verify the same trust root across several PRs for no review gain.
4. **No usable review focus.** Units are coupled by one contract (flags/JSON/enum/store path); a reviewer of slice 5 must already understand slices 1–4, so per-PR focus would not shrink while reviewing the same semantics repeatedly.

**Conditions carried by the exception** (not waived): all 7 work units stay individually verifiable; each unit's focused test command must be green at its own commit before the next unit starts; `./tests/validate.sh` must be green for the whole branch; no merge to `development` without a PR. Default posture stays `warn`; this repo's dogfood `gate_mode = "warn"` is unchanged. Reviewers should read the PR commit-by-commit in unit order.

## Work Units

Under `size:exception` these are **ordered commit slices inside one PR**, not separate PRs. Each keeps its own start/finish, focused test, runtime harness, and revert boundary so a bad slice can be reverted without dragging unrelated work.

| Unit | Goal | Est. lines | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Go identity + witness decode | ~300 | `go -C catalog/recipes/worktree-flow/gate test ./ledger/...` | `t.TempDir()` git repo (`git init`/`symbolic-ref`) | New `ledger` package deletable; no importer yet |
| 2 | Atomic ledger store + item rules | ~350 | same as unit 1 | `t.TempDir()` `state.json` | Revert `store.go`; nothing reads it in prod |
| 3 | Verdict predicate + `--ledger` CLI/JSON | ~420 | `go -C catalog/recipes/worktree-flow/gate test ./...` then `go build`/`--selftest` | `dist/worktree-gate-current --ledger --checkpoint …` | Revert flag; worktree gate path untouched |
| 4 | Durable witness from sync + synthetic provider | ~300 | `python3 -m unittest tests.test_tracker_ledger_witness -v` | `ai-specs sync` on a temp project via `tests/_fixture_catalog.py` | Delete witness writer; Go reads missing witness as dormant |
| 5 | Five checkpoint hosts invoke; drop heredoc grader | ~420 | `python3 -m unittest tests.test_tracker_card_gate_hook tests.test_premerge_guardian -v` | stamped hook in temp git repo, JSON on stdin, exit 0/2 | Revert host; legacy heredoc returns |
| 6 | One grader: doctor/trello_link retirement + parity corpus | ~450 | `python3 -m unittest tests.test_tracker_ledger_parity tests.test_doctor_tracker_card -v` | built binary driven by pinned corpus | Revert; parity failure re-appears |
| 7 | Docs, trust verification, close-out evidence | ~200 | `./tests/validate.sh` | `scripts/build-gate.sh` + `scripts/verify-gate-sums.sh` | Docs/revert only |

## Phase 1 — Unit 1: identity + witness (Go)

- [x] 1.1 RED: `catalog/recipes/worktree-flow/gate/ledger/identity_test.go` table cases for A2 (common-dir realpath + short branch, exactly-one slug enrichment, several active folders → `change-ambiguous`, archived slug kept via active→dated→legacy order, detached/unborn/no-common-dir → `identity_unavailable`, rename = new identity, two worktrees one identity). <!-- sdd-owner: implementation -->
- [x] 1.2 RED: `catalog/recipes/worktree-flow/gate/ledger/witness_test.go` for `bound`/`ambiguous`/`unbound`/`declared-not-bound`, plus missing/unreadable/unknown-`v` → dormant reason `witness-missing`. <!-- sdd-owner: implementation -->
- [x] 1.3 GREEN: implement `catalog/recipes/worktree-flow/gate/ledger/identity.go` (reuse `catalog/recipes/worktree-flow/gate/gitfacts.go` (read-only) via exported helpers, no tracker types there) and `catalog/recipes/worktree-flow/gate/ledger/witness.go` (read-only decode). <!-- sdd-owner: implementation -->
- [x] 1.4 TRIANGULATE: add `archive-aware` slug fixture asserting `tests/_change_paths.py::change_dir` (read-only) semantics match Go. <!-- sdd-owner: implementation -->
- [x] 1.5 REFACTOR + commit: no worktree `Decide`/`Event`/cleanup imports from `ledger` (verify by grep); gofmt clean. <!-- sdd-owner: implementation -->

## Phase 2 — Unit 2: store (Go)

- [x] 2.1 RED: `catalog/recipes/worktree-flow/gate/ledger/store_test.go`: missing file = empty items; corrupt JSON = `unevaluable`/`store-corrupt`, never synthesized; atomic `state.json.tmp.*` + rename leaves no temp residue; identity key `common_dir+"\x1f"+branch[+"\x1f"+change]`. <!-- sdd-owner: implementation -->
- [x] 2.2 RED: item rules — closed never reopened, reused branch opens a NEW item (D17), two `open` rows = conflict not pick, provider-neutral fields only (opaque `provider` unread), append-only `decisions[]` kinds `adjudicate|opt-out|open|close|link`, `opt-out` scoped to one checkpoint (D19), advisory ceiling 32 items / 64 KiB (no compaction). <!-- sdd-owner: implementation -->
- [x] 2.3 GREEN: implement `catalog/recipes/worktree-flow/gate/ledger/store.go` at `<git-common-dir>/ai-specs/ledger/state.json` (A3). <!-- sdd-owner: implementation -->
- [x] 2.4 TRIANGULATE/REFACTOR: concurrent-append test (two writers, one replace wins, no lost decision); commit. <!-- sdd-owner: implementation -->

## Phase 3 — Unit 3: verdict + CLI

- [x] 3.1 RED: `catalog/recipes/worktree-flow/gate/ledger/verdict_test.go` — 5 checkpoints × 3 modes table for the design posture matrix incl. `always` blocking missing/conflicted (`needs-item`) and pre-merge `identity_unavailable`, `warn` never blocking, `ask` → `decision=ask` exit 0. <!-- sdd-owner: implementation -->
- [x] 3.2 RED: `catalog/recipes/worktree-flow/gate/ledger/decide_test.go` — four-side evidence disagreement records conflict with no default winner; `--decide` persists then re-grade allows; failed persist exits 2. <!-- sdd-owner: implementation -->
- [x] 3.3 RED: `catalog/recipes/worktree-flow/gate/ledger_cmd_test.go` — flag surface (`--ledger --checkpoint --ledger-mode --project-root --witness --store --evidence --decide --explain`), exact JSON keys from design, exit 0 for `allow|ask|dormant|unevaluable` and 2 only for `block`, flag-parse fail-open on verdict calls. <!-- sdd-owner: implementation -->
- [x] 3.4 GREEN: implement `catalog/recipes/worktree-flow/gate/ledger/verdict.go`, `catalog/recipes/worktree-flow/gate/ledger/decide.go`, `catalog/recipes/worktree-flow/gate/ledger_cmd.go`, and the `--ledger` switch in `catalog/recipes/worktree-flow/gate/main.go` (cleanup-mode precedent). <!-- sdd-owner: implementation -->
- [x] 3.5 GREEN: extend `selftest` in `catalog/recipes/worktree-flow/gate/main.go` to assert ledger identity/verdict invariants in-process, still printing `ok` and keeping gate regexp compiles. <!-- sdd-owner: implementation -->
- [x] 3.6 TRIANGULATE: `go build` + `dist/worktree-gate-current --ledger --checkpoint work-start --ledger-mode warn` on this repo; confirm existing `--explain`/worktree corpus output unchanged. <!-- sdd-owner: implementation -->
- [x] 3.7 REFACTOR + commit: mode/enum helper names, no coupling to worktree gate semantics (D4/A1). <!-- sdd-owner: implementation -->

## Phase 4 — Unit 4: durable witness (Python bridge)

- [x] 4.1 RED: `tests/test_tracker_ledger_witness.py` — after sync, `<git-common-dir>/ai-specs/ledger/witness.json` exists with the four states, candidate ids for `ambiguous`, no provider guessed, and survives the `RESOLVED_CONFIG_TEMP` EXIT trap. <!-- sdd-owner: implementation -->
- [x] 4.2 RED: add synthetic provider `tests/fixtures/recipes/test-tracker-ledger/recipe.toml` declaring `tracker` (gated by `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES`) + `tests/fixtures/recipes/test-tracker-ledger-conflict/recipe.toml` for the ambiguous case. <!-- sdd-owner: implementation -->
- [x] 4.3 GREEN: add the atomic `mkstemp`+`os.replace` witness writer to `lib/_internal/recipe-materialize.py` right after `resolve_bindings`/`check_capability_conflicts`; keep it the only binding producer. <!-- sdd-owner: implementation -->
- [x] 4.4 GREEN: confirm `lib/sync.sh` deletes only `RESOLVED_CONFIG_TEMP` (no witness trap, no second temp root). <!-- sdd-owner: implementation -->
- [x] 4.5 TRIANGULATE: worktree case — witness written from the main checkout is read by a linked worktree (common-dir, not realpath cache). <!-- sdd-owner: implementation -->
- [x] 4.6 REFACTOR + commit; `python3 -m py_compile lib/_internal/recipe-materialize.py`. <!-- sdd-owner: implementation -->

## Phase 5 — Unit 5: checkpoint hosts

- [x] 5.1 RED: `tests/test_ledger_mode_config.py` for A9 mapping (`ledger_mode` wins; `gate_mode` off→skip, warn→warn, always→always; worktree `gate_mode` never read) against `catalog/recipes/trello-mcp-workflow/recipe.toml`. <!-- sdd-owner: implementation -->
- [x] 5.2 GREEN: add `[config.ledger_mode]` enum `always|ask|warn` default `warn` to `catalog/recipes/trello-mcp-workflow/recipe.toml` and its README config table in the same commit. <!-- sdd-owner: implementation -->
- [x] 5.3 GREEN+RED: `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` resolves the verified binary and grades `work-start` (no change folder required; `openspec/**` never blocked). <!-- sdd-owner: implementation -->
- [x] 5.4 GREEN+RED: `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` grades `apply-start` (`kind=path`) and `pr-review` (`kind=shell` `pr_create`), delete the heredoc `## Tracker` grader, prompt on `ask`, persist the answer via `--decide`; update `tests/test_tracker_card_gate_hook.py`. <!-- sdd-owner: implementation -->
- [x] 5.5 GREEN+RED: `lib/_internal/premerge_guardian.py` invokes `pre-merge` and `archive-close` at both stages, tolerates cold `AI_SPECS_HOME`, leaves tier/verify-evidence math untouched; update `tests/test_premerge_guardian.py`. <!-- sdd-owner: implementation -->
- [x] 5.6 TRIANGULATE: all five hosts return identical verdict/exit for one pinned fixture input (spec "All five checkpoints reach one predicate"). <!-- sdd-owner: implementation -->
- [x] 5.7 REFACTOR: hosts are acquisition/JSON bridges only — grep-assert no `is_valid_link`-style predicate added in shell or Python; point `catalog/recipes/git-pr-flow/commands/pr-create.md` at the verdict; commit. <!-- sdd-owner: implementation -->

## Phase 6 — Unit 6: one grader + parity corpus

- [x] 6.1 GREEN+RED: `lib/_internal/doctor.py` renders the JSON `doctor` finding under check `tracker-ledger` with A10 severities (INFO unbound, WARN ambiguous/declared-not-bound/missing-witness/conflict, ERROR infra) and stops grading; drop `_check_tracker_card_link` as a grader; update `tests/test_doctor_tracker_card.py`; assert no runtime-brief/`AGENTS.md` dormancy line (D15). <!-- sdd-owner: implementation -->
- [x] 6.2 GREEN: `lib/_internal/trello_link.py` stays a parser — comments + no new predicate; existing consumers delegate to the Go verdict. <!-- sdd-owner: implementation -->
- [x] 6.3 RED: pinned `tests/fixtures/tracker-ledger-corpus/*.json` covering every design test-table row (dormant, empty store × modes, consistent evidence, four-side conflict, persisted `--decide`, checkpoint-scoped opt-out, branch reuse → new item, two open items, detached HEAD per mode, `openspec/**` never blocked, missing binary → exit 0). <!-- sdd-owner: implementation -->
- [x] 6.4 GREEN: `tests/test_tracker_ledger_parity.py` drives `dist/worktree-gate-current --ledger` per fixture (mirror `tests/test_worktree_gate_parity.py` (read-only) shape) and fails on any host/Go divergence. <!-- sdd-owner: implementation -->
- [x] 6.5 TRIANGULATE: run corpus twice; byte-identical verdicts and no store residue; confirm existing worktree-gate corpus unchanged. <!-- sdd-owner: implementation -->
- [x] 6.6 REFACTOR + commit. <!-- sdd-owner: implementation -->

## Phase 7 — Unit 7: docs, trust, close-out

- [ ] 7.1 GREEN: update `docs/capabilities.md`, `docs/runtime-hooks.md`, `README.md`, `catalog/recipes/trello-mcp-workflow/README.md`, and `CHANGELOG.md` with the witness/store paths, five checkpoints, modes, dormancy-in-doctor, and "no provider writes in this slice". <!-- sdd-owner: implementation -->
- [ ] 7.2 VERIFY: `scripts/build-gate.sh` then `scripts/verify-gate-sums.sh` — same four assets, one trust root; regenerate `catalog/recipes/worktree-flow/bin/SHA256SUMS` only with canonical `go1.24.13`, else record the pending-release note. <!-- sdd-owner: implementation -->
- [ ] 7.3 VERIFY: `./tests/validate.sh` (py_compile, `bash -n`, gofmt, Go tests incl. `ledger`, unittest discovery) fully green; no runner path edits were needed because the module did not move. <!-- sdd-owner: implementation -->
- [ ] 7.4 GREEN: write `openspec/changes/tracker-ledger-foundation/verify-report.md` with one `Criterion N: PASS` row per the 11 proposal Success Criteria, plus RED/GREEN evidence per unit. <!-- sdd-owner: implementation -->
- [ ] 7.5 GREEN: append the Judgment Day input set to `openspec/changes/tracker-ledger-foundation/verify-report.md` (spec scenario → corpus fixture → test name) for up to 3 authorized rounds; record each round's fix re-run. <!-- sdd-owner: implementation -->
- [ ] 7.6 VERIFY: `python3 lib/_internal/premerge_guardian.py --root . --stage pre-archive` then `--stage pre-merge` pass; archive this change folder to `openspec/changes/archive/2026-09-13-tracker-ledger-foundation/` on the review branch and re-run the guardian after the move. <!-- sdd-owner: implementation -->
- [ ] 7.7 VERIFY: open ONE PR for the whole change with `gh` (base `development`, accepted `size:exception`), keeping the 7 work units as separate commits in unit order, no merge, and record the PR URL + total changed-line count + per-commit line counts in `openspec/changes/tracker-ledger-foundation/verify-report.md`. <!-- sdd-owner: implementation -->

## Dependencies / order

1 → 2 → 3 (Go core, no prod effect) → 4 (witness producer) → 5 (hosts) → 6 (grader collapse + parity) → 7 (docs/trust/close-out). Unit 5 depends on 3 + 4; unit 6 depends on 5. Units 1–3 are individually revert-safe with the ledger inactive (no witness = dormant). Under `size:exception` this order applies to commits within the single PR; do not reorder units 5 and 6, or the branch briefly carries two `## Tracker` graders.

## Tracker

- card_id `6aa60e32aee4c220de4b1887` · https://trello.com/c/0Tv0HZ6Q/125-epic-tracker-ledger-foundation (epic, context only)
