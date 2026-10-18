# Go 20 / U4 — the `init` verb, native

Unit document for the fourth port of the Python exit program (`odd/tasks/go-20-python-exit.md`, T4).
Worktree `.worktrees/go-20-u4a-init-core`, branch `change/go-20-u4a-init-core`, based on the epic tip.

## Specs

- S1 — Program request, verbatim (2026-10-15): "quiero que este como si se hubiera hecho con go desde
  0". Program rule (S4): zero Python under the product tree; bash exists only as wrappers (the shims
  of unported verbs) and never as the orchestrator of a verb.
- S2 — Session request, verbatim (2026-10-17): "continuemos donde nos quedamos respecto a la migración
  a full go". The unit continues the program at T4; nothing in this document re-opens a closed unit.
- S3 — No observable behaviour changes without evidence: RED→GREEN per rule, the differential (parity
  corpus and/or `tests/validate.sh`), and the recorded exit codes, stdout and stderr of every rule the
  port touches. The legacy `lib/init.sh` output is the contract, line for line, except where a
  divergence is written down below with its reason.
- S4 — `init` is idempotent and never overwrites the manifest. `ai-specs/ai-specs.toml` is user-owned:
  `--force` does not touch it, only the managed agent block in the root `.gitignore`.
- S5 — The classic (non-interactive) path is the slice's product: no prompt, template manifest,
  baseline `AGENTS.md`, both `.gitignore` files, the lock stamp and the next-steps text. Exit codes:
  `0` success, `1` unusable target / cancel / failure, `2` usage error.
- S6 — The interactive wizard stays in the legacy script for this slice. `init` decides the TUI gate
  with the legacy predicate — `--tui`, or (auto) stdin and stdout are TTYs, no `--name`, no `--force`
  and no manifest yet — and delegates the WHOLE verb to `lib/init.sh` with the original argv when it
  fires, so the wizard's behaviour, messages and exit codes are unchanged. The port owns the verb
  again once the native wizard lands (T4d).
- S7 — Asset sources are the embedded trees (`assets.FS("templates")`), not `${AI_SPECS_HOME}` on
  disk: the binary carries `ai-specs.toml.tmpl` and `gitignore-root.tmpl` at its own revision.
- S8 — Divergence, recorded not accidental: the legacy manifest render is
  `sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g"`, which mangles a project name containing `&`, `/` or a
  backslash (sed replacement syntax) and silently emits the wrong `name = "..."`. The port substitutes
  literally, so such a name lands verbatim. Pinned by a test; the legacy text stays the reference for
  every name the sed path already rendered correctly.
- S9 — The two steps with no native authority yet keep their existing seam, and each is named in the
  code: `refresh-bundled` (the flattened cache, whose native verb is a compatibility no-op because the
  assets are embedded) and the lock `[meta]` stamp. They run through the same legacy entry the shell
  used, exactly once, so a fresh `init` still produces the lock the rest of the system reads. T4c
  retires them.
- S10 — Divergence, recorded not accidental: the legacy route renders the baseline brief through
  Python, whose thin bridges announce themselves on stderr (`GO_*_BRIDGE_FALLBACK: no verified
  worktree-gate binary; using the temporary Python <authority>`). The native route renders in process,
  has no Python fallback to announce, and therefore prints none of those lines. Everything else on
  stderr is the contract.
- S11 — `ai-specs/.gitignore` is silent in the wrapper sense but not in `init`: the shell calls
  `gitignore-render.py` directly, so its `  ✓ wrote <abs path> (N dep(s))` status line IS part of
  init's stdout. The native step prints it through the same wrapper.

- S12 — The judgment round (below) found that the shell's own failure messages are not a contract: `shift 2`
  out of range on a trailing `--name`, `mkdir`'s diagnostic, and `gitignore-render.py`'s traceback are
  accidents of bash and Python. The port keeps their EXIT CODES and streams, and replaces their text
  with one line that names the flag, the directory or the manifest. Each is pinned, and none of them
  changes a successful run's bytes.

## Slicing

| slice | scope | retires |
| --- | --- | --- |
| **T4a** | classic non-interactive `init` native: args, target resolution, directories, template manifest, `AGENTS.md` baseline brief, root `.gitignore` block, `ai-specs/.gitignore`, lock stamp, next steps; TUI gate delegates | `init.sh:35-120,195-233,235-282,284-325,327-348` shell orchestration |
| T4b | the wizard, native (`internal/tui`): name, topology, agents, recipes, confirm, manifest render, JSON sidecar | `init_tui.py` |
| T4c | lock `[meta]` writer + `refresh-bundled` flatten retirement in both `init` and `sync` | `cli_version.py stamp-meta`, `refresh-bundled.py` |
| T4d | harness env write path (`ai-specs.env`, `ai-specs.env.example`, managed `.envrc`, `direnv allow`) | `env_scaffold.py` write subset, `envrc-scaffold.py` |

## Tasks

- T1 (S3, S5) — the native `runInit` route with the legacy argument contract (`--name`, `--name=`,
  `--force`, `--adopt-brief`, `--tui`, `--no-tui`, `--`, `-h`, unknown flag and extra positional
  errors with their exact messages and exit codes) and the `--help` text.
- T2 (S5) — target resolution (cwd default, not-a-directory error and exit 1, absolute path, name from
  basename) and the banner with its four rows.
- T3 (S4, S7) — directories, template manifest write (never overwrite), and the recorded sed
  divergence (S8).
- T4 (S5) — the baseline `AGENTS.md` chain in process: policy gate, render, placeholder fallback, and
  the `brief.render = false` branch, all with the legacy messages.
- T5 (S5) — root `.gitignore` agent block (append / keep / refresh on `--force`, marker-idempotent)
  and `ai-specs/.gitignore` from the manifest, both in process.
- T6 (S6) — the TUI gate and the delegation to the legacy script.
- T7 (S9) — the lock stamp and the bundled step, through the named seam.
- T8 (S5) — the next-steps text, byte-identical to the heredoc.
- T9 (S3) — tests: Go unit tests for the argument contract and each rule, plus a shell test that runs
  the native route and the legacy route in two throwaway projects and diffs stdout, stderr, exit code
  and the produced tree.

## Log

- L1 — Human, verbatim (2026-10-17): "continuemos donde nos quedamos respecto a la migración a full
  go".
- L2 — Evidence (2026-10-17), delegated read-only mapping of the legacy surface (`init.sh` 348 lines,
  `init_tui.py` 429, `env_scaffold.py` 667, `envrc-scaffold.py` 46, `refresh-bundled.py` 160,
  `cli_version.py` 302): the phases above, what is already native (`internal/target`, `internal/sync`
  brief/gitignore, `internal/cliversion` read side, `internal/lock` serialization) and what is not
  (manifest template write, wizard, env write path, `[meta]` writer). The slicing table is the
  consequence of that measurement.

## T4a — judgment and fix round

Three independent reads of the frozen candidate (`266684e0` on base `f2cb80a1`) ran after the unit was
written, because the native review of this candidate could not close (two of four lenses' models could
not finish; recorded in the program's memory, not in this document):

- **Judgment Day, judge A** — 8 rows: 4 WARNING, 4 SUGGESTION, 0 CRITICAL.
- **Judgment Day, judge B** — 6 rows: 1 HIGH, 3 WARNING, 2 SUGGESTION. Its HIGH is the same defect
  judge A filed as a WARNING at the same line, reproduced independently.
- **Independent verifier** — `PASS`, S1-S11 met, S8/S10 real and singular, one advisory (stderr text
  on the manifest-as-a-directory path), plus two regression tests to hand back.

The fix batch, in one commit:

| id | finding | fix |
| --- | --- | --- |
| C1 | a trailing `--name` bootstrapped the project where the shell aborts at `shift 2` with exit 1 and no writes | the parser refuses it before any write, exit 1, `--name requires a value` (S12) |
| C2 | bare `-` was read as a target path; the shell's `-*` matches it as an unknown flag | it is an unknown flag again, exit 2 with the help hint |
| C3 | the name was substituted into `name = "..."` unescaped, so a backslash or a quote produced a manifest that cannot parse — permanently, because init never overwrites it | `tomlBasicStringBody` escapes the TOML basic string; S8 grows this half and a test pins `a\q`, `a"b` and `a\nb` |
| C4 | the `ai-specs/.gitignore` step failed silently where the shell printed a traceback naming the manifest | the step prints one line naming both paths (S12) |
| C5 | `appendGitignoreBlock` truncated the user-owned root `.gitignore` before writing; the shell appended and renamed | the same bytes are written beside the target and renamed into place |
| C6 | one test asserted `filepath.Base` on its own literal, and the parser cases sat exactly where the drift was | the basename is asserted through `runInit`; `--name`, `--name=` and `-` are pinned; the next-steps text is compared whole |
| C7 | the new route constant broke `gofmt` alignment | `gofmt -w` on the touched files |
| C8 | the silent writer's comment claimed `init` uses it, contradicting S11 | the comment names the printing wrapper as init's call |
| C9 | `mkdir` failure was reported on stdout with a partial `✓` line already printed | both directories are created first, the diagnostic goes to stderr, and no `✓` is printed on failure |
| C10 | a signal-killed `refresh-bundled` child returned `-1`, which `os.Exit` turns into 255; the shell reports 128+signal | the 128+signal rule `runShim` already implements |
| C11 | the verifier's probe had no regression test | `test_manifest_directory_is_never_destroyed_and_exits_1` and `TestInitManifestDirectoryIsNotDestroyed` |

Evidence after the batch (same interpreter, `/tmp/pybin/python3`):

| gate | result |
| --- | --- |
| `go test ./internal/cli/ ./internal/sync/` | both packages `ok` |
| `python3 -m unittest tests.test_init_native tests.test_init_brief_route tests.test_init_tui` | 44 tests, OK |
| `python3 tests/parity/run.py` | 29 fixtures, failing 0 |

One incident is recorded rather than hidden: writing the C11 regression test, a first version passed the
project path to `--name`, which made the legacy route read its own cwd as the project and run a real
`init` in the worktree. The only write it made was a re-stamp of `ai-specs/.ai-specs.lock`, restored
with `git checkout`. The test now runs with `cwd` inside the throwaway project and its docstring says
why.

## T4a — verification of the first cut (superseded by the round above)

Evidence gathered before the judgment round, kept because it records the RED and the manual
differential that motivated the committed black-box test. The numbers below are pre-fix; the
post-fix gate is the table in the section above.

Run from `.worktrees/go-20-u4a-init-core` with
`PATH=/tmp/pybin:$PATH` (the canonical 3.14.7 interpreter; the machine's `/usr/bin/python3` is 3.9
and fails unrelated suites — recorded in the program's memory notes):

| gate | command | result |
| --- | --- | --- |
| Go suite | `go test ./cmd/... ./internal/...` | 0 failing packages |
| init contract, classic + wizard seam | `python3 -m unittest tests.test_init_native tests.test_init_brief_route tests.test_init_tui` | 42 tests, OK |
| differential parity | `python3 tests/parity/run.py` | 29 fixtures, failing 0 (both modes) |

- RED, observed: the S8 test (`test_name_with_ampersand_lands_literally_on_the_native_route`) fails
  against the base commit's binary, where `init` still routes to the shim and the manifest carries
  `name = "R{{PROJECT_NAME}}D"`; it passes with this slice.
- The manual differential before the committed test existed: two twin projects (same basename, own
  isolated homes), stdout identical after dropping the random `RECIPE_MCP_TEMP:` names, same produced
  file set, and the only stderr difference is S10's absent bridge notices.
- Two divergences are written down with their reason rather than hidden: S8 (literal name
  substitution) and S10 (no Python bridge notices). S11 records that the `ai-specs/.gitignore` status
  line is part of init's stdout, because the shell calls the renderer directly.
- Not a divergence, verified: the `keep` and `--force` branches of the root `.gitignore` block match
  the legacy bytes exactly, including the single separating newline the shell's `append_block` writes
  (which is NOT `gitignore-root-refresh.py`'s two-newline rule — the two algorithms differ and init
  uses its own).
