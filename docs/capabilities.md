# Recipe capabilities

A **capability** is an abstract role a recipe can *provide* or *consume*. It is
the seam that keeps the catalog general: foundational recipes speak in terms of
capabilities, and concrete integrations provide them. The tool is a detail; the
capability is the contract.

## Two tiers of recipe

| Tier | Role | Speaks in terms of | Examples |
|------|------|--------------------|----------|
| **Foundational** | Encodes a reusable *pattern*; tool-agnostic and configurable | capabilities | `worktree-flow`, `tdd-flow`, `session-context`, `git-pr-flow` |
| **Specific** | A concrete integration that *provides* a capability | a named tool/service | `trello-mcp-workflow`, `vault-canonical-store`, future `github-*`, `gitlab-*`, `jira-*` |

A foundational recipe should never hardcode a vendor (Trello, Engram, Obsidian,
GitHub). It refers to the capability it needs (`tracker`, `memory`,
`canonical-store`, …). A project wires a concrete provider to that capability
through the manifest.

## Canonical capabilities

| Capability | Meaning | Typical provider |
|------------|---------|------------------|
| `tracker` | Work-state tracking (cards/issues, status, dependencies) | `trello-mcp-workflow`; future `jira-*`, `github-issues-*` |
| `memory` | Operational/session memory (searchable continuity) | the gentle-ai stack (Engram) |
| `canonical-store` | Durable decisions and handoffs | `vault-canonical-store`; future `notion-*` |
| `vcs-pr-flow` | Branch → PR/MR → review → merge | `git-pr-flow` (GitHub/gh); future `gitlab-*` |
| `test-runner` | Red-green-refactor discipline with a project test command | `tdd-flow` |
| `worktree-isolation` | Per-change git worktrees + post-merge cleanup | `worktree-flow` |

This list is the shared vocabulary; new capabilities should be added here before
recipes start declaring them.

## Providing a capability

A recipe declares what it provides in `recipe.toml`:

```toml
[[capabilities]]
id = "tracker"
```

## Consuming a capability (convention)

There is intentionally **no `requires` field** in the schema today — consumption
is a *convention*, not an enforced dependency, to keep the manifest contract
small. A foundational recipe consumes a capability by:

- referring to it by name in its skill text ("the `tracker` capability"), never
  to a specific vendor, and
- documenting the capabilities it expects in its README.

If exactly one enabled recipe provides a capability, `ai-specs sync` auto-binds
it. When several could provide the same capability, the project disambiguates
with an explicit binding:

```toml
[[bindings]]
capability = "tracker"
recipe = "trello-mcp-workflow"
```

See [`docs/recipe-schema.md`](recipe-schema.md) for the `[[capabilities]]` and
[`docs/ai-specs-toml.md`](ai-specs-toml.md) for the `[[bindings]]` contracts.

## Why this matters

Mixing the pattern and the vendor in one recipe makes it impossible to reuse the
pattern with a different tool. Splitting them means a project assembles its flow
by picking one foundational recipe per capability plus one concrete provider —
and swapping the provider (Trello → Jira, GitHub → GitLab) never touches the
foundational layer.
