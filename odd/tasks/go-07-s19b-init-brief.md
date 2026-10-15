# Go 07 S19b — The baseline brief in init.sh, natively

Base `54cabc9` on `epic/go-single-binary` (S19a merged). Worktree
`.worktrees/go-07-s19b-init-brief`, branch `change/go-07-s19b-init-brief`.
S16 deferred blocker #2, second half: `lib/init.sh:235-262` is the last Python caller standing between
`recipe-materialize.py`, `agents-render.py` and `brief-render-policy.py` and their deletion.

## Specs
- S1 — Session request (verbatim): "continuemos con la migración completa a go por favor", plus the
  S19a decision that the shell reaches native code through the fail-open `AI_SPECS_GO_BIN` seam.
- S2 — `lib/init.sh`'s baseline render reaches native code through that seam and keeps its Python
  route untouched: with a usable seam the block runs the native gate and the native chain; without
  one (a direct `bash lib/init.sh`, or the bash launcher) every command is byte-for-byte the legacy
  one.
- S3 — Two internal verbs, each mirroring one call site rather than the whole block:
  - `brief-render-policy <toml-path>` is `lib/_internal/brief-render-policy.py` in its default mode:
    `true` or `false` on stdout, exit 0, and `error: <…>` on stderr with exit 1 when the manifest
    cannot be read or parsed; a non-boolean `[brief].render` stays the fail-safe `true`.
  - `baseline-brief <project-root> --resolved-config-out <path> [--adopt-brief]` is the chain the
    block runs: the materializer with only the resolved-config output requested, and then the
    AGENTS.md render with `--preserve-if-runtime-brief` and the optional `--adopt-brief`. The
    materializer's failure short-circuits the render, exactly like the shell's `&&`, and the exit
    status is the first failure's.
  Both are internal seams for the scripts, not product surface: they do not appear in `help`, and the
  block keeps its own `mktemp`/trap, its own success echo, its own placeholder fallback and its own
  stderr notices.
- S4 — No output change on either route: the block still prints `  ✓ render AGENTS.md (baseline
  brief)`, the placeholder branch still prints `  ! render skipped — fallback placeholder written`
  and the disabled branch still prints its two notices in order; the gate's empty-output-on-failure
  behaviour stays, so `set -e` cannot abort init where it did not before.
- S5 — The parity harness keeps its meaning: the legacy leg (`bin/ai-specs`, bash, no seam) runs the
  Python chain while the Go leg (the built binary, seam set) runs the native one, so the `fresh-init`
  fixture diffs them for real instead of comparing the same script with itself.
- S6 — No module is deleted and no other call site is touched. `sync.sh`, `doctor.sh` and the doctor
  keep their Python routes until the cutover; the gate's diagnostic text is Go's (recorded TOLERANT
  class, like the manifest fatal-error framing), while its exit code and stdout are frozen.

## Tasks
- T1 — S3: the sync-domain composition (`RenderBaselineBrief`) over the materializer and the renderer
  the pipeline already uses. Route: inline. Commit: pending.
- T2 — S3: the two internal verbs and their routes/flags in `internal/cli`. Route: inline.
  Commit: pending.
- T3 — S2/S4: the block's seam branches, with the gate's empty-output-on-failure semantics preserved.
  Route: inline. Commit: pending.
- T4 — S2/S5: route-preference and failure-branch tests, plus the retargeted init failure test whose
  fault injection moves from a `python3` stub to the seam. Route: inline. Commit: pending.
- T5 — Evidence: focused packages, `go test ./cmd/... ./internal/...`, parity both modes,
  `./tests/validate.sh`. Route: inline. Commit: pending.

## Log
- L1 — "continuemos con la migración completa a go por favor" (2026-10-15). S17 (target resolution),
  S18 (doctor brief probes) and S19a (the seam + the add-dep auto-sync) are merged; the epic tip is
  `54cabc9`.
- L2 — Reconnaissance (read-only subagent, recorded in the S19a doc) established what S19b needs:
  `EvaluateBriefRender` (`internal/sync/briefpolicy.go:41`) for the gate, `MaterializeRecipes` with
  `ResolvedConfigOut` (`internal/sync/materialize.go:49-58`, the call the spine makes at
  `sync.go:342-347`) and `RenderAgentsFile` (`internal/sync/agentsrender.go:1332`) with
  `PreserveIfRuntimeBrief`/`AdoptBrief`/`ResolvedConfigPath`.
- L3 — Surface decision, with the alternative recorded: one composite verb covering gate, chain and
  the placeholder fallback was rejected because it would move the shell's presentation and fallback
  logic into Go (the block's messages depend on which branch ran and on whether AGENTS.md exists).
  Two verbs, each a 1:1 replacement of one call site, keep every line of that flow where it is.
- L4 — Why this closes the blocker rather than moving it: with the seam the block needs no Python at
  all, so the three modules have no init caller left; what still keeps them alive is the cutover
  (`sync.sh`, `doctor.sh`, the parity legacy leg), which is S20 and the human's decision.

## Progress
- T1 done: `internal/sync/baselinebrief.go` composes `MaterializeRecipes` (with only
  `ResolvedConfigOut` requested, exactly the call the spine makes) and `RenderAgentsFile`
  (`PreserveIfRuntimeBrief`, optional `AdoptBrief`, `ResolvedConfigPath`). It owns no policy: the
  `[brief].render` gate stays the caller's, so the disabled and failure branches keep printing what
  they always printed. The materializer's failure returns 1 before the render, mirroring `&&`.
- T2 done, RED then GREEN: RED was the new `internal/cli` tests failing to build
  (`undefined: runBriefRenderPolicy`, `runBaselineBrief`, `routeBriefRenderPolicy`,
  `routeBaselineBrief`); GREEN is `internal/cli/brief.go` plus the two routes. The verbs are internal
  seams: routed natively, absent from `help.txt`, and pinned as such.
- T3 done: `lib/init.sh` keeps its block and replaces only the two call sites, through two small
  helper functions (`brief_render_policy`, `baseline_brief_render`). The gate keeps its shape — the
  command substitution stays inside `[[ ]]`, so a failing gate cannot abort init under `set -e` —
  and the `mktemp`/trap, the success echo, the placeholder fallback and the two skip notices are
  untouched. `bash -n lib/init.sh` is clean.
- T4 done, and non-vacuous: the three route tests are GREEN, and reverting `init.sh` (a `git stash`
  of that file alone) makes the route test fail because the seam's log never appears — the seam was
  ignored and the Python route ran. The pre-existing failure test needed no retarget: it drives
  `bin/ai-specs`, where the seam is unset, so it still injects its fault through the `python3` stub
  on the legacy route.
- T5 done: see Evidence.

## Evidence
Run from this worktree at base `54cabc9`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go vet ./internal/cli ./internal/sync` | clean | 0 |
| `go test ./internal/cli -run 'TestBriefRenderPolicyVerb\|TestBaselineBriefVerb\|TestInternalBriefVerbs'` | RED before the implementation (`undefined: runBriefRenderPolicy`), `ok` after | 0 (GREEN) |
| `python3 -m unittest tests.test_init_brief_route -v` | `Ran 3 tests` → `OK` (route preference, native failure fallback, cross-route bytes) | 0 |
| route test with `init.sh` stashed | RED: no seam log, i.e. the seam was ignored | 1 (expected) |
| `bash -n lib/init.sh` | syntax ok | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; both modes PASS | 0 |
| `./tests/validate.sh` | `Ran 2487 tests in 1639.314s` → `OK` | 0 |

Log: `~/.cache/s19b-evidence.log` (chained go test → build-gate → parity → validate). The
Python suite moves 2484 → **2487**: the three route tests are the only addition, so the delta is
accounted for rather than assumed. The parity `fresh-init` fixture now diffs the legacy Python chain
(the legacy leg never sets the seam) against the native one (the Go leg's shim sets it), which is the
differential S5 asks for.

## Native review (high tier, four lenses)
- Lineage `review-1bf072a18b4e15cf`, target `sha256:cd1da4de43537a0c76a26d52a0ac17f3f5595dcf99aebee64db381eddaafc045`,
  candidate `54cabc98d1ac9f10fb5f818e27b54df41f411571..7d01f56` (committed-only), 7 paths / 629
  changed lines, correction budget 200. Risk `high` (`process_boundary` **and** `shell_source` on
  `lib/init.sh`). Consent was resolved by the host.
- The group capture was **refused at admission**: the readability reviewer's first finding carried no
  proof reference (`at least one proof reference is required`). The provider did **not** consume that
  lens slot — 2/4 had been submitted (risk, resilience) and the rejected payload is preserved under
  `.git/gentle-ai/rejected-results/review-1bf072a18b4e15cf/`. Continuation, exactly as the refusal
  instructed: a fresh bound STATUS reoffered only the two missing slots, each was captured one at a
  time (forecast → acknowledgement → run), and the last admitted event closed the review
  **approved**. Authority burned (`gentle-ai.review-acknowledged/v1`); the refused bytes were never
  resubmitted.
- 7 advisory findings, all informational and non-blocking. See Follow-ups.

## Independent verification
The high-risk plan requires a separate verifier; the delegated `gentle-ai-verify` subagent is
reported **unavailable** (the same 30-minute bash stall as S18, not retried). The obligation was
discharged inline by a different method than the unit's own tests: a differential against the very
Python scripts the two verbs replace.

| Check | Result |
| --- | --- |
| `brief-render-policy` vs `brief-render-policy.py` over render unset / true / false / invalid | **4/4 MATCH**: same exit code and same stdout on every case, including the fail-safe `true` for the invalid value |
| the same gate on a missing manifest | rc 1 both, stdout empty both, stderr prefix `error: ` both — the diagnostic text itself is the recorded TOLERANT class |
| `baseline-brief` vs `materialize --resolved-config-out && agents-render` on a fixture project | rc 0 both, `AGENTS.md` **byte-identical** (272 bytes) |
| the intermediate resolved-config document both legs wrote | identical except `project_root`, which is the directory each leg was given (both 330 bytes, same keys, values and order) — the one diff is the fixture itself, not a divergence |

## Size
Disclosed: about 160 authored lines of product code (the two verbs and their routes, the shell helper
branches, and the `RenderBaselineBrief` composition), about 300 of tests (the verb matrix and the
three route tests with their stubs), and this document. Inside the plan's ~700-line cap.

## Follow-ups (recorded, not done here)
- Native review advisories, all informational: `R2-duplicated-seam-guard` (`lib/init.sh:247`, the seam
  guard repeated in the two helpers), `R2-duplicated-usage-literals` (`internal/cli/brief.go:63-79`),
  `R2-status-comment-disagrees-with-code` (`internal/sync/baselinebrief.go:31-33`),
  `R2-unexplained-decision-token` (`internal/sync/baselinebrief.go:59-60`),
  `R2-unknown-tokens-become-project-root` (`internal/cli/brief.go:70-75`, an unknown flag is taken as
  the positional root), `R3-adopt-and-usage-unproved` (`internal/sync/baselinebrief.go:56-66`,
  `--adopt-brief` is not covered by a test) and `R3-materialize-error-dropped`
  (`internal/sync/baselinebrief.go:52-54`). The two worth a pass together are the arg parser (unknown
  token → root) and the missing `--adopt-brief` coverage.
- S20 (the human's): flip `bin/ai-specs` to the native dispatcher and delete the three modules. After
  this unit no init caller needs them; what still does is the shell route of `sync.sh` / `doctor.sh`
  and the parity legacy leg.
