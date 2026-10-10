# Go 07 S12.2 — Template + docs actuators (native authority)

## Objective
Add the native root authority for the template actuator and the docs flow
policy in `internal/sync`.
Reuse the shared classify core (`classifyManagedOverride`) instead of
re-porting classification.
Verify against the real Python oracle.
Keep the sync CLI unwired until S14.

## Tracker
- **card_id**: `6ac8675b182833f019d256b1`
- **url**: https://trello.com/c/APUQZNIf
- **list**: In Progress

## Scope
- Base: epic/go-single-binary at 8b16d11 (classify shared core integrated, PR335).
- Worktree: .worktrees/go-07-s12-templates.
- Branch: change/go-07-s12-templates.
- Add internal/sync native authority for template actuation (dest resolution,
  rendering, classification, write + chmod, record payload) and the docs flow
  policy that sits above the copy authority from S12.1.

## Constraints
- Parent and Pi subagents only. No Herdr or Orca.
- User authorizes commit, push, PR, and merge into the epic after checks.
- Do not modify development or publish releases.
- Delivery A budget: 1200 additions plus deletions, including tests and documents.
- Delivery B exception: human approved 1300 additions plus deletions, including final verification evidence.
- Do not modify production Python. Python remains the differential oracle.
- Gate edits only extract the existing template actuator into shared code.
- The gate JSON wrapper must preserve its existing contract.
- Count relocated code as additions plus deletions. No move-adjusted exception.
- Do not wire the sync CLI. S14 owns orchestration and caller diagnostics.
- Do not extract the gate-hooks actuator here (follow-up unit of S12).
- Reuse the shared classify core rather than duplicating classification logic.
- Behavior tests require observed RED, GREEN, then proportionate refactoring.
- Run full validation before committing.

## Tasks
- [x] T1: Scout contract evidence recorded (template actuator, docs flow, reuse mechanism).
- [x] T2: Add the native authority contract tests and observe RED.
- [x] T3: Implemented absolute-target and slash-anchor fixes; GREEN and regression probes observed. Test-first chronology for this correction is unverified.
- [x] T4: Completed valid-input oracle coverage; prior policy/key/.git fixes pass independent checks.
- [x] T4a: Split delivery into shared extraction and root authority. Delivery A is isolated and self-checks passed; delivery B remains preserved.
- [x] T5: Run full validation and independent verification. **Done**: full suite exit 0, focused differential green, two independent verifier rounds PASS.
- [ ] T6: Commit, publish, merge into the epic, close the tracker, and clean the worktree.

## Acceptance criteria
- Native outcomes match the Python oracle per scenario: write, seed, skip,
  user-modified warn, managed-stale refresh, policy refusals.
- Exact Python strings preserved for lines, warnings, and info messages.
- Lock record payload fields match Python's set_managed_override output.
- Classification goes through the shared core, with no second port.
- Docs flow policy matches Python pre-existence handling on top of copy items.
- Root tests, vet, and full validation pass, and both parity modes show zero deltas.
- Each delivery respects its budget: A at most 1200 lines; B at most the approved 1300 lines.

## Checks
- Focused RED/GREEN: go test ./internal/sync/ -count=1 -v (template/docs focused run).
- Root suite: go test ./cmd/... ./internal/... and go vet ./internal/sync/.
- Gate suite unchanged: go test ./... inside the gate module.
- ./tests/validate.sh with durable exit records and bounded waits.
- Native review follows the user-owned switch and uses this slice base.

## Progress
- Parent rejects the worker's move-adjusted accounting: approximately 1857 raw lines exceed the 1200-line delivery budget.
- Proposed delivery A: gate shared template extraction plus task evidence.
- Proposed delivery B: root template/docs authority plus its tests and task evidence, based on A.
- B totals 1295 lines at the path-gap fix (1204 after comment restoration): 256 source + 915 tests + 124 task doc. The human approved a B-only ceiling of 1300, including final evidence.
- Comments were restored without behavior changes. The 24-case oracle and normal/linked .git checks still pass in worker self-checks.
- Independent verifier mv0go1qe-3-3wjm timed out without usable evidence. No verdict was accepted.
- Retry verifier mv0h0rsz-4-me1b returned PARTIAL without source edits: focused tests pass, but two Python divergences remain.
- FIXED empty-policy overwrite: DocRequest.ManagedEntry is now a doc-specific DocManagedEntry{Policy string; HasPolicy bool}, so an absent policy (auto) stays distinct from an explicit "" (preserve + confirm-required warn).
- FIXED noncanonical lock keys: the root uses posixAsPosix (Path.as_posix) for the record/lookup key while every diagnostic and the copy id stay raw; '..' is preserved, not Clean-ed.
- The oracle now seeds/reads managed entries under Path(target).as_posix(), so the differential catches a raw-key mismatch. Template record targets are normalized at the B wrapper too (t9); the gate contract is untouched.
- Correct only root authority and its tests. Shared delivery A files remain unchanged during validation.
- Added d10/d11 (empty vs missing policy), d12/d13 ("."/"//"), d14 (".." preserved), d15 (dangling symlink: bytes + record), and t9 (template key) scenarios.
- Added TestMaterializeTemplateGitDestParity: normal + linked TEMP Git worktrees, realpath identity, bytes/hash/mode/record vs the real Python oracle. Result: identical; the linked worktree lands in the primary repo's shared hooks dir.
- Delivery A has a clean isolated worktree: .worktrees/go-07-s12-template-core, branch change/go-07-s12-template-core.
- Worker mv0h2j83-5-n2rh completed the transfer of only the two gate extraction files into delivery A.
- Delivery A self-checks passed: root and gate tests, gate vet, formatting, source equality, and diff checks.
- Parent measured A at 1106 changed lines before verification evidence updates, counting both sides of moves.
- Native ASSESS reports high risk for A and requires independent verification.
- Verifier mv0h9cqy-6-w724 returned PARTIAL for A: source comparison and Go checks pass, but full validation exited 1.
- Python reported 19 failures and 1 error. Both 29-fixture parity modes pass; base attribution remains unproven.
- Explorer mv0j5tin-a-4nu3 traced A's rejection to committed-versus-built checksums. Verifier mv0jylkp-c-woug compares pinned-toolchain base and candidate before metadata changes.
- Parent verified the live board and card, then posted progress comment 6ac86f497f77f0cfbb567e78.
- Card stays In Progress until both deliveries integrate.
- ASSESS failed on undeclared untracked files; candidate remains unassessable and requires independent verification.
- Full validation, native review, commits, PRs, and merges remain pending.
- Worktree created at base 8b16d11; clean.
- Tracker card linked before source writes.
- T1 scout done (read-only explorer, task mv0fa60b-1-lv0m):
  - Python template path is a bridge: go_materialize_template recipe-materialize.py:1088 drives `worktree-gate --materialize-template`; fallback _python_materialize_template :1430; materialize_template :1261; refusals :1357-1428; render_template_bytes :1665; render_override_bytes util.py:651; sha CRLF-normalized util.py:659.
  - Gate Go twin exists but is package main (unimportable): gate/templateactuator.go renderTemplateBytes :179, resolveTemplateDest :223, writeTemplateContent :382, runMaterializeTemplate :415. Extraction to gate/shared follows the SX0c/SX0f precedent.
  - Docs flow: materialize_doc recipe-materialize.py:1551-1632, pre-existence policy with classify would_write=src bytes, seed/warn/refresh/backfill/skip, copy via go_apply_copy (native side exists in S12.1 ApplyCopyItems). Loop order :4033-4060: skills, commands, templates, docs, hooks. Docs consume no merged_cfg.
  - Reuse: root go.mod require/replace ai-specs.dev/worktree-gate; shared.ClassifyManagedOverride (gate/shared/classify.go:82) pure, no I/O; shared.Sha256Bytes :76 matches util.sha256_bytes. internal/sync/agentsrender.go:1118 has a private duplicate classifyManagedOverride — out of scope, follow-up only.
- T2 done (shared extraction + RED):
  - gate/shared/templateactuator.go: RenderTemplateBytes, ResolveTemplateDest, WriteTemplateContent, EnsureTemplateAncestorsReal, refusal strings, ManagedRecord, Refusal, MaterializeTemplate (envelope-independent shell). Gate main is now a thin JSON wrapper + historical-name aliases; gate suite unchanged.
  - RED observed: `go test ./internal/sync/ -run TestMaterialize` failed to build — undefined DocRequest, MaterializeDoc, MaterializeTemplate.
- T3 done (GREEN + probes):
  - internal/sync/templatedocs.go MaterializeTemplate delegates to shared.MaterializeTemplate; MaterializeDoc mirrors materialize_doc over ApplyCopyItems; returns record payloads, no lock/manifest write.
  - GREEN: `go test ./internal/sync/ -count=1` ok; `go test ./...` in gate module ok; `go vet ./internal/sync/` clean; gofmt clean.
  - Probes (fail then restore): wrong ResolveTemplateDest join → t1 fails; wrong MaterializeDoc join → d1-d3 fail; no CRLF fold in Sha256Bytes → gate TestTemplateActuatorCRLFShaParity fails. All restored green.
- T3/T4 fix evidence (observed RED then GREEN, discrimination probes restored):
  - RED d10 (scenario 17): native refreshed the empty-policy stale doc; Python preserved it with the confirm-required warning.
  - Probe 1: `target := raw` → scenario 19 (d12) "target":"./..." vs normalized; restored green. Probe 2: treat "" as auto → scenario 17 diverges; restored green. Probe 3: template record raw → scenario 23 (t9) diverges; restored green.
  - Final GREEN: `go test ./internal/sync/ -run TestMaterialize -count=1 -v` ok; `go test ./internal/sync/ -count=1` ok; `go vet ./internal/sync/` clean; gofmt clean.
  - `.git` verification: TestMaterializeTemplateGitDestParity normal + linked temp worktrees pass; Python and native write the SAME realpath (`.git/hooks/pre-commit` under the primary repo), same bytes/mode/record. Python's linked dest is spelled through the macOS `/private` realpath; identity is compared with EvalSymlinks.
  - Verifier disproved the relative-only assumption: schema accepts any string target. Absolute targets and exactly two leading slashes require parity fixes.
- Initial T4 evidence (reopened after independent verification):
  - TestMaterializePythonDifferential drives the real entry points (_python_materialize_template, materialize_doc) via python3 + lib/_internal/recipe-materialize.py over 27 scenarios (templates t1-t9, docs d1-d18). Compares wrote, dest identity, record fields, disk sha, and every emitted line. Green.
  - Discrimination probe: perturbed the "✓ template" message → differential "scenario 0 diverges"; reverted green.
  - Divergence vs task text item 4: the doc dir/symlink pre-existence path has NO skip line. Python materialize_doc returns before printing it (:1570-1577), so the native result emits the warn only; the differentiator is the oracle.
  - The earlier "by-design exclusions" note was wrong: the normalization mismatch is reproduced and now fixed; normal/linked .git parity is now verified, not merely decision-level.
- Verifier mv0itdpr-9-zsxb returned PARTIAL: previous fixes, 24-case oracle, .git tests, comments and B budget pass.
- FIXED both path blockers: pyPathJoin reproduces pathlib `base / part` (an absolute part replaces the root; '..' is preserved so the filesystem resolves it) and posixAsPosix keeps exactly two leading slashes while three or more collapse to one. Applied to BOTH the Source and dest joins.
- Retrospective regression probes: pre-fix joins fail scenarios 21 and 24; slash collapse fails scenario 25. GREEN after restore; no test-first chronology inferred.
- The oracle now emits and compares the projected DEST identity AND the bytes at that destination for docs. New cases: d16 (absolute dest), d17 ('//' dest), d18 (absolute source). Differential is 27 cases plus the .git tests.
- Limitation: a relative '..' through a SYMLINK ancestor is preserved in the join, but copyapply's MkdirAll Cleans the parent; not exercised (copyapply is out of B scope). Correct Python path semantics in root B only; no new schema restriction and no change to shared A.
- B sits on the epic tip ffdbbb6 (delivery A merged as PR #336) and delegates to `shared.MaterializeTemplate`; it re-ports no classification or actuation logic.
- Candidate measured at 1295 lines (256 source, 915 tests, 124 document) before this evidence, inside the approved 1300-line B ceiling.
- Focused after the rebase: `go test ./internal/sync/ -count=1` ok; the verbose differential run reports 36 subtests (28 oracle scenarios t1-t9 and d1-d18, 6 refusals/boundary, 2 .git dest parity) with zero skips; `go vet` and `gofmt -l` clean.
- Independent verification round 1: PASS on the slice-local claims, including the real oracle path (python3 plus lib/_internal/recipe-materialize.py), scope discipline against lib/**, internal/cli/** and internal/schema/**, and the merged shared API.
- Independent verification round 2: PASS on the full suite. `./tests/validate.sh` exit 0, `Ran 2380 tests in 1657.980s`, `OK (skipped=164)`, parity 29 fixtures with zero deltas in both gate modes, gate and root Go phases executed rather than skipped. Durable evidence: /Users/robert/.cache/s12-B-validate.log and .exit.
- Environment note: the suite requires the pinned Python 3.14.7 oracle and bash >= 4.4, and the run used the real go1.24.13 binary because the mise `go` shim strips the mise python path from Go children.
- T6 remains: commit the work unit, run the native review under the user-owned switch, and publish.
