# Trello MCP Workflow

Automated Trello board integration for ai-specs projects.

## Installation

```bash
ai-specs recipe add trello-mcp-workflow
ai-specs recipe init trello-mcp-workflow
```

This installs:

- **Skill**: `trello-mcp-workflow` — runtime skill with 4 capabilities (session-bootstrap, card-linking, state-sync, progress-comment).
- **Templates**: `card-feature.md`, `card-bug.md`, `card-spike.md`, `card-epic.md`, `card-handoff.md` — Trello card description templates.
- **Command**: `trello-workflow` — quick-reference command file for agents.

`ai-specs recipe add` only declares the recipe in `ai-specs/ai-specs.toml` and writes placeholder config.
`ai-specs sync` materializes the bundled assets.
`ai-specs recipe init trello-mcp-workflow` prints a read-only setup brief so the
project can confirm `board_id`, list mappings, and MCP readiness before sync.

## Template override ownership

The card templates are governed `condition = "not_exists"` overrides. Sync records
the last CLI-written bytes in `[managed.*]` in `ai-specs/.ai-specs.lock`:

| State / policy | Behavior |
|---|---|
| Managed current | No rewrite and no warning. |
| Managed stale + `auto` (default) | Refresh from the current catalog and update the lock. |
| Managed stale + `confirm` or `never-force` | Preserve and warn; refresh explicitly when ready. |
| User-modified or untracked custom | Preserve and warn; never force-overwrite. |

To intentionally replace a customized template, delete that target and sync:

```bash
rm ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md
ai-specs sync
```

Runtime hook scripts follow gate provenance instead of template policy. Sync
records a baseline of the exact bytes the CLI last rendered for each generated
hook; a byte mismatch or missing baseline is preserved with a warning, and an
explicit refresh (`ai-specs sync --refresh-gates`, or `rm <hook> && ai-specs sync`)
replaces a customized hook only after its exact pre-refresh bytes are saved to a
cache-only immutable backup. The ownership policy for templates is separate.

## Configuration

Add configuration under `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`:

| Field | Required | Default | Description |
|---|---|---|---|
| `board_id` | Yes | — | Trello board ID for the project. |
| `default_list` | No | `In Progress` | List name where new cards are created. |
| `epic_list` | No | `Epic` | List name where epic-type cards are placed. |
| `gate_mode` | No | `warn` | Legacy vocabulary: `off` / `warn` / `always`. Consulted only when `ledger_mode` is unset. |
| `ledger_mode` | No | `warn` | Ledger mode: `always` / `ask` / `warn`. `always` blocks missing or conflicted state; `ask` prompts per checkpoint (opt-out is checkpoint-scoped); `warn` reports on stderr and never blocks. Overrides `gate_mode`. |
| `reconcile` | No | — | Declarative remote-reconciliation mapping (`scope_field`, `max_age_seconds`, `expectations`). The recipe schema validates the declared shape and sync carries the project's block through unchanged. The Go gate reads only the project manifest, so an unbound block means every comparison is `unconfigured`, never a default. |

### Example

```toml
[recipes.trello-mcp-workflow]
enabled = true
version = "1.4.0"

[recipes.trello-mcp-workflow.config]
board_id = "69ec097f13e2d38ecd89a557"
```

The optional remote-reconciliation mapping declares which config fields a
comparison may read, is validated on load/sync (unknown keys, wrong types, or an
expectations list over the bound are rejected), and can be set with
`ai-specs recipe configure trello-mcp-workflow --set 'reconcile={...}'`:

```toml
[recipes.trello-mcp-workflow.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900

[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "delivery"
property = "list"
config_field = "default_list"
```

## Tracker-domain adapter

This recipe is a **Tracker-domain adapter**: it provides the `tracker` capability
and extends the one autonomous Go ledger by *mapping*, never by grading. The
mapping is declarative data in `[config.reconcile]` — it names the config field
that carries the scope and, per event, the neutral property each native value must
show. The Go core reads only the project manifest and hardcodes no Trello field,
list, or property name, so another provider (Jira, Linear) can declare the same
shape and feed the same neutral expectation/observation comparator.

Adapter mapping and Tracker-domain policy are separate surfaces. `ledger_mode` /
`gate_mode` decide *when* the ledger speaks and live in `[config.<recipe>]`;
`[config.reconcile]` only declares *what* the recipe maps. The mapping surface
rejects a policy key, and an unbound mapping is an explicit `unconfigured`, never
a default.

## Card-per-change contract

Every active OpenSpec change must carry a `## Tracker` section in `proposal.md`
(fallback `tasks.md`) with a non-empty `card_id` (and preferably `url`):

```markdown
## Tracker

- **card_id**: `<24-hex>`
- **url**: https://trello.com/c/...
```

The `## Tracker` section (and the `tracker.none` exemption) is presentation, not
the grader. The ledger is the only authority: the `tracker-card-gate` hook sends
`apply-start` / `pr-review` to the Go verdict, and `doctor` renders its finding.
The only documented exemption is `openspec/changes/<slug>/tracker.none`
(conceptual name `tracker:none`) with a one-line reason — logged and rare.
Archives are grandfathered.

The global contract is also declared in `openspec/config.yaml` under `tracking:`
(soft guidance for SDD agents). Operational `ledger_mode` / `gate_mode` /
`board_id` still come from recipe config in `ai-specs.toml`.

## Gate modes

The ledger is the only grader. The five checkpoints (`work-start`, `apply-start`,
`pr-review`, `pre-merge`, `archive-close`) all reach the same verified Go
`--ledger` predicate; this recipe only supplies the mode and the hooks.

### Tracker lifecycle host (`pre-merge`, `archive-close`)

The `pre-merge` and `archive-close` checkpoints are Plan Build-independent: the
Tracker-domain host grades them with **no `openspec/` tree**. Run it directly at
the lifecycle boundary — it is the generic command a future Jira/Linear recipe
reuses unchanged:

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/tracker_ledger_host.py" \
  <slug> --root "$PWD" --checkpoint pre-merge
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/tracker_ledger_host.py" \
  <slug> --root "$PWD" --checkpoint archive-close
```

`--root` is required (the resolved project root, never the process cwd) and the
slug is optional. `--stage pre-merge|pre-archive` remains a compatibility alias
for `--checkpoint pre-merge|archive-close`.

**`archive-close` is tracker item closure, not an OpenSpec archive.** It asks the
one Go predicate whether the bound tracker item can be closed; it never moves
`openspec/changes/<slug>/`, never infers closure from archive state, and never
writes. The host is acquisition only (evidence + JSON bridge): no provider write,
no network, no second grader.

This host and the ledger it calls are the generic Tracker domain, not Trello. A
provider recipe contributes **only** its `[config.reconcile]` native-state mapping:
provider recipes do not enable, configure, or implement the ledger, and they never
grade.

| Mode | Behavior |
|------|----------|
| `off` (`gate_mode`) | Ledger checkpoints skip (doctor still reports the witness). |
| `warn` | stderr verdict, never blocks (dogfood default). |
| `ask` | Prompt at each checkpoint; an explicit opt-out allows that checkpoint only. |
| `always` | Block production writes and `gh pr create` on missing or conflicted state. |

`ledger_mode` (enum `always|ask|warn`, default `warn`) wins whenever it is set.
When it is unset the legacy `gate_mode` maps forward: `off`→skip, `warn`→`warn`,
`always`→`always`. The config section is the **witness-bound** recipe
(`recipes.<witness recipe_id>.config`), so a non-legacy provider recipe drives the
checkpoint; the `trello-mcp-workflow` literal is only the bridge's fallback when no
witness resolves. The worktree gate mode is never read. One-shot env override:
`TRACKER_LEDGER_MODE` (and the legacy `TRACKER_CARD_GATE_MODE`). Production dirs
override: `TRACKER_CARD_GATE_PATHS` (default `lib catalog bin src`). The gate
**never** blocks `openspec/**` and **fails open** on a missing or unverified
binary, parse errors, or unavailable IO.

### The write surface

Items are opened, linked, closed, and exempted only by explicit writes — a parsed
`## Tracker` section never opens one, and grading never writes:

```bash
worktree-gate --ledger --checkpoint apply-start --ledger-mode warn \
  --project-root . --write '{"kind":"open"}'
worktree-gate --ledger --checkpoint apply-start --ledger-mode warn --project-root . \
  --write '{"kind":"link","item_id":"<24-hex>","url":"https://trello.com/c/...","native_type":"card","state":"in-progress","provider":{"list":"In Progress"}}'
worktree-gate --ledger --checkpoint archive-close --ledger-mode warn \
  --project-root . --write '{"kind":"close"}'
worktree-gate --ledger --checkpoint apply-start --ledger-mode warn \
  --project-root . --write '{"kind":"exempt","reason":"<first line of tracker.none>"}'
```

`--write` and `--decide` are mutually exclusive. Success adds
`"write":{"kind":…,"applied":…,"reason":…}` to the verdict; the idempotent no-ops
are `already-open`, `unchanged`, and `already-closed`. A failed write (invalid
payload, lock timeout, ambiguous identity, store IO) persists nothing, reports
`worktree-gate: ledger --write failed: …` on stderr, and exits `2` with no stdout
JSON. Grade paths keep failing open.

### Evidence sides

`tracker-card-gate.sh` and `tracker_ledger_host.py` pass a bridge-built `--evidence`
file (`lib/_internal/ledger_bridge.py`, acquisition only): `local` is the ledger's
own store snapshot, `code` is the change's `## Tracker` `card_id`, and `git` is that
same id when a `pr:` is recorded. The `--evidence` `remote` side stays **unwired**:
remote reconciliation is a separate, explicit `--reconcile` comparison documented in
the `trello-mcp-workflow` skill, and the grade path stays offline. A missing or
malformed artifact yields an empty side (fail open).

Where a host finds `openspec/changes/<slug>/tracker.none`, it treats it as evidence
only (blank `code` side) and grades; it never records the exemption itself (R1). The
file is the human act and is never created, modified, or deleted by a host. Durable
recording is the explicit human/agent `--write '{"kind":"exempt","reason":"<one line>"}'`;
removing the file does not revoke a recorded exemption.

### Witness, store, and activation

The tracker recipe supplies configuration and provider-private values; it is not
the grader. Activation is proven by the durable binding witness `ai-specs sync`
writes at `<git-common-dir>/ai-specs/ledger/witness.json` (`bound` / `ambiguous` /
`unbound` / `declared-not-bound`). Only `bound` activates the ledger; a missing or
unreadable witness is dormant (`witness-missing`) and no provider is ever guessed.
The per-identity record lives at
`<git-common-dir>/ai-specs/ledger/state.json`, written atomically; a branch reused
after its item closed opens a new item.

Dormancy is visible through **`doctor` only** — the `tracker-ledger` check renders
`unbound` (INFO), `ambiguous` / `declared-not-bound` / missing witness / recorded
conflict (WARN), infrastructure failure (ERROR), plus an INFO when a bound witness has
no `plan-build-flow` recipe enabled (`work-start is unhosted`; the other four
checkpoints keep grading). The runtime brief gains no
per-project dormancy line. The first slice records and reconciles evidence; it
performs **no** Trello MCP/API create, update, move, comment, or label call —
`always` blocks until an item is supplied, it does not create one. Provider item
vocabulary (board, lists, labels, card type) stays in `[config.*]` and out of the
ledger core item fields and the `## Tracker` contract.

Dual hooks share one script: `tracker-card-gate` (Edit/Write/…) and
`tracker-card-gate-shell` (Bash/Shell/…).

## Residual platform gaps

- **Cursor**: no pre-file-write hook — file-write matcher is skipped; shell id
  registers as `beforeShellExecution`.
- **OpenCode**: primary-agent pre-tool-use only — not subagent/MCP tool calls.
- **pi / omp**: this-process only — child processes are not covered.
- **MCP interception**: explicitly **not** implemented. Do not claim the gate
  prevents Trello MCP misuse; brief + skill anti-bypass cover that surface.

## Live evals (manual / nightly)

```bash
EVALS_LIVE=1 ./tests/evals/run-live-trello.sh
```

Not wired into `./tests/validate.sh`. See `tests/evals/scenarios/trello-mcp-workflow/`.

## Ceremony vocabulary note

Ceremony/depth classification lives in `plan-build-flow` (depth tiers `Light` /
`Standard` / `Full`). The legacy `trivial` / `local_fix` / `behavior_change` /
`domain_change` vocabulary is retired; see the `plan-build-flow` spec for the
retirement and migration mapping.
