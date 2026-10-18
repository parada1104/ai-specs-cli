# Go 20 / U5 — `skills` and `add-dep`, native

Unit document for T5 of the Python exit program (`odd/tasks/go-20-python-exit.md`). Worktree
`.worktrees/go-20-u5-skills`, branch `change/go-20-u5-skills`, based on the epic tip `e53e144`.

## Specs

- S1 — Program request, verbatim (2026-10-15): "quiero que este como si se hubiera hecho con go desde
  0". Program rule (S4): zero Python under the product tree; bash only as wrappers.
- S2 — Session request, verbatim (2026-10-18): "Ok, tú dale, continuemos." The order chosen for the
  program: U5 before U6 because its native cores already exist (`internal/projectcache`,
  `internal/lock`) and because closing skills shrinks the remaining bash before the largest module.
- S3 — No observable behaviour changes without evidence: RED→GREEN per rule, the differential against
  the legacy route (byte-level stdout/stderr/exit code on twin projects), and `tests/validate.sh` or the
  parity corpus where the route is reachable from a fixture.
- S4 — The three verbs keep the legacy argument contract exactly: flags, the two usage errors with
  exit 2, unknown flags, the positionals each accepts, and the help text of each script.
- S5 — The manifest is the only source of truth and the write is atomic: removing a `[[deps]]` block or
  adding one replaces the file through a temp file and a rename, exactly as `lock.py` does
  (`mkstemp` + `os.replace`), so a failure never leaves a truncated manifest.
- S6 — `[[deps]]` grammar is the shell's: the block this CLI writes is the block this CLI reads, and a
  hand-written block with the same keys behaves the same. Every other byte of the manifest survives.
- S7 — The cache is read through its ONE authority: bundled skills resolve from
  `internal/projectcache`'s `CacheRoot`, never from a second spelling of the path.
- S8 — `add-dep` still finishes by delegating to `sync` — the same seam the shell uses — and its exit
  code is the sync's exit code. It is the one step of this unit that stays a call into another verb.

## Slicing

| slice | scope | retires |
| --- | --- | --- |
| **U5a** | `skills list`, read-only: the three sections (registered `[[deps]]`, bundled from the cache, local under `ai-specs/skills/`) with their exact headers, counts and messages | `lib/skills-list.sh` (275) |
| **U5b** | `skills remove`: the `[[deps]]` block removal with the atomic write, the pruning of the vendored copy, and its messages | `lib/skills-remove.sh` (217) |
| **U5c** | `add-dep` / `skills add`: URL parsing and validation, the block it writes, the attribution default, and the `sync` delegation | `lib/skills-add.sh` (222), `lib/skills.sh` (39) |

The three share one manifest reader/writer; whichever slice lands first owns it.

## Tasks

- T1 (U5a, S4/S7) — `skills list` in Go: argument contract and help, the `=== ai-specs skills list ===`
  header, the three sections with their exact vocabulary, the "not initialized" branch, and the cache
  read through `internal/projectcache`.
- T2 (U5a, S3) — its differential: the same project read through both routes, byte-identical stdout,
  stderr and exit code.
- T3 (U5b, S4/S5) — `skills remove`: contract, the atomic manifest rewrite, the prune, the messages.
- T4 (U5b, S3) — differential including the case where the dep is absent, the manifest is read-only, and
  the vendored copy is missing while the block exists.
- T5 (U5c, S4/S6) — `add-dep`: URL forms accepted, the block written, attribution default, and the
  `sync` child's exit code propagated.
- T6 (U5c, S3) — differential with a local file:// or a stubbed fetcher, so the suite stays
  network-hermetic.
- T7 (all, S1) — the three bash scripts become thin wrappers that exec the binary, and their Python
  helpers (`project-cache.py`, `lock.py` usage) lose this caller.

## Log

- L1 — Human, verbatim (2026-10-18): "Ok, tú dale, continuemos."
- L3 — Evidence (U5a), the differential and its environment: `skills list` runs the shell script and
  the binary on the SAME project and compares stdout, stderr and exit code over ten cases — two deps
  (one vendored, one not), no manifest, local skills with and without `SKILL.md`, the bundled fallback
  when the home ships no `bundled-skills`, `--help`, an unknown flag, a second positional, a positional
  after `--`, and the cwd default — all byte-identical. Two deviations are recorded rather than hidden:
  a `cd` failure keeps exit 1 and the `cd: <path>: <reason>` shape but drops the script/line prefix
  (the shell's own noise, the S12 class), and a manifest whose `[[deps]]` is not an array prints the
  frozen `  ✗ manifest invalid: ` prefix with the port's parser text. The differential needs a Python
  with `tomllib` (the legacy oracle); with the machine's 3.9 it skips LOUDLY naming the probe and the
  fix, and every repo python-oracle package fails for the same pre-existing reason.
- L2 — Evidence (2026-10-18), mapping the legacy surface: `lib/skills.sh` 39 (dispatcher: `add` →
  `skills-add.sh` with `AI_SPECS_INVOKED_AS="ai-specs skills add"`, `list` → `skills-list.sh`,
  `remove` → `skills-remove.sh`), `skills-list.sh` 275, `skills-remove.sh` 217, `skills-add.sh` 222,
  and the two Python modules they drag in (`project-cache.py` 679, `lock.py` 490) — both of which
  ALREADY have native cores in `internal/projectcache` and `internal/lock`. The slicing above follows
  from that: the verbs are wiring around cores that exist, not new algorithms.
