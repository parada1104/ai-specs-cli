# Fix OpenCode env variable rendering for `${VAR}` form

## Problem

`lib/_internal/mcp-render.py` translates env references from `ai-specs.toml` into per-agent config files. The regex used to detect env-variable references is:

```python
_ENV_VAR_RE = re.compile(r"^\$([A-Z_][A-Z0-9_]*)$")
```

This matches the plain shell form `$VARIABLE_NAME` only. When a user writes the braced form `${VARIABLE_NAME}` in their TOML — which several existing projects already do — the value does NOT match the regex and passes through to the rendered config as a literal string:

| TOML input          | OpenCode `environment` (current) | OpenCode `environment` (expected) |
|---------------------|----------------------------------|-----------------------------------|
| `'$DEMO_API_KEY'`   | `"{env:DEMO_API_KEY}"` ✅         | `"{env:DEMO_API_KEY}"`            |
| `'${DEMO_API_KEY}'` | `"${DEMO_API_KEY}"` ❌            | `"{env:DEMO_API_KEY}"`            |

OpenCode cannot resolve `${VAR}` because its native syntax is `{env:VAR}`. The same defect affects generic (Claude/Cursor) rendering: instead of normalising to `${VAR}`, the braced input is preserved unchanged, so any downstream tool that depends on the canonical form sees inconsistent values across projects.

Confirmed locally with a one-line repro: `re.compile(r"^\$([A-Z_][A-Z0-9_]*)$").match("${DEMO_API_KEY}")` returns `None`.

## Solution

Update `_ENV_VAR_RE` to tolerate the braced form as a defensive fallback:

```python
_ENV_VAR_RE = re.compile(r"^\$\{?([A-Z_][A-Z0-9_]*)\}?$")
```

The single capture group still yields the variable name, so the existing `re.sub(r"{env:\1}", ...)` and `re.sub(r"${\1}", ...)` callsites keep working without further changes.

After the fix, both `$VAR` and `${VAR}` produce the same canonical output per agent:

- OpenCode (`environment` field): `{env:VAR}`
- Generic — Claude/Cursor (`env` field): `${VAR}`

The canonical TOML format remains plain `$VARIABLE_NAME`; the braced form is accepted as input only, not promoted as the recommended style.

## Affected modules

- `lib/_internal/mcp-render.py` — single-line regex change
- `tests/test_sync_pipeline.py` — add coverage for the `${VAR}` form across opencode/cursor/claude

## Risks

- **Behaviour change for users who relied on the literal pass-through**: extremely unlikely. The current behaviour produces unusable configs (OpenCode cannot resolve `${VAR}` natively). No realistic workflow depends on the literal output. Mitigation: documented in spec scenarios; coverage added.
- **Regex now accepts `${VAR` (unbalanced brace)**: benign — such input is almost certainly a typo intending `${VAR}` and the normalised output is more useful than the literal. No existing test relies on rejecting unbalanced braces.

## Rollback plan

Revert `lib/_internal/mcp-render.py` to the previous regex. No data migration required; output files are regenerated from TOML on every `ai-specs sync`.
