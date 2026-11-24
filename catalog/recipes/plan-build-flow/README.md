# Plan / Build Flow (Ambient)

An invisible change workflow: the bundled skill auto-invokes on substantial
requests, **classifies planning depth**, produces reviewable artifacts, waits
for authorization, then implements, validates, and closes — **without** `/plan`
or `/build` commands.

## What it provides

- **Skill `plan-build-flow`** (bundled) — depth classifier (full / standard /
  light), ambient planning/build contract, PR artifact gate, pre-merge archive
  gate, and graceful degradation without an orchestrator.
- **Doc** — this README at `ai-specs/recipes/plan-build-flow/README.md`.

## Planning depth (classifier)

| Tier | Typical use | Planning chain | Minimum artifacts before build |
|------|-------------|----------------|-------------------------------|
| **Full** | New capability, architecture, breaking or ambiguous scope | explore → proposal → spec → design → tasks | `tasks.md` plus `proposal.md` or `design.md`, and at least one `specs/**/*.md` |
| **Standard** | Scoped feature or bounded multi-file work | conditional explore → proposal → spec → tasks | `proposal.md`, `tasks.md`, and at least one `specs/**/*.md` |
| **Light** | Small fix with clear file/symbol target | proposal → tasks | `proposal.md` + `tasks.md` |

Direct "implement this" requests still run the classifier first when no change
folder exists yet.

## Full phase compatibility

Full planning uses the logical chain `explore -> proposal -> spec/design ->
tasks`. Proposal follows explore; spec and design may run in parallel only after
proposal; tasks waits for both. Each phase has explicit file-backed inputs and
outputs, and a host-advertised executor may implement only the current phase.
When no executor is advertised, the phase runs inline. An unavailable executor
may use that same fallback; malformed, partial, or blocked results stop and
preserve state rather than silently rerunning or skipping a phase.

Standard and Light behavior remains collapsed and unchanged. The recipe stays
provider-neutral and adds no runtime or model configuration.

Preflight is one session-level authority for execution mode, artifact store,
review budget, delivery strategy, and chain strategy. Plan-build consumes those
values without recollecting or overriding them. Its final artifact-derived
presentation covers intent, scope, key decisions, affected areas, risks, open
questions, and labeled recommendations/assumptions. Interactive mode asks
questions after the phase that exposed them; automatic mode records assumptions
and blocks unresolved product decisions. The final review requires explicit
accept, adjust, or stop.

The classifier always computes a signal tier and separately checks for an
explicit requested depth. Illustrative requests such as “full planning”,
“acotado con spec”, or “solo tasks” are compared with that signal; this is
guidance, not an exhaustive parser. A mismatch is a depth conflict: the agent
must ask which tier wins in either direction before writing that planning chain.
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
There is no `odd/config.yml` recipe manifest: the mode lives in the project
manifest. The project config is a separate file, `odd/config.yaml` — the same
shape and the same rules as `openspec/config.yaml`, described below.
- **`openspec`** is the unchanged OpenSpec-centered flow described below.

## Project config (`config.yaml`)

`config.yaml` configures the **project**, not the change. It survives in both
modes and is resolved per mode, never guessed: `openspec/config.yaml` in
`openspec`, `odd/config.yaml` in `odd` (`.yml` is not read).

The CLI validates only the keys it consumes, line by line, with no YAML
library:

- the file must exist and be a regular file whose real path stays inside the
  repository (a symlink pointing outside blocks);
- `schema:` must be non-empty;
- a `testing:` block must declare a non-empty `runner:` — the command that runs
  the project tests;
- `strict_tdd:`, when present, must be `true` or `false`; omitting it is fine.

Everything else — `context`, `rules.*`, `ai_specs_guidance` — is **opaque
prose** for the agent, and nothing validates it. A strict schema would repeat
the frozen schema oracle trap that already blocked declaring `[config.mode]` in
#589, so the file stays a superset the CLI reads selectively. The `tracking:`
block is read by the Tracker domain (`internal/doctor`, the ledger witness) and
must be kept intact.

### Freshness loop

The cause of a stale `config.yaml` is that nothing read it while grading. Now
it is graded, indirectly but deterministically, through the change's own
`## Verification` rows:

1. every backticked command that starts a row's evidence must resolve here — a
   `./`- or `/`-prefixed path must exist and be a regular file, a bare word must
   be found on `PATH`;
2. the `testing.runner` declared in the config must match the first word of at
   least one row.

Every `## Verification` row of a Standard or Full change must also name the
command it ran in backticks: a verdict without a command records no run, so
`- **S1: PASS** — nothing executed` blocks with `verification for S1 must name
the command it ran in backticks: record "- **S1: PASS** — `go test ./...`"`.

Standard and Full changes require both. A change with no verification rows —
Light, or a plan without `## Verification` — is exempt from both: that is the
proportional-ceremony rule (S12), not a bypass. A change that migrates the test
runner cannot pass the gate without updating `config.yaml` in the same change,
so obsolescence fails the gate instead of staying silent. Blockers name the
offending command and the config path, for example:

```text
openspec/config.yaml: verification for S1 names command "./nope.sh" which does
not exist: run the configured tests and record the real command
openspec/config.yaml: verification never runs the configured testing.runner
"./tests/run.sh": name it in at least one row or update config.yaml
```

## PR and merge gates

- **No PR** until the change folder contains its tier minimum files committed:
  Light requires `proposal.md` + `tasks.md`; Standard requires those plus a
  spec delta; Full requires `tasks.md`, `proposal.md` or `design.md`, and a
  spec delta.
- **Verify before archive:** Standard requires a dedicated `verify-report.md`
  with command, exit `0`, a valid calendar date, commit SHA, and a non-failing
  verdict. Full additionally requires strict `PASS`, `ready_for_archive: true`,
  and exactly one strict-PASS `Criterion N` mapping row for every top-level
  bullet under `## Success Criteria` in the authoritative source: `proposal.md`
  when present, otherwise `design.md`. A missing, empty, or duplicate heading
  in the authoritative source blocks Full; it never falls back to `design.md`.
  Light is advisory only.
- **Promote canonical specs before archiving:** Standard and Full deltas under
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
  or Full delta that is unpromoted or unresolved. Missing `explore.md` is never a
  guardian blocker.
- **Archive before merge** on the review branch — never after merge lands on the
  base branch. OpenSpec's canonical destination is
  `openspec/changes/archive/YYYY-MM-DD-<slug>/` with a valid ISO calendar date.
  The exact undated `archive/<slug>/` directory remains a legacy fallback only
  when no dated candidate exists. Multiple dated candidates, dated-plus-undated
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
  created holding a copy of the plan and the original plan stays where it is, so
  the odd guardian can require that archive. `--change` keeps the last path
  segment of a branch-shaped value, so CI passes `GITHUB_HEAD_REF` unchanged;
  an empty `<change>` or `--change=` is a usage error, never "omitted".
- **Tracker `archive-close` is not this archive.** Tracker item closure is a
  separate Tracker-domain checkpoint
  (`ai-specs tracker-card-gate --root <root>
  --checkpoint archive-close [slug]`). It
  never moves the change folder, never infers closure from OpenSpec archive
  state, and writes nothing. Plan Build's artifact guardian is tracker-free.
- **Project config freshness:** a Standard or Full change requires a valid
  `config.yaml` (see above), every command it records in `## Verification` must
  resolve, and its `testing.runner` must appear in at least one row.

## Third assurance layer: pull-request CI

The merge decision is the human's, and the assurance is layered so a merge
cannot happen by hand without the archive: the CLI verb is layer one, the skill
that obeys it is layer two, and `.github/workflows/pr-gate.yml` is layer three.
On every pull request against `development` that workflow builds the binary and
runs

```bash
./dist/ai-specs gates premerge --root . --change "$GITHUB_HEAD_REF"
```

`actions/checkout` leaves `HEAD` detached on `pull_request`, so the change
ladder cannot pair a branch with a change folder and the explicit `--change` is
mandatory. A branch-shaped value needs no preprocessing: `--change feat/alpha`
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
verify evidence before their PR/archive; no replan or restart is needed.
Historical archives are never rewritten. A stale PR is handled by its owning
agent, which adds the missing artifacts when that change resumes.

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
