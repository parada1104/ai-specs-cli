# Tasks — fix-opencode-env-renderer

Strict TDD applies. Each implementation task pairs a failing test (RED) with the minimum code change to pass (GREEN), then refactor if needed.

## 1. Testing

- [x] 1.1 (RED) Add `test_sync_renders_opencode_mcp_env_with_braced_dollar_syntax_input` in `tests/test_sync_pipeline.py` — same shape as `test_sync_accepts_mcp_environment_alias_and_renders_canonical_output` but using `'${DEMO_API_KEY}'` in TOML and asserting `"{env:DEMO_API_KEY}"` in the rendered `opencode.json`.
- [x] 1.2 (RED) Add `test_sync_renders_cursor_mcp_env_with_braced_dollar_syntax_input` in `tests/test_sync_pipeline.py` — TOML uses `'${DEMO_API_KEY}'`, asserts `"${DEMO_API_KEY}"` in the rendered `.cursor/mcp.json`.
- [x] 1.3 (RED) Add `test_sync_renders_claude_mcp_env_with_braced_dollar_syntax_input` in `tests/test_sync_pipeline.py` — TOML uses `'${DEMO_API_KEY}'`, asserts `"${DEMO_API_KEY}"` in the rendered `.mcp.json`.
- [x] 1.4 (RED → GREEN) Confirmed the opencode test fails with `AssertionError` (diff shows `"${DEMO_API_KEY}"` vs expected `"{env:DEMO_API_KEY}"`). Cursor/Claude tests already pass coincidentally pre-fix because the unmodified pass-through happens to match the canonical generic form; they remain as regression guards.

## 2. Implementation

- [x] 2.1 (GREEN) Updated `_ENV_VAR_RE` in `lib/_internal/mcp-render.py:54` from `r"^\$([A-Z_][A-Z0-9_]*)$"` to `r"^\$\{?([A-Z_][A-Z0-9_]*)\}?$"`. Single capture group preserved, so all three `re.sub` callsites (lines 65, 79, 119) work unchanged.
- [x] 2.2 (GREEN) Ran focused unittest selection — 8 env-related tests pass (3 new + 5 existing).

## 3. Documentation

- [x] 3.1 Added a bullet to `docs/ai-specs-toml.md` "Compatibility rules" documenting that both `$VAR` and `${VAR}` are accepted and listing the canonical per-agent output forms.

## 4. Verification

- [x] 4.1 Ran `./tests/validate.sh` — exit 0 (py_compile + bash -n + unittest all clear for affected modules).
- [x] 4.2 Full `./tests/run.sh` runs 253 tests; the same 6 failures reproduce on the unmodified main branch (AGENTS.md fan-out + outdated README needles) — pre-existing, unrelated to this change. Confirmed via `git stash` round-trip.
