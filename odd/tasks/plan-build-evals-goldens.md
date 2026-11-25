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
- **L5** — S2 answered empirically by the U1 golden: `sync` already materializes the
  hook served by the CLI (`exec "$cli" plan-build-gate` in `.claude/settings.json`)
  and writes no `.sh`. `cliServedHookCommand` matches on the recipe plus the
  `script` basename and the schema requires `script`, so `recipe.toml:45` cannot drop
  the line without changing the recipe-manifest contract — S3 therefore deletes the
  dead script and fixes the contradictory description, which is what "igual que
  worktree-gate" actually does (worktree-flow also keeps its `script` line).
