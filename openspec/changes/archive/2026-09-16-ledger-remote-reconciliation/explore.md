# Exploration: ledger remote reconciliation

Status: read-only exploration. No proposal, no spec delta, no code, no provider
call, no ledger write, no sync, no commit. Scope: `ledger-remote-reconciliation`.

Context verified before this artifact was written:

- Branch `feat/ledger-remote-reconciliation` at `6321248`, worktree
  `.worktrees/ledger-remote-reconciliation`, repo topology `standalone`,
  integration branch `development`.
- Artifact store: `openspec` (preflight). Interactive phase gate: explore only.
- OpenSpec context read: `openspec/config.yaml`, `openspec/specs/tracker-ledger/spec.md`,
  `openspec/specs/trello-state-sync/spec.md`.
- Live ledger state read (not modified):
  `<git-common-dir>/ai-specs/ledger/state.json` and `witness.json`.
- No `openspec/changes/ledger-remote-reconciliation/` proposal exists yet; this is
  the first artifact. **No `## Tracker` card is linked for this change** — the
  project rule requires a card before apply, and `openspec/**` writes are never
  gate-blocked, so the link section belongs in this change's proposal/tasks later.

## 1. Current state — what reconciliation exists today

### 1.1 The pilot, verified in the store

`/Users/robert/proyectos/nnodes/ai-specs-cli/.git/ai-specs/ledger/state.json`
holds exactly one item (`id: 1dad76c9587b8d8b`, PR #247 pilot, branch
`fix/vcs-auth-preflight-account-parse`):

```json
{ "status": "closed", "item_id": "6a558dd4cefa46b491c394d1",
  "native_type": "card", "provider_id": "trello-mcp-workflow",
  "url": "https://trello.com/c/pMaMPN8r/41-...",
  "state": "in-progress", "provider": { "list": "In Progress" },
  "decisions": [ open 16:18:26Z, link@apply-start 16:18:26Z, close@archive-close 20:36:08Z ] }
```

The witness is `bound` → `trello-mcp-workflow` (written 2026-09-15T19:20:27Z).

**This is the whole finding in one record:** `status = closed` while
`state = in-progress` and `provider.list = In Progress`. The card was moved to
Done by hand in Trello; the ledger's provider snapshot was never reconciled.
Local lifecycle was proven (open/link/close persisted across sessions and
worktree cleanup). **Automatic remote reconciliation is not implemented at all**
— not partially, not failing: absent.

### 1.2 Lifecycle state vs provider snapshot vs evidence — three different things

| Layer | Where | Who writes | Who reads it | Stale in the pilot? |
|---|---|---|---|---|
| Lifecycle `status` | `Item.Status` (`open`/`closed`) — `gate/ledger/store.go:135-150` | `--write open/close` (`applyWriteToStore`) | `Grade` (`Store.Primary`), hosts via verdict JSON | No (closed is correct) |
| Provider snapshot `Item.State` / `Item.Provider` / `Item.NativeType` / `Item.URL` | same `Item`, provider-neutral core fields + opaque `provider` | only `--write link` | **nothing** — not the predicate, not any host | **Yes — inert and stale** |
| Four-side `Evidence` (`local`/`remote`/`code`/`git`) | `gate/ledger/verdict.go:80-101`, file passed by `--evidence` | `local` = predicate default (`item.ItemID`); `code`/`git` = `lib/_internal/ledger_bridge.py`; `remote` = **no producer** | `Evidence.Conflict()` in `Grade` | n/a (remote empty) |

Two consequences the proposal must not blur:

1. **`Evidence.Remote` cannot express state at all.** `Conflict()` is an
   equality compare of each non-empty side against `local` (the item id). A
   remote producer that reads the same card would return the same id → never a
   conflict. A remote producer can only ever detect *link* mismatch, never
   `state`/`list` drift. Reconciling "Done vs In Progress" requires a **new
   comparison the Go predicate does not have**.
2. **The provider snapshot is write-only.** `Item.State`, `Item.Provider`,
   `Item.URL` are populated by `link` (`store.go:390-400`) and read by nothing.
   `store_test.go:314` even pins them as core JSON keys — but no consumer exists.
   Doctor grades `work-start` in `warn` **without `--evidence`**
   (`doctor.py:843-847`), so a stale snapshot never surfaces anywhere today.

### 1.3 Checkpoint map (all hosts are acquisition/JSON bridges; one Go predicate)

| Checkpoint | Host (path) | Evidence passed? | Consumed from verdict |
|---|---|---|---|
| `work-start` | `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh:233-243` | No | decision/reason/active |
| `apply-start` | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:755-757` | Yes (`ledger_bridge.evidence_payload`) | decision/reason/active/prompt |
| `pr-review` | same hook, shell `pr_create` branch (`:762-770`) | Yes | same |
| `pre-merge` | `lib/_internal/premerge_guardian.py::ledger_blockers` | Yes | decision/reason + blockers |
| `archive-close` | same, `--stage pre-archive` (`:653`, `:710-`) | Yes | same |
| (doctor) | `lib/_internal/doctor.py::_check_tracker_ledger` | **No** | copies `doctor` finding verbatim |

Known named residue (already documented, out of scope here): `plan-build-gate.sh:104`
still resolves config through the legacy literal `trello-mcp-workflow`
(`tests/test_ledger_mode_config.py::test_the_out_of_scope_residue_is_named_in_the_docs`).

### 1.4 Recipe autonomy modes and explicit controls — keep the axes separate

| Axis | Surface | Values / meaning | Blocks the user's change? |
|---|---|---|---|
| Ledger autonomy | `[recipes.<bound>.config] ledger_mode` (`catalog/recipes/trello-mcp-workflow/recipe.toml:81-86`), `--ledger-mode` | `warn` (default, report only) · `ask` (prompt per checkpoint; opt-out is checkpoint-scoped) · `always` (block missing/conflicted) | Only `always` blocks, and only on **missing/conflicted item** — never on provider unavailability |
| Legacy vocabulary | `gate_mode` (`off`/`warn`/`always`), maps forward only when `ledger_mode` is unset | `off` → skip checkpoints | No |
| Env one-shot | `TRACKER_LEDGER_MODE` (hosts), `WORKTREE_GATE_MODE` (worktree gate, unrelated) | Overrides config | No |
| Explicit agent/user actions | `--write open|link|close|exempt`, `--decide adjudicate|opt-out` | Deliberate, fail-closed on failure (exit 2) | Only if the caller treats exit 2 as fatal |
| Explicit sync/archive/pre-merge controls | `trello-state-sync` capability (agent + MCP), `premerge_guardian.py --stage pre-merge|pre-archive` (tier minima + verify evidence) | These are **not** autonomy settings; they are deliberate pre-merge/archive controls and must not be folded into `ledger_mode` | Pre-merge/archive guardian exits non-zero on defects |

Two host behaviors to preserve verbatim (both already match the confirmed
intent that gates govern *recipe autonomy*, not authority over the user's change):

- `tracker-card-gate.sh:634-645` — `ask` with no TTY is a hard block and refuses
  to record an opt-out; nothing is inferred.
- Every host fails open on a missing/unverified binary, cold cache, unparseable
  verdict, or lock timeout on a **grade** path (`premerge_guardian.py:596-601`,
  `tracker-card-gate.sh:649-660`). Availability failure ≠ missing item.

### 1.5 Remote adapters today: none programmatic

- The ledger path is offline by contract: `Evidence.Remote` has no producer
  (`docs/capabilities.md:132-140`, `README.md:186-188`), and
  `tests/test_ledger_evidence.py::test_bridge_is_acquisition_only_and_offline`
  asserts the bridge source contains no `socket`/`urllib`/`requests`/`gh`.
- Remote provider access is **agent-executed prose**, not an adapter:
  `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md`
  (`trello-state-sync`: move list, phase label, comment) plus `recipe.toml`
  `[[hooks]] action = "sync-card-state"` which `recipe-materialize.py:1013`
  explicitly defers to agent runtime.
- VCS side is `gh` CLI prose: `catalog/recipes/git-pr-flow` (capability
  `vcs-pr-flow`, `base_branch = "development"` in `ai-specs/ai-specs.toml:49`),
  `ai-specs/skills/release-flow/SKILL.md` (release = tag + `softprops/action-gh-release`
  CI, `main` promotion).
- The only provider-extension seam that exists is
  witness → `Binding.RecipeID` → `recipes.<id>.config` (spec "Witness-derived
  provider configuration lookup"). Provider vocabulary (board, lists, labels)
  lives in recipe config, never in ledger core (spec "Scope boundaries").

### 1.6 The desired mapping vs the semantics that exist now

Requested outcome mapping:

```
PR opened for review      -> Review
integration merge         -> Done
release  OR  merge master -> Published
```

Conflicts found (all must be decided, none invented here):

1. **Axis conflict.** The existing table is SDD phase → list, not delivery
   event → list: `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md:43-56`
   maps `proposal→Backlog, specs/design→…, tasks→Ready, apply→In Progress,
   verify→In Review, archive→Done`. Desired mapping replaces/augments that axis
   with events. `trello-state-sync` spec (`openspec/specs/trello-state-sync/spec.md`)
   is written entirely in phase vocabulary, including its degrade rule.
2. **No `Published` list exists in any artifact**, and the board's actual list
   set is not verifiable from the repository (requires an MCP read, out of
   bounds here). `default_list = "In Progress"`, `epic_list = "Epic"` are the
   only list names in recipe config.
3. **Branch naming.** This repo's promotion target is `main`, not `master`
   (`git-pr-flow.config.base_branch = "development"`; release-flow promotes
   `release/vX.Y.Z → main`). "merge to master" needs to be restated as
   "merge/promote to the release branch (`main`)" or become configurable.
4. **`archive-close` vs "integration merge".** Today `close` is written at the
   `archive-close` checkpoint (guardian `--stage pre-archive`), i.e. artifact
   archive time, which precedes the PR merge. The pilot proved this: closed at
   20:36 on the review branch, card moved to Done by hand afterwards. "Integration
   merge → Done" therefore needs a post-merge trigger that does not exist, or an
   explicit statement that close-at-archive is the intended proxy.
5. **Publication is CI-authored, not agent-authored** (`softprops/action-gh-release`
   per `CHANGELOG.md:86-89`). Any "release → Published" event source must read
   CI/tag state, not expect an agent to call a tool at the right moment.

## 2. Reusable assets (ladder rung 2 — read before writing anything)

- `WriteRequest` already carries `State`, `Provider`, `NativeType`, `URL`
  (`gate/ledger/store.go:236-247`) — a provider snapshot can be reported through
  the **existing** write surface with no new verb.
- `applyWriteToStore` `case WriteClose` (`store.go:369-380`) ignores the payload
  beyond the checkpoint: this is the one-line-shaped hole the pilot fell through.
- `Evidence` + `Conflict()` (`verdict.go:80-101`), `Item.State/Provider`,
  `Item.Conflict` snapshot, `PersistConflict`, `ReasonConflict`, `--decide
  adjudicate` are all already implemented and corpus-pinned.
- `tests/fixtures/tracker-ledger-corpus/*.json` (34 cases) already pin `remote`
  as an inhabited side (`05`, `06`, `07`, `08`, `09`, `11`), and
  `DESIGN_ROWS` in `tests/test_tracker_ledger_parity.py:36-` maps every row.
  Adding a case is a JSON file + one `DESIGN_ROWS` entry.
- `ledger_bridge.py` acquisition verbs (`slug`, `recipe`, `evidence`) are the
  established place to add a producer, and its fail-open posture is the model.

## 3. Smallest viable seam — options, ranked

**S1 — Snapshot honesty on the explicit write path (recommended, laziest).**
Make `close` (and reuse, not extend, `link`) carry the *observed* provider state,
and have the store record it: `Item.State`/`Item.Provider` updated at close time
from the payload, with the observation appended to the decision log. No remote
read, no network, no new host, no new checkpoint, no new verb, no new evidence
key. Fixes the pilot symptom exactly (`closed` + `in-progress` becomes
`closed` + observed state). Reconciliation stays where it already is —
the agent/MCP who actually moved the card reports what it saw.
*Size:* ~10–30 Go lines + ~60–100 test lines + 1–2 corpus rows. Well under the
400-line budget. Deepest rung that holds.

**S2 — Read-only remote producer, human-invoked only.**
A new producer (bridge verb or a separate `ai-specs` command) that reads the
provider once and writes the `--evidence` file; predicate gains a
provider-snapshot comparison so drift becomes `ReasonConflict` and goes through
the existing adjudication path. Constraints: never on a pre-tool-use hot path;
must keep the empty-side-is-never-a-conflict rule so a Trello/network outage
cannot block a change; must not become a second grader.
*Size:* Go predicate change + bridge producer + network-policy test carve-out +
corpus rows + docs ≈ 300–700 lines → likely crosses the 400-line review budget,
so it needs a delivery decision later (ask-on-risk; no chain invented here).

**S3 — Automatic reconciliation at checkpoints (MCP/gh from a hook).**
Rejected on evidence, not taste: pre-tool-use hosts are fail-open, cold-cache
safe, and network-free by contract; a hook doing a live provider read would put a
network call on every edit and make an outage look like a tracker problem. Also
collides with the standing "no provider writes in this slice" boundary.

**Sequencing note (no decision made).** S1 is a credible standalone slice; S2 is
the piece that would make "automatic" true. A proposal could scope S1 now and
name S2 as the next slice, mirroring how the previous slice named `remote` as the
deliberate gap.

## 4. Risks

| Risk | Why it bites | Mitigation direction |
|---|---|---|
| Provider outage misread as missing/conflicted item | `always` would then block the user's change — exactly what the confirmed intent forbids | Keep empty side ⇒ no conflict; keep availability failures fail-open; report honestly and leave reconciliation pending |
| `Evidence.Remote` reused for state and silently doing nothing | equality-compare against `local` (the item id) can never detect a state drift | If S2, the predicate needs an explicit snapshot comparison, tested by a corpus row that fails today |
| Writing a provider snapshot makes the ledger *look* authoritative about the provider | it is a report of the last observation, not a live truth | Name the timestamp/observation semantics in the spec and doctor message; never claim live state |
| Mapping change collides with `trello-state-sync` spec + phase table | two normative mappings for one card position | Decide the axis explicitly (event vs phase) and reconcile the canonical spec in the same change, or scope this change to one axis |
| `Published` list does not exist / is board-specific | hardcoding it repeats the hardcoded-literal mistake | Provider vocabulary stays in recipe config (`provider-private`), and the change must not put list names in ledger core |
| Network on the hot path | every edit pays it; cold cache breaks it | S2 only on human-invoked paths; hot path stays offline |
| Trust root drift | Go change ⇒ `SHA256SUMS`/4-arch rebuild in the same work unit | Same-work-unit rule already established |

## 5. Scope forecast (for the proposal phase, not a plan)

| Area | Files | Rough changed lines |
|---|---|---|
| Snapshot-on-close (S1) | `gate/ledger/store.go`, `gate/ledger/write_test.go`, `gate/ledger_cmd.go` docs | 40–130 |
| Corpus/pins | `tests/fixtures/tracker-ledger-corpus/*.json` (2–4), `tests/test_tracker_ledger_parity.py` | 30–90 |
| Remote producer (S2 only) | `lib/_internal/ledger_bridge.py` (or a new module), `tests/test_ledger_evidence.py`, hosts | 150–400 |
| Spec delta | `openspec/specs/tracker-ledger/spec.md` + possible delivery-event spec | 120–250 |
| Recipe config + agent guidance | `catalog/recipes/trello-mcp-workflow/{recipe.toml,README.md,skills/…,commands/trello-workflow.md}`, `docs/capabilities.md`, `docs/runtime-hooks.md`, `README.md`, `CHANGELOG.md` | 80–200 |
| Trust root (if Go changes) | `catalog/recipes/worktree-flow/bin/SHA256SUMS`, `dist/worktree-gate-current`, `scripts/*` | mechanical |

S1 alone: comfortably inside the 400-line review budget.
S1 + the event mapping + docs: borderline.
Any S2 variant: over budget → the delivery decision must be asked at
proposal/apply time under `ask-on-risk`. Chaining stays deferred until chosen.

## 6. Open product decisions (need the user, not the agent)

1. **What exactly is the publication event?** GitHub Release/tag created, merge
   into `main`, version bump in `VERSION`, or CI workflow green? (CI writes the
   release — `softprops/action-gh-release`.)
2. **`master` means what here?** This repo promotes to `main`; is "merge master"
   literally the release branch, configurable per project, or "any merge into the
   branch named by recipe config"?
3. **Is "integration merge → Done" a real event we can observe, or is
   `archive-close` (close written at archive time, before merge) the intended
   proxy?** The pilot closed at archive and moved the card by hand afterwards.
4. **Does the `Published` list exist on the board, and who creates it if not?**
   And is the mapping per-board/per-recipe config rather than global?
5. **Drafts / pre-merge publication:** is a draft release or a release candidate
   `Published`, or still `Done`?
6. **Reversals:** if a card is moved backwards (or a PR is reverted post-merge),
   does the ledger record a new decision and leave the state, or auto-follow?
   (Current model is append-only decisions + never reopen — reuse or extend?)
7. **Manual overrides:** does a human moving the card by hand become ledger
   truth (recorded via an explicit write), or an unresolved conflict awaiting
   adjudication? Confirmed intent says human decisions are never inferred.
8. **Is a read-only provider read allowed at all in the next slice**, and only
   from human-invoked paths — or is "reconciliation" deliberately restricted to
   reporting what the agent already observed (S1)?
9. **Who owns the reconcile action** when the ledger and provider disagree:
   `--write link/close` (agent), `--decide adjudicate` (human), or a new verb?
10. **Does the provider snapshot side become part of the conflict predicate**
    (`ReasonConflict` + adjudication), or a separate non-blocking doctor finding?
11. **Does remote evidence stay the equality-compared id side**, with snapshot
    drift carried by a new field — or does `remote` get redefined (breaking the
    pinned corpus rows `05`/`06`/`07`/`08`/`09`/`11`)?
12. **Relationship to `trello-state-sync`:** does the ledger become the grader of
    card position, or does the capability stay prose-only with the ledger merely
    recording observed state?
13. **Local closed work awaiting publication:** confirm the intended behavior —
    `closed` + "awaiting publication" is a legitimate resting state (the pilot),
    not an error.

## 7. Explicitly out of scope for this change (as understood)

No provider create/update/move/comment/label writes unless separately decided;
no new Go module/binary/release asset; no second grader (Python stays acquisition);
no relocation of a checkpoint host; no generic multi-capability ledger; no
provider vocabulary in ledger core; no unrelated global rewrites; no migration or
rewrite of historical archives; no folding of `trello-state-sync` /
`premerge_guardian` explicit controls into `ledger_mode`.

## 8. Recommendation

Adopt **S1** as the smallest viable seam if the goal is "the ledger must stop
recording a provider state it knows to be stale", and treat **S2** as the named
next slice for anything that must be *automatic*. Decide decisions 1–5 and 13
before proposal, because they determine whether this is a one-line snapshot fix
or a delivery-event reconciliation feature. Everything under S1 reuses code that
already exists; nothing under S3 is worth the hot-path cost.
