# GO-20 U2e T5 PR4a S3 — doctor live oracles

Card #191 (6adcb0a6f1eed7e544dcaf5e). Base: epic/go-single-binary 31fadc6. Branch: change/go-20-u2e-t5-pr4a-s3-doctor.
Goal: `internal/doctor/legacy_differential_test.go` stops exec'ing python3 and reading `lib/_internal/`, without silent coverage loss.

## Specs

- S1: `TestDifferentialCLIVersion` reads a frozen golden (`internal/doctor/testdata/cliversion_golden.json`) captured once from `lib/_internal/cli_version.py` `evaluate_cli_version`; no python3, no `lib/` access, no skip. The 28 case names are pinned by a hard-coded inventory (count, uniqueness, order) asserted before comparing; inputs stored in the golden must equal the in-test inputs; a missing golden fails.
- S2: `TestDifferentialCacheKey` is re-targeted, not frozen: Python `cache_key(Path(p))` hashes the absolute resolved TempDir path, so its output is host-dependent and cannot be frozen. `doctor.cacheKey` is a one-line delegate to `projectcache.CacheKey`, whose `/` value and `ResolvePath` vs Python `realpath` semantics are already frozen (#481). The re-targeted test pins, for the same 7 paths, `cacheKey(p) == sha256(ResolvePath(p))[:12] + "-" + SanitizeBasename(base(ResolvePath(p)))` and `cacheKey("/") == "8a5edab28263-project"`.
- S3: Negative controls: mutating one golden result fails only that case; deleting or duplicating a golden case fails the inventory.
- S4: `go vet ./internal/doctor/`, focused tests and full `tests/validate.sh` green at the exact head.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S3 | inline | |
| T2 | S2 | inline | |
| T3 | S4 | inline | |

## Log

- L1 (Robert, 2026-11-12): "si a ambos jajaj" — approving (1) start PR4a S3 with internal/doctor in a dedicated worktree with the usual cycle, and (2) updating card #191 description.
- L2: Card #191 description reconciled 2026-11-12 (old text kept under HISTORICAL).
- L3: Finding: `internal/doctor/cliversion.go:221` keeps its own `evaluateCLIVersion`, separate from `internal/cliversion.EvaluateCLIVersion` (frozen in #481); the cliversion golden does not cover the doctor copy, so S1 freezes it here. Duplicate implementation noted as follow-up, not changed in this slice.
- L4: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s3 (Robert's standing scratch convention).
