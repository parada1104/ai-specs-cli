# CLI Parity Contract — Go Migration Baseline

> **Status**: authoritative inventory for the Go single-binary migration epic.
> **Baseline**: `development` @ 592dbf9, measured 2026-08-18.
> **Post-migration delta**: §7, the CLOSED rows of §9, the settled records in
> §10 and §12 were rewritten to the state after `lib/` was deleted
> (`ae9a4c99`, 2026-11-13, GO-20 U2e T6). §1, §3's newline-collapse paragraph,
> §6's `doctor` row, §8's pinning paragraph and §11's Python-era rows still
> describe the pre-port tree; they are inventory history and are queued for
> their own pass, not silently fixed here.
> **Card**: [Go 01] Spike — parity contract + CLI surface matrix.
> **Consumers**: [Go 02] black-box test conversion, [Go 03] differential parity harness.

Every later port in this epic is verified against this document. It records what
the CLI observably does today, not what it should do. Where current behavior is
wrong, it is recorded under **Defects** and filed as a separate card — never
silently normalized during the port.

## Classification vocabulary

| Class | Meaning |
|---|---|
| **FROZEN** | Must be reproduced exactly. A delta is a parity failure. |
| **TOLERANT** | Semantically equal is enough; formatting may drift. Changing it churns golden tests, so change deliberately. |
| **FREE** | May change. Usually an artifact of the Python/Bash implementation with no Go analogue. |

## Go/no-go

**GO.** No blocker was found. Three findings materially change the plan and are
recorded in full below:

1. A whole-document Go TOML library is **not viable** for manifest writes.
2. The pipeline passed data between modules by grepping human-readable stdout.
   (Resolved for the Go route by GO-07 S16: it consumes return values — see §5.)
3. Three commands documented as read-only are not.

---

## 1. Command surface

`bin/ai-specs` dispatches 14 verbs. Bare invocation (no arguments) is rewritten
to `hub`, **not** to `help` — a deliberate product decision documented in
`README.md`.

| Verb | Aliases | Entry | Writes? | TTY branch? |
|---|---|---|---|---|
| `init` | — | `lib/init.sh` | yes | **yes** |
| `sync` | — | `lib/sync.sh` | yes | no |
| `sync-agent` | — | `lib/sync-agent.sh` | yes | no |
| `refresh-bundled` | — | `lib/refresh-bundled.sh` | **yes** (claims no) | no |
| `add-dep` | = `skills add` | `lib/skills-add.sh` | yes | no |
| `skills` | `add`/`list`/`remove` | `lib/skills.sh` | add/remove: yes | no |
| `doctor` | — | `lib/doctor.sh` | **yes** (claims no) | no |
| `rules-audit` | — | `lib/rules-audit.sh` | no | no |
| `recipe` | `list`/`add`/`init`/`remove`/`configure` | `lib/recipe.sh` | varies | add: **yes** |
| `configure-recipes` | — | `lib/recipe-config.sh` | yes | **yes, required** |
| `upgrade` | — | `lib/upgrade.sh` | yes | no |
| `version` | `-v`, `--version` | `lib/version.sh` | no | no |
| `help` | `-h`, `--help` | inline heredoc | no | no |
| `hub` | *(bare `ai-specs`)* | `lib/hub.sh` | **yes** (claims no) | **yes** |

`recipe configure` and `hub` are **absent from `ai-specs help`** despite being
real, reachable commands — `hub` is even the default.

---

## 2. Exit-code contracts — FROZEN

These are the primary machine-readable surface. Every one must be reproduced.

### `doctor`

```
exit 1  ⟺  count(ERROR) ≥ 1
exit 0  otherwise
```

WARN and INFO **never** affect the exit code, in any quantity. `0 OK, 12 WARN,
0 ERROR` exits 0, identically to an all-clean run. There is no code that
distinguishes "clean" from "warnings only".

Additionally: `2` for unknown flag / extra positional; `1` for a nonexistent
path (raw bash `cd:` error — `doctor.sh` lacks the `-d` guard that
`rules-audit.sh` has, so the Python guard is dead code).

#### Malformed-manifest guidance — accepted divergence (U2c)

For an unparseable `ai-specs/ai-specs.toml`, the `manifest` ERROR check keeps
its name, severity and message (`<rel> is not parseable`). Its guidance is the
native parser's diagnostic (`toml: line N: …`), not tomllib's text. Python
versions word tomllib differently, so the text is not pinned. The divergence is
accepted by the human (plan S4, 2026-10-22: the native message is accepted, not
tomllib's text). Doctor spawns no `python3` for it: the Python bridge is retired,
and the `doctor-bridge` check it fed is gone. Every other check, severity and
exit code stays FROZEN.

### `upgrade` — the richest map in the CLI

| Code | Meaning |
|---|---|
| 0 | Upgraded, already current, dry-run, or help |
| 1 | Broken/missing installation — **and** unknown argument (collision) |
| 2 | Not the standard global install (dev-channel guard) |
| 3 | Divergence, dirty tree, or `origin/main` absent locally |
| 4 | Fetch failed, or fast-forward merge failed |
| 5 | Post-upgrade symlink verification failed |

### `recipe configure`

| Code | Meaning |
|---|---|
| 0 | ok / no-op / dry-run |
| 1 | write failed, sync failed (`partial`), or doctor failed |
| 2 | argparse error |
| 3 | `ConfigureError` — validation, unknown key, secret-shaped literal |
| **4** | **blocked by `[tool]` CLI version policy** |

### `hub` / bare `ai-specs`

| Code | Meaning |
|---|---|
| 0 | Status printed, or interactive quit |
| 2 | Uninitialized **and** non-TTY (bash pre-guard, never launches Python) |
| 3 | `rich`/`questionary` unavailable and not installed |

### Cross-cutting

- `2` — unknown flag or unexpected positional, in every shell wrapper.
- `sync` collapses any per-target failure to `1`, and collapses
  `cli_version.py check-sync`'s `2` into `1`.
- The materializer step runs in-process (`sync` and `sync-agent`, GO-07 S14c), so its status
  is `0` or `1`: a failure collapses to `1` with the fatal `  ✗ <msg>` line on stderr and, at
  `sync`, the captured partial output replayed raw on both streams.

---

## 3. Manifest writes — FROZEN, and the decisive ADR input

**A Go TOML library that marshals the whole document is not viable.** Three
independent write paths exist, and all three operate on text, not on a parsed
document:

| Path | Mechanism | Why a marshaller breaks it |
|---|---|---|
| `recipe add` (`recipe-add.py`) | Literal append at EOF | Emits `= ""  # REQUIRED` and `# <key> = ""  # optional` placeholder comments that carry meaning |
| `recipe configure` / `configure-recipes` (`recipe-config-write.py`) | Line surgery | Preserves original indentation, inline comments, **and the exact run of spaces before `#`** |
| `recipe remove` (inline heredoc) | Segment delete by text | Must tolerate a manifest that is **not currently valid TOML** |
| `skills add` (inline heredoc) | Literal append at EOF | Hand-rolled serializer; escapes only `\` and `"` |
| `skills remove` (inline heredoc) | Segment delete by text | Same as `recipe remove` |

Any library that round-trips the document reorders tables, drops comments, and
re-quotes existing values. Every one of those is a parity failure against a file
committed in users' repositories.

**Required Go design**: a line/segment editor, plus a parse-validate-and-restore
step (`recipe add` and `recipe-config-write` both validate after writing and
restore the original bytes on failure).

### Segment-splitting rule — FROZEN

Both removal paths split the file on lines whose **first character at column 0
is `[`**. Consequences that must be reproduced:

- An indented `  [other.table]` is invisible to the segmenter and gets deleted
  along with the preceding block.
- Array *value* lines (`scope = ["root"]`) never start with `[` at column 0, so
  they correctly stay attached to their block.
- `[[recipes.<id>]]` (array-of-tables) does **not** match the recipe-removal
  regex, because `\s*` cannot consume the second `[`.
- Trailing inline comments after a header still match.

### The whole-file newline collapse — FROZEN as observed

Both removal paths run `re.sub(r"\n{3,}", "\n\n", content)` over the **entire
file**, not just the seam left by the deletion. This rewrites regions the user
never touched — including inside multi-line basic strings, where it silently
alters string content.

Almost certainly unintended. Recorded as FROZEN because any diff-based parity
test will trip on it, and as a defect below.

---

## 4. Filesystem contracts — FROZEN

### Cache key derivation

```
$AI_SPECS_HOME/cache/projects/<sha256(realpath)[:12]>-<sanitized-basename>/
```

If the Go port derives this differently, **every existing project loses its
cache**. Non-negotiable.

### Ownership and preservation rules

| Artifact | Rule |
|---|---|
| `ai-specs/ai-specs.toml` | **Never overwritten by `init`**, not even with `--force` |
| `AGENTS.md` | Written only in states `missing` / `managed_stale`; `marker`, `untracked`, `user_modified`, `undetermined` preserved |
| Templates (`condition = not_exists`) | Refreshed only when lock state is `managed_stale` **and** policy is `auto` |
| Gate hooks | Preserved when `user_modified` or lacking provenance, unless `--refresh-gates` |
| Recipe **docs** | `copy2`, **unconditional overwrite** — inconsistent with templates/gates (defect) |
| Bundled skills/commands in project | Deleted only when byte-identical to source or to a legacy lock hash |
| `.gitignore` (root) | Only the block between `# --- ai-specs: … ---` and `# --- end ai-specs ---` is managed |
| `.envrc` | Only the block between `# managed-by: ai-specs (do not remove block)` and `# end managed-by: ai-specs` |
| `<target>/.claude/commands/` etc. | **`rm -rf` every run** — user-added command files are destroyed |
| git index | **Never touched.** Remediation is printed as copy-pasteable `git rm --cached` text |

### Symlink kinds — FROZEN

- Instructions (`CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`,
  `.omp/AGENTS.md`) → **relative** symlink to `AGENTS.md`.
- Skills dirs (`.claude/skills`, `.cursor/skills`, …) → **absolute** symlink.
- A real file where a symlink belongs is a **hard error**, never overwritten:
  `    ✗ refuse to overwrite non-symlink: <path>`.

---

## 5. Inter-module coupling — the biggest port hazard

`sync.sh` extracts structured data from another module's **human-readable
stdout**:

```sh
RECIPE_NAMES=$(grep -oE '▸ recipe [^ ]+' …)          # from the materializer step
RECIPE_MCP_JSON=$(grep '^RECIPE_MCP_TEMP:' … | cut -d: -f2-)
```

The print format of one module is the API of another. In Go this collapses into
a return value, but **while both implementations coexist**, any cosmetic change
to materialize's output silently breaks sync.

**GO-07 S16 state.** The Go route no longer parses its own output: `sync` takes
the names from `MaterializeResult.PrintedRecipeIDs` and `sync-agent` takes the
recipe-MCP temp path from `MaterializeResult.RecipeMCPPath`. Those two fields
carry exactly what the greps would have found — the names are the recipes that
reached their `▸ recipe <id>` line, so a run that fails before the loop prints
names nothing (Python greps the partial stdout for the same set). The printed
lines themselves stay byte-identical and FROZEN (see §12) because the bash route
above still greps them; the freeze ends with that route, not with this collapse.

### The compact filter is load-bearing

`print_step_output` drops blank lines and every line whose first non-whitespace
character is one of:

```
✓   ·   ⇢   ▸
```

These four glyphs decide what a user sees during a normal `sync`. They look
decorative; they are the display contract. **FROZEN.**

---

## 6. Read-only claims that do not hold

Three commands are documented as read-only and are not.

| Command | Claim | Reality |
|---|---|---|
| `doctor` | "read-only" in `ai-specs help` | No project writes, but writes `__pycache__` into `$AI_SPECS_HOME/lib/_internal/`, and **executes** `git`, the gate binary's `--selftest`, and every recipe dep's `version_check` with `shell=True` |
| `refresh-bundled` | Standalone verb: none (compatibility no-op, D26). Refresh runs inside `sync`. | Standalone verb: no effect (Go). Inside `sync`: flattens bundled skills and commands into the cache, deletes leftover in-project copies under `ai-specs/skills/` and `ai-specs/commands/`, rewrites `ai-specs/.ai-specs.lock` |
| `hub` | implied by "status" | Rewrites `ai-specs.toml`, may run `pip install`, delegates to `init`/`sync`/`upgrade` |

`doctor`'s `version_check` execution matters most for the port: it is arbitrary
shell execution with a 5-second timeout, not a passive probe. [Go 08] must
reproduce it deliberately.

`rules-audit` and `version` are genuinely read-only.

---

## 7. TTY branching

Four surfaces branch on TTY. Everything else is byte-identical piped or
interactive. Since `lib/` was deleted (GO-20 U2e T5 PR4b, `ae9a4c99`) every rule
below is Go code; the legacy branch survives only as frozen bytes
(`tests/parity/goldens/`, `internal/cli/testdata/smoke_legacy.golden.json`).

| Surface | Rule |
|---|---|
| `init` | Wizard only when the terminal test passes **and** no `--name` **and** no `--force` **and** `ai-specs.toml` absent (`shouldInitTUI`, `internal/cli/init.go:303-318`). `--tui` in CI degrades to classic init (rc 3), it does not fail. FROZEN |
| `hub` | One matrix on `(initialized, tty)` (`internal/hub/hub.go:198-211`): uninitialized + non-TTY prints the uninitialized error and exits 2; initialized + non-TTY prints the status block and returns the **doctor's** exit code; initialized + TTY runs the menu; uninitialized + TTY runs the init offer. FROZEN |
| `configure-recipes` | **Requires** a TTY; exit 3 otherwise (`internal/cli/recipe.go:2890-2897`). The exit code is FROZEN; the message is the registered divergence recorded below |
| `recipe add` | The TTY branch is the config TAIL only: `ttyNow && (has_config || has_mcp_env)` opens the wizard, the `Configure now?` confirm and the dependency gate (`internal/cli/recipe.go:437-470`). Off the TTY, or with nothing to configure, the frozen `Next steps:` block prints. The manifest write has ALREADY happened at that point (`config.AppendRecipeSegments`, `internal/cli/recipe.go:383-391`), so a declined confirm leaves the recipe registered and prints `You can configure later with: ai-specs configure-recipes` |

`hub`'s anti-hang guarantee is now structural rather than a pre-guard:
uninitialized + non-TTY exits 2 and there is no interpreter left anywhere to
launch (`internal/hub/hub.go:198-203`). FROZEN.

### `configure-recipes` without a TTY — settled divergence (exit 3 both, message conditional)

Both implementations exit 3, and §2 has no exit-code row that distinguishes the
case; the STDERR text is not FROZEN because the two reach the gate by different
paths:

- Legacy (`lib/_internal/config_wizard.py:282-287` at `ae9a4c99^`): `ensure_deps`
  runs FIRST and returns 3 **silently** when `rich`/`questionary` cannot be
  imported from the install's vendor dir and the streams are not terminals
  (`lib/_internal/util.py:57-76`); only when those imports succeed does the TTY
  gate print `configure-recipes requires an interactive TTY`. The legacy stderr
  is therefore EMPTY in a home without vendored UI packages and carries the
  message in a home that has them.
- Go (`internal/cli/recipe.go:2890-2897`): there is no vendor bootstrap (the
  wizard is native), so the TTY gate runs first and the message always prints.

Both shapes are on record. The empty-stderr shape is what the parity harness
measured in its isolated home and registered as an accepted divergence with no
product change (GO-20 U2e T4, `b71069d`, 2026-10-24: "exit codes match, the text
is diagnostic"; `tests/parity/parity.py:1851-1863`). The message shape is the
frozen pre-flip launcher record `configure-recipes-nontty` in
`internal/cli/testdata/smoke_legacy.golden.json` (rc 3, empty stdout, that exact
stderr). Nothing else about the surface is normalized: the exit code, the
frozen `Project not initialized: missing <path>` refusal that precedes it, and
the whole `--help` surface stay FROZEN and are compared by the corpus
(`configure-recipes-surface`).

---

## 8. Network and external process surface

| Command | External call | Notes |
|---|---|---|
| `skills add` | `git clone --depth 1 --quiet <source> <tempdir>` | The **entire** network contract. Shallow, **not** sparse. No `--branch`, no `--filter`, no post-clone checkout |
| `upgrade` | `git fetch origin main`, `git merge --ff-only origin/main` | Only network calls; `--ff-only` is the "never break the install" guarantee |
| `upgrade` | `pip install --target lib/_vendor rich questionary` | Second network call; **never fatal** |
| `doctor` | each dep's `version_check` via `shell=True`, timeout 5 | Arbitrary shell |
| `doctor` | gate binary `--version` / `--selftest` | Timeouts 10 / 30 |
| `configure-recipes` | `direnv allow <root>`, timeout 10 | Mutates the user's direnv trust store |

**`skills add` has no revision pinning.** `[[deps]]` has no `ref`/`rev` key and
`--depth 1` always takes the remote default-branch HEAD. Two syncs of the same
manifest can vendor different content with no record. Recorded as a defect.

---

## 9. Defects found — file as separate cards

The card's scope requires these be filed, not normalized during the port.
Rows whose owner landed are kept for history and marked **CLOSED**, with the
commit and the behaviour that replaced them; a CLOSED row is no longer a
licence to reproduce the defect.

### Data loss

| # | Defect |
|---|---|
| D1 | **`recipe remove` destroyed lock data — CLOSED** (`ea61d132` PR #301, 2026-09-27, card `lVB8gEin`). The removal path now goes through the lock writer (`load_lock` → `remove_recipe_lock_entries` → `write_lock`), which preserves `[managed.*]`, `[deps]` and `[agents.*]`, and the lock is rewritten only when something was actually removed. The Go route is the same shape (`internal/lock/mutation.go:81`, `internal/cli/recipe.go:565,697`). NOT the same register as the S14-Δ `D1` (empty config key), which is a separate record in §10. **D2, D13 and D14 came from the same card: D13/D14 are the rows below, and D2's row still describes the pre-fix heredoc** (it is queued with §3 for its own pass) |
| D2 | **Whole-file `\n{3,}` collapse** in `recipe remove` and `skills remove` rewrites untouched regions, including inside multi-line strings |
| D3 | **`<target>/.claude/commands/` is `rm -rf`'d every sync**, destroying user-added command files |
| D4 | **Recipe docs are clobbered unconditionally** (`copy2`), unlike lock-tracked templates and gates |

### Correctness

| # | Defect |
|---|---|
| D5 | **`hub` discards the exit code.** `_run_noninteractive` computes Doctor's exit code and always returns 0. `ai-specs hub \| cat` on a broken project prints `3 error(s)` and exits 0, while `doctor` exits 1 |
| D6 | **`init` never stamps `[meta]`**, so a fresh project reports "last sync unknown" to `doctor` |
| D7 | **`skills list` status is always wrong.** It checks `ai-specs/skills/<id>`, but deps vendor to `ai-specs/.deps/<id>/skills/<id>/`. A correctly synced dep reports `✗ not synced` |
| D8 | **`skills list` bundled section is always `(none)`.** It scans the project, but bundled skills live in the cache |
| D9 | **`upgrade --dry-run` could write — CORRECTED in the Go route** (`a68440af`, card [Go 12], 2026-09-29). The mode-only-dirt remediation returns before `git checkout -- .` when `--dry-run`, so a preview writes nothing (`internal/upgrade/upgrade.go:276-281`, pinned by `TestModeOnlyDirtDryRunThenReal`, `internal/upgrade/upgrade_test.go:347-381`). The legacy script that wrote is deleted. D9 is a defect and not FROZEN, so the correction is allowed; recorded as a settled divergence in §10 |
| D10 | **`upgrade --dry-run` never fetches**, so "Target version" can be arbitrarily stale and it reports "Already up to date" when it is not |
| D11 | **`git fetch origin main` is refspec-less.** A clone without a configured `remote.origin.fetch` never advances `origin/main` and reports "Already up to date" forever |
| D12 | **The agents submenu can append a second `[agents]` block** at EOF when `[agents]` exists without `enabled` and is the last table |

### Robustness

| # | Defect |
|---|---|
| D13 | **Non-atomic writes — CLOSED** (`ea61d132` PR #301, 2026-09-27). Every manifest write in the Go route is `config.atomicWrite`: temp file in the target's directory, original mode preserved, then rename (`internal/config/writes.go:133-183`), called from `writes.go:238,286,333,386,392`, `agents.go:115` and `recipeconfig.go:1358,1445`. One deliberate divergence remains, recorded in §10: a manifest with no write bit is REFUSED instead of replaced |
| D14 | **No TOML validation after the removal paths — CLOSED** for both removals (`ea61d132` PR #301, 2026-09-27): a deletion that would turn a VALID manifest into an invalid one is refused with the original bytes untouched (`guardValidToInvalid`, `internal/config/writes.go:101-114`, called at `:282` and `:329`; an already-invalid manifest stays editable, as §3 requires). The one surviving hole is by design: `skills add` still appends without validating, mirroring the original (`writes.go:185-189`) |
| D15 | **`git clone` inherits the terminal**, so a private repo hangs on a credential prompt inside a step whose output sync captures to a temp file — an invisible hang |
| D16 | **No revision pinning for deps** (see §8) |
| D17 | **Dep content hashes are never recorded.** `vendor-skills.py` binds five `lock.py` functions and calls none — no drift detection for dep skills |
| D18 | **`skills remove` never prunes `.deps/<id>/`**, and its `--help` names the wrong path. Orphans accumulate |
| D19 | **`skills add` id validator accepts `_`** but `skill_contract.NAME_RE` does not, so `my_skill` is accepted then fails during vendoring |
| D20 | **`set -u` makes six error messages unreachable** in `skills-add.sh`: a trailing `--id` dies with `$2: unbound variable` and exit 1 |
| D21 | **Malformed manifest → exit 2 with zero output** in `skills add`; the diagnostic branch is dead code |
| D22 | **`sync-agent` standalone renders no hooks** (`--resolved-hooks` is only passed by `sync.sh`), leaving stale entries in `.claude/settings.json` |
| D23 | **Temp file leak** in sync-agent's local-materialize path |
| D24 | **`WARNING: no agents to sync` exits 0** after flatten/merge already ran and without `ensure_target_workspace`, so a subrepo gets no `AGENTS.md` but reports success |

### Cosmetic / documentation

| # | Defect |
|---|---|
| D25 | `hub` and `recipe configure` are absent from `ai-specs help` |
| D26 | **`refresh-bundled` claimed read-only and was not — SETTLED** (human decision, U2b, 2026-10-22: "ah si si, basta con sync"). The standalone verb is a documented no-op of compatibility that prints its embedded provenance and changes nothing (`internal/cli/refresh_bundled.go:29-81`); the effective refresh runs inside `sync` (`internal/sync`, `RefreshBundled`). The help text that still described the flatten was corrected in its own product PR (`6340c27b` PR #446, 2026-10-24) and now reads `refresh-bundled [path] Compatibility no-op: bundled assets are embedded in the CLI and sync refreshes them; this verb changes nothing` (`internal/cli/help.txt:8-9`). `hub` also writes; `doctor` no longer writes `__pycache__` (there is no Python) but still EXECUTES the gate binary's `--selftest` and every dep `version_check` through `sh -c` (§6). Recorded as a settled divergence in §10 |
| D27 | `init --force` is documented as regenerating `AGENTS.md`; it only affects `.gitignore` |
| D28 | Literal `\n` in `upgrade`'s dirty-tree abort (builtin `echo` without `-e`) |
| D29 | `--` silently discards the path argument in every recipe and skills wrapper |
| D30 | `upgrade` unknown-argument exits 1, colliding with "broken installation" |
| D31 | Mixed Spanish/English error surface (`Proyecto no inicializado` next to `Recipe has no init workflow`) |
| D32 | Three different behaviors for a missing `VERSION` file: `version.sh` exits 1, `hub` and `cli_version` return `unknown`, `gate_binary` returns `dev` |
| D33 | `skills list` uses `grep -E '^id = "'` instead of parsing TOML, over-excluding local skills whose name collides with any top-level `id` |
| D34 | `add-dep --help` and every error hint name the command `ai-specs skills add`, never the alias the user typed |
| D35 | `recipe configure` positional order is `<recipe_id> [path]`; every sibling uses `[path]` last; `configure-recipes` uses `[path]` first |

---

## 10. Ordering guarantees — FROZEN

A rewrite most easily breaks these.

1. **Target resolution before any write.** `sync`'s entire failure contract is
   "fail before writes".
2. **`refresh-bundled` before `vendor-skills` and `recipe-materialize`**, so the
   `.bundled/*` cache tier exists.
3. **Leftover removal before `write_lock`**, because `write_lock` drops the
   legacy `[skills]`/`[commands]` hash sections used as the migration signal.
4. **`recipe-materialize` before `agents-render`** (produces `--resolved-config`)
   **and before the fan-out loop** (produces `--recipe-mcp` / `--resolved-hooks`).
5. **`mkdir -p ai-specs/` before the TOML write** in `init`.
6. **`.gitignore` trailing-byte normalization before appending**, or the marker
   line concatenates onto an unterminated last line.
7. **`stamp-meta` last**, so `[meta]` reflects a completed sync.
8. **Root is always target index 0.**
9. **`errexit` stays off inside `run_step`** until captured output is replayed —
   restoring it earlier lets a failing `cat` replace the wrapped status.

### Locale-collation exception — TOLERANT (narrow, human-accepted 2026-10-04)

The ORDER of the `! preserved non-managed file <dir>/<base> (move it to
ai-specs/commands/ to manage it)` warning lines — and only their order — is
**TOLERANT** when a non-C locale collates differently from byte order. The
lines come from the `for extra in "$dest"/*` glob traversal in
`sync_one_agent`'s commands fan-out; Bash iterates in `strcoll` order while
the Go port sorts bytes (Go's stdlib has no strcoll).

Scope of the tolerance — everything else stays **FROZEN**:

- The message text, the per-file set, and the NUMBER of warning lines.
- The exit code, the banner/footer framing, and the resulting tree (the
  preserved files and the D3' in-place managed overwrite are file
  operations, not ordering).
- The managed-copy glob behavior (`for src in "$COMMANDS_SOURCE"/*.md` is
  count-only in output; its order is not user-visible).
- Nothing else about error or content formatting is normalized by this
  entry; it covers no other glob, stream, or surface.

Under `LC_ALL=C` (which the parity harness pins in `tests/parity/parity.py`
BASE_ENV) strcoll and byte order coincide, so the corpus remains zero-delta:
both implementations emit `B, Z, _x, a`.

Evidence (macOS, BSD glob + strcoll, measured 2026-10-04, reproduced twice):
temp project with `[agents] enabled = ['cursor']`, seeded `AGENTS.md`, and
unmanaged `.cursor/commands/{B,a,_x,Z}.md`; `LC_ALL=en_US.UTF-8
LANG=en_US.UTF-8 bash bin/ai-specs sync-agent . --all` prints the warnings as
`_x, a, B, Z` while the Go binary prints `B, Z, _x, a` (identical lines,
count, rc and tree). Decision: the human explicitly accepted the different
warning ordering under non-C locales as a narrow TOLERANT classification,
recorded here instead of being deferred to S16.

### Resolver-plan diagnostic framing — TOLERANT (narrow, human-accepted 2026-10-04)

When the standalone `sync-agent` plan extraction dies on a malformed or
root-less target-resolve plan (`python3 -c '…["root"]'`), the stderr
TRACEBACK FORMATTING is **TOLERANT**: interpreter paths, json module frames,
caret annotations and version-specific layout (they differ across
installations and interpreter versions — measured: 3.9.6 vs 3.13/3.14) may
be replaced by a stable portable Go diagnostic.

Since GO-07 S17 the plan is produced IN PROCESS by the native resolver
(`internal/target`, whose stdout bytes, stderr and exit code are differentially
pinned against `target-resolve.py`), so the Go route spawns
`target-resolve.py` at neither call site and no stub can inject a plan body.
This classification therefore governs (a) the shell route, which keeps its own
python call until the dispatcher cutover, and (b) the defensive extraction the
Go route keeps for a plan body it did not produce — that extraction still
distinguishes the two failure classes and prints the portable diagnostics, and
both are pinned directly by `TestStandalonePlanDiagnostics`.

FROZEN and unchanged:

- Exit code 1, raised BEFORE any banner, write or target work.
- The failure-class distinction: a malformed plan body (legacy
  `json.JSONDecodeError`) is reported differently from a plan without
  `"root"` (legacy `KeyError: 'root'`); the two must stay distinguishable.
- The deterministic diagnostic meaning: the reader learns WHICH class
  failed.
- No other error surface is normalized by this entry; the existing
  locale-collation exception above remains narrow and separate.

### Manifest fatal-error framing — TOLERANT (narrow, human-accepted 2026-10-05)

When the MCP_COUNT oracle dies on a MANIFEST that cannot be read or parsed
(`os.ReadFile` failure or a `tomllib`-equivalent parse error), the stderr
TRACEBACK FORMATTING is **TOLERANT**: interpreter paths, frames and
version-specific layout may be replaced by the stable portable `error: …`
diagnostic the port already emits (explicitly accepted by the human in place
of the Python traceback).

FROZEN and unchanged:

- Exit code 1, raised BEFORE the banner and before any write.
- The error MEANING: the diagnostic reports the manifest read/parse failure
  (the established `error: ` prefix is part of this pinned shape).
- The recipe-mcp fatal-error exception above (class distinction,
  IsADirectoryError/TypeError) remains separate and narrow, as do the
  locale-collation and resolver-plan exceptions.
- Nothing else is normalized; no logic change accompanies this entry.

### Recipe-mcp fatal-error framing — TOLERANT (narrow, human-accepted final bounded round 2026-10-04)

When the MCP_COUNT oracle (the inline python heredoc of `sync_one_agent`'s
caller) dies on a recipe-mcp JSON that cannot be read or has no len, the
stderr TRACEBACK FORMATTING is **TOLERANT**: interpreter paths, frames and
version-specific layout may be replaced by a stable portable Go diagnostic
that names the exception class.

FROZEN and unchanged:

- Exit code 1, raised BEFORE the banner and before any write.
- The failure-class distinction: a non-ENOENT read error (e.g. a directory
  path — legacy `IsADirectoryError`; permission — `PermissionError`) is
  reported differently from an unsized JSON body (number/bool/null — legacy
  `TypeError` for `len()`); the two classes must stay distinguishable and
  the diagnostic must name the correct class (no invented text).
- The len semantics of the non-fatal cases (missing file and malformed/empty
  JSON count as empty; dict/list/string lengths as measured).
- The existing locale-collation and resolver-plan exceptions remain narrow
  and separate; nothing else is normalized.

---

### Gate TOML seam parse detail — TOLERANT (narrow, human-accepted 2026-11-13)

The gate's recipe.toml readers (`shared.LoadRecipeTagMetadata`,
`shared.LoadRecipePrimitives`, which `internal/sync` also calls) and the retired
`--plan-resolved-config` seam used to hand each file to an ambient
`python3 -c 'import tomllib'` helper. GO-20 U2e T5 (#526, then PR4b) made them
parse in process. The **diagnostic TEXT of a parse failure** is TOLERANT:

- The Python seam reported a dead interpreter or a rejected manifest through
  `python3`'s own exception/exit status (`the manifest is not valid TOML (parser
  exit status 1)`); the Go reader reports the recipe and the failing TOML line
  (`recipe <id>: recipe.toml not found or not parseable`, `recipe <id>:
  [recipe]: missing or invalid required field 'name'`).
- Both forms name the FAILURE CLASS the caller acts on; neither carries file
  content, raw stderr or a traceback.

FROZEN and unchanged:

- Which class fails: the tag reader OMITS a recipe it cannot fully acquire, the
  primitive reader FAILS the whole acquisition (the Python authority raises
  `RecipeValidationError` for the same shapes).
- The shapes each reader swallows: a non-table `[provides]`, a non-list
  skills/commands/mcp value, an id-less `[recipe]` for the tag reader, and a
  timestamp value that has no JSON form.
- Every resolved decision and every exit code on the surfaces that survive
  (`internal/sync`'s tag-conflict warnings and primitive-conflict refusal).
- Nothing else is normalized; the entry is diagnostic framing only, and the
  locale-collation, resolver-plan, manifest-fatal and recipe-mcp exceptions
  above remain separate and narrow.

---

### Empty config key — TOLERANT (settled, human-decided 2026-10-13)

S14-Δ's `D1` (a different register from §9's `D1`): the gate core returned a
process error for a keyless field/table/manifest entry, while
`_python_merge_config` treated the empty string as an ordinary key. The human
decision was to fix all six S14-Δ divergences "with no exceptions", including
this one, which REVERSES the gate's strictness
(`odd/tasks/go-07-s14-divergences.md`, "Human decisions that bound this unit").

Settled by `08f1174` (PR #349, 2026-10-13): a keyless entry flows through the
same lookup as any other key
(`catalog/recipes/worktree-flow/gate/shared/mergeconfig.go:295-298`), so an empty
FIELD key keeps its default and an empty MANIFEST key takes the warning path
`recipe '<id>': unknown config key '' in manifest (ignored)` (`:346`).

TOLERANT covers one residual document shape, pinned rather than silent: an
INLINE `config = { ... }` table declared before a colliding flat key still
resolves differently — the authority is document-ordered and reports the later
flat key, the native two-pass reports the nested table
(`TestNativeResolvedConfigInlineConfigBeforeFlatKeyDivergence`,
`internal/sync/resolvedconfig_test.go:535-587`). No shipped recipe uses that
shape, and closing it would require document order to survive TOML parsing,
which the map-based `internal/toml` cannot represent. Nothing else is
normalized: the envelope shape, the warning text, every error class and every
exit code stay FROZEN and are pinned against the frozen records captured from
`_python_merge_config` (`internal/sync/testdata/*_golden.json`).

### `upgrade --dry-run` writes nothing — TOLERANT (settled, card [Go 12] 2026-09-29)

Recorded defect D9, not a FROZEN surface: the legacy `lib/upgrade.sh` ran the
mode-only-dirt `git checkout -- .` BEFORE its dry-run branch, so a preview could
write while printing "no changes will be made". The Go route deliberately
deviates (`internal/upgrade/upgrade.go:9-11`), and the deviation is the
behaviour now:

- `--dry-run` never remediates mode-only dirt and never writes
  (`upgrade.go:276-281`); pinned by `TestModeOnlyDirtDryRunThenReal`
  (`internal/upgrade/upgrade_test.go:347-381`, which fails if `Restoring file
  modes` appears on a preview or if the mode changed).
- Everything else in the surface is unchanged and FROZEN: the exit-code map in
  §2, the refusal precedence (not an install → 2, dirty tree → 3, fetch/merge →
  4, symlink verification → 5), and the preview's report text.

D10 (`--dry-run` never fetches, so "Target version" can be stale) is a separate
row and is untouched by this record.

### Non-atomic manifest writes and the read-only refusal — TOLERANT (settled, 2026-09-27)

Recorded defect D13, closed for every write path the CLI owns. Two facts, and
only the second is a divergence:

- Every manifest write is atomic: `config.atomicWrite` creates the temp file in
  the target's directory, copies the original mode (including setuid/setgid/sticky
  bits) and renames it over the target; a symlinked manifest is resolved first so
  the link is not replaced by a regular file (`internal/config/writes.go:133-183`).
  A failed validation restores the ORIGINAL bytes atomically (`:386-395`).
- TOLERANT is the read-only case ONLY. A manifest whose permission bits carry no
  write bit at all is REFUSED (`refusing to overwrite read-only manifest <path>`,
  `writes.go:156-158`) where the legacy `mkstemp` + `os.replace` completed the
  write. The refusal is deliberate (a temp+rename would silently bypass the mode
  the user set) and it is pinned on both write families
  (`TestSkillsRemoveReadOnlyDeviation`, `internal/cli/skills_test.go:862-934`;
  `TestSkillsAddReadOnlyDeviation`, `:1622-1686`;
  `TestSetEnabledAgentsRefusesReadOnlyManifest`,
  `internal/config/agents_safety_test.go:68-88`).

FROZEN and unchanged: the emitted manifest BYTES (CRLF/LF normalization, the
append text, the segment-splitting rule of §3), the guard messages, every exit
code, and the rule that the error path leaves the manifest untouched.

### No TOML validation on the `skills add` append — TOLERANT (settled, 2026-09-27)

Recorded defect D14. The two REMOVAL paths now validate before writing: a
deletion that would break a valid manifest is refused and the original bytes stay
in place (`guardValidToInvalid`, `internal/config/writes.go:101-114`; called at
`:282` and `:329`), while an already-invalid manifest remains editable (§3).
`recipe add` keeps its stronger unconditional validate-and-restore
(`:370-397`).

The one surviving non-validation is `skills add`/`add-dep`: `AppendDepsBlock`
appends without parsing the result, exactly as the original did, and the code
says so (`internal/config/writes.go:185-189`). TOLERANT covers only that append:
the append bytes, the placeholder texts and the escape rules stay FROZEN, and a
manifest that a subsequent read cannot parse is the user's to fix on both sides.

### Standalone `refresh-bundled` is a no-op — TOLERANT (settled, human-decided U2b 2026-10-22)

Recorded defect D26, settled by the human ("ah si si, basta con sync";
`odd/tasks/go-20-u2b-init.md` S2/L2). The standalone verb is a compatibility
no-op: it prints the embedded provenance and changes nothing
(`internal/cli/refresh_bundled.go:29-81`), while the refresh that matters runs
inside `sync` (`internal/sync`, `RefreshBundled`, ordered before vendoring and
materializing per §10).

Consequences that are NOT normalized:

- The two verbs are not comparable by design: the legacy standalone verb flattened
  the cache, restamped the lock, and exited 1 in an uninitialized project where Go
  exits 0 (`tests/parity/parity.py:1865-1876`). The corpus therefore records the
  gap instead of comparing it, and covers the effective refresh through the sync
  fixtures.
- The help text is part of the surface: it now states the no-op verbatim
  (`internal/cli/help.txt:8-9`, corrected by `6340c27b` PR #446, 2026-10-24) and is
  compared by the `surface-verbs` golden.
- `sync`'s own refresh step stays FROZEN byte for byte: its `▸ bundled skills +
  commands` step title, the `⇢ flattened N bundled skill(s)` / `command(s)` lines,
  the cache tier, the lock rewrite and the leftover removal
  (`internal/sync/sync.go:268-270`, `internal/sync/refreshbundled.go:73-76`).
  `init`'s step keeps its own header, `▸ refresh-bundled --init`
  (`internal/cli/init.go:662`), and the `--init` argument is accepted and ignored.

### Resolved symlink paths in output — TOLERANT (settled, L26, human-accepted)

The Go binary prints the RESOLVED spelling of a path where the Bash route printed
the spelling it was handed. The canonical case is `TMPDIR` under macOS's `/var` →
`/private/var` symlink: `doctor` prints `target: /private<TEMP>` while the legacy
leg printed `target: <TEMP>` (`internal/doctor/doctor.go:130` prints the resolved
root, `internal/projectcache/projectcache.go:65-74` is the `realpath` the root
went through). Measured in GO-20 U2e T2 (`odd/tasks/go-20-u2e-flip.md` L26/L28):
28 of 35 fixtures differ on the path form alone, and 0 differ when the scratch is
canonical.

Human-accepted as a divergence (card #191, T6: "resolved symlink path accepted as
divergence; parity runs with canonical TMPDIR"). The same class as the fixed
divergence D-U2e-1 (a symlinked `~/.ai-specs`) and as the open unit T9
(`binaryEntry` under a symlinked home).

Operational rule, and it is the reason the corpus stays zero-delta: **parity and
`validate` run with a CANONICAL `TMPDIR`** — `TMPDIR=/private/tmp/...` on macOS,
never `/var/folders/...` (`/private/tmp` is what all recorded runs use). Under a
canonical scratch the two spellings coincide and the corpus reports
`gate-absent failing=0`, `gate-present failing=0`. The tolerance covers the path
FORM only: the file/dir it names, the surrounding text, the exit codes and the
resulting tree stay FROZEN.

## 11. Environment variables

| Var | Effect |
|---|---|
| `AI_SPECS_HOME` | CLI install root. **Inconsistently honored**: `bin/ai-specs` exports it, but `hub.sh`, `doctor.sh`, `rules-audit.sh` and `version.sh` each re-derive it from `$BASH_SOURCE`. A user pointing it elsewhere gets the dispatcher from one tree and the Python from another |
| `AI_SPECS_SYNC_NESTED` | `1` suppresses `sync-agent`'s banner and footer |
| `AI_SPECS_GATE_BUILD` | `1` opts into local gate build |
| `AI_SPECS_GATE_OFFLINE` | `1` disables gate download |
| `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES` | `1` allows `test-*` recipes |
| `AI_SPECS_VENDOR_FIXTURE_ROOT` | Redirects `kepano/obsidian-skills` clones (test seam) |
| `AI_SPECS_INIT_TUI_PY` | Retired for the native `init` (U2b): ignored, the wizard runs natively. `lib/init.sh` still reads it until U2e deletes the shell path. |
| `TMPDIR` | Every `mktemp`; unwritable degrades a step to unfiltered output |
| `PYTHONDONTWRITEBYTECODE` | Incidentally suppresses the `__pycache__` writes in §6 |

---

## 12. Summary — the post-migration truth (frozen goldens vs live Go)

**State of the tree.** `lib/` is deleted (838 tracked files, GO-20 U2e T5 PR4b,
`ae9a4c99`, 2026-11-13) and no production Go path resolves or spawns an
interpreter: the surviving `exec` calls are `git`, the gate builder and its
`--version`/`--selftest` probes, `sh -c` for a dep `version_check`, and the
tracked binary (`internal/upgrade/upgrade.go:68`, `internal/target/topology.go:91`,
`internal/doctor/projectcache.go:140`, `internal/doctor/checks_tracker.go:255`,
`internal/gatebinary/gatebinary.go:563,596,609`, `internal/deps/deps.go:185`).
`bin/ai-specs` only resolves the native binary and execs it with the untouched
argv (`bin/ai-specs:1-33`); there is no Bash route and no Python fallback. The
TOLERANT records that describe legacy Python formatting (§2's malformed-manifest
guidance and §10's framing entries) stay as the record of WHY the Go output is
worded as it is; they normalize nothing that still runs.

**What the corpus compares now.** A FROZEN GOLDEN leg against a LIVE GO leg:

- 70 goldens in `tests/parity/goldens/<gate-mode>/<fixture>.json`, run in both
gate modes. 62 were captured from the legacy implementation at `6340c27`
(T2a) and are the only surviving legacy oracle. 8 (the four fixtures
`surface-verbs`, `all-recipes-enabled`, `nested-fanout-suppression` and
`skills-surface`, × 2 modes) were regenerated from Go output for approved
product changes, so for those four fixtures the frozen side is Go, not legacy
(`odd/tasks/go-20-u2e-flip.md` L32, L35, L36).
- `tests/parity/capture_goldens.py` cannot capture anything any more: it refuses
with exit 2 instead of freezing native output over the legacy corpus (S7b,
`118694e1`, PR #525). Regenerating the corpus is a human decision, never a
script run.
- Normalization registry (`tests/parity/parity.py:251-437`): N1-N6 are run
artifacts and the frozen help-heredoc quirk; N7/N8 were removed as dead (S7b);
N9 covers the `sync` clone-failure wrapper; **N10 (T7 cursor deny line) and N11
(`add-dep`/`skills add` clone failure) are ACTIVE pending a corpus recapture
decision** — measured in S7b, dropping them fails three fixtures in both gate
modes, because the frozen bytes still carry the legacy shapes; **N12-gate-cache-size
is PERMANENT**, never expires, because it normalizes the cached gate binary's
byte total, which moves with every legitimate gate change.
- The other oracles are the same shape: `internal/*/testdata/*_golden.json`
(frozen records captured once from the Python authority), `internal/cli/testdata/smoke_legacy.golden.json`
(17 pre-flip launcher records) and `tests/fixtures/init-{brief,native}/` (T3d,
T3e).

**Still FROZEN and measured:**

- Every exit code in §2.
- Manifest write formatting, via a line editor — never a marshaller (§3). The
write path is now atomic and validate-guarded, so §3's whole-file `\n{3,}`
collapse paragraph describes the frozen legacy bytes only; the Go removals do
not collapse (`internal/config/writes.go:91-99`) and §10's D13/D14 records say
what replaced it.
- Cache key derivation and every ownership rule in §4.
- The four compact-filter glyphs (§5). Their shell consumer is gone; they stay
frozen because they are the display contract of every `sync`.
- `▸ recipe <id>` and `RECIPE_MCP_TEMP:` as ordinary output lines. `internal/sync`
consumes them as return values and never parses its own output (GO-07 S16), and
the greps that needed the bytes died with `lib/*.sh`; the corpus still compares
the lines byte for byte in the goldens that contain them.
- Symlink kinds and the non-symlink refusal (§4).
- The TTY matrices in §7 and the recorded `configure-recipes` message
divergence. `hub`'s never-launch-an-interpreter guarantee is structural now:
uninitialized + non-TTY exits 2 and there is nothing left to launch.
- `git clone --depth 1` as the vendor transport (§8), now WITH revision pinning:
`[[deps]]` accepts `ref`/`rev` (`internal/sync/vendorskills.go:48-57`), a
sha-shaped ref is fetched and checked out, anything else becomes `--branch`
(`:253-268`). D16 is therefore closed in code; §8's "no revision pinning"
paragraph predates it.
- Every ordering guarantee in §10.

Everything classified TOLERANT may be reimplemented idiomatically; the settled
records in §2, §7 and §10 fix which deltas are accepted and which are not.
Everything FREE — `__pycache__`, `lib/_vendor` bootstrapping, sparse-checkout
narrowing, rich/questionary rendering — is gone with `lib/` and has no Go
analogue.
