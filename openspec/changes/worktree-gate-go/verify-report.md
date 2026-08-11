# Verify Report: worktree-gate-go

**Status: PASS (conditional)** — findings F1-F8 resolved in the worktree; F9
recorded as a technical decision (no history rewrite, no force-push); **round 3
blockers F10-F12 resolved** (scrub parity + release CI toolchain/cache, §7c).
Full suite green. This report is the record of the pre-fix FAIL state and the
fix evidence below.

**Fix state (2026-08-10, round 2)**: F1-F5 resolved with code/tests; F6-F7
recorded; **F8 resolved** (CI checksum gate canonicalized —
`scripts/verify-gate-sums.sh`, workflow updated, regression test green);
**F9 documented as a decision** — the 3.5 MB blob stays in the PR-chain
history by design until the open PRs merge; no rewrite or force-push without
separate authorization. Working tree contains the fix set (uncommitted, no
push); HEAD unchanged at `d2e40e6`.

**Round 3 (2026-08-11)**: F10 (Go scrub parity) fixed in `gate/extract.go`
with unit + hook tests; F11 (CI toolchain pin → go1.24.13) and F12 (go.sum
cache reference removed) fixed in the release workflow; digests regenerated
with the canonical toolchain; §7c records the full evidence.

_Original FAIL report follows for the record — see §7a "Fix evidence" for the
resolved state of each finding, §7b for round-2 findings F8/F9, §7c for
round-3 findings F10-F12._

- Verified worktree: `.worktrees/worktree-gate-go-phase-4`, branch `change/worktree-gate-go-phase-4`, HEAD `d2e40e6`
- Baseline: `development` @ `e080483`
- PR chain: #191 (base `development`, phases 0-2 / `e290efa`), #192 (base phase-0-2, phase 3 / `697a42c`), #193 (base phase-3, phase 4 / `d2e40e6`)
- Working tree: fix set present (uncommitted); no code or checkbox modified by this verification beyond the fix set.

---

## 1. Verdict per phase

| Phase | Tasks | Verdict | Basis |
|-------|-------|---------|-------|
| Phase 0 — Infrastructure | 0.1-0.10 | **PASS (with WARNING)** | All infra present and green. `0.8` intent leaked: a stray build artifact was committed (F1). |
| Phase 1 — Reference + parity oracle | 1.1-1.23 | **PASS (with WARNING)** | Frozen reference pinned by digest; 16-case corpus + tokenizer differential green. One dead skip-test with a now-false comment (F4). |
| Phase 2 — Go implementation to parity | 2.1-2.18 | **FAIL** | 2.1-2.10, 2.12-2.17 verified green. **2.11 not implemented** (F2); **2.18 partial** (F3). |
| Phase 3 — Distribution and configuration | 3.1-3.20 | **PASS (with WARNING)** | 29 dist/config/doctor tests green. `3.4` resolution order has no automated positive `WORKTREE_GATE_BIN` test (F5). |
| Phase 4 — Harness coverage, docs, cutover | 4.1-4.12 | **PASS** | 10 harness + 6 release tests green; docs, CHANGELOG, `VERSION` 0.22.0, follow-up slug recorded. |

No Phase 5 exists. Sync/archive not started and not attempted.

## 2. Verdict per verification item

| Item | Verdict | Evidence |
|------|---------|----------|
| V.1 `./tests/run.sh` green | **PASS** | `Ran 1539 tests in 400.443s` / `OK (skipped=1)` (invoked by `validate.sh`) |
| V.2 `./tests/validate.sh` green | **PASS** | `VALIDATE_EXIT=0`, two independent runs (6m58s, 6m42s) |
| V.3 `go vet` / `go test` green | **PASS (note)** | `vet_exit=0`; `go test -count=1 ./...` → `ok ai-specs.dev/worktree-gate 2.768s`; `gofmt -l` empty. `run.sh` wires `go test` only — `go vet` runs in CI and manually, never in `run.sh`. |
| V.4 Parity corpus identical both impls | **PASS** | `test_go_comparison_matches_bash_for_available_binary` — all 16 corpus cases, asserts equal `returncode`/`stdout`/`stderr` |
| V.5 Hook suite green both parameterizations | **PASS** | 78 Bash + 78 Go = 156 scenarios in `test_worktree_gate_hook.py` |
| V.6 Every spec scenario mapped to a passing test | **FAIL** | 31 of 35 scenarios mapped; **4 unmapped** (F2, F5) |
| V.7 Unavailable quality signals stated | **PASS** | Stated in §5, matching `openspec/config.yaml:27-39` |
| V.8 Latency **and git call counts** for both impls | **FAIL (partial)** | Latency recorded (prose); **git call counts measured for neither impl** (F3) |
| V.9 No production file outside declared areas | **FAIL** | 3.5 MB stale binary committed inside the catalog (F1) |
| V.10 CI checksum gate canonicalized (F8) | **PASS** | `scripts/verify-gate-sums.sh` (canonical digest comparison); workflow checksum job rewritten; regression test green (§7b) |
| V.11 Stale binary history decision (F9) | **RECORDED** | no rewrite/force-push; tree clean; blob confined to the open PR chain until merge (§7b) |

V.1-V.11 are all checked `[x]` at `tasks.md` (V.10/V.11 added in round 2 for
F8/F9). The original report's V.6/V.8/V.9 FAIL rows above describe the pre-fix
state; §7a/§7b record their resolution.

## 3. Commands executed

```
./tests/validate.sh                                       exit 0  (2 runs: 6m58s, 6m42s)
  → python3 -m py_compile lib/_internal/*.py tests/*.py   clean
  → bash -n lib/*.sh bin/ai-specs tests/*.sh              clean
  → gofmt -l catalog/recipes/worktree-flow/gate           (empty)
  → ./tests/run.sh → go test + unittest discover
     Ran 1539 tests in 400.443s — OK (skipped=1)
go -C catalog/recipes/worktree-flow/gate vet ./...           exit 0
go -C catalog/recipes/worktree-flow/gate test -count=1 ./... ok 2.768s  (45 test funcs)
gofmt -l catalog/recipes/worktree-flow/gate                  (empty)
python3 -m pytest tests/test_worktree_gate_parity.py \
  tests/test_worktree_gate_tokenizer.py tests/test_worktree_gate_metrics.py
     11 passed, 1 skipped, 277 subtests passed in 8.26s
python3 -m pytest tests/test_worktree_gate_hook.py \
  tests/test_worktree_gate_harness_phase4.py tests/test_worktree_gate_release_phase4.py \
  tests/test_gate_binary_dist.py tests/test_worktree_gate_dist_config.py \
  tests/test_doctor_worktree_gate.py
     200 passed, 44 subtests passed in 63.56s
go version                                                   go1.24.13 darwin/arm64
python3 -m pytest tests/test_worktree_gate_release_phase4.py \
  tests/test_worktree_gate_dist_config.py tests/test_worktree_gate_parity.py \
  tests/test_worktree_gate_metrics.py                            (round 2)
     32 passed, 79 subtests passed in 33.09s
bash -n scripts/verify-gate-sums.sh                              (clean)
sha256sum worktree-gate-* > SHA256SUMS                           CI simulation
diff -u catalog/recipes/worktree-flow/bin/SHA256SUMS dist/SHA256SUMS
     exit 1  ← the F8 bug: header + order fail a byte-level diff
bash scripts/verify-gate-sums.sh dist/SHA256SUMS \
  catalog/recipes/worktree-flow/bin/SHA256SUMS
     exit 0  "ok — 4 digest entries match the committed trust root"
bash scripts/verify-gate-sums.sh <corrupted> \
  catalog/recipes/worktree-flow/bin/SHA256SUMS
     exit 1  + "regenerate … and commit it" (real mismatch still fails)
```

Round-1 note kept above; the round-2 block (F8) is the release checksum
gate reproduction and the fix validation.

The single skip is `test_worktree_gate_parity.py:295` — see F4. It is **not** the Go differential;
the real differential (`:257`) ran and passed.

## 4. Spec coverage (V.6) — 31/35

| Requirement | Scenarios | Mapped to |
|-------------|-----------|-----------|
| Go gate implementation of record with behavioral parity | 8/8 | `test_worktree_gate_parity.py` (16 cases, both impls), `test_worktree_gate_hook.py` (156), `extract_pass2_test.go`, `pathutil_test.go`, `config_test.go`, `tokenize_diff_test.go` |
| Portable launcher indirection with a stable materialized path | **5/6** | `test_worktree_gate_harness_phase4.py`, `test_worktree_gate_dist_config.py` (sentinel upgrade, bash-3.2), `test_gate_binary_dist.py` (Rosetta mapping). **Unmapped: "Explicit binary override wins"** |
| Binary acquisition, verification and cache layout | 6/6 | `test_gate_binary_dist.py` (13 tests) |
| `gate_impl` configuration | 4/4 | `test_worktree_gate_dist_config.py`, `test_doctor_worktree_gate.py` |
| Multi-arch build matrix and reproducibility | 4/4 | `test_worktree_gate_release_phase4.py` (5 tests) |
| Diagnostics for gate implementation health | 4/4 | `test_doctor_worktree_gate.py` (7 tests, all five severity rows) |
| Gate invocation performance budget | **0/3** | **All three unmapped** |

Unmapped scenarios:

1. **"One process per invocation"** — no test asserts a single implementation process for a
   four-candidate event.
2. **"Git facts are memoized across candidates"** — requires *strictly fewer* `git` invocations
   than the frozen Bash implementation for the same event. No test counts `git` invocations for
   either implementation. See F2.
3. **"No hashing on the hot path by default"** — no test asserts that no digest is computed when
   `WORKTREE_GATE_VERIFY` is unset. `WORKTREE_GATE_VERIFY` appears in no test file.
4. **"Explicit binary override wins"** — `WORKTREE_GATE_BIN` is implemented in the launcher
   (`worktree-gate.sh:95-100`) but appears in tests only as `env.pop("WORKTREE_GATE_BIN", None)`
   (`test_worktree_gate_harness_phase4.py:137`). Coverage is manual smoke only
   (`apply-progress.md` §3.20).

## 5. Unavailable quality signals (V.7)

Per `openspec/config.yaml:27-39`, all `available: false`:

- **Coverage**: none. No coverage percentage can be reported for the Go or Python code.
- **Linter**: none for Python or Bash. `go vet` is the only static analysis available, and it is Go-only.
- **Type checker**: none. `lib/_internal/gate_binary.py` annotations are unverified.
- **Formatter**: none for Python or Bash. `gofmt` is Go-only; and `gofmt -l` **lists** files without
  a non-zero exit, so `validate.sh` would stay green on unformatted Go.
- Integration and e2e layers: `available: false`. Only the `unittest` unit layer exists.

## 6. Performance evidence (V.8)

| Implementation | Per-invocation (median) | Source |
|---|---|---|
| Go binary | 48.5 ms | `apply-progress.md` §2.18 (prose) |
| Bash reference | 145.3 ms | `apply-progress.md` §2.18 (prose) |

**Git call counts: not measured for either implementation.** No test or script counts `git`
subprocess invocations. This is the substance of both task 2.11 and the "Git facts are memoized
across candidates" scenario. `gitMemo` exists at `gitfacts.go:24` and is plausibly correct by
inspection, but memoization is unproven by test. See F2/F3.

## 7. Findings

### F1 — CRITICAL: 3.5 MB stale binary committed inside the catalog (blocks V.9)

`catalog/recipes/worktree-flow/gate/worktree-gate` — a **3.5 MB Mach-O 64-bit arm64 executable**,
added in `e290efa`, mode `100755`, blob `d9656d1`.

Proof it is stale and unverifiable:

```
./catalog/recipes/worktree-flow/gate/worktree-gate --version   → dev
./dist/worktree-gate-current --version                         → 0.22.0
```

It is a stray default `go build` output (binary named after the module directory). It:

- **contradicts the committed `catalog/recipes/worktree-flow/bin/README.md`**, which states
  verbatim: *"**No binaries are ever committed here** (design D4: `git clone` is the install
  channel; binaries in git would cost every user ~15-25 MB per release forever)"*;
- has **no entry in `SHA256SUMS`**, so it is an unverified executable inside a change whose entire
  trust model (D5) is digest-verified acquisition;
- is **arm64-only** and reports version `dev`, i.e. already drifted from source;
- escaped `.gitignore`, which covers `dist/` (line 43) but not `gate/worktree-gate`;
- is **not** used by any test (all suites key off `dist/worktree-gate-*`) and is **not** materialized
  by `recipe.toml` — pure dead weight shipped through the `git clone` install channel.

This is a release blocker and the reason V.9 fails.

### F2 — CRITICAL: task 2.11 not implemented (blocks V.6 and V.8)

Task 2.11 requires: *"`gitfacts` test asserting memoization: a four-candidate event issues strictly
fewer `git` invocations than the Bash implementation, counted via a `git` shim on `PATH`."*

`catalog/recipes/worktree-flow/gate/gitfacts_test.go` is **32 lines with two tests**
(`TestGitFactsAndCommon`, `TestGitFactsInvalidFailsOpen`). It contains **no `git` shim, no
invocation counting, and no comparison against the Bash implementation**. The task is checked
`[x]` and `apply-progress.md` §2.18 claims *"`gitfacts_test.go` pins the memoization contract"* —
that claim is **not supported by the file**.

### F3 — CRITICAL: task 2.18 partial + tautological assertions (blocks V.8)

`tests/test_worktree_gate_metrics.py` is the only automated performance evidence. It measures the
**Go side only** and its assertions cannot fail:

- `:40` `self.assertTrue(all(t >= 0 for t in timings))` — **tautology**: `time.perf_counter()`
  deltas are always `>= 0`. No plausible regression can trip it.
- `:38` `self.assertIn(result.returncode, (0, 2))` — accepts **both allow and block**, so it cannot
  detect a wrong gate decision.

It records no Bash timing and no `git` counts. The V.8 numbers exist only as prose in
`apply-progress.md`, unreproducible from the suite.

### F4 — WARNING: dead skip-test with a false comment

`test_worktree_gate_parity.py:295` `test_go_comparison_is_explicitly_skipped_until_binary_exists`
**always skips** — both branches call `self.skipTest`, so it executes zero assertions. Its comment
asserts *"there is never a 'worktree-gate-current' file"*, which is **false**: `scripts/build-gate.sh`
emits exactly that file (`apply-progress.md` §0.4) and it exists in `dist/`. The real differential
at `:257` is unaffected and passes, so V.4 stands — but this is a ghost test that will mislead the
next reader.

### F5 — WARNING: launcher resolution order only partially tested

`3.4` declares a four-step order (`$WORKTREE_GATE_BIN` → project-local → version-keyed cache →
legacy Bash). Tested: cache path, legacy fallback, no-implementation fail-open. Untested: the
`WORKTREE_GATE_BIN` override precedence and the project-local `bin/worktree-gate` step.

### F6 — WARNING: strict-TDD evidence is narrative, not structured

`strict_tdd: true` (`openspec/config.yaml:9`). `apply-progress.md` contains **no
`TDD Cycle Evidence` table**. RED (1.19) and GREEN (2.16) exist only as prose, and because
phases 0-2 landed as a **single commit** (`e290efa`), the RED state is **not recoverable from
history** — precisely the separation `tasks.md:33-34` demanded (*"PR 2 must land RED-on-Go by
design; that is the strict-TDD evidence"*). The tokenizer and URI/topology differentials described
in `apply-progress.md` §2 are credible RED→GREEN narratives, but they are unverifiable artifacts.

### F7 — WARNING: review workload forecast not honored, `Chain strategy` never recorded

Forecast (`tasks.md:5-44`): 5 chained PRs mandatory, 400-line budget risk **Critical**.
Actual: **3** chained PRs. Measured added lines excluding `openspec/` artifacts and the binary:

| PR | Commit | Forecast scope | Est. | Actual added | Over budget |
|----|--------|----------------|------|--------------|-------------|
| #191 | `e290efa` | PR 1 + PR 2 + PR 3 collapsed | ~2,250 | **+4,392** (52 files) | ~11× the 400 budget |
| #192 | `697a42c` | PR 4 | ~650 | **+1,431** (9 files) | ~3.6× |
| #193 | `d2e40e6` | PR 5 | ~250 | **+799** (8 files) | ~2× |

`Chain strategy` is still `pending` (`tasks.md:14,19`) and **no `size:exception` was recorded**. The
one constraint that *was* honored: no PR mixes implementation with distribution. Collapsing PRs 1-3
is also the direct cause of F6.

### 7a. Fix evidence (2026-08-10) — F1-F7 resolved

All findings below are resolved in the working tree (uncommitted, no push).

| Finding | Resolution | Evidence |
|---------|------------|----------|
| F1 (blocking) | stray binary removed from the index and ignored | `git rm --cached catalog/recipes/worktree-flow/gate/worktree-gate` (staged); `.gitignore` line `catalog/recipes/worktree-flow/gate/worktree-gate`; `git status` shows `D` staged, no binary in the tree. No `SHA256SUMS` entry ever existed for it. |
| F2 (blocking) | task 2.11 implemented | `gitfacts_test.go` `TestGitMemoDerivesEachFactOnce` (git shim on PATH, 4 facts x 2 repeats = exactly 4 git calls; failed lookup cached, 5th call only); `test_worktree_gate_metrics.py` `test_go_issues_strictly_fewer_git_invocations_than_bash` (4-candidate event, 2-submodule superrepo: Go < Bash, identical decision/stderr). |
| F3 (blocking) | tautologies removed | latency test asserts the corpus-pinned `expected_exit` per run (`assertEqual(returncode, expected)`); Bash + Go timing recorded; git counts asserted (Go < Bash). |
| F4 | ghost test deleted | `test_go_comparison_is_explicitly_skipped_until_binary_exists` removed from `test_worktree_gate_parity.py`; real differential at `:257` unchanged and green. |
| F5 | override precedence tested | `WorktreeGateLauncherResolutionTests` in `test_worktree_gate_dist_config.py` (4 tests): `WORKTREE_GATE_BIN` > project pin > cache, plus non-executable override ignored with warning. |
| F6 | TDD evidence recorded | `TDD Cycle Evidence` table in `apply-progress.md` (tokenizer / URI allowlist / topology / memoization RED->GREEN rows). RED states not recoverable from history — residual WARNING stands. |
| F7 | chain strategy + size exception recorded | `tasks.md` `Chain strategy` = "decided: collapse PRs 1-3"; `size:exception` documented with the measured 11x overrun. |

**Build-ordering fix (reviewer-found, not in the original report)**:
`scripts/build-gate.sh` copied `worktree-gate-current` **before** building the
target, so on a clean checkout the differential runners would test a stale or
missing binary. Fixed to copy after build; regression test
`test_build_script_refreshes_current_from_fresh_native_build` plants a stale
sentinel `worktree-gate-current` and asserts it is replaced by the fresh native
build. `SHA256SUMS` regenerated to match the current source (all four targets).

**V.6/V.8/V.9 now pass**:

- V.6: 35/35 spec scenarios mapped — the four previously unmapped performance
  scenarios now have tests ("One process per invocation",
  "Git facts are memoized across candidates", "No hashing on the hot path by
  default" in `test_worktree_gate_metrics.py`; "Explicit binary override wins"
  in `test_worktree_gate_dist_config.py`).
- V.8: latency recorded for both implementations and git call counts asserted
  (Go < Bash) in `test_worktree_gate_metrics.py`. Measured 2026-08-10 on this
  machine (corpus cases 01-04, 3 runs each after warm-up): **Go median 55.4 ms,
  Bash median 158.9 ms (2.9x)**; git call counts for the four-candidate
  two-submodule event: **Bash 72, Go 11** — same decision (exit 0).
- V.9: no production file outside the declared areas — the only stray was the
  removed binary.

### 7b. Round-2 findings (2026-08-10) — F8 resolved, F9 decision recorded

Review of the release plumbing after round 1 surfaced two additional findings.
Both are recorded with the exact state at time of writing.

| Finding | Verdict | Resolution / decision | Evidence |
|---------|---------|----------------------|----------|
| F8 (blocking) — CI checksum gate compared raw bytes | **RESOLVED** | canonical digest comparison | `scripts/verify-gate-sums.sh`; workflow checksum job rewritten; regression test green (below) |
| F9 — stale binary history hygiene | **DECISION (no rewrite)** | tree clean (F1); blob stays in PR history until merge; no force-push | §7b F9 (below) |

**F8 — the release would have failed on every tag push.**

The checksum job of `.github/workflows/release-worktree-gate.yml` emitted
`sha256sum worktree-gate-* > SHA256SUMS` (bare output, lexicographic order)
and compared it to the committed trust root
`catalog/recipes/worktree-flow/bin/SHA256SUMS` with a byte-level
`diff -u`. The committed file carries a 12-line documentation header and a
hand-maintained order (`darwin-arm64` before `darwin-amd64`), so even with
**four correct digests** the diff is non-empty and the release fails. No
existing test caught it: the local tests and `load_expected_digests()`
(`gate_binary.py`) all parse entries per-name, never file-level.

Reproduction on this machine (2026-08-10, fresh matrix builds):

```
$ sha256sum worktree-gate-* > SHA256SUMS   # CI emission, glob order
$ diff -u catalog/recipes/worktree-flow/bin/SHA256SUMS dist/SHA256SUMS
exit 1   ← release would FAIL (header + order differences)
$ bash scripts/verify-gate-sums.sh dist/SHA256SUMS catalog/recipes/worktree-flow/bin/SHA256SUMS
verify-gate-sums.sh: ok — 4 digest entries match the committed trust root
exit 0   ← only real digest mismatches fail
```

Fix:

1. `scripts/verify-gate-sums.sh` (new) — canonical comparison: keeps only
   `<64-hex>  worktree-gate-<goos>-<goarch>` lines, drops comments/blank
   lines, sorts by asset name; exit 1 on any real digest mismatch with the
   "regenerate … and commit it" message. Pure bash 3.2 (`grep`/`awk`/`sort`
   pipelines), matching the repo's portability constraints.
2. `.github/workflows/release-worktree-gate.yml` — the checksum job now runs
   `bash scripts/verify-gate-sums.sh dist/SHA256SUMS
   catalog/recipes/worktree-flow/bin/SHA256SUMS`; header contract updated.
3. `tests/test_worktree_gate_release_phase4.py` —
   `test_canonical_sums_comparison_ignores_header_and_order` (regression):
   builds the matrix, emits a bare sorted sums file, asserts the canonical
   comparison passes despite the header/order, and that a corrupted digest
   still fails with the regenerate message.

Verified: `bash -n scripts/verify-gate-sums.sh` clean; full focused run
`pytest tests/test_worktree_gate_release_phase4.py tests/test_worktree_gate_dist_config.py
tests/test_worktree_gate_parity.py tests/test_worktree_gate_metrics.py` →
**32 passed, 79 subtests**; live CI simulation (build → emit → canonical
compare) → exit 0; corrupted digest → exit 1 with the actionable message.

**F9 — the 3.5 MB binary stays in the PR-chain history; documented, not
rewritten.**

Facts verified 2026-08-10:

- The stray binary `catalog/recipes/worktree-flow/gate/worktree-gate` (3.5 MB,
  blob `d9656d1`, mode 100755) is committed **only** in `e290efa`
  ("feat(worktree-gate): complete Go gate apply phases 0-2", PR #191).
- `e290efa` is an ancestor of **all three open PR branches** —
  `change/worktree-gate-go` (#191), `change/worktree-gate-go-phase-3` (#192),
  `change/worktree-gate-go-phase-4` (#193) — locally and on `origin`.
- `e290efa` is **not** an ancestor of `development` (`git merge-base
  --is-ancestor e290efa development` → exit 1) nor of `main`.
- The blob exists exactly once in the object database; no later commit re-adds
  it. The tree fix (F1: `git rm --cached` staged + `.gitignore` entry) removes
  it from the delivered content of every PR in the chain.

Decision (technical): **do not rewrite history and do not force-push.** The
three branches are pushed and the PR chain is open; rewriting all three heads
and force-pushing is a destructive, authorization-gated operation, and the
assignment explicitly requires separate explicit authorization for any history
rewrite. The blob therefore remains reachable from the open PRs until they
merge — after merge it is reachable only from the merged commit lineage
(`e290efa`), where a future `git filter-repo`/BFG cleanup can drop it while the
PRs are still open or shortly after merge, before downstream branches
accumulate.

Impact: **none functional.** Nothing reads or ships the binary: F1 removed it
from the index, every suite keys off `dist/` builds, `recipe.toml`
materialization never references it, and `load_expected_digests()` only
consumes the text `SHA256SUMS`. The only cost is ~3.5 MB of unreachable
history per clone of the PR branches until cleanup.

Residual risk (explicit): if the PRs merge without a cleanup pass, the blob
becomes permanently reachable from `development`/`main` history. This is the
single open hygiene item, tracked as V.11 in `tasks.md`; a follow-up can
eliminate it before or immediately after merge.

### 7c. Round-3 findings (2026-08-11) — F10 scrub parity, F11/F12 release CI

Final verification of the pre-archive state surfaced three release blockers.
All resolved in the working tree (uncommitted, no push); HEAD unchanged at
`d2e40e6`.

| Finding | Verdict | Resolution | Evidence |
|---------|---------|------------|----------|
| F10 (blocking) — Go shell-mode extraction omits the Bash scrub semantics | **RESOLVED** | `scrub()` in `catalog/recipes/worktree-flow/gate/extract.go` mirrors `worktree-gate-legacy.sh:90-100` exactly (strip, empty/`.`/`-`, `&`-prefix, `/dev/null`/`/dev/stdout`/`/dev/stderr`/`/dev/fd/*`); applied in `dedupeStrings` — the same wrapper the reference applies in `dedupe(pass1 + pass2)`. Unit tests (`TestScrub`, `TestExtractPass1Scrub`, `TestExtractPass2Scrub`) + 12 hook scenarios added to the parameterized suite. | 54/54 differential sweep MATCH (31 scrub must-allow + 23 regression must-block); 180 hook scenarios green (90 bash + 90 go); `go test ./...` ok. |
| F11 (blocking) — release CI pins Go 1.22.x while SHA256SUMS generated with go1.24.13 | **RESOLVED** | workflow `go-version: "1.24.13"` (exact, canonical — same toolchain that regenerated the trust root; Go compiles different stdlib bytes per release). `build-gate.sh` warns on any non-canonical toolchain; SHA256SUMS header documents the constraint. | CI simulation: emit + `verify-gate-sums.sh` → `ok — 4 digest entries match the committed trust root`; release-phase4 tests green incl. new workflow-pin regressions. |
| F12 (blocking) — setup-go cache references nonexistent go.sum | **RESOLVED** | `cache-dependency-path` removed; comment documents the module has zero third-party deps and intentionally no go.sum (D8), so no cache key exists. | new regression test asserts no `cache-dependency-path:` key in the workflow. |

**F10 reproduction (pre-fix)**: `echo x > .`, `> -`, `> &*`, `> &1` and
`python3 -c "open('.','w')"` on protected main: Bash exit 0 (scrubbed), Go
exit 2 (blocked) — a differential false-block. Post-fix all 22 probe cases
MATCH.

**Digest regeneration (F11/F12)**: after the F10 source change, all four
digests changed; regenerated with the canonical toolchain
(`go1.24.13 darwin/arm64`) via `scripts/build-gate.sh`; byte-identical on a
second build (reproducibility confirmed per target); committed
`catalog/recipes/worktree-flow/bin/SHA256SUMS` updated.

**Full validation (2026-08-11)**:

```
go -C catalog/recipes/worktree-flow/gate test -count=1 ./...     ok (2.7s)
go vet ./...                                                     clean
gofmt -l catalog/recipes/worktree-flow/gate                      (empty)
python3 -m pytest tests/test_worktree_gate_parity.py \ 
  tests/test_worktree_gate_tokenizer.py                          10 passed, 277 subtests
python3 -m pytest tests/test_worktree_gate_metrics.py            5 passed
python3 -m pytest tests/test_worktree_gate_hook.py               180 passed (90+90)
python3 -m pytest tests/test_worktree_gate_release_phase4.py     9 passed, 12 subtests
bash scripts/verify-gate-sums.sh (CI sim)                        ok — 4 digest entries match
./tests/run.sh                                                   Ran 1580 tests — OK
./tests/validate.sh                                              VALIDATE_EXIT=0
```

Remaining risk: none blocking. The workflow pin (1.24.13) must move together
with any future digest regeneration; the new
`test_local_toolchain_matches_pinned_release_toolchain` enforces that pairing.

## 8. Recommendation

**Archive is cleared to proceed** once the fix set is committed and pushed as
a follow-up to PR #193 (`d2e40e6`). Blocking items F1, F2, F3 and F8 are
resolved; F4-F7 are recorded; F9 is a recorded technical decision with no
force-push. Two explicit open items for the parent, none blocking:

1. **F9 history cleanup** (V.11) — run `git filter-repo`/BFG on `e290efa`
   before the PRs merge (or immediately after, while the chain is still the
   only carrier of the blob), with its own authorization. Until then the
   ~3.5 MB blob is reachable only from the open PR branches.
2. Residual WARNINGs as recorded in round 1: the strict-TDD RED states are not
   recoverable from history (single commit `e290efa`), and the size exception
   stands.


_Original recommendation (pre-fix), kept for the record:_

**Do not archive. Do not release 0.22.0.** Two blocking fixes, both small:

1. **F1** — `git rm --cached catalog/recipes/worktree-flow/gate/worktree-gate` and add it to
   `.gitignore`. Non-negotiable before any release: it contradicts the change's own trust-root README.
2. **F2 + F3** — implement task 2.11 as written (`git` shim on `PATH`, four-candidate event, assert
   Go count `<` Bash count) and replace the tautological metrics assertions with the Bash-vs-Go
   comparison V.8 requires. This closes 2 of the 4 unmapped spec scenarios.

Then add the two remaining performance scenarios ("One process per invocation", "No hashing on the
hot path by default") and the `WORKTREE_GATE_BIN` override test (F5) to close V.6, and delete the
ghost test in F4.

Non-blocking but should be recorded before archive: F6 (add the `TDD Cycle Evidence` table) and F7
(record `Chain strategy` and the `size:exception` the delivered PR sizes actually took).

Everything else is genuinely done: the Go gate reaches verified byte-level parity with the frozen
Bash reference across the full corpus and 156 hook scenarios, distribution/acquisition/doctor are
well covered (29 tests), the harness surface is proven unchanged for all five harnesses, and the
full suite is green at 1,539 tests.

