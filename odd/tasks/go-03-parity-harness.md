# go-03-parity-harness

Card: [Go 03] Differential parity harness (legacy vs Go) — Trello 6a84e77110b9ac751af676f4
Worktree: .worktrees/go-03-parity-harness, branch change/go-03-parity-harness.
Boundary: worker leaves changes UNCOMMITTED (orchestrator stages/commits).

## Tasks

- [x] T1 Explore repo: _blackbox patterns, parity contract, card-04 Go shim, help heredoc quirk observed live (multi-line hub substitution confirmed).
- [x] T2 Build tests/parity/parity.py: side runners, snapshot w/ modes, normalization registry (justified), diff engine, fixture corpus.
- [x] T3 Build tests/parity/run.py: suite entry (builds Go binary CGO_ENABLED=0, falls back to identical-by-shim legacy-vs-legacy, prints report, exits nonzero on unexplained delta).
- [x] T4 Self-tests incl. negative mutation tests (RED evidence): tests/test_parity_harness.py.
- [x] T5 Wire phase into tests/run.sh; validate.sh picks it up.
- [x] T6 Full suite green: ./tests/run.sh; record evidence; report worker_done.

## Evidence

- `python3 tests/parity/run.py` → **exit 0**, ~25-30s (incl. CGO_ENABLED=0 Go build): all 8 fixtures zero deltas, legacy-vs-Go. First run exposed one real delta (`surface-verbs`: `help` stderr — legacy carried `GO_*_BRIDGE_FALLBACK` warnings, Go none). Disposition: same known quirk as N2 on the stderr channel — the legacy heredoc's command-substituted PATH child shares the parent's stderr, so help stderr is environment-defined, not implementation behavior; fixed with targeted rule N6 (`help` stderr → constant `<HELP_CMD_SUBST_STDERR>`, scoped to argv[0]=='help', unconditional, fully justified in NORMALIZATIONS). Not a card-04 defect.
- Flake-check only, nondeterminism rules added with written justification — N3 harness tempfile names (`ai-specs-(recipe-mcp|vendor)-*` → <RANDOM>), N4 cache project key (`cache/projects/<hex>-project` → <PROJECT_KEY>), N5 ISO-8601 timestamps (<ISO_TS>); snapshot_tree normalizes UTF-8 file contents before hashing (same rules, both legs) and excludes `.git/index` + `.git/logs/` (git stat-cache/reflog run artifacts); `_git` pins GIT_AUTHOR/COMMITTER_DATE so commits/refs/objects stay deterministic.
- `python3 -m unittest tests.test_parity_harness -v` → **exit 0**: `Ran 12 tests — OK` (NormalizationTests ×5 incl. N6, CorpusTests ×3, NegativeMutationTests ×4 = RED evidence: baseline zero deltas + injected stdout/tree/exit-code mutations each flagged).
- `./tests/run.sh` → **exit 0** (full suite): `Ran 2370 tests in 1723.204s — OK (skipped=164)`, parity phase included and green (independent run.py exit 0 above). validate.sh picks the phase up via run.sh.
- Gotcha fixed: `import parity` under `python3 -m unittest tests.…` resolved to the `tests/parity/` package, shadowing `parity.py`; `tests/parity/__init__.py` now re-exports `from .parity import *` so both sys.path routes expose the same API.
- Final git state: everything uncommitted (tests/parity/, tests/test_parity_harness.py, tests/run.sh +2 lines, odd/tasks/go-03-parity-harness.md) — orchestrator owns staging/commit/PR.
