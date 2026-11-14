# Go 07 S13 follow-ups — advisories, divergences, tracker gap and repo debt

## Purpose
Durable register of the non-blocking findings and environmental gaps left behind by the S13 slices
(PR1 bindings, PR2a/PR2b merge-config, PR3 resolved-config, PR4a hooks, PR4b-1 provider, PR4b-2
recipe-MCP). None of these items reopens or invalidates an approved review: every review that produced
them is approved, its receipt stands, and the findings are separate later work.

## 1. Advisory findings from the approved native reviews
| Id | Severity | Location | Source | What it asks for |
|---|---|---|---|---|
| `R3-witness-nonascii-bytes` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/witness.go:104-118` | PR1 bindings (`review-4244d619b602f106`) | Witness bytes diverge from Python for non-ASCII values (`café`); add a test before any fix. |
| `R3-unmemoized-gitcommon` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/witness.go:128-147` | PR1 bindings (`review-4244d619b602f106`) | The witness write bypasses the invocation git-fact memo; forward the fact as a value if wanted. |
| `R4-witness-unmemoized-gitfact` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/witness.go:128-147` | PR1 bindings (`review-4244d619b602f106`) | Same memo bypass; verdict-neutral and inert on `--resolve-bindings`. |
| `R3-conflict-parity-skipped` | SUGGESTION | `internal/sync/bindings_test.go:138-152` | PR1 bindings (`review-4244d619b602f106`) | Conflict-grader parity is asserted only where the Python grader does not raise; the test claims no silent pass. |
| `R3-empty-projectroot-witness` | WARNING | `internal/sync/bindings.go:57-64` | PR1 bindings (`review-4244d619b602f106`) | `BindingRequest` does not default `ProjectRoot`; an S14 wiring mistake would read from the cwd and skip the witness write. |
| `R4-project-root-unguarded` | SUGGESTION | `internal/sync/bindings.go:35-41` | PR1 bindings (`review-4244d619b602f106`) | S14 must set `ProjectRoot` explicitly; Python parity holds today. |
| `R4-acquisition-skip-unobservable` | SUGGESTION | `internal/sync/bindings.go:86-94` | PR1 bindings (`review-4244d619b602f106`) | A skipped unreadable recipe is not observable, so a silent skip cannot be told from a declared-nothing recipe. |
| `R4-gitfact-failure-collapse` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/gitcommon.go:38-45` | PR1 bindings (`review-4244d619b602f106`) | A failed git fact collapses to the empty string, so "no repository" and "git failed" are indistinguishable. |
| `R4-witness-crash-stale-revert` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/witness.go:185-189` | PR1 bindings (`review-4244d619b602f106`) | A crash between the temp write and the rename leaves a stale temp file behind. |
| `R2-001` | WARNING | `internal/sync/bindings_test.go:158-167` | PR1 bindings (`review-4244d619b602f106`) | Readability advisory; source records no further text beyond id, severity and location. |
| `R2-002` | WARNING | `internal/sync/bindings_test.go:175-178` | PR1 bindings (`review-4244d619b602f106`) | Readability advisory; source records no further text. |
| `R2-005` | SUGGESTION | `internal/sync/bindings_test.go:312-318` | PR1 bindings (`review-4244d619b602f106`) | Readability advisory; source records no further text. |
| `R2-003` | SUGGESTION | `odd/tasks/go-07-s13-bindings.md:152` | PR1 bindings (`review-4244d619b602f106`) | Readability advisory on a line of the bindings task document itself. |
| `R2-004` | SUGGESTION | `catalog/recipes/worktree-flow/gate/gitfacts.go:101` | PR1 bindings (`review-4244d619b602f106`) | Readability advisory; source records no further text. |
| `R2-006` | SUGGESTION | `catalog/recipes/worktree-flow/gate/shared/witness.go:55-69` | PR1 bindings (`review-4244d619b602f106`) | Readability advisory; source records no further text. |
| `R3-aliased-accessors` | SUGGESTION | `shared/orderedmap.go:34-41` | PR2a merge-config core (`review-c3c0685c7afcc4b7`) | `Keys()`/`Values()` expose aliased storage; harmless for read-only callers but a latent footgun for the PR2b consumer. |
| `R3-float-repr-untested` | WARNING | `internal/sync/mergeconfig.go:262-317` | PR2b merge-config root (`review-52f40ff65badb921`) | The Python-compatible float formatter has no focused threshold table test of its own. |
| `R3-acquisition-error-paths-untested` | SUGGESTION | `internal/sync/mergeconfig.go:76-165` | PR2b merge-config root (`review-52f40ff65badb921`) | Acquisition error paths (missing/unreadable `recipe.toml`, manifest read failure) are not pinned. |
| `R3-empty-recipe-table-dropped` | WARNING | (location not recorded in source) | PR3 resolved-config (`review-32aaed1d5717ae8e`) | The pre-existing empty-recipe-table divergence registered in section 2. |
| `R3-flat-config-collision-nondeterminism` | WARNING | (location not recorded in source) | PR3 resolved-config (`review-32aaed1d5717ae8e`) | The pre-existing flat-versus-`[config]` nondeterminism registered in section 2. |
| `R3-regex-wrapper-paren-injection` | WARNING | `internal/sync/hooksrunner.go:178` | PR4a hooks (`review-c61c0746cb253a5e`) | The fullmatch emulation wraps the pattern in `(?:...)`, so grouping-sensitive patterns can behave differently from `re.fullmatch`. |
| `R3-failure-paths-untested` | SUGGESTION | `internal/sync/hooksrunner.go:166-168` | PR4a hooks (`review-c61c0746cb253a5e`) | The regex compile-failure and marker-write failure paths are not pinned by a focused test. |
| `R3-nil-pystr-untested` | SUGGESTION | `internal/sync/hooksrunner.go:212-214` | PR4a hooks (`review-c61c0746cb253a5e`) | The top-level `nil`→`"None"` mapping is implemented but not exercised by a fixture (TOML has no null). |
| `R3-python-version-coupled-tripwires` | SUGGESTION | `internal/sync/hooksrunner_test.go:1` | PR4a hooks (`review-c61c0746cb253a5e`) | Several assertions pin Python 3.14.7 behavior, so a CPython change would trip them deliberately. |
| `R2-1` | WARNING | `internal/provider/provider.go:61-72` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | Readability advisory; source records no further text. |
| `R2-2` | WARNING | `internal/provider/provider.go:309-313` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | Readability advisory; source records no further text. |
| `R2-3` | SUGGESTION | `internal/doctor/depcheck.go:280-283` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | Readability advisory; source records no further text. |
| `R2-4` | SUGGESTION | `internal/provider/provider.go:175-177` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | Readability advisory; source records no further text. |
| `R3-doctor-mapping-untested` | SUGGESTION | `internal/doctor/depcheck.go:312` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | `resolveProviderDep` has no direct test of its `DepResult` mapping and detail strings. |
| `R3-runversion-grandchild-pipe-hang` | WARNING | `internal/provider/provider.go:220-245` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | Reliability advisory on the `_run_version` grandchild pipe. |
| `R4-runversion-waitdelay` | WARNING | `internal/provider/provider.go:216` | PR4b-1 provider (`review-2f6c9c0f71d806e2`) | Resilience advisory on the `_run_version` wait delay. |
| `R3-build-with-home-untested` | WARNING | `internal/sync/buildrecipemcp.go:180-190` | PR4b-2 recipe-MCP (`review-386468da95332b93`) | The build path with a non-nil `ai_specs_home` is not pinned by a focused test. |
| `R3-brittle-toml-message-pins` | SUGGESTION | `internal/sync/buildrecipemcp_test.go:440-480` | PR4b-2 recipe-MCP (`review-386468da95332b93`) | The TOML message pins are brittle against `internal/toml` message changes. |

Disposition for all 33: informational, non-blocking, later work; none opened a correction and none
reopens its review. All seven documents record their review as approved (authority burned with
`gentle-ai.review-acknowledged/v1`), and all seven list advisory ids, so no row is fabricated to fill a
missing list. The two PR3 advisories are the only ones whose source records no `file:line` location.
PR1 additionally lists two minor, pre-existing items as "not separately tracked" and therefore given no
ids: `TrackingDeclared` splits on `\n` where Python uses `splitlines()`, and Python `read_text()` can
raise `UnicodeDecodeError` outside its `except OSError`. `R3-aliased-accessors` is also carried forward
into PR2b as a consumer warning.

## 2. Registered parity divergences
Each was deliberately registered instead of "fixed" and none is a review failure. "Latent" means no wired production path observes it yet (the authorities are added but not called until S14).

- **Empty config key**: the shared gate core raises a process failure (`manifest entry missing key` / `field entry missing key`) where the Python fallback warns and continues (`unknown config key '' in manifest (ignored)`); Go fails, Python warns; pinned both sides by `mergeConfigDivergenceEmptyKey` in `internal/sync/mergeconfig_test.go`; pre-existing, latent.
- **Empty recipe table** (`[recipes.foo]` with no keys): the shared core drops the key where Python keeps `{"foo":{}}`; Go drops, Python keeps; pinned by `TestNativeResolvedConfigEmptyRecipeTableDivergence`; pre-existing in the moved code, latent.
- **Flat-versus-`[config]` collision**: resolves by Go map iteration order (observed twelve `2` and one `1` over thirteen runs) where Python is always the document-ordered value; Go nondeterministic, Python deterministic; pinned non-flakily by `TestNativeResolvedConfigFlatConfigCollisionNondeterminism`; latent.
- **Non-string binding coercion**: the shared str-keyed contract drops a non-string `capability`/`recipe` pair while Python coerces it; Go drops, Python coerces; the class is pinned in the resolved-config differential (`internal/sync/resolvedconfig_test.go`; source does not name the test) and the integer-`recipe` and boolean-`capability` siblings are the same class but unpinned; latent.
- **Spaced submodule paths**: the reader uses `strings.Fields` where Python uses `split(None, 1)`, so a path with a space could be listed by one side only; direction unresolved, untested, not exercised; latent.
- **Unpinned envelope internals and datetime boundary**: `reflect.DeepEqual` cannot see JSON key order in `recipes`/`bindings`, and the root rejects a datetime at TOML parse where Python fails at serialization; correct by construction but unpinned; latent.
- **`internal/config.PyStr` nested-value gap**: `map[string]any` renders as `map[a:1]` and nested `nil` as empty where Python `str()` prints `{'a': 1}` and `None`; Go differs; pinned by `TestRecipeHooksNestedValueReprDivergence`; pre-existing, owned by the repr port; latent.
- **RE2-versus-Python regex, fail-closed direction**: RE2 cannot express every Python `re.fullmatch` construct (backreferences, lookarounds), so the native path rejects patterns Python accepts; pinned by `TestRecipeHooksRegexEngineDivergence`; latent.
- **RE2-versus-Python regex, fail-open direction**: RE2-only syntax (`\p{L}`, a POSIX-class spelling Python treats as a literal set) and the ASCII-only `\W`/`\S`/`\D`/`\B` where Python is Unicode-aware let the runner validate a pattern Python would have failed sync on; Python-only constructs (`\uXXXX`, `\Z`, possessive quantifiers, atomic and comment groups, conditionals) and the `$`-before-trailing-newline asymmetry were unlisted before the repair; pinned by `TestRecipeHooksRegexPythonOnlyConstructs`; latent.
- **`recipe.id` charset gap**: neither authority validates the charset, so a `../`-bearing id escapes the `.recipe/` marker directory in both; identical on both sides (a shared pre-existing defect, not a Go-versus-Python divergence), and no pinning test is named in the source; latent.
- **Provider port divergences**: candidate order (Go sorted `os.ReadDir` vs Python `listdir`, observable only on version ties), version-tuple overflow (Go saturates at `MaxInt64`, Python arbitrary precision), and the Unicode `\d` digit class (Python accepts Nd, Go does not); all pinned with direction in the `internal/provider` differential; latent.
- **TOML error-text non-parity**: `internal/toml` messages are documented as not byte-parity with `tomllib`, surfacing through `cannot read recipe for provider resolution (<exc>)`; message-only, no outcome change; latent.
- **`internal/toml` value-subset gap**: a `[[provides.mcp]]` big integer above the Go `int64` range, or a TOML datetime, is refused by the Go reader while `tomllib` reads it; Go refuses, Python reads; this changes the outcome, not only the message (the resolver leaves the `{dep:jinna}` literal and warns instead of replacing `command`); pinned both directions by `TestBuildRecipeMCPDivergenceBigInteger` and `TestBuildRecipeMCPDivergenceDatetime`; pre-existing, latent while unwired; widening belongs to `internal/toml`.
- **Non-finite float message non-parity**: the Python `ValueError` text (`Out of range float values are not JSON compliant: nan`, a transport failure that never crosses the wire) and the adapter's acquisition-time message (`manifest "ratio" value: Out of range float values are not JSON compliant`) are deliberately asserted not byte-equal; pinned in `internal/sync/mergeconfig_test.go`; latent.

## 3. Tracker gap (availability, not a missing link)
- Every S13 unit was implemented while the Trello API key was missing or expired, so no card was created or linked and no `tracker.none` was written (the host never writes it; it is presentation and evidence only).
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed without recording a decision.
- Close-out once the tracker is reachable: create or link the S13 cards and record `card_id` and `url` in the `## Tracker` section of all seven documents — `odd/tasks/go-07-s13-bindings.md`, `odd/tasks/go-07-s13-mergeconfig.md`, `odd/tasks/go-07-s13-mergeconfig-root.md`, `odd/tasks/go-07-s13-resolvedconfig.md`, `odd/tasks/go-07-s13-hooks.md`, `odd/tasks/go-07-s13-provider.md`, `odd/tasks/go-07-s13-recipe-mcp.md` — then close them with a progress comment.

## 4. Native review transport debt
- **Whole-epic target replay**: a first `inspect` without `baseRef` bound the whole-epic target (`247ba91…`, base `e773514`), and three later `start` calls with `baseRef=24c4ca1` replayed it and returned `consent-binding-expired` (`native_invocation_attempted: false`); fix by passing `baseRef` plus `committedOnly` to `inspect` from the first call.
- **`review-resilience` empty output (host relay)**: the lens failed six times with `reviewer-empty-output` and `stopReason: length`, first on `nan/glm5.3-flash` and then on `nan/qwen3.8-flash` (131 000-token output ceiling), because both models exhaust their output budget reasoning and emit no text; the host relay reads `~/.pi/gentle-ai/models.json` (`gentlePiConfigHome()`), never the subagent `model_profiles` in `subagents.json`, and the lens completed once routed to `anthropic/claude-sonnet-5-5`. This is a **different root cause** from the one in `odd/tasks/go-07-s12-followups.md` section 3 (the child-spawn `nan` provider registration gap, gentle-shell#1587): the S12 diagnosis does not explain these host-relay failures, which are a model output-budget and relay-configuration issue, not a missing-provider issue.
- **Expired consent binding**: an unanswered host consent binding expires after 10 minutes with `lineage_created: false` and nothing mutated; re-running `start` mints a fresh envelope.
- **Correction to the S12 register**: the S12 register states that a frozen candidate view remains at `.git/gentle-ai/candidate-views/339f233d-cb1a-4d2d-a568-c3fccd71839b` for the still-open lineage `review-e84515b5410c08c0`; that directory is **empty on disk today**, and only `.git/gentle-ai/review-transactions/v2/review-e84515b5410c08c0` remains; the decision about that lineage is still open.

## 5. Repository and environment debt
- The mise `go` shim rebuilds the child PATH and strips `mise/installs/*`, so the real Go 1.24.13 and Python 3.14.7 binaries (`~/.local/share/mise/installs/{go/1.24.13,python/3.14.7}/bin`) must be prefixed for the suite to run; never use the shims.
- `lib/skills-list.sh:224` expands an empty array under `set -u`, which fails on bash 3.2 (the macOS default); the suite needs bash >= 4.4 and nothing in the repository declares that requirement.
- Child agents need `~/.gentle-shell/agent/models.json` for the `nan` provider; removing that file breaks every subagent profile pinned to `nan/*`.
- An unexplained one-shot gate-suite failure was observed once during PR4a verification and did not reproduce in the full validation run or in two clean re-runs of the untouched gate module.
- New fact from this series: a full `./tests/validate.sh` on a clean worktree reports 2482 tests only after `bash scripts/build-gate.sh` has produced `dist/worktree-gate-current` in that same worktree; without it the bridge classes skip and the count drops (PR2b observed `Ran 2380 tests ... OK (skipped=164)`).

## 6. Where the S13 evidence lives
- PR1 bindings: `odd/tasks/go-07-s13-bindings.md`, logs `~/.cache/s13-pr1-validate.{log,exit}`, PR #341 merged as `7384c65`.
- PR2a merge-config core: `odd/tasks/go-07-s13-mergeconfig.md`, logs `~/.cache/s13-pr2a-validate.{log,exit}`, PR #342 merged as `fe6e7cf`.
- PR2b merge-config root authority: `odd/tasks/go-07-s13-mergeconfig-root.md`, logs `~/.cache/s13-pr2b-validate{,2}.{log,exit}`, PR #343 merged as `64f9217`.
- PR3 resolved-config: `odd/tasks/go-07-s13-resolvedconfig.md`, logs `~/.cache/s13-pr3-validate.{log,exit}`, PR #344 merged as `de50ef5`.
- PR4a hooks runner: `odd/tasks/go-07-s13-hooks.md`, logs `~/.cache/s13-pr4a-validate.{log,exit}`, PR #345 merged as `24c4ca1`.
- PR4b-1 provider resolver: `odd/tasks/go-07-s13-provider.md`, logs `~/.cache/s13-pr4b1-validate.{log,exit}`, PR #346 merged as `da7e370`.
- PR4b-2 recipe-MCP authority: `odd/tasks/go-07-s13-recipe-mcp.md`, logs `~/.cache/s13-pr4b2-validate.{log,exit}`, PR #347 merged as `e86578f`.
- Closing note: S13 is complete with those seven merged units on `epic/go-single-binary`.

## Contradictions recorded between sources
- `odd/tasks/go-07-s13-provider.md` task T4 still reads as if the PR and squash-merge are "the remaining closing step", but the same document records the work-unit commit and the approved review, and `odd/tasks/go-07-s13-recipe-mcp.md` takes base `da7e370` (the merged PR4b-1); both are recorded here.
- `odd/tasks/go-07-s13-bindings.md` states a 1200-line budget in its acceptance criteria while the same document records 1548 additions plus deletions as a human-accepted overage; both are recorded.
