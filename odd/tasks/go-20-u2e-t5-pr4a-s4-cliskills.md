# GO-20 U2e T5 PR4a S4 — internal/cli skills live oracles

Base: epic/go-single-binary 233dcd2. Branch: change/go-20-u2e-t5-pr4a-s4-cliskills.
Goal: `internal/cli/skills_test.go` stops exec'ing the legacy `lib/skills-*.sh` scripts and
`python3` without silent coverage loss. Same discipline as PR #496 (doctor), #497 (rulesaudit),
#498 (skills), #499 (conflicts), #501 (briefpolicy), #502 (checksync), #503 (envscaffold),
#504 (refreshbundled), #505 (vendorskills), #511 (platform.sh), #512 (sync-agent.sh).

## Specs

- S1: `TestSkillsListDifferentialAgainstShell` (same name) no longer execs
  `bash lib/skills-list.sh`: it replays the frozen legacy output of
  `internal/cli/testdata/skills_list_golden.json` against the built binary. No bash, no python3,
  no `lib/` read, no environment skip — a missing bundle fails.
- S2: `TestSkillsRemoveDifferentialAgainstShell` (same name) reads
  `testdata/skills_remove_golden.json` the same way.
- S3: `TestSkillsAddDifferentialAgainstShell` (same name) reads
  `testdata/skills_add_golden.json` the same way.
- S4: The five deviation tests no longer exec the shell either: their legacy side is the frozen
  record named in the bundle (`missing path` for list/remove/add, `read-only manifest` and
  `table-shaped deps` for add, `trailing flag` for add). The native side and the recorded
  divergence are asserted in the test exactly as before.
- S5: Every native (non-legacy) assertion of the five tables and five deviation tests survives: the
  exit code, stdout and stderr in the tokenized space, the produced project tree, the untouched-project
  assertion for the pre-write refusals, the read-only CLI home snapshot, the 0444 mode assertions,
  the in-process `Run` cases (`*NativeNeedsNoBashOrPython`, `SkillsListInvalidManifestContinues`) and
  the shared fixtures/helpers (`skillsListHome`, `skillsListFixtures`, `buildRemoveFixture`,
  `buildAddFixture`, `skillsTreeSnapshot`).
- S6: Each bundle carries a hard-coded case inventory in capture order (count, uniqueness, order),
  checked against both the bundle and the in-test corpus before any comparison. A bundle that drops,
  duplicates or reorders a case fails as an inventory error.
- S7: Frozen inputs equal the in-test corpus: every record pins the pre-run project tree (paths plus
  bytes, snapshotted before the run), and the test compares its own fixture before running the port,
  so fixture drift fails as input drift.
- S8: No absolute host path leaks into a bundle: `<DIR>` (project root), `<HOME>` (cli home),
  `<SRC>` (local vendor source) and `<ROOT>` (the checkout, for bash's own script-name prefix) are
  tokenized in stdout, stderr and file bytes, resolved spelling included. `grep -c
  "/Users/\|/private/\|/var/folders"` over the three bundles is 0.
- S9: Negative controls: mutating one frozen result, deleting a case, duplicating a case and drifting
  a frozen input each fail rc=1 in every family.
- S10: `gofmt -l internal/cli` empty, `go vet ./internal/cli/` rc=0, the converted tests free of live
  `lib/`/`python3` execution (comments only), `go test ./internal/cli/ -count=1` rc=0 (also under
  `umask 077`).
- S11: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestSkillsListDifferentialAgainstShell` (`bash lib/skills-list.sh <args>` vs `skills list`, compared on rc, stdout, stderr) | FREEZE | the payload is host-independent once `<DIR>`/`<HOME>` are tokenized: the listing prints dep ids, the stable manifest fields, skill names and the catalog's own names. It is the only oracle for the four list sections (`internal/cli/skills.go` has in-process assertions only for the malformed-manifest branch). 1:1 with the live table's ten cases. |
| `TestSkillsRemoveDifferentialAgainstShell` (`bash lib/skills-remove.sh` vs `skills remove`, compared on rc, stdout, stderr and the twin project trees) | FREEZE | stdout is the three success lines with the project root tokenized; the tree is the manifest, the pruned `.deps` copy and the lock, all frozen as bytes. RETIRE was rejected: `skills remove` has no other frozen oracle (the `*NativeNeedsNoBashOrPython` guard asserts routing, not output). 1:1 with the twelve cases. |
| `TestSkillsAddDifferentialAgainstShell` (`bash lib/skills.sh add` / `lib/skills-add.sh` / `lib/skills.sh` vs `skills add` / `add-dep` / `skills`, compared on rc, stdout, stderr and the twin trees) | FREEZE | same shape: stdout carries the summary and the sync delegation's lines with the project root tokenized; the tree carries the appended `[[deps]]`, the vendored `.deps` copy and the lock. It is the only oracle for the dispatcher remainder and the add-dep label. 1:1 with the thirty-one cases. |
| `TestSkillsRemoveReadOnlyDeviation`, `TestSkillsAddReadOnlyDeviation`, `TestSkillsRemoveMissingPathDeviation`, `TestSkillsListMissingPathDeviation`, `TestSkillsAddMissingPathDeviation`, `TestSkillsAddTrailingFlagDeviation`, `TestSkillsAddNonListDepsDeviation` (each ran a bash leg plus native assertions) | FREEZE (legacy side only) | the divergence itself stays a test-local assertion (the port words the failure differently by ADR 0002); the frozen record supplies the legacy rc/stdout/stderr, so no shell runs and the recorded divergence is still visible in one place. |

Normalizations, documented in the test and in each bundle's `oracle.note`:

- **Tokenization.** `<DIR>`, `<HOME>`, `<SRC>` and `<ROOT>` replace each run's temporary project
  roots, cli homes, vendor sources and the checkout (bash prefixes its diagnostics with the script
  path), raw and symlink-resolved.
- **Attribution (`<OWNER>`).** `skills add` derives the attribution for a local source from the
  path's first segment (`/Users/...` → "Users", `/tmp/...` → "tmp"), so the bundle freezes
  `<OWNER>` and the test expands it from its own fixture path. This keeps the bundle host-free on
  macOS and on Linux.
- **Lock `synced_at`.** The delegating sync stamps the lock with "now" at second granularity; both
  sides normalize it to `<wall-clock>` (the parity corpus's own normalization). Every other lock byte
  compares.
- **Dep skill hash.** The lock's `[deps.*.skills.*]` hash covers the rendered `SKILL.md`, whose
  frontmatter embeds the dep's source — a temp directory here — so both sides normalize that value
  to `<dep-hash>`. The rendered bytes themselves are frozen and are the hash's input.
- **Modes.** File and directory modes are **not** frozen: fixtures are created with `os.WriteFile`
  under the process umask and this package also runs `t.Parallel()` tests that must not have the
  umask pinned under them. The two read-only cases assert their explicit `0444` natively.
- **Tracebacks.** The two Python-traceback records (read-only add, table-shaped deps) replace
  absolute paths with `<PATH>`: the interpreter install layout is host state, not behaviour; the
  tests assert the traceback's class (`PermissionError` / `Traceback`).

Shapes the live tables never exercised stay untouched, and the helpers other files use are reused
read-only: `skillsOracle`, `skillsRemoveGateBinary`, `skillsRemoveEnv`, `skillsProjectTree` and
`runOne`/`smokeSetup`/`locateRoot` are still defined here because `recipe_test.go` uses them for its
own live legs; no converted test in this file calls them.

Cases: list 11 (`10` table + `missing path`), remove 14 (`12` + `read-only manifest` + `missing
path`), add 35 (`31` + `trailing flag` + `missing path` + `read-only manifest` + `table-shaped
deps`).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6, S7, S8, S9 | inline | |
| T2 | S10, S11 | inline | |

## Log

- L1 (Robert): "a" — convert the `internal/cli` live `lib/` reads now, one PR per file, same cycle;
  the file surfaced in the S4 inventory of PR #511.
- L2: Baseline at 233dcd2: all fourteen affected tests pass, none skip, with `dist/ai-specs` built
  first (`logs/01-baseline-skills.log`, rc=0; `logs/00-build.log`, rc=0).
- L3: Scratch/TMPDIR: `/Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-cliskills`
  (Robert's standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at
  its `tmp/`, logs in its `logs/`.
- L4: RED before any bundle existed: `skills_list_golden.json: 10 records, want 11` and
  `read frozen oracle skills_{remove,add}_golden.json: no such file or directory` for all ten
  frozen tests (`logs/02b-red-all.log`, rc=1).
- L5: Capture harness was temporary code in `internal/cli/zz_capture_temp_test.go`, gated by
  `CLI_SKILLS_CAPTURE=<dir>` and deleted before the commit, so the normalization is the same code
  the frozen tests compare with. It rebuilt each case's fixture from the same corpus and ran the real
  legacy route (with `WORKTREE_GATE_BIN` pinned so the shell's lock bridge prints no fail-open
  warning), froze the pre-run project tree alongside the legacy output, and failed if an absolute
  host path survived tokenization.
- L6: First capture pass was **not** reproducible for add (`cmp` differed): (a) three lock dep
  hashes changed because the hash covers a rendered `SKILL.md` that embeds the temp source path, and
  (b) the local source's derived attribution is the path's first segment. Fixed by the `<dep-hash>`
  normalization and the `<OWNER>` placeholder; the second pass is byte-identical
  (`logs/05-capture-cmp.log`, rc=0).
- L7: Captured twice into fresh scratch dirs and `cmp` byte-identical for all three bundles:
  list 18571 B / 11 cases, remove 25106 B / 14 cases, add 48734 B / 35 cases. Oracle metadata
  `python: 3.14.7`, `base: 233dcd2`, script blob ids
  `skills-list.sh 031e174b…`, `skills-remove.sh 8e56be3e…`, `skills-add.sh 9e114ece…`,
  `skills.sh baa2c558…`. No wall-clock value and no git value is frozen.
- L8: Negative controls (each rc=1; bundle restored and `cmp`-verified after each): mutate one frozen
  result → stdout mismatch (`logs/07a-nc-list-1-mutate.log`, `logs/07b-nc-remove-1-mutate.log`,
  `logs/07c-nc-add-1-mutate.log`); delete a case → `10/13/34 records, want 11/14/35`; duplicate a
  case → `11/15/36 records, want 11/14/35`; drift a frozen input → `project input: node … differs`.
- L9: Independence: with `lib/skills-list.sh`, `lib/skills-remove.sh`, `lib/skills-add.sh` and
  `lib/skills.sh` temporarily renamed away, the fourteen converted tests PASS — none skips
  (`logs/08-independence-renamed-lib.log`, rc=0, 14 PASS / 0 SKIP); `git status --short lib/` empty
  afterwards.
- L10: `gofmt -l internal/cli` empty, `go vet ./internal/cli/` rc=0; the only `python3`/`os/exec`
  references left in the file are the `skillsOracle` and worktree-gate helpers that `recipe_test.go`
  still calls, the gate build, comments and the PATH-shadowed native guards
  (`logs/13-grep-live-legs.log`). Host-path grep over the three bundles: 0.
- L11: `go test ./internal/cli/ -count=1` rc=0 (`logs/16-green-package-tests.log`) and the same under
  `umask 077` rc=0 (`logs/14-green-umask077.log`).
- L12: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/17-build.log`), `./tests/validate.sh`
  rc=0 (`logs/18-validate.log`).
- L13: A PATH-shadowed run (only `go`/`git`/`sh`/`bash` linked in, so `python3` is unreachable) was
  attempted as an extra independence probe and is NOT evidence: the native sync delegation's
  child-output filter cannot create its temp file under that stripped PATH (`internal/sync/step.go`,
  pre-existing, unrelated to this change), so five sync rows fail for environmental reasons. The
  independence claim rests on L9 (all four scripts renamed away, 14 PASS / 0 SKIP) and on the grep in
  L10 — no converted test execs anything.
- L14: The worktree carries only the three bundles, the test file and this doc; the capture harness,
  both captures and all logs live in the scratch dir, and the commit is left to the orchestrator
  (worker boundary).
