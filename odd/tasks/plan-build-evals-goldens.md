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
- **L8** — Scope state after L6: T1 landed as `5f6add54`, never pushed. T2 was launched to a
  worker and cancelled on L6; T3, T4, T5 and T5b never started, so **U2-U5 are untouched**.
  The live scenarios and the specs still describe the retired contract. `sync` already materializes the
  hook served by the CLI (`exec "$cli" plan-build-gate` in `.claude/settings.json`)
  and writes no `.sh`. `cliServedHookCommand` matches on the recipe plus the
  `script` basename and the schema requires `script`, so `recipe.toml:45` cannot drop
  the line without changing the recipe-manifest contract — S3 therefore deletes the
  dead script and fixes the contradictory description, which is what "igual que
  worktree-gate" actually does (worktree-flow also keeps its `script` line).
