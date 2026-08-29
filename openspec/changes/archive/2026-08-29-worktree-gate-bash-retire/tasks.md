# Tasks: retire the Bash worktree gate

Depth: full

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1,400–1,800 total: ≈450–600 additions, ≈950–1,250 deletions (catalog `worktree-gate-legacy.sh` alone ≈550 deleted lines) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 Phase 1 (~150–200) → PR 2 launcher fail-open slice (~150–250) → PR 3 materialization + doctor + catalog deletion + suite retarget (~800–1,000, atomic) → PR 4 docs + dogfood + full validation (~250–350) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High
```

### Work-unit boundaries

| Unit | Goal | Finish / verification | Rollback |
|------|------|----------------------|----------|
| PR 1 (Phase 1) | `gate_impl=bash` rejected; acquisition has no Bash branch | Focused dist-config + gate-binary suites green | Revert enum commit; `bash` usable again |
| PR 2 (Phase 2, launcher slice) | Launcher fails open without legacy fallback | `bash -n` + launcher/hook tests green | Revert launcher commit; fallback #4 restored |
| PR 3 (Phase 2 materialization + doctor, then Phase 3) | Legacy materialization gone; doctor retabled; catalog file deleted; suites Go-only | `./tests/run.sh` green | Revert; materializer + catalog file restored, suites dual-run again |
| PR 4 (Phase 4) | Docs, CHANGELOG, dogfood resync, full validation | `./tests/validate.sh` green | Revert docs/CHANGELOG |

Constraint: PR 3 is atomic by design. Deleting `materialize_legacy_gate`, the
catalog file, and retargeting the suites that reference them must land together;
splitting them would land a red tree.

Strict TDD (design §4.3, config `strict_tdd: true`): every behavior slice below
records RED evidence (failing test) before the production edit, then GREEN
evidence via focused `./tests/run.sh`. Slice order: `gate_impl=bash` rejected →
fail-open warning → legacy materialization gone → doctor → catalog deletion +
suite retarget → docs last.

---

## Phase 1 — Infrastructure: `gate_impl` enum reduction and acquisition (PR 1)

Evidence (2026-08-29): `./tests/run.sh` GREEN — Ran 1785 tests in 563.784s — OK. `recipe.toml` enum `["auto", "go"]`; `GATE_IMPL_VALUES = ("auto", "go")`; no `bash` acquire branch.

- [x] 1.1 RED: extend `tests/test_worktree_gate_dist_config.py` — configuration
      validation raises for `gate_impl = "bash"` with an actionable error naming
      `bash` as removed and `auto | go` as the only valid values (same
      `RuntimeError` shape as `lib/_internal/recipe-materialize.py:611` uses
      today for other invalid values); convert the predecessor
      rollback-rehearsal test (`gate_impl=bash` sync materializes legacy) into
      this rejection contract. Record RED evidence. <!-- sdd-owner: implementation -->
- [x] 1.2 RED: extend `tests/test_gate_binary_dist.py` — replace
      `test_gate_impl_bash_skips_acquisition`: `gate_binary.acquire` is invoked
      for `auto` and `go` (no early return for `bash`); offline `auto` and
      offline `go` both degrade without installing and no warning or result key
      claims a Bash fallback. Record RED evidence. <!-- sdd-owner: implementation -->
- [x] 1.3 GREEN: `lib/_internal/recipe-materialize.py` — `GATE_IMPL_VALUES`
      → `("auto", "go")` (`:480`); update the invalid-value error (`:611`) to
      name `bash` as removed and `auto | go` as valid. <!-- sdd-owner: implementation -->
- [x] 1.4 GREEN: `catalog/recipes/worktree-flow/recipe.toml` — `[config.gate_impl]`
      `enum = ["auto", "go"]` (`:73`); rewrite `help_text` (`:74`) without the
      Bash-fallback/rollback wording; bump the recipe version. <!-- sdd-owner: implementation -->
- [x] 1.5 GREEN: `lib/_internal/gate_binary.py` — delete the
      `gate_impl == "bash"` early return (`:408-409`); update the `attempted`
      result-key comment (`:384`), module docstring (`:22-24`), `acquire`
      docstring (`:392-394`), and `_degradation_hint` (`:621-624`) so no
      message promises auto→Bash degradation: `auto` and `go` both degrade to
      fail-open with a doctor ERROR. <!-- sdd-owner: implementation -->
- [x] 1.6 GREEN evidence: run the focused suites
      (`tests/test_worktree_gate_dist_config.py`, `tests/test_gate_binary_dist.py`)
      via `./tests/run.sh`; record RED→GREEN in apply-progress.
      <!-- sdd-owner: implementation -->

## Phase 2 — Implementation: launcher fail-open, materialization retirement, doctor retable (PR 2 + start of PR 3)

Evidence (2026-08-29): `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh` OK. Fallback #4 gone; leftover legacy.sh not materialized; doctor bash=ERROR leftover=INFO.

### Launcher fail-open slice (PR 2)

- [x] 2.1 RED: add launcher-level tests (launcher suite within
      `tests/test_worktree_gate_hook.py`) — with `WORKTREE_GATE_BIN` unset, no
      project-local pin, and no verified cache, the launcher exits `0` (fail
      open), prints **exactly one** stderr warning naming the unresolved gate
      binary and the recovery action (`ai-specs sync` /
      `ai-specs sync --refresh-gates` / `ai-specs doctor`), and never execs a
      same-directory `worktree-gate-legacy.sh` even when that file is planted
      (sentinel/marker assertion). Record RED evidence.
      <!-- sdd-owner: implementation -->
- [x] 2.2 RED: launcher precedence tests — `WORKTREE_GATE_BIN`, project-local
      pin, and verified version-keyed cache each resolve with no fail-open
      warning (existing order preserved above the fail-open floor); the
      launcher never branches on `stamped_gate_impl` to select an
      implementation. Record RED evidence. <!-- sdd-owner: implementation -->
- [x] 2.3 GREEN: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` — delete
      fallback #4 (`:175-183`, `local_legacy=` at `:180`); resolution order
      becomes env → project-local → version-keyed cache (`.verified` receipt
      check kept) → exactly one stderr warning → `exit 0`; update the header
      resolution list (`:7` references the legacy reference); keep sentinels
      (`stamped_gate_scope="`, `stamped_gate_impl` stamping), bash 3.2 only,
      one-warning-per-invocation invariant, no digest hashing on the hot path.
      Retarget any existing tests asserting fallback #4 in the same slice.
      <!-- sdd-owner: implementation -->
- [x] 2.4 Verify `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
      is clean and the launcher slice is green via focused `./tests/run.sh`.
      <!-- sdd-owner: implementation -->

### Legacy materialization slice (start of PR 3)

- [x] 2.5 RED: extend `tests/test_worktree_gate_dist_config.py` and
      `tests/test_worktree_gate_harness_phase4.py` — ordinary sync writes only
      the cleanup override and the launcher; `LEGACY_HOOK_REL` is never created
      (leftover absent); when a leftover
      `ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` exists it
      is NOT classified, refreshed, backed up, or rewritten (bytes unchanged);
      the worktree-flow freshness preflight requires only the cleanup template,
      launcher, and `SHA256SUMS` trust root — no legacy-gate input — and never
      writes project assets or the lock. Record RED evidence.
      <!-- sdd-owner: implementation -->
- [x] 2.6 GREEN: `lib/_internal/recipe-materialize.py` — delete
      `materialize_legacy_gate` (`:684-704`, incl. the `✓ hook script` print at
      `:704`), `LEGACY_HOOK_REL` (`:485`), and the call site (`:1227`); remove
      the legacy gate from the preflight input list. Leftover consumer copies
      stay in place (design D2). <!-- sdd-owner: implementation -->
- [x] 2.7 Retarget the materialization-observing suites in the same slice so the
      tree stays green: `tests/test_worktree_gate_harness_phase4.py` (auto does
      NOT write legacy.sh; missing binary → one fail-open warning, no Bash
      exec) and `tests/test_worktree_root_propagation.py` (drop the expected
      materialized legacy.sh path). <!-- sdd-owner: implementation -->

### Doctor retable slice (design §1.3 severity table)

- [x] 2.8 RED: extend `tests/test_doctor_worktree_gate.py` — stamped/config
      `gate_impl = "bash"` → ERROR naming the retired value and the recovery
      ("set `auto` or `go`, then `ai-specs sync`"; doctor read-only); leftover
      materialized `worktree-gate-legacy.sh` → INFO with the manual `rm` hint
      (not classified as a governed stale asset); `gate_impl=auto` with no
      verified binary → ERROR (no "falling back to Bash" WARN); no
      "rollback lever" text anywhere in doctor output. Record RED evidence.
      <!-- sdd-owner: implementation -->
- [x] 2.9 GREEN: `lib/_internal/doctor.py` `_check_worktree_gate` — replace the
      sample-output block (`:776-777`), the rollback-lever INFO (`:829`), and
      the auto→Bash WARN (`:848`) per the design §1.3 severity table; doctor
      stays read-only and never mutates manifest or lock.
      <!-- sdd-owner: implementation -->

## Phase 3 — Testing: catalog deletion and Go-only suite retarget (end of PR 3)

Evidence (2026-08-29): catalog `worktree-gate-legacy.sh` deleted; parity/hook/tokenizer/metrics Go-only; corpus includes `mv a b 2>&1`. `./tests/run.sh` 1785 OK.

- [x] 3.1 Retarget `tests/test_worktree_gate_parity.py`: delete the
      `LEGACY` / `materialize_legacy` runner half; drive the Go binary only
      against corpus `expect`; skip loudly only when the Go binary is absent
      (`dist/worktree-gate-*` / cache) — never because legacy.sh is gone; no
      case executes or compares against the retired script.
      <!-- sdd-owner: implementation -->
- [x] 3.2 Retarget `tests/test_worktree_gate_hook.py`: default SUT becomes the
      Go binary (or the stamped launcher exec'ing it); remove the `LEGACY_GATE`
      parameterization; historical line citations may remain in comments only
      as prose, never as an executable oracle. <!-- sdd-owner: implementation -->
- [x] 3.3 Retarget `tests/test_worktree_gate_tokenizer.py`: keep the
      python3 `shlex.split(cmd, posix=True)` oracle (the Go tokenizer contract,
      predecessor D9) plus the Go `--tokenize`/`--explain` diagnostic; drop the
      legacy-script framing and any execution of
      `worktree-gate-legacy.sh:129-133`. <!-- sdd-owner: implementation -->
- [x] 3.4 Add the `mv a b 2>&1` case to
      `tests/fixtures/worktree-gate-tokenizer-corpus.json` with the python3
      shlex answer as expected tokens (spec: "Tokenizer behavior is pinned by
      the Go-only corpus"); assert the corpus passes against the Go gate; the
      retired Bash tokenizer behavior MUST NOT be re-introduced as an expected
      result. <!-- sdd-owner: implementation -->
- [x] 3.5 Retarget `tests/test_worktree_gate_metrics.py`: drop the
      Go-strictly-less-than-Bash git-call comparison; keep Go-only assertions
      (memoization shim-count ceiling / `module_records` once, one
      implementation process, no hashing on the hot path); do not keep a
      vendored Bash copy for comparison (design D11).
      <!-- sdd-owner: implementation -->
- [x] 3.6 Regression confirmation for the MODIFIED requirements — the existing
      freshness/launcher scenarios stay green and unchanged: stale cleanup
      override replaced with verified bytes; unknown cleanup override replaced
      with canonical ownership; customized launcher force-replaced by ordinary
      sync and `--refresh-gates`; current assets idempotent; failed
      verification fails closed; failed replacement rolls back governed state;
      preflight precedes project writes; stale cache binary not accepted;
      committed `SHA256SUMS` digest authoritative; doctor freshness evidence
      actionable and read-only; version/lock drift distinguishable. Cover any
      scenario whose test referenced the legacy gate as a freshness target.
      <!-- sdd-owner: implementation -->
- [x] 3.7 Retarget `.github/workflows/release-worktree-gate.yml` parity job
      (`:131-150`): build a host-native Go binary first (ubuntu-latest →
      `linux/amd64` via the same `scripts/build-gate.sh` flags), then run
      `python3 -m unittest tests/test_worktree_gate_parity.py -q`; rename the
      job and step (no "frozen reference" / "Bash reference"); keep the job
      independent of the matrix `build` job so a checksum failure cannot skip
      behavioral proof; release checksum logic untouched.
      <!-- sdd-owner: implementation -->
- [x] 3.8 Delete `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh`
      from the catalog — LAST in this phase, after nothing references it;
      remove the byte-identity/manifest check referencing the catalog blob and
      any remaining `LEGACY_GATE` test helpers; grep-assert no production or
      test reference to the legacy file or `gate_impl="bash"` remains.
      <!-- sdd-owner: implementation -->
- [x] 3.9 Verify Go-only coverage: `./tests/run.sh` green; audit skip reasons —
      no suite skips Go coverage because a Bash reference is missing; Go unit
      tests under `catalog/recipes/worktree-flow/gate/` no longer skip because
      a Bash reference is absent (skip only when the binary itself is absent).
      <!-- sdd-owner: implementation -->

## Phase 4 — Docs, dogfood and full verification (PR 4)

Evidence (2026-08-29): docs + CHANGELOG updated. `./tests/run.sh` 1785 OK. `./tests/validate.sh` and dogfood 4.5 recorded in apply-progress.md.

- [x] 4.1 Update `docs/runtime-hooks.md`: resolution chain shows three steps
      plus the one-warning fail-open floor (no Bash step); `gate_impl` documents
      only `auto | go`; remove "kept for one minor release as the rollback
      path" and every `bash` row; add the replacement rollback story
      (per-invocation `WORKTREE_GATE_MODE=off` / `WORKTREE_GATE_BIN=<path>`;
      per-install `rm -rf $AI_SPECS_HOME/cache/bin/worktree-gate` then
      `ai-specs sync`; full revert = install the previous CLI and
      `ai-specs sync`); `auto` without binary is a doctor ERROR.
      <!-- sdd-owner: implementation -->
- [x] 4.2 Update `docs/recipes-catalog.md`: only `auto | go` documented; no
      legacy Bash gate entry or rollback path. <!-- sdd-owner: implementation -->
- [x] 4.3 Update `catalog/recipes/worktree-flow/README.md`: remove "Rollback
      levers: set `gate_impl = "bash"`"; document `auto | go`, the fail-open
      warning, doctor severities (retired value ERROR, leftover file INFO with
      the `rm` hint, missing binary ERROR), and recovery actions.
      <!-- sdd-owner: implementation -->
- [x] 4.4 Update `CHANGELOG.md`: user-facing breaking change — `gate_impl=bash`
      rejected at sync; the catalog no longer ships
      `hooks/worktree-gate-legacy.sh`; already-materialized copies become inert
      (doctor INFO); the launcher fails open with exactly one stderr warning
      when no binary resolves. <!-- sdd-owner: implementation -->
- [x] 4.5 Dogfood resync verification: NOTE — the apply agent MUST first read
      `/Users/robert/proyectos/nnodes/ai-specs-cli/.pi/skills/dogfood-verification-isolation/SKILL.md`
      before running any ai-specs sync against this repo's own `ai-specs/`.
      Inside this worktree (`.worktrees/worktree-gate-bash-retire`), run the
      sync per that skill and verify: the materialized
      `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` has no fallback
      #4 and `stamped_gate_impl ∈ {auto, go}`; no
      `worktree-gate-legacy.sh` is written into the project; `ai-specs doctor`
      reports no retired-value ERROR. <!-- sdd-owner: implementation -->
- [x] 4.6 Full suite before final verification: `./tests/validate.sh` green
      (`py_compile`, `bash -n`, unittest discovery), plus `go vet` / `go test`
      under the existing guarded wiring.
      <!-- sdd-owner: implementation -->
- [x] 4.7 Spec-scenario sweep and exit criteria: map every delta scenario in
      `specs/worktree-flow/spec.md` to a passing test (explicit list in
      apply-progress); confirm exit criteria — `gate_impl = bash` no longer
      exists anywhere (config, docs, launcher); the launcher fails open with
      one stderr warning when no binary resolves; all parity, tokenizer, hook,
      dist and doctor suites pass with the Go binary as the only
      implementation. <!-- sdd-owner: implementation -->
- [x] 4.8 Scope guard: confirm out-of-scope surfaces untouched —
      `lib/_internal/hooks-render.py` and generated `.opencode` / `.pi` /
      `.omp` / Cursor wrapper templates, `catalog/recipes/trello-mcp-workflow/**`,
      Go gate policy (`decide.go`, extraction, topology), no automatic deletion
      of consumer leftover files, no Windows work (design §8).
      <!-- sdd-owner: implementation -->

## Parent-owned post-apply actions

- [ ] 5.1 Start or reuse bounded review. <!-- sdd-owner: parent -->

---

## Fixture governance and edit authority

All test fixtures are ephemeral resources owned by this change and MUST be
created beneath the authorized worktree (or test-managed temporary children)
using `TemporaryDirectory()` during execution. Before apply, verify the edit
root with `git rev-parse --show-toplevel`; it MUST resolve to
`.worktrees/worktree-gate-bash-retire`. The only persistent edits permitted are
files under this worktree.

## Tracker

- **card_id**: 112
- **url**: https://trello.com/c/6a92941668e7be9d46c0193b

## Artifact path

`openspec/changes/worktree-gate-bash-retire/tasks.md`
