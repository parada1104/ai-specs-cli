# Verify report: harness-hook-tool-name-matching

**Change**: `harness-hook-tool-name-matching`  
**Branch**: `change/harness-hook-tool-name-matching`  
**Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli-hook-gate-explore`  
**Depth**: Full  
**Trello**: [#53](https://trello.com/c/mMtm3KhA)  
**Verified**: 2026-07-24

## Commands

| Layer | Command | Result |
|-------|---------|--------|
| RED | `python3 -m unittest tests.test_hooks_render.HooksRenderTests.test_opencode_plugin_shim -v` (assert `"i"` before fix) | FAIL — AssertionError: `"i"` flag absent from OpenCode shim |
| GREEN | same after `render_opencode` `"i"` fix | PASS |
| Focused | `python3 -m unittest tests.test_hooks_render -v` | PASS — 9 tests |
| Full | `./tests/validate.sh` | PASS — Ran 1047 tests in 275.211s, OK |

## D ground truth (recorded in design.md)

`@earendil-works/pi-coding-agent` subagent example spawns a **separate process**.
Parent `tool_call` handlers do not see child write/edit calls. Docs updated to
"✅ (this process)" for pi/omp; omp rows added.

## Delivered

- E: OpenCode matcher case-insensitive + unit assert
- A: `docs/runtime-hooks.md` omp + honest status + known-gaps
- B: worktree-flow brief rule + skill + README
- Specs promoted; CHANGELOG Unreleased; recipe version 1.2.4
