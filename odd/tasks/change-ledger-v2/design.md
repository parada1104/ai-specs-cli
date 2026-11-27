# Rediseño del ledger — **v3**

**Estado**: propuesta, sin código. `development` @ `36438150`.
**Base**: inventario read-only de los ledgers actuales + reproducción del gate en proyectos temporales. Cada afirmación sobre el presente lleva `archivo:línea` o salida real de comando.
**Fuera de alcance**: no se editó el repo, no hubo commit, PR ni `sync`.

---

## 0. Qué cambió respecto de la v2

| # | v2 decía | v3 |
|---|---|---|
| 1 | El gate de escritura de `plan-build-flow` quedaba bloqueante "como contrato de la capacidad" y era decisión abierta nº 2 | **Resuelto en contra de la v2**: el gate **no bloquea por falta de plan** en ningún modo; **infiere el tier** y sólo bloquea cuando una afirmación queda **refutada por hechos objetivos**, nombrando la regla y las salidas. Se documenta con evidencia qué hace hoy (§1) y se marca el defecto |
| 2 | `none` era un valor válido del tier | `none` (small) es una **afirmación**: el CLI no la prueba, sólo la **refuta** con hechos; si la refuta, el bloqueo dice la regla y las salidas |
| 3 | `changes start --none` quedaba como waiver vigente | **Deja de ser precondición del gate**: no hay nada que desbloquear porque nada bloquea por falta de plan. Su retiro es decisión abierta (§8 D3) |
| 4 | `require` opt-in "del proyecto" | **`require` por proyecto con detalle por checkpoint** |
| 5 | Card con `ask` como default general | `ask` **al habilitar un tracker** (Trello/Jira/OpenProject); sin tracker, `off` |
| 6 | Desacuerdo tracker↔change: "se informa" | Se informa **y se ofrecen decisiones concretas** de reconciliación (§5.4) |
| 7 | Stores: archivos por ledger (recomendado) | Igual, **más el invariante explícito**: nadie lee ni escribe esos archivos por fuera del entrypoint del CLI; el layout es implementación, no contrato (§6) |

---

## 1. Qué hace hoy `plan-build-gate` sin plan: ODD y SDD

**Cómo se invoca**: `ai-specs plan-build-gate` lee **un evento JSON por stdin** y no acepta argumentos de grading (`internal/cli/planbuildgate.go:66–101,146–166`). El hook pre-tool-use de `plan-build-flow` lo llama con matcher `Edit|Write|MultiEdit|NotebookEdit` y `blocking = true` (`catalog/recipes/plan-build-flow/recipe.toml:48–54`); Cursor queda fuera porque no tiene hook pre-escritura (`internal/sync/hooksrender.go:816–834`). Directorios de producción por defecto: `src lib catalog` (`PLAN_BUILD_GATE_PATHS` los cambia).

**Reproducción** (proyecto git aislado en `feat/alpha`, evento con `tool_input.file_path = src/app.go`):

| Caso | Exit | Stderr |
|---|---:|---|
| ODD, sin plan | **0** | vacío |
| OpenSpec, carpeta activa **sin** `tasks.md` | **2** | bloqueo + waiver |
| OpenSpec, con `tasks.md` | **0** | vacío |
| OpenSpec, sin carpeta, tras `changes start --none` | **0** | vacío |
| ODD, escritura a mano en `odd/specs/` sin delta | **2** | bloqueo con la regla |

**Textos reales** (verbatim):

```
plan-build-gate: refusing to Write 'src/app.go' — no active change folder (openspec/changes/<slug>/tasks.md) found. Classify the change and write the plan first, then implement. Writing planning artifacts under openspec/changes/ is never blocked.
If this change genuinely needs no plan, run 'ai-specs changes start --none --root <planning-root>' and retry the write instead of writing a plan nobody needs.
```

```
plan-build-gate: refusing to Write 'odd/specs/cap/spec.md' — ODD mode is active and 'odd/specs/' is written by promotion, not by hand; … Write the delta there and run 'ai-specs specs promote' …
```

**Lectura por modo**

| Modo | Sin plan, escritura de producción | Bloquea? | Pide autorización? |
|---|---|---|---|
| **ODD** | exit **0**, silencio (`internal/cli/planbuildgate.go:214–222`) | **No** — ya cumple la decisión | No |
| **SDD/OpenSpec** | exit **2** + waiver (`:224–253`, `internal/planbuild/pair.go:229–235`) | **Sí** | **Sí**: `ai-specs changes start --none` |
| ODD, `odd/specs/` a mano | exit **2** (`:196–202,265–275`) | Sí, por otra razón (ver abajo) | No |

**Tier: quién lo infiere hoy**

- El **gate de escritura no infiere tier**: no parsea planes ni deriva `none|light|standard`; sólo exige un plan utilizable (o el waiver) en OpenSpec (`:214–253`).
- El **guardian de merge** sí infiere: `none` sin artefactos, `light` con plan **o** delta, `standard` con ambos (`internal/gates/gates.go:34–82`). Pero el guardian **ODD no calcula ese tier**: reporta `none` u `odd` (`:218–220`).
- Las **refutaciones por diff** ya existen y se aplican aun en `none` (`internal/gates/refute.go:99–175`): specs canónicas sin delta que las aporte, y rutas de alto riesgo declaradas por el proyecto sin la evidencia requerida. **Ese es el mecanismo que la inferencia debe reutilizar.**

**Veredicto: es un defecto respecto de la decisión, con un matiz que hay que decir sin adornos.**

La decisión del 24-nov **se implementó sólo para ODD**. El commit `e931c4f4` dice: *"The tier stops being declared and starts being derived from the artifacts; none passes unless objectively refuted; the odd hook stops demanding a plan and protects only odd/specs"*, y el registro aprobado elige explícitamente la declaración local para OpenSpec: *"hook en `openspec`: acepta una declaración local, `ai-specs changes start --none`"* (`odd/tasks/plan-build-openspec-odd-align.md:319–324`), tras estudiar y descartar la alternativa de relajar OpenSpec como ODD.

Es decir: no es que el código se haya desviado de la decisión; es que **la decisión nueva la reemplaza** para OpenSpec. Consecuencias concretas a corregir en el rediseño:

1. `internal/cli/planbuildgate.go:224–253` — el bloqueo por falta de plan en OpenSpec debe convertirse en **inferencia + refutación**.
2. `:141–143,224–227,240–252` — los textos que **mandan a la ceremonia** desaparecen; el mensaje nuevo nombra la regla refutada y sus salidas.
3. `internal/planbuild/ceremony.go:23–50` + `internal/cli/changes.go:131–149` — `changes start --none` deja de ser alternativa al plan.
4. `internal/gates/gates.go:218–220` — el guardian ODD debe calcular el mismo tier de tres valores para que la refutación escale igual en ambos modos.
5. Tests que hoy fijan lo contrario y **deberán cambiar de expectativa**, no de intención: `TestPlanBuildGateRefusesProductionWriteWithoutPlan` (`planbuildgate_test.go:119`), `…OpenspecPairedFolderNeedsAPlan` (`:720`), `…RefusalOffersTheNoneWaiver` (`:637`).

**Lo que NO es defecto y no se toca**: el guard de `odd/specs/` (escribir a mano lo que la promoción genera) y el guardian de archive-antes-de-merge. Ambos bloquean por **hechos refutables con nombre y salida**, que es exactamente la forma que la decisión pide.

---

## 2. Resumen ejecutivo

El ledger actual no tiene un problema de modelo de datos: tiene un problema de **propiedad** (un store para tres dominios) y de **contrato de entrada** (heurísticas sobre carpetas y markdown). La v3 mantiene la arquitectura de tres ledgers por capacidad y le agrega la corrección de fondo del punto 1: **el CLI infiere, no autoriza**. `none` es una afirmación refutable; el bloqueo, cuando ocurre, siempre nombra la regla y las salidas.

---

## 3. Inventario (condensado)

| Aspecto | Hoy |
|---|---|
| Store | `{v:1,items,opt_outs}` en `<git-common-dir>/ai-specs/ledger/state.json`; `flock` + temporal + fsync + rename; corrupción = error explícito (`ledger/store.go:183–255`) |
| Identidad | `common_dir + rama [+ change]`; compartida por worktrees del clone (`ledger/identity.go:73–111`) |
| Estado vivo | **14 filas, todas cerradas**, último `decision.at` = `2026-10-08T14:40:02Z` = mtime. Ninguna unidad de noviembre registró |
| Disparador real | hook de agente con matcher `lib catalog bin src` + `## Tracker`/`card_id` (`internal/cli/trackercardgate.go:213–324`, `internal/tracker/section.go:17–26`) |
| Veredictos | tracker **siempre 0** (informa); `plan-build-gate` y guardian pueden 2; `worktree-gate --ledger` 2 sólo en `always` |
| Evidencia de merge | ascendencia ∨ `git cherry` ∨ patch-id ∨ `ls-tree` por rutas ∨ PR con head OID == tip (`runner/cleanup.go:147–278,453–545`) |
| Muerto | `PersistConflict`, `AppendDecisionToPrimary` sin llamador |
| Cobertura | `tests/scenarios/ledger_cleanup_flow.sh` (bind, 4 estrategias, limpieza, idempotencia, hooks); sin e2e para otros checkpoints, negativas de tier ni `archive` |

**Mapa propiedad real**: `ledger/identity.go`, `worktree.go`, `runner/cleanup.go` → **L1**; `internal/planbuild/*`, `planbuildgate.go`, `internal/gates/*`, `internal/cli/changes.go` → **L2**; `internal/tracker/*`, `trackercardgate.go`, `runner/ledger_reconcile.go` → **L3**. El monolito es tres ledgers compartiendo archivo.

---

## 4. L1 — Ledger de worktree

Posee: identidad del árbol, prueba de merge, limpieza, integración.

| Hecho | Verificación | `unknown` si |
|---|---|---|
| W1 identidad | common-dir real, rama, tipo (main/linked/detached), `status --porcelain` | no hay work tree |
| W2 merge | las cinco estrategias + verificación por PR | ninguna prueba |
| W3 limpieza | worktree ausente ∧ remoto verificado ausente ∧ rama local ausente | remoto inalcanzable |
| W4 integración | `origin/<integration>` contiene W2; snapshot fresco | sin snapshot/remote |

Checkpoints: `tree-open`, `merged`, `cleaned`, `integrated`. Verbos: `worktree-gate --ledger` (primitiva), `worktree-cleanup` (ejecutor). No sabe de planes, tiers, cards ni review.
**Tras el merge el CLI sólo verifica la integración y entrega el comando** — no ejecuta `pull`.

---

## 5. L2 — Ledger de plan-build-flow (reescrito)

Posee: la capa del change — qué artefacto existe, qué tier se **infiere**, qué se **refuta**, y si el archive ocurrió.

### 5.1 Inferencia, no autorización

| Hecho | Cómo se determina | Nota |
|---|---|---|
| P1 artefacto | carpeta OpenSpec activa **∨** plan ODD emparejado por rama; ambiguo ⇒ `unknown` | nunca bloquea por ausencia |
| P2 tier | **inferido**: sin plan = `none`; plan = `light`; plan + delta = `standard` (`internal/gates/gates.go:34–82`) | se calcula **también en ODD** (hoy no: `:218–220`) |
| P3 verificación | una fila por `S#` declarado + refutaciones del diff (`evidence.go:131–175`, `refute.go:99–175`) | sólo si el tier lo pide |
| P4 archive | carpeta del change en el árbol de archivo **en el commit a mergear**, con paridad (`gates.go:175–181`) | `not-applicable` sin carpeta |

### 5.2 `none` es una afirmación refutable

El CLI **no prueba** que un cambio sea small. Enumera **refutaciones objetivas** y, si alguna se cumple, sube el tier mínimo:

| Refutación | Hecho objetivo |
|---|---|
| R1 specs canónicas sin delta que las aporte | el cambio toca `odd/specs/` o `openspec/specs/` sin delta |
| R2 rutas de alto riesgo declaradas sin la evidencia exigida | rutas del proyecto marcadas (auth, permisos, datos, migraciones, contratos) |
| R3 tamaño/estructura | el diff excede el umbral configurado o toca N módulos con contrato compartido |
| R4 contrato consumido por terceros | se modifican flags/CLI/exports/formato de config |
| R5 orfandad de evidencia | hay plan declarando `S#` sin filas de verificación |

Cuando una refutación se cumple, el bloqueo **nombra la regla** (R1…R5), el hecho que la cumple y **las salidas**: escribir el plan/delta, registrar la decisión de exención, o escalar. Nunca "pedí permiso".

### 5.3 Checkpoints

`work-start` (inferencia + refutación; **no** autorización), `planned`, `ready`, `archived`. Si `plan-build-flow` no está habilitado, la capa colapsa a `none` y el compuesto lo dice.

**Se retira la ceremonia**: `branch.<b>.aiSpecsCeremony=none` deja de ser precondición (`internal/planbuild/ceremony.go:23–50`, `internal/cli/changes.go:131–149`). Durante la ventana de doble lectura se acepta el valor como dato inerte para no romper repos existentes; no desbloquea nada porque nada bloquea por falta de plan.

---

## 6. L3 — Ledger de tracker

| Hecho | Verificación | Sin tracker |
|---|---|---|
| T1 binding | fila propia ∨ evidencia documental con precedencia (OpenSpec → plan ODD → rama) | `not-applicable` |
| T2 estado del item | observación del adaptador (Trello/Jira/OpenProject) | `unknown` si el provider falla |
| T3 conciliación | estado del item vs estado derivado del change | — |
| T4 cierre | item cerrado al archivar/mergear | — |

### 6.4 Desacuerdo tracker↔change: informar y ofrecer decisiones

Cuando T3 detecta desacuerdo, el compuesto **no corrige nada**; muestra el desacuerdo y **ofrece decisiones explícitas**, cada una registrable:

| Decisión ofrecida | Efecto |
|---|---|
| `sync-to-provider` | mover el item al estado que dicta el change |
| `accept-provider-state` | aceptar el estado del item y ajustar la expectativa del change |
| `relink` | el binding apunta al item equivocado: re-vincular |
| `create` | no hay item correcto: crear uno nuevo |
| `exempt` | exención registrada con motivo |
| `escalate` | dejarlo pendiente para un humano, con nota |

Cada una es una entrada append-only; el CLI nunca decide por su cuenta cuál aplicar.

---

## 7. Arquitectura, contratos y composición

### 7.1 Contratos entre ledgers

1. Cada ledger es dueño de sus hechos; **nunca** lee el store de otro.
2. Conversan por interfaz de consulta pura (`Facts()`, `Verdict(checkpoint)`); las escrituras son comandos del dueño.
3. La composición es de sólo lectura, ordenada L2 → L1 → L3.
4. Hecho ausente = `unknown`, se muestra; nunca default optimista.
5. Desacuerdo (entre ledgers o texto↔hechos) = dato visible, no corrección silenciosa.

### 7.2 Vista compuesta: `ai-specs changes status` / `changes next`

Familia `changes` (nuestra: `archive`, `start --none`). No posee estado; compone.

```
$ ai-specs changes status
change:  fix/installer-home-resolution   branch worktree, clean        [L1]
phase:   merged → pending cleanup                                     [L1]
plan:    light (plan only)   refutations: none   archived: no (not applicable) [L2]
tracker: not-applicable (no tracker recipe bound)                     [L3]
review:  no receipt for 4eb3e609 (risk high → recommended)
next:    ai-specs worktree-cleanup
why:     merged by squash (patch-id verified against origin/development)
blocked: nothing
```

`--json`, `--explain` (evidencia por hecho), `--ledger worktree|plan|tracker` (drill-down), `--rebuild` (recaché). Códigos: `0` sin bloqueo, `2` sólo con `require` opt-in, `3` si un hecho necesario es `unknown`. Naming: se mantiene `changes`; slug posicional opcional, resolución por rama sin argumento.

### 7.3 Flujo según capacidades activas

| Capacidad | Activa | Inactiva |
|---|---|---|
| `worktree-flow` | L1 completo; hooks de git | L1 responde merge/integración con git puro; sin cleanup automático |
| `plan-build-flow` | L2: inferencia, refutación, archive | capa `none`, declarada como tal |
| tracker | L3 con `ask` por defecto | `not-applicable` |

### 7.4 Posturas

`observe` / **`advise` (default en todo nivel de riesgo)** / `require` **por proyecto, con detalle por checkpoint** (opt-in explícito). El riesgo ordena la recomendación; no es un interruptor. El matcher por carpetas deja de decidir el flujo: queda como señal débil.

### 7.5 Card/item

`off` sin tracker; **`ask` por defecto al habilitar un tracker** (pregunta si crear o vincular; sin TTY informa y sigue, registrando la respuesta para no repreguntar); `always` opt-in. Genérico por adaptador: Trello card, Jira issue, OpenProject work package.

---

## 8. Estado, storage e invariante

- **Tres stores**: `.git/ai-specs/ledgers/{worktree,plan,tracker}.json`, en el common-dir, con `flock` + temporal + fsync + rename.
- **Invariante nuevo**: **todo pasa por el entrypoint del CLI**. Nadie lee ni escribe esos archivos directamente — ni agentes, ni hooks, ni scripts, ni tests de terceros. El layout en disco es **implementación, no contrato**: podrá migrar a un solo archivo o a otro motor sin romper a nadie, porque lo único estable es `ai-specs changes …` (y sus formas `--json`).
- **Ningún campo `state` escribible** para el change: la fase se deriva.
- **Caché descartable** por identidad, reconstruible con `--rebuild`.
- **Receipts de revisión** consumidos de gentle-ai (SHA de candidato + veredicto + origen), referenciados, nunca fabricados; `unknown` si no están.

**Migración**: v1→v2 con doble lectura una versión; **las 14 filas quedan como histórico** sin remigrar; cada decisión de v1 se copia al ledger que la posee (bindings/opt-outs → tracker, cierres → worktree), con tracker como dueño por defecto y reporte único si es ambiguo; los hooks `post-checkout`/`post-merge` sólo refrescan caché; el wrapper de Pi se arregla en el rediseño con el contrato "stderr siempre visible; el exit code expresa bloqueo, no visibilidad" y un test que lo fija.

---

## 9. Tabla de cambio respecto del diseño actual

| | Elemento | Motivo |
|---|---|---|
| **DEJAMOS** | Store append-only con flock + rename en common-dir | correcto y auditable; se replica por ledger |
| | Evidencia múltiple de merge + `gh pr list` (head OID == tip) | única forma honesta con squash/rebase |
| | Orden de `cleanup` y "sin prueba no borra" | propiedad de seguridad más valiosa |
| | Guardian de archive-antes-de-merge y guard de `odd/specs/` | bloquean con hechos + regla + salida, no por autorización |
| | Refutaciones por diff (`internal/gates/refute.go`) | es exactamente el mecanismo que la inferencia necesita |
| | Tier derivado de artefactos | gramática que ya querés |
| | `--explain`, JSON estable, mecánica `ask` | auditable y reutilizable |
| **QUITAMOS** | Un store para tres dominios | causa estructural |
| | Bloqueo por falta de plan en OpenSpec + waiver `start --none` | contradice la decisión: el CLI infiere, no autoriza |
| | El gate de escritura sin inferencia de tier | deja la inferencia sólo en el merge |
| | Guardian ODD sin tier de tres valores | impide escalar la refutación igual en ambos modos |
| | Matcher `lib catalog bin src` como disparador | falla en silencio |
| | `## Tracker` como requisito de bind | el texto es evidencia, no contrato |
| | `status: open|closed` como estado | campo escribible que miente |
| | `PersistConflict`/`AppendDecisionToPrimary` | código muerto |
| | `archive-close` como concepto aparte | duplica el hecho del archive |
| **CAMBIAMOS** | Gate de escritura: de "exigir plan" a **inferir tier + refutar** | decisión de producto |
| | `none`: de valor del tier a **afirmación refutable** | el CLI no prueba small, lo refuta |
| | Mensajes de bloqueo: de "pedí permiso" a **regla + hecho + salidas** | hace audible el porqué |
| | De monolito a **tres ledgers** con contratos | separa lo que compite por un archivo |
| | De `worktree-gate --checkpoint X` a `changes status|next` compuesto | la pregunta real es "qué me toca" |
| | De modo global a **postura por transición** (`advise` default) | no obliga por defecto |
| | Ciclo hasta `integrated`, **verificado, no ejecutado** | el humano decide cuándo tocar `development` |
| **AGREGAMOS** | Reglas de refutación R1–R5 con salidas nombradas | sustituyen la autorización por hechos |
| | `changes status|next` (`--json`, `--explain`, `--ledger`, `--rebuild`) | vista compuesta inexistente |
| | `unknown` de primera clase + exit 3 | "no sé" ≠ "podés" |
| | Fases `cleaned`/`integrated` verificadas | el ciclo no termina al borrar |
| | Menú de decisiones de conciliación (§6.4) | el humano elige, el CLI informa |
| | Modo `always|ask|off` por provider | genérico y explícito |
| | Invariante "sólo por el CLI" | el layout deja de ser contrato |
| | Test "aviso visible con exit 0" | cierra el agujero que ocultó el WARN |

---

## 10. Decisiones abiertas

1. **Retiro de `changes start --none`**: ¿se depreca ya (efecto inerte durante una versión y luego se elimina) o se conserva como marcador de intención humana sin efecto sobre el gate?
2. **Umbrales de R3** (tamaño/módulos): ¿valores por defecto del CLI o declarados por proyecto?
3. **`require`**: ¿granularidad por checkpoint con lista explícita, o por capacidad entera con excepciones?
4. **`changes status` sin argumento**: resolución por rama (recomendado) o slug obligatorio.
5. **T3 por defecto**: ¿`sync-to-provider` como recomendación inicial, o ninguna recomendación y sólo el menú?
6. **Histórico**: ¿las 14 filas se congelan tal cual o se anota a qué ledger pertenecen al migrar?
7. **Cuarto ledger**: VCS/PR (hoy repartido entre `git-pr-flow` y `worktree-cleanup`) como candidato siguiente; fuera de alcance.
8. **Guardian ODD con tier de tres valores**: implica tocar expectativas de `internal/gates/*_test.go`; ¿se hace en la misma unidad o en una siguiente?

---

*Propuesta de diseño v3. Sin cambios en el repositorio; nada aplicado.*
