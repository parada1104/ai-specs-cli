# Estudio: ODD (Gentle Shell) frente a plan-build-flow (ambos modos)

Fuentes y abreviaturas:
- `routing` = `gentle-pi/contracts/review-provider-contract-mirror/v1.6.0/bundle/routing/pi.md` (el bloque ODD normativo, que se renderiza literal en el prompt).
- `orch-rdd` = `.../bundle/orchestration/pi.md` (el contrato de revisión nativa).
- `orc*.md` = `gentle-pi/assets/orchestrator*.md`.
- `AS` = la worktree `plan-build-openspec-odd-align`.

**Estado del código leído:** HEAD `82101c05` (PR #590). Mientras hacía este estudio, el árbol de trabajo se guardó en `stash@{0}` con el mensaje *"tiers D1+D2 (config opcional, full fuera) - pausado por Robert, pendiente de su estudio de ODD"*. Ese stash quita `full` y deja `config.yaml` como opcional, pero todavía no añade un tier "sin ceremonia". Las citas `file:line` son de HEAD, salvo que diga otra cosa.

---

## 1. ODD de punta a punta

### Protocolo (obligatorio, en orden, en cada petición; `routing:5-13`)
1. **Authorize.** Investigar, explicar, revisar o proponer es solo lectura. Si no está claro que haya autorización, una sola pregunta y se espera (`:7`).
2. **Explore.** Proporcional a la petición. No se delega lo que igual se va a leer (`:8`).
3. **Resolve uncertainty.** Investigación opcional solo ante una incertidumbre con nombre. Como máximo un *assumption challenge*. Se pregunta solo si queda una decisión humana (`:9`).
4. **Classify.** *"Size the task by the orchestrator's Task Size section; its criteria and resume test alone decide small or large"* (`:10`).
5. **Track before the first write.** Solo en tareas grandes: `odd/tasks/<feature>.md`, más el espejo en Engram y el `todo`, *"without asking permission"* (`:11`).
6. **Implement task by task.** Handoff por referencia (`Spec: odd/tasks/<f>.md, T2, S3-S4`), test-first y work-unit commits de ~400 líneas. *"push, pull request creation, and merge remain the user's decisions"* (`:12`).
7. **Close.** Resultado verificado, cada check fallido o pendiente, siguiente paso y la línea Risk (`:13`).

### Task Size: cómo se decide (`routing:21-29`, `orchestrator.md:17-25`)
Una tarea es **small** si se cumplen las tres:
1. **Understood:** *"the outcome is specified, what and where to change is known, and no product or design decision is open; prove it within one bounded read batch (at most 3 calls, approximately 10k tokens)"*.
2. **Contained risk:** no toca ningún ítem de alto riesgo.
3. **Resumable from the diff:** *"if the session stopped now, someone could finish the work from the original request and `git diff` alone"*.

Además:
- *"The number of files, commands or tests, fixes, or a requested todo list never decides size."*
- **Large** *"only when the resume test fails: several sessions, waiting on something external, separate deliverables the user wants apart (never several features asked at once), or requirements too long to survive compaction"* (`routing:27`).
- Camino small: *"No explore, worker, or verifier; no feature document, mirror, or commits unless the user asks"* (`routing:29`).
- *"small work creates no durable task artifacts"* (`routing:71`).
- Si el presupuesto de lectura se excede (más de 3 lookups), se dispara el Mapping trigger y el tamaño se re-evalúa (`routing:47`).

**Zona gris que el contrato deja abierta:** una tarea de alto riesgo que pasa el resume test no es small, porque falla el criterio 2, pero tampoco es large, porque large depende solo del resume test. En la práctica se trabaja inline, con línea Risk y verificador, y sin documento. **Riesgo y tracking son ejes distintos.**

### Alto riesgo (cita literal, `routing:31`)
> *"**High risk**: a mistake would be hard to detect, hard to undo, or reaches beyond the change. It touches (1) data or irreversible effects that cannot be rebuilt: migrations, changing or deleting existing stored data, persisted format changes (not saving new records, nor a dev or test database a seed or backup rebuilds); (2) security: auth, permissions, credentials, secrets, guards, sandbox; (3) contracts others consume: changing or removing public API, CLI flags, config formats, published exports, or mirrored prompts or contracts others already consume (not adding a flag, command, or optional field; requested changes are not high risk by themselves; unrequested breaks in shared code are); (4) concurrency: locks, races, async ordering; (5) delivery or environment: installers, release, CI, deploy, dependencies; (6) no test would catch a regression in what changes. Count "unclear" as high only when a bounded look cannot tell whether (1)-(5) apply."*

Toda modificación de código cierra con `Risk: item N (reason)` o `Risk: none` (`routing:33`).

### Documento de feature (`routing:11`, `:73`; `orchestrator-memory.md:7-12`)
El orden es fijo:
1. Cabecera de 2-3 líneas: objetivo, rama, estrategia de entrega y test runner.
2. `## Specs` con `S1..Sn`, que citan *"the user's exact strings… verbatim… never summarize or reword"* y no añaden requisitos que nadie pidió.
3. `## Tasks`, una línea por tarea: ID, `S#`, ruta, estado y commit o evidencia RED/GREEN.
4. `## Log` al final, con `L1` = la petición original verbatim.

Un cambio de requisito añade una entrada al Log, reescribe solo el `S#` afectado y reabre solo su tarea (`:74`). Solo se marca lo que se observó con prueba. **ODD no define sección `## Verification`.** La evidencia va en Tasks y en Log, y el verificador devuelve *"a verdict per `S#`"* en su informe (`routing:12`).

### Verificación
- Test-first por defecto, *"one RED test per requested rule"*, más un *differential check* contra la base en un `mktemp -d` (`routing:78`).
- Verificador independiente *"only a change whose Risk line names a high-risk item"*. Ni el tier nativo ni un assess fallido añaden uno (`routing:48`; `orchestrator-verification.md:18`).
- El spot check del padre, que re-ejecuta un comando reportado, es obligatorio en todos los tiers (`verification.md:18`).
- Se permite una sola corrección por blocker (`verification.md:42`).
- *"never invent acceptance gates the user did not ask for"* y *"Before adding or retaining an auxiliary gate, name its acceptance criterion, concrete threat, or authoritative contract"* (`routing:72`; `verification.md:32`).

### Commits y work units
- *"Commit work units in reviewable slices of about 400 authored changed lines… a unit whose Risk line names a high-risk item gets its own commit"* (`routing:12`).
- Las 400 líneas son *"only as a planning heuristic… not a task acceptance criterion, hard cap, counter-trigger, automatic stop, forced split, or RDD trigger"* (`routing:72`).
- El camino small no hace commits salvo que el usuario los pida. Commitear fuera de ODD large requiere petición explícita (`orchestrator.md:82`).

### Entrega
- Push, PR y merge los decide el usuario (`routing:12`, `orch-rdd:71`).
- Se elige una estrategia por feature: `ask-on-risk` (default), `auto-chain`, `single-pr` o `exception-ok`. Pasadas unas 400 líneas, se aplica solo cuando el usuario pide publicar (`routing:80`). El menú tiene tres opciones: feature-branch-chain, stacked-to-main y single PR como *least recommended* (`orchestrator-tracking.md:41-49`).

### RDD: revisión nativa
- **Interruptor del usuario**, encendido por defecto, se apaga con `gentle-ai review mode disable` y nunca se toca automáticamente (`routing:85-92`).
- **Candidato** = un work-unit commit o un PR slice, *"never… the accumulated feature branch"* (`routing:13`).
- **Tras cada work-unit commit**: `assess --base-ref <last reviewed boundary> --committed-only`. El resultado se lee así (`routing:79`):
  - `review_due=true (high_risk)` → se revisa.
  - `passive` → avanza el boundary.
  - `under_budget` (medium) → queda pendiente hasta un commit de alto riesgo o hasta que el usuario lo pida.
  - Un assess fallido nunca baja el tier.
  - **Ojo:** `docs/readme-reference.md:31` está desfasado: dice que medium "defers to the PR slice… reviews at slice close".
- **Escalar:** `--escalate-item N` sube el tier, nunca lo baja (`routing:81`).
- **Consentimiento:** se pide por candidato, en el envelope `consent/v3`. *"Global RDD enabled permits review; it never grants consent for this candidate"* (`orch-rdd:32`).
- **Lentes:** de 0 a 4 (risk, resilience, readability, reliability), según la consecuencia y no según el número de ficheros (`orch-rdd:11-18`).
- **Presupuestos:**
  - `lens_context_budget_exceeded` → *"Terminal: reduce the candidate scope and start a new transaction, or `D`"* (`orch-rdd:57`).
  - Corrección: `min(200, ceil(original_changed_lines / 2))` y una sola transacción (`readme-reference.md:916, 932`).
- **Entrega:** *"A review outcome is informational and never authorizes delivery"*. Con RDD encendido, el guard previo a la entrega bloquea push o PR hasta que el rango esté evaluado y la revisión debida resuelta (`orch-rdd:71`).
- **Lección de #590:** el PR chocó con `lens_context_budget_exceeded` (4.611 líneas, 38 ficheros; `AS/odd/tasks/plan-build-openspec-odd-align.md:282, 285`) porque el assess se hizo sobre la rama acumulada y no commit a commit, que es justo lo que ODD prohíbe.

### Memoria / Engram (`orchestrator-memory.md:7-24`)
- El fichero es canónico. El espejo Engram (`odd/<feature>/tasks`) y el `todo` son proyecciones que se refrescan en cada transición de tarea.
- *"never author comparison or reconciliation scripts or gates for them"* (`:18`).
- Si Engram no está disponible, se marca "mirror pending" y no se bloquea nada.
- Retomar: `mem_context` → `mem_search` → `mem_get_observation` → fichero, y reconciliar (`:20`).

---

## 2. Juicio del agente frente a lo verificable objetivamente

| Decisión ODD | Naturaleza | Qué podría ver un CLI |
|---|---|---|
| ¿Hay autorización de cambio? | Juicio | Nada |
| Understood (≤3 llamadas, ~10k tokens) | Juicio con señal de runtime: el aviso de exploración salta a partir de 3 lookups o 40 KB, *"guidance, not a gate"* (`readme-reference.md:781`) | Nada (es sesión, no repo) |
| Riesgo contenido (ítems 1-6) | Juicio. RDD `assess` calcula un tier nativo en Go | Rutas que tocan CI, deps o migraciones son señales objetivas, pero solo indicios (el ítem 3 depende de si "otros consumen") |
| Resume test (small/large) | **Juicio puro** | Nada fiable: los conteos están prohibidos explícitamente |
| Documento existe y tiene forma | Verificable | Fichero regular, secciones, IDs `S#`/`T#` |
| `S#` verbatim, sin requisitos inventados | Juicio | Nada |
| `## Tasks` enlaza `S#` declarados | Verificable | Sí (el parser ya lo hace en openspec) |
| Tarea marcada solo con prueba observada | Mixto | Que el sha sea ancestro de HEAD, o que exista evidencia; no que la prueba sea adecuada |
| Línea `Risk:` | Presencia verificable, contenido de juicio | Solo si se registra (trailer) |
| ~400 líneas | Medible, pero **prohibido como gate** | Solo informativo |
| Commit propio para unidad de alto riesgo | Juicio | Nada |
| Tier RDD, `reviewDue`, consumo de revisión | **Objetivo** (lo emite el proveedor Go, no el agente) | Lo leería el CLI solo integrándose con `gentle-ai` |
| Push, PR, merge | Decisión humana | Nada; como mucho capas de CI |
| Espejo Engram sincronizado | ODD prohíbe convertirlo en gate | Nada |

**Conclusión:** ODD tiene una sola decisión objetiva fuerte, el tier RDD, y está fuera del agente. Small/large es juicio por diseño.

---

## 3. plan-build-flow hoy (HEAD `82101c05`)

### Resolución de modo (común)
- `planbuild.Decide`/`Resolve`: el modo explícito gana. En `auto`, existir `<git-common-dir>/gentle-ai/` da `odd` (`internal/planbuild/mode.go:36-63, 87-93`).
- Identidad del change = último segmento de la rama (`pair.go:30-42`).
- La escalera es: rama emparejada → único change activo → bloqueo por ambigüedad, con `--change` como override (`pair.go:150-175`).

### Hook `plan-build-gate` (pre-tool-use)
- Solo vigila directorios de producción, por defecto `src lib catalog` (`internal/cli/planbuildgate.go:57, 180, 285-298`). En este repo `internal/` y `cmd/` no están vigilados salvo que se defina `PLAN_BUILD_GATE_PATHS`.
- `openspec/changes/**` y, en odd, `odd/**` siempre se pueden escribir (`:174-179`). Antes se califica el checkpoint `work-start` del ledger (`:189`).
- **odd:** cualquier escritura en producción exige `odd/tasks/<rama>.md` como fichero regular contenido en el repo. Con HEAD detached, bloquea (`:192-195, 230-242`; `pair.go:186-201`). **Confirmado: no hay camino small.** En `main` exigiría `odd/tasks/main.md`.
- **openspec:** exige que exista el `tasks.md` del change emparejado; si la rama no empareja y hay un único change activo, sirve ese (`:197-200`). Solo comprueba que exista, no la gramática.

### `gates premerge`
La CLI está en `internal/cli/gates.go:81-226`. En odd, un slug posicional se ignora y se usa la rama (`:194-196`).

**openspec**, `CheckPremerge` (`internal/gates/gates.go:142-165`):
- Bloquea si la carpeta del change sigue activa. El archivo con fecha debe existir y tener contenido: plan, delta o `archived.md` (`archive.go:23-154`).
- Tier tomado de `Depth:`; **sin `Depth:` es `standard`** (`gates.go:36-41`). Acepta `light|standard|full` (`:50`).
- Gramática del plan en todos los tiers: `## Specs`, `## Tasks` como tabla con cabecera fija `| ID | Spec | Tarea | Ruta | Commit |` (`tasks.go:83-116, 212`). `## Log` obligatorio desde standard (`gates.go:100`).
- Delta obligatorio desde standard (`gates.go:80-92`).
- Celdas `Commit` deben ser ancestros de HEAD (`commits.go`).
- `## Verification` una fila por `S#`, con comando en backticks y sin fallos registrados (`evidence.go:180-245`). Light queda exento.
- `openspec/config.yaml` obligatorio en standard/full, más el bucle de frescura del runner (`config.go:137-148`).
- Paridad de promoción en standard/full (`parity.go:13`).

**odd**, `CheckPremergeOdd` (`gates.go:181-212`):
- El plan debe existir. **No se parsea su gramática** ni se califican Verification ni Commits.
- No puede quedar carpeta activa. **El archivo con fecha es siempre obligatorio**, sin exención por `Depth:` (`:176-177, 205-207`).
- Paridad del delta, tanto activo como archivado.

### Resto
- **`ai-specs changes archive`:** sin delta, crea la carpeta con fecha y el marcador `archived.md` (`internal/cli/changes.go:229-277`).
- **CI:** `.github/workflows/pr-gate.yml:17-19, 67-69` corre en **todo** PR contra `development` con `--change "$GITHUB_HEAD_REF"`. El modo está declarado `odd` en el manifiesto (L18).

### Prosa desfasada respecto al código
- `SKILL.md:231` dice que el archivo copia `tasks.md` y que un plan sin `Depth:` está exento. El código usa `archived.md` y no tiene exención.
- `README.md:19-21` sigue exigiendo `proposal.md` en light y standard; `checkMinimums` ya no lo exige.
- El SKILL de HEAD conserva `full` y su cadena de fases (`:93, 132-159`). El stash lo quita, pero en Go `full` sigue vivo (`tiers.go`, `evidence.go:153-177`).

---

## 4. Brechas y choques

1. **El camino small choca con el hook en modo odd.** ODD: small *"creates no durable task artifacts"*. El hook bloquea la primera escritura en producción sin `odd/tasks/<rama>.md` (`planbuildgate.go:237-241`). Con ODD obedeciendo su contrato, cada tarea small en `src/lib/catalog` se bloquea, y la única salida es crear un documento que ODD prohíbe para esa tarea.
2. **El mismo choque, en CI y premerge, para todo PR.** `CheckPremergeOdd` exige plan y archivo (`gates.go:189-193, 205-207`), y `pr-gate` corre en todo PR. Un PR small sale rojo siempre. En openspec, sin change, `ResolveChange` falla: o bien `--change` apunta a un archivo inexistente (BLOCKED), o bien sale el error de uso.
3. **El archivado en odd es ceremonia que ODD no tiene.** ODD no conoce "archivar": el documento de feature se queda en `odd/tasks/`. El marcador `archived.md` sin delta no aporta información; la información ya está en el plan, que no se mueve. Además, choca con la decisión de openspec en #590: *"archive obligatorio cuando hay carpeta de change"*.
4. **Tiers frente a Task Size: ejes distintos mezclados.** El clasificador usa tamaño y alcance: *"single-file tweak"*, *"multi-file"* (`SKILL.md:93-95, 118`). ODD dice que el número de ficheros *"never decides size"*. ODD separa tres ejes:
   - tracking (resume test),
   - verificación (riesgo),
   - y, en ai-specs, regla observable (delta).

   Light frente a standard es en realidad "¿cambia una regla?", no "¿es más grande?".
5. **Sin `Depth:` se aplica el máximo de ceremonia** (`gates.go:36-41`). Es lo contrario de lo orgánico: la ausencia de declaración debería significar "lo mínimo que los artefactos demuestran".
6. **La autorización del plan no encaja.** En openspec, *"The final plan always requires an explicit accept, adjust, or stop"* (`SKILL.md:195`) y el plan se detiene antes de construir. En ODD la autorización es el paso 1 de la petición y el documento se crea *"without asking permission"*. En odd, §4.1 ya lo respeta; openspec mantiene la parada.
7. **Asimetría de verificación por `S#`.** En openspec se califica `## Verification`, una sección que ODD no tiene. En odd no se califica nada. Por su parte, la tabla de Tasks que exige el parser (cabecera en español) no es el formato de ODD ("one line per task"): calificar el documento ODD con `ParsePlan` pelearía con el prompt de Gentle.
8. **El SKILL convierte las 400 líneas en orden.** *"split it into one commit per work unit"* (`SKILL.md:409-410`). ODD lo declara heurística, *"not… forced split"*. El CLI no lo califica, lo cual está bien (R2 rechazado), pero la prosa contradice el contrato.
9. **Commits y revisión.** plan-build exige un commit por fila de Tasks; el camino small de ODD no commitea salvo petición. Además, plan-build no menciona assess por commit: #590 lo pagó con `lens_context_budget_exceeded`. Además, `assess` siempre compara `baseRef..HEAD`, así que un rango interior no se puede revisar (L19): las rebanadas tienen que nacer encadenadas.
10. **Riesgo y runner.** plan-build no tiene línea `Risk:` ni noción de ítems; el único proxy es el tier. ODD lleva el runner en la cabecera del documento; en odd no hay `config.yaml`, así que no hay frescura del runner en ese modo.

---

## 5. Recomendaciones

### Principio
El CLI **no puede probar small**: el resume test y "understood" son juicio, y los conteos están prohibidos. Lo honesto es invertir la carga:
- **la ausencia de plan es una declaración válida** (es lo que ODD produce);
- el CLI **solo la refuta** con contradicciones objetivas del diff;
- el tier se **deriva de los artefactos presentes**, no de una línea declarada.

Así se evita inventar checks débiles (no "presencia de Log", no conteo de líneas).

### 5.1 Cómo reconocer "sin ceremonia" de forma determinista

**Definición, técnica:** un change es `none` si en la rama no existe `odd/tasks/<c>.md` ni `openspec/changes/<c>/` (activo o archivado) ni delta.

En premerge y CI, `none` pasa con veredicto `OK (none, change <c>)` **salvo refutación objetiva**. Refutaciones posibles:
- **R-a. Toca specs canónicas** (`odd/specs/**`, `openspec/specs/**`) sin delta: es un bypass de la promoción. Bloquea. *Técnico.*
- **R-b. Toca rutas de alto riesgo declaradas por el proyecto**, por ejemplo una lista en `ai-specs.toml`: `.github/**`, `go.mod`, `migrations/**`, installers. Es determinista porque la lista es explícita y del proyecto, no inferida. Aun así, ODD dice que el riesgo no implica tracking, solo verificador. Por eso la consecuencia es una **decisión de Robert**:
  - (i) exigir light, porque el plan es el único sitio de plan-build donde vive la evidencia de verificación; o
  - (ii) exigir solo un trailer `Risk: item N` con evidencia.

  Recomiendo (i) en openspec, que no tiene el verificador de ODD, y (ii) en odd.
- **No usar** conteos, número de commits ni antigüedad de la rama. ODD los excluye y producirían falsos positivos.

**Hook pre-tool-use:** en el momento de la primera escritura no hay diff, así que no puede distinguir small de large.
- **odd (recomendado, técnico):** quitar la exigencia de plan en el hook. ODD ya tiene su paso 5 ("Track before the first write") y la refutación vive en premerge y CI. Mantener el hook solo para escrituras bajo `odd/specs/**`.
- **openspec (decisión de Robert):** o se mantiene el hook pero aceptando una declaración local `git config branch.<b>.aiSpecsCeremony none`, escrita por un verbo `ai-specs changes start --none`, que no viaja ni es un artefacto; o el hook se relaja igual que en odd. Recomiendo la declaración local: en openspec no hay prompt ODD que haga cumplir el Task Size, y el hook es lo único que obliga a clasificar.

### 5.2 Mapear tiers a Task Size

| Tier openspec | Equivalente ODD | Se deriva de (determinista) | Exige |
|---|---|---|---|
| **none** | small | sin plan, sin carpeta, sin delta | nada; refutaciones R-a/R-b |
| **light** | large sin regla observable | plan presente, sin `specs/` | plan con `## Specs` + `## Tasks`; `## Log` opcional; archivo solo porque hay carpeta |
| **standard** | large con regla observable | plan + delta | plan + `## Log` + `## Verification` por `S#` + paridad + archivo |

- **Que el tier salga de los ficheros** (técnico). `Depth:` pasa a ser redundante o, como mucho, un check de coherencia: si se declara y no coincide con los artefactos, bloquea. Se elimina el default "sin Depth → standard" (`gates.go:36-41`) y gran parte del "depth conflict ask" del SKILL §2.
- **El criterio para pasar de `none` a plan es el de ODD**, copiado tal cual al SKILL: understood + contained risk + resume test, y *"line/file counts never decide"*. Hay que borrar las señales de tamaño de `SKILL.md:93-95, 115-119` (técnico, prosa).
- **Caso límite, decisión de Robert:** un cambio small que cambia una regla observable, por ejemplo un default documentado. ODD dice small; ai-specs necesita el delta para que las specs canónicas sigan siendo ciertas. Propuesta: permitir **delta sin plan**. `archiveHoldsChange` ya acepta un archivo que solo tenga delta (`archive.go:151-154`); en odd habría que dejar de exigir plan cuando solo hay delta (`gates.go:189`).

### 5.3 Archivo
- **Decisión de Robert:** alinear odd con la regla de openspec, *"archive obligatorio solo cuando hay carpeta de change"*. Hoy odd lo exige siempre (S10). Recomiendo alinearlas: sin `odd/changes/<c>/` no hay nada que mover, y `archived.md` no aporta información que el plan no tenga.

### 5.4 Verificación, commits, revisión
- **odd:** no calificar la gramática del documento ODD con `ParsePlan`. Su formato lo gobierna el contrato de Gentle, y una cabecera de tabla fija en español chocaría. Como mucho, comprobar que es un fichero regular, que es lo que ya se hace (técnico).
- **openspec:** mantener `## Verification` por `S#` solo en standard. Es el sustituto razonable del verificador de ODD en un modo sin ODD (ya decidido).
- **SKILL §7.3:** reescribir la regla de 400 líneas con el texto de ODD (heurística, sin división forzada) y dejar los commits por tarea solo para light/standard. `none` no commitea nada extra (técnico, prosa).
- **Revisión:** añadir al SKILL, en ambos modos cuando RDD está activo, *"assess after each work-unit commit with `--committed-only` from the last reviewed boundary"*, y entregar en rebanadas encadenadas desde el inicio. No duplicar el guard de entrega de RDD en ai-specs: ODD ya bloquea push y PR sin assess (`orch-rdd:71`) (técnico).

### 5.5 Limpieza técnica, sin decisión de producto
- Alinear `SKILL.md:231` y `README.md:17-21` con el código.
- Completar en Go la retirada de `full` que el SKILL del stash ya describe (`tiers.go`, `evidence.go:153-177`, `criteria.go:126-180`).

### Resumen de decisiones para Robert
1. Consecuencia de tocar rutas de alto riesgo en `none`: plan light o trailer `Risk:` (5.1 R-b).
2. Hook en openspec: declaración local `--none` o relajarlo como en odd (5.1).
3. Small con regla observable: permitir delta sin plan (5.2).
4. Archivo en odd solo si hay carpeta de change (5.3).
5. Lista de rutas de alto riesgo en `ai-specs.toml`: sí o no, y quién la mantiene.

Todo lo demás (quitar el plan del hook en odd, tier derivado de artefactos, veredicto `none` en premerge y CI, prosa, limpieza de `full`) es técnico.
