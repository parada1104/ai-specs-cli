# GO-20 U2e T5 PR4a S4 follow-up — platform.sh fixture freeze

Base: epic/go-single-binary f798526. Branch: change/go-20-u2e-t5-pr4a-s4-platformsh.
Goal: `internal/sync/syncagent_test.go`'s platform matrix test stops reading `lib/_internal/platform.sh` live
and skips when it is absent, and pins the same oracle from a frozen bundle instead. Same discipline as
PR #494–#510 (S4 slices), and the identical pattern as #510 (`hooksrunner_test.go` +
`testdata/hooksrunner_re_golden.json`, reusing `readGoldenBundle` / `validateGoldenIdentity` from
`agentsrender_test.go`).

## Specs

- S1: `TestAgentPlatformMatchesPlatformSh` keeps its name and every native assertion (all 8 agents × the 7
  asserted fields — `instructions_path`, `skills_dir`, `mcp_config_path`, `mcp_key`, `native`, `commands_dir`,
  `runtime_hooks_target` — compared against `platformGet`). The oracle becomes the frozen bundle
  `internal/sync/testdata/platform_sh_golden.json`; there is no skip, so a missing golden fails.
- S2: **Decision: freeze the whole per-agent case blocks, not a field projection.** The bundle holds exactly
  what `platformShBlocks` returned — one record per `        <agent>)` block, verbatim block text, in source
  order (a block runs from the end of its agent line to the start of the next agent line). Reasons: (a) it is
  1:1 with the helper's output, so nothing is editorially selected; (b) the test's own field-extraction regex
  keeps running over real oracle bytes instead of a hand-built map, so the assertion surface is unchanged, not
  narrowed; (c) the unasserted block content (`agents_dir`, the shell comments) is oracle bytes the splitter
  already produced and stays available to a future assertion with no re-capture; (d) the frozen payload is
  reproducible from the recorded blob. A projection of the 7 asserted fields was rejected as strictly less
  faithful for the same provenance cost.
- S3: `oracle` metadata names `source: lib/_internal/platform.sh`, `blob:
  c03817f17be904ec1839b0081f2cf5e41d9b0107` (= `git rev-parse f798526:lib/_internal/platform.sh`), `base`,
  the `driver` (the splitter `(?m)^        (\w+)\)$`) and a `note`. The loader refuses a bundle missing any of
  those keys, refuses a `source` other than `lib/_internal/platform.sh`, and refuses a payload containing
  `/Users/`, `/private/` or `/var/folders`.
- S4: The hard-coded, independent inventory is `platformShGoldenNames` — 8 agents, source order
  (`claude`, `cursor`, `opencode`, `codex`, `copilot`, `gemini`, `pi`, `omp`) — checked by
  `validateGoldenIdentity` (exact count, order, uniqueness) before any block is used.
- S5: Every other assertion in `syncagent_test.go` is preserved; `TestAgentPlatformCoversEightAgents` is
  untouched, and the file's provenance header now names the frozen bundle for the platform matrix.
- S6: Negative controls: mutating one block, deleting one agent and duplicating one agent each fail rc=1, with
  the golden restored byte-identically (`cmp`) afterwards.
- S7: No `lib/` access: the focused test passes rc=0 (PASS, not SKIP) with `lib/_internal/platform.sh`
  temporarily renamed, and `grep 'lib/_internal\|lib/[a-z_-]*\.sh' syncagent_test.go` shows only provenance
  text, the frozen-metadata assertion and the (pre-existing, out-of-slice) `lib/sync-agent.sh` live read.
- S8: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, `go test ./internal/sync/ -count=1`
  rc=0, `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.
- S9: The bundle is read-only — it is loaded through `readGoldenBundle` and never rewritten by a test, so
  re-capturing needs a human decision (recorded in the `note`).

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `platformShBlocks` reading `lib/_internal/platform.sh` (8 agent case blocks, `t.Skipf` when absent) | FREEZE | the payload is the block text of a file that is deleted in PR4b, with no host-dependent value, so it freezes verbatim. RETIRE was rejected: the 56 field assertions (8 agents × 7 fields) are the only oracle for `agentPlatformMatrix`, and no other bundle carries them (`agentsrender/briefgov/agentsstep` freeze brief/governance strings; `hooksrunner*` freeze hooks). RETARGET has no candidate. The live leg consumed exactly these 8 blocks, so the freeze is 1:1. |

## Frozen bundle

`internal/sync/testdata/platform_sh_golden.json` — 6789 B, `oracle` (5 keys) + 8 `cases`
(`{"agent", "block"}`) in source order. Blocks are byte-verbatim slices of the base blob, so the only
reviewable content is the mapping agent → block.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S7 | inline | |
| T2 | S8–S9 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order; this follow-up was found in the S4 final inventory after
  PR #510, where `syncagent_test.go:232` was classified as "live fixture oracle with a skip".
- L2: Scratch/TMPDIR: `/Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-platformsh` (Robert's
  standing scratch convention); every `go test` / `go build` / `validate.sh` ran with `TMPDIR` pointing at its
  `tmp/`, and each command's rc is appended to its log in `logs/`.
- L3: Baseline before any edit: `TestAgentPlatformMatchesPlatformSh` + `TestAgentPlatformCoversEightAgents`
  rc=0 (`logs/00-baseline.log`).
- L4: RED after the restructure with no golden: `read frozen oracle: open
  testdata/platform_sh_golden.json: no such file or directory` at `syncagent_test.go:293`, rc=1
  (`logs/01-red-missing-golden.log`).
- L5: The capture harness was throwaway, outside the worktree
  (`~/.hermes/.../go20-pr4a-s4-platformsh/cap/main.go`): it reproduces the retired splitter regex verbatim
  and emits the loader's own struct shape, so the frozen payload is the legacy split output, not a projection
  of the port. The worktree copy is byte-identical to the base blob (`git show f798526:… | cmp -` clean).
- L6: Captured twice and `cmp` byte-identical (`logs/02-capture1.log`, `logs/02-capture2.log`,
  `logs/03-capture-cmp.log` cmp_rc=0): 6789 B, 8 blocks in source order, `blob` metadata
  `c03817f17be904ec1839b0081f2cf5e41d9b0107` = `git rev-parse f798526:lib/_internal/platform.sh`. The leak grep
  over the committed golden for `/Users/`, `/private/`, `/var/folders` is 0.
- L7: First GREEN attempt failed rc=1 on a loader check of mine that assumed block text starts AT the agent
  line; the retired helper sliced from the END of the agent line (`src[m[1]:end]`). The check was corrected to
  "the block carries its own field echoes" (the true invariant) instead of changing the frozen bytes, which
  keep the legacy boundaries.
- L8: Negative controls, each rc=1, golden restored from the pristine capture and `cmp`-verified
  byte-identical after each (`logs/05-nc-mutate-block.log` — `agent cursor field mcp_key: go = "mcpServers",
  platform.sh = "mcpServersX"`; `logs/06-nc-delete-agent.log` — `7 records, want 8`;
  `logs/07-nc-duplicate-agent.log` — `9 records, want 8`).
- L9: No-`lib/` proof: with `lib/_internal/platform.sh` renamed for the duration of the run, the focused test
  was rc=0 with `--- PASS` on both platform tests and zero `SKIP` lines (`logs/08-lib-renamed.log`); the file
  was restored immediately, `git status --short lib/` was empty and the restored file is byte-identical to the
  base blob.
- L10: GREEN: `gofmt -l internal/sync` empty (`logs/09-gofmt.log`), `go vet ./internal/sync/` rc=0
  (`logs/10-vet.log`), `go test ./internal/sync/ -count=1` rc=0 in 11.94s (`logs/12-package-tests.log`).
- L11: Remaining `lib/` inventory over `internal/` (`logs/13-internal-librefs.log`, 152 hits): 111 provenance
  comments; 24 live repo-`lib/` read/exec sites in `internal/cli` (`skills_test.go` 11, `recipe_test.go` 13 —
  all `os.Stat` + `t.Skipf` differential oracles); `internal/sync/syncagent_test.go:33` reads
  `lib/sync-agent.sh` live with a skip (the two `cd`-line/usage-heredoc pins — same failure mode, out of this
  slice's goal, reported as a follow-up); the rest are assertion/diagnostic strings
  (`gates/docs_contract_test.go:203`, `target/differential_test.go:524`, `lock/lock_test.go:57`,
  `sync/sync_test.go:22`, `cli/smoke_test.go:376-385`, `syncagent_test.go:273-274`) or native guards that
  prove the legacy `lib/` route does NOT run (`cli/init_test.go:127,428,615-634`,
  `cli/upgrade_route_test.go:82,99-100`, `upgrade/*`, `cli/recipe_test.go:643`).
- L12: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/15-build.log`) and `./tests/validate.sh` rc=0
  (`logs/16-validate.log`). The worktree carries only the three intended paths; no scratch, cache or capture
  artifact is inside it, and the change is left unstaged for the orchestrator.
