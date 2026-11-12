# GO-20 U2e T5 PR4a S4 — hookactuator live oracle

Base: epic/go-single-binary 3adbef1. Branch: change/go-20-u2e-t5-pr4a-s4-hookactuator.
Goal: `internal/sync/hookactuator_test.go` stops exec'ing python3 and reading
`lib/_internal/recipe-materialize.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync),
PR #503 (envscaffold), PR #504 (refreshbundled), PR #505 (vendorskills), PR #506 (conflictscore),
PR #507 (copyapply).

## Specs

- S1: `TestMaterializeHookPythonDifferential` is replaced by `TestMaterializeHookFrozenDifferential` (same
  coverage, no live python differential), which reads a frozen golden
  (`internal/sync/testdata/hookactuator_golden.json`) captured once from the real Python hook authority,
  `_python_materialize_hook_script` (`lib/_internal/recipe-materialize.py:2233`). No python3, no `lib/` access,
  no skip: a missing golden fails.
- S2: The frozen payload carries, per case, the input projection the fixture drove (rel, script, recipe id, the
  merged config, the refresh flag, the recorded provenance, the seeded destination's bytes hash and mode, the
  destination kind and the refresh backup's content-hash name) and the outcome Python produced (`wrote`, `dest`,
  `record`, `disk_sha`, `mode`, `backup`, `backup_sha`, `lines`, `refused`), so the native `MaterializeHook`
  still compares against the legacy outcome for every field the live leg compared.
- S3: Every native assertion of the live leg survives case by case: the same nine-case state matrix (fresh
  write, managed_current backfill, no-provenance preserve, user-modified preserve, managed-stale refresh,
  refresh backup, refresh rollback, symlink guard, refresh symlink guard), asserted per family on `wrote`,
  destination identity, record fields, bytes and mode on disk, the refresh backup bytes, every emitted line and
  whether the call refused.
- S4: The fixture surface is pinned, not merely supplied: the golden freezes the shared fixture layout
  (`hook_sha256`, the root-relative `recipe_dir` and `project_root`) and the test re-derives it from the fixture
  builder, so a layout or hook-source drift fails as `fixture layout diverges` instead of passing through.
- S5: The test asserts a hard-coded independent inventory of the case names (count, uniqueness, order) against
  the golden through `validateGoldenIdentity` before comparing anything, and requires every frozen case to have
  a native scenario and every native scenario a frozen case. A golden that drops, duplicates or reorders a case
  fails as an inventory error.
- S6: Frozen inputs equal the in-test corpus: each case's input projection is compared before any outcome, so
  fixture drift fails as `case "<name>" input diverges`, not as an opaque payload mismatch.
- S7: No absolute host path leaks into the golden: the temporary base directory is tokenized as `<BASE>` on both
  sides (the same `tdProjectText` projection the live leg already applied), the loader refuses any `dest` or
  `backup` that does not start with `<BASE>/` and any payload matching `/Users/`, `/private/` or
  `/var/folders`, and every fixture layout path is root-relative.
- S8: Modes are frozen only where they are explicitly set (the shared write path's `chmod 0755`, the backup
  snapshot's `chmod 0644` and a seeded destination's declared bits); no directory mode and no mode that follows
  the process umask is frozen, so the test passes under `umask 077` without pinning it process-wide
  (`t.Parallel` exists in `internal/sync`).
- S9: Negative controls: mutating one frozen outcome, deleting a case, duplicating a case and drifting one
  frozen input each fail rc=1.
- S10: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, `hookactuator_test.go` free of live
  `python3` / `syncPythonAuthority` / `runSyncPythonOracle` / `lib/_internal` (only the provenance comments
  match), `go test ./internal/sync/ -run 'TestMaterializeHook' -count=1` rc=0 (also under `umask 077`) and
  `go test ./internal/sync/ -count=1` rc=0; `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and
  `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestMaterializeHookPythonDifferential` (python3 driving `lib/_internal/recipe-materialize.py`'s `_python_materialize_hook_script` with the lock/print sinks patched and a project-cache stub, versus the native `MaterializeHook`, compared by `reflect.DeepEqual` over the projected `hkOutcome`) | FREEZE | the payload is tokenizable and stays meaningful in token space: the only host-dependent values are the fixture's temp base (already replaced by `<BASE>` on both sides by `tdProjectText`, which the live leg itself used) and the mode of each file the fixture or the authority sets explicitly, so the whole nine-case outcome matrix — wrote, dest identity, record fields, bytes, mode, backup identity and bytes, emitted lines, refusal — is preserved verbatim. RETIRE was rejected: no other frozen oracle covers this authority. `testdata/hooksrender_golden.json` and `testdata/hooksrunner_golden.json` (PR2-B #488/#489) freeze `lib/_internal/hooks-render.py` and `execute_hooks` — the per-agent hook-config render and the hook command runner — not the gate actuation `MaterializeHook` runs, and the only other gate-surface test (`catalog/recipes/worktree-flow/gate/hookgateactuator_test.go`) covers the wrapper's envelope decode/encode, not this decision + write matrix. RETARGET was rejected for the same reason. The leg also owned the `dest`/`backup` identity assertions the native `TestMaterializeHookStateParity` deliberately zeroes out, so dropping it would have lost coverage no other test holds. The live leg ran exactly this one matrix, so the freeze is 1:1. |

Normalizations, all documented in the test and in the golden's `oracle.note`:

- **`<BASE>` token.** The legacy driver printed absolute destinations and backup paths. The fixture's temp base
  becomes `<BASE>` on both sides through the existing `tdProjectText` projection (which also keeps the POSIX
  doubled-leading-slash form distinct), so the comparison runs in the golden's host-independent space and the
  golden keeps the token visible (`<BASE>/project/ai-specs/recipes/worktree-flow/hooks/h1-fresh-write.sh`).
  Every non-path value is already host-free: the rel-path record fields, the content hashes and the emitted
  lines.
- **Modes are frozen only when explicit.** The shared write path finishes with `file.Chmod(0o755)`, the refresh
  snapshot is `os.Chmod`-ed to `0o644` and every seeded destination is `os.Chmod`-ed to its declared bits by
  `prepare`, so no frozen mode follows the process umask. Directory modes are not frozen at all — the fixture
  creates them with `os.MkdirAll` (`0o755`) and nothing asserts them. A run under `umask 077` proves it.
- **Fixture layout.** `hook_sha256` pins the hook source bytes every case writes; `recipe_dir` and
  `project_root` pin the fixture's root-relative shape. The per-case input projection pins the request fields and
  the seeded state (bytes hash, declared mode, symlink kind, backup name).
- **Authority note.** The legacy driver replaced only the lock/print sinks and the project-cache lookup (the
  Python-precomputed immutable backup path), so the frozen payload is what the live Python call sites would have
  produced — not a second implementation's output.

Shapes the live leg never exercised stay untouched, and the helpers it shares with other files are reused
read-only: `tdProjectText`, `tdReadRegular`, `tdHasPrefix` and `writeTDFile` from `templatedocs_test.go`,
`readGoldenBundle` and `validateGoldenIdentity` from `agentsrender_test.go`, and
`syncPythonAuthority` / `runSyncPythonOracle` only inside the throwaway capture (they stay in `stamps_test.go`,
still used by the templatedocs and hooksrunner tests). The native tests in the file
(`TestMaterializeHookStateParity`, `TestMaterializeHookRefusalsAndBoundary`, `TestMaterializeHookRefreshRollback`)
are preserved: they never called the python oracle.

Cases (9): `h1-fresh-write`, `h2-managed-current`, `h3-no-provenance`, `h4-user-modified`,
`h5-managed-stale-refresh`, `h6-refresh-backup`, `h7-refresh-rollback`, `h8-symlink-guard`,
`h9-refresh-symlink-guard`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S9 | inline | |
| T2 | S10 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then the materialize-helper consumers; one worker at a time, merge after validate.
- L2: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-hookactuator (Robert's
  standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at its `tmp/`.
- L3: Baseline of the live leg before any edit: `TestMaterializeHookPythonDifferential` rc=0 in 0.12s
  (`logs/00-baseline-live-leg.log`).
- L4: RED before the golden existed, with the restructured test in place:
  `hookactuator_test.go:700: read frozen oracle: open testdata/hookactuator_golden.json: no such file or
  directory`, rc=1 (`logs/02-red-missing-golden.log`).
- L5: The capture harness was temporary code (`internal/sync/zz_hookcapture_test.go`: the legacy Python driver
  plus a `HOOKACTUATOR_CAPTURE=<path>` gate) and was removed before the commit, so the golden's shape is exactly
  the struct the frozen test decodes. It built the fixture from the same builder, tokenized the legacy output
  through the same projection and wrote the payload with HTML escaping off. So the golden is legacy output, not
  port output.
- L6: Captured twice into fresh scratch files and `cmp` byte-identical (`logs/04-capture-cmp.log`, rc=0 —
  `logs/03-capture1.log`, `logs/03-capture2.log`): 11435 B, 9 cases. Oracle metadata `python: 3.14.7`,
  `script: lib/_internal/recipe-materialize.py`, `driver: read_recipe + _python_materialize_hook_script with
  load_lock / write_lock / warn / info sinks patched and a project-cache stub`. The grep over the committed
  golden for `/Users/`, `/private/`, `/var/folders` is 0.
- L7: Negative controls (each rc=1; the golden was restored from a scratch copy and `cmp`-verified byte-identical
  after each): mutating one frozen mode → `outcome diverges` (`logs/06-nc-mutate.log`), deleting the rollback
  case → `hookactuator: 8 records, want 9` (`logs/06-nc-delete.log`), duplicating the first case →
  `hookactuator: 10 records, want 9` (`logs/06-nc-duplicate.log`), drifting one frozen seed hash →
  `case "h2-managed-current" input diverges` (`logs/06-nc-inputdrift.log`).
- L8: `gofmt -l internal/sync` empty (`logs/11-gofmt.log`, rc=0), `go vet ./internal/sync/` rc=0
  (`logs/12-vet.log`), and the `syncPythonAuthority|runSyncPythonOracle|python3|lib/_internal|exec.Command`
  grep over the test matches the two provenance comments only.
- L9: `-run 'TestMaterializeHook'` rc=0 (`logs/05-green-focused.log`), the same narrowed run under `umask 077`
  rc=0 (`logs/07-green-umask077.log` — every frozen mode is explicitly set, so the umask cannot change the
  payload), `go test ./internal/sync/ -count=1` rc=0 in 13.3s (`logs/08-green-package-tests.log`).
- L10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/09-build.log`), `./tests/validate.sh` rc=0
  (`logs/10-validate.log`).
- L11: The worktree carries only the three intended paths (test, golden, this doc); the capture harness, both
  captures and all logs live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
