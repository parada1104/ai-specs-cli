# change-ledger-t4 (ODD)

Unit T4 of the change-ledger-v2 redesign: the invisible Pi advisory (S13). Master
plan, design and audit live in `odd/tasks/change-ledger-v2.md` and
`odd/tasks/change-ledger-v2/`.

## Specs

- **S13** — The invisible Pi advisory is fixed inside this redesign. Verbatim,
  master plan: «el aviso invisible del wrapper de Pi se arregla dentro del
  rediseño».
- **S15** — Delivery shape. Verbatim: «partiendo el rediseño en unidades
  entregables y ordenadas (cada una con su PR, < ~1000 líneas, con su prueba de
  flujo real y su RED)».

## Tasks

- T4.1 — The generated pi/omp extension relays the advisory a hook writes on
  stderr with exit 0: one visible line, best effort, and the blocking path
  (`res.status === 2`) unchanged. Template: `internal/sync/hooksrender.go`,
  `hooksRenderPiExt` (shared by pi and omp). Specs: S13. route: inline.
  commit: 7af06bf1.
- T4.2 — Tests: the RED that pins the relay in the rendered wrapper for both
  adapters, plus the producer contract the wrapper consumes (`plan-build-gate
  --explain` writes an advisory on stderr with exit 0, already pinned by T2's
  flow matrix). Specs: S13. route: inline. commit: 7af06bf1.
- T4.3 — Validation: focused suites, real flow with the built binary (render a
  project and inspect the generated wrapper for pi and omp), and the RED
  baseline. Specs: S13, S15. route: inline. commit: 7af06bf1.

## Verification

| Spec | Evidence |
| --- | --- |
| S13 | (a) the rendered `.pi/extensions/worktree-flow-worktree-gate.ts` and `.omp/extensions/worktree-flow-worktree-gate.ts` contain the advisory relay (`res.status === 0`, `customType: "ai-specs-advisory"`, `display: true`) and cut a multi-line message to one line; (b) `res.status === 2` still returns `block: true` with `res.stderr`, so an advisory is never read as a block; (c) `plan-build-gate --explain` really does produce that advisory: exit 0 with the line on stderr; (d) the relay is best effort — a failing `sendMessage` cannot change the hook's decision. |
| S15 | One PR for the unit, well under ~1000 lines, with the real-flow check and the RED in `## Log`. |

## Log

- **L1** — Mandate, verbatim: «Sigue con el plan maestro: T4 (aviso de Pi visible)
  y luego T5, T6… en orden, una unidad = un worktree nuevo desde development + un
  PR, con el mismo mandato (validar con flujo real y RED, revisión nativa antes del
  merge; si la revisión se traba en el paso de validación como en T2, me avisas con
  LISTO REVISAR-<unidad> y reviso yo; PR, merge, `ai-specs sync` del repo con su PR).
  Sin release hasta T12, salvo que una unidad lo necesite. Guarda en Engram antes de
  cada merge (el hook retira el worktree). Cuando el contexto pase de ~45 %, cierra
  en Engram y avísame con LISTO RELEVO para abrir sesión nueva. Avísame al cerrar
  cada unidad con una línea LISTO <unidad> (p. ej. LISTO T4).»
- **L2** — Defect, located: `hooksRenderPiExt` relays only `res.stdout` on exit 0
  (as a hidden `pi.sendMessage` with `customType: "ai-specs-worktree-gate"`,
  `display: false`). A hook that writes its advisory on **stderr** with exit 0 —
  `plan-build-gate --explain`, the tracker advisory — is dropped, and the caller
  sees nothing. That is the invisible advisory of S13.
- **L3** — Scope: pi and omp share `hooksRenderPiExt`, so both are fixed by one
  template change. The generated OpenCode plugin (`hooksRenderOpencode`, non-2 exit
  → `return`) and the Cursor shell shim have no message channel in the generated
  code, so they are **out of this unit** and recorded here as a decision for a
  later one (a channel must be chosen from the host's own API, not invented).
- **L4** — Handover state: the RED is written and observed (the rendered wrapper
  lacks `const advisory = res.status === 0 ?`), the implementation is not started,
  and the branch is clean apart from that untracked test file.
- **L5** — Implemented in `hooksRenderPiExt` (`internal/sync/hooksrender.go`), the
  template pi and omp share: after the untouched `res.status === 2` block and the
  S6 stdout notice, the extension relays `res.stderr` on exit 0 as one visible
  line — `const advisory = res.status === 0 ? (res.stderr || "").split("\n")…`,
  first non-empty trimmed line, `customType: "ai-specs-advisory"`,
  `display: true`, delivery best effort (`try`/`.catch`) so a host that refuses the
  message cannot change the decision. No new channel invented: it is the same
  `pi.sendMessage` the S6 notice already used.
- **L6** — Decision, frozen oracle: `testdata/hooksrender_golden.json` is the
  snapshot the retired Python renderer produced and that module no longer exists,
  so it cannot be regenerated. The differential test therefore moved only the 16
  frozen pi/omp adapter bodies, mechanically and by exactly one block each (the
  `ai-specs-advisory` relay); every other byte of the bundle is untouched, so the
  parity it still proves is real. The S6 test
  (`TestCLIServedPiExtensionDeliversMessageOnAllow`) pinned the exit-2 block against
  the stdout notice contiguously, which is why the advisory goes after that
  notice instead of between them: the pinned expectation stays valid rather than
  being rewritten to fit a new layout.
- **L7** — Validation, real flow (scratch `/private/tmp/change-ledger-v2/t4`):
  RED observed before the fix, GREEN after (`go test ./internal/sync/`, then
  `go test ./...` clean). Built `./cmd/ai-specs`; `ai-specs init` + `recipe add
  worktree-flow` + `sync` on a scratch git project rendered
  `.pi/extensions/worktree-flow-worktree-gate.ts` and the `.omp` twin with the
  relay in place. Then the generated adapters were **executed** with Node 24 type
  stripping against a fake `$AI_SPECS_HOME/dist/ai-specs`: exit 0 with a
  multi-line stderr produced exactly one message
  `{customType: "ai-specs-advisory", content: "plan-build-gate: tier light;
  refutations none", display: true}` for pi and omp; a `sendMessage` that throws
  left the decision at `undefined` (allow); exit 2 still returned
  `{block: true, reason: <raw stderr>}` with no advisory. Producer side, real
  binary: `plan-build-gate --explain` on a scratch odd-mode repo, feature branch,
  no plan → rc 0, empty stdout, `plan-build-gate: tier none; refutations none` on
  stderr, which is exactly the line the wrapper now relays.
