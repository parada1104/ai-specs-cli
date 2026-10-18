# Target/topology proof in Go

## Tracker

- **card_id**: `6aaeeeb06eac46779a89ef47`
- **shortLink**: `zkQEFfnC`
- **url**: https://trello.com/c/zkQEFfnC/140-migrate-target-topology-proof-into-go
- **list**: In Progress

## Goal

Move the bounded submodule-to-superproject topology proof used by the Plan Build hook into the existing zero-dependency Go gate binary. Keep the shell hook as acquisition/argument plumbing, preserve compatibility bridges where the binary is not available, and do not add a second Go binary or parser dependency.

## Scope

- Add a read-only Go query contract for topology/central-root proof.
- Preserve fail-closed behavior for standalone, proven submodule, and ambiguous/unproven contexts.
- Route `plan-build-gate.sh` topology acquisition through the verified Go binary.
- Add focused RED/GREEN and parity coverage.
- Rebuild the Go trust root and record final validation evidence.

## Non-goals

- Wholesale migration of `target-resolve.py` or all Python topology consumers.
- Migrating init/doctor/hub paths that resolve topology before binary acquisition.
- New Go binary, third-party dependency, TUI, or acquisition-only migration.

## Tasks

- [x] T1 — Pin the Go query and shell delegation contracts with failing tests.
- [x] T2 — Implement the minimal Go topology proof query and focused unit coverage.
- [x] T3 — Replace Plan Build's duplicate topology proof with verified Go delegation and parity coverage.
- [x] T4 — Rebuild trust assets, run full validation, and record evidence.

## Decisions

- The current repository has one Go module/binary (`worktree-gate`); `ai-specs` remains a Bash dispatcher during the strangler migration.
- The first seam is the already rigorous topology proof consumed by Plan Build, not the broader Python target planner. Python `detect_submodules` and Go proof semantics differ; changing all consumers in one slice would be unsafe.
- Ambiguous or unproven topology must fail closed rather than infer a central root.

## Evidence

- Exploration: delegated read-only map identified three topology implementations and ranked the Plan Build proof as the smallest coherent seam.
- RED: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveCentralRoot' -count=1 .` failed 5/5 as expected because `--resolve-central-root` is not defined; `python3 -m unittest tests.test_plan_build_gate_hook` ran 43 and failed only the 3 new delegation tests.
- GREEN: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveCentralRoot|TestModuleRecords|TestClassifyStandalone|TestCentralFromCommon|TestLegacyCentral' -count=1 .` passed 11 selected tests; `go vet` passed and `gofmt -l` was clean.
- T3 GREEN: `python3 -m unittest tests.test_plan_build_gate_hook` -> 44 tests OK (evidence observed independently by the parent, not re-run in this update).
- T3 shell lint: `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` passed.
- T4 trust-root regeneration: built all four assets with the canonical `go1.24.13` toolchain into a temporary absolute output dir outside the repository — `OUT="$(mktemp -d /tmp/t4-gate-build.XXXXXX)"` -> `/tmp/t4-gate-build.6Uvk9u`, then `scripts/build-gate.sh "$OUT"` -> "build-gate.sh: done — 4 targets built" (darwin/arm64, darwin/amd64, linux/amd64, linux/arm64) with no non-canonical toolchain warning; `go version` = `go1.24.13`.
- T4 checksum generation: `(cd "$OUT" && shasum -a 256 worktree-gate-* | grep -v current) > /tmp/t4-gate-sums.txt` produced a bare 4-line file (no header, no `worktree-gate-current` entry).
- T4 verify (pre-update): `./scripts/verify-gate-sums.sh /tmp/t4-gate-sums.txt catalog/recipes/worktree-flow/bin/SHA256SUMS` exited 1 with all four digests differing — the committed trust root was stale before regeneration.
- T4 trust root: only the four digest lines of `catalog/recipes/worktree-flow/bin/SHA256SUMS` were replaced (`git diff --stat` = 4 insertions / 4 deletions); the documentation header is unchanged. New digests: darwin-amd64 `f6af08edaac36238f619d2f380bff6f5bc5982eabaa0f499eb69d2620ac139b6`, darwin-arm64 `382ad6740566749bf64d8d882184e4b43aa93350db15cc1322ab33a594413cc2`, linux-amd64 `6be19ba798c04a1d115b67ad00d5cfe5c38198f077fa4169f15e26394fa5f916`, linux-arm64 `aae01d913667ab02cf208f5169819e27fe09440d68ddf50784fd8edacb7f31e8`.
- T4 verify (post-update): `./scripts/verify-gate-sums.sh /tmp/t4-gate-sums.txt catalog/recipes/worktree-flow/bin/SHA256SUMS` exited 0 -> "verify-gate-sums.sh: ok — 4 digest entries match the committed trust root".
- T4 final validation (observed independently by the verifier in this dedicated worktree):
  - `./tests/validate.sh` -> exit 0.
  - Go gate packages -> pass.
  - Python suite -> `Ran 2191 tests in 606.228s — OK (skipped=142)`.
  - `go test ./...` -> pass; `go vet ./...` -> pass; `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` -> pass.
  - Canonical build and `scripts/verify-gate-sums.sh` -> pass.
  - Worktree clean before and after the validation run.
- Implementation commits: `02c9e01` (pin central-root query contract), `4e32be2` (add central-root topology query), `953adf7` (delegate topology proof to Go), `b9652ca` (refresh topology trust root).
- Native review: the parent attempted native START from the provider-issued route after a fresh inspect against target `sha256:8f9823d93ca0b49e4fb7616fc8bdda6f93f8b864ab77602886e31ad40d7aaf3d`; the provider stopped before authority creation with `lens_context_budget_exceeded`. `mutation_outcome=none`: no review authority or approval was created, and no review consent was requested. The candidate must be split into smaller reviewable PR/commit slices before retry. This document records no native review approval.

## Status

T1–T4 complete; the Go query now composes the existing proof helpers, preserves the legacy fail-closed fallback, and supports linked submodule worktrees; Plan Build's duplicate topology proof is replaced by verified Go delegation with parity coverage. T4 is closed with the trust root rebuilt with canonical `go1.24.13` and re-verified, and the full validation suite passing end to end in this worktree. Implementation commits: `02c9e01`, `4e32be2`, `953adf7`, `b9652ca`. Native review was not obtained: the attempted native START stopped before authority creation with `lens_context_budget_exceeded` (`mutation_outcome=none`, no consent requested), so the candidate must be split into smaller reviewable PR/commit slices before retry; review remains parent-owned.
