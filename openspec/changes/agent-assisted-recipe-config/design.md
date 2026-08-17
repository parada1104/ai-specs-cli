# Design: agent-assisted recipe configuration

## Problem restated

Close the loop from natural-language recipe-setup intent to a verified project
state, without requiring the user to already know topology, manifest layout,
recipe contracts, or sync semantics — while preserving existing config and
overrides and staying clear of sibling override-lock work (#63).

## Technical approach (outcomes-first)

Assisted configuration is a **five-phase agent workflow** where the
deterministic parts are owned by a **minimum non-interactive helper** and the
conversational parts stay in the literacy skill:

```text
NL intent
  → INSPECT   (helper: recipe-configure inspect --json)      [deterministic]
  → RECOMMEND (agent: reviewable proposal from the JSON)     [conversational]
  → APPLY     (helper: recipe-configure apply --set ...)     [deterministic]
  → SYNC      (helper --sync → ai-specs sync)                [deterministic]
  → REPORT    (helper: versioned JSON report)                [deterministic]
```

Rule of thumb for the split: **anything a test must assert byte-for-byte lives
in the helper; anything requiring judgement lives in the skill.**

Do **not** over-build: compose `resolve_repo_topology`, `recipe_schema`,
`update_recipe_config`, `cli_version`, `ai-specs sync`, and `ai-specs doctor`.
No new subsystems, no MCP wrapper, no changes to the interactive wizard.

## Defects reproduced during planning

Both reproduced against `lib/_internal/recipe-config-write.py` in this worktree
(read-only probe on a temp manifest, no production code changed):

```text
input:   integration_branch = "main"  # team decision 2026-01
         worktrees_dir='.worktrees'

apply {integration_branch: "development"}
output:  integration_branch = "development"        ← inline comment DESTROYED

apply {integration_branch: "main", worktrees_dir: ".worktrees"}   (semantic no-op)
output:  integration_branch = "main"
         worktrees_dir = ".worktrees"              ← reformatted; bytes CHANGED
         true no-op byte-identical: False
```

Convergent re-apply (same values twice in a row) *is* byte-identical today —
but that is a weaker property than the acceptance criterion. `test_replace_
existing_key` in `tests/test_recipe_config_write.py` asserts the **own-line**
comment survives and is silent about the inline one, which is why this shipped.

These two defects are the technical justification for the human's
"helper, not skill-only" decision.

## Architecture decisions

| Decision | Choice | Alternatives | Rationale |
|---|---|---|---|
| Delivery | Minimum non-interactive helper + skill playbook | Skill-only prose | Determinism, byte-identical no-op, comment preservation are code properties |
| Helper surface | `ai-specs recipe configure` (new subcommand in `lib/recipe.sh`) | Flags on `configure-recipes`; standalone binary | Sits beside `list/add/init/remove`; leaves the TTY wizard untouched |
| Helper responsibility | inspect \| apply \| (optional) sync \| report | Also own recommendation text | Recommendation needs judgement — keep it in the agent |
| Determinism | `--json` output is a pure function of project state | Free-form stdout | Enables unit tests without an LLM |
| Apply | Extend `update_recipe_config` (value-equality skip + inline-comment carry) | New writer; whole-file round-trip | One write path; the wizard inherits the fix |
| Doctor | **Parse, do not modify** | Add `doctor --json` | Shared surface; out of this change's blast radius |
| Sync | Helper invokes `ai-specs sync` behind `--sync`; never re-implements it | Agent invokes sync free-hand | Structured partial-failure reporting needs the exit path in one place |
| `cli_version` | Two distinct classes, handled at different phases | One "version gap" bucket | Pin violations abort sync *before writes*; lock staleness is informational |
| Topology | `resolve_repo_topology` + `config_schema` enum | Require `init.md` per recipe | `worktree-flow` ships no `init.md`; grounding must not depend on one |
| Evals | New client on the **unchanged** `tests/evals/` system | New eval framework; browser automation | Scenario contract, fixtures, assertions, isolation stay canonical; every runtime is already wired |
| Cross-runtime execution | Optional orchestration skill (Orca/OMP) that only invokes the existing runners and aggregates | Bake multi-runtime logic into the harness; mandate one runtime | Keeps eval semantics untouched while making prompt/skill portability observable |
| Overrides | Preserve; report suspected drift only | Force-refresh managed overrides | #63 owns lock provenance |
| Secrets | Redact / `${env:VAR}` only | Raw tokens in transcripts | Existing recipe-init + MCP conventions |

---

## Gap resolutions

### G1 — Helper deterministic contract

**Surface**

```text
ai-specs recipe configure <recipe-id> [path] [options]

  --inspect                 emit grounding document; never writes
  --set KEY=VALUE           repeatable; desired config (implies apply)
  --dry-run                 compute the plan, write nothing
  --sync                    after a successful write, run `ai-specs sync <path>`
  --ignore-cli-version      forward the sync escape hatch; recorded in the report
  --json                    machine-readable output (deterministic)
```

Path defaults to the current directory, matching sibling `recipe` subcommands.
Without `--json` the helper prints a human summary of the same data; `--json`
is the contract under test.

**Determinism guarantees** (the testable definition):

1. Same project bytes + same argv ⇒ **byte-identical** stdout.
2. Object keys emitted in a fixed declared order; map-like collections sorted
   by key; arrays in a documented deterministic order (schema order for fields,
   sorted for detected paths).
3. **No** wall-clock timestamps, durations, PIDs, hostnames, temp paths, or
   absolute host paths in `--json`. Paths are project-root-relative.
4. `schema_version` is an integer that only increases; unknown-to-the-reader
   fields are additive.
5. Values are typed per `config_schema` (`bool`/`int`/`str`/enum), never
   stringly-typed reprs of booleans.
6. Secrets are never echoed: `${env:VAR}` passes through verbatim; a literal
   that matches a secret-shaped field is rejected, not printed.

**Exit codes** (stable, and the reason a shell playbook can branch on them):

| Code | Meaning | `status` in report |
|---|---|---|
| 0 | Applied, or nothing to do | `ok` / `no-op` |
| 1 | Apply or sync failed | `failed` / `partial` |
| 2 | Usage error | — (no report) |
| 3 | Rejected before touching anything (schema-invalid value, unknown key, unknown recipe, secret literal) | `rejected` |
| 4 | Refused before touching anything (preflight gate, e.g. `[tool]` pin violation) | `blocked` |

Codes 3 and 4 are distinct on purpose: 3 is "your request is wrong", 4 is "your
project is not in a state where this can succeed". Both guarantee **zero
writes**.

**`--inspect --json` document** (field order as listed):

```json
{
  "schema_version": 1,
  "recipe": {"id": "worktree-flow", "enabled": true, "present_in_manifest": true},
  "schema": {"fields": [{"key": "repo_topology", "type": "str", "required": false,
                         "enum": ["auto","standalone","monorepo-apps","monorepo-submodules"],
                         "default": "auto", "help_text": "..."}]},
  "current_config": {"integration_branch": "main"},
  "grounding": {
    "topology": {"resolved": "monorepo-submodules", "configured": "auto", "via": "auto",
                 "submodules": ["libs/core"], "gitmodules_present": true},
    "mcp": {"required": false, "present": []},
    "cli_deps": [{"name": "gh", "present": true}]
  },
  "preflight": {"cli_version": {"installed": "1.4.0", "pin": null, "pin_kind": null,
                                "policy_ok": true, "lock_cli_version": "1.3.0",
                                "lock_state": "stale"}},
  "unknown_keys": []
}
```

`grounding.topology` is omitted (not faked) when the recipe declares no
topology field. `unknown_keys` lists manifest keys absent from the schema —
surfaced, never deleted.

### G2 — Topology grounding for `worktree-flow` without `init.md`

`catalog/recipes/worktree-flow/` ships `README.md`, `commands`, `hooks`,
`recipe.toml`, `skills`, `templates` — and **no `init.md`**. Only
`playwright-mcp`, `playwright-ui-flow`, and `trello-mcp-workflow` ship one.
Grounding therefore derives from three sources that always exist:

1. **Schema** — `[config.repo_topology]` in `recipe.toml`: enum
   `auto|standalone|monorepo-apps|monorepo-submodules`, plus `help_text` that
   already explains each value. The helper echoes `enum` + `help_text` so the
   agent can explain the choice without inventing semantics.
2. **Detection** — `util.resolve_repo_topology(repo_root, config_value)` →
   `TopologyResolution(resolved, configured, via, submodules,
   gitmodules_present)`, built on `detect_submodules` (`.gitmodules` +
   `git submodule status`, `-` prefix skipped).
3. **Current config** — the configured value, so `auto` vs an explicit pin is
   visible.

**The `monorepo-apps` blind spot.** `resolve_repo_topology` documents that
`auto` **never** resolves to `monorepo-apps`; with no `.gitmodules` it returns
`standalone`. An apps-style monorepo therefore *looks* standalone to detection.
The flow must not silently accept that: when `gitmodules_present` is false and
the recipe declares a topology field, the recommendation SHALL present
`monorepo-apps` as an explicit user question rather than asserting
`standalone`. (Mechanically the two are identical for worktree-flow —
`monorepo-apps` is naming-only per its `help_text` — so this is a labelling
question, and saying so is part of the honest recommendation.)

Git failures degrade to `standalone` without raising; the helper marks that
case `via: "auto"` with `gitmodules_present: false` and the agent states the
degradation as an assumption instead of a fact.

`init.md`, when present (`trello-mcp-workflow`), is consumed as **additional**
Q&A material — never as a precondition.

### G3 — Inline comment preservation

Cause: on replacement, `update_recipe_config` rebuilds the whole line as
`f"{indent}{key} = {value}\n"`, discarding everything after the value.

Fix: split the existing line into `value_part` and `trailing_comment` and
re-emit the comment. The splitter MUST be TOML-string-aware — a `#` inside a
basic (`"…"`), literal (`'…'`), or multi-line string is **not** a comment:

```text
url = "https://x/#frag"   # real comment
      └────value────────┘ └──carried──┘
```

Rules:

- Comment carried verbatim, including the whitespace run that preceded `#`.
- Key not previously present ⇒ no comment invented.
- Value spans multiple lines (array/inline-table across lines) ⇒ the helper
  **refuses** the key (exit 3, `rejected`, reason `multiline-value`) rather
  than guessing. Scalars and single-line arrays cover every field in
  `config_schema` today; the refusal is the honest boundary.

This lands in the shared writer, so `config_wizard.py` inherits it. Its
observable behavior is otherwise unchanged, and `tests/test_config_wizard.py`
locks that.

### G4 — Byte-identical no-op apply

Two independent changes; both are needed:

1. **Value-equality skip.** Before writing, parse the current manifest
   (`tomllib`) and drop from the write set every key whose current *parsed*
   value already equals the desired value. Skipped keys keep their original
   bytes: spacing, quote style, and inline comment. This is what makes
   `worktrees_dir='.worktrees'` survive an apply that asks for `".worktrees"`.
2. **Short-circuit write.** If the assembled text equals the original text,
   return without calling `write_text` — no mtime churn, no lock/watcher noise.

Contract, and the exact wording the test asserts: *applying a recommendation
whose values already match the effective config leaves the manifest
byte-identical and reports `status: "no-op"` with `applied.changed: []`.*

Equality is on **parsed** values, so `1` vs `1.0` and `true` vs `"true"` are
correctly unequal (the latter is also a schema type error → exit 3).

### G5 — Sync partial-failure semantics

Ground truth from `lib/sync.sh`:

- Steps run sequentially: `.gitignore` → root gitignore block → bundled skills
  + commands → vendored skills → recipe materialize → `AGENTS.md` → per-target
  `sync-agent.sh`.
- Target loop failure prints `"Stopped on first failure; previous writes are
  not rolled back."` and exits 1.
- `cli_version.py stamp-meta` runs **only after every step succeeds** — a
  partial sync leaves `.ai-specs.lock` `[meta].cli_version` **unstamped**.

Therefore a failed sync after a successful manifest write is a genuine
**partial** state, not a failure to be retried blindly. Report contract:

| Situation | `status` | Exit |
|---|---|---|
| Write happened, sync succeeded | `ok` | 0 |
| Nothing to write, no sync requested | `no-op` | 0 |
| Write happened, sync failed | **`partial`** | 1 |
| Write refused by preflight | `blocked` | 4 |
| Write rejected by validation | `rejected` | 3 |

`partial` MUST carry `sync.failed_step`, `sync.exit_code`,
`sync.rolled_back: false`, `sync.lock_stamped: false`, and a remediation line.
The agent SHALL NOT describe a `partial` outcome as configured, synced, or
done. `--dry-run` never reaches sync.

### G6 — Doctor consumption and the structured report

`doctor.py` is line-oriented (`SEV  name  message  (guidance)`) plus a final
`Summary: N OK, N INFO, N WARN, N ERROR`; exit is 1 only when an ERROR exists —
**WARNs do not fail it**. There is no JSON mode, and adding one is out of scope.

The helper therefore:

- runs `ai-specs doctor <path>`, records `doctor_exit` verbatim;
- parses the last `Summary:` line into `ok/info/warn/error`;
- sets `parsed: false` and leaves the counts `null` when the line is absent or
  malformed — **never** silently reports zero warnings;
- lifts the `cli-version` check line, when present, into `gaps`.

Report schema (`report_version: 1`, deterministic field order):

```json
{
  "report_version": 1,
  "status": "ok|no-op|partial|failed|blocked|rejected",
  "recipe": "worktree-flow",
  "applied": {
    "changed":   [{"key": "repo_topology", "from": "auto", "to": "monorepo-submodules"}],
    "unchanged": ["integration_branch"],
    "preserved": ["worktrees_dir", "custom_key"]
  },
  "preflight": {"cli_version": {"installed": "1.4.0", "pin": null, "policy_ok": true,
                                "lock_cli_version": "1.3.0", "lock_state": "stale"},
                "blocked_reason": null, "ignore_cli_version": false},
  "sync":   {"ran": true, "exit_code": 0, "failed_step": null,
             "rolled_back": false, "lock_stamped": true},
  "verify": {"doctor_exit": 0, "parsed": true, "ok": 12, "info": 0, "warn": 1, "error": 0},
  "assumptions": ["topology detected via auto; monorepo-apps not distinguishable by detection"],
  "drift":       ["override ai-specs/recipes/worktree-flow/overrides/x.md differs from catalog"],
  "gaps":        ["lock cli_version 1.3.0 != installed 1.4.0 (next sync restamps)"]
}
```

`preserved` lists keys present in the manifest and untouched by this apply —
the machine-checkable form of "we did not clobber your config". Override drift
in `drift` is **report-only**; nothing under `overrides/` is written (#63).

### G7 — Pre-sync `cli_version` drift

Two classes, conflated in the previous draft, separated here:

| Class | Source | Effect on sync | Phase | Handling |
|---|---|---|---|---|
| **A — `[tool]` pin violation** | `parse_tool_policy` + `check_policy` (`exact`/`min`) | `cli_version.py check-sync` returns 1 → **`sync.sh` exits 1 before any write** | **Preflight, before apply** | Refuse: exit 4, `status: "blocked"`, zero writes |
| **B — lock staleness** | `.ai-specs.lock` `[meta].cli_version` != installed | none; sync runs and `stamp-meta` restamps | Report | Informational entry in `gaps` |

Class A must be checked **before** the manifest write. If it were checked
after, the flow would leave an edited manifest that `sync` refuses to process —
exactly the half-configured state this capability exists to prevent. This is a
hard ordering requirement, not an optimization.

`--ignore-cli-version` is the explicit escape hatch: it downgrades class A from
blocking to a recorded override (`preflight.ignore_cli_version: true`), forwards
the flag to `sync`, and the closing report states the policy was bypassed.
`evaluate_cli_version` also returns ERROR for a malformed `[tool]` policy — that
is class A as well (blocked, exit 4).

---

## Runtime evidence: a new client on the existing eval system

### Why this is a separate evidence tier

Unit tests prove the helper is deterministic. They cannot prove that an agent,
given a natural-language sentence, actually reaches for the helper, waits for
approval, and reports honestly. That is a runtime property of the
skill + runtime combination, and only a real runtime can show it.

| | Unit tests | Runtime evals |
|---|---|---|
| Location | `tests/test_*.py` | `tests/evals/eval_*_live.py` |
| Discovery | `./tests/run.sh` (`-p 'test_*.py'`) | excluded by the `eval_*` naming |
| Needs an LLM | No | Yes (`EVALS_LIVE=1`) |
| Deterministic | Yes | No — N-of-M trials |
| Gates the PR | **Yes** | No (nightly/pre-release) |
| Proves | Helper contract, write path, report schema | NL entry, approval gate, honest reporting |

Neither substitutes for the other. Both are required as acceptance evidence for
this change; only the first is a merge gate.

### The eval system does not change

`tests/evals/` is canonical and stays that way. This change adds a **client**
— the same shape every other capability already has — and changes nothing
about how evals are defined, isolated, or judged:

| Element | Status |
|---|---|
| Scenario contract (`scenario.toml` globs, `required_content`, `forbidden_phrases`, `fixture`, `mode`) | **Unchanged** |
| Fixture model (tempdir project, `materialize_project`, `init_git_repo`, baseline commit) | **Unchanged** |
| Assertions and pass criteria (incl. N-of-M via `EVALS_TRIALS`) | **Unchanged** |
| Isolation guarantees | **Unchanged** |
| Runner shape + `EVALS_*` env contract | **Unchanged** |
| Runtime support (`claude`, `cursor-agent`, `opencode`, `pi`, `omp`) | **Unchanged** — all already wired |
| `tests/evals/lib/project_fixture.py` | One **strictly additive** function (below) |

`assert_natural_prompt` still rejects meta-prompts (`/plan`, "haz un plan"), so
eval prompts stay real user sentences. No browser, no UI automation: every
runtime is a CLI process.

**One fixture gap to close.** `setup_runtime_skills` resolves skills from
`catalog/recipes/<id>/skills/` (or the materialized `.recipe` dir). The assisted
flow ships in `bundled-skills/harness-recipes`, which that resolver cannot
reach. Add a sibling `setup_bundled_skills(root, runtime, names)` that copies
`bundled-skills/<name>/SKILL.md` into `RUNTIME_SKILL_DIRS[runtime]`
(`.omp/skills`, `.pi/skills`, …). Purely additive: no existing caller, no
existing behavior, and no existing scenario is affected.

### Optional orchestration layer (Orca/OMP)

**What it is.** Orca/OMP is the IDE/harness the maintainer works in. An
orchestration skill installed there lets a **parent agent** drive the existing
eval runners across several real runtimes — `cursor-agent`, Claude Code, a
separate OMP session — and collate the results.

**What it is not.** It is not a runtime (the runtimes are the agent CLIs the
harness already supports), not an eval runner (the runner is still
`run-live-assisted-configure.sh`), not a scoring service, and not a
dependency. Nothing about it is required to execute or to pass the evals.

**Boundary — the layer only invokes and aggregates:**

| Allowed | Prohibited |
|---|---|
| Invoke `./tests/evals/run-live-*.sh` with different `EVALS_RUNTIMES` / `EVALS_SCENARIOS` / `EVALS_TRIALS` | Modify scenarios, prompts, fixtures, or assertions |
| Run per-runtime invocations in parallel and fan results back in | Relax, reinterpret, or re-judge a pass/fail outcome |
| Build a comparison matrix and summarize divergence | Alter isolation, N-of-M, or exit criteria |
| Record provenance per run | Become a precondition for running evals |

**Concrete advantage** (why bother at all): the artifact under test here is a
*prompt-and-skill* contract, and prompt-and-skill contracts are not portable by
default. A playbook that reliably makes Claude Code stop before apply can fail
the same approval gate on `cursor-agent` — different tool-calling shape,
different default eagerness. Running the same scenario across runtimes turns
that into observable data:

```text
scenario                             claude    cursor-agent   omp(session B)
ac_recommend_stops_before_apply      pass      FAIL           pass
  → the approval gate is runtime-sensitive, not universally satisfied
```

Multiagent coordination is the second gain: the runs are independent, so a
parent can fan them out concurrently and aggregate, instead of the maintainer
serially babysitting five scenarios × three runtimes.

**Degradation contract.** With no orchestration skill installed, the operator
runs `EVALS_RUNTIMES=... ./tests/evals/run-live-assisted-configure.sh` from a
plain shell and gets exactly the same scenarios, assertions, and verdicts. The
layer is a convenience for breadth and speed, never a semantic participant.

### Client layout

```text
tests/evals/eval_assisted_configure_live.py     # unittest module (live)
tests/evals/run-live-assisted-configure.sh      # opt-in runner, this client only
tests/evals/scenarios/assisted-configure/<scenario>/{scenario.toml,prompt.txt}
```

Named `assisted-configure` because this is a capability, not a catalog recipe;
scenario folders stay one level under `scenarios/`, matching the existing
per-client convention. The runner keeps the existing `EVALS_PREFER` ordering —
no runtime is privileged or required.

### Scenarios (5)

| Scenario | Fixture | Asserts |
|---|---|---|
| `ac_recommend_stops_before_apply` | `worktree-flow`, submodules | Recommendation produced; `ai-specs/ai-specs.toml` **unchanged** (git-clean) without approval |
| `ac_topology_grounded_without_initmd` | `worktree-flow`, submodules, no `init.md` | Recommendation cites detected topology + submodule evidence; no hardcoded consumer paths |
| `ac_apply_sync_verify_report` | `plan-build-flow` | Approval in-turn → config written, sync ran, report has all schema fields |
| `ac_noop_reapply_preserves_bytes` | `plan-build-flow`, pre-configured | Second identical request → manifest byte-identical; agent says no-op, not "configured" |
| `ac_blocked_cli_version_pin` | `[tool]` pinned to an unsatisfiable version | Flow blocks **before** writing; manifest unchanged; report explains the pin |

`ac_noop_reapply_preserves_bytes` and `ac_blocked_cli_version_pin` assert a
**byte hash** of `ai-specs/ai-specs.toml` captured before the run — the
strongest available form of "did not touch it".

These are the scenarios whose cross-runtime spread is most informative: the
approval gate, the honesty of a no-op, and the refusal to write when blocked
are exactly the behaviors that vary by runtime temperament.

### Isolation and blast radius

Inherited from the existing harness, not redefined here:

1. Every scenario runs in `tempfile.TemporaryDirectory()` (OS temp), created and
   cleaned per test — **never** inside the repo or `.worktrees/`.
2. `init_git_repo` + a baseline commit; assertions read
   `git status --porcelain` inside the fixture.
3. `forbidden_path_globs` bars writes to fixture paths that must stay put;
   `absent_path_globs` bars creation of anything the flow must not create.
4. Runtime process cwd is the fixture. Where a runtime is launched with a
   permissive approval mode (as the harness already does for `omp`), that is
   safe **because** the cwd is a throwaway temp project; it is never pointed at
   the CLI worktree.
5. Sync inside a scenario targets the fixture project only.
6. Before and after a live run, the operator follows
   `dogfood-verification-isolation`: `git status --short` in the worktree must
   be clean, and `git diff -- AGENTS.md` must be empty. Any CLI-generated
   project state in the worktree is reverted, never committed.
7. Orchestration, when used, inherits all of the above unchanged — it spawns
   runner processes and never touches fixtures directly.

### Reproducible evidence

Each recorded run captures: scenario id, runtime id, model id, trial index,
`EVALS_*` env, CLI `VERSION`, worktree commit SHA, exit code / `timed_out`,
the helper's `--json` report from inside the fixture, and the assertion
outcome. When several runtimes are exercised, evidence is recorded **per
runtime** so a divergence is attributable rather than averaged away.

Evidence is transcribed into the change's `verify-report.md` during the verify
phase — the fixture itself is disposable, so the report is the artifact.
Flakiness is handled by the harness's existing N-of-M rule (`EVALS_TRIALS=3` →
2 of 3), not by re-running until green, and not by the orchestration layer.

---

## Sync vs init-contract reconciliation

| Flow | Mutate manifest? | Invoke sync? |
|---|---|---|
| `ai-specs recipe init` / propose-only `init.md` | No — stays read-only | No |
| `ai-specs recipe configure` (this capability) | Yes, after recommendation + approval | Only with `--sync` |
| `ai-specs configure-recipes` (TTY wizard) | Yes, as today | Unchanged |

Spec strategy is **add-only**: new assisted-configure requirements.
`recipe-init-contract`'s propose-only posture is not weakened or reworded.

## Boundary with #63 (override ownership)

May: detect and **report** that an override differs from catalog; instruct the
agent not to clobber overrides.

Must not: extend `.ai-specs.lock` with per-override hashes; force-update
user-modified overrides; define per-artifact governance categories for
hooks/templates.

## Validation approach

- **Unit (merge gate)** — helper determinism (byte-identical `--json` across
  repeated invocations), exit-code table, report schema, inline-comment carry,
  `#`-inside-string safety, byte-identical no-op, preserve-unmentioned-keys,
  preflight class-A block ordering (no write), sync partial-failure mapping,
  doctor `parsed: false` fallback, wizard regression.
- **Skill content** — playbook names commands that exist; mentions approval
  gate, sync, preserve, no-secret-literals, report fields.
- **Runtime evals (evidence, not gate)** — the 5 scenarios above across at
  least two runtimes, so a runtime-specific failure of the approval gate is
  visible. Runtime selection stays `EVALS_RUNTIMES`/`EVALS_PREFER`; none is
  mandated. Orchestration, if used, only fans these runs out.
- **Suites** — `./tests/run.sh` and `./tests/validate.sh` before commit/PR.
- No live consumer checkout in CI; submodule topology comes from
  `add_initialized_submodule` fixtures.

## File-level sketch (indicative — apply confirms after RED)

- `lib/_internal/recipe-config-write.py` — inline-comment carry + no-op skip
- `lib/_internal/recipe-configure.py` — new helper
- `lib/recipe.sh` — register `configure`, extend usage
- `bundled-skills/harness-recipes/SKILL.md` (+ `harness-lifecycle` cross-link)
- `tests/test_recipe_config_write.py`, `tests/test_config_wizard.py` — regression
- `tests/test_recipe_configure.py` — helper contract
- `tests/evals/lib/project_fixture.py` — `setup_bundled_skills`
- `tests/evals/eval_assisted_configure_live.py`, `run-live-assisted-configure.sh`,
  `scenarios/assisted-configure/**`, `tests/evals/README.md`
- Docs pointer (`docs/recipes-catalog.md` and/or `docs/ai/troubleshooting.md`)
- Spec delta promoted into `openspec/specs/` on apply

Avoid editing override-lock / materialize force-update paths reserved for #63.

## Risks

| Risk | Mitigation |
|---|---|
| Agents skip verify | Helper exit codes + skill checklist + eval `ac_apply_sync_verify_report` |
| Shared writer change regresses the wizard | Wizard regression tests in the same RED phase |
| Comment splitter mis-parses `#` in strings | TOML-string-aware splitter + explicit test; refuse multi-line values |
| `partial` reported as success | Distinct status + exit 1 + eval asserts the agent's wording |
| Eval flakiness treated as failure | Existing N-of-M rule; evals are evidence, not a merge gate |
| Live eval mutates the worktree | Temp-dir fixtures + `dogfood-verification-isolation` pre/post check; orchestration spawns runners only |
| Orchestration drifts into eval semantics | Explicit allowed/prohibited boundary + degradation contract: identical verdicts from a plain shell |
| Broader MVP inflates the diff | Chained PRs (see `tasks.md` forecast) |
| Scope creep into #63 | Hard non-goal; override handling is report-only |
