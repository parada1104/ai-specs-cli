# change-ledger-t3 (ODD)

Unit T3 of the change-ledger-v2 redesign: the workstream. A worktree that is not a
change owes nothing, and the CLI tells the two apart from facts instead of from a
declaration. Master plan, design and audit live in `odd/tasks/change-ledger-v2.md`
and `odd/tasks/change-ledger-v2/`.

## Specs

- **S16** — A worktree that is not a change owes nothing, and the CLI tells the two
  apart from facts. Verbatim, master plan S16: «Hay que tener mucho cuidado con las
  tareas que crean un worktree/workstream pero no son efectivamente un change.»
- **S3** — One unit is one change: its own branch-paired plan, only the deltas it
  implements, promoted and archived before its merge.

## Tasks

- T3.1 — The shared discriminator: one collector, consumed by the write gate and by
  the reporting verb, that classifies a worktree as change or workstream from
  product-path divergence (committed **and uncommitted**) or from an artifact that
  names a change (plan, delta). Specs: S16. route: inline. commit: 3231fe0d.
- T3.2 — `ai-specs changes workstream [--reason <text>] [--end] [--json]`: report the
  facts, or record an explicit append-only decision in the plan ledger for the
  genuinely ambiguous case. Refused when the change carries an artifact. Specs: S16.
  route: inline. commit: 6d244c53.
- T3.3 — Validation: focused suites, the real-flow matrix with the four T3 scenarios,
  and the RED baseline. Specs: S16, S3. route: inline. commit: 6d244c53.

### Slice boundary

One PR, two commits: `3231fe0d` (discriminator + ledger, with its unit tests) and
`6d244c53` (verb, flow matrix, help surface, parity normalization). Together they
are ~250 authored code lines and ~450 test/doc lines, which is past the ~400-line
slice guidance; the two commits are the review boundary and the code of the two
halves is not independently deliverable (the verb exists to ask the gate's
question), so the slice ships whole.

## Verification

| Spec | Evidence |
| --- | --- |
| S16 | (a) a fresh worktree with no divergence is reported `workstream`, exit 0, and no ledger row, no card and no archive are demanded; (b) after a product commit the same worktree is a change with tier `none`; (c) a declared workstream with product commits stays a workstream until `--end`, and the declaration is append-only; (d) a declaration on a worktree that carries a plan is refused naming the rule; (e) uncommitted product divergence makes it a change again, which is the gap T2's private copy left. |
| S3 | Plan only (`light`), no delta folder — see T2's L2 and this unit's L2. |

## Log

- **L1** — «Después sigue con T3 (workstream + changes workstream) con el mismo
  mandato, pero NO lo mergees: cuando esté validado, avísame con LISTO T3-LISTO para
  revisarlo.»
- **L2** — Delivery shape: this unit ships `light` (plan only). The `plan-build-flow`
  delta stays in the master plan because `odd/changes/plan-build-odd-companion/` is
  still an active change folder declaring the same delta, exactly as T1 and T2 did.
- **L3** — The plan ledger's store: `.ai-specs/plan-ledger.jsonl` at the repository
  root, append-only, one JSON object per line, mode-agnostic. T7 adds the other two
  ledgers over the same file; T3 adds only the `workstream` and `workstream-end`
  events, because they are the ones the discriminator has to read.
- **L4** — Réplica del hueco que T2 dejó anotado: el discriminador privado del gate
  solo miraba el diff **commiteado**. T3 lo sustituye por el colector compartido, que
  además consulta la divergencia sin commitear. El chequeo extra de git cuesta una
  llamada y solo se paga cuando nada más dice «change».
- **L5** — Contrato del verbo, decidido así: `--root` obligatorio (igual que sus
  hermanos `archive` y `start`, y nunca el cwd del proceso); `--reason` y `--end`
  son decisiones opuestas, así que juntos son error de uso; el reporte sin flags
  sale siempre 0 (es un reporte, no un gate), una declaración o un cierre
  rechazado sale 1 y el error de uso sale 2. `--json` imprime un objeto con
  `classification` y los hechos observados, y en declaración/cierre imprime el
  mismo objeto ya con la decisión aplicada.
- **L6** — El evento del ledger es `{"event","at","reason"?}`. T3 escribe solo
  `workstream` y `workstream-end`; un lector que encuentra una línea ilegible la
  ignora en vez de confiar en ella, y gana el último evento conocido.
- **L7** — El discriminador queda en un solo sitio, `WriteFacts.isWorkstream()`,
  alimentado por `WriteFactsFor`; el verbo lo consume con
  `WriteFactsFor(root, mode, changeFromBranch, "")`, el mismo pairing por rama que
  usa el gate, así que ambos no pueden discrepar. El hueco real que faltaba era el
  fichero **untracked**: `git diff <merge-base>` ya lee el working tree y reporta la
  edición sin commitear de un fichero rastreado; añade `planbuild.UntrackedPaths`,
  pagada solo cuando ningún otro hecho ya dice «change», y sin cambiar la semántica
  de `DiffEntries`, que el guardián comparte.
- **L8** — Superficie de ayuda: `changes workstream` entra en `help.txt` y en
  `changesUsage`, y el hueco de los goldens congelados se cubre con la normalización
  N17 —misma razón y misma forma que N14/N16— en `parityHelpSnapshot` y en
  `tests/parity/parity.py`, sin reescribir los bytes legacy.
- **L9** — El aviso de deprecación de `changes start` y su usage vuelven a nombrar
  el verbo que ahora existe (`changes workstream`), reparando el puntero que T2 L10
  punto 3 retiró a propósito mientras no hubiera verbo.
- **L10** — `.ai-specs/plan-ledger.jsonl` es estado de ejecución por checkout y queda
  untracked; su política de ignore pertenece al dueño del store de planes (T6/T7),
  no a T3. No es una ruta de producto, así que no altera la clasificación de ningún
  worktree.
- **L11** — RED observado antes de implementar: el verbo no existía
  (`ai-specs changes: unknown subcommand 'workstream'`, exit 2), un fichero de
  producto untracked dejaba el worktree como workstream, y una declaración en el
  ledger no cambiaba nada. Los casos de `gates` y los cuatro escenarios de flujo
  fallaban por esas tres razones y pasan desde la implementación.
- **L12** — Validación observada: `go build ./...` y `go vet ./...` limpios,
  `go test ./...` verde en todos los paquetes (incluidos `gates`, `cli` y los tests
  de flujo con el binario construido), y la matriz de flujo real con el binario de
  la rama en `/private/tmp/change-ledger-v2/t3/flow.sh`: los cinco escenarios de
  S16 y los tres rechazos (declaración con plan, declaración con delta, `--end`
  sin declaración) se comportan como el plan exige. El verificador independiente
  (`gentle-ai-verify`) no está disponible para worktrees enlazados —`probesRun: 0`,
  «unsupported repository: ordinary single-root Git directory required»—, así que
  esa evidencia es la del padre, no la de un tercero, y se reporta como tal.
- **L13** — `./tests/validate.sh` falla en su tramo de paridad en este entorno, y
  **no por T3**: el mismo arnés sobre una copia limpia de `f37a782d` falla con el
  mismo conjunto de 19 fixtures y los mismos 38 deltas (`.ai-specs.lock` y los
  árboles `ai-specs/recipes/**` que el lado Go no materializa), y con `TMPDIR`
  apuntando a una ruta sin symlink los fallos bajan de 28 a 19 en ambos árboles. El
  tramo Go de `validate.sh` (build, vet, tests) sí queda verde.
