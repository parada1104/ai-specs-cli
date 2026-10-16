# Go 20 — Python exit: the binary as the only entry point

Program document (like `go-single-binary-epic.md`): it fixes the END STATE and slices the remaining
work into units. Base `ecfb843` on `epic/go-single-binary`. Worktree
`.worktrees/go-20-python-exit`, branch `change/go-20-python-exit`.

## Specs
- S1 — Session request (verbatim): "decisión 1 me gusta la 4 al final es 'un nuevo inicio' ya con esto
  mas limpio sin tanto python+bash+go podria ser solo go +bash como wrappers" y "quiero que este como
  si se hubiera hecho con go desde 0".
- S2 — Decisions 2 and 3, confirmed by the human as recommended: **B2** — flip the entry point first
  and delete in a following unit, with the caller audit as the gate; **C2** — the parity harness keeps
  an explicit legacy entry/flag so the shell route stays runnable from the live tree.
- S3 — The compiled binary is the ONLY entry point: it is installed as `~/.local/bin/ai-specs` and the
  bash launcher stops being the entry. `AI_SPECS_HOME` stops being "a git checkout you run" and
  becomes the data home (catalog, cache, and `lib/` only while unported verbs still need it).
- S4 — "Como si se hubiera hecho con Go desde 0": zero Python under the product tree; bash exists only
  as wrappers (the shims of unported verbs, the catalog recipe hooks) and never as the orchestrator of
  a verb.
- S5 — No observable behaviour changes without evidence: every unit carries RED→GREEN, its
  differential (parity corpus and/or `tests/validate.sh`) and its native review. The suite counts and
  the parity fixture set are quoted per unit, never asserted.
- S6 — Review budget rules the slicing: one unit = one small PR against the epic with its own review;
  a verb port is sliced further when it does not fit one review.
- S7 — Measured scope at `ecfb843`: **19 bash scripts / 3.039 lines** and **43 Python modules /
  21.346 lines** under `lib/`; of the 19 verbs, 9 are native and 7 are still shims (`hub`, `init`,
  `add-dep`, `skills`, `recipe`, `configure-recipes`, `upgrade`).
- S8 — The program is finished when: `ai-specs` (the binary) runs every verb natively; `lib/` has no
  Python; every surviving bash file is a thin wrapper that calls the binary; the install/upgrade path
  installs and updates a binary install kind; and the parity harness still has a legacy reference.
- S9 — Install source (human decision, verbatim option): "Release verificado + build local de
  respaldo" — `install.sh` downloads the published release asset and verifies it against its
  SHA256SUMS (the same verified-artifact pattern the gate binary already uses), and only builds with
  `go build` when there is no asset for the platform or no network; the native upgrade path uses the
  same channel and the same verification. The binary is installed as the entry point
  (`$INSTALL_BIN/ai-specs`), not symlinked into a checkout, and `AI_SPECS_HOME` remains the data home
  (catalog, cache, and `lib/` only while unported verbs still need it).

## Measured scope

Native verbs today: `refresh-bundled`, `sync`, `sync-agent`, `doctor`, `rules-audit`,
`brief-render-policy`, `baseline-brief`, `version`, `help`.
Shimmed verbs and what they drag in:

| Verb (shim) | bash | Python invoked by those scripts |
| --- | --- | --- |
| `doctor` | `doctor.sh` 46 | `doctor.py` 1560 |
| `rules-audit` | `rules-audit.sh` 44 | `rules-inventory.py` 557 |
| `hub` | `hub.sh` 53 | `hub.py` 689 |
| `init` | `init.sh` 348 | `init_tui.py` 429, `env_scaffold.py` 667, `envrc-scaffold.py` 46, `refresh-bundled.py` 160, `cli_version.py` 302, plus the sync-side renderers already native |
| `add-dep` | `skills-add.sh` 222 | (registration is inline; its sync leg goes through the seam since S19a) |
| `skills` | `skills.sh` 39, `skills-list.sh` 275, `skills-remove.sh` 217 | `project-cache.py` 679, `lock.py` 490 |
| `recipe` | `recipe.sh` 42, `recipe-add.sh` 59, `recipe-init.sh` 59, `recipe-list.sh` 49, `recipe-remove.sh` 203 | `recipe-add.py` 302, `recipe-init.py` 272, `recipe-list.py` 129, `recipe-configure.py` 590, `recipe-read.py` 134 |
| `configure-recipes` | `recipe-config.sh` 49 | `config_wizard.py` 300 |
| `upgrade` | `upgrade.sh` 386 | `changelog.py` 351 (the native engine exists in `internal/upgrade`) |
| (sync route, already native) | `sync.sh` 299, `sync-agent.sh` 551 | `recipe-materialize.py` 4265, `agents-render.py` 881, `hooks-render.py` 481, `mcp-render.py` 315, `target-resolve.py` 207, `flatten-resolved-skills.py` 56, `gitignore-render.py` 64, `gitignore-root-refresh.py` 77, `toml-read.py` 223, `vendor-skills.py` 236, `refresh-bundled.py` 160, `cli_version.py` 302, `env_scaffold.py` 667, `project-cache.py` 679, `lock.py` 490 |
| catalog assets | `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`, `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | the ledger/spec-promotion helpers those hooks call |

Two findings that shape the order:

- The binary ALREADY shims the unported verbs (`internal/cli/shim.go`), so A4 is **stageable**: the flip
  changes where the binary comes from and who is the entry, and the bash behind it stays as an internal
  detail until each verb is ported. Nothing forces a big-bang verb port.
- `lib/sync.sh` stays alive after the flip for one reason only: `lib/upgrade.sh` (still a shim) calls it.
  Under the binary the seam is set, so that call can go native in the same unit the `upgrade` route is
  wired — which is what unlocks deleting `sync.sh`, `sync-agent.sh` and the sync-side Python.

## Tasks
- T1 — **U1 (the flip)**: wire the `upgrade` route natively (`internal/upgrade` already exists; its
  wiring is the recorded follow-up), install the binary as the entry point, add the explicit legacy
  entry for the parity harness (C2), and route `upgrade.sh`'s sync leg through the seam. No deletion.
- T2 — **U2 (the first deletion, after the audit)**: audit `lib/_internal/*.py` callers with the flip
  applied and delete what no live caller reaches (`doctor.py`, `rules-inventory.py`, the sync-side
  renderers and the substrate only the already-native routes used), keeping the parity evidence green.
- T3 — **U3 `hub`**: the interactive launcher (689 lines of Python + a TUI). Includes the decision of
  what replaces the wizard UX now that the parity contract classifies rich/questionary rendering as
  FREE.
- T4 — **U4 `init`**: the manifest write, the scaffold cluster (`env_scaffold.py` 667,
  `envrc-scaffold.py`, `refresh-bundled.py`, `cli_version.py`) and the TUI seam, on top of the
  renderers that are already native.
- T5 — **U5 `skills` + `add-dep`**: the three `skills-*.sh` scripts plus `project-cache.py` and
  `lock.py` usage (both have native cores in `internal/projectcache` and `internal/lock`).
- T6 — **U6 `recipe` + `configure-recipes`**: seven scripts plus `recipe-*.py` and
  `config_wizard.py`, on top of `internal/schema` and `internal/sync` authorities.
- T7 — **U7 the substrate tail**: `util.py` 864, `recipe_schema.py` 864, `gate_binary.py` 739 (whose
  native twin already exists in `internal/gatebinary`), `toml-read.py`/`toml_write.py`, and the two
  catalog hooks that still call Python.
- T8 — **U8 the close**: `lib/` without Python, bash only as wrappers, the install/upgrade ADR, and the
  verification that the suite and the parity corpus prove the end state.

Each task is sliced into review-sized units when it is opened; the sizes above are the reason T4–T6
cannot each be one PR.

## Log
- L1 — The human's decision (verbatim, 2026-10-15): "decisión 1 me gusta la 4 al final es 'un nuevo
  inicio' ya con esto mas limpio sin tanto python+bash+go podria ser solo go +bash como wrappers",
  "2 2?" and "3 3?" (confirming B2 and C2 as recommended), and "quiero que este como si se hubiera
  hecho con go desde 0".
- L2 — Measured reconnaissance at `ecfb843`, recorded in the tables above: 19 bash scripts / 3.039
  lines, 43 Python modules / 21.346 lines, 9 native verbs and 7 shims, plus the per-script Python
  invocation map and the two catalog hooks that call Python.
- L3 — Grep discipline that avoided a false audit: several `lib/_internal/*.py` mentions inside the
  gate's Go files and the catalog docs are COMMENTS that name the Python module they mirror
  (`util.py`, `recipe_schema.py`, `lock.py`), not callers. The real callers are the bash scripts.
- L4 — The staging insight this program depends on: `internal/cli` already dispatches unported verbs to
  bash, so A4 does not require porting them first, and the flip can land before the ports.
- L5 — Consequence recorded for U1: `lib/upgrade.sh` calling `lib/sync.sh` is the last thing keeping
  the shell sync alive after the flip; the seam (S19a) makes that call native, which is what makes U2's
  deletion of the sync-side Python possible.
- L6 — Open decision handed to the human (it shapes U1): where the binary comes from at install time —
  a verified release asset, a local `go build`, or both in a defined order. The repo already has the
  verified-artifact pattern for the gate binary, and the upgrade engine already distinguishes install
  kinds.
- L7 — Decided (2026-10-15): verified release asset first, local `go build` as the fallback (S9).
  Consequences recorded for U1: it depends on the release assets card (`odd/tasks/go-12-upgrade.md`
  names Go 15's release assets and `<home>/bin/SHA256SUMS`), the verification must reuse the
  `internal/gatebinary` digest discipline rather than inventing a second one, and the upgrade engine
  gains the binary install kind next to the legacy checkout kind.
- L8 — U1 is sliced before it is opened, because it is three different risks in one story:
  **U1a** wire the native `upgrade` route (its core already exists), **U1b** the install/release/data
  home change, **U1c** the explicit legacy entry for parity plus `upgrade.sh`'s sync leg through the
  seam (the change that orphans the shell sync).

## Progress (to be completed by the implementation)
