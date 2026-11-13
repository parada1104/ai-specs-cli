# Go 07 S18 — Run the doctor's brief probes in process (retire the brief half of the bridge)

Base `44cd3d8` on `epic/go-single-binary` (S17 merged). Worktree
`.worktrees/go-07-s18-doctor-brief`, branch `change/go-07-s18-doctor-brief`.
S16 deferred blocker #2, first half: `lib/_internal/recipe-materialize.py`'s deletion needs the
doctor brief ops on the native chain.

## Specs
- S1 — Session request (verbatim, from the S16 register): "`lib/_internal/recipe-materialize.py`:
  also needs the doctor brief ops ported onto the native chain (`internal/doctor/bridge.py:60-101`
  calls `build_resolved_config`, the S14b enrichers, `resolve_bindings`,
  `agents-render._render_lines`/`brief_effective_state` and
  `brief-render-policy.has_dead_recipe_fragments`)"; continued this session with "continuemos con la
  migración completa a go por favor".
- S2 — The doctor's two brief probes run in process. `checkBriefProvenance` and
  `checkBriefRenderPolicy` (`internal/doctor/checks_brief.go`) call new read-only exports in
  `internal/sync`; `bridgeOps()` stops requesting `brief_state`/`brief_dead_fragments` and
  `bridge.py` stops implementing them — it no longer loads `recipe-materialize.py`,
  `agents-render.py` or `brief-render-policy.py`. `toml_error` stays Python on purpose: tomllib's
  diagnostic text is interpreter-owned and version-dependent, and that op is not in the blocker
  path.
- S3 — No output change. Every brief state keeps its severity, message, guidance and ordering, and
  the dead-fragments warning is emitted under exactly the same conditions. Both probes degrade
  exactly like the Python op did: any failure of the provenance chain reports the state as
  `undetermined`, and any failure of the dead-fragments chain emits no check at all.
- S4 — The probes reproduce the Python semantics exactly, not approximately. The provenance chain is
  the bridge's `_resolved_for_brief`: the projection, then catalog defaults, then brief fragments,
  then the auto-bindings under the enabled guard (with a resolution error reported as a failure,
  because Python's `resolve_bindings` raises there, which the op turns into `undetermined`), then
  `_render_lines` (which validates `[brief]` mode keys first) and `brief_effective_state` with the
  lock path Python's `_brief_lock_path` uses. The dead-fragments chain is the projection plus
  `attach_brief_fragments_to_resolved` only — no catalog defaults, no auto-bindings — and its
  predicate treats an absent or non-object `brief_fragments` as no fragments and any truthy section
  value as dead.
- S5 — Differential evidence: a fixture matrix compares both probes against the real Python modules
  (the same chain the bridge ran) for the states `missing`, `managed_current`, `managed_stale`,
  `untracked`, `user_modified`, `marker` and `undetermined`, and for the dead-fragment predicate
  across a recipe with fragments, a recipe with none, a recipe missing from the catalog and a
  `brief_fragments` value that is not an object.
- S6 — No module is deleted. `recipe-materialize.py`, `agents-render.py` and
  `brief-render-policy.py` stay for the shell routes (`lib/init.sh:242-245`, `lib/sync.sh`,
  `lib/doctor.sh`) until the dispatcher cutover; this unit only removes the doctor's dependency on
  them.

## Tasks
- T1 — S4: split `enrichResolvedConfig` into the two Python-shaped halves so the dead-fragments
  chain can run fragments without catalog defaults. Route: inline. Commit: pending.
- T2 — S2/S4: add the read-only brief view to `internal/sync` (provenance state, dead-fragments
  predicate) with the fixture matrix of S5 as RED-first tests. Route: inline. Commit: pending.
- T3 — S2/S3: wire the doctor's two checks to the native probes and trim `bridge.py` to
  `toml_error`; keep the check outcomes byte-identical. Route: inline. Commit: pending.
- T4 — S5: differential tests against the real Python modules for both probes. Route: inline.
  Commit: pending.
- T5 — Evidence: focused packages, `go test ./cmd/... ./internal/...`, parity both modes,
  `./tests/validate.sh`. Route: inline. Commit: pending.

## Log
- L1 — "continuemos con la migración completa a go por favor" (2026-10-15). Session continues the
  S16 deferred blockers; S17 (target resolution) merged as PR #358 → `44cd3d8` first.
- L2 — Reconnaissance (read-only subagent) mapped the seam: the bridge is one batched call
  (`internal/doctor/bridge.go:111`) with three ops (`bridgeOps()` :73-98); the brief consumers are
  `checks_brief.go:47` (`brief_dead_fragments`) and `:69` (`brief_state`); the native authorities
  already exist (`internal/sync/resolvedconfig.go:40`, `resolvedcatalog.go:25`,
  `bindings.go:54`, `agentsrender.go:616/:645/:1198`) but the composition is missing, so this unit
  writes wiring, not ports.
- L3 — Decision (evidence-backed): the dead-fragments op uses a *different* chain than the
  provenance op in Python (`op_brief_dead_fragments` skips the catalog defaults and the
  auto-bindings), so the native side mirrors both chains instead of collapsing them into one.

## Progress
- T1 done: `enrichResolvedConfig` is now the two Python-shaped halves —
  `mergeCatalogDefaultsIntoResolved` then `attachBriefFragmentsToResolved`, with the shared
  `resolvedRecipesDoc`/`resolvedRecipeEntry`/`catalogRecipe` helpers — so a caller that needs
  fragments without catalog defaults can run the second half alone, as Python's bridge does.
- T2 done, RED then GREEN: `internal/sync/briefview.go` adds the read-only view
  (`BuildBriefView` → `BriefView{WouldWrite, State}`, `BriefHasDeadRecipeFragments`) over the
  existing authorities. RED was the new fixture matrix failing to build (undefined
  `BuildBriefView`/`BriefViewRequest`/`BriefHasDeadRecipeFragments`); GREEN is the differential
  suite plus the predicate unit test. `materializeEnrichedResolvedConfigDocument` now shares
  `orderedResolvedDocument`, so the JSON round trip exists once.
- T3 done, RED then GREEN: `checks_brief.go` calls the native probes and `bridgeOps()` requests
  only `toml_error`, so a healthy project never spawns Python; `bridge.py` is down to that one op
  and no longer imports recipe-materialize.py, agents-render.py or brief-render-policy.py. RED
  (observed before the wiring) was `bridgeOps() = [brief_state]` on a parseable manifest,
  provenance degrading to `undetermined` and the dead-fragments check emitting nothing with no
  bridge object; GREEN is the hermetic `internal/doctor` suite. The now-unused `manifestHasContent`
  helper is retired rather than left dead.
- T4 done: the differential oracle (`internal/sync/testdata/briefref_ref.py`) replays the bridge's
  own two chains through the real legacy modules, and the fixture matrix compares them against the
  native view: six ownership states plus the invalid-`[brief]`-mode failure, and five
  dead-fragments scenarios including a catalog recipe the loader rejects (both legs skip it).
  Fixture discovery worth recording: an invalid `recipe.toml` is skipped by *both* chains, so a
  fragment expectation against a malformed fixture would have passed vacuously — `[recipe]`
  requires `description` and `version`, and the matrix now pins the skipped case explicitly.
- T5 done: see Evidence.

## Evidence
Run from this worktree at base `44cd3d8`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go vet ./internal/sync ./internal/doctor` | clean | 0 |
| `go test ./internal/sync -run 'TestBuildBriefView\|TestBriefHasDeadRecipeFragments' -count=1` | RED before the implementation (`undefined: BuildBriefView`), `ok` after | 0 (GREEN) |
| `go test ./internal/doctor -run 'TestBridgeOpsRequestOnlyTomlError\|TestBriefProbesRunWithoutTheBridge' -count=1` | RED before the wiring (`bridgeOps() = [brief_state]`, `undetermined`, no dead-fragments check), `ok` after | 0 (GREEN) |
| `go test ./internal/doctor -count=1` | whole package ok | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok, including `internal/sync` and `internal/doctor` | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; `gate-absent failing=0, gate-present failing=0` | 0 |
| `./tests/validate.sh` | `Ran 2482 tests in 1665.530s` → `OK`, parity both modes PASS | 0 |

Log: `~/.cache/s18-evidence.log` (chained go test → build-gate → parity → validate).

## Native review (high tier, four lenses)
Bound STATUS → group capture of all four reviewers → last-event closure:

- Lineage `review-061d312ddc16eece`, target `sha256:9d27f58eeb856b49fb835bedfcaa15386da95e4195174f6300d99f434ddfa20d`,
  candidate `44cd3d8010437758527611a255beb4de9bea9438..7a8c069c6ef42c4234788358fe47ffaaa98b2fe3`
  (committed-only), 11 paths / 1210 changed lines, correction budget 200. Risk `high` with reason
  `process_boundary` (`internal/doctor/bridge.py`). Consent was resolved by the host.
- Group capture over `pi_host_relay`: 4/4 reviewers submitted, closure `approved` on the last
  admitted event (no correction offered), authority burned (`gentle-ai.review-acknowledged/v1`).
- Input-shape facts learned the hard way, both rejected before any authority existed
  (`lineage_created: false`): the native `--lenses` enum takes the BARE names
  (`risk,resilience,readability,reliability`), the tier fixes the COUNT (low 0, medium exactly 1,
  high all four), the reason is length-bounded, and `baseRef` must be the full 40-character commit
  id — the abbreviated form is refused. Each wrong shape cost one round trip.
- 9 advisory findings, all informational and non-blocking (readability R2-1..R2-5, reliability
  R3-autobindings-unproved / R3-render-policy-guards-unproved / R3-tomllib-toplevel-import,
  resilience R4-tomllib-import-hoisted). See Follow-ups.

## Independent verification
The plan for a high-risk candidate requires a separate verifier. The delegated `gentle-ai-verify`
subagent **timed out** (stalled 30 minutes inside a bash call), so the role is reported
**unavailable** and was not retried. The obligation was discharged inline instead, with bounded
commands and by a method different from the unit's own tests:

| Check | Command (temp-dir fixture) | Result |
| --- | --- | --- |
| Trimmed op set | `grep -n "OPS = {" -A 4 internal/doctor/bridge.py` | exactly `toml_error`; the only remaining mentions of the three retired modules are the docstring lines recording the retirement |
| No-Python path | Go `doctor` with `PATH` pointing at a bin dir holding no `python3` | `INFO brief-provenance  runtime brief ownership: missing` present; no `doctor-bridge` ERROR (rc 1 came from the checks that legitimately need the gate binary in that stripped environment) |
| Read-only contract | sha256 of every file under the project root and the CLI home, before and after that run | identical both times |
| Advisory closure | new `explicit bindings reach the rendered brief` scenario, both legs | GREEN, with the non-vacuous assertion that the bound `GitHub` label reaches the bytes |

## Size
Disclosed: about 330 authored lines of product code (`briefview.go` 300 with its comments, the
resolvedcatalog split, the doctor wiring, `bridge.py` minus two ops), about 380 of tests (the
differential matrix with its oracle, the predicate unit test, the hermetic doctor probes), about 20
of bridge/comment trimming, and this document. Inside the plan's ~700-line cap.

## Follow-ups (recorded, not done here)
- Native review advisories, all non-blocking and none reopening the candidate: `R2-1`
  (`internal/doctor/checks_brief.go:34-36`), `R2-2` (`internal/sync/materialize_glue.go:258`),
  `R2-3` (`internal/doctor/bridge.py:4-9`), `R2-4` (`internal/sync/briefview.go:99`), `R2-5`
  (`internal/doctor/bridge.go:26-27`) — readability, informational; `R3-render-policy-guards-unproved`
  (`internal/doctor/checks_brief.go:37-38`); `R3-tomllib-toplevel-import` / `R4-tomllib-import-hoisted`
  (`internal/doctor/bridge.py:17`).
- `R3-autobindings-unproved` was **closed in this unit** rather than deferred: the differential matrix
  now includes the explicit-`[[bindings]]` scenario above.
- **Discovered while closing that advisory, not fixed here**: Python's renderer prints the
  unknown-VCS warning to **stdout** (`lib/_internal/agents-render.py:350`), which is the same stream
  the legacy `bridge.py` used for its JSON response — so a project binding `vcs-pr-flow` to a recipe
  id outside the label table could make the old bridge's response unreadable and the doctor report
  `undetermined`, while the native path reports the real state. It is a bridge defect that the port
  does not reproduce; recording it as a deliberate TOLERANT divergence needs its own pin and a
  parity-contract note, so it is registered here instead of being silently absorbed.
- `lib/init.sh:242-245` still execs the same three modules for the baseline AGENTS.md render; the
  native brief view is the chain it needs, so the init cutover is its own unit (S19 in this series).
- `lib/sync.sh` / `lib/skills-add.sh:208` still call the shell sync; the dispatcher cutover that
  deletes `recipe-materialize.py`, `agents-render.py` and `brief-render-policy.py` stays separate
  and needs the human's decision.
