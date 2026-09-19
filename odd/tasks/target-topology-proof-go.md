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
- [ ] T3 — Replace Plan Build's duplicate topology proof with verified Go delegation and parity coverage.
- [ ] T4 — Rebuild trust assets, run full validation, and record evidence.

## Decisions

- The current repository has one Go module/binary (`worktree-gate`); `ai-specs` remains a Bash dispatcher during the strangler migration.
- The first seam is the already rigorous topology proof consumed by Plan Build, not the broader Python target planner. Python `detect_submodules` and Go proof semantics differ; changing all consumers in one slice would be unsafe.
- Ambiguous or unproven topology must fail closed rather than infer a central root.

## Evidence

- Exploration: delegated read-only map identified three topology implementations and ranked the Plan Build proof as the smallest coherent seam.
- RED: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveCentralRoot' -count=1 .` failed 5/5 as expected because `--resolve-central-root` is not defined; `python3 -m unittest tests.test_plan_build_gate_hook` ran 43 and failed only the 3 new delegation tests.
- GREEN: `go -C catalog/recipes/worktree-flow/gate test -run 'TestResolveCentralRoot|TestModuleRecords|TestClassifyStandalone|TestCentralFromCommon|TestLegacyCentral' -count=1 .` passed 11 selected tests; `go vet` passed and `gofmt -l` was clean.
- Final validation: pending.

## Status

T1–T2 complete; the Go query now composes the existing proof helpers, preserves the legacy fail-closed fallback, and supports linked submodule worktrees. T3 in progress.
