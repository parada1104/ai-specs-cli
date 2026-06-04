# Verify report: recipe-runtime-hooks

Independent verification (separate from the apply): full suite, live 4-harness
sync inspection, source review against the specs, idempotency.

## Result: PASS (with 1 bug found & fixed, minor follow-ups noted)

## Evidence

- **Suite:** `./tests/validate.sh` → exit 0, **349 tests OK** (py_compile + bash -n + unittest), after the verify fix.
- **Live sync** on a temp fixture enabling all 4 harnesses + `worktree-flow`:
  - Claude → `.claude/settings.json` managed block (`PreToolUse`, matcher, command → `$CLAUDE_PROJECT_DIR/...gate.sh`, `env.WORKTREE_GATE_PROTECTED`). ✓
  - OpenCode → `.opencode/plugin/worktree-flow-worktree-gate.ts` (`tool.execute.before`, spawnSync, exit 2 → throw, documents #5894/#2319). ✓
  - Pi → `.pi/extensions/worktree-flow-worktree-gate.ts` (`pi.on("tool_call")`, return `{block:true}`). ✓
  - Cursor → **warn-and-skip**, exact message: `! cursor: no pre-file-write hook exists; skipping hook 'worktree-flow:worktree-gate' (matcher 'Edit|Write|MultiEdit|NotebookEdit') for cursor`. ✓ (honest, per spec)
  - Script materialized once to `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`. ✓
- **Idempotency:** second `sync` → `.claude/settings.json`, OpenCode, Pi all byte-identical (diff empty). ✓
- **Spec coverage:** schema validation (5 scenarios), event map + warn-and-skip, direct (Claude) vs adapter (Cursor/OpenCode/Pi) rendering, normalized contract, managed block, config→env — all have passing tests.

## Bug found & fixed during verify

- **Pi adapter read snake_case field names** (`call?.tool_name` / `call?.arguments`).
  Verified June 2026 research says Pi exposes the tool name as **camelCase
  `toolName`** and input as **`input`**. With snake_case, `toolName` resolved to
  `""`, the matcher never matched, and **the hook would silently never fire on
  Pi**. The golden test was too coarse to catch it (asserted only `pi.on`/`block`).
  - Fix: `hooks-render.py` Pi renderer now reads `call?.toolName ?? call?.tool_name ?? call?.name`, `tool_input: call?.input ?? call?.arguments`, and imports `ExtensionAPI` (was `Pi`). Added a regression assertion (`call?.toolName`, `ExtensionAPI`) to `test_pi_extension_shim`.

## Accepted deviations / known follow-ups (not blocking)

1. **OpenCode/Pi relative `SCRIPT` path** — the shims spawn the script by its
   project-relative path; if the harness runs from a non-root cwd the script may
   not resolve. This **fails open** (no crash, just no enforcement). Harden later
   by resolving against the plugin `directory` (OpenCode has it) / an absolute path.
2. **Cursor wrapper message channel** — wrapper surfaces script *stdout* as
   `agent_message`, while the normalized contract puts block messages on *stderr*.
   Moot for worktree-gate (skipped on Cursor); refine for future Cursor-targeted
   shell/MCP hooks.
3. **TS shims not executed against live OpenCode/Pi runtimes** (none in CI) —
   validated structurally by golden tests only. The Pi bug above is exactly the
   risk; mitigated by cross-checking verified research. Recommend a smoke test on
   real harnesses before advertising full parity.
4. **Subrepo fan-out** — the standalone `sync-agent` enrichment path does not emit
   hooks; the primary `ai-specs sync` path does. Follow-up if subrepo hooks needed.

## Scope hygiene note (for merge)

The `recipe-docs-cleanup` branch still carries the earlier **prototype** (template-
bundled `worktree-gate.sh` + worktree-flow version bump + fixture pin + a
`docs/runtime-hooks.md` + README compat row). Those overlap with the productized
versions here. Before merge, slim `recipe-docs-cleanup` to the pure docs
(`docs/recipes-catalog.md` + README recipes link) so the two branches don't collide.
