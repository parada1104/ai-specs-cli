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

## U5b — the read-only divergence, decided

`skills remove` on a manifest with mode 0444 is the one behavioural difference from the shell, and it is
deliberate. Measured on the same project:

| route | 0444 manifest | result |
| --- | --- | --- |
| `lib/skills-remove.sh` | `ai-specs.toml` | **removes the `[[deps]]` block, prunes `.deps/<id>/`, leaves mode 444** |
| native | same file | **refuses** (`refusing to overwrite read-only manifest …: permission denied`), mutates nothing |

The shell succeeds because `mkstemp` + `os.replace` only needs a writable DIRECTORY — the user's mode
bits never enter the decision. That was found and fixed as a defect in U3b-2: `internal/config.atomicWrite`
now refuses a manifest nobody can write, pinned by `TestSetEnabledAgentsRefusesReadOnlyManifest`, and the
port inherits the same rule here because it goes through the same writer (S5).

Keeping the refusal, for three reasons: it is the safer side of the same behaviour, it fails closed with a
message that names the file and the mode, and weakening `atomicWrite` to match the shell would reopen the
defect that U3b-2's review round closed. `TestSkillsRemoveReadOnlyDeviation` pins BOTH observed sides, so
the difference is a recorded decision rather than a silent one.

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
- L4 — Evidence (U5b), the differential and its environment: `skills remove` runs
  `bash lib/skills-remove.sh` and the binary on byte-identical TWIN projects (the verb mutates, so one
  shared project cannot serve both legs) under one shared env — isolated `AI_SPECS_HOME`, `LC_ALL=C`,
  `PYTHONDONTWRITEBYTECODE=1`, and a LOUD tomllib probe that skips naming the fix when the machine's
  python3 is too old (the suite runs with `PATH=/tmp/pybin:$PATH`) — comparing stdout, stderr and exit
  code byte for byte after normalizing each leg's own project root out of the messages, PLUS the
  resulting trees (manifest bytes, the pruned `.deps` copy, the lock file). Twelve cases all
  byte-identical: a vendored dep (block removed, `ai-specs/.deps/<id>/` pruned with the sibling left,
  lock hashes dropped and the rewritten lock byte-identical across legs), a dep that exists but was
  never vendored (no prune and no lock line, lock bytes untouched), an absent dep, a missing id,
  `--help`, an unknown flag, a third positional, a manifest whose only table is `[[deps]]` (the file
  empties and stays valid TOML), a missing manifest (`ERROR: <path> not found.` exit 1), a traversal id
  (the C2 guard fires before any write, exit 2), a positional after `--`, and the cwd default. The
  lock leg rides the shell's `lock.py` → `worktree-gate --write-lock` bridge pinned through
  `WORKTREE_GATE_BIN` to a freshly built gate; unpinned it fails open with a
  `GO_LOCK_WRITE_BRIDGE_FALLBACK` stderr warning the native leg never prints, which would measure the
  bridge instead of the verb. `RemoveDepSegment` matched the script exactly on both message lines
  (`  ✓ removed [[deps]] …` to stdout, `  ✗ dep … not found` to stderr exit 1) and on the valid→invalid
  guard; three behaviours are RECORDED rather than hidden: (1) READ-ONLY MANIFEST — the script's
  `mkstemp`+`os.replace` bypasses the file mode (rename needs write on the DIRECTORY), so the legacy
  leg completes block removal, prune AND lock while keeping the 0444 mode, whereas the native leg
  refuses inside the shared `config.atomicWrite` with the pinned
  `R4-atomicwrite-readonly-refusal` rule (odd/tasks/go-20-u3b-advisories.md), exit 1, nothing mutated —
  the script is the contract and the refusal lives outside this unit's edit surface, so
  `TestSkillsRemoveReadOnlyDeviation` pins both observed sides instead of a byte match; (2) a failing
  `cd` keeps the U5a S12-class deviation: exit 1 and `cd: <path>: <reason>` without bash's
  `lib/skills-remove.sh: line N:` prefix; (3) a corrupt lock file exits 1 like the script's traceback
  but words the Go load error (ADR 0002 diagnostic class), and an rm/write I/O failure words its own
  message. The script NEVER touches the cache — the home is snapshotted around the whole table to
  prove the native verb writes nothing into it — and the routing proof (`skills remove` with PATH
  emptied) pins that the verb no longer execs the shim, while bare `skills`, `skills add` and
  `skills --help` still resolve to `lib/skills.sh` byte for byte. Spec note: the U5b slicing row does
  not mention the D17 lock-hash step the script performs after the prune; it is ported here because
  the script is the contract (T7's wrapper retirement will lose this Python caller).
