# ODD Feature: go-lock-copy

**Card**: GO-08 (Trello `6ab6a1718abb6ec9790f65a6`, https://trello.com/c/qMWIt0VY)
**Worktree**: `.worktrees/go-lock-copy` — branch `feat/go-lock-copy` from development `6d4890e`
**Pipeline role**: MAIN implementation lane (slice 5). Delivery (native review, validate.sh, PR, merge) is SERIALIZED through the main session per `decision/parallel-strangler-protocol`.

## Goal

Strangler slice 5: Go owns the lock-file writer and the materialize copy actuator, Python keeps fail-open bridges. Slices 6/7 consume the lock format from here — format stability is a parity trap.

## Tasks

- [ ] Scout: exact contract of lock.py writer + copy actuator + customization preservation. **Status: pending**
- [ ] WU1: Go lock writer core — port lib/_internal/lock.py:86-136 (hand-rolled TOML emitter + atomic replace) to Go with parity tests. Lock acquisition/serialization semantics preserved. **Status: done (committed as 2c37373)**
- [ ] WU2: Go copy actuator — materialize 768-828/949-957 + dispatch 2905-2935; customization preservation (project-cache.py 156-377: CRLF-normalized byte-match, legacy-lock hash) byte-identical; Python fail-open bridge. **Status: done (copy actuator committed as WU3; dispatch loop untouched — per-item bridging preserves its order exactly; slice 6 owns template, slice 7 consumes lock)**
- [ ] Validation + native review per WU (serialized lane). **Status: pending**

## Out of scope

- template actuator (GO-09, parallel prep lane), hook/gate actuator, execute_hooks decision, advisory PR.

## Evidence log

- Worktree + branch created from development 6d4890e. Card GO-08 in In Progress. Worktrees carry only AGENTS.md (no .pi skills/hooks); user pre-authorized dedicated worktrees.
- WU1 envelope clarification: original pinned flat agents envelope (`"agents": {"<harness>": "hash"}`) contradicted 1:1 parity with lock.py:110-116; parent resolved option A — nested `"agents": {"<harness>": {"<filename>": "hash"}}`.
- WU1 RED: `go test ./...` build-failed with 8 undefined symbols (`runWriteLock`, `lockHeader`, `lockTOMLString`, `renderLock`, `lockWriteRequest`, `lockManagedEntry`, `writeLockFile` via CLI tests) before lockwrite.go existed.
- WU1 GREEN: `go test ./...` → `ok ai-specs.dev/worktree-gate 43.853s` + `ok ai-specs.dev/worktree-gate/ledger (cached)`; `gofmt -l .` empty; `go vet .` clean. Byte pins ground-truthed against live Python write_lock output (join inserts a blank line after LOCK_HEADER's own trailing newline; final section's blank line is rstripped).
- WU1 differential parity: /tmp/lockwrite-parity-driver.py ran 9 fixtures (empty lock, meta only, meta-header-only edge, managed sorted/skip-empty, agents nested sorted, full lock, quote/backslash escaping, raw control chars, unknown-meta-keys-ignored) through Python write_lock vs `dist/worktree-gate-current --write-lock` → 9/9 byte-identical.
- WU1 build: `./scripts/build-gate.sh` (canonical go1.24.13, no toolchain warning) → 4 targets + `dist/worktree-gate-current`; `./scripts/verify-gate-sums.sh <generated> catalog/recipes/worktree-flow/bin/SHA256SUMS` → 4/4 digest entries match; SHA256SUMS regenerated in tree with the go-lock-copy provenance note.
- WU1 files: catalog/recipes/worktree-flow/gate/lockwrite.go (new), gate/lockwrite_test.go (new), gate/main.go (`--write-lock` flag + dispatch, mirroring `--write-recipe-config`), bin/SHA256SUMS (regenerated). No Python files touched.
- WU2 lock bridge RED: new tests/test_lock_bridge.py (12 tests) run before implementation — all errored (`AttributeError: module 'lock_bridge' has no attribute 'GO_LOCK_WRITE_BRIDGE_FALLBACK'`, 14 errors incl. subtests) against the unbridged write_lock.
- WU2 lock bridge GREEN: python3 -m unittest tests.test_lock tests.test_lock_bridge → 12 tests OK (no WORKTREE_GATE_BIN); with WORKTREE_GATE_BIN=dist/worktree-gate-current → 12 tests OK. Real-binary success test pins byte-identity vs _write_lock_python, legacy [skills]/[recipes] dropping through load_lock→write_lock, missing-parent creation, and no temp files left in the lock directory.
- WU2 fail-open divergence pinned: exit-2 {"error": ...} envelopes fall back with exactly one GO_LOCK_WRITE_BRIDGE_FALLBACK warning naming the opaque Go refusal string (documented in write_lock docstring — full-state idempotent atomic replace, no destructive ambiguity, unlike orphans deletion).
- WU2 caller-surface regression: tests.test_runtime_brief_ownership + tests.test_override_ownership → 51 tests OK with WORKTREE_GATE_BIN set; without it, 6 pre-existing environmental failures fire (GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK stderr assertions in test_runtime_brief_ownership, a bridge this WU did not touch — the lock bridge warning never enters those asserted streams).
- WU2 files: lib/_internal/lock.py (write_lock dispatcher + go_write_lock + lazy _load_gate_binary + _lock_write_bridge_home parents[2] + _write_lock_python rename, all call sites untouched), tests/test_lock_bridge.py (new). No commit (parent owns).
- WU3 copy-actuator RED (Go): gate/copyapply_test.go (10 tests) run before implementation — build failed with undefined `runApplyCopy`, `copyResult`, `copyItem`, `applyCopyDecision`, `copyFileStat`.
- WU3 copy-actuator GREEN (Go): `go test ./...` → ok (main + ledger); `gofmt -l .` empty; `go vet` clean. Copy decision (source presence, command overwrite condition) separated from execution in `applyCopyDecision` so the dest-is-directory decision stays observable; execution failure exits 2 {"error"} and the Python bridge fails open (fallback rmtree+copytree rewrites bundled-skill dest wholesale → partial Go copy safe to redo; copy2 idempotent).
- WU3 copy-actuator RED (Python): tests/test_copy_apply_bridge.py (13 tests) run before the bridge existed — 20 errors, all AttributeError on missing `GO_COPY_APPLY_BRIDGE_FALLBACK`/`GO_COPY_APPLY_BRIDGE_TIMEOUT_SECONDS`.
- WU3 copy-actuator GREEN (Python): 13/13 OK with the rebuilt dist binary (0 skips) and identical result with WORKTREE_GATE_BIN pinned; tree identity (structure/content/modes/mtimes), print/warn parity, exact RuntimeError parity, envelope contract (argv `--apply-copy`, one item per call with `commands_dir`), 8-case fallback matrix (one warning each), dep-skill boundary pin (vendor-skills.py stays Python).
- WU3 pre-existing failure (NOT this WU): tests.test_materialize_bridge OrphanApplyBridgeTests.test_applied_outcome_prints_todays_messages_and_prunes_lock fails at base 9418457 — the test's stub answers `--write-lock` with the orphans plan payload, so the WU1b lock bridge warns `GO_LOCK_WRITE_BRIDGE_FALLBACK ... did not match the write-lock envelope` and fails open (lock still written correctly). Proven untouched: lock.py, tests/test_materialize_bridge.py and clean_orphans are byte-identical to base (git diff empty). Parent decision needed: fix the stub to answer `--write-lock` or accept the environmental failure.
- WU3 boundary: materialize_dep_skill stays Python by design — it is true acquisition (vendor-skills.py sync_dep_target), not a blind copy; pinned by test_dep_skill_boundary_stays_python. materialize_template (864-947) untouched (slice 6).
- WU3 build: ./scripts/build-gate.sh (canonical go1.24.13) → 4 targets + dist/worktree-gate-current; ./scripts/verify-gate-sums.sh <generated|grep -v current> SHA256SUMS → 4/4 match; SHA256SUMS regenerated with the go-lock-copy WU2 note.
- WU3 files: gate/copyapply.go (new), gate/copyapply_test.go (new), gate/main.go (`--apply-copy` flag + dispatch), lib/_internal/recipe-materialize.py (copy bridge + 3 fallback bodies + 3 rewired copiers only), tests/test_copy_apply_bridge.py (new), bin/SHA256SUMS (regenerated). No commit (parent owns).
