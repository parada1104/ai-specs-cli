# Pre-Merge Guardian Ownership and Ledger Ports

## Objective

Make the pre-merge artifact guardian execute only when the Plan Build/OpenSpec flow is active, move artifact/archive governance out of provider-specific VCS merge workflows, and close the pre-archive spec-promotion gap. At the same time, establish an autonomous ledger core with a Tracker-domain port: provider recipes extend the ledger by mapping native states to neutral semantic expectations and observations.

This is an Organic Driven Development change. It does not select the SDD phase chain or create an OpenSpec change folder unless a later explicit decision changes the workflow. The ODD task document is the parent recovery authority until implementation is authorized and a tracker card is linked.

## Problem

`premerge_guardian.py` currently combines two independent hosts: Plan Build artifact/tier/archive validation and tracker-ledger checkpoint grading. GitHub, GitLab, and Bitbucket merge skills invoke it unconditionally, so a project that does not use Plan Build inherits SDD/OpenSpec ceremony. Plan Build's archive-tail currently moves the change folder but does not promote delta specs into `openspec/specs/`; the first pre-archive guardian therefore accepts unpromoted Standard/Full deltas and canonical specifications silently drift.

The tracker ledger itself is more generic than its host: its Go store and pure `Reconcile` comparator can support a Tracker domain, while current witness/checkpoint/evidence plumbing is tracker-shaped. Tracker closure must remain independent of Plan Build and must never be inferred from OpenSpec archive state.

## Confirmed architecture

```text
Autonomous ledger core
  └── Tracker domain port
        ├── Trello recipe adapter
        ├── Jira recipe adapter (future)
        └── Linear recipe adapter (future)

Plan Build / OpenSpec
  verify → promote delta specs → artifact guardian → archive

VCS controller
  PR/MR → merge → worktree/branch cleanup
```

- One authoritative Go grader and one trust root remain.
- Provider recipes supply native-state mappings and observations; they do not configure ledger behavior or implement grading.
- The artifact guardian remains read-only and never writes tracker state or canonical specs.
- Existing wire checkpoint names remain compatible unless a later explicit decision approves a migration.
- No provider-specific Go implementation, automatic provider write, network hook, or second grader is introduced.

## Work units

### W1 — Separate artifact guardian from tracker host

Extract the tracker-ledger acquisition/JSON host currently fused into `premerge_guardian.py::main()` into an explicit tracker-domain host/entry point. Leave `premerge_guardian.py` responsible only for Plan Build artifact, tier, verify, archive, and promotion-parity checks. Preserve the current Go `--ledger` predicate, witness resolution, evidence bridge, fail-open grade behavior, and fail-closed decision/write behavior.

**Acceptance:** artifact guardian tests cannot observe tracker calls; tracker host tests prove `pre-merge` and `archive-close` remain available without Plan Build; no duplicate grading logic exists.

**Observed evidence:** `python3 -m unittest tests.test_ledger_mode_config tests.test_premerge_guardian tests.test_tracker_ledger_host` — 78 tests OK; `./tests/run.sh` — exit 0. `premerge_guardian.py` no longer contains tracker symbols or invokes the Go ledger; `tracker_ledger_host.py` carries the extracted acquisition-only host and preserves the wire checkpoints.

### W2 — Define the autonomous ledger and Tracker port seam

Generalize the boundary around the existing pure `ledger.Grade`/`ledger.Reconcile` seams. Keep Tracker as a domain port, not a provider: Trello/Jira/Linear recipes map native statuses/lists/fields to neutral semantic events and properties. Make recipe configuration an adapter input rather than a switch that enables or changes ledger behavior. Preserve the existing Trello mapping and witness compatibility while documenting the future extension point.

**Acceptance:** the core has no Trello/Jira/Linear vocabulary; one declarative adapter can feed expectations/observations; absent or unbound adapters yield dormant/unconfigured behavior without guessing or changing core policy; existing Trello parity corpus stays green.

**Observed evidence:** Added a non-provider `fixture-tracker` adapter case feeding the same pure comparator, a policy-vs-mapping rejection case, production-Go provider-vocabulary guard, and Tracker-domain adapter documentation. `python3 -m unittest tests.test_ledger_mode_config tests.test_tracker_ledger_parity tests.test_tracker_ledger_host tests.test_premerge_guardian` — 87 tests OK (6 binary-corpus skips); `./tests/run.sh` and `./tests/validate.sh` — 2032 tests OK (142 skips). No Go production bytes changed, so the existing trust root remains valid.

### W3 — Make Plan Build promote specs before archive

Align Plan Build's archive lifecycle with the existing SDD archive composition contract. Before the first pre-archive guardian and before moving the change folder, compose applicable ADDED/MODIFIED/REMOVED delta requirements into canonical `openspec/specs/` with collision, destructive-change, and idempotency safeguards. Keep the guardian read-only: it validates canonical/delta parity and blocks Standard/Full when promotion is missing or unresolved; Light and ODD task-only bridges remain unaffected.

**Acceptance:** a Standard/Full change cannot archive an unpromoted delta; a promoted delta passes; repeated/archive-resumed composition is safe; REMOVED and same-domain collision behavior is explicit; a concrete regression covers the previously observed `Specs Synced: None` drift.

**Observed evidence:** Added `spec_promotion.py` as the explicit stdlib-only writer and shared read-only parity checker. ADDED/MODIFIED/new-domain/idempotent/collision/REMOVED/RENAMED/path/symlink/all-or-nothing cases are covered; the guardian blocks unpromoted Standard/Full deltas and remains tracker-free. `python3 -m unittest tests.test_spec_promotion tests.test_premerge_guardian tests.test_plan_build_flow_recipe` — 114 tests OK; `./tests/run.sh` — 2067 tests OK (142 skips). Collision detection was added and revalidated; no Go/trust-root files changed.

### W4 — Remove SDD artifact ownership from VCS merge workflows

Update GitHub, GitLab, and Bitbucket merge skills so they own provider transport, review/merge, and worktree cleanup only. Remove unconditional Plan Build artifact preconditions, archive-tail ownership, and artifact guardian calls from VCS guidance. Update `vcs-pr-flow` and `plan-build-flow` contracts, catalog docs, and golden tests so VCS-only projects can merge without Plan Build while Plan Build projects retain their own archive gate.

**Acceptance:** VCS-only merge guidance does not require `openspec/changes/<slug>/`, archive artifacts, or `premerge_guardian.py`; Plan Build guidance still requires promotion → guardian → archive; provider siblings remain behaviorally aligned.

**Observed evidence:** Removed the SDD artifact precondition, archive-tail ownership, and guardian invocation from GitHub, GitLab, and Bitbucket merge skills; each now states that Plan Build owns artifacts when enabled. Provider-specific auth, approval, SHA/protected-head, merge, and cleanup contracts remain. `python3 -m unittest tests.test_git_pr_flow_recipe tests.test_gitlab_mr_flow_recipe tests.test_bitbucket_pr_flow_recipe` — 141 tests OK; `./tests/run.sh` — exit 0, 2076 tests OK.

### W5 — Keep Tracker lifecycle independent of Plan Build and VCS artifact policy

Give the Tracker-domain host an explicit lifecycle trigger/command for its `pre-merge` and `archive-close` checkpoints that does not depend on Plan Build artifacts or a VCS-specific command parser. Keep tracker item close semantics and ODD closure independent. Preserve checkpoint wire names for compatibility and make the distinction between tracker item close and OpenSpec archive explicit in docs/doctor.

**Acceptance:** a tracker-bound ODD change can close its item without Plan Build or OpenSpec archive; a Plan Build change can archive without tracker details when no Tracker adapter is bound; the same Go ledger predicate is used at every hosted checkpoint.

**Observed evidence:** Added direct `--checkpoint pre-merge|archive-close` to the provider-neutral Tracker host while retaining `--stage` compatibility. The host works without an `openspec/` tree, rejects contradictory flags before grading, emits no writes, and keeps the same Go predicate. Tracker recipe skill/quick-reference/README/brief and runtime docs now distinguish Tracker `archive-close` from OpenSpec archive. `python3 -m unittest tests.test_tracker_ledger_host tests.test_trello_mcp_workflow_recipe` — 36 tests OK; broader contract/recipe suites — 551 tests OK; `./tests/run.sh` — 2084 tests OK (142 skips).

### W6 — Update canonical contracts, migration notes, and acceptance evidence

Promote the changed contracts into the canonical tracker-ledger, plan-build-flow, and vcs-pr-flow specifications; update runtime-hook/capability documentation and recipe tests. Link the existing guardian backlog card (prefer #113; #114 is a duplicate) before production writes, record the final tracker link here, and run focused plus full validation and live Trello/ODD acceptance.

**Acceptance:** all changed requirements have executable regression coverage; no stale contract still says the VCS skill owns the artifact guardian; `./tests/validate.sh`, Go tests, focused recipe tests, checksum/build checks, and live acceptance pass.

**Observed evidence (implementation/validation):** Canonical Plan Build, Tracker Ledger, and VCS contracts now match the ownership split; the Plan Build gate resolves witness-bound recipe policy; stale guardian references are removed. `./tests/validate.sh` — 2109 tests OK (142 skipped), exit 0, after the final native-review correction; focused contract/recipe suites and shellcheck were also run (shellcheck reports only three pre-existing SC2034 warnings). Native review `review-c1af30148cdc29ad` froze the worktree against the `development` HEAD tree, admitted all four lenses, required and validated a bounded 6-line correction for `R4-malformed-delta-silent-noop`, then completed approval and acknowledgement. Final PR/live acceptance remains pending.

## Scope

### In scope

- Ownership boundary between Plan Build artifact governance and Tracker ledger lifecycle.
- Canonical spec promotion before Plan Build archive.
- Autonomous ledger core/Tracker-domain port contract and declarative provider mapping seam.
- GitHub, GitLab, and Bitbucket skill/spec/docs alignment.
- Trello behavior-preserving adapter/parity coverage.

### Out of scope

- Implementing Jira, Linear, or other provider adapters beyond the generic contract and Trello compatibility.
- New provider network calls or automatic provider writes.
- A second grader, a new runtime event schema, or a parallel ledger framework.
- Rewriting historical archives except where a migration explicitly proves an affected contract.
- Changing the user-owned tracker status/list mapping policy.

## Acceptance criteria

1. The pre-merge artifact guardian is Plan Build-owned and does not invoke tracker logic.
2. Plan Build promotes Standard/Full delta specs before the pre-archive guardian and archive-tail; unpromoted specs cannot silently archive.
3. VCS merge workflows do not impose SDD/OpenSpec planning or archive ceremony when Plan Build is absent.
4. Tracker ledger closure remains independently executable in ODD and SDD; tracker item close is never inferred from OpenSpec archive state.
5. Recipes extend the autonomous ledger through Tracker-domain mappings; core behavior remains provider-neutral and one Go grader remains authoritative.
6. Existing Trello ledger, VCS, Plan Build, archive-path, and trust-root contracts remain green, with new regressions for the ownership and promotion gaps.

## Risks and checks

- **Contract migration:** `tracker-ledger` currently pins pre-merge/archive-close to the guardian and forbids a generic capability ledger; `vcs-pr-flow` currently owns archive timing. Update these deliberately in one coherent contract delta.
- **Trigger fidelity:** runtime hooks expose only pre-tool-use/session events, not first-class merge/archive events. Prefer an explicit domain host/command over heuristic provider command parsing.
- **Canonical spec collisions:** promotion must preserve unrelated requirements, reject unresolved same-domain collisions, and treat destructive REMOVED operations explicitly.
- **Backward compatibility:** retain `pre-merge`/`archive-close` wire values, witness version compatibility, closed observation shape, and Trello corpus behavior.
- **Verification:** use `python3 -m unittest` focused guardian/recipe suites, Go ledger tests, `./tests/validate.sh`, canonical gate build/checksums, and live Trello plus tracker-only ODD acceptance.

## Open technical decisions (recommendations)

- Use an explicit domain host/command rather than adding merge/archive runtime-hook event types.
- Keep the first port declarative around `Reconcile`/expectations/observations; add a formal Go interface only where it removes real duplication.
- Keep checkpoint wire names and migrate naming only in a later compatibility-scoped change.
- Keep generic Golden consumption on the existing pure comparator seam; do not create a new ledger framework.

## Tracker

- **card_id**: `6a93705377a8419254748437`
- **shortLink**: `XwdM0Dmi`
- **url**: https://trello.com/c/XwdM0Dmi/113-guardian-convert-premergeguardian-into-an-automated-gate-hook-ci-align-parser-with-sdd-report-format
- **list**: In Progress
- **note**: Duplicate #114 is not used for this change.
- **PR**: pending.

## Progress

- [x] Exploration: guardian ownership, ledger coupling, Plan Build archive lifecycle, and spec-promotion gap validated.
- [x] Product boundary: autonomous ledger core + Tracker port + provider mappings; Plan Build artifact guardian independent.
- [x] W1 — separate artifact guardian from tracker host; focused and configured suites green.
- [x] W2 — define the autonomous ledger and Tracker port seam; adapter and neutrality contracts pinned.
- [x] W3 — make Plan Build promote specs before archive; parity and active-domain collision checks are green.
- [x] W4 — remove SDD artifact ownership from VCS merge workflows; provider symmetry tests are green.
- [x] W5 — keep Tracker lifecycle independent of Plan Build and VCS artifact policy; direct host route is green.
- [x] W6 — update canonical contracts, migration notes, and acceptance evidence; automated evidence and native review are green.
- [x] Link/confirm tracker card #113 before first production write.
- [ ] Final verification, acceptance, PR, archive, and merge.

## Next step

Commit/push this normalized candidate and open the PR after the final native review. Do not merge without a new explicit user authorization.
