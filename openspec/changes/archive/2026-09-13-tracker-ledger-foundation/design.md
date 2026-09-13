# Design: Tracker-only Go Ledger foundation

One new `--ledger` mode on the existing `worktree-gate` binary grades tracker
lifecycle. Python writes a durable binding witness and consumes JSON. No second
binary, no Python grader, no MCP writes.

Proposal source: `proposal.md` rev 2 (D1–D19 closed). Explore: `explore.md`.
Specs were authorized in parallel and are not an input; tasks must reconcile
any later spec naming with the contracts below.

## Quick path

1. Sync persists a tracker witness under the Git common-dir; Go only reads it.
2. Hosts call `worktree-gate --ledger --checkpoint <name>` and honor the JSON
   verdict (`0` allow / `2` block).
3. Human conflict/opt-out answers are persisted in the same ledger store.
4. `./tests/run.sh` already runs `go test` on this module; add a ledger corpus
   and host-parity tests in the same change.

## Decisions (A1–A11)

Product locks D1–D19 are not reopened. These close the vehicle questions.

| ID | Decision |
|---|---|
| A1 | Keep the module at `catalog/recipes/worktree-flow/gate`. Add package `ledger` plus a `package main` dispatcher. Do not relocate the module or add a second asset. Tracker types must not import worktree `Decide` / `Event` / cleanup types; `main` only switches on `--ledger`. Shared Git facts stay in `gitfacts.go`. |
| A2 | Identity is `realpath(git-common-dir)` + `symbolic-ref --quiet --short HEAD` + optional change slug. Detached HEAD, unborn branch, or empty common-dir → `identity_unavailable` (never heuristic). Same branch in two worktrees is one identity (D2). A rename is a new identity. |
| A3 | Ledger store is `<git-common-dir>/ai-specs/ledger/state.json`. Not `openspec/**`, not a committed file, not the realpath project cache. Common-dir is shared across worktrees and is already untracked. |
| A4 | Witness is `<git-common-dir>/ai-specs/ledger/witness.json`, written by sync after `resolve_bindings`, atomic `mkstemp` + `os.replace`. Go never re-derives catalog bindings. |
| A5 | Store is a JSON object with `items[]`. Primary = the single `open` item for the identity. Closed items stay but are never reopened (D17). Collision of two `open` items → human adjudication. No compaction in this slice; advisory ceiling 32 items or 64 KiB. |
| A6 | Current conflict lives on the item (`conflict` snapshot) plus append-only `decisions[]`. Hosts print the evidence from the verdict JSON; the human answer is written back with `--decide`. |
| A7 | Core item fields are provider-neutral. Recipe config and an opaque `provider` object stay in `recipe.toml`. No Trello field is promoted. |
| A8 | Ledger is the only grader. `## Tracker` / `tracker.none` are presentation and authoring. `trello_link.py` stays a parser. Legacy validity copies delegate or fail a parity test. |
| A9 | One project `ledger_mode`: `always \| ask \| warn`, default `warn` (D18). Tracker `gate_mode` maps `off→skip checkpoints`, `warn→warn`, `always→always`. `ask` is only via `ledger_mode`. Worktree `gate_mode` is never read. Opt-out is checkpoint-scoped (D19). |
| A10 | Dormancy is `doctor` only (D15). Severities: unbound INFO, ambiguous WARN, declared-not-bound WARN, missing witness while a tracker recipe is enabled WARN, infra unevaluable ERROR. |
| A11 | Change slug is optional enrichment. Exactly one resolvable active folder adds it; several active folders omit it and flag collision. Archive uses the `change_dir` order (active → latest dated archive → legacy undated). Slug does not change when the folder is archived. |

### Deferred (do not implement)

- Generic / multi-capability ledger (D1)
- Worktree-specific identity or ledger (D2)
- Historical archive migration or rewrite (D13)
- Provider vocabulary or `## Tracker` parser/schema changes (D11, D14)
- MCP/API create/update/move/comment/label (D11)
- Module relocation to a neutral `go/` path (A1 judged; not needed)
- Compaction / retention enforcement (advisory ceiling only)
- Broad Python→Go migration beyond this seam (D5)

## Placement (A1)

```text
catalog/recipes/worktree-flow/gate/
  main.go              --ledger switch (cleanup precedent)
  ledger_cmd.go        package main: flags, git facts, JSON, exit 0/2
  gitfacts.go          reused; no tracker types
  ledger/
    identity.go        common-dir + branch + optional change
    witness.go         read-only witness decode
    store.go           atomic state.json
    verdict.go         checkpoint × mode predicate
    decide.go          persist human decision / opt-out
    *_test.go          table-driven stdlib tests
```

Trust path unchanged: `SHA256SUMS`, `gate_binary.py`, version-keyed cache,
`scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`,
`.github/workflows/release-worktree-gate.yml`. The digest file updates because
the same four-arch asset changes, not because a second asset appears.

`tests/run.sh` (`go -C catalog/recipes/worktree-flow/gate test ./...`) and
`tests/validate.sh` (`gofmt -l catalog/recipes/worktree-flow/gate`) already
cover the new package. No runner path change unless the module moves (it does
not).

## Binding witness (A4)

`resolve_bindings` returns `capability → recipe` and **omits** ambiguous caps.
Sync must persist that map plus the conflict list from
`check_capability_conflicts`, then write the witness **outside**
`RESOLVED_CONFIG_TEMP` so `lib/sync.sh`'s EXIT trap cannot delete it.

```json
{
  "v": 1,
  "capability": "tracker",
  "state": "bound",
  "recipe_id": "trello-mcp-workflow",
  "candidates": [],
  "written_at": "2026-09-13T00:00:00Z"
}
```

| `state` | When |
|---|---|
| `bound` | `tracker` is in the resolved map |
| `ambiguous` | two or more enabled recipes declare `tracker` and no `[[bindings]]` |
| `unbound` | no enabled recipe declares `tracker` |
| `declared-not-bound` | `openspec/config.yaml` `tracking:` (or equivalent declaration) exists, but the capability is not bound |

Go treats missing/unreadable/unknown-`v` witness as dormant (`unbound` reason
`witness-missing`). It never guesses a provider.

Activation: `state == bound` only. Declaration is supply (D6).

## Ledger store (A3, A5, A6)

Path: `<git-common-dir>/ai-specs/ledger/state.json`.

```json
{
  "v": 1,
  "items": [
    {
      "id": "16-hex-sha256-of-identity-key-plus-opened-at",
      "identity": {
        "common_dir": "/abs/path/.git",
        "branch": "change/tracker-ledger-foundation",
        "change": "tracker-ledger-foundation"
      },
      "status": "open",
      "item_id": "",
      "provider_id": "trello-mcp-workflow",
      "native_type": "",
      "url": "",
      "state": "",
      "provider": {},
      "exemption": "",
      "conflict": null,
      "decisions": []
    }
  ]
}
```

Identity key (not stored as a filename):  
`common_dir + "\x1f" + branch + ["\x1f" + change]`.

Atomic write (both languages): create `state.json.tmp.*` in the same directory,
fsync, `os.replace` / `os.Rename`. Readers treat missing file as empty items.
Corrupt JSON is `unevaluable` (`reason=store-corrupt`), never a synthesized
item.

A closed item is never selected. New work on a reused branch opens a new item
(D17). Two `open` rows for one identity is a conflict, not a pick.

`decisions[]` entries:

```json
{
  "at": "RFC3339",
  "checkpoint": "pre-merge",
  "kind": "adjudicate" | "opt-out" | "open" | "close" | "link",
  "choice": "local|remote|code|git|exempt|continue",
  "note": ""
}
```

`kind=opt-out` applies only to that `checkpoint` (D19).

## Work identity (A2, A11)

| Input | Outcome |
|---|---|
| Common-dir + named branch, 0 or 1 active change | Derive identity; enrich slug when exactly one |
| Several active `openspec/changes/<slug>/` (not `archive/`) | Identity without change; `collision=change-ambiguous` |
| Change archived mid-item | Keep stored slug; resolve folder via `change_dir` |
| Detached HEAD / unborn / no common-dir | `identity_unavailable` |
| Branch renamed | New identity; old open item stays until human closes/links |

Change discovery is archive-aware (`tests/_change_paths.py::change_dir`):
active `changes/<slug>/`, else latest dated `archive/YYYY-MM-DD-<slug>/`, else
legacy `archive/<slug>/`. Implemented in Go; Python is not the grader.

Identity uses the **owner** repository (`git -C <cwd-or-flag> rev-parse`), not
the planning root. Guardian `--root` is the planning root and must also pass
`--project-root` / cwd of the owning repo.

## Evidence and human adjudication (A6, D16)

Four sides, all present in the verdict (empty string = unavailable, not a
winner):

| Side | Source this slice |
|---|---|
| local | ledger item snapshot |
| remote | synthetic fixture or host-supplied `--evidence` JSON (no MCP) |
| code | `## Tracker` / `tracker.none` parse (presentation, not a grade) |
| git | branch, optional PR id if the host supplies it |

Disagreement → `conflict` + `decision=ask` (or `block` in `always` at
pre-merge). The host prints the four sides and calls `--decide` with the
human choice. No default winner.

This slice does not create or update provider items. `always` "create/link
before work" means the verdict is `needs-item` / block until a human (or later
slice) supplies `item_id`; the binary does not call Trello.

## Provider adapter (A7)

| Layer | Owns |
|---|---|
| Ledger core | `item_id`, `provider_id` (recipe id), `native_type`, `url`, `state`, `exemption`, evidence refs |
| Opaque `provider` | anything else; unread by the predicate |
| `recipe.toml` | `board_id`, lists, isolation, `gate_mode`, `ledger_mode` |

Synthetic fixture: `tests/fixtures/recipes/test-tracker-ledger/` declaring
`tracker`, gated by `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES`. No live board.

## Mode and checkpoint posture (A9, D10, D18, D19)

One `ledger_mode` for the project. New recipe field
`[config.ledger_mode]` enum `always | ask | warn`, default `warn`.

Existing tracker `[config.gate_mode]` `off | warn | always` (this repo:
`warn`) maps as:

| `ledger_mode` set? | Effective mode |
|---|---|
| yes | that value |
| no, `gate_mode=off` | checkpoints skip; doctor still reports the witness |
| no, `gate_mode=warn` | `warn` |
| no, `gate_mode=always` | `always` |

Do not read `recipes.worktree-flow.config.gate_mode`. Do not change this
project's dogfood `warn`.

| Checkpoint | Host | `always` | `ask` | `warn` |
|---|---|---|---|---|
| `work-start` | `plan-build-gate.sh` | Block first production write (or SDD proposal start) until an open item exists or `tracker.none` is recorded | Prompt; opt-out allows this checkpoint only | Stderr verdict |
| `apply-start` | `tracker-card-gate.sh` `kind=path` | Block production path on missing/conflicted item | Prompt; opt-out allows | Stderr |
| `pr-review` | `tracker-card-gate.sh` `kind=shell` `pr_create` + `pr-create.md` | Block `gh pr create` on missing/conflicted | Prompt; opt-out allows | Stderr |
| `pre-merge` | `premerge_guardian.py` `--stage pre-merge` | Block missing, conflicted, or `identity_unavailable` | Prompt; opt-out allows merge | Stderr; never block |
| `archive-close` | `premerge_guardian.py` `--stage pre-archive` | Block conflicted or missing close decision | Prompt; opt-out allows | Stderr |

Work-start does not require a change folder (D9). `openspec/**` writes stay
non-blocking at path hosts. Guardian artifact/verify-evidence logic is
untouched; it only adds a ledger invoke.

Opt-out is stored as `decisions[]` for that checkpoint. The next checkpoint
prompts again (D19).

### Unevaluable / outage

| Cause | Non-pre-merge | Pre-merge `always` | Pre-merge `ask`/`warn` |
|---|---|---|---|
| Missing/unverified binary, acquire fail, `--selftest` fail | Fail-open; doctor ERROR | Fail-open; doctor ERROR (infra exemption — not a merge-eligibility change) | Fail-open; doctor ERROR |
| Witness missing / dormant | Skip ledger (inactive) | Skip ledger; doctor shows dormancy | Same |
| `identity_unavailable` | `warn`/`ask` report; `always` blocks production writes | Block | Report / prompt |
| Corrupt store / IO on state | Fail-open + doctor ERROR | Fail-open + doctor ERROR (infra) | Fail-open + doctor ERROR |
| Unsupported host / no `--ledger` on old binary | Fail-open one-line stderr (existing launcher) | Same | Same |

Flag-parse errors on `--ledger` verdict calls fail open (gate precedent).
`--decide` persist fails closed (exit `2`, cleanup precedent).

No new hook event.

## CLI and JSON contract

```text
worktree-gate --ledger
  --checkpoint work-start|apply-start|pr-review|pre-merge|archive-close
  --ledger-mode warn|ask|always
  --project-root PATH
  [--witness PATH] [--store PATH] [--evidence PATH]
  [--decide JSON]
  [--explain]
```

`--selftest` and `--version` stay global. `--selftest` still prints `ok` and
must keep compiling gate regexps **and** exercise ledger identity/verdict
invariants (in-process, no network).

Stdout JSON (always for `--ledger`; `--explain` is an alias):

```json
{
  "capability": "tracker",
  "active": true,
  "checkpoint": "pre-merge",
  "mode": "warn",
  "decision": "allow",
  "reason": "",
  "identity": {"common_dir": "", "branch": "", "change": null, "key": ""},
  "item": null,
  "conflict": null,
  "prompt": null,
  "doctor": {"severity": "OK", "name": "tracker-ledger", "message": ""}
}
```

`decision`: `allow` | `block` | `ask` | `dormant` | `unevaluable`.  
Exit `2` only when the host must stop (`block`, or failed `--decide`).  
`ask` exits `0` with `decision=ask` so the host can prompt; the host re-invokes
`--decide` then re-grades.

## Python bridge (D5)

| File | Role |
|---|---|
| `lib/_internal/gate_binary.py` | Acquire / verify / cache. No ledger predicate. |
| `lib/_internal/recipe-materialize.py` | After `resolve_bindings`, atomic-write witness to common-dir. |
| `lib/sync.sh` | Leave witness alone; keep deleting only `RESOLVED_CONFIG_TEMP`. |
| `plan-build-gate.sh` | Resolve binary; `--ledger --checkpoint work-start`; map exit. |
| `tracker-card-gate.sh` | Same for `apply-start` / `pr-review`; remove the heredoc grader. |
| `lib/_internal/premerge_guardian.py` | Both stages; tolerate cold `AI_SPECS_HOME`. |
| `lib/_internal/doctor.py` | Print `doctor` from JSON; drop `_check_tracker_card_link` as a grader. |
| `lib/_internal/trello_link.py` | Parse `## Tracker` only. |

Hosts never compile or download on the hot path. Missing binary: one stderr
line, exit `0`, doctor ERROR.

## File-level data flow

```text
ai-specs.toml + catalog
        │ resolve_bindings + check_capability_conflicts
        ▼
recipe-materialize.py ──atomic──► <common-dir>/ai-specs/ledger/witness.json
        │
        │  (RESOLVED_CONFIG_TEMP still dies on sync exit)
        ▼
checkpoint host ──gate_binary.py──► worktree-gate --ledger
        │                                 │
        │                                 ├─ read witness (activate or dormant)
        │                                 ├─ derive identity (gitfacts)
        │                                 ├─ read/write state.json (atomic)
        │                                 └─ stdout JSON + exit 0/2
        ▼
host prints prompt / blocks / allows
        │
        └─ optional --decide ──► persist decisions[] ──► re-grade
```

Doctor is a host with no write side-effect.

## Doctor (A10, D15)

Check name: `tracker-ledger`. No `AGENTS.md` / runtime-brief dormancy line.

| Witness / verdict | Severity | Guidance |
|---|---|---|
| bound, no conflict | OK | — |
| unbound | INFO | enable one tracker recipe or ignore |
| ambiguous | WARN | add `[[bindings]]` capability=`tracker` |
| declared-not-bound | WARN | enable the provider or remove the declaration |
| witness missing and a tracker-capable recipe is enabled | WARN | `ai-specs sync` |
| infra unevaluable | ERROR | sync / `--refresh-gates` |
| conflict recorded | WARN | adjudicate at the next checkpoint |

## Tests

Go: table-driven `ledger/*_test.go` with `t.TempDir()` stores. Corpus:
`tests/fixtures/tracker-ledger-corpus/*.json`  
`(identity + evidence + mode + checkpoint) → (decision, conflict, exit)`.

| Case | Expect |
|---|---|
| no witness | dormant / allow |
| bound + empty store + warn + work-start | allow + report |
| bound + empty store + always + apply-start | block `needs-item` |
| bound + open item, consistent evidence | allow |
| four-side disagreement, no decision | ask or block by mode |
| `--decide` persisted | next grade allow (same checkpoint) |
| opt-out `ask` at apply-start | allow apply; pre-merge asks again |
| closed item + same branch + new change | new open item (D17) |
| two open items same identity | conflict, no pick |
| detached HEAD + always + pre-merge | block `identity_unavailable` |
| detached HEAD + warn | allow + report |
| missing binary (host) | exit 0 + doctor ERROR |
| `openspec/**` path | never blocked by path hosts |
| worktree-gate corpus (existing) | unchanged |

Python parity: `tests/test_tracker_ledger_parity.py` drives
`dist/worktree-gate-current --ledger` like `test_worktree_gate_parity.py`.
Update `tests/test_tracker_card_gate_hook.py`, `tests/test_premerge_guardian.py`,
`tests/test_doctor_tracker_card.py` so they invoke the bridge, not a second
grader.

`./tests/run.sh` = Go unit tests (now including `ledger`) + unittest.  
`./tests/validate.sh` = `py_compile` + `bash -n` + `gofmt -l` on the same
module + `run.sh`. Release CI already `go vet` / `go test` this module.

## Rollout and rollback

Additive. Default `warn`. This repo stays `warn`.

1. Land mode + witness + hosts + corpus in one change (digest update is the
   same asset).
2. Promotion to `ask`/`always` is a human `ledger_mode` edit (D18).
3. Rollback: revert the PR. `--ledger` disappears; worktree gate and its
   digests remain a consistent older asset. Missing witness = dormant, never
   `bound`. No historical data to un-migrate.

## File change list (implementation)

| Path | Change |
|---|---|
| `catalog/recipes/worktree-flow/gate/ledger/*.go` | New package |
| `catalog/recipes/worktree-flow/gate/ledger_cmd.go` | Dispatcher |
| `catalog/recipes/worktree-flow/gate/main.go` | `--ledger` flag |
| `catalog/recipes/worktree-flow/bin/SHA256SUMS` | Same four assets, new digests |
| `lib/_internal/recipe-materialize.py` | Witness writer |
| `lib/sync.sh` | Do not trap-delete the witness |
| `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` | work-start invoke |
| `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | apply/pr invoke; drop heredoc grader |
| `lib/_internal/premerge_guardian.py` | both stages |
| `lib/_internal/doctor.py` | consume JSON |
| `lib/_internal/trello_link.py` | parser-only comments; no new predicate |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | `ledger_mode` |
| `catalog/recipes/git-pr-flow/commands/pr-create.md` | point at verdict |
| `tests/fixtures/recipes/test-tracker-ledger/` | synthetic provider |
| `tests/fixtures/tracker-ledger-corpus/` | pinned cases |
| `tests/test_tracker_ledger_parity.py` | seam parity |
| `docs/capabilities.md`, hook docs, `CHANGELOG.md` | contract text |

Unchanged on purpose: `gate_binary.py` API, worktree-gate event/decide path,
guardian tier/verify math, `openspec/config.yaml` `tracking:` as non-enforcing
declaration.

## Assumptions tasks must honor

1. Specs written in parallel may use different heading/ids; the CLI flag names,
   JSON keys, witness `state` enum, and store path in this file win until a
   human revises this design.
2. Native type is recipe-configured; core stays type-agnostic (proposal
   assumption 1).
3. Pre-merge block uses the existing guardian, not a new merge path
   (assumption 2).
4. Production-write definition stays the current path-kind set; `openspec/**`
   exempt (assumption 3).
5. Synthetic fixture is the proof surface (assumption 4). Delivery strategy
   and the 400-line budget are tasks-phase (assumption 5). D1–D19 stay closed
   (assumption 6).
6. Owner-root vs planning-root: identity and store follow Git common-dir of
   the owning repo; change slug is resolved from the planning root.
7. `always` does not perform provider writes in this slice; it only blocks
   until a human-supplied item id (or exemption) exists.
8. Do not add a hook event, a second Go module, a Python grader, or a
   committed ledger under `openspec/changes/**`.

## Tracker

- **card_id**: `6aa60e32aee4c220de4b1887`
- **url**: https://trello.com/c/0Tv0HZ6Q/125-epic-tracker-ledger-foundation

Context only; not a provider-vocabulary contract (proposal `## Tracker`).
