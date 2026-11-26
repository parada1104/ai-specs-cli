---
name: plan-build-flow
description: >
  Ambient change workflow. Derive the change's tier from its artifacts, produce
  the plan and its optional delta, stop for authorization, then implement,
  validate, and close without exposing slash commands. Block PR creation without
  planning files; archive before merge.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Starting a substantial feature, fix, or refactor that will modify code or artifacts"
    - "User gives a direct implementation task without prior planning artifacts"
    - "User describes a multi-step change that needs planning before implementation"
    - "Continuing implementation after the user authorizes a plan"
    - "Opening a PR or finishing a change: validate artifacts, archive pre-merge, close out"
    - "Deciding how much planning ceremony a change needs (a plan alone or a plan with a delta)"
    - "Deciding how to run planning/build phases when no orchestrator or memory is available"
---

# Ambient Plan / Build Flow

This skill runs invisibly behind normal agent work. There are **no** `/plan` or
`/build` slash commands. Derive what each substantial request needs, write the
plan and its delta, stop for authorization, then implement, validate, and close.

## 1. What plan and build mean (internal)

| Phase | Contract |
|---|---|
| **Plan** | Derive the tier from the artifacts, write the plan and, when the change alters a rule, its delta, stop and wait for human review/authorization. Never implement production code. |
| **Build** | Implement an authorized plan, validate, ensure planning files are committed on the review branch, open PR only when the artifact gate passes, and run archive-tail **before merge**. |

Speak to the user in plain language ("here is the plan", "implementing now") —
never expose internal phase names, tier names, or slash verbs.

## 2. The tier is derived from the artifacts

There is nothing to classify and no size to estimate. In `openspec` mode the
tier of a change is read by the CLI, at the gate and at the pre-merge guardian,
from the artifacts the branch carries (`odd` mode has no tier: ODD owns planning,
Section 4.1):

| Tier | Artifacts | What the guardian grades |
|---|---|---|
| **none** | no plan, no delta, no change folder — an ODD small change | nothing: the diff is only refuted by R-a and R-b (Section 7.4) |
| **light** | a plan alone, or a delta alone | the plan grammar (`## Specs`, `## Tasks`), every `Commit` cell an ancestor of `HEAD`, and promotion of the delta when the change carries one |
| **standard** | a plan and a delta | the plan grammar, `## Log` with `L1`, one `## Verification` row per `S#`, a command that resolves, promotion parity, and every `Commit` cell an ancestor of `HEAD` |

A delta with no plan stays light: a delta never forces a plan. Whether a change
needs a plan at all is ODD's Task Size, not an estimate — the outcome is
understood, the risk is contained, and the work stays resumable from the diff.
The number of files, lines, commands or tests never decides the size, and no
line count forces a split; the ~400 authored-line figure slices work units
(Section 7.3), never a tier. A change that needs no durable task artifact writes
no plan, no folder and no delta.

`Depth: <tier>` in the plan and `--tier <tier>` on the guardian are coherence
assertions, never a choice: a value that contradicts the artifacts blocks,
naming the derived tier and both exits. Keep the line only when it matches, and
drop it freely.

```text
archive/<folder>: the tier is derived from the artifacts of the change, and they derive 'light', not the declared 'standard' — add a specs/ delta to reach standard, or drop the declaration
```

A request for the removed full tier ("full SDD", "full planning", "flujo
completo") is not a tier any more: explain that it was removed, point at the
artifacts above, and let the delta decide whether the change is light or
standard.

### Deriving the artifacts while you work

| Situation | Artifacts |
|---|---|
| A small change that leaves no durable task artifact (ODD small) | none: no plan, no folder, no delta |
| A change that needs a plan and alters no rule | the plan alone, `openspec/changes/<slug>/tasks.md` |
| A change that adds, modifies or removes an observable rule | the plan plus a delta under `specs/<domain>/spec.md` |
| A rule change that needs no plan | the delta alone |
| "Implement/build/fix now" with no artifact yet | derive first — verbs never skip the plan the change needs |

Trivial read-only questions write nothing.

### Standard explore criteria

At Standard, write `explore.md` before authorization when any of these signals
apply: **multi-approach** (two or more plausible approaches with material
trade-offs), **unknown surface** (concrete files cannot yet be named), a docs or
skill **conflict**, user **uncertainty** about the approach or location, or a
prior attempt on the same intent that failed or was reverted.

Skip explore only when concrete paths and expected behavior are known, one
obvious approach exists in a known area, and no conflict, uncertainty, or retry
signal applies. Record the decision in `tasks.md` as one line:
`Explore: skipped — <short reason>`. Missing `explore.md` is a plan-phase gap;
the archive and merge guardians never block on it.

## 3. Removed: the full tier (private)

The former **Full** tier is gone. It used to run a logical phase machine
(`explore -> proposal -> spec/design -> tasks`) with a host-advertised executor
per phase. That dispatch is no longer part of this recipe: a change declaring
`Depth: full`, or graded with `--tier full`, blocks with the migration message
that names the removed tier and `Depth: standard`. Migrate by rewriting the
`Depth:` line to `standard` and keeping the plan plus its spec delta; standard
and light remain collapsed and unchanged, and the recipe stays provider-neutral
with no runtime or model configuration.

## 3.1 Preflight composition

Preflight is one session-level authority for **execution mode**, **artifact
store**, **review budget**, **delivery strategy**, and **chain strategy**.
Plan-build consumes those resolved values and passes them through the flow; it
must never recollect or override them, and phase executors cannot replace them.

Interactive mode asks once for unresolved preflight choices, then asks only
phase-specific product questions. Automatic mode does not duplicate those
prompts; it records the resolved values and proceeds according to them. The
artifact store remains a persistence preference, while file-backed readiness and
the existing worktree, verify, archive, topology, and PR gates remain
authoritative.

## 3.2 Artifact-derived plan presentation

At each review point, derive a concise technical presentation from the
artifacts available so far:

| Field | Evidence |
|---|---|
| **Intent** | The original request and the plan's `## Specs` |
| **Scope** | The `## Specs` requirements and, when the change carries one, the delta |
| **Key decisions** | Plan prose (`## Design` when it exists) and task choices |
| **Affected areas** | The `## Tasks` rows and their paths |
| **Risks** | `explore.md`, trade-offs, and unresolved constraints |
| **Open questions** | Questions recorded by the phase that exposed them |
| **Recommendations / assumptions** | Explicitly labeled conclusions derived from the artifacts |

Interactive mode asks open questions after the phase that exposed them,
especially after explore, and then offers **accept**, **adjust**, or **stop**.
Automatic mode records recommendations as labeled assumptions or decision notes;
unresolved product decisions block rather than being silently decided. The final
plan always requires an explicit **accept**, **adjust**, or **stop**. A
recommendation may be accepted or adjusted; it is never treated as an implicit
product decision.

## 3.3 Plan/build lifecycle

- **Plan** writes the plan and, when the change alters a rule, the delta (Section 2), then **stops**.
- **Build** runs: apply -> verify -> artifact/PR gates -> archive-tail (pre-merge).

## 4. When to invoke

| Situation | Invoke plan? | Invoke build? |
|---|---|---|
| Quick question / read-only exploration | No | No |
| Substantial or direct implementation request without prior artifacts | Yes — derive the artifacts, plan, stop | No |
| User approves a pending plan ("go ahead", "implement it", "build it") | No | Yes |
| User asks to open a PR | No | Yes — but only after artifact gate (Section 7) |
| Trivial one-line fix explicitly scoped by user | Light tier — `tasks.md` plan only, then may build inline | Maybe same turn after micro-plan |

Prefer the project's native plan/review UX when available (e.g. plan mode) —
this skill supplies the artifact trail behind that surface. **All planning
artifacts MUST be written during plan mode**; production code is never
modified while planning. Do not tell the user to "run /plan" or similar —
respond to natural requests ("necesito implementar…") by writing the plan and
its delta before touching production code.

## 4.1 Companion mode (ODD owns the change)

Applies only when the repository resolves to `odd` mode: `[recipes.plan-build-flow.config] mode` in the project manifest, default `auto`, which infers `odd` when the Gentle AI state directory exists at `<git-common-dir>/gentle-ai/`. An installed tool or a leftover `odd/tasks/` file is not evidence. In `openspec` mode, Sections 2-7 apply unchanged.

- ODD owns planning, approval and verification. Do not classify a tier, write a second proposal/design/tasks, or ask for another go-ahead. Tracker and delivery obligations still apply.
- The change plan is `odd/tasks/<change>.md`, where `<change>` is the last path segment of the branch name (`feat/thing` and `fix/thing` both resolve to `thing`). No guardian demands it: a small ODD change writes none, and the pre-tool-use gate requires no plan for a production write.
- The one protected odd path is the canonical spec tree `odd/specs/`, because promotion is its only writer. A hand write there is refused unless the branch pairs a change that carries a delta under `odd/changes/<change>/specs/`, active or already archived; the refusal names the missing delta and `ai-specs specs promote`. Every other write under `odd/`, and every write under `openspec/changes/`, is allowed.
- Write a delta at `odd/changes/<change>/specs/<domain>/spec.md` only when the change adds, modifies or removes an observable product rule, or corrects a documented rule to match verified behavior. Label a documentation correction as such, never as a behavior change. A behavior-preserving change carries no delta and no change folder.
- Reuse ODD verification as the evidence, then promote with `ai-specs specs promote <slug> --root <root>` before the guardian and merge (Section 7). In `odd` mode it composes `odd/changes/<slug>/specs/<domain>/spec.md` into `odd/specs/<domain>/spec.md`; `openspec/specs/**` is neither read nor written.
- In companion mode the OpenSpec plan shape, the `## Verification` rows and the tier are not graded at all: ODD owns the plan's shape and the evidence. The delta is the only artifact the guardian reads. Sections 4.2, 6 and 7 apply in `odd` only where they talk about that delta.
- The pre-merge guardian reads the branch pairing and grades only what the change's folder and delta say: no active `odd/changes/<change>/` may remain when the change carries one, the dated archive must hold the change, and every delta must be promoted into `odd/specs/`. It never reads the plan: ODD owns that document's shape and its evidence. A change with no plan, no folder and no archive is `none` and passes with `OK (none, change <change>)` unless the diff refutes it (Section 7.4).
- Archiving is mechanical when the change carries a folder: `ai-specs changes archive <change> --root <planning-root>` moves only `odd/changes/<change>/` to `odd/changes/archive/YYYY-MM-DD-<change>/`; the plan stays at `odd/tasks/<change>.md` because it belongs to the feature, not to the change. The verb also accepts a change with no folder: it creates the dated folder holding an `archived.md` marker naming the change and the date, and leaves the plan where it is. Without a folder, nothing requires that archive.

### 4.2 The plan file (`openspec` mode, normative)

The plan of a change is one file, `openspec/changes/<slug>/tasks.md`, and the CLI
parses it. Its sections, in this order, are `## Specs`, `## Tasks`, `## Design`
(optional), `## Verification`, and `## Log` last; `## Verification` and
`## Log` are graded from standard up. `S#` quotes the human request
verbatim — it never restates `proposal.md` — and every `## Tasks` row links the
`S#` it implements. `proposal.md`, `design.md` and `verify-report.md` may still
be written, but the plan is what the CLI reads. Any other section is prose the
CLI ignores.

```text
# <change>                            (2-3 header lines: what, delta, card)
## Specs
- **S1.** <requirement, one line> — *"<verbatim quote from the human>"*
## Tasks
| ID | Spec | Tarea | Ruta | Commit |
| T1 | S1, S2 | <what is done + its tests> | worker | pending |
## Design                              (optional, only if a decision needs explaining)
## Verification                        (standard: one row per S#)
- **S1: PASS** — `<command>`, exit 0, commit `<sha>`, `<date>`
## Log
- **L1** — <the human's original request, verbatim>
```

`## Log` with `L1` is required at standard — a plan plus a delta — and optional
for light. A standard change with no `## Log` blocks with
``archive/<folder>/tasks.md: `## Log` section is missing: add `- **L1** — <the human request>` because tier standard requires it``.

Migration: an in-flight change written before this release must be rewritten
into the plan shape before it can be archived.

## 5. Orchestrator degradation policy

- **Orchestrator present** — let it drive phases via sub-agents.
- **Orchestrator absent** — run equivalent phases inline in one conversation.

## 6. Persistence and readiness

Persistence is separate from readiness. `artifact_store_default` is an
external-session persistence preference rendered into the generated brief; it
never proves readiness.

- **Persistent memory present** — Engram MAY mirror planning artifacts, but a
  memory-only presence never satisfies any readiness check.
- **Absent** — fall back to file artifacts under `openspec/changes/<slug>/`.
- **Default** — when Engram is available but no preflight resolved a store,
  artifacts are written as files under `openspec/changes/<slug>/`, never
  memory-only.

Readiness is always proven by file-backed artifacts in the canonical
change-folder tree, and the readiness artifact is the plan: `tasks.md` with its
`## Specs` and `## Tasks` sections (Section 4.2), plus, from standard up, its
`## Log` and one `## Verification` row per `S#`. There are no per-tier file
minima beyond that. `proposal.md` alone is never the readiness marker. The resolved store (`openspec|engram|both`) is never
consulted for readiness and never alters any artifact gate, PR/archive gate,
staged verify gate, or pre-merge guardian decision. Engram MAY mirror artifacts
but never replaces them.

## 7. Artifact, PR, and merge gates

### 7.1 Change folder required

A change that carries a plan MUST have a slug folder at `openspec/changes/<slug>/`
(excluding `archive/`) before implementation begins on that change. If the user
jumps straight to "implement X" and the change needs a plan, create the folder
and write the plan first. That refusal is not a dead end: if the change genuinely
needs no plan, run `ai-specs changes start --none --root <planning-root>` and
retry the write instead of writing a plan nobody needs. A change that needs no
plan (`none`) creates no folder and passes unless its diff is refuted
(Section 7.4).

The `openspec` hook accepts a local declaration that the branch needs no plan:
`ai-specs changes start --none --root <planning-root>` writes
`branch.<branch>.aiSpecsCeremony=none` into this clone's local git config. It is
never an artifact, never travels with the branch and is never committed, and it
relaxes only that plan requirement: the ledger `work-start` checkpoint still
runs, the mode is untouched, and the pre-merge refutations still grade the diff.

Exception — `odd` mode (Section 4.1): the plan is `odd/tasks/<change>.md` and no
`openspec/changes/<slug>/` folder is created. A change that carries a delta keeps
it at `odd/changes/<change>/specs/<domain>/spec.md`, which serves as the change
folder for the tail gates.

### 7.2 PR creation gate (hard stop)

Do **not** run `gh pr create`, `glab mr create`, or equivalent until:

1. The matching `openspec/changes/<slug>/` folder exists on the current branch
   with the plan from Section 4.2, or with the delta when the change carries no
   plan. In `odd` mode (Section 4.1) that folder is not required: the gate is
   satisfied by `odd/tasks/<change>.md` and, when the change carries a delta, by
   `odd/changes/<change>/`.
2. The plan and, when the change carries one, its delta are present and
   **committed**. `proposal.md`, `design.md` and `verify-report.md` are still
   accepted and honoured when present; they are not required.
3. Implementation and verification for that slug are complete (or the user
   explicitly accepts opening a draft-only PR for review of planning).

If the gate fails, stop with a plain-language blocker: complete the artifacts the
change needs, commit the files, then retry PR creation.

### 7.3 Pre-merge archive gate (hard stop)

Archive-tail MUST run on the **review branch** before merge — never defer until
after the merge lands on the base branch, in both modes. Standard changes
require conforming verification evidence before this step; light verification is
advisory.

The archive move itself is mechanical and mandatory before merge whenever the
change carries a change folder, in both modes:
`ai-specs changes archive [<slug>] --root <planning-root>` (optional
`--date YYYY-MM-DD`, default today in UTC). It refuses while a delta is
unpromoted, refuses when a dated archive for the change already exists, refuses
a destination that resolves through a symlink or outside the repository, and
never deletes anything. A change with no delta folder is still archived: the
dated folder is created holding an `archived.md` marker that names the change
and the date, and the original plan stays where it is. `--change`
accepts a branch-shaped value — CI passes `GITHUB_HEAD_REF` — and keeps its last
path segment, the same normalization the branch pairing applies. An empty
`<slug>` or an explicitly empty `--change=` is a usage error, never "omitted".
A change name that is not a single path segment (`..`, `../../../offtree`,
`a/b`) is a usage error too: the guardian exits `2` with
`ERROR: invalid change name: <name> (expected a single path segment)` and never
grades a folder outside the change tree. A change with no plan, no folder and no
archive is `none`: it owes no archive and passes unless its diff is refuted.

The evidence artifact is the plan's `## Verification` section (Section 4.2): one
row per `S#` declared under `## Specs`, in the same file.

```markdown
## Verification
- **S1: PASS** — `go test ./internal/gates/...`, exit 0, commit `604a441`, 2026-08-07
- **S2: N/A** — the CLI cannot judge this requirement, and the reason is why
```

Standard requires one row — `PASS`, `FAIL` or `N/A` — for every declared `S#`;
`N/A` must carry its reason. Every `PASS` row must also record exactly one
command in backticks, with no prose before it and no bare second backticked
value beside it, because a verdict without a command records no run: a row like
`- **S1: PASS** — nothing executed` blocks with
`verification for S1 must name the command it ran in backticks: record "- **S1: PASS** — `go test ./...`"`.
A row that records `exit 1`, `exit 2` or `failed` blocks as a failing run, and
two rows for the same `S#` block naming the id. An `N/A` row is exempt from the
command requirement because its reason replaces the run. Any `FAIL` blocks.
`ready_for_archive` is not part of the plan artifact: coverage replaces it there.

`verify-report.md` remains **accepted as the legacy evidence artifact**, graded
exactly as before: a dedicated report whose canonical block is

```markdown
## Verify evidence
- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-07
- Commit: 604a441
- ready_for_archive: true

## Success-criteria mapping
- Criterion 1: PASS — concise evidence for the first criterion
- Criterion 2: PASS — concise evidence for the second criterion
```

On that legacy source, Standard requires the dedicated report with a non-failing
verdict, command, exit `0`, date, and 7–40 character commit SHA. The report is
read only when the plan has no `## Verification` section at all; a present but
unparseable section is a blocker, never a fallback. `S1` and
`Criterion 1` are interchangeable labels, so an old mapping row keeps satisfying
a plan requirement. Light may warn when evidence is absent but MUST NOT block
solely for it.

Enforce the evidence gate twice: before archive-tail and again in the guardian
before merge. There is no bypass flag. Labels may use `Status`/`Overall`,
`Exit code`/`Exit status`, and `SHA`/`Revision` synonyms.
Verification enforcement is advisory for Light and blocking for Standard at
both stages.

Sequence on the review branch:

1. Implement and verify.
2. Commit and push, one commit per `## Tasks` row, or one per work unit when a
   row runs past roughly 400 authored changed lines. That figure is a planning
   heuristic for slicing: it is never a cap, a counter-trigger or a forced
   split, and a single reviewable unit may exceed it with its reason stated.
   Every commit is a Conventional Commit, and tests and docs land in the same
   commit as the behavior they cover. Write each commit's sha into the `Commit`
   cell of its row. When native review is on, grade each work-unit commit as it
   lands — `assess --base-ref <last reviewed boundary> --committed-only` — so a
   candidate never grows into the accumulated branch.
3. Open a PR (artifact gate satisfied).
4. Promote canonical specs. Compose every delta under
   `openspec/changes/<slug>/specs/<domain>/spec.md` into
   `openspec/specs/<domain>/spec.md` with the promotion helper (the
   `ai-specs specs promote` verb) and commit the result on the review branch:

```bash
ai-specs specs promote <slug> --root <planning-root>
```

   Composition follows the archive contract: `## ADDED Requirements` appends,
   `## MODIFIED Requirements` replaces the full canonical block with the exact
   same requirement name, unrelated canonical requirements and sections survive,
   a missing canonical domain is created from the delta, and a rerun after an
   interruption is a no-op. A colliding `ADDED`, a `MODIFIED` target that does
   not exist, or an unsupported `## RENAMED Requirements` delta blocks without
   writing. A destructive `## REMOVED Requirements` delta blocks unless the
   removal was explicitly approved, in which case re-run with `--allow-removed`.
   Only the promoter writes canonical specs; the guardian only validates them
   and never mutates `openspec/specs/`. Light and changes with no `specs/`
   deltas have nothing to promote.

5. In `openspec` mode, before moving the change folder, run the executable
pre-archive gate and stop if it exits nonzero. `--root` is the resolved planning
root from the request context — required, never the process cwd; a subrepo
request passes the proven superproject root:

```bash
ai-specs gates premerge <slug> --root <planning-root> --stage pre-archive
```

The command must pass for Standard; do not archive or continue when it
reports missing or failed evidence, or an unpromoted or unresolved delta. Light
remains advisory. This is the pre-archive check; the pre-merge guardian below
remains required after archive. `--stage pre-archive` grades the OpenSpec tree,
so `odd` mode skips it and goes straight to archiving the change folder.

6. After the pre-archive gate passes, run archive-tail with the verb:
   `ai-specs changes archive <slug> --root <planning-root>` moves
   `openspec/changes/<slug>/` (plan included) →
   `openspec/changes/archive/YYYY-MM-DD-<slug>/`, defaulting to today in UTC and
   validating `--date`. In `odd` mode only `odd/changes/<slug>/` moves and
   `odd/tasks/<slug>.md` stays as the feature record. Then commit and push to
   the review branch.
   The exact undated `openspec/changes/archive/<slug>/` form remains readable
   only as a legacy fallback for historical archives; new archives use the
   dated provider form. The guardian fails closed when multiple dated
   candidates, dated-plus-undated candidates, invalid dates, or near-match
   names are present.
7. Run the pre-merge guardian; merge only after you have asked the user whether
   it is time to go and they explicitly approved. Pass `--base <ref>` when the
   base ref does not resolve here, so the refutations of a `none` change are
   graded against the right diff.

The commit check has two declared limits. It holds only on the review branch,
because a squash-merge rewrites the shas and the guardian never runs on the base
branch. And the CLI grades the recorded sha, never whether a slice was a
reasonable size: the granularity stays in prose, and only the record is graded.

#### Project config (`openspec/config.yaml`) — optional and ungraded

`openspec/config.yaml` is an optional project artefact, and the premerge neither
requires nor grades it: the CLI does not depend on the OpenSpec CLI, so a missing
file never blocks and its `schema:`, `testing.runner:` and `strict_tdd:` keys are
not validated. `odd` mode has no config file and never grades one. OpenSpec and
agents may still read `context`, `rules.*` and `ai_specs_guidance`; the `tracking:`
block is read by the Tracker domain (`internal/doctor`, the ledger witness) and
must be kept intact.

One rule survives, over this change's own `## Verification` rows and graded from
Standard up: every command a row records must resolve here — a relative or
absolute path must exist on disk, a bare word must be on `PATH`. The blocker
names the plan that records the command:

```text
openspec/changes/archive/<slug>/tasks.md: verification for S1 names command "./nope.sh" which does not exist: record the real command
```

A change with no rows, or with no command in any row, is exempt: that is the
Light ceremony reduction, not a loophole.

### 7.4 Pre-merge merge guardian (hard stop)

Before `gh pr merge` / `glab mr merge` / Bitbucket merge (or equivalent), verify
the archive is complete. Prefer the shared helper when available. `--root` is
the propagated planning root from the request context (required, never the
process cwd): for a subrepo-context change whose planning root is the proven
superproject, pass `<super>/openspec`'s parent — the superproject root — so the
guardian inspects the canonical tree and never a subrepo-local change folder.

```bash
ai-specs gates premerge <slug> --root <planning-root>
# optional: --tier none|light|standard   (coherence assertion, never a choice)
# optional: --base <ref>                (ref the objective refutations grade against)
```

`--tier` is graded against the tier the artifacts derive (Section 2): a value
that contradicts them blocks naming the derived tier and both exits, and
`--tier full` blocks with the migration message. `--base` pins the ref the
diff refutations of a `none` change are graded against; without it the guardian
uses the project's `[recipes.git-pr-flow.config] base_branch`, resolved as
`origin/<base>`, `<base>`, then `main` or `master`.

The helper ships with the CLI install under `~/.ai-specs` (not copied into
consumer projects).

Hard blockers (do **not** merge):

1. `openspec/changes/<slug>/` still exists (active, not archived).
2. No exact valid archive candidate exists at
   `openspec/changes/archive/YYYY-MM-DD-<slug>/`; the exact undated
   `openspec/changes/archive/<slug>/` form is accepted only as a legacy
   fallback when no dated candidate exists, is graded by the same plan grammar,
   and must hold the plan or the delta like any other archive.
3. The archive holds nothing of the change — its plan, its delta, or the
   `archived.md` marker — or its `tasks.md` does not parse as the plan, or a
   standard change has no `## Log` with `L1`. There are no per-tier file minima
   beyond the plan's own grammar. `proposal.md`, `design.md` and
   `verify-report.md` are accepted and honoured when present, but they are not
   required.
4. Standard verification is incomplete: a declared `S#` has no row in
   `## Verification`, or a row is `FAIL`.
5. A Standard delta under `specs/<domain>/spec.md` is not promoted into
   `openspec/specs/<domain>/spec.md`, or cannot be composed (collision, missing
   `MODIFIED` target, unsupported `RENAMED`, destructive `REMOVED` without
   explicit approval). Run the promoter first; the guardian only validates and
   never writes canonical specs or tracker state.
6. A plan declaring the removed full tier (`Depth: full`, or `--tier full`)
   blocks with the migration message that points at `Depth: standard`; it is
   never downgraded silently.
7. Standard: a verification row names a command that does not resolve. The
   blocker names the plan and the offending command; `openspec/config.yaml` is
   neither required nor graded.

The promotion check is delta-scoped: a light change and a change with no
`specs/` deltas are never blocked by it.

The plan grammar is graded on **every** archive, whatever its folder is called:
the folder name never decides which rules apply, so an undated `archive/<slug>/`
is graded exactly like a dated one. An archive also has to prove it holds
something of the change — its delta, its plan, or the `archived.md` marker; an
empty folder, dated or not, blocks with
`archive candidate <path> is empty: an archive must hold the change's plan (tasks.md) or its delta (specs/**/*.md)`.
An archive that predates this contract and still has a free-form `tasks.md`
must be re-worded into the plan shape if anyone needs to re-run the guardian on
it; historical archives are otherwise never rewritten.

In `odd` mode the guardian grades the ODD tree instead (Section 4.1): an active
`odd/changes/<change>/` folder must be gone, the dated archive must exist and
hold the plan, the delta, or the `archived.md` marker, and every
`odd/changes/<change>/specs/` delta must be promoted into `odd/specs/` — read
from the archived folder once the change is archived, so a delta that is not
promoted blocks after archiving with the same "is not promoted into
odd/specs/..." wording. It never grades `odd/tasks/<change>.md`: ODD owns that
document's shape and its evidence. The archive is owed only by a change that
carries a folder, in both
modes; a change with no folder and no archive is `none` and passes with
`OK (none, change <change>)`. Because `odd/tasks/<change>.md`
survives the archive, the branch still pairs the change after archiving; in
`openspec` pass the slug or `--change`, since the plan moved into the archive
with the change folder.

### Refutations of a `none` change

The CLI cannot prove an ODD small task, so a change with no plan, no folder and
no archive is a valid declaration that passes. Only a contradiction of the diff
refutes it, graded at the pre-merge guardian and in CI, never at the gate:

- **R-a** — the branch changes `odd/specs/**` or `openspec/specs/**` and the
  change carries no delta, active or archived. That is a promotion bypass.
  Exit: write the delta under `<mode>/changes/<change>/specs/` and run
  `ai-specs specs promote`. The blocker names the canonical spec, the change and
  the delta path.
- **R-b** — the branch changes a path the project declares as high-risk. The
  list is opt-in and project-owned: `[recipes.plan-build-flow.config]
  high_risk_paths` in `ai-specs/ai-specs.toml`, an array of path patterns where a
  trailing `/**` covers a prefix and anything else is a whole-path shell-style
  match. The key is not declared in the recipe schema, so write it by hand; a
  project that declares nothing is never refused by R-b. The response is
  per-mode: in `openspec` the change must carry a plan (tier light), because the
  plan is where the verification evidence lives; in `odd` one commit of the
  range must carry a `Risk: item N (<reason>)` trailer, N in 1..6 — a bare
  `Risk: none` does not satisfy it. The six items are: 1 data or irreversible
  effects that cannot be rebuilt (migrations, rewriting or deleting stored data,
  format changes, writing stored data without validation); 2 security (auth,
  permissions, credentials, secrets, guards, sandbox); 3 contracts others
  already consume (public API, CLI flags, config formats, exports, mirrored
  prompts — adding a flag or a command is not); 4 concurrency; 5 delivery or
  environment (installers, release, CI, deploy, dependencies); 6 no test would
  catch a regression in what changes.

No diff computable means no refutation: without a base ref, in a shallow clone
or outside a repository, the guardian refutes nothing rather than blocking every
small change. `--base <ref>` pins it, and CI passes the pull request's base sha.

Tracker item closure (`ai-specs tracker-card-gate --root <root> --checkpoint
archive-close`) is a separate Tracker-domain checkpoint: it is not this OpenSpec
archive, it never moves the change folder, and this guardian never grades or
writes tracker state.

### 7.5 The layer above the agent: pull-request CI

The guardian has three assurance layers, and the agent is only the second one.
The third is `.github/workflows/pr-gate.yml`: on every pull request against
`development` it builds the CLI and runs

```bash
trusted/dist/ai-specs gates premerge \
  --root "$GITHUB_WORKSPACE/candidate" --change "$GITHUB_HEAD_REF" \
  --base "$BASE_SHA"
```

`--base` is the pull request's base sha: a CI clone may have no base ref to
resolve, so without it the refutations degrade to none.

The workflow triggers on `pull_request_target`: definition and evaluator come
from the base branch, and `candidate/` is data only.
`actions/checkout` leaves `HEAD` detached, so the gate cannot
derive the change from the branch and the explicit `--change` is mandatory. A
branch-shaped value is accepted as is: `--change feat/alpha` keeps its last path
segment (`alpha`), so `GITHUB_HEAD_REF` needs no preprocessing. Making that job a **required** status check is a repository setting, not a file
in this repo — see `catalog/recipes/plan-build-flow/README.md` for the exact
`gh api` command the owner runs once. The workflow grades committed state only,
so the archive commit must be pushed for the check to pass.

### In-flight plans and stale PRs

Plans already in flight when this contract ships are grandfathered through
their current work: a plan that no longer matches the grammar is rewritten before
its PR or archive, without restarting the plan. The one mandatory edit is the
removed tier: a plan still declaring `Depth: full` rewrites it to
`Depth: standard` and keeps its spec delta, otherwise the guardian blocks with
the migration message; a `Depth:` line that no longer matches the artifacts is
either corrected or dropped.
Historical archives are never rewritten; a stale PR is handled by its owning
agent when that change resumes.
Missing `explore.md` is never a guardian blocker. The guardian evaluates only
the slug under check; older archived changes and stale PRs are not rewritten.

Post-merge archive is **rejected** — if merge already happened, do not treat the
merged base branch as the archive boundary.

## 8. Change-slug derivation

- The change is derived, never guessed: on build it is the last path segment of
  the current branch (`feat/thing` and `fix/thing` both give `thing`), and
  `--change` is the only override.
- Ambiguity is a refusal, not a guess: when the branch pairs with no active
  change and more than one is active, the CLI stops and names the rule and the
  candidates instead of picking one.
- On plan, derive a kebab-case slug from intent and create the folder; that is
  the one step where the slug is written rather than resolved from the branch.

## 9. Worktree deference

- Plan artifacts may be written before a worktree exists when small.
- Build **must** use a dedicated worktree when `worktree-flow` is enabled.

## 10. Cross-repository planning boundaries

- For a recognized initialized submodule linked worktree in the supported shared
  layout, derive the planning root from repository **topology** and use the
  containing **superproject** as the central source of truth.
- The canonical `openspec/changes/<slug>/` tree belongs to that superproject.
  Central active-plan lookup and planning-artifact writes are allowed only under
  `openspec/changes/**`, including its archive subtree; this is not a bypass for
  superproject production paths.
- **Standalone** repositories and non-submodule worktrees retain nearest-root
  behavior. If the relationship cannot be established, resolution is **fail-safe**
  and falls back to that nearest repository rather than guessing a parent.
- Keep one canonical plan: no duplication, synchronization, or orchestration
  across subrepositories, and no worktree, branch, or pull-request side effects.

## 11. Archive-tail graceful no-op

Archive-tail runs at step 6 of Section 7.3 (before merge):

- **Change-folder close** — move `openspec/changes/<slug>/` →
  `openspec/changes/archive/YYYY-MM-DD-<slug>/` using a valid ISO calendar date
  (required for new archives). The exact undated
  `openspec/changes/archive/<slug>/` form is a legacy fallback only.
- **Vault summary** — no-op with note if canonical store absent.
- **Tracker comment** — no-op with note if tracker absent.
