# Tasks: playwright-ui-flow

Depth: **full**

Source explore: `openspec/changes/playwright-ui-flow/explore.md`
Source proposal: `openspec/changes/playwright-ui-flow/proposal.md`
Source design: `openspec/changes/playwright-ui-flow/design.md`
Source specs:
- `openspec/changes/playwright-ui-flow/specs/ui-browser-testing/spec.md`

Execution mode: **strict TDD**. Phase 1 MUST show RED before Phase 2 catalog
content lands.

Legend: `[P]` = can run in parallel with sibling `[P]` tasks in the same phase.
Unmarked tasks are sequential within the phase.

Tracker: Trello https://trello.com/c/QssRysPv (card 44)

Design locks (do not reopen without re-auth):
- Topology **B**: `playwright-ui-flow` (base) + `playwright-mcp` (add-on); hybrid = enable both
- Capability `ui-browser-testing` declared only on base
- Skills: `ui-browser-testing` (discipline) + `playwright-cli` + `playwright-mcp` (thin adapters)
- No third hybrid recipe; no `mode` flag; no conditional-MCP harness work

---

## Phase 1 — Tests scaffolding (RED)

- [x] **T1.1** — Create `tests/test_playwright_ui_flow_recipes.py` (or split files)
  with ROOT, recipe path helpers, and unittest conventions matching nearby
  recipe materialize tests.
  **Done when:** file collects; no false-green for missing recipes.

- [x] **T1.2** `[P]` — RED: both `catalog/recipes/playwright-ui-flow/recipe.toml`
  and `catalog/recipes/playwright-mcp/recipe.toml` load via recipe schema;
  capability `ui-browser-testing` declared **only** on base.
  **Req:** Schema validation / capability declaration.

- [x] **T1.3** `[P]` — RED: enable **both** recipes → no FATAL primitive conflict
  (distinct skill/command/mcp ids); no capability ambiguity (one provider);
  tags do not overlap.
  **Req:** Conflicts (hybrid) + tag hygiene (D8).

- [x] **T1.4** `[P]` — RED: enable base only → materialize `ui-browser-testing` +
  `playwright-cli` skills; recipe-MCP output has **no** `playwright` server.
  **Req:** Materialize (CLI-only).

- [x] **T1.5** `[P]` — RED: enable both → `playwright-mcp` skill + `playwright`
  MCP preset present; `ui-browser-testing` resolves exactly once.
  **Req:** Materialize (hybrid) + skill resolution.

- [x] **T1.6** `[P]` — RED: config merge surfaces `ui_smoke_command`;
  `validate-config` passes with all keys unset; brief renders hybrid
  precedence + `{config.ui_smoke_command}` substitution when set.
  **Req:** Config + brief render.

- [x] **T1.7** `[P]` — RED: MCP preset / docs contain no literal secrets;
  enabling `tdd-flow` + `playwright-ui-flow` syncs cleanly.
  **Req:** Secrets + cross-capability.

- [x] **T1.8** — Confirm Phase 1 RED evidence (fail for missing recipes/assets,
  not import/syntax). Record command + summary.

---

## Phase 2 — Catalog recipes (GREEN)

- [x] **T2.1** — Implement `catalog/recipes/playwright-ui-flow/` per design
  sketch: `recipe.toml`, `skills/ui-browser-testing/SKILL.md`,
  `skills/playwright-cli/SKILL.md`, `commands/ui-smoke.md`, `init.md`,
  `README.md`.
  **Done when:** T1.2/T1.4/T1.6 assertions for base go green.

- [x] **T2.2** — Implement `catalog/recipes/playwright-mcp/` per design sketch:
  `recipe.toml` with `[[provides.mcp]]`, `skills/playwright-mcp/SKILL.md`,
  `init.md`, `README.md`. Adapter defers policy to `ui-browser-testing`.
  **Done when:** T1.3/T1.5/T1.7 go green.

- [x] **T2.3** — Skill content quality: discipline encodes hybrid table +
  evidence + tdd-flow cross-link; adapters open with deferral line; MCP adapter
  documents base-augmentation / explore-only degrade.
  **Done when:** content review matches spec scenarios; skill-sync/frontmatter
  valid.

- [x] **T2.4** — Update `docs/capabilities.md` (`ui-browser-testing` row) and
  `docs/recipes-catalog.md` (both recipes).
  **Done when:** docs assertions / manual check pass.

- [x] **T2.5** — Re-run focused recipe tests + `./tests/validate.sh`; record
  GREEN evidence.

---

## Phase 3 — Close-out prep

- [x] **T3.1** — Write `verify-report.md` against success criteria in proposal.
- [ ] **T3.2** — Commit planning + implementation on `feat/playwright-ui-flow`
  (when user asks); open PR only after artifact gate; archive-tail before merge.

---

## Authorization gate

Stop here until the user authorizes implementation of this plan (design locks
above). Defaults for non-blocking nits unless overridden at auth:

- Ship only `ui-smoke` command in v1 (no `ui-test` slash command).
- Document `@smoke` grep convention in init/README; do not enforce in harness.
- MCP preset uses `@playwright/mcp@latest` (manifest override for pin/browser).
