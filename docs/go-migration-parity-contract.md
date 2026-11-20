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

`bin/ai-specs` dispatches the verbs below, including the native `worktree-gate`
route added by `odd/tasks/gate-into-cli.md` T4. Bare invocation (no arguments) is rewritten
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
| `worktree-gate` | — | `catalog/recipes/worktree-flow/gate/runner` | varies | no |

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

#### Worktree-gate doctor probe — accepted divergence (T6, Robert L5)

The `worktree-gate` check keeps its roster id/name, report-line format, and
advisory severity/exit behavior, but checks the installed CLI at
`$AI_SPECS_HOME/dist/ai-specs` (or `$HOME/.ai-specs/dist/ai-specs` when the
install-home default applies) by running `worktree-gate --selftest`. A missing
or non-executable CLI, or a failing selftest, remains an `ERROR` because the
gate fails open. Doctor says `reinstall ai-specs via install.sh` if the CLI is
missing or not executable, and `run ai-specs upgrade` if the selftest fails.
This replaces inspection of a separately acquired, cached,
digest-verified gate binary; doctor no longer validates its version, receipt,
digest, or cache size. The tracker-ledger doctor query also reaches the same
CLI via the `worktree-gate` subcommand, with the same reinstall guidance for
an unavailable CLI and upgrade guidance for a non-machine-readable verdict.
Approved by Robert in L5; this is the
intentional divergence from the legacy binary-inspection contract. The same
advisory `WARN worktree-gate` check also flags a preserved
`ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` that still
resolves `cache/bin/worktree-gate` without resolving the installed CLI; delete
the customized override and run `ai-specs sync` to refresh it.

#### Sync no longer warns about a missing gate binary — accepted divergence (T6)

With the separate gate binary retired, `sync` no longer tries to acquire it, so
the warning it printed when no usable gate binary was available (for example
offline) is gone. The stderr of that scenario is therefore no longer compared in
the frozen materialization differential. The gate is now served by the
installed CLI: `doctor` reports a missing or failing CLI (`ERROR`) and a
materialized launcher that still resolves the retired binary (`WARN`), and the
launcher itself fails open with its own one-line warning.

#### `worktree-gate` verb in help — accepted divergence (gate-into-cli)

`ai-specs help` lists the new `worktree-gate [gate args...]` verb, which the
legacy help never had. The parity harness drops exactly that line (N14) and
canonicalizes the wording of the unavailable-gate `ERROR tracker-ledger` line
(N15, per the doctor-probe divergence above); severity and line identity are
still compared. N15 does not normalize `worktree-gate` check lines or the
unreadable Tracker verdict diagnostic.

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

The interactive Help action runs `ai-specs help` without a project path:
`help` treats a positional argument as a command topic, not a target.

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

T6b (`odd/tasks/gate-into-cli.md`): sync replaces a launcher whose bytes still
match the lock's managed hash when the rendered catalog version differs;
customized launchers are preserved and already emit the hook-actuator warning
with `rm <path> && ai-specs sync` / `--refresh-gates` guidance. The
`--refresh-gates` option explicitly bypasses ownership classification and
replaces the launcher after backing up its previous bytes. This task adds an
advisory `worktree-gate` doctor warning for the retired cache-binary launcher;
it directs users to `ai-specs sync` and names `--refresh-gates` for customized
files. Sync's existing warning and output are unchanged: adding another
stderr line would alter the FROZEN exact-output surface (S5), so no special
old-launcher sync warning is added here.

### Symlink kinds — FROZEN

- Instructions (`CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`,
  `.omp/AGENTS.md`) → **relative** symlink to `AGENTS.md`.
- Skills dirs (`.claude/skills`, `.cursor/skills`, …) → **absolute** symlink.
- A real file where a symlink belongs is a **hard error**, never overwritten:
  `    ✗ refuse to overwrite non-symlink: <path>`.
- A **real directory** at a skills path (`.claude/skills`, `.pi/skills`, …) is
  the same hard error, never removed. Since T6 of `sync-ownership`, the legacy
  `rm -rf` of a real skills dir is gone: `sync`/`sync-agent` refuses it in
  preflight, before any target write of that run, with one line naming the path
  and its resolution —
  `    ✗ refuse to overwrite non-symlink: <path> — move or delete it so ai-specs can create the symlink`
  — and exit 1; the directory and its contents are untouched. A symlink there
  (correct, stale or dangling) keeps the relink behavior.
- An agent destination reached through a **symlinked ancestor** directory
  (`.claude/skills` while `.claude` → somewhere else, `.pi/mcp.json` while
  `.pi` is a link, …) is judged by where that link LANDS (T6b): allowed when it
  fully resolves **inside the main project root** (the root that contains the
  declared subrepos), so `.claude -> ./config/claude` syncs normally and every
  write lands inside the project; refused — with exit 1 and nothing written
  inside or outside — when it resolves outside that root, when a chained link
  ends outside it, or when it cannot be resolved (dangling or looping). The
  refusal is the shared guard's line —
  `    ✗ ancestor path is a symlink: <ancestor> — replace it with a real directory so ai-specs can write inside the project`
  — and the comparison uses the resolved root (the platform's `/var` →
  `/private/var` style links must not decide it). `sync` and `sync-agent` run
  the SAME preflight over every fan-out target and every selected agent's
  destinations (`instructions_path`, `skills_dir`, `mcp_config_path`,
  `commands_dir`, `runtime_hooks_target`) as their first action, before the
  lock, `.gitignore`, `.envrc`, `AGENTS.md`, materialize or dep-prune steps, so
  a refusal leaves the project byte-identical. The inside-root rule is ONE
  helper (`internal/sync/syncagent.go` `symlinkedAncestorOutside`) shared by
  the agent-destination preflight AND the runtime-hook prune plan
  (`internal/sync/hooksprune.go` `pruneHookFile`): a deletion through an
  inside-root link still stays inside the project, so `planHookShimPrune`, the
  claude/cursor hook-wiring bucket prune and the recorded-shim apply all accept
  it while still refusing outside, chained-outside and dangling links. The
  remaining guards keep the STRICT rule (any symlinked ancestor refuses,
  `shared.EnsureTemplateAncestorsReal`): the shared `pruneFile` still used by
  command pruning (`internal/sync/commandsprune.go`) and recipe-artifact orphans
  (`internal/sync/orphans.go`), recipe/template artifacts
  (`catalog/recipes/worktree-flow/gate/shared/templateactuator.go`) and the sync
  lock in the CLI home (`internal/sync/synclock_unix.go`). Those were not
  relaxed because the inside-root full-sync case never reaches them (the linked
  agent dirs hold no command cache, recipe artifact or sync lock). The per-run
  body repeats the check for nested and direct child runs.
- **Write containment (T6c).** A listed destination is not the whole write path:
  the leaf itself (`.pi/mcp.json`), a generated child
  (`.pi/extensions/<shim>.ts`) or a generated subdirectory
  (`commands/`, `hooks/`, `plugin/`) can be its own symlink. One guard,
  `agentWriteGuard` (`internal/sync/agentwrite.go`), resolves where a write
  would ACTUALLY land — following an existing leaf symlink and every ancestor,
  with `resolveWriteTarget` re-joining a not-yet-existing tail onto the deepest
  existing ancestor and treating a dangling/looping link as an error — and
  refuses when that location is outside the resolved main project root or
  inside the protected `ai-specs/` / `.git/` trees. Every agent-destination
  write and create routes through it: the MCP JSON/TOML renderer
  (`internal/sync/mcprender.go` target + `.bak`), the hook settings/shim
  writers (`internal/sync/hooksrender.go` `hooksWriteJSONFile`/`hooksWriteText`,
  all five renderers, including the prune rewrite), the invalid-JSON backup
  (`internal/sync/jsontarget.go`), the command copies and their directory
  (`internal/sync/syncagent.go` `syncCommands`), and the agent skills /
  instruction links (`makeSkillsSymlink` / `makeRelativeSymlinkWithRefusal`
  guard the parent because those writes REPLACE the leaf link, never write
  through it). The preflight runs the same guard over the five listed
  destinations, so the common cases refuse before any write. An agent parent
  that resolves inside the root but into `ai-specs/` or `.git/` is refused
  (case-insensitive spellings are a follow-up). Deliberately NOT routed: the
  target's own `ai-specs/**` mirror (`mirrorDirectory`), AGENTS.md rendering,
  `.gitignore`/`.envrc` scaffolding, vendored/bundled caches, recipe artifacts
  and the mktemp capture files — none of them is an agent destination, and the
  `ai-specs/` refusal would reject the first two by design.
- **Project-tree, cache and brief containment (T6d).** The same mechanism
  (`internal/projectcache` `ContainWrite`/`ResolveWriteTarget`, used through
  `projectWriteGuard` / `homeWriteGuard` in `internal/sync`) now covers the
  sites the agent preflight cannot see:
  - project tree — the subrepo `ai-specs/` mirror and its `skills/`+`commands/`
    tree (`internal/sync/syncagent.go` `ensureTargetWorkspace`,
    `mirrorDirectory`, including the `rm -rf` that precedes the copy), the
    ai-specs and root `.gitignore` renders (`internal/sync/gitignore.go`), the
    env scaffolding files and their `.bak` (`internal/sync/envscaffold.go`),
    and the brief render (`internal/sync/agentsrender.go`, which now refuses
    BEFORE creating a dangling outside target). `sync` also runs a
    project-tree preflight over those destinations plus every fan-out target's
    `ai-specs/` mirror before its first step, so a refusal is write-free.
    Unlike an agent destination, `ai-specs/` is a legitimate target here; only
    the resolved-root containment applies.
  - CLI home — the per-project cache and its `meta.toml`
    (`internal/projectcache` `EnsureCache`), the `sync.lock`
    (`internal/sync/synclock_unix.go`), the `.deps` vendoring tree
    (`internal/sync/vendorskills.go`, project-owned variant included) and the
    `.bundled` caches (`internal/sync/refreshbundled.go`). These paths are
    ai-specs-owned, so the strict rule applies: ANY symlink below the home —
    a dangling leaf included — refuses instead of being created or written
    through.
  - A refused generated hook shim now fails the run (rc 1) instead of leaving
    the shell's `if python3 ...; then echo ✓; fi` swallow in place: reporting
    success after a write that did not happen is worse than the deviation.
- **Orphan and leftover cleanup containment (T6e).** Orphan cleanup deletes
  `<root>/<name>` under three roots, so the ROOT and each candidate are
  contained before anything is deleted: the in-project `ai-specs/.deps` root
  follows the project rule, the CLI-cache `.deps` and `.recipe` roots the strict
  home rule (`internal/sync/orphans.go` `containOrphanRoots`, called by
  `CleanOrphans` before the shared apply). A redirected root fails the run
  (rc 1) and deletes nothing, and the same roots are part of the sync spine's
  project-tree preflight so the refusal happens before the materialize step
  writes anything. `CacheRoot` RESOLVES the per-project cache-key directory, so
  the containment check uses the LEXICAL key path — a symlink there refuses
  (this also covers the standalone `sync-agent` route that used to write into
  the link target inside the home). The best-effort leftover sweeps
  (`internal/projectcache` `RemoveBundledSkillLeftovers`,
  `RemoveRecipeCommandLeftovers`, `RemoveLegacyOrigin`, `MergeCommands`) contain
  each tree they delete the same way (a symlink LEAF stays the primitive's own
  shutil-shaped refusal, so the pinned messages are unchanged), and the
  recipe-artifact prune contains each managed artifact path before removing it.
- **Leftover, migration and legacy-cleanup containment (T6f).** The
  bundled-command and recipe-command leftover removal, the legacy override
  migration, and the legacy `premerge_guardian.py` / empty `bin/` removal go
  through one project-rule handle (`internal/projectcache` `ProjectTree`), so a
  symlinked `commands/`, `recipes/<id>/` or `bin/` that resolves outside the
  project never has an outside file deleted or written. The sync spine's
  project-tree preflight also covers `ai-specs/.deps`, each target's
  `ai-specs/` managed dirs and every `ai-specs/recipes/<id>`, so the refusal
  (rc 1) happens before the first write.

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
| `doctor` | "read-only" in `ai-specs help` | No project writes, but legacy Python wrote `__pycache__`; Go **executes** `git`, the installed CLI's `worktree-gate --selftest` and `--ledger` routes, and every recipe dep's `version_check` with `shell=True` |
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
| `doctor` | installed CLI `worktree-gate --selftest` and tracker-ledger route | Selftest timeout 30 seconds |
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
| D10 | **`upgrade --dry-run` never fetched** — **CORRECTED in the Go route** (after v0.25.1). `--dry-run` now runs the same fetch as a real upgrade (a failure exits 4) and then reports the real target. The fetch moves only `refs/remotes/origin/main`; the working tree is untouched, pinned by `TestDryRunFetchesSoTheTargetIsCurrent`. D10 is a defect and not FROZEN |
| D11 | **`git fetch origin main` was refspec-less** — **CORRECTED in the Go route** (after v0.25.1). Both paths now fetch `+refs/heads/main:refs/remotes/origin/main`, so a clone without a configured `remote.origin.fetch` advances `origin/main`; pinned by `TestFetchAdvancesOriginMainWithoutConfiguredRefspec` |
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

### Concurrent `sync` runs — settled divergence

Concurrent `ai-specs sync` runs for one project are serialized by an exclusive
per-project advisory lock at `<cache-root>/sync.lock`. A waiter blocks for up to
120 seconds, then exits 1 with a retry diagnostic. This covers the Go CLI's
`sync` command only: standalone `sync-agent` and runs using different CLI homes
are not serialized by this lock.

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

D10 (`--dry-run` never fetched, so "Target version" could be stale) was a separate
row and was corrected afterwards; see its row above.

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

### CLI argument validation and leading TOML BOM — TOLERANT (settled divergence)

The Go dispatcher rejects `help <unknown-topic>` and extra positional arguments
following `version`, `-v` or `--version` with exit 2 and usage guidance; bare
help and valid help topics print the same embedded root help, and plain version
invocations retain their output. This intentionally improves on the legacy
launcher, which silently accepted the unknown topic and discarded version
arguments. The shared TOML reader accepts and strips exactly one leading UTF-8
BOM; BOM-only input is an empty document, while a second or interior BOM remains
invalid. All other parser and CLI behavior is unchanged.

### Invalid input now fails loudly — TOLERANT (settled, human-decided 2026-11-15, BUG-9)

The legacy exit codes below were defects, not contract. The Go route now fails
with exit 1 for: an unknown agent in `[agents].enabled` (the remaining agents
still sync, the run ends 1 without the success footer); `[agents].enabled` that
is not a list; a manifest without a `[project]` table (checked before any
write); `skills list` and `recipe list` on an unparseable manifest (`skills
list` still prints every section). In both gate modes, the
`sync-agent-arg-contract` goldens pin exit 1 and the exact error summary
`ERROR: 3 unknown agent(s) in the agent list; the others were synced.` for the
unknown-agent step, with no success footer. The partial tree is unchanged.

### Non-symlink instructions refusal continues agent fan-out — TOLERANT (settled)

A real file at an agent's instructions path is still refused with the unchanged
`    ✗ refuse to overwrite non-symlink: <path>` line and remains untouched. The
Go sync-agent adds `    → move or delete the file above, then re-run sync`,
skips that agent's later artifacts, and continues syncing remaining agents.
After the loop it exits 1 with an `ERROR: <n> agent instruction file(s) were
left untouched (<agents>); the other agents were synced.` summary instead of a
success footer. This deliberately changes the sync exit/stderr contract (risk
item 3); the legacy fanout-matrix captures are minimally updated by hand with
the new hint and summary. Skills-link refusal and other failure kinds remain
unchanged.

### `tracker evidence` rejects an invalid slug — TOLERANT (settled, human-decided 2026-11-15)

The python original (`ledger_bridge.evidence_payload`) wrote the empty payload
for any slug it could not resolve, including a malformed one. The Go verb now
exits 1 with `ERROR: invalid change slug "<slug>"` and writes no outfile when the
slug is not a plain single path segment. Unchanged and still FROZEN: an empty
slug and a slug naming a nonexistent change exit 0 with the empty payload (hooks
rely on it). The outfile is now created mode 0600 instead of 0644. Pinned by
`TestTrackerEvidenceInvalidSlugFailsAndWritesNothing`,
`TestTrackerEvidenceOutfileIsPrivate` and `TestEvidencePayloadRejectsAnInvalidSlugFormat`.
### `doctor` ledger relevance ignores an unbound witness — TOLERANT (settled divergence)

`trackerLedgerInPlay` (`internal/doctor/checks_tracker.go`) diverges from the python
original `doctor._tracker_ledger_in_play`: a tracker witness whose state is
`unbound` (written by `sync` when no tracker exists) no longer makes the ledger
relevant, so `doctor` does not grade a ledger there. A configured or declared
tracker, or a witness in any other state, still counts.

### Non-finite literals in sync JSON targets — TOLERANT (accepted divergence, Robert, 2026-11-18)

Legacy Python accepted bare `NaN`, `Infinity`, and `-Infinity` in hook and MCP
JSON targets. Go sync treats those literals as invalid JSON: before rewriting,
it saves the exact original bytes to the first free `.bak` or `.bak.N`, warns
once on stderr naming the target and backup, then regenerates as from `{}`.
This applies to existing JSON write targets, not read-only inputs or Codex TOML.
Valid overflowing numbers such as `1e400` remain valid input, and Go writes
the resulting infinities as `1e999` / `-1e999` (legacy wrote bare `Infinity`),
so a later sync never treats its own output as invalid. The same rule applies to
recipe MCP input: a recipe `--recipe-mcp` document carrying bare `NaN` or
`Infinity` fails with one `error:` line naming that file and leaves the target
untouched, where legacy wrote the bare constants into the target (and `nan` /
`inf` for Codex TOML).

### MCP config merge is per server — BEHAVIOUR CHANGE (Robert, 2026-11-18, S5)

Legacy replaced the whole `mcpServers` / `mcp` key (and rebuilt every
`[mcp_servers.*]` block of Codex TOML), so a user-added server was destroyed on
every sync. Go records the server names it manages per target in the lock's
`[mcp."<target>"]` section (name → sha256 of the rendered entry) and touches
only those:

- a recorded server is updated in place, and removed from every agent target
  when the manifest stops declaring it — including when zero servers remain
  (legacy skipped the render entirely at zero, leaving the stale entries);
- a server the lock does not record is never overwritten or deleted; a
  same-named entry that was never recorded as ours is kept and reported with
  `    ! preserved user MCP server "<name>" in <target> (not recorded as
  ai-specs-managed)` (conservative choice: user-authored wins);
- **Codex TOML is written inside one managed block** (`.codex/config.toml` only;
  the JSON agents merge per server above). ai-specs emits
  `# managed-by: ai-specs (do not remove block): MCP servers` /
  `# end managed-by: ai-specs: MCP servers` — the same managed-block shape as
  the `.envrc` block — around its own tables and never rewrites a byte outside
  those markers: comments, CRLF line endings, ordering, formatting and servers
  declared in any TOML form survive exactly. The block is appended after one
  blank separator line when absent, and removed entirely (with that separator)
  once no managed server remains, which restores the original bytes. The markers
  are only trusted when the split validates semantically: the whole file, the
  outside bytes and the block must each parse, the block must hold nothing but
  `[<mcp_key>.<name>]` tables, and the whole file must equal the outside document
  plus the block's servers name by name. Malformed markers (begin without end,
  duplicates, end before begin) and marker lines that do not delimit a real block
  — an example inside a user multi-line string, a begin marker moved above a
  user table — refuse the render: rc 1, one `error:` line, file byte-identical.
  A managed server name that is not a bare TOML key is written quoted
  (`[mcp_servers."dev.server"]`, with `\UXXXXXXXX` above U+FFFF so a non-BMP
  name round-trips instead of becoming an invalid UTF-16 surrogate pair), and
  after rendering every written name must exist at `<mcp_key>.<name>` in the
  parsed result or the render refuses, so a dotted name can never nest into a
  user server. Beyond proving the reconstruction, the block is ours only around
  servers the lock records: any server inside the markers that is not in the
  `[mcp."<target>"]` record (a hand-moved end marker wrapping a user table, or a
  block with no record at all) refuses the render with one line naming the
  unowned server(s) and telling the user to move them outside the block, leaving
  the file and the lock byte-identical. ai-specs writes no comment inside its
  block other than the two markers, so a comment line between them (the parsed
  comparison cannot see text) is user content the marker cut would delete: the
  render refuses, names the comment and asks for it to be moved outside the
  markers, leaving the file and the lock byte-identical. The definitive guard is
  the **block hash**: the lock records `[mcp-block."<target>"]` with the sha256
  of the exact bytes ai-specs last wrote for the block (from the begin marker
  line through the end marker line, end-of-line included — the same span the
  marker cut removes). On every sync, a block whose current bytes differ from
  that hash was edited by hand — an inline comment on a generated line, an
  edited value, an added table, a moved marker — and refuses atomically: rc 1,
  one `error:` line saying the block was edited and to move user content outside
  the markers or restore the block (e.g. by deleting it), with the file and the
  MCP record byte-identical. A matching hash proceeds normally (update or prune).
  There is no adoption path: a block present with no recorded hash has no proof
  it is ours, so it refuses with its own line ("the managed MCP block has no
  recorded hash; delete the block (ai-specs regenerates it) or move user content
  outside the markers"), file and MCP record byte-identical — the managed-block
  format never shipped in a release, so no real project holds a hashless block.
  This tightens an earlier behavior:
  a manual edit to a managed value inside the block used to be overwritten on
  the next sync and now refuses, under "if in doubt, don't touch". The outside
  content must parse with `internal/toml.Parse` before anything is written, so
  the user's servers are read semantically: a manifest server whose name already
  exists outside is preserved with the collision warning and no table is emitted
  for it, so a document is never made to redeclare a key, and when the outside
  does not parse nothing is added or changed. A final self-check re-parses the
  merged document and refuses rather than write TOML our own parser rejects.
  Accepted limitation: because user bytes are never rewritten, an outside
  document that `internal/toml` accepts but a stricter TOML implementation
  rejects is preserved as written — nothing is lost, the file is left exactly as
  the user wrote it;
- removals print `    ✓ removed managed MCP server "<name>" from <target>`;
- a JSON target with recorded servers is regenerated through the same recovery
  path even when the manifest declares none left, so a corrupt JSON target is
  still backed up and rewritten instead of keeping a stale managed server
  (Codex TOML has no such recovery: it is refused, never rewritten);
- a target that holds servers ai-specs never recorded and has no lock entry yet
  (the upgrade case) gets ONE notice per target — `    ! <target>: MCP servers
  not recorded as ai-specs-managed: <names> (remove them if ai-specs generated
  them)` — and a bare `[mcp."<target>"]` marker entry meaning "the user has been
  told about the servers ai-specs does not manage here", so it is not repeated.
  The marker survives while an unrecorded server or a preserved collision
  remains at that target, and is dropped once none do. Nothing is deleted for it;
- a lock read failure stops the MCP step before touching the target, so nothing
  is deleted without the ownership record.

Accepted corpus divergence: every Codex TOML target now carries the managed
markers (and a plain rewrite therefore differs by construction), `mcp-per-agent`
(the seeded `stale` server survives in `.mcp.json`, `opencode.json` and
`.codex/config.toml`) and
`sync-agent-recipe-mcp-matrix` (step 3's zero-server invocation prunes the
`only` server recorded by step 2 instead of printing the skip notice), plus the
new `[mcp.*]` lock section in fixtures that render an MCP target. The frozen
goldens still capture the legacy behavior; they were not regenerated.

Also accepted: a failed render never truncates the target. Legacy's
`write_text()` opened and truncated the target before the encode error, so a
Codex TOML with a server name that cannot be UTF-8 encoded was left empty; Go
refuses before opening and leaves the file byte-identical.

### Lock hygiene: shim, dep and retired-target records — BEHAVIOUR CHANGE (Robert, 2026-11-19, S3)

"Borrar siempre lo gestionado" needs an ownership record, so sync now records
and prunes three things legacy could not:

- **Generated hook shims** are recorded in the lock's `[hooks]` section
  (project-relative shim path → the owning hook's managed id,
  `ai-specs:hooks:<recipe>:<id>`) when `sync`/`sync-agent` writes them. Pruning
  keys on that record instead of the `GENERATED by ai-specs` marker alone: a
  recorded shim whose hook a recipe no longer declares is deleted **even when
  the user edited it or stripped the marker**, because the lock — not the file
  content — is the ownership proof. A path the lock assigns to a hook that is
  still live (or that a template claimed during materialization) is never
  deleted, and a shim that merely looks generated but has no record is **kept**
  and reported once with `  ! <paths>: generated hook shims not recorded as
  ai-specs-managed (kept; delete them if ai-specs did not write them)`. Records
  whose shim is already gone are dropped.
- Records are project-relative for every fan-out target, so a declared
  subrepo's shim is `sub-a/.pi/extensions/<recipe>-<hook>.ts`, and deletion
  authority exists only inside a target this sync visits: a recorded path with
  a dead owner and an existing generated shim.
- **Every sync drops any `[hooks]` record whose shim is absent**, regardless of
  which target is visited or whether that target is still declared (an omitted
  subrepo, a removed agent). Dropping a record only removes deletion authority,
  which is the safe direction — otherwise the record of an unvisited target
  would outlive its shim and a hand-authored file later placed at that path
  would be deleted by the next sync that visits the target as if it were the
  missing generated shim.
- **`[deps.*]`** entries are pruned by the same authority that removes the
  vendored `.deps` trees, but the manifest's declared dep set — not the
  filesystem orphan plan — decides: a dep the manifest no longer declares loses
  its recorded hashes even when both dep trees are already gone, printing
  `  ✓ removed stale lock entries for dep '<id>'` (legacy only pruned them in
  `skills remove`).
- **`[managed.*]`** entries naming a recipe the manifest no longer enables are
  dropped once their artifact is already gone (e.g. a retired template or doc
  target). A record whose artifact still exists is kept and the file is never
  deleted by this rule, so nothing unproven is removed; entries with no owning
  recipe (e.g. the runtime brief) always survive.

Accepted corpus divergence: the lock bytes of fixtures that render hook shims
or orphan a dep now gain `[hooks]` records and lose stale `[deps.*]`/
`[managed.*]` entries (`hooks-five-runtimes`, `sync-agent-recipe-mcp-matrix`,
`orphan-cleanup`, `all-recipes-enabled`, `cache-layout`, `doctor-healthy`,
`mcp-per-agent`). The failing fixture set is unchanged — the same seven of 35 in
each gate mode — and no golden was regenerated.

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
- Install fixture modes: the parity harness stages the shipped install assets
  in each scratch home as 0755 directories and 0644 regular files (0755 for
  executables). Some sync steps copy asset modes, so using the live checkout
  directly would leak its local permissions. Project snapshots still compare
  actual output modes, including explicitly seeded fixture permissions.
- Normalization registry (`tests/parity/parity.py`): N1-N6 are run
artifacts and the frozen help-heredoc quirk; N7/N8 were removed as dead (S7b);
N9 covers the `sync` clone-failure wrapper; **N10 (T7 cursor deny line) and N11
(`add-dep`/`skills add` clone failure) are ACTIVE pending a corpus recapture
decision** — measured in S7b, dropping them fails three fixtures in both gate
modes, because the frozen bytes still carry the legacy shapes; **N12-gate-cache-size
is PERMANENT**, never expires, because it normalizes the cached gate binary's
byte total, which moves with every legitimate gate change. **N13-version-stamps**
  also normalizes the matching version in `doctor`'s successful `worktree-gate`
  detail and its cache directory segment under `cache/bin/worktree-gate/`;
  the rest of the path, check status and selftest result stay compared.
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
