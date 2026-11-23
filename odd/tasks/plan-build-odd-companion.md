# plan-build-odd-companion

Plan Build gets one execution mode per repository: under ODD the plan lives in `odd/` and only spec deltas are added; without ODD nothing changes.
Delta: `odd/changes/plan-build-odd-companion/specs/plan-build-flow/spec.md`. Proposal: `odd/changes/plan-build-odd-companion/proposal.md`.
Card: `6b0458aac00e111e539bf60b` — [fFNNuMFY](https://trello.com/c/fFNNuMFY)

## Specs

- **S1.** Modo por repositorio en `ai-specs/ai-specs.toml`: `[recipes.plan-build-flow.config] mode = "auto" | "odd" | "openspec"`, default `"auto"`.
- **S2.** En `auto` se infiere: *"si gentle-ai / Gentle Shell está presente en el proyecto, el modo es odd"*; el valor explícito gana sobre la inferencia.
- **S3.** En `odd`, el plan del change es `odd/tasks/<nombre>.md` emparejado con la rama, con prefijos normalizados; el gate lo valida como archivo regular y, si falta, bloquea con un mensaje claro. No hay `tasks.md` de OpenSpec.
- **S4.** `odd/specs/<dominio>/spec.md` son las canónicas en modo `odd`.
- **S5.** `odd/tasks/<change>.md` funciona igual que hoy.
- **S6.** `odd/changes/<change>/specs/<dominio>/spec.md` son los deltas, solo si el change cambia reglas observables.
- **S7.** `ai-specs specs promote` reusa el código y cambia solo la raíz según el modo.
- **S8.** No hay `odd/config.yml`.
- **S9.** En `mode = "openspec"` todo queda intacto; acercar ese flujo a ODD es otro change.
- **S10.** El modo elige el árbol del plan; nunca exime del requisito de plan previo (la regla canónica "no mode bypass" sobrevive).

## Tasks

| ID | Spec | Tarea | Ruta | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1, S2, S10 | Resolución de modo: paquete Go nuevo `internal/planbuild/mode.go` con `ResolveMode(manifest, gitCommonDir)`. Lee `[recipes.plan-build-flow.config].mode` con el helper que ya usa el ledger; `auto` sondea `<git-common-dir>/gentle-ai/`. Tests: explícito gana; `auto` + state dir → `odd`; `auto` sin state dir → `openspec`; manifiesto ausente/ilegible/valor inválido → `openspec`; common dir irresoluble → `openspec`. | worker | pendiente |
| T2 | S3 | Emparejamiento de rama: `ChangeFromBranch(name)` (último segmento, sin prefijo) y `PlanPath(root, change)`. Tests de tabla: `feat/x`→`x`, `fix/x`→`x`, `x`→`x`, `user/feat/x`→`x`, detached HEAD→error. Verifica que la ruta esperada sea exactamente `odd/tasks/<change>.md`. | worker | pendiente |
| T3 | S3, S10 | Ramas del gate en `internal/cli/planbuildgate.go` sin tocar el camino `openspec`: en `odd` exige el plan de ODD; rechaza si falta, si es directorio o si HEAD está detached; ignora `openspec/changes/*/tasks.md`; permite escribir bajo `odd/**`. Tests por regla, con exit code, mensaje exacto y estado del disco intacto tras el rechazo: allow con plan, block sin plan (mensaje nombra la ruta y `mode = "openspec"`), block con directorio, block pese a `tasks.md` de OpenSpec presente, allow al escribir el plan, block en detached HEAD. | worker | pendiente |
| T4 | S10 | Regresión de `openspec` mode: reejecutar los tests actuales del gate sin cambios y añadir un test que fije que el comportamiento con y sin carpeta de change es idéntico al de antes del change. | worker | pendiente |
| T5 | S4, S5, S6, S7, S8 | Promoción por modo: en `internal/specs/promote.go` los roots (origen y canónico) se resuelven por modo; el compositor, los blockers, la colisión por dominio y la idempotencia no cambian. Tests: en `odd` compone `odd/changes/<slug>/specs/<domain>/spec.md` en `odd/specs/<domain>/spec.md`; `openspec/specs/**` queda byte-idéntico; sin delta no hay promoción ni blocker; rerun idempotente; `odd/config.yml` no se crea ni se lee. | worker | pendiente |
| T6 | S1, S3, S6 | Documentación: `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` §4.1 y §7.1, el README de la receta y la ayuda de `ai-specs specs promote` / `ai-specs plan-build-gate`. Tests: los que ya fijan el vocabulario del SKILL siguen verdes; uno nuevo fija que §7.1 nombra la excepción de `odd`. | inline | pendiente |
| T7 | S9 | Aislamiento de `openspec` mode: `./tests/run.sh` y el resto de la suite Go sin cambios de expectativas. Cualquier diff en el flujo `openspec` es regresión, no ajuste. | inline | pendiente |
| T8 | — | Cierre: `./tests/validate.sh`, `docs/go-migration-parity-contract.md` con las superficies nuevas, y decisión sobre los puntos abiertos de abajo. | inline | pendiente |

**Estado**: T1–T6 y T8 implementados en `2598c1f`, `59dca51`, `76daeec`, `de76962` y `83b7a1c`; T7 verificado con la suite y con la paridad comparada contra el baseline de `development`. El smoke del binario en un repo temporal cubre gate, promoción y premerge en ambos modos.

## Puntos abiertos

### A. `work-start` del ledger en modo `odd`

Evidencia: el gate califica `work-start` **antes** del plan (`internal/cli/planbuildgate.go:126-130`). El predicado puro (`catalog/recipes/worktree-flow/gate/ledger/verdict.go`) permite por defecto: sin binding → `dormant`; con binding y sin item → `outcome(needs-item)`, que en `warn` (default) resuelve `allow`, en `ask` resuelve `ask` (exit 0) y en `always` resuelve `block` (exit 2).

Propuesta: **no cambiar la política**. El tracker sigue siendo una obligación independiente del modo y el gate sigue siendo su host. Lo único que entra en este change es el diagnóstico: cuando el rechazo viene del ledger, el mensaje lo dice; cuando viene del plan, dice `odd/tasks/<change>.md`. Mover el host fuera de este gate queda fuera, como change futuro.

### B. `gates premerge` en modo `odd`

Evidencia: `internal/gates/gates.go:115-137` exige carpeta activa en `openspec/changes/<slug>/` y luego su archivado; `internal/gates/archive.go:21` busca `openspec/changes/archive/<slug>/`. El modo `odd` no tiene ciclo de vida de OpenSpec, así que ambos chequeos se quedan sin objeto.

**Resuelto por Robert** (L7): el CLI resuelve el modo con el mismo resolvedor que el gate (`planbuild.ResolveRepo`), descubre el change por el emparejamiento de rama (`planbuild.Branch` + `ChangeFromBranch`) y valida la paridad `odd/changes/<change>/specs/` → `odd/specs/`; no exige archivado de OpenSpec y un change sin delta pasa con su plan. Implementado en `gates.CheckPremergeOdd` + `specs.CheckFolderParityForMode`.

### C. Dos árboles canónicos

Los 55 dominios actuales viven en `openspec/specs/`. En `odd`, `odd/specs/` arranca vacío y el mismo dominio puede existir en ambos con contenido distinto.

Propuesta: no migrar nada en este change; `openspec/specs/` queda intacto y sin leer. Una migración de una vez (`specs migrate`) es decisión aparte, con su propio riesgo de reescritura masiva.

### D. Punto ciego de `auto`

Un proyecto con Gentle AI instalado pero sin uso previo no tiene `<git-common-dir>/gentle-ai/` todavía, así que `auto` resuelve `openspec`.

Propuesta: documentarlo y dejar que el `mode = "odd"` explícito lo resuelva; opcionalmente, que la primera ejecución nativa cree el directorio de forma idempotente, en un change aparte.

### E. §7.2 del SKILL exige la carpeta de OpenSpec para abrir PR

Evidencia: `SKILL.md` §7.2, item 1 — *"The matching `openspec/changes/<slug>/` folder exists on the current branch"*. En `odd` sin delta no hay carpeta, así que el gate de PR se detendría.

Propuesta: en `odd`, el item 1 se satisface con `odd/tasks/<change>.md` y, si hay delta, con `odd/changes/<change>/`. Robert acotó el encargo a §4.1 y §7.1, así que esta sección queda sin tocar y es la primera que hay que ajustar al implementar T6.

### F. `odd/` está excluido de git en este clon

Evidencia: `.git/info/exclude:10` contiene `odd/`, así que `git check-ignore -v odd/tasks/plan-build-odd-companion.md` lo marca ignorado y `git status` no lo ve. Los 154 ficheros de `odd/tasks/` están versionados porque se añadieron con `-f` en su momento.

Consecuencia: en `mode = "odd"`, `odd/specs/**` sería el contrato canónico pero nacería sin versión. Es fatal para el propósito del change.

Propuesta: el change debe exigir que `odd/specs/` y `odd/changes/` estén versionados. Dos piezas: (a) documentarlo y añadir la línea al `doctor` o al sync que detecte `odd/` excluido; (b) no tocar `.git/info/exclude` de forma automática: es configuración local del clon, no artefacto del repo.

## Log

- **L1** — Robert, verbatim: *"sesión nueva para el change plan-build-odd-companion (esta worktree, rama plan-build-odd-companion en c49aa77). Robert pidió SOLO PREPARACIÓN: no implementes, no commitees, no hagas push ni PR."*
- **L2** — Robert acotó el objetivo: ODD es dueño del plan y las tareas; el change escribe deltas que se promueven a las canónicas; de OpenSpec solo se rescatan las specs (`openspec/specs/**`), y el flujo de `proposal`/`tasks.md` de OpenSpec ya no es el camino.
- **L3** — Robert, verbatim: *"En modo ODD (ODD presente y dueño del change): ODD lleva el plan y las tareas. De OpenSpec solo sumamos las specs […] Sin ODD: plan-build-flow sigue como hoy, centrado en OpenSpec, con tasks.md incluido. No lo cambiamos en este change; acercarlo a ODD será un change futuro."*
- **L4** — Robert decidió el diseño completo (S1–S9) y pidió señal de detección robusta y verificable para "Gentle Shell presente", descartando las frágiles.
- **L5** — Robert, verbatim: *"No commitees, no hagas push y no toques todavía el código Go."*
- **L6** — Señal elegida: `<git-common-dir>/gentle-ai/`. Descartadas por frágiles: `.pi/.claude/.cursor/.omp/.opencode` (los crea `ai-specs sync` para cualquier target), `AGENTS.md`, registros de hook en `ai-specs/.ai-specs.lock`, `$HOME/.gentle-shell` (máquina, no proyecto), variables `PI_*` (no verificables desde un CLI suelto) y la mera presencia de `odd/tasks/*.md` (154 ficheros versionados: satisfaría el gate para siempre).
- **L7** — Robert decidió, verbatim: *«claramente el gate debe permitir ambos modos; por eso decidimos dejar todo eso del lado del CLI, porque el CLI puede dar una configuración más determinista del modo y del estado»*. Confirma que el repo quede en `odd` por `auto` al mergear, y pide el premerge ODD en este PR y en la misma corrección que los hallazgos R1-001 y R1-002.
- **L8** — Decisiones técnicas de "entrega en odd" (mías, según el criterio de L7): (a) el veredicto es plan emparejado + paridad de deltas; ni archivado, ni mínimos de tier, ni `verify-report.md`, porque en ODD la verificación es de ODD; (b) `--stage pre-archive` sigue siendo solo de `openspec`; (c) `<slug>` se acepta y se ignora en `odd`, porque el change viene de la rama; (d) una rama sin commits empareja igual (`symbolic-ref`), y HEAD detached bloquea; (e) el plan y la carpeta del change deben estar contenidos en el repo, así un symlink en un componente padre no los resuelve a otra parte; (f) `openspec` no cambia: el empaquetado de symlinks y el guardián son los mismos.
