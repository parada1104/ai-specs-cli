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
| **Standard** | Scoped feature or multi-file fix in a known area; intent is clear but needs written requirements | spec → tasks (skip explore/proposal/design unless they reduce risk) | `tasks.md` plus at least one spec delta under `specs/` |
| **Light** | Small bugfix, single-file tweak, typo, or user names exact file(s) and expected edit | tasks only | `tasks.md` |

### Classifier signals (quick)

| Signal | Tier |
|---|---|
| "Add OAuth", "redesign sync", "new recipe", "breaking v3" | Full |
| "Add field to X", "extend handler for Y", bounded multi-file feature | Standard |
| "Fix typo in README", "change default in config.toml line 12", one-line bug | Light |
| User says "implement/build/fix now" but **no** `openspec/changes/<slug>/` exists yet | Classify first — verbs do **not** skip planning |
| User says "go ahead" and a matching change folder already exists | Build (tier already chosen) |

Trivial read-only questions skip the classifier entirely.

## 3. Phase mapping (private)

- **Plan** runs the chain for the classified tier (Section 2), then **stops**.
- **Build** runs: apply → verify → artifact/PR gates → archive-tail (pre-merge).

## 4. When to invoke

| Situation | Invoke plan? | Invoke build? |
|---|---|---|
| Quick question / read-only exploration | No | No |
| Substantial or direct implementation request without prior change folder | Yes — classify, plan, stop | No |
| User approves a pending plan ("go ahead", "implement it", "build it") | No | Yes |
| User asks to open a PR | No | Yes — but only after artifact gate (Section 7) |
| Trivial one-line fix explicitly scoped by user | Light tier — plan (tasks only) then may build inline | Maybe same turn after micro-plan |

Prefer the project's native plan/review UX when available (e.g. plan mode) —
this skill supplies the artifact trail behind that surface. **All classified
planning artifacts MUST be written during plan mode**; production code is never
modified while planning. Do not tell the user to "run /plan" or similar —
respond to natural requests ("necesito implementar…") by classifying and
planning.

## 5. Orchestrator degradation policy

- **Orchestrator present** — let it drive phases via sub-agents.
- **Orchestrator absent** — run equivalent phases inline in one conversation.

## 6. Memory degradation policy

- **Persistent memory present** — may persist cross-session facts.
- **Absent** — default to file artifacts under `openspec/changes/<slug>/`.

## 7. Artifact, PR, and merge gates

### 7.1 Change folder required

Every non-trivial change MUST have a slug folder at `openspec/changes/<slug>/`
(excluding `archive/`) before implementation begins on that change. If the user
jumps straight to "implement X", create the folder and run the classified plan
first.

### 7.2 PR creation gate (hard stop)

Do **not** run `gh pr create`, `glab mr create`, or equivalent until:

1. The matching `openspec/changes/<slug>/` folder exists on the current branch.
2. Tier minimum files from Section 2 are present and **committed**.
3. Implementation and verification for that slug are complete (or the user
   explicitly accepts opening a draft-only PR for review of planning — still
   requires the tier minimum files).

If the gate fails, stop with a plain-language blocker: complete planning for
the classified depth, commit the files, then retry PR creation.

### 7.3 Pre-merge archive gate (hard stop)

Archive-tail MUST run on the **review branch** before merge — never defer until
after the merge lands on the base branch. This aligns with the bound VCS merge
workflow.

Sequence on the review branch:

1. Implement and verify.
2. Commit and push implementation **and** planning files.
3. Open PR (artifact gate satisfied).
4. **Before merge:** run archive-tail (Section 8) — move
   `openspec/changes/<slug>/` → `openspec/changes/archive/<slug>/`, commit,
   push to the review branch.
5. Merge only after explicit user approval and the pre-merge archive commit is
   on the PR branch.

### 7.4 Pre-merge merge guardian (hard stop)

Before `gh pr merge` / `glab mr merge` / Bitbucket merge (or equivalent), verify
the archive is complete. Prefer the shared helper when available:

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py" \
  <slug> --root <repo-root>
# optional: --tier light|standard|full
```

The helper ships with the CLI install under `~/.ai-specs` (not copied into
consumer projects).

Hard blockers (do **not** merge):

1. `openspec/changes/<slug>/` still exists (active, not archived).
2. `openspec/changes/archive/<slug>/` is missing.
3. Archived folder lacks the tier minimum files (Light: `tasks.md`; Standard:
   `tasks.md` + `specs/**/*.md`; Full: those plus `proposal.md` or `design.md`).

If any blocker fires, stop with plain language and complete archive-tail first.

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
  `openspec/changes/archive/<slug>/` (required).
- **Vault summary** — no-op with note if canonical store absent.
- **Tracker comment** — no-op with note if tracker absent.
