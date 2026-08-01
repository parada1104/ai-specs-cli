# Proposal: Compact sync output with opt-in verbose

## Status

**Retro-planning.** Production code for this change was already written in the
`ai-specs-cli-sync-output` worktree (192 uncommitted lines across `lib/sync.sh` and
`lib/sync-agent.sh`) with no change folder, no tier classification, and no tests. This
proposal reconstructs the contract after the fact so the work can be verified against
something. Per `CLAUDE.md`, implementation must not continue past the planning gate
without authorization.

## Why (motivation)

`ai-specs sync` and `ai-specs sync-agent` print every per-step detail line unconditionally.
On a repo with recipes and multiple agents the output is a wall of `✓` lines, so the
signal that matters (warnings, skips, failures) is buried. A user cannot tell at a glance
whether a sync was clean.

## Intent

1. Default to a **compact** summary: one `syncing <label>` line per step, with
   warnings/notices/errors passed through untouched.
2. Add `-v/--verbose` to both commands to restore the current full detail.
3. On step failure, always print the **full unfiltered** output regardless of mode, so
   compaction never hides a diagnosis.

## Scope (in)

1. **Verbosity contract**
   - `-v` / `--verbose` flag on `sync` and `sync-agent`; forwarded to nested `sync-agent`
     invocations during public-root fan-out.
   - Compact mode suppresses lines whose first non-whitespace character is a
     success/detail marker (`✓`, `·`, `⇢`, `▸`); every other non-blank line survives.
   - Failure path bypasses filtering entirely.
2. **Step wrapper** — `run_step LABEL CMD...` captures stdout and stderr separately and
   replays each on its original stream after filtering.
3. **Nested-run suppression** — child `sync-agent` runs under fan-out must not repeat the
   header/footer banner; the parent owns the framing.
4. **Marker hygiene** — notices that must survive compaction move off `·` onto `ℹ`.
5. **Regression fix** — restore the fan-out terminal `exit 0` (see below).
6. **Tests** — behavior tests for compact/verbose/failure paths and for fan-out
   termination. Strict TDD per project convention.

## Scope (out)

- Any change to what sync actually writes. This is an output-presentation change only.
- Machine-readable output (`--json`) or log levels beyond the two modes.
- Reformatting the underlying step commands' own messages.
- Colorization or TTY detection.

## Defect found during retro-planning

**The current uncommitted code removes the `exit 0` that terminated the public-root
fan-out block** (`lib/sync-agent.sh:163`). The block at line 121 spawns one child
`sync-agent` per resolved target; `exit 0` was what ended the parent's work there.

Without it, control falls through to line 166 and the parent performs an **additional full
sync pass** using its own `SOURCE_ROOT`/`TARGET_PATH` after all children have finished.
Because the same change adds `export AI_SPECS_SYNC_NESTED=1` before the loop — which
suppresses the header and the `✓ sync-agent complete` footer — that extra pass runs
**silently**. A cosmetic change is masking the duplicate work it introduced.

This must be fixed and covered by a test before anything here merges. It is the reason
this change cannot be classified as cosmetic.

## Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| `sync-output-verbosity` | **New** | Compact-by-default step output, `-v/--verbose` opt-in, failure-always-full, nested-run framing |

## Impact (modules)

| Area | Change |
|------|--------|
| `lib/sync.sh` | `-v` flag, `print_step_output`, `run_step`, steps converted to `run_step` |
| `lib/sync-agent.sh` | same, plus `sync_one_agent` extraction, `AI_SPECS_SYNC_NESTED` framing, fan-out `exit 0` restore |
| `tests/` | **New** behavior coverage; currently zero tests touch this change |
| `docs/` | Document the two modes and the failure-always-full guarantee |
| `CHANGELOG.md` | `[Unreleased]` entry |

## Risks accepted / to decide at the gate

1. **Stream interleaving is lost.** `run_step` captures stdout and stderr to separate
   files and replays them sequentially, so a warning's position relative to progress
   output is no longer preserved. Acceptable for a summary view; must be stated in the spec
   so it is a documented property rather than a surprise.
2. **`shopt -s inherit_errexit` is switched on mid-file** in both scripts. Under
   `set -euo pipefail` this makes command substitutions inherit errexit, which can turn
   previously-tolerated failures into hard exits anywhere below that line. Needs explicit
   test coverage; it is the highest-risk line in the diff after the `exit 0` removal.
3. **`sync_one_agent` extraction adds `|| return $?` on ~12 call sites.** A mechanical
   refactor of the agent loop is bundled with a presentation feature. It may deserve its
   own commit for reviewability.
4. **`run_step` temp files are not trapped.** `mktemp` results are removed on both normal
   and failure paths inside the function, but an `set -e` exit originating elsewhere leaks
   them. Low severity; the enclosing scripts already trap other temps.

## Rollback

Revert the branch. The change is presentation-only once the `exit 0` regression is fixed,
so no user data or generated artifact depends on it.

## Success criteria

1. `ai-specs sync` on a recipe-enabled repo prints one line per step plus any warnings,
   with no `✓` detail spam.
2. `ai-specs sync -v` reproduces today's full output.
3. A failing step prints its complete stdout+stderr in both modes.
4. Public-root fan-out runs exactly N child syncs for N resolved targets and no extra
   parent pass.
5. Full suite green, including new tests for each of the above.
