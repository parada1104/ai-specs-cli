# [Go 07.S8] Skill contract — Go `VendoredSkillMarkdown`

Card: `[Go 07.S8]` — https://trello.com/c/WNMoUR5w (parent epic `go-single-binary`)
Branch: `change/go-07-s8-skill-contract` · Worktree: `.worktrees/go-07-s8-skill-contract`
Base: `004a538` (S7 merge) · Plan: `odd/tasks/go-07-sync-plan.md` (S8 row)

## Scope decision

- `split_frontmatter`, `parse_frontmatter`, `_strip_quotes`,
  `_split_inline_list` were already ported in Go 08 (`internal/skills/contract.go`).
- The only remaining production surface is
  `render_skill_markdown(from_dep(dep, upstream))`, called by
  `vendor-skills.py:196` on the sync path (deps vendoring). Ported as one
  library entry, `skills.VendoredSkillMarkdown(dep *toml.Table, upstream)`.
  No wiring: vendor-skills stays Python until its owning slice (S2/S4–S7
  precedent).
- **Not ported (YAGNI)**: `normalize_local_skill`, `from_local_skill`,
  `read_skill_text`, `validate_sync_metadata` and the `sync-metadata` CLI. Zero
  production callers (only `tests/test_skill_contract.py`,
  `test_orca_aware_delegation_skill.py`, `test_harness_cli_literacy.py`); they
  die with the Python in S16.
- `internal/config` exports `PyStr`/`Truthy` (thin wrappers) so `str(x or y)`
  over tomllib values reuses the existing reprs instead of a sixth copy.

## Evidence

- RED: `go test ./internal/skills -run Vendored` → `undefined: VendoredSkillMarkdown`.
- GREEN: `TestVendoredSkillMarkdownDifferential` — 25 cases against the REAL
  `skill_contract.py` (deps parsed by `tomllib`, as `load_deps` does): rendered
  markdown byte-equal, or identical `SkillContractError` text. Covers scalar/list
  scope + auto_invoke, empty-list fallbacks, missing/empty/list upstream
  description, numeric/bool tomllib values through `str()`, every error branch
  (id, source, version, scope type/items, upstream frontmatter), YAML quote
  escaping, Python-only whitespace (`\x1c`–`\x1f`) in `strip()`.
- Mutations killed (9/9): backslash escape, `rstrip(". ")`, `strip()` →
  `TrimSpace`, root scope default, list-description repr, version default, …
- `go vet ./... && go test ./...` rc 0; `./tests/validate.sh` → see PR.
