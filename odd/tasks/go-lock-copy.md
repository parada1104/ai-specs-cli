# ODD Feature: go-lock-copy

**Card**: GO-08 (Trello `6ab6a1718abb6ec9790f65a6`, https://trello.com/c/qMWIt0VY)
**Worktree**: `.worktrees/go-lock-copy` — branch `feat/go-lock-copy` from development `6d4890e`
**Pipeline role**: MAIN implementation lane (slice 5). Delivery (native review, validate.sh, PR, merge) is SERIALIZED through the main session per `decision/parallel-strangler-protocol`.

## Goal

Strangler slice 5: Go owns the lock-file writer and the materialize copy actuator, Python keeps fail-open bridges. Slices 6/7 consume the lock format from here — format stability is a parity trap.

## Tasks

- [ ] Scout: exact contract of lock.py writer + copy actuator + customization preservation. **Status: pending**
- [ ] WU1: Go lock writer core — port lib/_internal/lock.py:86-136 (hand-rolled TOML emitter + atomic replace) to Go with parity tests. Lock acquisition/serialization semantics preserved. **Status: done (implementation; commit by parent)**
- [ ] WU2: Go copy actuator — materialize 768-828/949-957 + dispatch 2905-2935; customization preservation (project-cache.py 156-377: CRLF-normalized byte-match, legacy-lock hash) byte-identical; Python fail-open bridge. **Status: pending**
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
