# ledger-live-cleanup (ODD)

Post-merge worktree cleanup must work again, and the tracker ledger must actually
record each change in the flow that runs: bind at `apply-start`, in both ODD and
OpenSpec modes, with a missing entry no longer silent. Worktree
`.worktrees/ledger-live-cleanup`, branch `change/ledger-live-cleanup`, base
`development@75daa44e`. No commit, push or PR in this task.

## Specs

- **S1** — Post-merge cleanup must work: diagnose why it does not run today.
  Verbatim: «Limpieza post-merge: diagnostica por qué no corre.» The six merged
  worktrees must become removable by a current CLI, and the dead end must not be
  silent: an installed CLI that cannot serve the cleanup verb must say so.
- **S2** — The fix belongs to the CLI source, never to the global installation.
  Verbatim: «arréglalo en el CLI (no parchees la instalación global)». Nothing
  under `~/.ai-specs` or `~/.local/bin` is edited, and the repository's own
  `.git/hooks/post-merge` is not hand-patched.
- **S3** — The ledger must be active: every change is registered automatically in
  the flow that actually runs. Verbatim: «Activar el ledger: que cada change quede
  registrado automáticamente en el flujo que realmente corre (p. ej. bind en
  apply-start)».
- **S4** — Both planning modes are covered. Verbatim: «en ambos modos ODD y
  OpenSpec (hoy la evidencia solo lee openspec/changes)».
- **S5** — A missing ledger entry must stop passing silently. Verbatim: «que su
  ausencia deje de pasar en silencio».
- **S6** — The grade-purity defect is fixed when it lives in the same code.
  Verbatim: «Incluye el arreglo de pureza del grade que encontraste si cae en el
  mismo código».
- **S7** — The remaining audit findings stay out of scope. Verbatim: «lo demás de
  tu auditoría queda para después».
- **S8** — Evidence is focused tests plus one real-flow test driven by a binary
  built from this branch. Verbatim: «tests focalizados más una prueba del flujo
  real con el binario compilado desde la rama (apply → merge simulado en repo de
  prueba → fila en el ledger y worktree retirado)».
- **S10** — The cleanup fix must not depend on shipping the release first: the
  design has to survive version skew. Verbatim: «el diseño no debería depender de
  eso: el hook no puede volver a fallar en silencio por desfase entre el binario
  instalado y el que generó el hook. Que detecte que falta el verbo (o la versión
  mínima) y dé un mensaje accionable, o que tenga un fallback razonable; cúbrelo
  con un test.» A release that ships `worktree-cleanup` is authorized, but only as
  the final step of this unit, after the real-flow tests, the PR and the merge,
  and never as a precondition of the design.
- **S11** — The merge strategy is the human's choice: nothing in the flow may
  impose or assume one. Verbatim: «la estrategia de merge la elige el usuario, no
  el CLI ni el escenario, y el ledger y la limpieza deben detectar el merge sea
  cual sea el tipo.» Merge detection and the ledger close must work for a merge
  commit (`--no-ff`), a fast-forward, a rebase and a squash, and the scenario must
  run all four. A strategy that cannot be detected reliably is reported with
  evidence instead of forced.
- **S12** — Cleanup must not depend on a local merge. Verbatim: «añade un
  disparador de limpieza que no dependa del merge local, para que squash y los
  merges hechos en GitHub cierren la fila y retiren el worktree igual que
  merge/ff/rebase (p. ej. al actualizar development por pull/checkout, o en
  ai-specs sync; elige tú el diseño y justifícalo)». The scenario must cover it:
  a squash merged remotely and a simulated provider merge, both closing the row
  and removing the worktree with no manual `worktree-cleanup`, with its RED.
- **S13** — An ODD change without a `## Tracker` section stays a strong advisory.
  Verbatim: «Change ODD sin sección Tracker: se queda en aviso fuerte, sin
  bloquear.»
- **S9** — No existing worktree is deleted, no ledger backfill, no delivery.
  Verbatim: «No borres los 6 worktrees existentes ni hagas backfill del ledger, ni
  commit/push/PR: detente con el diff validado y reporta qué cambiaste, evidencia
  de las pruebas y cualquier decisión de producto pendiente.»

## Tasks

- T1 — Diagnose the cleanup dead end with citations (S1). route: inline. commit: none.
- T2 — Fix the cleanup path in the CLI: verb-capability guard and actionable
  failure instead of a silent `unknown command` (S1, S2). route: inline.
- T3 — Bind the ledger at `apply-start` in the flow that runs, for ODD and
  OpenSpec identities (S3, S4). route: worker.
- T4 — Make a missing ledger entry visible at the merge/review checkpoints
  (S5). route: worker.
- T5 — Make the grade path pure again (S6). route: inline.
- T6 — Focused tests for T2, T3, T4, T5 (S8). route: inline.
- T7 — Real-flow test: build the branch binary, apply, simulate the merge in a
  scratch repo, assert one ledger row and the worktree removed (S8). route: inline.
- T8 — Report diff, evidence and open product decisions; stop before delivery
  (S9). route: inline.

## Log

- **L1** — «Robert decidió: el ledger debe estar activo, y hay que arreglar la
  limpieza de worktrees post-merge, que no funciona. Nuevo trabajo (ya no es solo
  lectura), siguiendo el harness del repo (ODD) en un worktree nuevo desde
  development:
  1) Limpieza post-merge: diagnostica por qué no corre. Un worker de Claude
  encontró que el hook llama a `worktree-cleanup`, que no existe en el release
  0.26.2 instalado, y que falta el snapshot de sync; por eso siguen vivos 6
  worktrees ya mergeados. Confírmalo y arréglalo en el CLI (no parchees la
  instalación global).
  2) Activar el ledger: que cada change quede registrado automáticamente en el
  flujo que realmente corre (p. ej. bind en apply-start), en ambos modos ODD y
  OpenSpec (hoy la evidencia solo lee openspec/changes), y que su ausencia deje de
  pasar en silencio. Incluye el arreglo de pureza del grade que encontraste si cae
  en el mismo código; lo demás de tu auditoría queda para después.
  Pruebas: tests focalizados más una prueba del flujo real con el binario
  compilado desde la rama (apply → merge simulado en repo de prueba → fila en el
  ledger y worktree retirado). No borres los 6 worktrees existentes ni hagas
  backfill del ledger, ni commit/push/PR: detente con el diff validado y reporta
  qué cambiaste, evidencia de las pruebas y cualquier decisión de producto
  pendiente. Termina con la línea LISTO FIX.»
- **L2** — «Decisión de Robert sobre T1: si la limpieza se arregla publicando un
  release con worktree-cleanup, el release queda autorizado, pero como paso final
  de esta unidad (después de las pruebas del flujo real, el PR y el merge), no
  antes. Y el diseño no debería depender de eso: el hook no puede volver a fallar
  en silencio por desfase entre el binario instalado y el que generó el hook. Que
  detecte que falta el verbo (o la versión mínima) y dé un mensaje accionable, o que
  tenga un fallback razonable; cúbrelo con un test. Aparte:
  odd/tasks/ledger-live-cleanup.md quedó sin trackear en el checkout principal de
  development; muévelo al worktree .worktrees/ledger-live-cleanup y deja limpio el
  checkout principal. Sigue sin commit/push/PR hasta que reportes el diff validado.
  Usa la línea LISTO FIX FINAL solo cuando todo el trabajo (incluida la prueba del
  flujo real) esté terminado y no queden subagentes corriendo; para avances
  intermedios no pongas marcador.»
- **L2 finding** — the feature document was never in the main checkout: it exists
  only at `.worktrees/ledger-live-cleanup/odd/tasks/ledger-live-cleanup.md`, where
  it is untracked (expected, no commit yet). `git status` in the main checkout is
  clean, and the worktree shows `?? odd/tasks/ledger-live-cleanup.md`. Nothing to
  move; the observation most likely came from the worktree's own status.
- **L3 finding (T1, corrects my own brief)** — base `development@75daa44e` already
  serves `worktree-cleanup`: `internal/cli/worktreecleanup.go` exists and was
  introduced by `f3a53084` (PR #585). The verb is therefore missing only in the
  installed release 0.26.2 (`~/.local/bin/ai-specs`, `~/.ai-specs/dist`), which is
  why six merged worktrees stayed: the hook was generated by a development sync
  and executed by an older binary, and it proved only that the binary exists.
  S10 is what makes that visible, and the release is the final step that closes it.
- **L4 finding (S5, found by the failing full suite)** — `trackerGateLedger`
  discarded the predicate's stderr (`io.Discard`) and its caller returned without
  any output when the verdict JSON was empty, so every refused grade — usage error,
  lock timeout, corrupt store — became silence. That is the same disease S5 names,
  and it hid the real reason in the validate run: `worktree-gate: ledger_mode off;
  skipping pre-merge` while the test had written a `warn` snapshot. Fixed: the
  predicate's first stderr line is kept and reported, and the only silent case left
  is a deliberately disabled ledger.
- **L5 evidence** — `tests/scenarios/ledger_cleanup_flow.sh` GREEN: `bound card
  47ab3c9e13e2d38ecd89a567 at apply-start (ledger row recorded)`, `would remove
  feat`, `closed ledger item for change/feat`, `removed feat`, one item
  `{"status":"closed","item_id":"47ab3c9e13e2d38ecd89a567","identity":
  {"branch":"change/feat","change":""}}`, `git worktree list` main only. RED with
  `SOURCE_REF=75daa44e`: fails at check 5, `apply-start wrote no ledger store … so
  the change was never registered`. The flow has no remote, so remote deletion and
  base sync are exercised only as documented skips.
- **L6 evidence (parity baseline)** — `./tests/validate.sh` exits 1 only on
  `tests/parity`: 19/35 fixtures fail in both modes, and every delta is
  `ai-specs/.ai-specs.lock` (64 changed, 8 extra). The same harness on a clean
  clone of `75daa44e` fails the identical fixture set (`diff` of the failure names
  is empty), so it is pre-existing, not a regression from this unit.
- **L7 evidence (S8, observed by the parent)** — `bash
  tests/scenarios/ledger_cleanup_flow.sh` → exit 0, checks (1)-(8) PASS, ending
  `apply -> squash merge -> one ledger row bound and closed, worktree removed`.
- **L8** — «Observación de Robert antes de autorizar: la estrategia de merge la
  elige el usuario, no el CLI ni el escenario, y el ledger y la limpieza deben
  detectar el merge sea cual sea el tipo. Hoy el escenario solo prueba squash.
  Revisa: (1) que nada en el flujo (hook, cleanup, skills/recetas, mensajes)
  imponga o asuma squash; si algo lo hace, corrígelo; (2) que la detección de
  "mergeado" y el cierre de la fila funcionen con merge commit (--no-ff),
  fast-forward, rebase y squash, incluido el caso de PR mergeado en GitHub con la
  rama remota borrada si aplica; (3) parametriza
  tests/scenarios/ledger_cleanup_flow.sh para correr las cuatro estrategias y
  pruébalo, con RED en la base donde corresponda. Si alguna estrategia no se puede
  detectar de forma fiable, dilo con evidencia en vez de forzarla. Sigue sin
  commit/push/PR. Termina con LISTO FIX FINAL solo cuando todo esté probado y no
  queden subagentes corriendo.»
- **L8 finding (already verified)** — the cleanup proofs are strategy-neutral:
  ancestry (`runner/cleanup.go:180`), per-commit `git cherry` patch equivalence
  (`:184`), combined patch-id (`:206`), combined tree equivalence (`:236`), and the
  local branch is removed with `-d` falling back to `-D` (`:351-374`). What did
  impose squash: `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md:176`
  (`gh pr merge --squash`), the same for gitlab (`glab mr merge --squash`, :170,
  :173) and the three `recipe.toml` cleanup lines, plus the scenario itself.
- **L9 evidence (S11, four strategies)** — `bash tests/scenarios/ledger_cleanup_flow.sh`
  with `MERGE_STRATEGY` unset → exit 0, `PASS merge / ff / rebase / squash`, each
  asserting its own precondition: `merge` tip `2c8a59f` is an ancestor but not the
  integration tip; `ff` tip `96b496e` equals the integration tip; `rebase` rewrites
  the commits (`486cadb` gone from `main`, subject `feat` intact on top of `main
  moves ahead`); `squash` leaves a single `squash change/feat` commit with the
  branch tip off `main`. All four close the ledger row at `archive-close` and
  remove worktree plus branch. RED on `75daa44e` is the ledger bind at check (5) for
  every strategy; there is no cleanup RED because base already serves the verb.
- **L9 finding** — the generated `post-merge` hook fires on `merge`, `ff` and
  `rebase` (not on `--squash`), so a plain `git merge --no-ff` cleans up mid-merge
  and deletes the branch the assertions still need. The scenario disables hooks for
  the merge step only (`git -c core.hooksPath=/dev/null`) and must therefore prove
  the automatic trigger separately; the reverse proof (hook enabled, no explicit
  verb) is what the added check (9) covers.
- **L10 evidence (S11 complete)** — the squash imposition is gone: `gh pr merge
  <strategy>` / `glab mr merge <mr-number> <strategy>` with the strategy chosen by
  the human, and the three brief lines now read ``(`git branch -d`, or `-D` after a
  squash or a rebase)``. The six frozen-oracle occurrences were re-derived, not
  recaptured: `git diff --numstat` shows 3/3 in each fixture and the diff spans only
  that sentence; `go test ./internal/schema/ ./internal/config/ -run
  'Differential|Frozen'` is green. `bash tests/scenarios/ledger_cleanup_flow.sh`
  (run by the parent) → exit 0, `5/5 flow cases`: the four strategies each asserting
  its own precondition, plus check (9) where the generated `post-merge` hook alone
  closes the row and removes the worktree; a hook-disabled mutation of the same
  script fails (9) with `the automatic post-merge trigger did not fire`, so the
  check cannot pass vacuously.
- **L10 residual** — `squash` never triggers `post-merge`, so for that strategy the
  close depends on the merge skill invoking `ai-specs worktree-cleanup`; `merge`,
  `ff` and `rebase` close themselves through the hook. On `75daa44e` check (9) is
  unreachable (it dies at the ledger bind), so the base cannot exercise it at all.
  Final `./tests/validate.sh` on this tree: no Go test or package failure; the only
  red is `tests/parity`, 19/35 in both modes, the same fixture set as a clean clone
  of `75daa44e`.
- **L11** — «Robert aprobó todo: 1) En este mismo PR, añade un disparador de
  limpieza que no dependa del merge local, para que squash y los merges hechos en
  GitHub cierren la fila y retiren el worktree igual que merge/ff/rebase (p. ej. al
  actualizar development por pull/checkout, o en ai-specs sync; elige tú el diseño y
  justifícalo). Añade ese caso al escenario (squash sin llamar a worktree-cleanup a
  mano, y merge "remoto" simulado) con su RED. 2) Change ODD sin sección Tracker:
  se queda en aviso fuerte, sin bloquear. 3) Con todo verde: archive del change
  (obligatorio antes del merge), commits por unidad, PR a development. Tú eres
  dueño del PR hasta que esté listo: vigila checks y reviews y corrige. Luego merge
  (usa la estrategia habitual del repo), release que incluya worktree-cleanup y la
  sonda del hook, y después ai-specs sync del propio repo con el binario de
  development. Verifica al final que la instalación global ya tiene
  worktree-cleanup y que la limpieza retira los worktrees mergeados que corresponda
  (reporta cuáles retiró). Fuera de este PR, para una unidad aparte: slugs ODD en
  todo el tracker, el error que se traga worktreeGateSnapshot y la parity del lock.
  Reporta hitos breves (PR abierto, merge, release, sync) y termina con LISTO
  ENTREGA cuando todo esté hecho y no queden subagentes ni jobs.»
- **L12 design decision (mine, as authorized)** — the trigger is a generated
  `.git/hooks/post-checkout` that calls `ai-specs worktree-cleanup --if-integration`:
  the verb reads the sync snapshot and no-ops unless the checked-out branch is the
  integration branch, so the hook holds no policy and cannot guess. Why not `sync`:
  making sync destructive would delete worktrees for every project on upgrade, with
  no human at the moment, and sync is the step that otherwise only writes files. Why
  post-checkout: a provider-side merge lands locally as `git checkout <integration>`
  (or `git switch`) after a fetch, which no existing hook covers; `post-merge`
  already covers `git pull` with merge or fast-forward.
- **L13 retrospective native review of the merged tree** — `review-dc6e083a50e1ec8e`
  (base `75daa44e…`, committedOnly, lenses risk/resilience/reliability, tier high,
  review of the squash commit `11915d6`) returned `correction_required` with three
  blocking findings, all in code merged minutes earlier and none caught by the
  scenario or the focused suites:
  - **R1-001 / R3-001 (BLOCKER, CRITICAL)** — `materializeWorktreeCLIHook` writes
    `.git/hooks/post-checkout` unconditionally, so a project that already has its
    own post-checkout hook loses it on the next sync. `post-merge` was always
    regenerated by design; `post-checkout` was never ours to claim.
  - **R1-003 (BLOCKER)** — `tracker evidence` with an explicitly empty slug no
    longer means "no scope": `EvidencePayload` now delegates to `trackerFields`,
    which infers the active OpenSpec change, so unscoped callers receive that
    change's `card_id`. Verified against `75daa44e`, where the empty slug returned
    empty evidence: a regression this change introduced.
  - **R3-002 (informational)** — the probe proves the verb, not the
    `--if-integration` option, so a binary that serves the verb without the flag
    passes the probe and then no-ops silently.
  - **R3-003 (informational)** — an unreadable planning document is skipped like an
    absent one, which can hide a real card.
  - **R4-postcheckout-latency (informational)** — the hook fires on file restores
    too (`git checkout -- <path>`), where `$3` is `0`, so a full pass runs when
    nothing about the branch changed.
  `R1-002` and `R4-postcheckout-exit-status` were **refuted**: git ignores a
  post-checkout hook's exit status, so the verb's exit 2 cannot fail a checkout.
  The lineage is left as an open record: the corrections land in a fresh candidate
  and a fresh review, because the merged branch is protected and this authority
  cannot be repaired from here.
- **L14** — those five findings are fixed in `change/cleanup-review-fixes` before
  the release, so the shipped hook never clobbers a user hook and `tracker
  evidence` keeps its documented contract.
