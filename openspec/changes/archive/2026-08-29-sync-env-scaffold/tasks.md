# sync-env-scaffold — tasks

Depth: standard

## Objective

Estandarizar el renderizado de envs: sync regenera `ai-specs.env.example` en la
raíz + asegura `.envrc` (merge-safe), y advierte (no falla) las variables que
una recipe habilitada requiere pero que aún no tienen valor en `ai-specs.env`.
Nunca escribe en `ai-specs/` ejemplos de env, nunca escribe secretos, nunca toca
`.env` de la app.

## Context

- `lib/_internal/env_scaffold.py::collect_env_vars(project_root)` junta las
  `$VAR` de recipes habilitadas desde el manifest.
- `generate_env_example(root)` escribe la raíz `ai-specs.env.example` **y aún
  escribe stubs en `ai-specs/.env.example` + `ai-specs/.envrc.example`**
  (líneas 249-250) — esos stubs se eliminan.
- `ensure_root_envrc(root)` produce el bloque merge-safe.
- `load_harness_env(root)` lee `ai-specs.env` (dict vacío si falta).
- `main()` de env_scaffold ya hace generate + ensure de forma no-interactiva.
- `lib/sync.sh` corre pasos vía `run_step`; hoy NO invoca env_scaffold (solo
  init/configure/recipe-add lo hacen de forma interactiva).

## Tasks

- [x] T1. `env_scaffold.py::generate_env_example`: eliminar los dos
      `_write_deprecation_stub` bajo `ai-specs/` (líneas 249-250). El generador
      ya no crea `ai-specs/.env.example` ni `ai-specs/.envrc.example`.
- [x] T2. `env_scaffold.py`: añadir una función `missing_required_values(root)`
      que retorna las vars requeridas por recipes habilitadas sin valor en
      `ai-specs.env` (diff de `collect_env_vars` vs `load_harness_env`).
- [x] T3. `env_scaffold.py::main()`: además de generate + ensure, imprimir a
      stderr una línea `! <VAR> sin valor en ai-specs.env — ejecuta
      ai-specs configure-recipes` por cada var faltante. Exit sigue siendo 0
      (advertencia no fatal).
- [x] T4. `lib/sync.sh`: añadir `ENV_SCAFFOLD_PY=".../env_scaffold.py"` y un
      `run_step "harness env (.envrc + ai-specs.env.example)" python3
      "$ENV_SCAFFOLD_PY" "$ROOT_PATH"` tras vendored-skills (paso 3) y antes de
      AGENTS.md (paso 4). Actualizar el comentario header del pipeline.
- [x] T5. Tests black-box: `bin/ai-specs sync` en proyecto temp verifica:
      (a) env de recipe recién habilitada aparece en `ai-specs.env.example`;
      (b) `.envrc` creado con managed block; (c) custom `.envrc` preservado;
      (d) warning no-fatal emit para var sin valor en `ai-specs.env` y exit 0;
      (e) sync NO crea `ai-specs.env` ni toca `.env`; (f) NO crea
      `ai-specs/.env.example` ni `ai-specs/.envrc.example`.
- [x] T6. Ajustar la spec canónica `openspec/specs/harness-env-scaffold/spec.md`
      para remover la mención de stubs deprecated bajo `ai-specs/` (según el
      delta del change).
- [x] T7. `./tests/validate.sh` (sin pipe, exit real) verde contra la suite
      existente.

## Non-goals

- Sin prompting / sin escribir secretos en sync.
- No migrar el flujo interactivo `offer_harness_env`.
- No tocar el `.env` de la app.
- El work de jinna-mcp-recipe (worktree aparte con conflicto sin resolver) queda
  fuera de este change.