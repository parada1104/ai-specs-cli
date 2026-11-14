# GO-20 U2e T5 PR4a S4 — templatedocs live oracle

Base: epic/go-single-binary 60dc974. Branch: change/go-20-u2e-t5-pr4a-s4-templatedocs.
Goal: `internal/sync/templatedocs_test.go` stops exec'ing python3 and reading
`lib/_internal/recipe-materialize.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync),
PR #503 (envscaffold), PR #504 (refreshbundled), PR #505 (vendorskills), PR #506 (conflictscore),
PR #507 (copyapply), PR #508 (hookactuator).

## Specs

- S1: `TestMaterializePythonDifferential` is replaced by `TestMaterializeFrozenDifferential` (same coverage, no
  live python differential), which reads a frozen golden (`internal/sync/testdata/templatedocs_golden.json`)
  captured once from the real Python authority, `_python_materialize_template`
  (`lib/_internal/recipe-materialize.py:1430`) and `materialize_doc` (`:1551`). No python3, no `lib/` access, no
  skip: a missing golden fails.
- S2: `TestMaterializeTemplateGitDestParity` keeps its name and its native assertions (the authority wrote, the
  hook landed in the primary repo's shared hooks dir, realpath-relative destination identity) but compares
  against the frozen record of the same Python authority on a `.git/hooks/...` destination in a normal and a
  linked temporary Git worktree instead of a live python run. A missing golden fails; only a missing `git`
  binary skips (the fixture needs a throwaway repo).
- S3: The frozen payload carries, per matrix case, the input projection the fixture drove (kind, tokenized
  target and source, recipe id, condition, update policy, config, managed entry, destination kind, seeded bytes
  hash) and the outcome Python produced (`wrote`, `dest`, `record`, `disk_sha`, `lines`, `error`); per git case
  it carries the kind, target, source rel path, source hash and the root-relative repo layout plus the same
  outcome fields Python's git driver printed (`dest_rel`, `sha`, `mode`, `record`, `lines`, `error`). So the
  native authorities still compare every field the live legs compared.
- S4: Every native assertion of both live legs survives case by case: the full 27-case state matrix (templates
  t1-t9 and docs d1-d18: write, seed, skip, user-modified warn, managed-stale refresh, stale non-auto warn,
  directory/symlink/dangling preservation, noncanonical target keys, out-of-project and doubled-slash
  destinations, absolute doc source) and both git worktree kinds.
- S5: The fixture surface is pinned, not merely supplied: the golden freezes the matrix fixture layout
  (root-relative `recipe_dir` and `project_root`, the rendered-source family bytes hash and the doc body hash)
  and the git leg's root-relative repo layout per kind, and the test re-derives both from the builders, so a
  layout or source drift fails as `fixture layout diverges` instead of passing through.
- S6: The test asserts hard-coded independent inventories of the case names (count, uniqueness, order) — 27
  matrix cases and the two git kinds — through `validateGoldenIdentity` before comparing anything, and requires
  every frozen case to have a native scenario and every native scenario a frozen case. A golden that drops,
  duplicates or reorders a case fails as an inventory error.
- S7: Frozen inputs equal the in-test corpus: each case's input projection is compared before any outcome, so
  fixture drift fails as `case "<name>" input diverges`, not as an opaque payload mismatch.
- S8: No absolute host path leaks into the golden: the temporary base directory is tokenized as `<BASE>` (and
  the POSIX doubled-slash form as `<//BASE>`) through the same `tdProjectText` projection the live leg already
  applied to both sides, the loader refuses any payload matching `/Users/`, `/private/` or `/var/folders` and
  any absolute path that does not start with the token, and every fixture layout path is root-relative.
- S9: Modes are frozen only where they are explicitly set: the matrix leg freezes no mode at all, and the git
  leg freezes only the written hook's 0755, which the authority derives from the SOURCE mode by an explicit
  `chmod` on the destination — the fixture sets that source mode explicitly (after the umask), exactly as the
  hookactuator fixture does. No directory mode is frozen and no frozen mode follows the process umask, so the
  tests pass under `umask 077` without pinning it process-wide (`t.Parallel` exists in `internal/sync`).
- S10: Negative controls: mutating one frozen outcome, deleting a case, duplicating a case and drifting one
  frozen input each fail rc=1.
- S11: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, `templatedocs_test.go` free of live
  `python3` / `syncPythonAuthority` / `runSyncPythonOracle` / `lib/_internal` (only the provenance comments
  match), `go test ./internal/sync/ -run 'TestMaterialize' -count=1` rc=0 (also under `umask 077`) and
  `go test ./internal/sync/ -count=1` rc=0; `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and
  `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestMaterializePythonDifferential` (python3 driving `lib/_internal/recipe-materialize.py`'s `_python_materialize_template` / `materialize_doc` over the 27-scenario matrix, with the lock/copy/print sinks patched, versus the native `MaterializeTemplate` / `MaterializeDoc`, compared by `reflect.DeepEqual` over the projected `tdOutcome`) | FREEZE | the payload is tokenizable and stays meaningful in token space: the only host-dependent values are the fixture's temp base (already replaced by `<BASE>` / `<//BASE>` on both sides by `tdProjectText`, which the live leg itself used), and no mode is compared at all, so the whole outcome matrix — wrote, dest identity (including the two out-of-project destinations), record fields, bytes on disk, every emitted line and the error — is preserved verbatim. RETIRE was rejected: no other frozen oracle covers these authorities. `testdata/copyapply_golden.json` (PR #507) freezes `PlanCopyItems`, the *plan* the materialize call sites hand to the copy executor, not the template/doc classification + write matrix, and `testdata/materialize_golden.json` freezes the gate-side `--materialize-template` envelope, not this root wrapper's matrix. RETARGET was rejected for the same reason. The leg also owned the `dest` identity assertions the native `TestMaterializeStateParity` deliberately zeroes out, so dropping it would have lost coverage no other test holds. The live leg ran exactly this one matrix, so the freeze is 1:1. |
| `TestMaterializeTemplateGitDestParity` (python3 driving `_python_materialize_template` through `resolve_template_dest` in a throwaway normal and linked Git worktree, versus the native `MaterializeTemplate`, compared by `reflect.DeepEqual` over realpath-relative identity, bytes, mode, record and lines) | FREEZE | the outcome is already host-free: the destination is compared as a realpath-relative path under the primary repository, the emitted line carries a project-relative target, and the only absolute values in the driver's output were dropped before the comparison. The 0755 mode is set explicitly by the shared write path, so the freeze is umask-independent. RETIRE was rejected because no other test covers a `.git/hooks/...` destination: `TestMaterializeStateParity` and `TestMaterializeRefusalsAndBoundary` only write inside `ai-specs/recipes/...`, and the gate-side `materialize_golden.json` fixture has no `.git` destination. RETARGET has no candidate. The live leg ran exactly these two worktree kinds, so the freeze is 1:1. |

Normalizations, all documented in the test and in the golden's `oracle.note`:

- **`<BASE>` / `<//BASE>` tokens.** The legacy matrix driver printed absolute destinations; the fixture's temp
  base becomes `<BASE>` on both sides through the existing `tdProjectText` projection, which maps the POSIX
  doubled-leading-slash spelling (`.git`-unrelated `d17`) to the distinct `<//BASE>` token. The frozen input
  projection tokenizes the two absolute targets/sources the same way, so the golden holds no host path.
- **Modes.** The matrix leg freezes no mode (none is compared); the git leg freezes the written hook's 0755,
  which the authority chmods from the fixture's explicitly-set source mode. No directory mode is frozen. A run
  under `umask 077` proves it.
- **Fixture layout.** The matrix leg pins `recipe_dir`, `project_root`, the rendered template source hash and the
  doc body hash; the git leg pins each kind's root-relative `main`, `project_root` and `recipe_dir` plus the
  hook source hash (in the per-case input, because the linked worktree's project root differs).
- **Authority note.** The legacy drivers patched only the lock/copy/print sinks, so the frozen payload is what
  the live Python call sites would have produced — not a second implementation's output.

Shapes the live legs never exercised stay untouched, and the helpers they share with other files are reused
read-only: `tdProjectText`, `tdReadRegular`, `tdHasPrefix`, `tdLines`, `writeTDFile`, `tdCount`, `tdGit`,
`tdBuildGitProject`, `tdRealRel` from this file, `readGoldenBundle` and `validateGoldenIdentity` from
`agentsrender_test.go`, and `syncPythonAuthority` / `runSyncPythonOracle` only inside the throwaway capture
(they stay in `stamps_test.go`, still used by the hooksrunner tests and retired in the next slice). The native
tests in the file (`TestMaterializeStateParity`, `TestMaterializeRefusalsAndBoundary`) are preserved: they never
called the python oracle.

Cases (27): `template_t1.sh` … `template_t8.sh`, `doc_d1.md` … `doc_d7.md`, `doc_d8-dir`, `doc_d9-link`,
`doc_d10.md` … `doc_d14.md`, `doc_d15-dangling`, `doc_d16.md`, `doc_d17.md`, `doc_d18.md`, `template_t9.sh`
(capture order); git: `normal`, `linked`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S10 | inline | |
| T2 | S11 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then the materialize-helper consumers; one worker at a time, merge after validate.
- L2: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-templatedocs (Robert's
  standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at its `tmp/`.
- L3: Baseline of both live legs before any edit: rc=0 in 1.07s (`logs/00-baseline-live-legs.log`).
- L4: RED with the restructured frozen tests in place and no golden:
  `templatedocs_test.go:1185: read frozen oracle: open testdata/templatedocs_golden.json: no such file or
  directory` for the matrix leg and `:1268` for the git leg, rc=1 (`logs/01-red-missing-golden.log`).
- L5: The capture harness was temporary code (`internal/sync/zz_tdcapture_test.go`: the two legacy Python
  drivers plus a `TEMPLATEDOCS_CAPTURE=<path>` gate) and was removed before the commit, so the golden's shape
  is exactly the struct the frozen tests decode. It built both fixtures from the same builders, tokenized the
  legacy output through the same projection and wrote the payload with HTML escaping off — legacy output, not
  port output. The inventory order was corrected after the first GREEN attempt caught `template_t9.sh` in the
  builder between `doc_d15-dangling` and `doc_d16.md`, and the doubled-slash non-vacuity marker was tightened to
  the real `<//BASE>` prefix.
- L6: Captured twice into fresh scratch files and `cmp` byte-identical (`logs/03-capture-cmp.log`, rc=0 —
  `logs/02-capture1.log`, `logs/02-capture2.log`): 33091 B, 27 matrix cases + 2 git cases. Oracle metadata
  `python: 3.14.7`, `script: lib/_internal/recipe-materialize.py`. The grep over the committed golden for
  `/Users/`, `/private/`, `/var/folders` is 0.
- L7: Negative controls (each rc=1; the golden was restored from a pristine scratch copy and `cmp`-verified
  byte-identical after each): mutating one frozen outcome → `outcome diverges` (`logs/07-nc-mutate.log`),
  deleting `template_t9.sh` → `templatedocs matrix: 26 records, want 27` (`logs/07-nc-delete.log`),
  duplicating the first case → `28 records, want 27` (`logs/07-nc-duplicate.log`), drifting one frozen seed hash
  → `case "template_t2.sh" input diverges` (`logs/07-nc-inputdrift.log`).
- L8: `umask 077` caught a real umask dependency in the git fixture, not in the authority: the fixture wrote the
  hook source with mode 0755 and the authority chmods the destination to the SOURCE mode, so under a 077 umask
  the frozen `mode: 755` became `700` (`logs/06-green-umask077.log`, first run rc=1). Fixed by having
  `tdBuildGitProject` chmod the source explicitly (the same neutralization `prepare` does in the hookactuator
  fixture), which keeps the frozen 0755 and loses no assertion; the frozen payload is unchanged because the
  capture ran under the default umask and the source mode is not part of any frozen input. Re-run rc=0.
- L9: `gofmt -l internal/sync` empty (`logs/11-gofmt.log`), `go vet ./internal/sync/` rc=0 (`logs/12-vet.log`),
  and the `syncPythonAuthority|runSyncPythonOracle|python3|lib/_internal|exec.Command` grep over the test
  matches two provenance comments and the surviving `exec.Command("git", ...)` fixture helper only
  (`logs/11-greps.log`).
- L10: `-run 'TestMaterialize'` rc=0 with 110 subtests (`logs/05-green-focused.log`), the same run under
  `umask 077` rc=0 (`logs/06-green-umask077.log`), `go test ./internal/sync/ -count=1` rc=0 in 12.35s
  (`logs/08-green-package-tests.log`), `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/09-build.log`),
  `./tests/validate.sh` rc=0 with `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`
  (`logs/10-validate.log`).
- L11: Findings. (a) `odd/tasks/go-07-s12-templates.md:113` still names `TestMaterializePythonDifferential`;
  that doc is outside this slice's edit surfaces, so it is reported, not edited. (b) The test-file diff is
  369+/237- (606 changed lines), above the ~500 guideline; both legs are complete and validated, so the parent
  may want two commits (`matrix` then `gitdest`) for review. (c) The worktree carries only the three intended
  paths; the capture harness, both captures, the pristine copy and all logs live in the scratch dir, and the
  commit is left to the orchestrator (worker boundary).
