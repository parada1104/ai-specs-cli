# Design: Remove SDD/OpenSpec from Documentation

## Technical Approach

Pure documentation cleanup: delete 3 files/directories and surgically edit 7 files to remove every user-facing SDD/OpenSpec reference. No code changes, no manifest schema changes, no behavior changes. The guiding principle is conciseness — match the main branch's terse style, never add fluff.

## Architecture Decisions

### Decision: Hard-delete vs. archive

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Hard-delete | 404 for external bookmarks; gentle-ai now owns SDD docs | **Chosen** — product no longer ships SDD, so keeping docs is actively confusing |
| Archive to `docs/deprecated/` | Preserves URLs but still surfaces dead content | Rejected — creates maintenance burden and signals the feature still exists |

### Decision: README replacement content

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Replace SDD section with "Harness engineering" / "Primitives" / "Fan-out" | Accurately describes what ai-specs-cli does today | **Chosen** — aligns docs with actual product |
| Simply delete SDD section with no replacement | Leaves a gap in key concepts | Rejected — README should still explain the product's purpose |

### Decision: AGENTS.md rewrite scope

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Rewrite Runtime Flow, Context Sources, Conflict Policy, Workflow Rules | Removes all OpenSpec/worktree/SDD language | **Chosen** — these sections are the primary SDD prescription in the runtime brief |
| Leave AGENTS.md unchanged | SDD references persist in the canonical runtime brief | Rejected — contradicts the change intent |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `docs/ai/sdd.md` | Delete | Full SDD provider guide — product no longer ships SDD |
| `docs/ai/examples/config.yaml` | Delete | OpenSpec `spec-driven` config example — no provider to configure |
| `catalog/skills/openspec-sdd-conventions/SKILL.md` | Delete | SDD conventions skill — no OpenSpec workflow to convention-alize |
| `README.md` | Modify | Remove `ai-specs sdd` CLI row; replace SDD key concept with harness-engineering note; remove "and SDD configuration" from manifest description |
| `AGENTS.md` | Modify | Strip OpenSpec/SDD from Purpose, Runtime Flow, Context Sources, Conflict Policy, Workflow Rules |
| `docs/ai-specs-toml.md` | Modify | Cut `[sdd]` from canonical surface, field table, manifest section, and example; remove SDD links from See also |
| `docs/ai/troubleshooting.md` | Modify | Delete "SDD failures" section (3 subsections); remove SDD link from See also |
| `docs/ai/examples/minimal-manifest.toml` | Modify | Remove `[sdd]` block |
| `catalog/README.md` | Modify | Remove `openspec-sdd-conventions` entry and its `[[deps]]` example |
| `catalog/recipes/trello-mcp-workflow/README.md` | Modify | Change "ai-specs SDD workflows" → "ai-specs projects" |
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` | Modify | Remove all SDD/OpenSpec language; generalize capabilities to project tracking primitives |

## Detailed Edits

### 1. `README.md`

**Remove** the `ai-specs sdd enable/disable/status` row from the CLI table.

**Remove** the entire "SDD (Spec-Driven Development)" key-concept section (heading + paragraph + toml block + paragraph).

**Replace** it with a concise "Harness engineering" paragraph:

```markdown
### Harness engineering

`ai-specs` treats agent configuration as infrastructure: a single manifest fans out
to every enabled tool. The primitives are skills, MCP servers, recipes, and
derived instructions — versioned, vendored, and reproducible.
```

**In the Manifest key concept**, change:
> "Declares enabled agents, MCP servers, skill dependencies, recipes, and SDD configuration."

to:
> "Declares enabled agents, MCP servers, skill dependencies, and recipes."

**In the opening paragraph**, keep the existing gentle-ai mention but tighten:
> "Agent orchestration (multi-phase planning, multi-model sub-agents, profiles) is handled by gentle-ai. ai-specs focuses on the spec layer and tool integrations (recipes) — the fan-out across repos and harnesses."

(No change needed to opening paragraph — already accurate and concise.)

### 2. `AGENTS.md`

**Purpose line**: Change
> "Purpose: per-project AI harness for agent configuration, MCPs, recipes, memory, tracker integration, and OpenSpec/SDD workflows."

to:
> "Purpose: per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration."

**Runtime Flow section**: Replace entire section with:

```markdown
## Runtime Flow

- A session works on one explicit user request or Trello card.
- The orchestrator coordinates work inline using project skills and the runtime brief.
- `explore` can run without a worktree when it only produces thinking.
- Artifact phases and implementation phases run in a dedicated worktree when they write files.
- VCS/PR provider: GitHub through `gh` CLI.
```

**Context Sources section**: Replace
> "OpenSpec is the source of truth for specs, changes, tasks, apply evidence, verify reports, and archives."

with:
> "Specs and changes are tracked in the project's designated spec store (configurable per project)."

And remove the OpenSpec bullet entirely; keep Vault, Engram, Skills bullets.

**Conflict Policy section**: Replace
> "Trello controls work state; OpenSpec controls SDD artifacts; Vault controls canonical decisions and handoffs"

with:
> "Trello controls work state; Vault controls canonical decisions and handoffs"

**Workflow Rules section**: Replace
> "For OpenSpec changes, use the project SDD workflow."

with:
> "Follow the project's designated workflow for structured changes."

Also remove the worktree-specific rules that are SDD-centric:
- "`explore` can run without a worktree when it only produces thinking. Create the worktree before `openspec-new-change` or any artifact-writing phase."
- "Artifact phases (`proposal`, `specs`, `design`, `tasks`) and implementation phases (`apply`, `verify`, `archive`) run inside the dedicated worktree."

Replace with neutral language:
- "Create a dedicated worktree for changes that write artifacts or modify code. Pure exploration can happen before a worktree if it writes no files."

### 3. `docs/ai-specs-toml.md`

**Canonical V1 surface list**: Remove `- [sdd] (optional)` from the bulleted list.

**Field classification table**: Remove the entire `[sdd]` row:
> `| [sdd] | enabled, provider, artifact_store | optional; provider = openspec in v1 |`

**Manifest sections**: Delete the entire `### [sdd]` subsection (heading, toml block, and paragraph referencing `docs/ai/sdd.md`).

**Example manifest**: Remove the final `[sdd]` block from the example:
```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

**See also**: Remove:
> `- [docs/ai/sdd.md](ai/sdd.md)`

Also remove the sentence in Compatibility rules:
> "Recipe-specific schema details live in [docs/recipe-schema.md](recipe-schema.md). SDD provider behavior lives in [docs/ai/sdd.md](ai/sdd.md)."

Change to:
> "Recipe-specific schema details live in [docs/recipe-schema.md](recipe-schema.md)."

### 4. `docs/ai/troubleshooting.md`

**Subtitle**: Change
> "Common issues and fixes for the ai-specs SDD integration and manifest pipeline."

to:
> "Common issues and fixes for the ai-specs manifest pipeline."

**Delete** the entire "SDD failures" section and its three subsections:
- `openspec: command not found`
- `openspec init` fails with "already initialized"
- `artifact_store = "memory"` but `openspec/` is missing

**See also**: Remove:
> `- [docs/ai/sdd.md](sdd.md) — SDD provider contract`

### 5. `docs/ai/examples/minimal-manifest.toml`

**Remove** the `[sdd]` block:
```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

### 6. `catalog/README.md`

**Remove** the entire `openspec-sdd-conventions` entry (heading, description, and `[[deps]]` block).

**Remove** the `[[deps]]` example under the entry.

**Update** the "Repeat with" sentence to remove `openspec-sdd-conventions`:
> "Repeat with `testing-foundation` paths as needed, or use `ai-specs add-dep` with `--subdir` pointing at `catalog/skills/<name>` (same effect)."

### 7. `catalog/recipes/trello-mcp-workflow/README.md`

**First line**: Change
> "Automated Trello board integration for ai-specs SDD workflows."

to:
> "Automated Trello board integration for ai-specs projects."

### 8. `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md`

**Description frontmatter**: Change
> "Automated Trello board integration for ai-specs SDD (Spec-Driven Development) workflows."

to:
> "Automated Trello board integration for ai-specs projects."

**Capability: trello-card-linking**: Remove OpenSpec-specific language. Change trigger from:
> "OpenSpec change creation (openspec-new-change or openspec-propose)."

to:
> "New structured change or feature request."

Remove references to `openspec/changes/my-feature` paths and OpenSpec artifact lists. Generalize to:
> "Change name, change folder path (relative to project root), and list of expected artifacts."

**Capability: trello-state-sync**: Remove SDD phase-to-list and phase-to-label mapping tables entirely. Replace with a generic note:

```markdown
## Capability: trello-state-sync

Synchronize project phase transitions with Trello card position and labels.

### Trigger

Phase transitions defined by the project's workflow (e.g., design → implementation → review → done).

### Steps

1. Identify the linked card (from session context or change metadata).
2. Resolve the target list ID by name using board lists (query with `trello_get_lists`).
3. Move the card to the target list using `trello_move_card`.
4. Update labels on the card using `trello_update_card_details`.
5. Post a phase-transition comment using `trello_add_comment`.

Phase-to-list and phase-to-label mappings are project-specific and configured in the recipe config or project conventions.
```

**Capability: trello-progress-comment**: Remove SDD-specific triggers. Change:
> "After successful completion of apply and verify SDD phases."

to:
> "After significant implementation milestones or at project-defined review points."

Remove references to `apply-progress.md` and `verify-report.md` as hard requirements. Generalize:
> "Collect available progress data (changed files, test results, review notes) from the project workspace."

**Graceful Degradation (General)**: Change:
> "Trello failures never block agent progress. The agent continues its SDD workflow regardless of Trello availability."

to:
> "Trello failures never block agent progress. The agent continues its work regardless of Trello availability."

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Static | No `sdd` / `openspec` / `spec-driven` strings in modified docs | `grep -ri` across affected files |
| Link integrity | No broken internal cross-references | Manual review: every `[]()` link in modified files must resolve |
| Consistency | README, AGENTS.md, and ai-specs-toml.md describe the same manifest surface | Diff-check: manifest description matches field table matches example |

## Migration / Rollout

No migration required. This is a documentation-only change. Users with existing `[sdd]` blocks in their manifests will see them ignored by future `ai-specs sync` versions (the validator already tolerates unknown sections with a warning).

## Open Questions

- [ ] Should the `ai-specs sdd` CLI subcommand also be removed from `lib/` code in a follow-up change? (Out of scope for this docs-only change, but noted for future cleanup.)
- [ ] Should `ai-specs/skills/` in this repo still contain `openspec-*` skills for backward compatibility? (Proposal says delete the catalog entry; the dogfooding copy under `ai-specs/skills/` may need separate evaluation.)
