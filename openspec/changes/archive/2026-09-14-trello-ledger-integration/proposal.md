# Proposal: Turn the bound Trello recipe into a real ledger adapter — no provider writes

Change: `trello-ledger-integration` · Tier: Full · Explore: `explore.md` (approved)
Proposal question round: closed — answers recorded as L1–L7 (see `## Proposal question round`)
Research: **unselected** for this change — no research artifact exists or is implied.
Tracker: card 127 (this change's primary item — see `## Tracker`)
Foundation: `openspec/changes/archive/2026-09-13-tracker-ledger-foundation/` (its D1–D19 stand; L-numbers are this change's decisions and do not reopen them)

## Why

The foundation shipped a **verified grader with no writer**. The ledger can answer "is there a tracked
item for this work identity?" at all five checkpoints, but nothing in production ever records one, so the
honest verdict for a real project is `needs-item` forever — and in this project's dogfooded `warn` posture
that is a **silent no-op**. Three consequences we pay for today:

1. **The adapter is a façade.** `Store.OpenItem`, `Store.CloseItem` and `AppendDecisionToPrimary` have zero
   production callers (`grep` matches only `ledger/*_test.go` plus the definitions). The machine-written
   decision kinds `open`/`close`/`link` are declared, validated by the store, and **unreachable**. A bound
   project gets a grader and no way to satisfy it.
2. **The conflict predicate cannot fire.** `--evidence` is accepted by the CLI but **no production host ever
   passes it**, so a verdict with an item is always `allow`. The exact pain the foundation was built for —
   `ledger.item_id != ## Tracker card_id` — is undetectable in the field.
3. **`tracker.none` is invisible.** `verdict.go` honors `Item.Exemption` → allow/exempt, but nothing writes
   it, so the documented "logged and rare" escape hatch is prose with no record.

This change owns one slice: **make the already-bound `trello-mcp-workflow` recipe a real adapter** — a
durable item exists, it carries code/Git evidence, it can be linked and closed, and disagreement between the
ledger and the human-facing `## Tracker` artifact surfaces as a conflict a human adjudicates — **without any
provider write.**

## What Changes

**A machine-write surface, shaped like the one that already works.** Extend the proven `--decide` pattern in
`gate/ledger_cmd.go`: one new JSON-in-flag write verb (e.g. `--write`) carrying
`{kind: open|link|close|exempt, item_id, url, native_type, state, provider, reason}` plus the existing
`--checkpoint`. Parse → validate → **locked** read-modify-write → re-grade → print verdict. No new
subcommand tree, no second binary, no new module. `link` is the real adapter verb: it records the provider's
native id/url/state on the provider-neutral core fields, which is what makes local/code/git comparison
meaningful.

**Idempotent, collision-safe writes.** `OpenItem` is deliberately non-idempotent (its doc says `Primary`
reports the collision), so the writer must be **open-if-absent under the same `withStoreLock`** the append
primitive already uses — otherwise the first retried open manufactures a conflict for a human. The writer
also refuses (fail closed) on `Identity.Collision == change-ambiguous`, guards the second-precision
`NewItemID` collision, and makes repeated `link` / `close-after-close` explicit rather than silently
duplicating decisions.

**A thin evidence bridge (three of four sides).** A Python bridge — thin acquisition/JSON only, per
foundation D5 — builds the `--evidence` file from local facts: `local` = the store snapshot, `code` = the
change's `## Tracker` `card_id` / `pr:` parsed by the existing `lib/_internal/trello_link.py` (parser, never
a grader — foundation A8), `git` = locally derivable branch/HEAD/commit plus the recorded PR URL.
`tracker-card-gate.sh::_ledger_grade` and `premerge_guardian.py::ledger_blockers` pass `--evidence`.
No `gh`, no MCP, no network on a pre-tool-use hot path; unreadable evidence keeps failing open.

**`tracker.none` becomes a durable, auditable exemption** stored as `Item.Exemption` and honored by the
existing `DecisionAllow/ReasonExempt` branch at every checkpoint — a persistent change-scoped record, not a
per-checkpoint re-prompt.

**Provider binding stops being hardcoded.** Three production sites look up `trello-mcp-workflow` by literal
(`premerge_guardian.py:501`, `tracker-card-gate.sh:494`, `doctor.py::_tracker_ledger_in_play`). Resolve the
bound recipe id from `<git-common-dir>/ai-specs/ledger/witness.json` (`Binding.RecipeID` — hosts may read the
witness; reading is not grading) and use it for `ledger_mode`/`gate_mode`, with the current literal as
fallback when the witness is missing. This is the same seam a future provider will use.

**The spec gets corrected, not the grader.** The archived spec scenario *"Branch reuse after closed work
opens a new item"* says the **grade** opens the item; `Grade` is documented pure and opens nothing. The
opening verb moves to the explicit write path and the scenario wording is fixed in the spec delta. **`Grade`
stays pure.**

## Confirmed decisions

Product answers, confirmed by the user before this proposal was written. Specs, design, and tasks elaborate
these; they do not reopen them.

| # | Decision |
|---|---|
| L1 | `tracker.none` is a **persistent, auditable, change-scoped exemption** stored as `Item.Exemption` and honored at every checkpoint. It is **not** mapped onto the existing checkpoint-scoped `opt-out` path. |
| L2 | **Item opening is explicit.** An item is opened by a deliberate write, never automatically because `## Tracker` parses. Grading never writes. |
| L3 | **Evidence is local / code / Git.** `remote` evidence is **deferred** to a later slice; a read-only tracker MCP read is not in scope. This slice ships a deliberate 3-of-4 reconciliation. |
| L4 | **No TTY remains a hard block.** `ask` with no terminal keeps today's exit `2`. `ask` is not downgraded to report-and-proceed to make agent harnesses comfortable. |
| L5 | **`work-start` stays hosted by `plan-build-flow`.** No checkpoint host is relocated here; the unhosted-in-this-project case is made visible (doctor / documented limitation), and `catalog/recipes/plan-build-flow/**` gets no content change. |
| L6 | **Write failures fail closed** — a failed `--write` persists nothing and exits `2`, like `--decide`. Grade paths keep failing open. |
| L7 | **Conservative posture overall:** nothing in this slice performs a provider create/update/move/comment/label (foundation D11 unchanged), and no new Python predicate is introduced (D3, D5 unchanged). |

## Scope

### In

1. **Writer + idempotency (Go).** The `open|link|close|exempt` write surface reusing the `--decide` flow;
   locked open-if-absent, re-link/close policy, bounded lock attempt (`LOCK_NB` + short retry; grade fails
   open, write fails closed), `change-ambiguous` refusal, duplicate-item-id guard; `ledgerSelftest` extended;
   new `ledger/write_test.go`; **trust root regenerated in the same work unit**.
2. **Evidence bridge + `tracker.none` (Python + hosts).** One bridge producing the `--evidence` JSON from
   `## Tracker` / `tracker.none` / local git facts; `tracker-card-gate.sh` and `premerge_guardian.py` pass it;
   `tracker.none` → `Item.Exemption` per L1; corpus rows + new `tests/test_ledger_evidence.py`.
3. **Provider-id lookup + docs (smallest unit).** Witness `recipe_id` + legacy fallback replacing the three
   hardcoded lookups; checkpoint-owner map recorded; `docs/capabilities.md`, `docs/runtime-hooks.md`, the
   recipe README + skill `## Tracker` section, `CHANGELOG.md`; the stale `tracker-card-gate.sh` header comment
   fixed in the same unit.

### Out (non-goals — preserved from exploration, deliberately unwidened)

- **Any provider MCP/API write**: create, update, move, comment, label (foundation D11 stands). This change
  is the "supplies the item" slice, still without provider writes.
- **`remote` evidence** (L3) — no producer, no MCP read.
- **Relocating a checkpoint host**, including `work-start` out of `plan-build-flow` (L5).
- **Any content change under `catalog/recipes/plan-build-flow/**`.**
- **Jinna or any second provider adapter.** `jinna-mcp-recipe` declares no `[[capabilities]]` and cannot bind
  `tracker` today; the witness → `recipe_id` → provider-config lookup is the only seam built for it.
- A generic/multi-capability ledger, a ledger "plugin" API, or a worktree ledger (D1, D2).
- Historical archive migration or rewrite; revalidation of in-flight changes (D13).
- Provider vocabulary or `## Tracker` parser/schema changes; promoting `board_id`/list names/labels into
  ledger core (D11, D14).
- Compaction/retention enforcement beyond the advisory `MaxItems = 32` / `MaxStoreBytes = 64 KiB` ceiling.
- A second Go module, binary, `SHA256SUMS`, release asset, or third-party dependency.
- Broad Python→Go migration beyond the seams this change touches.
- UI, dashboards, analytics, new configuration knobs that never change.

### Dead weight named (explore §8 rule)

Everything the foundation declared but this slice still does not reach is named here, not left silently
unreachable: `Evidence.Remote` stays **prose-only with no producer** (L3). `Item.Provider` stays an opaque
provider payload written by `link` but never read by the predicate (D14). `Store.OptOuts`, `PersistConflict`,
`clearConflict`, and compaction are unchanged. `DecisionOpen/Close/Link` and `Item.Exemption/State/NativeType/
URL` **stop** being dead weight in this slice — that is the point. No symbol is added speculatively.

## Affected areas

| Area | Change |
|---|---|
| `catalog/recipes/worktree-flow/gate/ledger_cmd.go`, `main.go` | New write flag + payload validation on the existing `ledgerOptions`/verdict flow; write failures exit `2` (L6); existing flag-parse fail-open on grade calls preserved. |
| `catalog/recipes/worktree-flow/gate/ledger/store.go` | Locked idempotent open-if-absent, link/close policy, bounded lock attempt, item-id duplicate guard — following the `AppendDecisionToPrimary` precedent exactly. |
| `catalog/recipes/worktree-flow/gate/ledger/write_test.go` (new) | Open idempotency, open under lock, same-second open, link, close, close-after-close, `change-ambiguous` refusal, `-race -count=3`, no `state.json.tmp.*` residue. |
| `catalog/recipes/worktree-flow/gate/ledger/verdict.go` | Read-only use of `Item.Exemption` (already implemented); `Grade` remains pure. |
| `lib/_internal/` evidence bridge (new thin helper) | `## Tracker` + `tracker.none` + local git → `--evidence` JSON. Reuses `trello_link.py`; never grades; failure is fail-open. |
| `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | Passes `--evidence`; issues the write verb; witness-based recipe-id lookup with legacy fallback; stale header comment corrected. |
| `lib/_internal/premerge_guardian.py` | `ledger_blockers` passes evidence; `resolve_ledger_mode` uses the witness `recipe_id`; must keep working from a cold `AI_SPECS_HOME` with no project cache. |
| `lib/_internal/doctor.py` | `_tracker_ledger_in_play` uses the witness `recipe_id` (+ fallback); renders the unhosted-`work-start` limitation. |
| `openspec/specs/tracker-ledger/**` (delta at specs phase) | Correct "the grade opens the item"; add the write surface, exemption, evidence-sides, and failure-posture requirements. |
| `tests/fixtures/tracker-ledger-corpus/**`, `tests/test_tracker_ledger_parity.py` | New rows + matching `DESIGN_ROWS` pins: open-then-allow, code-vs-ledger conflict, `tracker.none` exemption, idempotent re-open, link mismatch. |
| `tests/test_ledger_evidence.py` (new) + `tests/test_tracker_card_gate_hook.py`, `tests/test_premerge_guardian.py`, `tests/test_ledger_mode_config.py`, `tests/test_doctor_tracker_card.py` | Evidence acquisition, exemption mapping, `--evidence` actually passed, provider-id lookup, failure posture; all five hosts must keep returning the same verdict. |
| `catalog/recipes/worktree-flow/bin/SHA256SUMS`, `dist/worktree-gate-current`, `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, `.github/workflows/release-worktree-gate.yml` | Trust root regenerated in the same work unit as the Go change; release workflow stays the only release path. |
| `docs/capabilities.md`, `docs/runtime-hooks.md`, `catalog/recipes/trello-mcp-workflow/README.md` + skill, `CHANGELOG.md` | Writer, 3-sided evidence, `tracker.none` as `Item.Exemption`, checkpoint-owner map. |

**Unchanged on purpose:** `go.mod` (stdlib only), the `## Tracker` block shape and `required_fields`,
`openspec/**` non-blocking writes, the exit-code contract (`0` for `allow|ask|dormant|unevaluable`, `2` only
for `block` / failed persist), this project's `warn` dogfood posture, all archives, and
`catalog/recipes/plan-build-flow/**`.

## Risks

| Risk | Mitigation |
|---|---|
| **A retried open creates two open items** → a self-inflicted `ErrMultipleOpen` conflict for a human | Open-if-absent under `withStoreLock`, pinned by a RED test before the writer lands. |
| **`NewItemID` collides within one second** (second-precision hash of identity+openedAt) | Explicit duplicate-id guard or nonce in the writer; corpus row for the collision. |
| **Unbounded `flock` wait on a pre-tool-use hook** stalls edits | Bounded `LOCK_NB` + short retry; grade → fail open, write → fail closed (L6). |
| **`change-ambiguous` merges two changes into one item** | Writer refuses when `Identity.Collision != ""` unless an explicit slug is supplied. |
| **Trust-root regeneration forgotten** (four-arch `SHA256SUMS` + `dist/` rebuild) | Same work unit as the Go change; parity suites skip loudly without `dist/worktree-gate-current`, so `./tests/validate.sh` is the backstop. |
| **Evidence acquisition grows the hot path** (python3 parse per edit) | Reuse the existing pure parser; no `gh`, no MCP, no network; any failure fails open. |
| **A second/fourth `## Tracker` grader sneaks in via the bridge** | Bridge is acquisition-only; the `## Tracker` predicate stays the single Go grader; parity tests hold the seam. |
| **3-sided evidence looks like full reconciliation** | Stated in docs and spec as a deliberate gap (L3); `remote` named as unwired, not implied. |
| **`ask` stays theoretical without a tty** (L4 keeps the hard block) | Documented limitation, not silently softened; `warn` remains the adopted posture, so nothing depends on `ask` in agent harnesses yet. |
| **`premerge_guardian.py` cold-cache path** (ships in the CLI install) | The bridge adds no project-cache dependency; existing cold-path tests extended. |
| **Provider coupling persists** if the witness lookup is skipped | Unit 3 is small and behavior-identical when the witness is absent; if cut for review budget, the three literals are recorded as remaining work, not as done. |
| **Scope creep toward provider writes / Jinna / plan-build-flow** | Non-goals above; nothing in scope touches a provider API or `plan-build-flow`. |
| **Vault scope path missing** | Explore's friction items 1–5 (no-tty `ask`, no-evidence→allow, stale gate header, trust-root step, write posture) are recorded **here** as L4–L6 and named risks instead of being pushed to tracker follow-up cards; re-record in Vault when the scope path exists. |

## Rollback

Additive and reversible by revert; nothing historical is rewritten.

1. **Revert the change branch/PR.** The write verb, evidence bridge, and witness-based lookup disappear; the
   binary returns to the foundation's verdict-only surface. Regenerate `SHA256SUMS` + `dist/` in the revert so
   the trust root matches the reverted tree — a stale digest is a fail-open, not a silent continue.
2. **Stop the writer, keep the grader.** Hosts that stop passing `--evidence` return the project to today's
   behavior exactly (item present → `allow`), which is the state every bound project is in now. Nothing
   depends on evidence to be safe.
3. **Data left behind is inert.** Items, `link` decisions, and `Item.Exemption` written before the revert stay
   in `<git-common-dir>/ai-specs/ledger/state.json`. Reverting the code cannot make them wrong, but an
   exemption written by a reverted writer must not be trusted as current policy: to abandon the exemption for
   a change, delete `openspec/changes/<slug>/tracker.none` and adjudicate in the ledger rather than editing
   `state.json` by hand.
4. **No migration exists in either direction** — no archive rewrite, no provider mutation to undo, no
   external side effect to compensate (L7 is the real rollback guarantee: nothing left the machine).

## Success criteria

1. A real (non-test) code path opens exactly one primary item per work identity, and a second invocation of
   the same write does **not** create a second open item (open-if-absent under lock).
2. `DecisionOpen`, `DecisionClose`, `DecisionLink` and `Item.{ID,URL,State,NativeType,Provider,Exemption}`
   each have at least one production writer; no declared kind remains reachable only from tests.
3. The conflict predicate **fires in production**: a fixture where `## Tracker card_id != ledger item_id`
   yields `ReasonConflict` through at least the apply-start and pre-merge hosts, with `remote` explicitly
   absent and documented as deferred (L3).
4. `tracker.none` produces a durable `Item.Exemption` visible in `state.json`, is honored at every checkpoint
   as allow/exempt, and does not register as a conflict (L1).
5. `Grade` is still pure: no code path lets a grade mutate the store, and the spec delta corrects the "grade
   opens the item" scenario (L2).
6. Failure postures are pinned by tests: write failure → exit `2` and no store change (L6); grade-path lock
   timeout / bad evidence → fail open; `change-ambiguous` write → refused.
7. Today's semantics are preserved verbatim where not intentionally changed: exit codes, `openspec/**` never
   blocked, no-TTY `ask` still exit `2` (L4), corrupt store → `unevaluable` + doctor ERROR, dormant/unbound →
   skip, this project's `warn` posture, `work-start` still hosted by `plan-build-flow` (L5).
8. Zero hardcoded `trello-mcp-workflow` config lookups remain in `premerge_guardian.py`,
   `tracker-card-gate.sh`, and `doctor.py` — or the residue is named in `tasks.md` as remaining work.
9. `./tests/validate.sh` passes, `scripts/verify-gate-sums.sh` is green against the rebuilt four-target trust
   root, `go.mod` is unchanged, and every new corpus row has a `DESIGN_ROWS` pin.
10. Docs state honestly what the ledger does **not** know (no remote side, no provider write, no automatic
    open), and no plan-build-flow content changed.

## Delivery & review workload expectation

- **Expected size: well over the 1200 changed-line review budget**, plus a regenerated trust root. The
  foundation slice it builds on shipped 9,796 insertions / 61 files under an accepted `size:exception`.
- **The three scope units above are the intended review units**, in dependency order (writer → evidence +
  `tracker.none` → provider lookup + docs). Each is independently reviewable and each keeps its own tests,
  corpus rows, and docs with its code. The Go unit and its `SHA256SUMS`/`dist` rebuild never split apart.
- **Delivery strategy is `ask-on-risk`: this proposal does not choose.** Before apply, the delivery decision
  (chained PRs per work unit vs a single PR vs an exception) is surfaced to the human. `size:exception` is
  never inferred — it requires explicit acceptance of `size:exception`. No chain strategy is invented here.
- **Review-facing invariant:** a reviewer must never see a diff that changes the binary without the matching
  trust-root update, or that adds evidence acquisition without the parity rows that pin it.

## Design questions for specs/design (product is locked above)

- **DW1 — Who issues the `tracker.none` exempt write:** the agent invoking the writer, or the host that sees
  `openspec/<change>/tracker.none`? L1 fixes the *storage and honoring*; the call site stays open. Whichever
  is chosen, the file remains the human-authored act and the ledger remains the record.
- **DW2 — Exact write-flag shape and payload schema** (`--write` vs `--track`, field names, whether
  `reason` is required for `exempt`) — must reuse `DecisionRequest.Normalize/Validate` conventions.
- **DW3 — Bounded-lock parameters** (retry count/backoff) and whether they differ per checkpoint.
- **DW4 — `link`/`close` idempotency representation**: no-op signal vs explicit "already closed" verdict text,
  without inventing a new decision kind outside the closed `decisionKinds` set.
- **DW5 — Evidence bridge placement and contract** (`lib/_internal/` helper vs hook-local logic), keeping it
  acquisition-only and cold-cache-safe.

## Assumptions carried forward

1. "Provider-neutral core fields" means `Item.{ID,ProviderID,NativeType,URL,State,Exemption}`; Trello-shaped
   config stays in `recipe.toml` / `[recipes.<id>.config]` (D14).
2. The synthetic fixture recipes plus the corpus — not the live board — remain the proof surface (D11).
3. This project stays `gate_mode = "warn"` with no `ledger_mode`; adopting `ask`/`always` is a separate human
   choice (D18), and L4 is why `ask` is not the near-term adoption path in agent harnesses.
4. Production paths keep the current definition of the non-read-only-write boundary; `openspec/**` stays
   exempt at the path gate.
5. Delivery strategy, changed-line budget, and PR shape are decided at the delivery step under `ask-on-risk`,
   not in this proposal.

## Proposal question round (closed)

The round ran before this proposal and the user answered; the questions are recorded so specs/design do not
re-ask them.

| Question (business/product framing) | Answer | Recorded as |
|---|---|---|
| What should `tracker.none` mean to the ledger — a change-scoped exemption or the existing checkpoint-scoped opt-out? | Persistent, auditable, change-scoped exemption stored as `Item.Exemption`. | L1 |
| Who opens the item — an explicit write, or the host auto-opening when `## Tracker` parses? | Explicit write only; parsing never opens. | L2 |
| Is 3-sided (local/code/git) reconciliation enough for this slice, or is a read-only remote read required now? | 3-sided is enough; `remote` deferred. | L3 |
| Should `ask` with no terminal keep blocking, or report-and-proceed? | Keeps the hard block. | L4 |
| Does `work-start` move to the tracker recipe? | No; stays in `plan-build-flow`, unhosted case made visible. | L5 |
| Failure posture of the new write path? | Fail closed (exit `2`), like `--decide`. | L6 |
| Overall risk appetite for provider side effects? | Conservative: no provider writes, no new predicates, no auto-open. | L7 |

## Planning depth

**Full** (carried from explore; tier and signal depth agree, no depth conflict). Chain: explore (done) →
proposal (this file) → specs → design → tasks → apply → verify. `Explore: written` (`explore.md`). Research
is unselected for this change — there is no `research.md` and none should be inferred.

## Tracker

- **card_id**: `6aa703fdcf61a90ec702d58b`
- **url**: https://trello.com/c/ie4mQykZ/127-feature-trello-ledger-integration-make-bound-tracker-a-real-ledger-adapter
- **list**: In Progress
- **parent epic**: https://trello.com/c/0Tv0HZ6Q/125-epic-tracker-ledger-foundation

This is this change's **primary tracker item** and the ledger binding for the work identity
(`ai-specs-cli` common dir + branch `change/trello-ledger-integration`). Card id + url are the `## Tracker`
contract; the board, list, labels, comments, and card title are provider-private Trello shape and must not be
read as ledger core fields, item-state vocabulary, or a `## Tracker` schema requirement (foundation D11, D14).
No provider write happens from this section: recording the link is a parse, and the ledger write that records
this card is the feature this proposal asks for.
