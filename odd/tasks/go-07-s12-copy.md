# Go 07 S12.1 — Root copy-apply authority

## Objective
Add the native root authority for copy application.
Call the shared copy-apply authority in process.
Verify it against the real Python oracle.
Keep the sync CLI unwired until S14.

## Tracker
- **card_id**: `6ac693bcdb52008d3a0bc0af`
- **url**: https://trello.com/c/VXiZ3bKa
- **list**: Review
- **ledger_item**: `92da5c339806685b`

## Scope
- Base: epic/go-single-binary at b36ba7e.
- Worktree: .worktrees/go-07-s12-copy.
- Branch: change/go-07-s12-copy.
- Add internal/sync copy application over the shared authority.
- Acquire copy items natively instead of shelling out to the gate bridge.
- Add a Python-oracle differential for the native authority.

## Constraints
- Parent and Pi subagents only. No Herdr or Orca.
- User authorizes commit, push, PR, and merge into the epic after checks.
- Do not modify development or publish releases.
- Budget: 1200 authored additions plus deletions, including tests and documents.
- Expected total: 500–700 lines. Stop before exceeding the budget.
- The doc pre-existence policy lives above this authority. The differential compares the items Python would actually send, not full doc flows.
- Do not modify production Python. Python remains the differential oracle.
- Do not modify gate sources; the shared authority is already in place.
- Do not wire the sync CLI. S14 owns orchestration and caller diagnostics.
- Do not extract classify or the template and hook actuators here.
- Reuse the shared authority rather than duplicating copy logic.
- Behavior tests require observed RED, GREEN, then proportionate refactoring.
- Run full validation before committing.

## Tasks
- [x] T1: Add the native authority contract tests and observe RED. Compile RED observed for the three missing symbols; go build stays green.
- [x] T2: Implement the root authority over the shared copy entry point to GREEN. Two mutation probes failed the matching tests before restore.
- [x] T3: Add the Python-oracle differential and confirm boundaries. Real Python entry points drove the comparison; a discrimination probe failed the test before revert.
- [x] T4: Run full validation, independent verification, and applicable native review. Functional verification returned ACCEPT; an overlay mutation probe failed at the mutated field.
- [ ] T5: Commit, publish, merge into the epic, close the tracker, and clean the worktree. **In progress**.

## Acceptance criteria
- The native authority produces the same results as the shared entry point for the same items.
- Per-item outcomes match the Python oracle: ok, source-missing, and failure ordering.
- Envelope ordering is preserved and an empty plan yields an empty non-null list.
- Invalid input surfaces the same class of error the authority documents.
- The authority does not write hashes, locks, or project files; Python keeps those concerns until S14.
- Source tests exercise the native authority, not only the Python path.
- The Python-oracle differential compares real outcomes, not recorded expectations.
- Root tests, vet, and full validation pass, and both parity modes show zero deltas.
- Authored diff stays within 1200 lines.

## Checks
- Focused RED/GREEN: go test ./internal/sync/ -run CopyApply -count=1 -v.
- Root suite: go test ./cmd/... ./internal/... and go vet ./internal/sync/.
- Gate suite unchanged: go test ./... inside the gate module.
- ./tests/validate.sh with durable exit records and bounded waits.
- Native review follows the user-owned switch and uses this slice base.

## API contract
- `func PlanCopyItems(catalogDir string, recipeIDs []string, roots CopyRoots) []shared.CopyItem` derives the copy items Python sends today.
- `func ApplyCopyItems(items []shared.CopyItem) ([]shared.CopyResult, error)` executes them in process through `shared.RunApplyCopy` and returns the shared result type.
- `CopyRoots` carries the project cache roots the caller already resolves: recipe skills root, commands directory, and project root for docs.
- Derivation per kind, matching Python: bundled skill source is the recipe skills directory, destination is the cache recipe skills root plus recipe id, skills, and skill id, and the id is the skill id. Command source is the recipe directory plus the declared path, destination is the commands directory plus the command id and .md, and the id is the command id. Doc source is the recipe directory plus the declared source, destination is the project root plus the declared target, and the id is the target.
- Only bundled skills are planned. Non-bundled skill sources belong to the dependency path and stay out of scope.
- Items keep the enabled recipe order and each recipe's declared skill, command, and doc order.
- Returns a non-nil empty slice when nothing is planned.
- The executor performs no writes outside the copy destinations the items name.
- It performs no hashing, lock writes, printing, warnings, or doc classification. Those stay with the Python call sites until S14.
- It reports per-item status in request order and maps a delivered error envelope to a Go error.
- It must not shell out to the gate bridge.

## Progress
- SX0c integrated the shared copy-apply authority at epic b36ba7e.
- Parent verified the new feature worktree and registered it.
- Parent linked this unit card before source writes.
- No source edits, checks, commits, or review results exist yet.
- T1 added the contract tests and pinned the API: CopyRoots with RecipeSkillsRoot, CommandsDir, and ProjectRoot; PlanCopyItems deriving bundled-skill, command, and doc items in Python's loop order; ApplyCopyItems executing in process through shared.RunApplyCopy.
- The tests also pin skipping an unloadable recipe, skipping non-bundled skill sources, a non-nil empty plan, and mapping a delivered error envelope to a Go error naming the failed item.
- Authored diff: 352 lines before this passive update.
- T2 implemented both functions in 105 lines. Focused tests, the root suite, vet, and formatting passed.
- Mutation probes: breaking the command destination join failed the plan tests; replacing the error mapping with a generic error failed the executor error test. Both were restored and GREEN re-confirmed.
- Authored diff: 457 lines. The executor forwards the shared error text verbatim because that envelope already names the failing item.
- The differential against the Python oracle, full validation, commits, and native review remain pending.
- T3 added 324 lines: a real Python-oracle differential for the plan and a plan-to-apply boundary test. The oracle monkeypatches only the copy call and the lock write, so the derivation under test is the real one.
- Non-vacuity is structural: both sides must yield exactly three items with the three expected kinds and no dependency-skill item.
- A discrimination probe perturbed the oracle destination and failed the comparison before revert.
- Known by-design divergence: Python emits no doc item when the destination already exists, while the native plan is pure derivation and always emits. The differential compares a fresh project; the preservation policy stays with the Python call site until S14.
- Authored total: 781 code lines plus this document. Full validation, commits, and native review remain pending.
- Full validation exited 0 in 39m06s: Python suite 2380 tests with 164 skips, gate and root Go packages ok, 29 fixtures per gate mode with zero deltas.
- Independent verification returned ACCEPT. It confirmed the oracle drives the real Python entry points, the executor spawns no process, and a plan mutation is caught: an overlay probe changing the command destination failed the differential at that exact field without editing any repository byte.
- Differential limits recorded honestly: the envelope commands_dir, apply-result statuses, cross-recipe order, and the doc-exists preservation policy are pinned natively rather than differentially.
- Authored total: 882 lines including this document. Commits and native review remain pending.

## Deferred scope and follow-ups
- Native review review-6bd1a6b97d3b9104 classified the candidate high risk because the change touches a process boundary, then admitted all four lenses: risk, resilience, readability, and reliability. Exact acknowledgement consumed its authority.
- Eight informational advisories, no correction requested. The load-bearing ones: declared source, doc destination, and item id are joined without containment checks at copyapply.go:56-66; the doc-exists boundary at :61-67; and the silent recipe skip at :36-39.
- The containment advisories match today's Python behavior, which performs the same joins. Record them as non-blocking follow-ups for the S14 wiring instead of hiding them.
- The doc-exists preservation policy stays with the Python call site until S14, so the differential compares a fresh project only.
- Rollback removes the two new files. No Python source, gate source, or trust manifest changes exist to revert.

## Python retirement boundary
- sync.go and syncagent.go still invoke Python materialize. S14 replaces those exec seams.
- The copy bridge and its Python fallbacks remain the oracle until their verified retirement.
- Template and hook actuators still need a shared classify extraction before their own units.
- S13, S14, and S16 still cover the remaining materialize paths.

## Next step
Commit the verified work unit and run native review against base b36ba7e. Freeze source bytes.
