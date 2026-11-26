# Plan / Build Flow (Ambient)

An invisible change workflow: the bundled skill auto-invokes on substantial
requests, **derives the change's tier from its artifacts**, produces the plan
and its optional delta, waits for authorization, then implements, validates,
and closes — **without** `/plan` or `/build` commands.

## What it provides

- **Skill `plan-build-flow`** (bundled) — artifact-derived tiers (none / light /
  standard), ambient planning/build contract, PR artifact gate, pre-merge
archive gate, and graceful degradation without an orchestrator.
- **Doc** — this README at `ai-specs/recipes/plan-build-flow/README.md`.

## Planning tier (derived from the artifacts)

The tier is never declared and never estimated from size. The CLI reads it from
the artifacts the branch carries, at the gate and at the pre-merge guardian:

| Tier | Artifacts | What the guardian grades |
|------|-----------|--------------------------|
| **none** | no plan, no delta, no change folder — an ODD small change | nothing; only the diff refutations R-a and R-b apply |
| **light** | a plan alone, or a delta alone | the plan grammar (`## Specs`, `## Tasks`), every `Commit` cell an ancestor of `HEAD` and, when there is a delta, its promotion |
| **standard** | a plan and a delta | the plan grammar, `## Log` with `L1`, one `## Verification` row per `S#`, a command that resolves, promotion parity, and every `Commit` cell an ancestor of `HEAD` |

Direct "implement this" requests still write the artifacts the change needs
first. Whether a change needs a plan at all is ODD's Task Size: understood
outcome, contained risk, and resumable from the diff. File, line, command and
test counts never decide, and no line count forces a split — the ~400
authored-line figure slices work units and never decides a tier.

`Depth: <tier>` in the plan and `--tier <tier>` on the guardian are coherence
assertions, never a choice: a value that contradicts the artifacts blocks,
naming the derived tier and both exits.

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

The tier is what the artifacts say, so there is no signal tier to compare and no
depth conflict to ask about: write the plan when the change needs one and the
delta when it changes an observable rule, and keep any `Depth:` line matching
what you wrote.

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
`openspec` while it stays absent. The schema declares `mode` with
`auto | odd | openspec`, so `ai-specs recipe configure --set mode=odd` sets it and
an absent value means `auto`.
- **`odd`** keeps every planning artifact under `odd/`: canonical specs at
`odd/specs/<domain>/spec.md`, the change plan at `odd/tasks/<change>.md`, and a
delta at `odd/changes/<change>/specs/<domain>/spec.md` only for a change that
adds, modifies or removes an observable rule. `<change>` is the branch name's
last path segment after stripping any prefix, so `feat/thing` and `fix/thing`
both pair with the plan `odd/tasks/thing.md`. `ai-specs specs promote` composes
`odd/changes/<slug>/specs/<domain>/spec.md` into
`odd/specs/<domain>/spec.md`; `openspec/specs/**` is neither read nor written. `ai-specs gates premerge` pairs the change from the branch and grades only
what the folder and the delta say: every `odd/changes/<slug>/specs/` delta must
be promoted, and an archived dated folder is required only when the change still
carries `odd/changes/<slug>/`. It never reads `odd/tasks/<change>.md`: ODD owns
that document. A change with
no plan, no folder and no archive is `none` and passes with
`OK (none, change <slug>)` unless its diff is refuted (see below). The
pre-tool-use hook requires no plan for a production write; the only protected
odd path is the canonical tree `odd/specs/`, written by promotion and refused to
a hand write unless the paired change carries a delta under
`odd/changes/<slug>/specs/`, active or archived.
There is no `odd/config.yml`: the mode lives in the project manifest.
- **`openspec`** is the OpenSpec-centered flow described below. Its hook still
  requires the plan of the paired change, with one local escape hatch:
  `ai-specs changes start --none --root <root>` writes the per-branch git config
  entry `branch.<branch>.aiSpecsCeremony=none`, which is never an artifact,
  never travels and is never committed. It relaxes only that plan requirement:
  the ledger `work-start` checkpoint still runs and the pre-merge refutations
  still grade the diff.

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
`S#` block naming the id. A standard change — a plan plus a delta — also
requires `## Log` with `L1`; light keeps it optional.

## PR and merge gates

- **No PR** until the change's plan parses: `openspec/changes/<slug>/tasks.md`
  with `## Specs` and `## Tasks`, plus `## Log` with `L1` from standard up (a
  change with a plan and a delta). `proposal.md`, `design.md` and
  `verify-report.md` are still accepted, but they are no longer required. Every
  archive is graded by that grammar whatever its folder is called, and it must
  hold the plan or the delta: an empty folder blocks.
- **Verify before archive:** a standard change records its evidence in the
  plan's `## Verification` section — one row per declared `S#`, each `PASS`
  naming the command it ran in backticks. The legacy `verify-report.md` is only
  read when the plan has no `## Verification` section. Light is advisory only.
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
- **Two enforcement points:** in `openspec` mode run the pre-archive gate
  (`--stage pre-archive`) before archive-tail and run the pre-merge guardian
  again after archive; `odd` mode grades the tree at pre-merge only. Both points
  block a standard delta that is unpromoted or unresolved. Missing `explore.md`
  is never a guardian blocker.
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
  created holding an `archived.md` marker that names the change and the date,
  and `--change` keeps the last path
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
- **Verification commands resolve:** every command a standard change records in
  `## Verification` must resolve on disk (see above). `config.yaml` is neither
  required nor graded.
- **Refutations of a `none` change:** a change with no plan, no folder and no
  archive passes unless its diff contradicts the declaration, graded at the
  pre-merge guardian and in CI. **R-a** blocks when the branch changes
  `odd/specs/**` or `openspec/specs/**` and the change carries no delta (write the
  delta and run `ai-specs specs promote`). **R-b** blocks when the branch changes
  a path the project declares in `[recipes.plan-build-flow.config]
  high_risk_paths` of `ai-specs/ai-specs.toml` — an opt-in array of path patterns
  read by the CLI and deliberately not declared in the recipe schema, so write it
  by hand; a trailing `/**` covers a prefix and anything else matches the whole
  path. In `openspec` the exit is a plan (tier light); in `odd` it is a commit in
  the range carrying a `Risk: item N (<reason>)` trailer, N in 1..6, which a bare
  `Risk: none` does not satisfy. The six items, in order: 1 data or irreversible
  effects that cannot be rebuilt (migrations, rewriting or deleting stored data,
  format changes, writing stored data without validation); 2 security (auth,
  permissions, credentials, secrets, guards, sandbox); 3 contracts others
  already consume (public API, CLI flags, config formats, exports, mirrored
  prompts; adding a flag or a command is not); 4 concurrency; 5 delivery or
  environment (installers, release, CI, deploy, dependencies); 6 no test would
  catch a regression in what changes. No diff computable means no refutation, so a
  shallow clone never blocks every small change: `gates premerge --base <ref>`
  pins the ref, and CI passes the pull request's base sha.
- **`gates premerge` flags:** `--tier none|light|standard` is a coherence
  assertion, never a choice; a value that contradicts the derived tier blocks
  naming the derived tier and both exits, and `--tier full` blocks with the
  migration message. `--base <ref>` pins the diff the refutations grade; without
  it the guardian uses the project's `[recipes.git-pr-flow.config] base_branch`,
  tried as `origin/<base>`, `<base>`, then `main` or `master`.

## Third assurance layer: pull-request CI

The merge decision is the human's, and the assurance is layered so a merge
cannot happen by hand without the archive: the CLI verb is layer one, the skill
that obeys it is layer two, and `.github/workflows/pr-gate.yml` is layer three.
On every pull request against `development` that workflow builds the binary and
runs

```bash
trusted/dist/ai-specs gates premerge \
  --root "$GITHUB_WORKSPACE/candidate" --change "$GITHUB_HEAD_REF" \
  --base "$BASE_SHA"
```

`--base` is the pull request's base sha: a CI clone may have no base ref to
resolve, so without it the refutations of a `none` change degrade to none.

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

Plans already in flight when this contract ships are renamed into the plan shape
and given the evidence their tier asks for before their PR/archive; no replan or
restart is needed. One edit is mandatory: a plan still declaring `Depth: full`
rewrites it to `Depth: standard` and keeps its spec delta, otherwise the guardian
blocks with the migration message, and a `Depth:` line that no longer matches the
artifacts is corrected or dropped. Historical archives are never rewritten. A
stale PR is handled by its owning agent, which fixes its artifacts when that
change resumes.

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
tree — `tasks.md` and, from standard up, its `## Log` and `## Verification` rows
— and never by the store selection. Engram MAY mirror artifacts but never replaces
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
