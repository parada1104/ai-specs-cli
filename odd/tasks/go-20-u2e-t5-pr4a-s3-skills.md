# GO-20 U2e T5 PR4a S3 — skills live oracles

Base: epic/go-single-binary 7eb5d0c. Branch: change/go-20-u2e-t5-pr4a-s3-skills.
Goal: `internal/skills/{legacy_differential,flatten,vendored}_test.go` stop exec'ing python3 and reading
`lib/_internal` (`skill-resolution.py`, `flatten-resolved-skills.py`, `skill_contract.py`), without silent
coverage loss. Same discipline as PR #496 (doctor) and PR #497 (rulesaudit).

## Specs

- S1: `TestDifferentialCollectSkills` reads a frozen golden (`internal/skills/testdata/skill_resolution_golden.json`) captured once from `lib/_internal/skill-resolution.py` `collect_skills` over the same four-tier tree; no python3, no `lib/` access, no skip. Missing golden fails.
- S2: `TestFlattenDifferential` reads a frozen golden (`internal/skills/testdata/flatten_golden.json`) captured once from `lib/_internal/flatten-resolved-skills.py` `main` over the same six fabricated cases and the same sandbox layout (dest inside the cache root, inside root, through a symlinked parent, a dangling link, a dest that is a file). Missing golden fails.
- S3: `TestVendoredSkillMarkdownDifferential` reads a frozen golden (`internal/skills/testdata/vendored_skill_golden.json`) captured once from `lib/_internal/skill_contract.py` `from_dep` + `render_skill_markdown` over the same 26 dep/upstream pairs. Missing golden fails.
- S4: Each frozen test asserts a hard-coded independent inventory of case names (count, uniqueness, order) against both the in-test corpus and the golden before comparing anything, so deleting, duplicating or reordering a golden case fails as an inventory error.
- S5: Frozen inputs equal the in-test corpus: the golden carries a per-case input manifest (sha256 of every fabricated file, tokenized relative path for the tree cases; sha256 of `dep.toml` and `SKILL.md` for the vendored cases) asserted before any comparison, so input drift fails as input drift, not as an opaque payload mismatch.
- S6: No absolute host path leaks into a golden: the resolved scan root/home/cache directories are tokenized as `<ROOT>` / `<HOME>` / `<CACHE>` in the frozen payload, the frozen snapshot and the frozen input manifest; `grep -c /Users/\|/private/\|/var/folders` over `internal/skills/testdata` is 0.
- S7: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a corpus input each fail rc=1.
- S8: `gofmt -l internal/skills` empty, `go vet ./internal/skills/` rc=0, three files free of live `python3` / `exec.Command` / `lib/_internal` / `lib/_vendor`, `go test ./internal/skills/` rc=0, `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestDifferentialCollectSkills` (`skill-resolution.py collect_skills`) | FREEZE | payload is `{source, path}` per skill id; every path is under the resolved root/home/cache, so tokenizing `<ROOT>`/`<HOME>`/`<CACHE>` keeps the full payload, including which tier won and the exact winning directory. |
| `TestFlattenDifferential` (`flatten-resolved-skills.py main`) | FREEZE | rc, stdout, and the dest projection (`kind rel mode mtime content`) are reproducible once the two host-dependent inputs are pinned: dest in stdout is tokenized, and the corpus file/dir mtimes are fixed (copytree/copy2 propagate them), so the snapshot mtimes are constants instead of wall clock. stderr stays out of the comparison for the two failure cases exactly as in the live test: Python writes a traceback, the port writes one `error:` line (documented deviation), and a traceback is not byte-reproducible. |
| `TestVendoredSkillMarkdownDifferential` (`skill_contract.py from_dep` + `render_skill_markdown`) | FREEZE | rendered markdown and the `SkillContractError` messages are pure functions of the dep table and the upstream text; no host data at all. |

No RETARGET and no RETIRE: no leg depends on an un-tokenizable host value (unlike doctor's `cache_key`,
which hashes the absolute TempDir), so freezing keeps full payload coverage. Cases: 1 (resolution),
6 (flatten: `four-tiers-duplicates-nested-modes-links`, `stale-dest-is-wiped`, `no-skills`,
`dest-through-symlinked-parent`, `dangling-link-stops-after-failing-skill`, `dest-is-a-file`),
26 (vendored).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6, S7 | inline | |
| T2 | S8 | inline | |

## Log

- L1 (Robert): "si, avancemos con las entregas" — authorizing continuing the PR4a S3 deliveries with the skills slice.
- L2: Baseline at 7eb5d0c: the three live python differentials are green (`logs/00-baseline-live-legs.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s3-skills (Robert's standing scratch convention).
- L4: Finding (flatten): the snapshot pins `mtime`, which the live test could compare only because both legs ran in the same run. Freezing it needs a deterministic input mtime, so the corpus pins a fixed epoch on every fabricated file and directory; `copytree`/`copy2` propagate it through `copystat`, so the frozen snapshot carries constants.
- L5: Capture harness was a temporary `internal/skills/zz_scratch_capture_test.go` (python3 exec); it cross-checked the live port against the legacy run before writing, so the goldens are legacy output, not port output. It was moved out of the worktree before the commit (preserved in the scratch dir as `zz_scratch_capture_test.go.retired`) so the tree carries no scratch file.
- L6: Captured twice into two fresh scratch dirs and `cmp` byte-identical (rc=0); oracle metadata `python: 3.14.7`. `json.Encoder.SetEscapeHTML(false)` keeps the tokens readable (`<ROOT>` not `\u003cROOT\u003e`). Golden sizes: resolution 2172 B, flatten 9215 B, vendored 11181 B.
- L7: Finding (flatten): the first capture leaked the interpreter and script paths into the frozen `stderr` of the two failure cases, which are not asserted anyway. Fixed by freezing `stderr` only where the two legs agree (`compare_stderr` true), and the recapture has zero host-path matches.
- L8: Finding (vendored): the live test decoded the oracle's JSON but never asserted stderr; the frozen golden pins all 26 cases as `out` or `error`, which is the full payload the live assertion used.
- L9: Negative controls (each rc=1; golden/corpus restored and `cmp`-verified after each): mutate a flatten result (`logs/07-nc1-mutate-result.log`), a vendored result (`08-nc2`), a resolution result (`09-nc3`); delete a case → `frozen oracle has 5 cases, inventory 6` (`10-nc4`); duplicate a case → `27 cases, inventory 26` (`11-nc5`); drift a flatten corpus input → `frozen inputs differ from the corpus tree` (`12-nc6`); drift a vendored corpus text → `frozen inputs differ from the corpus` (`13-nc7`).
- L10: `gofmt -l internal/skills` empty, `go vet ./internal/skills/` rc=0, `grep -n "python3\|exec\.Command\|lib/_internal\|lib/_vendor"` over the three files matches comment lines only (`logs/14-grep-live-legs.log`), `go test ./internal/skills/` rc=0 (`15-package-tests.log`). RED before the goldens existed: all three failed with `read frozen oracle ...: no such file or directory`, rc=1 (`03-red-missing-golden.log`).
- L11: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`16-build.log`), `./tests/validate.sh` rc=0 (`17-validate.log`): `Ran 2345 tests ... OK (skipped=165, expected failures=3)`, parity summary `PASS`, all Go packages `ok`.
- L12: Diff size: 452 insertions + 157 deletions = 609 changed lines across the three test files, 9 over the ~600 budget, goldens excluded. Recorded as a finding for the parent rather than splitting the slice mid-flight.
- L13: Finding (out of scope): `odd/tasks/go-07-s8-skill-contract.md` records "25 cases" for the vendored differential while the corpus has carried 26 since `unicode-space-strip` was added; left untouched (outside this slice's edit surfaces).
- L14: Follow-up (host umask): the frozen flatten snapshot carries the umask-derived modes (0755/644), so the old live differential was umask-invariant and the frozen one was not — a umask 077 developer got a false failure (`logs/18-red-umask077.log`, rc=1). `TestFlattenDifferential` now pins `syscall.Umask(0o022)` with a `t.Cleanup` restore; no test in `internal/skills` uses `t.Parallel` (umask is process-wide). No golden was recaptured: GREEN rc=0 under umask 077 (`19-green-umask077.log`) and under the default umask (`20-green-default-umask.log`), package `-count=1` rc=0 under both.
