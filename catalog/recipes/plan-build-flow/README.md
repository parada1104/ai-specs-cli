# Plan / Build Flow (Ambient)

An invisible change workflow: the bundled skill auto-invokes on substantial
requests, **classifies planning depth**, produces reviewable artifacts, waits
for authorization, then implements, validates, and closes — **without** `/plan`
or `/build` commands.

## What it provides

- **Skill `plan-build-flow`** (bundled) — depth classifier (standard /
  light), ambient planning/build contract, PR artifact gate, pre-merge archive
gate, and graceful degradation without an orchestrator.
- **Doc** — this README at `ai-specs/recipes/plan-build-flow/README.md`.

## Planning depth (classifier)

| Tier | Typical use | Planning chain | Minimum artifacts before build |
|------|-------------|----------------|-------------------------------|
| **Standard** | Scoped feature, bounded multi-file work, or a new capability | conditional explore → proposal → spec → tasks | `tasks.md` plan (`## Specs`, `## Tasks`) plus at least one `specs/**/*.md` delta |
| **Light** | Small fix with clear file/symbol target | proposal → tasks | `tasks.md` plan only, no delta |

Direct "implement this" requests still run the classifier first when no change
folder exists yet.

## Removed: the full tier

The former **Full** tier no longer exists. A change that declares `Depth: full`
(or is graded with `--tier full`) blocks instead of being downgraded silently,
with a migration message that points at standard:

```text
archive/<folder>: the full tier was removed: migrate the change to `Depth: standard` (the plan plus at least one `specs/` delta)
```

Migrate by rewriting `Depth: full` to `Depth: standard` and keeping the plan
plus at least one `specs/` delta. The former explore → proposal → spec/design →
tasks phase machine is no longer part of this recipe; standard and light remain
collapsed and unchanged.

Preflight is one session-level authority for execution mode, artifact store,
review budget, delivery strategy, and chain strategy. Plan-build consumes those
values without recollecting or overriding them. Its final artifact-derived
presentation covers intent, scope, key decisions, affected areas, risks, open
questions, and labeled recommendations/assumptions. Interactive mode asks
questions after the phase that exposed them; automatic mode records assumptions
and blocks unresolved product decisions. The final review requires explicit
accept, adjust, or stop.

The classifier always computes a signal tier and separately checks for an
explicit requested depth. Illustrative requests such as “acotado con spec” or
“solo tasks” are compared with that signal; this is guidance, not an exhaustive
parser. A request for “full planning” names the removed tier: explain that it
was removed, point at standard, and record `Depth: standard` only once the human
agrees. A mismatch is a depth conflict: the agent must ask which tier wins in
either direction before writing that planning chain.
When the conflict is resolved, `tasks.md` records standalone `Depth: <tier>` plus
the requested, signal, decided, and decision-source annotation lines. A same-turn
preference may resolve the conflict without a repeat ask; a deeper decision still
requires its complete planning chain.

## Execution modes

Plan Build resolves exactly one execution mode per repository from the project
manifest, keyed `mode` under `[recipes.plan-build-flow.config]`:

```toml
[recipes.plan-build-flow.config]
mode = "auto"   # auto | odd | openspec
```

- **`auto`** (default) infers the mode from the Gentle AI state directory inside
the Git common directory (`<git-common-dir>/gentle-ai/`): present resolves to
`odd`, absent to `openspec`. An explicit `odd` or `openspec` wins over the
inference, and a project where that directory does not exist yet resolves to
`openspec` while it stays absent. Write `mode` by hand: the recipe schema
does not declare it yet, so `ai-specs recipe configure` cannot set it.
- **`odd`** keeps every planning artifact under `odd/`: canonical specs at
`odd/specs/<domain>/spec.md`, the change plan at `odd/tasks/<change>.md`, and a
delta at `odd/changes/<change>/specs/<domain>/spec.md` only for a change that
adds, modifies or removes an observable rule. `<change>` is the branch name's
last path segment after stripping any prefix, so `feat/thing` and `fix/thing`
both pair with the plan `odd/tasks/thing.md`. `ai-specs specs promote` composes
`odd/changes/<slug>/specs/<domain>/spec.md` into
`odd/specs/<domain>/spec.md`; `openspec/specs/**` is neither read nor written. `ai-specs gates premerge` pairs the change from the branch, checks that
the plan exists and every `odd/changes/<slug>/specs/` delta was promoted, and
requires the delta folder to be archived before merge.
There is no `odd/config.yml`: the mode lives in the project manifest.
- **`openspec`** is the unchanged OpenSpec-centered flow described below.

## Project config (`config.yaml`) — optional and ungraded

`config.yaml` configures the **project**, not the change. It is an `openspec`
artefact only, at `openspec/config.yaml` (`odd` mode has no config file, and
`.yml` is not read). The premerge neither requires nor grades it: the CLI does
not depend on the OpenSpec CLI, so a missing file never blocks and its
`schema:`, `testing.runner:` and `strict_tdd:` keys are not validated. OpenSpec
and agents may still read `context`, `rules.*` and `ai_specs_guidance`; the
`tracking:` block is read by the Tracker domain (`internal/doctor`, the ledger
witness) and must be kept intact.

### Verification commands

One rule survives, graded from Standard up over the change's own
`## Verification` rows: every command a row records must resolve here — a `./`-
or `/`-prefixed path must exist and be a regular file, a bare word must be found
on `PATH`. A change with no rows, or with no command in any row, is exempt: that
is the proportional-ceremony rule (S12), not a bypass. The blocker names the plan
that records it:

```text
openspec/changes/archive/<slug>/tasks.md: verification for S1 names command "./nope.sh" which does not exist: record the real command
```

Every `## Verification` row of a Standard change must also name the
command it ran in backticks: a verdict without a command records no run, so
`- **S1: PASS** — nothing executed` blocks with `verification for S1 must name
the command it ran in backticks: record "- **S1: PASS** — `go test ./...`"`.
The row must record exactly one command with no prose before it and no bare
second backticked value; a row recording `exit 1`, `exit 2` or `failed` blocks as
a failing run; an `N/A` row with a reason is exempt; and two rows for the same
`S#` block naming the id. A plan that declares `Depth: standard` also requires
`## Log` with `L1`; Light keeps it optional.

## PR and merge gates

- **No PR** until the change's plan parses: `openspec/changes/<slug>/tasks.md`
  with `## Specs` and `## Tasks`, plus `## Log` with `L1` from Standard up (Light
  keeps it optional) and a spec delta for Standard. `proposal.md`,
  `design.md` and `verify-report.md` are still accepted, but they are no longer
  required. Every archive is graded by that grammar whatever its folder is
  called, and it must hold the plan or the delta: an empty folder blocks.
- **Verify before archive:** Standard requires a dedicated `verify-report.md`
  with command, exit `0`, a valid calendar date, commit SHA, and a non-failing
  verdict. Light is advisory only.
- **Promote canonical specs before archiving:** Standard deltas under
  `openspec/changes/<slug>/specs/<domain>/spec.md` are composed into
  `openspec/specs/<domain>/spec.md` by the promotion helper
  `ai-specs specs promote <slug> --root <planning-root>`. ADDED
  requirements are appended, MODIFIED requirements replace the full canonical
  block with the exact same requirement name, unrelated canonical requirements
  and sections survive, a missing canonical domain is created from the delta, and
  a rerun after an interruption is a no-op. A colliding ADDED requirement, a
  MODIFIED target that does not exist, and an unsupported RENAMED delta block
  without writing; a destructive REMOVED delta blocks unless the removal was
  explicitly approved, then re-run with `--allow-removed`. Only the promoter
  writes canonical specs; the guardian only validates them and never mutates
  `openspec/specs/`. Light changes and changes with no deltas are unaffected.
- **Two enforcement points:** run the verify gate before archive-tail and run
  the pre-merge guardian again after archive. Both points also block a Standard
  delta that is unpromoted or unresolved. Missing `explore.md` is never a
  guardian blocker.
- **Archive before merge** on the review branch — never after merge lands on the
  base branch. OpenSpec's canonical destination is
  `openspec/changes/archive/YYYY-MM-DD-<slug>/` with a valid ISO calendar date.
  The exact undated `archive/<slug>/` directory remains a legacy fallback only
  when no dated candidate exists; it is graded by the same plan grammar as a
  dated archive, and it must hold the plan or the delta like any other archive
  (an empty folder blocks). Multiple dated candidates, dated-plus-undated
  candidates, invalid dates, and near-match names fail closed with named
  blockers.
- **Archive with the verb:** `ai-specs changes archive [<change>] --root
  <planning-root> [--date YYYY-MM-DD]` is the mechanical archive step in both
  modes. It resolves the change with the same ladder as the guardian, refuses
  while a delta is unpromoted, refuses to create a second archive for a change,
  refuses a destination that resolves through a symlink or outside the
  repository, and moves with one `os.Rename` (never deletes). In `openspec` the
  plan lives inside the change folder and moves with it; in `odd` only
  `odd/changes/<change>/` moves and `odd/tasks/<change>.md` stays as the feature
  record. A change with no delta folder is still archived: the dated folder is
  created holding a three-line `archived.md` marker instead of the plan copy, and
  `--change` keeps the last path
  segment of a branch-shaped value, so CI passes `GITHUB_HEAD_REF` unchanged;
  an empty `<change>` or `--change=` is a usage error, never "omitted", and so
  is a name that is not a single path segment (`..`, `../../../offtree`, `a/b`):
  the guardian exits `2` with `ERROR: invalid change name: <name> (expected a
  single path segment)` instead of grading a folder outside the change tree.
- **Tracker `archive-close` is not this archive.** Tracker item closure is a
  separate Tracker-domain checkpoint
  (`ai-specs tracker-card-gate --root <root>
  --checkpoint archive-close [slug]`). It
  never moves the change folder, never infers closure from OpenSpec archive
  state, and writes nothing. Plan Build's artifact guardian is tracker-free.
- **Verification commands resolve:** every command a Standard change records in
  `## Verification` must resolve on disk (see above). `config.yaml` is neither
  required nor graded.

## Third assurance layer: pull-request CI

The merge decision is the human's, and the assurance is layered so a merge
cannot happen by hand without the archive: the CLI verb is layer one, the skill
that obeys it is layer two, and `.github/workflows/pr-gate.yml` is layer three.
On every pull request against `development` that workflow builds the binary and
runs

```bash
trusted/dist/ai-specs gates premerge \
  --root "$GITHUB_WORKSPACE/candidate" --change "$GITHUB_HEAD_REF"
```

`actions/checkout` leaves `HEAD` detached on `pull_request_target`, so the change
ladder cannot pair a branch with a change folder and the explicit `--change` is
mandatory; the evaluator comes from the base branch and `candidate/` is data.
`--change feat/alpha`
keeps its last path segment (`alpha`) through the same helper the branch pairing
uses, so `GITHUB_HEAD_REF` can be passed as is. The workflow grades committed
state only: push the archive commit or the check cannot pass.

Making that job a **required** status check is a repository branch-protection
setting, not a file in this repo. The owner applies it once:

```bash
gh api --method PUT \
  repos/<owner>/<repo>/branches/development/protection/required_status_checks \
  -H "Accept: application/vnd.github+json" \
  --input - <<'JSON'
{"strict": true, "contexts": ["pr-gate"]}
JSON
```

Plans already in flight when this contract ships add missing `proposal.md` or
verify evidence before their PR/archive; no replan or restart is needed. One
edit is mandatory: a plan still declaring `Depth: full` rewrites it to
`Depth: standard` and keeps its spec delta, otherwise the guardian blocks with
the migration message. Historical archives are never rewritten. A stale PR is
handled by its owning agent, which adds the missing artifacts when that change
resumes.

## Capability

- `plan-build-flow` — ambient plan/build workflow (skill-only surface).

## Enable

```toml
[recipes.plan-build-flow]
enabled = true
version = "1.8.0"
```

Then run `ai-specs sync`.

## Delivery contracts

`artifact_store_default` is an optional string configuration that declares the
external-session persistence preference for this project's planning artifacts.
It accepts `openspec`, `engram`, or `both`, and defaults to `openspec` when the
project manifest does not override it.

```toml
[recipes.plan-build-flow.config]
artifact_store_default = "both"
```

The value is a persistence preference only: plan-build readiness is always
proven by file-backed artifacts under the canonical `openspec/changes/<slug>/`
tree — `tasks.md`, tier minimum planning files, and `verify-report.md` — and
never by the store selection. Engram MAY mirror artifacts but never replaces
them; a memory-only presence cannot satisfy any readiness check.

During `ai-specs sync`, the resolved value is materialized into the generated brief as a
workflow rule. An external session runtime may consume that rule when asked where planning
artifacts should live; this recipe declares the repository default but does not control that
runtime's session behavior.

## Worktree coexistence

Implementation defers to `worktree-flow` when enabled. The recipe syncs
standalone without a hard dependency.

## Cross-repository planning boundaries

When a recognized submodule code worktree is used, repository topology identifies
the containing **superproject** and its **central** planning tree. Central active
plan lookup and artifact writes stay within that planning tree; they never grant a
superproject-wide production-write bypass. A **standalone** repository continues
to use its own nearest planning root. If the topology cannot be proven, resolution
is **fail-safe** and retains nearest-repository behavior. The recipe has **no
duplication** and **no orchestration** boundary: it does not copy, synchronize, or
create per-repository plans or change worktrees and branches.
