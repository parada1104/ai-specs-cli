---
name: plan-build-flow
description: >
  Ambient change workflow. Classify planning depth, produce reviewable artifacts,
  stop for authorization, then implement, validate, and close without exposing
  slash commands. Block PR creation without planning files; archive before merge.
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
    - "Deciding how much planning ceremony a change needs (full vs spec+tasks vs tasks-only)"
    - "Deciding how to run planning/build phases when no orchestrator or memory is available"
---

# Ambient Plan / Build Flow

This skill runs invisibly behind normal agent work. There are **no** `/plan` or
`/build` slash commands. Classify every substantial request, plan to the
appropriate depth, stop for authorization, then implement, validate, and close.

## 1. What plan and build mean (internal)

| Phase | Contract |
|---|---|
| **Plan** | Classify depth, write the minimum reviewable planning artifacts for that tier, stop and wait for human review/authorization. Never implement production code. |
| **Build** | Implement an authorized plan, validate, ensure planning files are committed on the review branch, open PR only when the artifact gate passes, and run archive-tail **before merge**. |

Speak to the user in plain language ("here is the plan", "implementing now") —
never expose internal phase names, tier names, or slash verbs.

## 2. Change depth classifier

Before any production edit, classify the request into exactly one tier. Always
compute the signal tier from size and scope, then separately detect an explicit
depth request when the user names a tier or a clearly equivalent planning depth.
Record the decided tier in `tasks.md` as one standalone lowercase line, for
example `Depth: standard`. Downgrade is allowed when new facts appear; upgrade
when scope grows.

Illustrative request phrases include (this is an illustrative set, not an
exhaustive parser or fixed token whitelist):

- **Full** — "full SDD", "full planning", "flujo completo", or
  "planificación completa".
- **Standard** — "standard", "spec + tasks", or "acotado con spec".
- **Light** — "light", "tasks only", "solo tasks", or "solo tareas".

The explicit request is compared with the signal; it never replaces signal
classification. If they match, proceed with that shared tier. If they differ,
this is a **depth conflict**: do not silently adopt either side. Ask the user
which value wins before writing planning artifacts for the decided tier. The ask
must fire in both directions, including when the user requests a deeper tier
than the signal suggests; it may recommend a tier, but the user decides.

The ask is required unless the same user turn already states which side wins,
such as "use full even if it looks standard". A same-turn resolution adopts the
stated value without a second ask and records `Decision source: user`. Merely
restating the requested tier or adding scope detail does not resolve the
conflict. Until the conflict is resolved, do not implement production code or
pretend that a tier is decided. With no explicit request, preserve signal-only
classification and proceed without conflict handling.

Whenever a conflict was detected, annotate `tasks.md` with the following labels
on separate lines. Keep the `Depth:` line standalone; never append annotation to
it:

Depth: full

Requested depth: full
Signal depth: standard
Decided depth: full
Decision source: user

`Decided depth` MUST equal the tier on the `Depth:` line. Use
`Decision source: user` whenever a human chose, including same-turn resolution.
When there is no conflict, the ordinary standalone `Depth:` line remains
sufficient and no conflict annotation is required.
The annotation labels are prefixed so existing tier inference sees exactly one
`Depth:` line. If conflict resolution selects a deeper tier than the signal,
complete the entire planning chain required by that decided tier before build
authorization counts as satisfied; never relabel a shallower artifact set as a
deeper chain.

| Tier | When to use | Planning chain (private) | Minimum artifacts before build |
|---|---|---|---|
| **Full** | New capability, architecture or cross-cutting refactor, breaking change, ambiguous scope, or user cannot point to concrete files | explore → proposal → spec → design → tasks | `tasks.md` plus `proposal.md` or `design.md`, and at least one spec delta under `specs/` |
| **Standard** | Scoped feature or multi-file fix in a known area; intent is clear but needs written requirements | conditional explore → proposal → spec → tasks | `proposal.md`, `tasks.md`, and at least one spec delta under `specs/`; explore when criteria below fire |
| **Light** | Small bugfix, single-file tweak, typo, or user names exact file(s) and expected edit | proposal → tasks | `proposal.md` and `tasks.md` |

### Standard and Full explore criteria

At Standard, write `explore.md` before authorization when any of these signals
apply: **multi-approach** (two or more plausible approaches with material
trade-offs), **unknown surface** (concrete files cannot yet be named), a docs or
skill **conflict**, user **uncertainty** about the approach or location, or a
prior attempt on the same intent that failed or was reverted. Full keeps
`explore.md` first in its chain.

Skip explore only when concrete paths and expected behavior are known, one
obvious approach exists in a known area, and no conflict, uncertainty, or retry
signal applies. Record the decision in `tasks.md` as one line:
`Explore: skipped — <short reason>`. Missing `explore.md` is a plan-phase gap;
the archive and merge guardians never block on it.

### Classifier signals (quick)

| Signal | Tier |
|---|---|
| "Add OAuth", "redesign sync", "new recipe", "breaking v3" | Full |
| "Add field to X", "extend handler for Y", bounded multi-file feature | Standard |
| "Fix typo in README", "change default in config.toml line 12", one-line bug | Light |
| User says "implement/build/fix now" but **no** `openspec/changes/<slug>/` exists yet | Classify first — verbs do **not** skip planning |
| User says "go ahead" and a matching change folder already exists | Build (tier already chosen) |

Trivial read-only questions skip the classifier entirely.

## 3. Full phase compatibility (private)

Full planning runs these logical phases in order:

`explore -> proposal -> spec/design -> tasks`

| Phase | Required inputs | Output artifact | Dependency |
|---|---|---|---|
| **explore** | User intent, repository context, and known constraints | `explore.md` | None |
| **proposal** | `explore.md` plus the original intent | `proposal.md` | After explore |
| **spec** | `proposal.md` | `specs/**/*.md` | After proposal |
| **design** | `proposal.md` | `design.md` | After proposal |
| **tasks** | `proposal.md`, the spec delta, and `design.md` | `tasks.md` | After both spec and design |

Spec and design may run in parallel only after proposal is complete. Tasks waits
for both outputs. Each phase owns its output artifact and must leave prior
artifacts intact.

The host may advertise a **host-advertised executor** for the **current** logical phase. The
advertisement is an optional capability boundary consisting of the phase name,
the required input artifacts, and the output artifact it can produce. The
recipe remains provider-neutral: it does not select a runtime, model, or
external execution service.

Dispatch one phase at a time. If no phase executor is advertised, run the
current phase inline. An advertised executor that is unavailable may also use
the inline fallback. A complete result is accepted only after its output is
present and matches the current phase contract.

Malformed, partial, or blocked executor results are a stop condition. Stop and
preserve all existing artifacts and state, report the blocked phase and result status,
and wait for a decision. Do not silently rerun the executor, accept an
incomplete artifact, or skip the phase. Do not skip phases because an earlier
executor was unavailable; inline fallback still completes that phase.

This phase compatibility applies only to Full. Standard and Light remain
collapsed and unchanged; they do not acquire Full's phase dispatch.

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
| **Intent** | Original request and `proposal.md` |
| **Scope** | `proposal.md` in-scope and out-of-scope boundaries |
| **Key decisions** | Proposal, spec, design, and task choices |
| **Affected areas** | Proposal/design paths and task work units |
| **Risks** | `explore.md`, design trade-offs, and unresolved constraints |
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

- **Plan** runs the chain for the classified tier (Section 2), then **stops**.
- **Build** runs: apply -> verify -> artifact/PR gates -> archive-tail (pre-merge).

## 4. When to invoke

| Situation | Invoke plan? | Invoke build? |
|---|---|---|
| Quick question / read-only exploration | No | No |
| Substantial or direct implementation request without prior change folder | Yes — classify, plan, stop | No |
| User approves a pending plan ("go ahead", "implement it", "build it") | No | Yes |
| User asks to open a PR | No | Yes — but only after artifact gate (Section 7) |
| Trivial one-line fix explicitly scoped by user | Light tier — plan (`proposal.md` + `tasks.md`) then may build inline | Maybe same turn after micro-plan |

Prefer the project's native plan/review UX when available (e.g. plan mode) —
this skill supplies the artifact trail behind that surface. **All classified
planning artifacts MUST be written during plan mode**; production code is never
modified while planning. Do not tell the user to "run /plan" or similar —
respond to natural requests ("necesito implementar…") by classifying and
planning.

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
change-folder tree — `tasks.md`, tier minimum planning files, and
`verify-report.md`. The resolved store (`openspec|engram|both`) is never
consulted for readiness and never alters any classifier, PR/archive gate,
staged verify gate, or pre-merge guardian decision. Engram MAY mirror artifacts
but never replaces them.

## 7. Artifact, PR, and merge gates

### 7.1 Change folder required

Every non-trivial change MUST have a slug folder at `openspec/changes/<slug>/`
(excluding `archive/`) before implementation begins on that change. If the user
jumps straight to "implement X", create the folder and run the classified plan
first.

## Commit consent (hard rule)

The flow never commits or pushes on its own. `git commit`, `git push`, `git
merge`, and any command that writes history are **user-authorized actions**, not
steps the flow performs to satisfy its own gates.

Whenever a step below says files must be committed, that means: **propose** the
commit and wait. Present what would be committed — the file list, the branch, and
the message you intend to use — as a proposal, then stop. Only run it after the
user accepts. If the user does not answer, the work stays uncommitted; that is a
valid resting state, and reporting "committed" for anything they did not approve
is a false report.

This applies even when a gate is blocking. A gate that requires committed files
is satisfied by asking the user, never by committing to unblock yourself. Never
infer standing consent from an earlier approval: each commit is its own ask.

### 7.2 PR creation gate (hard stop)

Do **not** run `gh pr create`, `glab mr create`, or equivalent until:

1. The matching `openspec/changes/<slug>/` folder exists on the current branch.
2. Tier minimum files from Section 2 are present and **committed**.
3. Implementation and verification for that slug are complete (or the user
   explicitly accepts opening a draft-only PR for review of planning — still
   requires the tier minimum files).

If the gate fails, stop with a plain-language blocker: complete planning for
the classified depth, ask the user to authorize committing those files, and retry
PR creation only after they accept.

### 7.3 Pre-merge archive gate (hard stop)

Archive-tail MUST run on the **review branch** before merge — never defer until
after the merge lands on the base branch. Standard and Full changes require a
conforming `verify-report.md` before this step; Light verification is advisory.

The canonical evidence block in `verify-report.md` is:

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

Standard requires the dedicated report with a non-failing verdict, command,
exit `0`, date, and 7–40 character commit SHA. Full additionally requires
strict `PASS` and `ready_for_archive: true`, plus exactly one `Criterion N: PASS`
mapping row for every top-level bullet under `## Success Criteria` in the
authoritative source: `proposal.md` when present, otherwise `design.md`.
Missing or empty criteria in an existing `proposal.md` block Full; the guardian
does not fall back to `design.md`. Duplicate `## Success Criteria` headings in
the authoritative source are also rejected. Criterion numbers are 1-based and
contiguous; duplicate mapping rows, missing numbers, unknown numbers, and
non-PASS mapping statuses block Full. The guardian enforces this deterministic
mapping at both gates. Light may warn when evidence is absent but MUST NOT block
solely for it.

Enforce the evidence gate twice: before archive-tail and again in the guardian
before merge. There is no bypass flag. Labels may use `Status`/`Overall`,
`Exit code`/`Exit status`, and `SHA`/`Revision` synonyms.
Verification enforcement is advisory for Light, blocking for Standard, and
required for Full at both stages.

Sequence on the review branch:

1. Implement and verify.
2. Propose committing and pushing implementation **and** planning files, and wait
   for the user to accept. Do not proceed to step 3 on your own if they decline —
   the change simply stays local.
3. Open a PR (artifact gate satisfied), once the user has authorized the push.
Before moving the change folder, run the executable pre-archive gate and stop
if it exits nonzero. `--root` is the resolved planning root from the request
context — required, never the process cwd; a subrepo request passes the proven
superproject root:

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py" \
  <slug> --root <planning-root> --stage pre-archive
```

The command must pass for Standard and Full; do not archive or continue when it
reports missing or failed evidence. Light remains advisory. This is the
pre-archive check; the pre-merge guardian below remains required after archive.

4. After the pre-archive gate passes, run archive-tail — move
   `openspec/changes/<slug>/` → `openspec/changes/archive/YYYY-MM-DD-<slug>/`,
   using a valid ISO calendar date. The move itself is a working-tree change; the
   commit and push that publish it need the user's approval like any other.
   The exact undated `openspec/changes/archive/<slug>/` form remains readable
   only as a legacy fallback for historical archives; new archives use the
   dated provider form. The guardian fails closed when multiple dated
   candidates, dated-plus-undated candidates, invalid dates, or near-match
   names are present.
5. Run the pre-merge guardian; merge only after explicit user approval.

### 7.4 Pre-merge merge guardian (hard stop)

Before `gh pr merge` / `glab mr merge` / Bitbucket merge (or equivalent), verify
the archive is complete. Prefer the shared helper when available. `--root` is
the propagated planning root from the request context (required, never the
process cwd): for a subrepo-context change whose planning root is the proven
superproject, pass `<super>/openspec`'s parent — the superproject root — so the
guardian inspects the canonical tree and never a subrepo-local change folder.

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py" \
  <slug> --root <planning-root>
# optional: --tier light|standard|full
```

The helper ships with the CLI install under `~/.ai-specs` (not copied into
consumer projects).

Hard blockers (do **not** merge):

1. `openspec/changes/<slug>/` still exists (active, not archived).
2. No exact valid archive candidate exists at
   `openspec/changes/archive/YYYY-MM-DD-<slug>/`; the exact undated
   `openspec/changes/archive/<slug>/` form is accepted only as a legacy
   fallback when no dated candidate exists.
3. Archived folder lacks the tier minimum files (Light: `proposal.md` +
   `tasks.md`; Standard: `proposal.md` + `tasks.md` + `specs/**/*.md`; Full:
   `tasks.md` + `proposal.md` or `design.md` + `specs/**/*.md`).
4. Standard lacks a conforming dedicated `verify-report.md`.
5. Full lacks a conforming dedicated `verify-report.md` with strict `PASS` and
   `ready_for_archive: true`.

### In-flight plans and stale PRs

Plans already in flight when these minima ship are grandfathered only through
their current work: add missing `proposal.md` or verify evidence before PR or
archive, without restarting the plan. Historical archives are never rewritten;
a stale PR is handled by its owning agent when that change resumes.
Missing `explore.md` is never a guardian blocker. The guardian evaluates only
the slug under check; older archived changes and stale PRs are not rewritten.

Post-merge archive is **rejected** — if merge already happened, do not treat the
merged base branch as the archive boundary.

## 8. Change-slug derivation

- On plan, derive a kebab-case slug from intent.
- On build, resolve slug from outstanding change folders under
  `openspec/changes/` (excluding `archive/`).
- If multiple outstanding plans exist, ask which to build.

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

Archive-tail runs at step 4 of Section 7.3 (before merge):

- **Change-folder close** — move `openspec/changes/<slug>/` →
  `openspec/changes/archive/YYYY-MM-DD-<slug>/` using a valid ISO calendar date
  (required for new archives). The exact undated
  `openspec/changes/archive/<slug>/` form is a legacy fallback only.
- **Vault summary** — no-op with note if canonical store absent.
- **Tracker comment** — no-op with note if tracker absent.
