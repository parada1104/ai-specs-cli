# Recipe capabilities

A **capability** is an abstract role a recipe can *provide* or *consume*. It is
the seam that keeps the catalog general: foundational recipes speak in terms of
capabilities, and concrete integrations provide them. The tool is a detail; the
capability is the contract.

## Two tiers of recipe

| Tier | Role | Speaks in terms of | Examples |
|------|------|--------------------|----------|
| **Foundational** | Encodes a reusable *pattern*; tool-agnostic and configurable | capabilities | `worktree-flow`, `tdd-flow`, `session-context` |
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
| `vcs-pr-flow` | Branch → PR/MR → review → merge | `git-pr-flow` (GitHub/gh), `gitlab-mr-flow` (GitLab/glab), `bitbucket-pr-flow` (Bitbucket/bb) |
| `test-runner` | Red-green-refactor discipline with a project test command | `tdd-flow` |
| `ui-browser-testing` | Browser UI suites/smokes (+ optional explore surface) | `playwright-ui-flow` (CLI/discipline); optional add-on `playwright-mcp` |
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

## Tracker lifecycle: the Tracker Ledger

When a `tracker` capability is bound, its lifecycle is graded by **one Go
predicate**, shipped as a `--ledger` mode of the existing verified
`worktree-gate` binary. The provider recipe supplies configuration; it is not the
grader. Core item fields stay provider-neutral (item id, provider id, native type,
URL, state, exemption, evidence references); anything provider-specific lives in
provider recipe config and in an opaque object the predicate never reads. No
provider vocabulary is promoted into the `## Tracker` authoring contract, and no
universal artifact field is introduced.

- **Activation witness.** `ai-specs sync` persists the already-computed binding
  outcome at `<git-common-dir>/ai-specs/ledger/witness.json`, recording exactly one
  state: `bound`, `ambiguous`, `unbound`, or `declared-not-bound`. A declaration
alone is supply, not activation: only `bound` activates the ledger, and a missing,
unreadable, or unknown-version witness stays dormant (`witness-missing`) without
ever guessing a provider.
- **Ledger store.** One record per work identity (Git common dir + current short
  branch, optionally enriched by the active change slug) at
  `<git-common-dir>/ai-specs/ledger/state.json`, written atomically. A missing file
  reads as an empty item set; a corrupt file reads as unevaluable, never as a
  synthesized item. A branch reused after its item closed opens a new item, and two
  open items for one identity are a human conflict, never a silent pick.
- **Five checkpoints.** `work-start` (plan-build gate), `apply-start` and
  `pr-review` (tracker gate), and `pre-merge` / `archive-close` (pre-merge guardian)
  all reach the same predicate and share one exit contract (`0` allow/ask/dormant,
  `2` only when the host must stop). Path hosts never block `openspec/**`.
- **Modes.** One project `ledger_mode`: `always`, `ask`, or `warn` (default `warn`;
  promotion is an explicit human configuration change). `ask` opt-out is
  checkpoint-scoped, so the next checkpoint prompts again.
- **Dormancy is `doctor` only.** A `tracker-ledger` check reports `unbound` (INFO),
  `ambiguous` / `declared-not-bound` / missing witness / recorded conflict (WARN),
  and infrastructure failure (ERROR). The runtime brief gains no dormancy line.
- **No provider writes in this slice.** The ledger records and reconciles evidence;
  it performs no MCP/API create, update, move, comment, or label call. `always`
  blocks until an item is supplied, it does not create one.

This is the project-level Python→Go strangler policy applied at the seam it
touches: the Go predicate is the single authoritative grader for `## Tracker`
validity, and the legacy Python copies (the link parser, the hook heredoc, and
`doctor`) delegate to it or are held by parity tests that fail on divergence. The
Python that remains is a thin acquisition/witness/JSON bridge only.

## Why this matters

Mixing the pattern and the vendor in one recipe makes it impossible to reuse the
pattern with a different tool. Splitting them means a project assembles its flow
by picking one foundational recipe per capability plus one concrete provider —
and swapping the provider (Trello → Jira, GitHub → GitLab) never touches the
foundational layer.
