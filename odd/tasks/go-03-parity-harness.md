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

- Reliability fix (review finding: wired CI phase failed open): `run.py` default mode now REQUIRES the Go build — missing/failed build → explicit stderr message + **exit 2**; legacy-vs-legacy identical-by-shim is reachable ONLY via explicit `--self-test` (acceptance a). New `RunEntrypointTests` ×2 (14 total).
- RED: new contract tests against the committed fail-open `run.py` → `FAILED (errors=2)` (old `main()` had no argv contract); old behavior demonstrated live: go-less PATH → `OLD_DEFAULT_EXIT=0` with silent "identical-by-shim" note and zero deltas measured against nothing.
- GREEN: `python3 -m unittest tests.test_parity_harness` → `Ran 14 tests — OK`, exit 0. `python3 tests/parity/run.py` → exit 0 (8 fixtures, failing: 0, legacy-vs-Go). Sabotage (`env -i PATH=/usr/bin:/bin`) → **exit 2** + fail-loudly stderr. `python3 tests/parity/run.py --self-test` → exit 0, all 8 fixtures `legacy-vs-legacy (explicit --self-test)`, failing: 0.
- N6 kept functionally unchanged (narrowing not trivial: empty-vs-nonempty distinction would break legacy-vs-Go equality; noise is unbounded); reviewer tradeoff sentence added to its justification.
- Full suite with the fix: `./tests/run.sh` → **exit 0**, parity phase `fixtures: 8, failing: 0` (legacy-vs-Go), `Ran 2372 tests in 1975.514s — OK (skipped=164)`.
- Report wording updated: self-test mode now reads "explicit --self-test", not "Go build absent".

## Original evidence

- `python3 tests/parity/run.py` → **exit 0**, ~25-30s (incl. CGO_ENABLED=0 Go build): all 8 fixtures zero deltas, legacy-vs-Go. First run exposed one real delta (`surface-verbs`: `help` stderr — legacy carried `GO_*_BRIDGE_FALLBACK` warnings, Go none). Disposition: same known quirk as N2 on the stderr channel — the legacy heredoc's command-substituted PATH child shares the parent's stderr, so help stderr is environment-defined, not implementation behavior; fixed with targeted rule N6 (`help` stderr → constant `<HELP_CMD_SUBST_STDERR>`, scoped to argv[0]=='help', unconditional, fully justified in NORMALIZATIONS). Not a card-04 defect.
- Flake-check only, nondeterminism rules added with written justification — N3 harness tempfile names (`ai-specs-(recipe-mcp|vendor)-*` → <RANDOM>), N4 cache project key (`cache/projects/<hex>-project` → <PROJECT_KEY>), N5 ISO-8601 timestamps (<ISO_TS>); snapshot_tree normalizes UTF-8 file contents before hashing (same rules, both legs) and excludes `.git/index` + `.git/logs/` (git stat-cache/reflog run artifacts); `_git` pins GIT_AUTHOR/COMMITTER_DATE so commits/refs/objects stay deterministic.
- `python3 -m unittest tests.test_parity_harness -v` → **exit 0**: `Ran 12 tests — OK` (NormalizationTests ×5 incl. N6, CorpusTests ×3, NegativeMutationTests ×4 = RED evidence: baseline zero deltas + injected stdout/tree/exit-code mutations each flagged).
- `./tests/run.sh` → **exit 0** (full suite): `Ran 2370 tests in 1723.204s — OK (skipped=164)`, parity phase included and green (independent run.py exit 0 above). validate.sh picks the phase up via run.sh.
- Gotcha fixed: `import parity` under `python3 -m unittest tests.…` resolved to the `tests/parity/` package, shadowing `parity.py`; `tests/parity/__init__.py` now re-exports `from .parity import *` so both sys.path routes expose the same API.
- Final git state: everything uncommitted (tests/parity/, tests/test_parity_harness.py, tests/run.sh +2 lines, odd/tasks/go-03-parity-harness.md) — orchestrator owns staging/commit/PR.
- Reliability fix (fail-open → fail-loud): RED-first `RunEntrypointTests` added to tests/test_parity_harness.py — RED verbatim: both tests ERROR with `TypeError: main() takes 0 positional arguments but 1 was given` (old main() had no argv, no fail-loudly path, silently fell back), `FAILED (errors=2)`, exit 1. GREEN: run.py rewritten (exit 2 + stderr fail-loudly marker `Go build (cmd/ai-specs) is REQUIRED` when build_go_binary returns None and not --self-test); `Ran 14 tests — OK`. One test assertion aligned to the exact implemented marker (parent allowed "the exact fail-loudly marker you implement": message says "Go build", not "go build").
- Spec deviation (minimal, evidence-backed): the supplied run.py snippet did not force the legacy legs under `--self-test` — on a machine WITH go it silently ran legacy-vs-go, contradicting run.py's own docstring, the mocked test name, and step-4's expected "legacy-vs-legacy" note. Fixed with `go_cli = None if self_test else parity.build_go_binary(...)` so --self-test is the EXPLICIT legacy-vs-legacy mode.
- run.py exit codes observed: default `python3 tests/parity/run.py` → **0** (8 fixtures, legacy-vs-go, failing: 0); sabotage `env -i PATH=/usr/bin:/bin HOME=$HOME python3 tests/parity/run.py` → **2** with fail-loudly stderr (no go on stripped PATH; env -i worked on this machine); `--self-test` → **0** with per-fixture `[legacy-vs-legacy (identical-by-shim: Go build absent)]` + acceptance-a note.
- parity.py docs updated (no behavior change): build_go_binary docstring (policy is the CALLER's), module design-constraints bullet (explicit --self-test, never silent CI fallback), N6 justification + reviewer-noted tradeoff sentence. N6 behavior untouched (still unconditional constant for argv[0]=='help').
- Full unit file re-verified post-fix: `python3 -m unittest tests.test_parity_harness` → `Ran 14 tests — OK` (12 prior + 2 new). Not run: ./tests/run.sh (~29 min, deferred to orchestrator per task rules).
