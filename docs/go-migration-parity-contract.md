# CLI Parity Contract — Go Migration Baseline

> **Status**: authoritative inventory for the Go single-binary migration epic.
> **Baseline**: `development` @ 592dbf9, measured 2026-08-18.
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
2. The pipeline passes data between modules by grepping human-readable stdout.
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
- `recipe-materialize.py`'s own rc is passed through unchanged by `sync`.

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
RECIPE_NAMES=$(grep -oE '▸ recipe [^ ]+' …)          # from recipe-materialize.py
RECIPE_MCP_JSON=$(grep '^RECIPE_MCP_TEMP:' … | cut -d: -f2-)
```

The print format of one module is the API of another. In Go this collapses into
a return value, but **while both implementations coexist**, any cosmetic change
to materialize's output silently breaks sync.

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
| `refresh-bundled` | "zero in-project writes", "pure cache repair" | Deletes files under `ai-specs/skills/` and `ai-specs/commands/`, rewrites `ai-specs/.ai-specs.lock` |
| `hub` | implied by "status" | Rewrites `ai-specs.toml`, may run `pip install`, delegates to `init`/`sync`/`upgrade` |

`doctor`'s `version_check` execution matters most for the port: it is arbitrary
shell execution with a 5-second timeout, not a passive probe. [Go 08] must
reproduce it deliberately.

`rules-audit` and `version` are genuinely read-only.

---

## 7. TTY branching

Only four surfaces branch on TTY. Everything else is byte-identical piped or
interactive.

| Surface | Rule |
|---|---|
| `init` | Wizard only when `-t 0 && -t 1` **and** no `--name` **and** no `--force` **and** `ai-specs.toml` absent. `--tui` in CI degrades to classic init (rc 3), it does not fail |
| `hub` | Four-state matrix on `(initialized, tty)` → `INTERACTIVE_HUB` / `NONINTERACTIVE_STATUS` / `OFFER_INIT` / `ERROR_UNINITIALIZED` |
| `configure-recipes` | **Requires** a TTY; exit 3 otherwise |
| `recipe add` | Interactive only when `tty && (has_config || has_mcp_env)`; the dependency gate runs **before** the manifest write, so a declined install aborts with no change |

`hub`'s bash pre-guard is an explicit anti-hang guarantee: uninitialized +
non-TTY exits 2 **without ever launching Python**. FROZEN.

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

### Data loss

| # | Defect |
|---|---|
| D1 | **`recipe remove` destroys lock data.** The inline heredoc regenerates the lock emitting only `[meta]` and `[agents.*]`, discarding `[managed."<path>"]`, `[skills]`, `[deps]`. Every governed override/gate/brief loses its provenance baseline, so `auto` gates that would have been refreshed become "no provenance → preserved with warning" |
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
| D9 | **`upgrade --dry-run` can write.** The mode-only-dirt `git checkout -- .` runs before the dry-run branch, while printing "no changes will be made" |
| D10 | **`upgrade --dry-run` never fetches**, so "Target version" can be arbitrarily stale and it reports "Already up to date" when it is not |
| D11 | **`git fetch origin main` is refspec-less.** A clone without a configured `remote.origin.fetch` never advances `origin/main` and reports "Already up to date" forever |
| D12 | **The agents submenu can append a second `[agents]` block** at EOF when `[agents]` exists without `enabled` and is the last table |

### Robustness

| # | Defect |
|---|---|
| D13 | **Non-atomic writes** in every inline heredoc (`write_text`), while `lock.py` uses `mkstemp` + `os.replace` |
| D14 | **No TOML validation after `recipe remove` / `skills remove`**, unlike `recipe add` and `recipe-config-write` |
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
| D26 | `refresh-bundled`, `doctor`, and `hub` claim read-only and are not (§6) |
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

---

## 11. Environment variables

| Var | Effect |
|---|---|
| `AI_SPECS_HOME` | CLI install root. **Inconsistently honored**: `bin/ai-specs` exports it, but `hub.sh`, `doctor.sh`, `rules-audit.sh` and `version.sh` each re-derive it from `$BASH_SOURCE`. A user pointing it elsewhere gets the dispatcher from one tree and the Python from another |
| `AI_SPECS_SYNC_NESTED` | `1` suppresses `sync-agent`'s banner and footer |
| `AI_SPECS_GATE_BUILD` | `1` opts into local gate build |
| `AI_SPECS_GATE_OFFLINE` | `1` disables gate download |
| `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES` | `1` allows `test-*` recipes |
| `AI_SPECS_VENDOR_FIXTURE_ROOT` | Redirects `kepano/obsidian-skills` clones (test seam) |
| `AI_SPECS_INIT_TUI_PY` | Overrides the wizard entrypoint (test seam) |
| `TMPDIR` | Every `mktemp`; unwritable degrades a step to unfiltered output |
| `PYTHONDONTWRITEBYTECODE` | Incidentally suppresses the `__pycache__` writes in §6 |

---

## 12. Summary of what the port must reproduce exactly

- Every exit code in §2.
- Manifest write formatting, via a line editor — never a marshaller (§3).
- Cache key derivation and every ownership rule in §4.
- The four compact-filter glyphs (§5).
- Symlink kinds and the non-symlink refusal (§4).
- The TTY matrices in §7, including `hub`'s never-launch-Python guarantee.
- `git clone --depth 1` argv, and the absence of pinning (§8) until D16 is fixed.
- Every ordering guarantee in §10.

Everything classified TOLERANT may be reimplemented idiomatically. Everything
FREE — `__pycache__`, `lib/_vendor` bootstrapping, sparse-checkout narrowing,
rich/questionary rendering — has no Go analogue and should simply disappear.
