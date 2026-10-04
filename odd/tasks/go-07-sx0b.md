# [Go 07.SX0b] Shared lock-write extraction — feature document

Card: `6ac10f3c93fccd467e0ce9fe` — https://trello.com/c/zFFoclwY (In Progress)
Parent card: `6a84e7acca1baf394d9b482b` — https://trello.com/c/ITeLc5xL
Epic card: [Go single-binary migration](https://trello.com/c/qwlHQ7Xa)
Branch: `change/go-07-sx0b` · Worktree: `.worktrees/go-07-sx0` (reused) · Base: `7dfbc38a4ce1d9cf30d8438bcbb15d67d8d17735` (PR #326 merge)
Branch/worktree verified 2026-10-03: `change/go-07-sx0b` @ `7dfbc38`, tree clean.

## Tracker

- card_id: `6ac10f3c93fccd467e0ce9fe`
- url: https://trello.com/c/zFFoclwY
- list: In Progress
- ledger row: `846c921bcb179dea` (branch-level bind, change/go-07-sx0b, applied 2026-10-03T14:21:03Z)

## Scope (parent brief, binding)

Extract only self-contained `lockwrite.go` into `shared/lockwrite.go` with
exported `RunWriteLock` and minimal required types/helpers for root import.
Thin main dispatch calls `shared.RunWriteLock`. `lockwrite_test.go` stays
package main; full coverage preserved. No helper/domain extraction beyond
lock. Root `go.mod` already wired — no change. Preserve byte-exact TOML,
atomic replace, error/exit/JSON envelope and control-char refusals. S10 owns
the lock-entry-prune port; prune is NOT in SX0b. No `recipe-materialize.py`
or runtime wiring changes. No release/tag. Parent owns commits/push/PR/merge.

Allowed edit surfaces: `gate/lockwrite.go`, `gate/shared/lockwrite.go`,
`gate/shared/jsonstring.go`, `gate/lockwrite_test.go`,
`gate/main.go` (write-lock dispatch only), `gate/copyapply.go`,
`gate/recipeconfigwrite.go`, `gate/recipeconfigwrite_test.go`
(helper definition removal and reference/import updates only),
`internal/sync/gateshared_import_test.go` (lock and approved helper cases),
`catalog/recipes/worktree-flow/bin/SHA256SUMS`, this file.

## Goal

Make the gate's authoritative lock writer importable in-process from the root
binary without changing the gate binary's observable behavior, completing the
lock half of S10's needs (writer side; prune stays S10-owned).

## Tasks

- [x] T1 — Dependency report: exact cross-package dependencies of
      lockwrite.go; surface-extension request to parent if required.
      COMPLETED: pyJSONString finding reported before edits; parent approved
      Option 1 + narrow surface extension (recipeconfigwrite_test.go
      2-line rename).
- [x] T2 — Extraction: `shared/lockwrite.go` (package shared, exported
      `RunWriteLock` + minimal required types/helpers); delete main copy;
      thin dispatch at `main.go:239`; test file stays package main with
      identifier renames; import-test lock cases. Per approved Option 1:
      `pyJSONString` moved verbatim to `shared/jsonstring.go` as
      `PyJSONString` (parent-extended surfaces: copyapply.go +
      recipeconfigwrite.go reference updates, recipeconfigwrite_test.go
      2-line rename). RED→GREEN import evidence recorded.
- [x] T3 — Checks: gate gofmt/vet/build/test; root build/test; RED→GREEN
      import evidence; checksum cycle (build-gate.sh go1.24.13, SHA256SUMS
      regen, verify-gate-sums.sh). All green: gate main 44.535s + ledger
      2.746s; root 17 packages ok; import cases 7/7 PASS; digests regenerated
      (darwin-amd64 4cd6df04…, darwin-arm64 3a77ff71…, linux-amd64 795c1e1b…,
      linux-arm64 8c9bc72e…); verify-gate-sums.sh ok; smoke --write-lock ok.
- [x] T4 — Full validation via verifier: `./tests/validate.sh`, both parity
      modes, detached run with exit marker. CONCLUSIVE:
      `/tmp/sx0b-validate.log` `VALIDATE_EXIT:0`; parity gate-absent failing=0
      + gate-present failing=0 PASS (19 fixtures/mode); `Ran 2482 tests in
      1993.125s` → OK; 36m44s. Note: bridge-fallback lines show the Python
      lock-write authority in gate-absent fixtures (expected; the Go writer
      path is covered by gate-present parity + the lock unit tests).
- [x] T5 — RDD per current protocol (prefer native; no indefinite failed-START
      retries; no resets; parent decides JD) + parent report. **TERMINAL:
      native review UNAVAILABLE — no lineage, no native approval.** Chain:
      parent committed the exact candidate as
      `0a1f25dfeb47a0733e9342f1df0a9b30f43556de` (clean tree; 10 stat
      entries / 11 path endpoints, R084 lockwrite.go → shared/lockwrite.go,
      342+/126−); committed-range inspect VERIFIED (base-diff kind,
      base_tree == 7dfbc38 tree, candidate_tree == HEAD tree, 11 paths); one
      START attempt with explicit baseRef + committedOnly — provider ignored
      the input and offered the wide ambient candidate (bootstrap base
      `e773514`, ~250 paths, target `4dd9bc30`, stale binding `7bc6e0e4`,
      `native_invocation_attempted:false`, `lineage_created:false`); unchanged
      retries stopped per instruction. ASSESS (explicit unavailable):
      risk medium (executable_change: SHA256SUMS), 11 paths / 1184 lines,
      candidate `consumed:false`, writerProfile large (runtime); its
      `independentVerifier:false` does NOT reduce the human's mandatory
      verifier bar. **Fallback SATISFIED: independent verifier
      `mut4mtxt-7-xtun` PASS** (scope/source/functional evidence, pre-commit,
      this exact candidate: exact body comparison, fresh focused/vet/format,
      digest reproduction, full suite corroborated). This is NOT native
      approval. Binding limits: the full-suite log
      (`/tmp/sx0b-validate.log`, `VALIDATE_EXIT:0`, parity both modes zero
      deltas, 2482 tests OK) predates the commit and carries no tree hash;
      log-to-commit equivalence rests on the clean tree and the parent
      verifier's corroboration. Zero lineages created by this session.
      JD and delivery: parent (push/PR/merge of `0a1f25d`).

## Dependency finding (T1, reported before edits)

`lockwrite.go` calls `pyJSONString(...)` — defined in
`recipeconfigwrite.go:214` (the `--write-recipe-config` domain, OUT of SX0b
surfaces) and also used by `copyapply.go` (U6/SX0c) and
`recipeconfigwrite.go` itself. The test file does not use it. This is the
sortedUnique class: a shared helper required by the extracted domain while
its sibling callers stay out of scope. Options reported to the parent:

1. **Move `pyJSONString` into shared as `PyJSONString`** with reference
   updates in `copyapply.go` + `recipeconfigwrite.go` (two files outside the
   current surfaces — requires explicit surface extension; single authority
   from day one; SX0c never re-touches).
2. **Temporary duplicate in shared** (no new surfaces) — two authorities,
   against the standing no-duplicate rule.

Parent approved Option 1 and the required test references before source edits.
The helper move is complete and independently verified.

## Risks / traps

- Byte-exact TOML emitter + control-char refusals (GO-08) must move verbatim.
- `pyJSONString` semantics (backslash → quote → control escapes, CPython
  json.dumps-compatible) must be preserved exactly wherever it lands.
- Atomic replace + temp-file mode parity (CreateTemp 0600 = mkstemp default).

## Evidence

Independent verifier `mut4mtxt-7-xtun` confirmed the eleven-path candidate.
The extraction preserves the base behavior. Shared import tests passed 7/7.
Gate lock tests, root/gate vet and formatting checks passed.
Canonical Go 1.24.13 builds reproduced all four regenerated digests.
The verifier corroborated the full-validation log without repeating the suite.
The log has no source-tree hash; fresh focused checks support the recorded result.
Nonblocking note: `WriteLockFile` is exported without an external caller.
Parent delivery and native review remain pending.
