# Exploration: trello-ledger-integration

Change: `trello-ledger-integration` · Tier: Full · Phase: explore
Worktree: `.worktrees/trello-ledger-integration` · Branch: `change/trello-ledger-integration`
Base: `development` @ `8e87323` (`ai-specs: remove plan-build-flow`)
Artifact store: openspec · Execution: interactive · Test command: `./tests/validate.sh`

Read scope: the merged Tracker Ledger foundation
(`openspec/changes/archive/2026-09-13-tracker-ledger-foundation/`, `catalog/recipes/worktree-flow/gate/ledger/**`,
`lib/_internal/{doctor,premerge_guardian,trello_link}.py`) and the Trello recipe
(`catalog/recipes/trello-mcp-workflow/**`). No production file was edited; only this file was created.

---

## 1. Context

The foundation shipped a **verified grader** with no **writer**. The ledger can answer
"is there a tracked item for this work identity?" at all five checkpoints, but nothing in production
ever records an item, so the honest verdict for a real project is `needs-item` forever — and in the
dogfooded `warn` posture that is a silent no-op. This exploration maps the smallest slice that turns
the already-bound `trello-mcp-workflow` recipe into a **real adapter**: a durable item exists, it carries
code/Git evidence, it can be linked and closed, and disagreements between the ledger and the human-facing
`## Tracker` artifact surface as a conflict instead of being ignored.

**Central finding.** `Store.OpenItem`, `Store.CloseItem`, and `AppendDecisionToPrimary` have **zero
production callers** (`grep -rn 'OpenItem\(|CloseItem\(|AppendDecisionToPrimary' --include=*.go` matches only
`ledger/*_test.go` and the definitions in `ledger/store.go`). The only production writer of
`<git-common-dir>/ai-specs/ledger/state.json` is `--decide` → `PersistDecision` (opt-out / adjudicate) and
`PersistConflict`. The machine-written decision kinds `open`/`close`/`link` are **declared and accepted by
the store but unreachable** (`ledger/store.go` `decisionKinds`, `DecisionOpen|DecisionClose|DecisionLink`).
`decide.go`'s own doc comment states this: *"the machine-written kinds open/close/link go through the store
API directly"* — that API has no CLI surface.

---

## 2. Foundation behavior already implemented (do not re-implement)

| Capability | Where (exact symbol) | State |
|---|---|---|
| Work identity (common dir + branch + optional change slug) | `ledger/identity.go::DeriveIdentity`, `IdentityKey`, `ActiveChangeSlugs`, `ResolveChangeDir` | Implemented; archive-aware slug order pinned by `tests/_change_paths.py` parity |
| Detached HEAD / unborn branch / no common dir | `identity.go::ReasonIdentityUnavailable` | Implemented (`Available()` false → non-guessed) |
| Change-slug collision | `identity.go::CollisionChangeAmbiguous` | Implemented: slug omitted, collision flagged |
| Durable binding witness (read-only in Go) | `ledger/witness.go::ReadBinding`, `Binding.Active()`, `WitnessPath` | Implemented; sync writes it |
| Witness producer (Python bridge) | `lib/_internal/recipe-materialize.py` `LEDGER_WITNESS_RELPATH` (~L754), `write_tracker_witness` (~L855-880) | Implemented (atomic `mkstemp` + `os.replace`) |
| Store: version 1, items[], opt_outs[], atomic write, flock | `ledger/store.go::LoadStore`, `SaveStore`, `writeAtomic`, `withStoreLock`, `StorePath` | Implemented; `StoreCorruptError` → unevaluable |
| Item model + provider-neutral core fields | `store.go::Item` (`item_id`, `provider_id`, `native_type`, `url`, `state`, `provider`, `exemption`, `conflict`, `decisions`) | Model implemented; most fields have **no writer** (see §4) |
| Item primitives (unlocked) | `store.go::OpenItem`, `CloseItem`, `AppendDecision`, `Primary`, `OpenItems`, `HasOptOut`, `HasScopedOptOut`, `NewItemID` | Implemented; only tests call the mutators |
| Locked append primitive | `store.go::AppendDecisionToPrimary(path, key, d)` | Implemented, **unused in production** |
| Human decision persist (fail-closed) | `ledger/decide.go::PersistDecision`, `DecisionRequest.Normalize/Validate`, `DecisionChoices` | Implemented for `kind ∈ {adjudicate, opt-out}` only |
| Conflict snapshot persist | `decide.go::PersistConflict`, `clearConflict` | Implemented |
| Five-checkpoint × three-mode predicate (pure) | `ledger/verdict.go::Grade`, `resolve`, `outcome`, `ExitCode`, `Checkpoint*`, `Mode*`, `Decision*`, `Reason*`, `DoctorFinding` | Implemented; 5×3×8 matrix tested |
| Checkpoint-scoped opt-out incl. no-item scoped opt-out | `verdict.go` `HasScopedOptOut` branch; `decide.go` `Store.OptOuts` | Implemented (JD round-1 correction) |
| Evidence model + conflict predicate | `verdict.go::Evidence`, `Evidence.Conflict()`, `Conflict`, `Prompt` | Implemented; `Local` defaults to `item.ItemID` inside `Grade` |
| CLI + JSON contract | `gate/ledger_cmd.go::runLedger`, `ledgerOptions`, `newLedgerOut`, `loadLedgerEvidence`, `persistLedgerDecision`, `storedLedgerSlug` | Implemented: `--ledger --checkpoint --ledger-mode --project-root [--witness --store --evidence --decide]` |
| Flags + selftest wiring | `gate/main.go` (`ledgerRun`, `ledgerCheckpoint`, `ledgerMode`, `ledgerProjectRoot`, `ledgerWitness`, `ledgerStore`, `ledgerEvidence`, `ledgerDecide`), `ledgerSelftest()` | Implemented |
| Host bridges | `trello-mcp-workflow/hooks/tracker-card-gate.sh::_ledger_mode/_ledger_binary/_ledger_field/_ledger_ask/_ledger_grade`; `lib/_internal/premerge_guardian.py::resolve_ledger_mode/_ledger_binary/_ledger_ask/ledger_blockers`; `plan-build-flow/hooks/plan-build-gate.sh` (work-start) | Implemented (apply-start, pr-review, pre-merge, archive-close, work-start) |
| Doctor rendering (dormancy = doctor only) | `lib/_internal/doctor.py::_check_tracker_ledger`, `_tracker_ledger_in_play`, `_tracker_ledger_binary`, `_tracker_ledger_guidance`, `_tracker_declared` | Implemented (INFO/WARN/ERROR map) |
| Parity corpus + synthetic provider | `tests/fixtures/tracker-ledger-corpus/01..25-*.json`, `tests/fixtures/recipes/test-tracker-ledger{,-conflict}/`, `tests/test_tracker_ledger_parity.py`, `tests/test_tracker_ledger_witness.py` | Implemented (25 rows, `DESIGN_ROWS` pin) |
| Mode config field | `catalog/recipes/trello-mcp-workflow/recipe.toml` `[config.ledger_mode]` (enum `always|ask|warn`, default `warn`) + legacy `gate_mode` | Implemented |

Dogfooded posture (verified in this worktree): `ai-specs/ai-specs.toml` enables
`worktree-flow`, `git-pr-flow`, `session-context`, `tdd-flow`, `trello-mcp-workflow` (`gate_mode = "warn"`,
no `ledger_mode`), `vault-canonical-store` → effective ledger mode `warn`. `plan-build-flow` is **not**
enabled here.

---

## 3. Intentionally deferred by the foundation (non-goals that still hold)

From `archive/2026-09-13-tracker-ledger-foundation/design.md` "Deferred (do not implement)" and D11/D14/D13:
generic/multi-capability ledger; worktree-specific identity or ledger; historical archive migration or
rewrite; provider vocabulary or `## Tracker` parser/schema changes; **MCP/API create/update/move/comment/label**;
module relocation to `go/`; compaction/retention enforcement (advisory ceiling only: `MaxItems = 32`,
`MaxStoreBytes = 64 KiB`); broad Python→Go migration.

Also explicitly written in
`archive/2026-09-13-tracker-ledger-foundation/specs/tracker-ledger/spec.md` (Modes requirement):
*"In this slice `always` MUST NOT perform provider create/update calls; it blocks until a human (or a later
slice) supplies the item."* — this change **is** the "supplies the item" slice, still without provider writes.

**Spec/implementation divergence to resolve in this change.** That spec's scenario
*"Branch reuse after closed work opens a new item"* says the **grade** opens the item
(`WHEN the ledger grades the checkpoint THEN a new open primary item is opened`), but `Grade` is documented
pure and opens nothing; only `store_test.go::TestReusedBranchOpensNewItemD17` exercises `OpenItem` directly.
The opening verb has to move to an explicit write path; the scenario wording must be corrected in the spec
delta, not implemented as "the grader writes the ledger".

### Jinna (non-goal, future provider)

`catalog/recipes/jinna-mcp-recipe/recipe.toml` is an OpenProject **MCP provider** (id `jinna-mcp-recipe`,
`[[provides.mcp]] id = "jinna"`, needs_mcp `["jinna"]`). It declares **no** `[[capabilities]]`, so it is not
a tracker provider and cannot bind `tracker` today. No Jinna integration, config, capability, or adapter
work belongs in this change. A future provider would add a `tracker` capability and reuse the same witness →
`recipe_id` → provider-config lookup seam; nothing provider-specific may enter ledger core
(`ledger/store.go::Item` core fields) or the `## Tracker` contract.

---

## 4. Scope this change should own (the end-to-end production slice)

### 4.1 Item lifecycle write surface (open / link / close)

- **Gap:** no CLI reaches `OpenItem`/`CloseItem`; `Item.ItemID/URL/State/NativeType/Provider` are never
  written by production code (`store.go::OpenItem` only sets `ID`, `Identity`, `Status`, `ProviderID`,
  `Decisions`).
- **Smallest shape:** extend the existing JSON-in-flag pattern instead of adding subcommands. Reuse
  `ledgerOptions` (`gate/ledger_cmd.go`) + `main.go`'s flag block with one new flag (e.g. `--write JSON`
  or `--track JSON`) whose payload is `{kind: open|link|close, item_id, url, native_type, state, provider}`
  plus the existing `--checkpoint`. Then reuse the proven path: parse → validate → locked read-modify-write →
  re-grade → print verdict (exactly the `--decide` flow).
- **`link` is the real adapter verb:** it records the provider's native card id/url/state for the identity
  (provider-neutral core fields), so local vs remote/code comparisons become meaningful.
- **Reuse over new code:** `AppendDecisionToPrimary` already implements the locked append; a
  `PrimaryUnderLock`/`OpenIfAbsent` sibling in `store.go` should follow it exactly (same `withStoreLock`).
- **Dead-weight decision:** either wire `DecisionLink`/`item.Exemption`/`item.state` in this slice or
  explicitly defer them. Leaving them unreachable forever is the worst option.

### 4.2 Code / Git / PR evidence

- **Gap:** `--evidence` accepts a JSON file of `{local, remote, code, git}`, but **no production host ever
  passes `--evidence`** (`grep -n '\-\-evidence' lib catalog` → only tests and `ledger_cmd.go`). So today a
  verdict with an item is always `allow`: the conflict predicate is unreachable in production.
- **Smallest safe slice (3 of 4 sides, no network):**
  - `local` — already the ledger snapshot (`verdict.go::Grade` defaults `ev.Local = item.ItemID`).
  - `code` — parse the change's `## Tracker` `card_id` (and `tracker.none`) with the existing parser
    `lib/_internal/trello_link.py` (foundation A8 keeps it a **parser**, not a grader) plus the `pr:` field
    the skill documents. Reuse rung 2 of the ladder: do not write a new parser.
  - `git` — locally derivable only (branch/HEAD/commit + the `pr:` URL recorded in `## Tracker`). No `gh`
    call on the hot path.
  - `remote` — **deferred**: needs a tracker MCP read. Recorded as a deliberate 3-side gap.
- **Seam:** a thin Python helper (bridge role, D5) that builds the evidence file for the host; hosts then
  pass `--evidence <tmp>`: `tracker-card-gate.sh` (`_ledger_grade`) and
  `premerge_guardian.py::ledger_blockers`. Bad/unreadable evidence already fails open in
  `ledger_cmd.go::loadLedgerEvidence` (stderr + empty evidence) — keep that.
- **Payoff:** the ledger detects `ledger.item_id != ## Tracker card_id` as a `ReasonConflict` — the exact
  "two rules that can disagree" pain the foundation was built for.

### 4.3 `tracker.none` handling

- **Gap:** `verdict.go::resolve` honors `item.Exemption != ""` → `DecisionAllow/ReasonExempt`, but nothing
  writes `Item.Exemption` (only `verdict_test.go:86` sets it). `tracker.none` is documented by the recipe
  README/SKILL/`AGENTS.md` as "presentation, logged and rare" and is currently invisible to the ledger.
- **Host-side gap:** neither `tracker-card-gate.sh` nor `premerge_guardian.py` looks for
  `openspec/changes/<slug>/tracker.none`; `doctor.py` only scans for a `tracking:` declaration.
- **Two candidate mappings — a genuine product decision (§6, Q1):**
  (a) change-scoped exemption: `--write kind=exempt` with the one-line reason → `Item.Exemption`, allowed at
  every checkpoint, auditable in the store; (b) checkpoint-scoped opt-out via the existing
  `PersistDecision` `opt-out` path — no new store concept, but re-prompts at every checkpoint, contradicting
  "logged and rare".
- Note the conflict predicate cannot carry the exemption: `Evidence.Conflict()` compares raw strings, so
  feeding `"tracker.none"` as the `code` side against a `local` card id yields a **conflict**, not an allow.

### 4.4 Provider binding / config lookup

- **Gap: the provider id is hardcoded in three places** even though the witness carries `recipe_id`:
  - `lib/_internal/premerge_guardian.py:501` → `recipes.get("trello-mcp-workflow").config`
  - `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:494` → same literal
  - `lib/_internal/doctor.py::_tracker_ledger_in_play` → `recipes.get("trello-mcp-workflow")`
- **Smallest slice:** resolve the bound recipe id from
  `<git-common-dir>/ai-specs/ledger/witness.json` (`ledger/witness.go::Binding.RecipeID`, exposed in the
  verdict JSON only as `capability`/`item` today — the host reads the witness directly, which it may, since
  reading is not grading), then read `[recipes.<recipe_id>.config].ledger_mode` / `gate_mode`, with the
  legacy `trello-mcp-workflow` key as fallback when the witness is missing/unreadable (fail-open).
- This is also the seam a future provider uses; `board_id`/list names stay in `recipe.toml`, never in core.

### 4.5 Checkpoint ownership

Current owner map and its weakness:

| Checkpoint | Host (production path) | Owner recipe |
|---|---|---|
| `work-start` | `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` | `plan-build-flow` (unrelated to `tracker`) |
| `apply-start` | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` (`kind=path`) | `trello-mcp-workflow` |
| `pr-review` | `tracker-card-gate.sh` (`kind=shell`, `pr_create`) | `trello-mcp-workflow` |
| `pre-merge` | `lib/_internal/premerge_guardian.py --stage pre-merge` | CLI install (not the project) |
| `archive-close` | `premerge_guardian.py --stage pre-archive` | CLI install |

Two ownership facts this change must decide and document:
1. **`work-start` is unhosted in the dogfooded repo.** The ledger activates on the `tracker` capability, but
   its first checkpoint lives in `plan-build-flow`, which `8e87323` removed from this project's
   `ai-specs/ai-specs.toml`. A tracker-bound project can therefore have a ledger that never grades
   `work-start`.
2. **`premerge_guardian.py` ships in the CLI install**, so its ledger path must keep working with a cold
   `AI_SPECS_HOME` and no project cache (foundation risk, still open).
Recommendation: this change **does not relocate a host** (that is a separate risk), but records the owner
map, keeps `plan-build-flow` as the `work-start` host where it is enabled, and makes the unhosted case
visible (doctor or a documented limitation). Content changes to `catalog/recipes/plan-build-flow/**` must be
avoided as instructed.

### 4.6 Idempotency and concurrency

- **Existing:** `store.go::withStoreLock` = `flock(LOCK_EX)` on `<store>.lock`, released by the kernel on
  close (a crashed writer cannot wedge the ledger). `AppendDecisionToPrimary` is the locked
  read-modify-write precedent. `runLedger` loads the store unlocked at start and only `--decide`/conflict
  persist re-lock.
- **Gaps the writer must close:**
  1. **`OpenItem` is not idempotent by design** (`store.go` doc: *"does not check for an existing open item;
     `Primary` reports that collision"*). A retried/duplicated `open` would create a second open row →
     self-inflicted `ErrMultipleOpen` conflict for a human. The writer must be open-if-absent.
  2. **`NewItemID` collides inside one second:** `sha256(identityKey + "\x1f" + openedAt)` truncated to 16
     hex, with RFC3339 second precision. Two opens in the same second for one identity produce the **same
     item id**.
  3. **Unbounded lock wait on a hot path:** `Flock(LOCK_EX)` blocks forever. A wedged `--write` from a hook
     would stall edits. Recommend a bounded attempt (`LOCK_NB` + short retry) with different posture per
     caller: grade paths fail open, write paths fail closed.
  4. **Change-slug ambiguity merges work:** `DeriveIdentity` omits the slug when several active change
     folders exist and only sets `Collision`. Two changes on one branch then share one identity key → one
     shared item. The writer must refuse (fail closed) on `Collision == change-ambiguous` unless the caller
     supplies an explicit slug.
  5. **Idempotent `link`/`close`:** linking identical `item_id`/`url` twice must not append duplicate
     decisions; closing an already-closed item must not create one and must not silently succeed as a no-op
     without a signal.

### 4.7 Fail-open / warn / ask semantics

Already implemented and to be preserved verbatim:
- exit `0` for `allow|ask|dormant|unevaluable`; exit `2` only on `block` or failed `--decide` persist
  (`verdict.go::ExitCode`, `ledger_cmd.go`); a `--ledger` flag-parse error fails open (`main.go`); a
  destructive mode refuses (cleanup precedent).
- dormant/unbound → skip the ledger; corrupt store → `unevaluable` + doctor ERROR; `openspec/**` writes never
  blocked (`tracker-card-gate.sh` path branch exits 0); missing/unverified binary → one stderr line + exit 0
  + doctor ERROR.
- `ask` → `_ledger_ask` prompts on `/dev/tty`, persists `kind=opt-out choice=continue`, re-grades;
  `identity_unavailable` is reported and **not** recorded (no durable key) in both
  `tracker-card-gate.sh` and `premerge_guardian.py::ledger_blockers`.
- Remaining friction to decide/record (user asked for friction items to be recorded in Vault; Vault is
  unavailable, so they are recorded here and in §6):
  1. **`ask` with no terminal blocks.** Both hosts return `2` when `/dev/tty` cannot be opened. In an
     agent/CI harness there is typically no tty, so `ask` converts every checkpoint into a hard block.
  2. **No evidence ⇒ no conflict ⇒ permanent allow.** §4.2 is what makes `warn`/`ask` meaningful.
  3. **Stale header comment** in `tracker-card-gate.sh` still lists `shell ∈ {pr_create, archive}` after
     `archive-close` moved to the guardian (recorded by the foundation verify report, never fixed).
  4. **Trust root regeneration** is mandatory for any Go change (see §5) — mechanical, but it is the step
     most likely to be forgotten.
  5. **`--write` failure posture** must be explicit (recommend fail-closed exit `2`, like `--decide`).

---

## 5. Tests, trust root, and exact surfaces

Existing suites that must stay green and are the natural home for new rows:
- Go: `catalog/recipes/worktree-flow/gate/ledger/{identity,store,verdict,decide,witness}_test.go`,
  `catalog/recipes/worktree-flow/gate/ledger_cmd_test.go` (`TestLedgerTwoOpenItemsConflict`,
  `TestLedgerItemPopulatedWhenOpenItem`), `main_test.go`.
- Corpus + parity: `tests/fixtures/tracker-ledger-corpus/` (25 rows, `DESIGN_ROWS` one-per-row in
  `tests/test_tracker_ledger_parity.py`; runner drives `dist/worktree-gate-current`, with residue guard
  `LEDGER_ALLOWED_NAMES = {witness.json, state.json, state.json.lock}`, two-run stability, and a mutation
  guard).
- Hosts: `tests/test_tracker_card_gate_hook.py`, `tests/test_plan_build_gate_hook.py`,
  `tests/test_premerge_guardian.py`, `tests/test_ledger_mode_config.py`
  (`test_all_five_hosts_block_on_the_same_verdict` / `..._allow_...`), `tests/test_doctor_tracker_card.py`.
- Witness/sync: `tests/test_tracker_ledger_witness.py` (14 tests: four states, trap survival, atomic write,
  linked worktree).
- Recipe: `tests/test_trello_mcp_workflow_recipe.py` (asserts `tracker.none` in rules + skill),
  `tests/evals/scenarios/trello-mcp-workflow/ac_missing_card_gate_no_bash_skip/`.

New tests this change should add (RED first, per `strict_tdd` in `openspec/config.yaml`):
1. `ledger/write_test.go` — open idempotency, open under a lock, second-open-in-same-second, link, close,
   close-after-close, `change-ambiguous` refusal, `-race -count=3` concurrency, no `state.json.tmp.*` residue.
2. Corpus rows + `DESIGN_ROWS` entries: open-then-allow, code-vs-ledger conflict, `tracker.none` exemption,
   idempotent re-open, link mismatch.
3. `tests/test_ledger_evidence.py` (new) — `## Tracker` → evidence `code`; missing section;
   `tracker.none` → exemption/skip; malformed artifact fails open.
4. Extend the three host suites — `--evidence` actually passed, provider-id mode lookup, failure posture.
5. Keep `./tests/validate.sh` (runs `py_compile`, `bash -n`, `gofmt -l`, then `tests/run.sh` →
   `go -C catalog/recipes/worktree-flow/gate test ./...` + `unittest discover`) as the single gate.

**Trust root (mandatory, same PR):** any change under `catalog/recipes/worktree-flow/gate/**` changes the
binary, so `catalog/recipes/worktree-flow/bin/SHA256SUMS` (four targets) must be regenerated via
`scripts/build-gate.sh` + `scripts/verify-gate-sums.sh`, `dist/worktree-gate-current` rebuilt for the parity
runners, and `.github/workflows/release-worktree-gate.yml` stays the only release path. No new module, no new
asset, no new dependency (`go.mod` unchanged; `go list -deps ./ledger` = stdlib only).

Docs to update when behavior lands: `docs/capabilities.md` (~L70-105), `docs/runtime-hooks.md` (~L228-252
ledger + checkpoint-host table), `catalog/recipes/trello-mcp-workflow/README.md` ("No provider writes in
this slice" paragraph and the `## Tracker`/`tracker.none` paragraph), the skill's `## Tracker` section,
`CHANGELOG.md`.

---

## 6. Unresolved product decisions (proposal question round)

1. **What does `tracker.none` mean to the ledger?** Change-scoped exemption (stored as `Item.Exemption`, allowed
   at every checkpoint) or the existing checkpoint-scoped opt-out (re-prompts at the next checkpoint, no new
   store concept)? Who writes it — the agent, or the host when it sees the file?
2. **Who opens the item?** The agent explicitly running the writer (`--write kind=open`) as part of the
   `trello-card-linking` capability, or the host auto-opening on the first successful `## Tracker` parse?
   (Auto-open makes the "missing item" verdict vanish without a human act; explicit keeps the ledger honest.)
3. **Is 3-sided reconciliation (local/code/git) enough for this slice**, with `remote` deferred, or is a
   read-only tracker MCP read required now? D11 only forbade create/update, so a read is technically allowed.
4. **`ask` with no terminal:** keep the current hard block (exit 2), or report-and-proceed like
   `identity_unavailable`? This is the difference between `ask` being usable in agent harnesses and not.
5. **`work-start` ownership:** leave it in `plan-build-flow` (unhosted in this dogfooded repo), or move it to
   the tracker recipe because the ledger activates on the `tracker` capability?
6. **`--write` failure posture:** fail closed like `--decide` (recommended), or degrade to warn?

---

## 7. Risks

| Risk | Notes / mitigation |
|---|---|
| Trust-root regeneration (four-arch `SHA256SUMS` + `dist/` rebuild) | Mechanical but load-bearing; parity suites skip loudly without `dist/worktree-gate-current`. Do it inside the same work unit as the Go change. |
| **Two open items from a retried open** | `OpenItem` is deliberately non-idempotent; the writer must be open-if-absent under the lock, or the first retry creates a human conflict. |
| Item ID collision within one second | `NewItemID` = second-precision hash; needs a nonce or an explicit duplicate-id guard. |
| `change-ambiguous` silently merges two changes into one item | Fail closed in the writer when `Identity.Collision != ""`. |
| Unbounded `flock` wait on a pre-tool-use hook | Bounded `LOCK_NB` retry; grade paths fail open, write paths fail closed. |
| `ask` unusable without a tty | Friction #1 in §4.7 — decide or the mode stays theoretical. |
| Evidence acquisition grows the hot path (python3 parse per edit) | Reuse `trello_link.py` (pure parse), no `gh`/network, and keep any failure fail-open. |
| `premerge_guardian.py` cold-cache path | Already an open foundation risk; the evidence bridge must not add a cache dependency. |
| Spec/implementation divergence on "grade opens the item" | Correct the spec wording in the delta; keep `Grade` pure. |
| Provider coupling persists (`trello-mcp-workflow` hardcoded ×3) | §4.4; fallback keeps behavior identical when the witness is absent. |
| Scope creep (provider writes, Jinna, plan-build-flow edits, archive migration) | Explicit non-goals; `catalog/recipes/plan-build-flow/**` untouched; no MCP create/update. |
| Review budget | The foundation slice was 9,796 insertions / 61 files under an accepted `size:exception`. A writer + evidence bridge will exceed 400 lines; a delivery decision (chained PRs) will be needed via `ask-on-risk` when the proposal reaches delivery. |
| Vault unavailable (configured scope path missing) | Friction items 1-5 in §4.7 and the merge/cleanup notes cannot be recorded in Vault now; must not be substituted with a filesystem path and must not become tracker follow-up cards (per the user's stated preference). Re-record when the scope path exists. |

---

## 8. Recommendation for proposal scope

**Own one slice: "the bound Trello recipe becomes a real ledger adapter, without provider writes."**

Work-unit sketch (each independently reviewable, in dependency order):

1. **Writer + idempotency (Go).** A machine-write surface for `open|link|close` (plus optional `exempt`)
   reusing the `--decide` flow; `store.go` gains a locked, idempotent open-if-absent and a re-link/close
   policy; bounded lock attempt; `change-ambiguous` refusal; duplicate-id guard; `ledgerSelftest` extended;
   `ledger/write_test.go`. Regenerate the trust root.
2. **Evidence bridge + `tracker.none` (Python + hosts).** One bridge that turns `## Tracker` /
   `tracker.none` / local git facts into the `--evidence` JSON; `tracker-card-gate.sh` and
   `premerge_guardian.py::ledger_blockers` pass it; corpus rows + `tests/test_ledger_evidence.py`;
   `tracker.none` mapped to the decision from §6-Q1.
3. **Provider-id binding lookup + docs.** Replace the three hardcoded `trello-mcp-workflow` lookups with the
   witness `recipe_id` (+ legacy fallback); record the checkpoint-owner map; update `docs/capabilities.md`,
   `docs/runtime-hooks.md`, the recipe README/skill, `CHANGELOG.md`; keep the stale-comment fix in the same
   unit.

**Explicit non-goals for this change:** provider MCP/API create/update/move/comment/label (still D11); remote
evidence if §6-Q3 says defer; moving `work-start` if §6-Q5 says keep; `plan-build-flow` content; Jinna or any
provider adapter beyond the bound-recipe lookup; a generic ledger; compaction; archive migration; a second Go
module/asset.

**Dead-weight rule:** whatever this change does not wire (`DecisionLink`, `Item.Exemption`, `Item.State`,
`AppendDecisionToPrimary`, `item.native_type`) must be named in the proposal as deliberately deferred or
deleted — not left as unreachable code that reads like a implemented feature.

**Delivery expectation:** expect >400 changed lines plus a regenerated trust root; surface the delivery
decision (chained PRs vs `size:exception`) at the proposal's delivery step under `ask-on-risk`, not by
choosing silently.
