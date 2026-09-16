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
- **Explicit item opening.** An item is opened only by a deliberate write, never
  because a `## Tracker` section parses. Grading is pure: no checkpoint, in any mode,
  creates, mutates, or deletes store state, so `always` with no item blocks and leaves
  the store byte-identical.
- **Write surface.** `worktree-gate --ledger --write '<json>'` records exactly four
  kinds beside the existing `--decide`: `open` (open-if-absent under the store lock),
  `link` (native id, URL, native type, state, and an opaque provider payload on the
  provider-neutral core fields), `close`, and `exempt`. `--write` and `--decide` are
  mutually exclusive. Writes are idempotent where they can be: a retried `open`
  reports `applied: false` / `already-open`, a repeated `link` reports `unchanged`,
  and a second `close` reports `already-closed`. Success adds a
  `write: {kind, applied, reason}` sidecar to the verdict JSON. Every declared
  decision kind and core item field now has a production writer.
- **Failed writes fail closed.** A validation failure, a lock timeout (bounded ~100 ms
  attempt), a `change-ambiguous` identity without an explicit slug, or a store IO
  error persists nothing, leaves the store byte-identical, prints
  `worktree-gate: ledger --write failed: …` on stderr, and exits `2` with no stdout
  JSON. Grade paths keep failing open (missing or unverified binary, unreadable
  evidence, flag-parse errors on verdict calls, grade-path lock timeout).
- **Three of four evidence sides.** `lib/_internal/ledger_bridge.py` builds the
  `--evidence` file from local facts only: `local` is the ledger's own store snapshot,
  `code` is the change's `## Tracker` `card_id`, `git` is that same native id when a
  `pr:` is recorded, and `remote` has **no producer in this `--evidence` file**. The
  bridge never grades, never calls `gh`, MCP, or the network, works from a cold CLI
  install with no project cache, and turns any read failure into an empty side (fail
  open). Branch names and PR URLs are deliberately not evidence sides: the conflict
  predicate equality-compares every non-empty side against the local item id.

  Remote reconciliation is a separate, **explicit and opt-in** comparison, never an
  `--evidence` side and never part of the grade: `worktree-gate --ledger --reconcile
  '<observation.json>' --reconcile-event <event>` compares declared properties only.
  The transport acquires the observation, the recipe declares the mapping
  (`[config.reconcile]`), and the gate reads the project manifest — a project without
  the block gets an explicit `unconfigured`, never a default. It writes neither the
  store nor the provider, stays off every hook, and leaves the graded exit code
  unchanged.
- **`tracker.none` is evidence, not a durable exemption on its own.** The human-authored
  `openspec/changes/<slug>/tracker.none` is presentation/evidence-only; it becomes
  `Item.Exemption` only through the explicit human/agent `exempt` write, whose reason
  is supplied in the payload. No host auto-records it (R1) — hosts treat it as blank
  `code` evidence and never create, modify, or delete the file. Once recorded it is
  honored at every checkpoint as allow/exempt and never registers as a conflict.
  Removing the file does not auto-revoke it — reopening evidence is a human act
  (`--decide '{"kind":"adjudicate","choice":"…"}'` clears the exemption).
- **No provider writes in this slice.** The ledger records and reconciles evidence;
  it performs no MCP/API create, update, move, comment, or label call. `always`
  blocks until an item is supplied, it does not create one.
- **Witness-derived configuration.** The tracker gate, the pre-merge guardian, and
  `doctor` resolve the bound recipe id from the witness and read
  `recipes.<id>.config` for `ledger_mode` / `gate_mode`; the legacy literal survives
  only as the bridge's fallback. Reading the witness is acquisition, not grading.
- **Remaining work, named not silent.** `work-start` is still hosted by
  `plan-build-flow`, whose `plan-build-gate.sh` resolves its config through the legacy
  literal and is intentionally out of scope here. A tracker-bound project without
  `plan-build-flow` enabled has an unhosted `work-start`; `doctor` reports it as an
  INFO line (`work-start is unhosted`) while the other four checkpoints keep grading.

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
