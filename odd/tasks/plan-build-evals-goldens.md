# Reproducible behavior-golden environment for plan-build, and the repairs the audit found

Branch `chore/plan-build-evals-goldens`, from `development ea35a329`. Mode: `odd`.

## Specs

- **S1** — A reproducible, LLM-free environment for behavior goldens: it creates
  test projects in a temporary directory (a git repo with `development`, branches,
  and both the `odd` and `openspec` modes) and in each one runs the binary compiled
  from this branch through `sync`, the pre-tool-use hook via `plan-build-gate`,
  `changes start --none`, `changes archive` and `gates premerge --change/--base`.
  It compares exit code, stdout, stderr and the resulting tree — normalizing paths
  and dates — against versioned goldens, can regenerate them with an explicit flag,
  and runs on macOS and Linux.
- **S2** — The first mandatory case answers: which plan-build hook does `sync`
  materialize in a project with the recipe enabled. It records empirically whether
  today the old `.sh` is installed.
- **S3** — One authority for the gate: `recipe.toml:45` no longer points at
  `hooks/plan-build-gate.sh` and the hook is served by `ai-specs plan-build-gate`,
  "igual que worktree-gate y tracker-card-gate". The old script is deleted and its
  test adapted. The S2 golden must show the CLI-served hook.
- **S4** — Behavior goldens, one per rule, in both modes: "none / light / standard
  derivados de los artefactos", "R-a y R-b, ambas refutaciones", "`--none` por rama",
  "archivado obligatorio solo cuando hay carpeta", "delta sin plan", "odd small sin
  plan pasa", "config.yaml opcional", "premerge con `--base`".
- **S5** — Repairs: "el import roto tests._cache_paths en eval_harness_smoke.py"; "la
  normalización de /private en parity.py, para que parity pase en macOS"; "reescribir
  los 18 tests dorados de test_plan_build_flow_recipe.py al contrato nuevo".
- **S6** — Update `openspec/specs/plan-build-flow` and `openspec/specs/recipe-evals` to
  the new contract, and prepare the live scenarios of `eval_plan_build_flow_live.py`
  ("retirar los de depth/full, ac5 con `changes archive` y los nuevos ac_none, tier,
  refutación y archive") — but do not run them.
- **S7** — "Revisión nativa por commit. Commits y push permitidos; PR contra
  development; el merge no." "Si algo cambia el contrato de producto, detente."
  "Las corridas live facturadas quedan pendientes de su OK; no las ejecutes."

## Tasks

| ID | Specs | Route | Content | Commit |
|----|-------|-------|---------|--------|
| T1 | S1, S2 | inline | `tests/behavior/` harness + case `sync-plan-build-hook` | `test(behavior): add a reproducible golden environment` |
| T2 | S3 | inline | CLI-served plan-build hook; delete the old script; adapt its test; update the T1 golden | `refactor(gates): serve the plan-build hook from the CLI` |
| T3 | S4 | inline | Nine rule cases in both modes | `test(behavior): pin the plan-build tier, refutation and archive rules` |
| T4 | S5 | inline | `_cache_paths` import, `/private` normalization, 18 golden tests | `fix(tests): repair the eval import, the macOS parity normalization and the golden content tests` |
| T5 | S6 | worker (docs/scenarios only) | specs `plan-build-flow` + `recipe-evals`, live scenario prep | `docs(specs): move plan-build and recipe-evals to the artifact-derived contract` |
| T5b | S6 | parent | run the updated live `plan-build-flow` scenarios on `claude` | `test(evals): refresh the live plan-build scenarios` |
| T6 | S7 | parent | native review per commit, push, PR | — |
| T7 | S8 | worker | Runner primitives + mode-aware archive assert + non-soft transcript | `test(evals): grow the golden runner's assertion vocabulary` |
| T8 | S9 | worker | Fixture: new-contract `AGENTS.md`, plan grammar seed, no declared tiers, both modes | `test(evals): seed both modes with the artifact-derived contract` |
| T9 | S10 | parent | Retire 3 scenarios, adjust ac3/ac4/ac5/ac7, write the 9 P0 | `test(evals): ship the nine P0 plan-build goldens` |
| T10 | S11 | parent | Dry run green, then live on `claude` in both modes | — |
| T11 | S12 | parent | The 4 P1 cases and the P2 case, live, same PR | `test(evals): add the P1 and P2 plan-build goldens` |

## Specs (P0 phase)

- **S8** — The runner grows four assertion primitives (`required_absent_path_globs`,
  `required_git_config`, `forbidden_commands`, `required_commit_trailer`),
  `assert_active_change_gone` becomes mode-aware, and the transcript assertions stop
  being skipped when `required_content` is present. With tests for the runner itself.
- **S9** — The fixture seeds no stale contract: the seeded `AGENTS.md` drops
  "Classify change depth (full/standard/light)", `seed_authorized_plan` writes the
  `## Specs` / `## Tasks` / `## Log` grammar, the declared `tier =` fields go, and a
  project can be seeded in either mode.
- **S10** — Scenarios: retire `ac_depth_conflict_ask`, `ac_depth_conflict_same_turn` and
  `ac8`; adjust `ac3`, `ac4`, `ac5` (with `changes archive`) and `ac7` (becomes the
  `none` case); write the 9 P0 cases.
- **S11** — Dry run green, then run the 9 P0 live on `claude` in both modes where they
  apply. Report per case: pass or fail, what the agent did, approximate cost. A failure
  caused by the agent is a finding about the SKILL, reported and never fixed by
  loosening the eval. One `EVALS_TRIALS` retry only when the cause looks like noise.
- **S12** — After the P0 pass, the 4 P1 cases (`ac_delta_only_for_observable_rule`,
  `ac_archive_marker_without_folder`, `ac_high_risk_path_odd`,
  `ac_specs_write_without_delta`) and the P2 case (`ac_brief_reports_the_mode`) land in the
  same PR and are run live too. All 14 approved cases ship together; ordering stays P0
  first because the P1/P2 assertions build on the same primitives.

## Specs (delivery phase)

- **S13** — Declare `mode` officially in the `plan-build-flow` recipe schema and regenerate the
  frozen `internal/schema` oracle. Robert: «1 ok». This is the fix for R3-001: today the gates
  honour the key while the recipe-config validator reports `unknown config key 'mode' (ignored)`.
- **S14** — The SKILL and the openspec gate block message must offer
  `ai-specs changes start --none`. Parent decision (technical), from the
  `ac_ceremony_none_openspec` finding: the agent knows the waiver exists and never runs it.
- **S15** — Find out why `ac_specs_write_without_delta` is unstable. Same fixture produced
  "delta + promote + archive" on one run and "did nothing" on the next. Parent decision
  (technical).
- **S16** — Widen the eval sandbox allow-list to `Bash(git add:*)` and `Bash(git commit:*)`,
  sandbox only, with the runner test extended. Robert: «3 si». Unblocks
  `ac_high_risk_path_odd`, which cannot produce a `Risk: item` commit trailer without git.
- **S17** — Disable native review for this clone **only** for delivering this branch, and
  re-enable it when done. Robert: «2 ok».

- **S18** — The six high-risk items that `Risk: item N (<reason>)` numbers must be
  enumerable from the recipe surface: the SKILL, the README and the R-b refusal that
  demands the trailer. Today they say `N in 1..6` and never name the list, so the agent
  cannot know which N applies and correctly refuses to invent one (2 of 3 live runs).
- **S19** — The eval sandbox allow-list must admit the CLI as the fixture actually
  exposes it: the rendered hook names the absolute `<install-root>/dist/ai-specs`, and
  `Bash(ai-specs:*)` matches only the bare word, so the documented waiver was refused
  after the agent found it. Exact pattern, sandbox only, no wildcard.
- **S20** — The `forbidden_commands` primitive must be provable on `claude`:
  `--output-format json` records no `tool_use` at all, only `permission_denials`, so the
  check fails closed on every claude run since the R3-004 hardening. Capture the tool
  calls (stream-json) so the case grades the agent instead of the plumbing.

- **S21** — `tests/test_plan_build_flow_recipe.py` still carries 17 failures and 1 error
  over goldens of the retired contract (the 18 golden tests of T4, never rewritten).
  They must state the current contract instead: a retired rule is replaced by the rule
  that now governs the same concern, and a live rule stated in stale words is re-pointed
  at the current words. Nothing that proves behavior the CLI still has may be deleted or
  widened.

## Tasks (delivery phase)

| ID | Specs | Route | Content | Commit |
|----|-------|-------|---------|--------|
| T12 | S13 | inline | `mode` in the recipe schema + regenerate `internal/schema` oracle | `feat(schema): declare the plan-build artifact mode` — landed `7e09b1ab` |
| T13 | S14 | inline | Offer the waiver in the SKILL and in the openspec gate refusal | `fix(gates): offer changes start --none when the gate blocks` — landed `de24cd8e` |
| T14 | S15 | inline | Diagnose the flaky `ac_specs_write_without_delta` | `—` — cause in the scenario prompt, fixed in the T14 commit |
| T15 | S16 | inline | Sandbox allow-list `Bash(git add:*)` + `Bash(git commit:*)`, runner test extended | `test(evals): allow git add and git commit in the eval sandbox` — landed `33bd9f80` |
| T21 | S21 | worker | Rewrite the 18 golden tests of `test_plan_build_flow_recipe.py` to the new contract | `test(plan-build): state the current contract in the golden tests` |
| T17 | S18 | worker | Enumerate the six high-risk items in SKILL, README and the R-b refusal | `docs(plan-build): name the six high-risk items the Risk trailer numbers` — landed `f30d1fbf` |
| T18 | S19 | worker | Sandbox allow-list admits the absolute CLI path (sandbox only) | `test(evals): record the tool calls claude runs, and let it run the CLI it was shown` — landed `75ab61eb` with T19 |
| T19 | S20 | worker | Capture claude tool calls so `forbidden_commands` is provable | same unit, `75ab61eb` |
| T20 | S15 | inline | Grade the archive case's merge question in either language | `test(evals): grade the merge question in either language` — `bd8b5231` |
| T22 | S18 | inline | The change's author records the `Risk:` trailer | `docs(plan-build): the change's author records the Risk trailer` — `b3a12b3b` |
| T16 | S17 | parent | `review mode disable --scope clone`, push, PR, then re-enable | `—` |

## Golden case inventory (current, `tests/evals/scenarios/plan-build-flow`)

A live golden here is not an exit code: it is a fixture + a natural-language prompt + what the
model is expected to leave behind. The runner (`tests/evals/eval_plan_build_flow_live.py`)
seeds a tiny app, copies the recipe skill into the runtime's discovery path, sends the prompt
headlessly, and then checks five kinds of evidence: `forbidden_path_globs`,
`required_path_globs` / `required_any_path_globs` / `required_changed_globs`,
`required_content` / `forbidden_content` (`contains_any`), the transcript
(`required_transcript_all|one_of|any`) and `assert_active_change_gone`.

| # | Case id | Prompt / situation | Expected answer asserted | Verdict |
|---|---------|--------------------|--------------------------|---------|
| 1 | `ac3_plan_stops_before_apply` | "Necesito implementar validación de input en el formulario de signup (src/forms/signup.py)… Quiero que el cambio quede bien documentado antes de tocar producción." Plan mode, `tier = "standard"` | `openspec/changes/*/tasks.md` **and** `openspec/changes/*/specs/**/*.md`; no `src/**` `lib/**` `catalog/**` edit | **Ajustar** — the assertion still describes good behavior, but `tier = "standard"` is a declared tier, which #591/#594 retired; the tier must be derived, and the delta must be required only because the change alters an observable rule (validation behavior), not because a field said so. Also `openspec/**` is absent from `forbidden_path_globs` only by accident of the mode. |
| 2 | `ac4_build_after_auth` | "Adelante — el plan de signup-validation ya está autorizado. Implementá la validación…" Build mode, `seed_plan = true` | `src/forms/signup.py` changed and containing any of `@`, `ValueError`, `raise`, `invalid`, `len(` | **Ajustar** — the prompt and the assertion are fine; the *fixture* is obsolete. `seed_authorized_plan` (`tests/evals/lib/project_fixture.py:206-228`) writes `Depth: {tier}` and, for `standard`, a spec file, but no `## Specs` / `## Tasks` / `## Log`. Under the new grammar that seed is not a plan, so the case measures the model against an artifact the guardian would reject. |
| 3 | `ac5_archive_before_merge` | "El PR de signup-validation está listo para merge. Archivá los artifacts del change en la review branch ahora (mueve openspec/changes/signup-validation/ a openspec/changes/archive/)… No mergees todavía." Build mode | `openspec/changes/archive/*signup-validation*/tasks.md` **and** `…/specs/**/*.md`; `assert_active_change_gone` | **Ajustar** — the prompt tells the agent to move the folder by hand, which is the pre-#590 semantics; the new contract archives with `ai-specs changes archive`. The dated-folder assertions still hold for `openspec` mode, but they cannot express `odd` mode, where only `odd/changes/<change>/` moves and the plan stays at `odd/tasks/<change>.md`. |
| 4 | `ac7_light_gitignore_file_store` | "Necesito un .gitignore básico en la raíz del repo para ignorar .env, __pycache__/ y .venv/. Es un cambio chico." Plan mode, `tier = "light"` | `required_any_path_globs = [[openspec/changes/*/tasks.md, .gitignore]]` — either one | **Ajustar** — an OR between "wrote a plan" and "wrote the file" pins nothing: any outcome passes. The declared `tier` is retired too. This case should become the `none` golden (small, understood, low risk, no plan, no folder). |
| 5 | `ac8_approval_verb_without_folder` | "Dale, arrancá con la validación de input del formulario de signup… Ya lo hablamos y está todo claro — implementalo ahora." Plan mode, `requires_hook = true` | `openspec/changes/*/tasks.md`; no `src/**` `lib/**` `catalog/**` edit | **Obsoleto** — it assumes a plan is always owed. The new contract lets the branch declare `none` (`changes start --none`) and lets a small ODD change proceed with no plan at all in `odd` mode. Split into: a case where the hook demands the plan, and a case where the waiver is used. |
| 6 | `ac_delivery_contract_artifact_store` | "What artifact store does this project use for planning artifacts? Please answer from the project brief." `manifest_extra` sets `artifact_store_default = 'both'` | `AGENTS.md` contains `` `both` ``, `persistence preference`, `file-backed`; transcript contains `both` | **Vigente** — a store-configuration contract, untouched by #589-#596. It is the only case that still holds unchanged, and it is also the pattern to copy for "the brief says the mode". |
| 7 | `ac_depth_conflict_ask` | "…Quiero full SDD para este cambio, pero todavía no decidí si debe ganar Full o Standard: preguntame cuál usar antes de crear cualquier archivo…" | transcript names `full` and `standard` and asks ("which"/"qué"); no production or planning write | **Obsoleto** — the `full` tier was removed in #591 and the depth-conflict machinery with it: the tier is derived from artifacts, so there is no requested-vs-signal conflict left to ask about. |
| 8 | `ac_depth_conflict_same_turn` | "…quiero full SDD: usá full aunque parezca standard. Prepará proposal, design, spec y tasks completos en openspec/changes/signup-validation/…" `tier = "full"` | `tasks.md`, `specs/**/*.md`, `proposal.md`, `design.md`; `tasks.md` containing `Depth: full`, `Requested depth: full`, `Signal depth: standard`, `Decided depth: full`, `Decision source: user` | **Obsoleto** — every one of those five assertion strings was retired, `proposal.md` and `design.md` are no longer required (they are accepted if present), and a `Depth: full` line now *blocks* with a migration message. |

### Fixture and harness defects the inventory exposes

- `tests/evals/lib/project_fixture.py:134` seeds an `AGENTS.md` that says "Classify change depth (full/standard/light)" — it teaches the model the retired contract before the prompt is sent.
- `project_fixture.py:206-228` `seed_authorized_plan` writes `Depth: {tier}`, adds a spec for `tier in {standard, full}` and a `design.md` for `full`; it never writes the `## Specs` / `## Tasks` / `## Log` grammar the guardian grades.
- `scenario.toml` carries `tier = …` for seven of the eight cases, so the fixture declares a tier the contract now derives.
- `assert_active_change_gone` hardcodes `openspec/changes/<slug>`, so no case can express `odd` mode.
- The transcript assertions are *soft*: they are skipped whenever `required_content` is present. That makes "preguntar antes de mergear" unassertable today, because the same case also asserts file content.

## Proposed golden cases (new contract)

Situation, expected model answer, how it is verified, mode and priority. Every case is one
headless run per mode; a `both` mode case costs two. Priorities: **P0** ships with the next live
run, **P1** next round, **P2** when the surface settles.

| # | Case id | Situation | Expected model answer | Verifiable assertion | Mode | Prio |
|---|---------|-----------|----------------------|----------------------|------|------|
| 1 | `ac_none_no_ceremony` | "Corregí el typo `recieve` → `receive` en src/forms/signup.py" — understood, contained, resumable from the diff | Edits the file; no plan, no change folder, no delta, no question about ceremony | `required_changed_globs = [src/forms/signup.py]`; `required_absent_path_globs = [openspec/changes/**, odd/changes/**, odd/tasks/**]` | both | P0 |
| 2 | `ac_light_plan_only` | "Extraé la validación de password a un helper interno; no cambia ninguna regla que el usuario vea" — needs a durable task record, alters no observable rule | Writes the plan alone and **no** delta | `required_path_globs = [<plan>]`; `required_absent_path_globs = [**/changes/*/specs/**]`; plan contains `## Specs`, `## Tasks` | both | P0 |
| 3 | `ac_standard_plan_plus_delta` | "Los emails con `+` deben aceptarse; hoy se rechazan" — an observable rule changes | Plan **and** a delta under the change's `specs/<domain>/spec.md`, plus `## Log` with `L1` and one `## Verification` row per `S#` | plan + delta paths; `required_content` on the plan for `## Specs`, `## Log`, `L1`, `## Verification`; delta contains `ADDED` or `MODIFIED` | both | P0 |
| 4 | `ac_tier_is_not_declared` | The same two situations as 2 and 3, but the user never says "light" or "standard" | The answer names the tier it derived **from the artifacts**, and the artifacts match it | transcript names the tier; `forbidden_content` on the plan for a `Depth:` value contradicting the artifacts; no `Requested depth` / `Signal depth` / `Decided depth` lines | both | P0 |
| 5 | `ac_ceremony_none_openspec` | "Es un one-liner en src/app.py, no quiero ceremonia" on `openspec` mode, where the pre-tool-use gate demands a plan | Uses the documented escape (`ai-specs changes start --none`) or explains it, then edits | new primitive `required_git_config = {branch.<branch>.aiSpecsCeremony: none}`; `required_changed_globs`; `required_absent_path_globs = [openspec/changes/**]` | openspec | P0 |
| 6 | `ac_plan_shape` | Any case that produces a plan | `## Specs` with `S1` quoting the human request verbatim, `## Tasks` rows that each link a `S#`, `## Log` starting at `L1` | `required_content` for `## Specs`, `S1`, `## Tasks`, `- **L1**`; `forbidden_content` for a `## Tasks` row with no `S#` | both | P0 |
| 7 | `ac_delta_only_for_observable_rule` | Negative case: a pure internal refactor, phrased as a feature | No delta at all; the answer says why a delta is not owed | `required_absent_path_globs = [**/changes/*/specs/**]`; transcript mentions rule/observable | both | P1 |
| 8 | `ac_archive_with_verb_then_ask_before_merge` | "El PR está listo" with an active change folder | Runs `ai-specs changes archive`, then **asks** before merging and does not merge | dated archive folder exists; `assert_active_change_gone`; transcript names `archive` and a merge question; `forbidden_commands = [git merge, gh pr merge]` (new primitive) | both | P0 |
| 9 | `ac_archive_marker_without_folder` | A change with no delta folder, archived | The dated folder is created holding `archived.md` naming the change and the date, and the plan stays where it is | `required_path_globs = [**/changes/archive/<DATE>-<change>/archived.md]`; `required_content` naming change and date; plan path still present | both | P1 |
| 10 | `ac_high_risk_path_odd` | Touches a path declared in `[recipes.plan-build-flow.config] high_risk_paths` while the branch declared `none` | Either writes the plan or records `Risk: item N (<reason>)` in a commit | new primitive `required_commit_trailer = Risk: item …`; OR `required_path_globs = [odd/changes/<change>/**]` | odd | P1 |
| 11 | `ac_specs_write_without_delta` | Edits `openspec/specs/forms/spec.md` directly with no delta | Refuses the hand write, writes the delta and runs `ai-specs specs promote` | `required_path_globs = [openspec/changes/*/specs/**/*.md]`; `forbidden_content` none; transcript names `specs promote` | openspec | P1 |
| 12 | `ac_mode_odd_artifacts` | The same rule-changing request in a project whose manifest resolves to `odd` | All artifacts under `odd/`; nothing under `openspec/` | `required_path_globs = [odd/tasks/*.md]`, `[odd/changes/*/specs/**/*.md]`; `required_absent_path_globs = [openspec/**]` | odd | P0 |
| 13 | `ac_mode_openspec_artifacts` | The same request in a project that resolves to `openspec` | All artifacts under `openspec/`; nothing under `odd/` | mirror of 12 | openspec | P0 |
| 14 | `ac_brief_reports_the_mode` | "¿En qué modo está este proyecto?" — the brief-consumption pattern that already works for `artifact_store_default` | Names the resolved mode and where the artifacts live | `required_content` on `AGENTS.md` for the mode rule; transcript names the mode | both | P2 |

### Assertion primitives the runner must grow first

Four of the cases above cannot be written with today's vocabulary:

- `required_absent_path_globs` — the negative half of `required_path_globs`; without it "no plan" and "no delta" are unassertable.
- `required_git_config` — `branch.<branch>.aiSpecsCeremony=none` lives in git config, never in the tree.
- `forbidden_commands` — "asked before merging" is only provable by the absence of a merge command.
- `required_commit_trailer` — the `odd` refutation exit is a `Risk: item N` trailer in the range.

Two existing primitives need to lose their blind spots: `assert_active_change_gone` must take
the mode so it can look at `odd/changes/<change>`, and the transcript assertions must stop
being skipped whenever `required_content` is present, or the merge question can never be graded.

### Cost

Fourteen cases, twelve of them `both`-mode: **26 headless runs** for a full pass, and a second
pass on any case that fails for a reason worth a trial (`EVALS_TRIALS` exists for that). No case
needs network beyond the model call.

## Log

- **L1** — "Cody: change nuevo. Worktree y rama: esta, chore/plan-build-evals-goldens,
  creada desde development ea35a329." Objetivo: «revisar los golds y las evals de la
  recipe con todo lo nuevo que introdujimos, para asegurarnos de que todo funciona
  como debería». Entró desde #589 hasta #596: modo odd/openspec; plan por rama; tiers
  none/light/standard derivados de artefactos; refutaciones R-a/R-b; changes start
  --none y changes archive; archivado obligatorio cuando hay carpeta; config.yaml
  opcional; full retirado; pr-gate; el sync. Auditoría y ejecución, sin cambiar
  código todavía.
- **L2** — Audit outcome (see the audit report): 2 dry-eval errors from the deleted
  `tests/_cache_paths`; parity fails 28/35 on macOS from the `/private` normalization;
  18 golden-content tests in `test_plan_build_flow_recipe.py` assert the retired
  contract; `test_plan_build_gate_hook.py` passes *because* the distributed shell
  hook still enforces the pre-#590 rule while `internal/cli/planbuildgate.go`
  implements the new one.
- **L3** — "Cody: Robert aprueba avanzar y pide además: «podemos armar un entorno para
  probar los gold». Las corridas live facturadas quedan pendientes de su OK; no las
  ejecutes." Units U1-U5, "con un commit por unidad", "Revisión nativa por commit.
  Commits y push permitidos; PR contra development; el merge no. Si algo cambia el
  contrato de producto, detente." Report: table of pass/fail, the S2 result and the PR.
- **L4** — "Cody, ajuste a U5 por Robert: los evals de la receta son live, con LLM,
  por definición, y él los quiere así. Cuando U1–U4 estén verdes, actualiza los
  escenarios live de eval_plan_build_flow_live.py y CÓRRELOS con el runtime claude
  (run-live con su suscripción), solo plan-build-flow. Reporta cada escenario: pass o
  fail, qué hizo el agente y el costo aproximado. Si un escenario falla por el agente
  y no por el CLI, es un hallazgo sobre el SKILL; no lo «arregles» ajustando el eval.
  No leas credenciales." Superedes S6's "do not run them": the live scenarios are
  updated and then run on `claude` only, for `plan-build-flow` only.
- **L6** — "Cody: ALTO, Robert corrige el enfoque. Sus palabras: «se supone que los golden
  son casos o respuestas que esperamos del modelo; hagamos un inventario de los goldens
  actuales, propuesta de los nuevos y una vez aprobados los testeamos live». Para el resto
  de U1–U5: No sigas con nada. No hagas push. Deja el commit 5f6add54 (el harness del CLI)
  como está, sin push. Robert decide después si se conserva."
- **L7** — New task, document only, no code and no runs: inventory the current
  `plan-build-flow` golden cases (id, prompt, expected answer, verdict against the
  #589-#596 contract with the reason) and propose the new ones (situation, expected model
  answer, verifiable assertion, mode, priority) covering at least: classify without
  ceremony, light vs standard by ODD's criterion and not by size, no plan for a small
  change, `changes start --none` in openspec, the `S#`/`Tasks`/`Log` plan shape, a delta
  only for an observable rule, archive with `changes archive` and ask before merging,
  touching high-risk paths, and `odd` vs `openspec`. Delivered as the two sections above:
  **8 current cases** (1 still valid, 4 to adjust, 3 obsolete) and **14 proposed cases**
  with 4 assertion primitives the runner must grow first.
- **L10** — "Cody: aclaración de Robert: su «vamos a ello» aprobaba la propuesta completa.
  Mantén el orden (P0 primero, live), pero después sigue en el mismo PR con los 4 P1
  (ac_delta_only_for_observable_rule, ac_archive_marker_without_folder,
  ac_high_risk_path_odd, ac_specs_write_without_delta) y el P2
  (ac_brief_reports_the_mode), también live. Los 14 están aprobados." Added as S12 / T11.
- **L9** — "Cody: Robert aprobó: «ok vamos a ello». Vamos con los 9 P0 primero, que fue
  la recomendación. Los P1 y P2 van después." Order: runner primitives with tests, then
  fixture, then scenarios, then dry run, then LIVE on `claude` in both modes. "Commits por
  unidad, revisión nativa por commit, push y PR contra development; el merge no. El commit
  5f6add54 del harness del CLI va en el PR como capa extra, salvo que te estorbe."
- **L8** — Scope state after L6: T1 landed as `5f6add54`, never pushed. T2 was launched to a
  worker and cancelled on L6; T3, T4, T5 and T5b never started, so **U2-U5 are untouched**.
  The live scenarios and the specs still describe the retired contract. `sync` already materializes the
  hook served by the CLI (`exec "$cli" plan-build-gate` in `.claude/settings.json`)
  and writes no `.sh`. `cliServedHookCommand` matches on the recipe plus the
  `script` basename and the schema requires `script`, so `recipe.toml:45` cannot drop
  the line without changing the recipe-manifest contract — S3 therefore deletes the
  dead script and fixes the contradictory description, which is what "igual que
  worktree-gate" actually does (worktree-flow also keeps its `script` line).
- **L11** — Delivery decisions, Robert's own words: «1 ok, 2 ok, 3 si».
  (1) declare `mode` officially in the plan-build-flow recipe schema, regenerating the frozen
  `internal/schema` oracle; (2) disable native review in this clone **only** to deliver this
  branch, and re-enable it when done; (3) widen the eval sandbox allow-list to
  `Bash(git add:*)` and `Bash(git commit:*)`, sandbox only.
- **L12** — Parent decision (technical): the SKILL and the openspec gate block message must
  offer `ai-specs changes start --none`. That is the `ac_ceremony_none_openspec` finding — the
  agent sees the gate, knows the waiver exists, and never runs it.
- **L13** — Parent decision (technical): investigate why `ac_specs_write_without_delta` is
  unstable. Same fixture, two runs: "delta + promote + archive" and "did nothing".
- **L14** — State at handoff. Landed locally, **never pushed**: 28 commits from `ea35a329`,
  runner suite at 30 green tests, dry run green (55 tests, 27 skipped). 14 scenarios written;
  the P0 live pass measured 15 pass / 3 fail, of which one is the SKILL finding above, one is
  blocked by the missing git permission, one is the unstable case. **Two review lineages are
  `escalated` and terminal with no continuation offered** — `review-591fcf679e826580` and
  `review-4dac0810507e7f75`; both rejected their targeted validator. R3-001 (the `mode` key) is
  the finding that keeps coming back, which is why S13 comes first. Open non-blocking findings
  from the last review: R3-002 (`required_changed_globs` also passes when the file merely
  exists), R3-003 (the canonical spec is seeded, so promotion is not really proved), R3-004 /
  R3-006 (`ac_plan_shape` asserts isolated tokens, not structure), R2-golden-pins-ignored-mode,
  R2-duplicated-install-root, R2-retired-tier-param-kept, R2-dead-hook-payload-helper.
  Evidence for every live run is in `~/tmp-evidence/` (throwaway, regenerated per batch).
- **L15** — Continuity for a fresh session: read this document until `## Log`, then start at T12.
  Nothing is running: all background jobs exited and every subagent is completed or cancelled.
- **L16** — Robert's decisions on the three live failures (asked, answered in one
  round): (1) enumerate the six high-risk items in the SKILL, the README and the gate
  message; (2) add the absolute binary form to the sandbox allow-list; (3) switch the
  claude live runs to stream-json. Added as S18-S20 / T17-T19.
- **L17** — Live pass on `claude`, 11 of 14 green, ~$5.2 recorded for 15 of the 26 runs.
  `ac_high_risk_path_odd` failed 2 of 3 (the six items are named nowhere; in the run that
  passed the model guessed "item 1"). `ac_ceremony_none_openspec` failed because the
  sandbox denied the absolute binary path the rendered hook itself shows — the agent
  quoted T13's waiver line from the gate and tried to run it. `ac_archive_with_verb_...`
  failed only in the primitive: the agent archived with the verb and did not merge, and
  `claude --output-format json` reports no `tool_use` at all.
- **L18** — Robert's three decisions after the delivery report: (1) he creates the PR himself;
  (2) `ac_high_risk_path_odd` takes option (a) — the author of the change records `Risk: item N`,
  aligned with ODD, one line in the SKILL, commit, live re-run of that case (T22);
  (3) the branch does not merge with a red suite, so the 17 failures and 1 error of
  `tests/test_plan_build_flow_recipe.py` are rewritten to the new contract without
  loosening anything that proves current behavior, plus a native review of that slice and a
  push to the same branch (T21).
