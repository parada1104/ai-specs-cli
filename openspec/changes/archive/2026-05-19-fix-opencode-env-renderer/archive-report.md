# Archive report — fix-opencode-env-renderer

**Archived:** 2026-05-19
**Branch:** fix/opencode-env-renderer
**Status:** ready-to-merge

## Outcome

- Bug fix landed for `lib/_internal/mcp-render.py:54`. The `_ENV_VAR_RE` regex now matches both `$VAR` and `${VAR}` env-variable references; canonical output per agent is unchanged (`{env:VAR}` for OpenCode, `${VAR}` for generic Claude/Cursor).
- TDD evidence: opencode braced test went RED → GREEN; cursor/claude braced tests pass as regression guards (they were green pre-fix by coincidence — see tasks 1.4).
- Spec coverage: new capability `mcp-env-rendering` promoted to `openspec/specs/mcp-env-rendering/spec.md` with 5 scenarios (opencode braced, cursor braced, claude braced, byte-identical $VAR/${VAR} parity, literal pass-through).
- Documentation: `docs/ai-specs-toml.md` Compatibility rules now document both forms.

## Files changed

- `lib/_internal/mcp-render.py` — regex one-liner (`_ENV_VAR_RE`).
- `tests/test_sync_pipeline.py` — 3 new tests covering `${VAR}` form for opencode, cursor, claude.
- `docs/ai-specs-toml.md` — added compatibility-rules bullet for the accepted env reference forms.
- `openspec/specs/mcp-env-rendering/spec.md` — new capability spec promoted from delta.
- `openspec/changes/archive/2026-05-19-fix-opencode-env-renderer/` — full SDD trail (proposal, delta spec, tasks, this report).

## Verification

- `./tests/validate.sh` — exit 0.
- Env-rendering focused suite (3 new + 5 existing tests) — all pass.
- Full `./tests/run.sh` — 247 / 253 pass; the 6 failures (AGENTS.md fan-out, README needles) reproduce on unmodified main and are unrelated to this change.

## Risks accepted

- Relaxed regex now also accepts the malformed `${VAR` (open brace, no close). Treated as benign — almost certainly a typo intending `${VAR}`, and the normalised output is more useful than the literal. No existing test relies on rejecting unbalanced braces.

## Follow-up (out of scope here)

- The 6 pre-existing test failures need separate triage:
  - 4 AGENTS.md fan-out tests expect content/files that the current sync pipeline no longer produces.
  - 2 testing-foundation tests reference README phrases (`Testing foundation exists`, `skill-sync`) that have been removed from the README.
