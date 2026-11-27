# Auditoría — hooks, entorno de desarrollo y ledger

**Repositorio**: `/Users/robert/proyectos/nnodes/ai-specs-cli` (`development` @ `36438150`)
**Alcance**: solo lectura. Nada se editó, commiteó, sincronizó ni borró. `git status` quedó limpio en todo momento.
**Método**: tres verificadores independientes read-only (partes A, B, C) + comprobaciones propias del orquestador.

---

## Alta

### 1. El repo no fuerza el CLI de `development`: resuelve al release global (B)

`which -a ai-specs` → solo `/Users/robert/.local/bin/ai-specs` (duplicado en PATH, un único ejecutable) → symlink a `~/.ai-specs/dist/ai-specs`. No hay build del checkout en PATH, y el entorno no exporta `AI_SPECS_HOME` ni `AI_SPECS_GO_BIN` (direnv **sí** está activo y permitido: `Loaded RC allowed 0` / `Found RC allowed 0`).

Hoy no se nota porque el global es **byte-idéntico** a un build canónico de `development`: `f33a49fa25c8f8c6867931a13f8e211e42326de59209d06ae52a60ab5e1ccf45` en los tres (binario global, sello `0.27.1`, `bin/SHA256SUMS:31`, build en scratch). Es equivalencia **por coincidencia de versión**, no por construcción: en cuanto `development` avance una PR, el repo seguirá usando el binario viejo sin avisar.

### 2. `dist/ai-specs` del checkout está obsoleto y nada lo reconstruye (B)

`go version -m` → `vcs.revision=9742cf5199ee7eebabaca503ec66b3bedda433cb` (**PR #597**, anterior al ledger de #599), `CGO_ENABLED=1`, sha `bbdd962b75f537cc81c40980fe9706d1a6c59480713bcd20755a92acc338dc14`, y **no existe `dist/VERSION`**. No está trackeado por git, así que puede envejecer indefinidamente. `bin/ai-specs` lo prefiere sobre cualquier otra ruta y su self-heal compara versión y bytes, **no la revisión de fuente** (`bin/ai-specs:132–155,257–283`). Es una trampa real: ejecutar `./bin/ai-specs` hoy corre un binario previo al arreglo del ledger.

### 3. El ledger no registró ninguna de las dos unidades (C)

`.git/ai-specs/ledger/state.json`: **14 filas, todas `closed`**, 15.243 bytes, último `decision.at` = `2026-10-08T14:40:02Z` (clavado al mtime), fila más nueva `change/go-07-sx0f-classify` — verificado por el auditor y reconfirmado por el orquestador.

No es un evento git: lo crea un **hook de agente pre-tool-use** que llama `ai-specs tracker-card-gate` en `apply-start`, y su matcher por defecto es `lib catalog bin src` — **no** `internal` ni `tests` (`catalog/recipes/trello-mcp-workflow/recipe.toml:187–200`, `internal/cli/trackercardgate.go:213–275`). Para registrar hacía falta: escritura en directorio elegible dentro del worktree de la rama + veredicto `needs-item` activo + documento de planificación con `card_id` (**OpenSpec primero, `odd/tasks/<feature>.md` como fallback**).

Causa confirmada para las dos:

- `ledger-live-cleanup`: **no tiene `## Tracker` ni `card_id`** en `odd/tasks/ledger-live-cleanup.md` (ni en el snapshot de #599).
- `installer-home-resolution`: no tuvo plan (ODD small path) y tocó `internal/` + `tests/`, fuera del matcher.

Descartado como causa: los hooks de git sólo **cierran** filas existentes, no las crean; el squash no impide el registro.

---

## Media

### 4. Tres hooks locales de Claude apuntan a un script inexistente (A)

`.claude/settings.local.json:26,37,48` (`UserPromptSubmit`, `Stop`, `Notification`) invocan `node /Users/robert/.bridgespace/bin/bs-claude-hook.cjs`, que **no existe**. Rotura externa al repo, pero es ruido en cada prompt/stop de Claude.

### 5. El aviso "fuerte" del tracker nunca llegó por Pi (C)

El texto existe (`internal/cli/trackercardgate.go:59–61,379–396,421–448`, con test en `trackercardgate_test.go:388–435`), pero el wrapper generado para Pi sólo reenvía stderr con **exit 2** y este gate devuelve **0** (`.pi/extensions/trello-mcp-workflow-tracker-card-gate.ts:38–45`). Los tests lo fijan en Go; **ningún artefacto demuestra que se emitiera** en estas dos unidades. El escenario `tests/scenarios/ledger_cleanup_flow.sh:249–268` valida el bind exitoso, no el aviso.

### 6. `ledger-live-cleanup` nunca archivó su change, pese a la instrucción explícita (C)

L11 del documento dice "archive del change (obligatorio antes del merge)". `11915d6f` (#599) sólo añade el documento; `25c48380` (#600) y `cb0072e0` (#603) lo actualizan. **No hay archivo antes ni después** del merge en el grafo. La unidad del instalador **no debía** archivar (small path, sin carpeta) — informativo, no defecto.

### 7. `doctor` da un consejo de reparación engañoso para los dos hooks de git (A)

Dice `rm .git/hooks/post-merge && ai-specs sync` por "missing ownership metadata". Pero `sync` **regenera `post-merge` incondicionalmente** y `post-checkout` también (lleva marcador de receta): borrarlos no es necesario (`internal/sync/worktreecleanup.go:115–144`). El aviso sale de la lógica genérica de `stale-override` (`internal/doctor/checks_overrides.go:66–108`), que no entiende este caso.

### 8. Ni el snapshot ni el lock dicen qué ejecutable corrió (B)

`cache/projects/df4360950abd-ai-specs-cli/runtime.toml` existe bajo **ambos** homes (global 14:18:29, checkout 01:17:12) y no registra versión ni ruta del CLI (`internal/projectcache/runtimesnapshot.go:47–48,88–114`). `ai-specs/.ai-specs.lock:9–10` sólo trae `cli_version` + `synced_at`. La versión coincide, pero **nada prueba qué binario sincronizó**.

---

## Baja / Informativa

- **Launcher retirado aún presente**: `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` existe pero **no está cableado** en ningún hook de git ni de agente (`rg` sin coincidencias fuera de catálogo/migración/doctor/tests/docs). `catalog/recipes/worktree-flow/README.md:78–80,178` todavía promete materializar el launcher; `docs/recipes-catalog.md:344` ya lo llama obsoleto.
- **Cursor sólo cablea gates de shell**: no hay pre-file-write, así que `worktree-gate`, `tracker-card-gate` y `plan-build-gate` no aplican ahí (`internal/sync/hooksrender.go:822–832`).
- **MCP vault-fs rechazaría el scope actual**: el valor resuelto no es directorio (`nonempty=True unexpanded_dollar=False directory=False`) y el CLI corta antes de lanzar (`internal/cli/mcp_vaultfs.go:69–88`). Puede variar según el entorno del agente.
- **Los 5 hooks de agente (Claude/Pi/OMP/OpenCode) y los 2 de git funcionan**: los verbos existen (`worktree-cleanup`, `worktree-gate`, `tracker-card-gate`, `plan-build-gate` responden `--help` con exit 0), los scripts del catálogo existen, los dos hooks instalados son **byte-idénticos** a sus generadores Go (`internal/sync/materialize_loop.go:128–150`), y `core.hooksPath` está sin definir (se usa `.git/hooks`).
- **Hooks de git, resolución de CLI**: ambos prefieren `${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs` y caen a PATH; el ejecutable existe y resuelve. `post-checkout` (`:2–14`) sondea `--if-integration` y ejecuta `worktree-cleanup --if-integration`; `post-merge` (`:2–9`) ejecuta `worktree-cleanup`. El snapshot vivo tiene `integration_branch = "development"` (`runtime.toml:10`).
- **`doctor`**: 31 OK, 1 INFO, **5 WARN, 0 ERROR**. WARNs: `mcp` no declarado (lo aporta la receta: benigno), `tracker-ledger` sin item vinculado (hueco real de trazabilidad, no de cableado), launcher retirado (inerte) y los dos `stale-override` del punto 7.
- **Acciones `on-sync`** (`validate-config`, `bootstrap-board`, tres diferidas) son acciones Go, no comandos externos (`internal/sync/hooksrunner.go:70–82`): no hay nada roto ahí.

---

## Recomendaciones (sin aplicar)

1. **Fijar el CLI de development** para el repo, gestionado por `direnv`: `AI_SPECS_GO_BIN` al launcher/build del checkout (y decidir si `AI_SPECS_HOME` debe seguir apuntando al home global). Sin eso, la regla depende de que las versiones coincidan.
2. **Eliminar la trampa de `dist/`**: o se reconstruye con `CGO_ENABLED=0 -trimpath -buildvcs=false` y sello, o se borra; y que `bin/ai-specs` compare revisión de fuente, no sólo versión/bytes.
3. **Registrar el ejecutable** (ruta + digest) en el snapshot o el lock, para que "qué binario sincronizó" sea auditable.
4. **Ledger**: extender el matcher del gate o exigir `## Tracker` con `card_id` en todo plan ODD; y arreglar el wrapper de Pi para que un aviso en stderr con exit 0 no se pierda.
5. **Archivar antes del merge** de verdad (el change de `ledger-live-cleanup` quedó sin archivar) y corregir el consejo de `doctor` para hooks de receta.
6. Reparar o retirar los tres hooks de Claude y/o `bs-claude-hook.cjs`.

---

## Qué no se pudo verificar

Si el hook de agente **se disparó** en esos momentos, con qué binario y home exactos, y si el aviso apareció en una sesión o log no retenido. Los auditores tampoco pudieron crear scratch para ejecutar hooks reales: el guard de rama protegida les denegó las escrituras en `$s/` y `$s/work/.git/hooks/`, así que **no se ejecutó cleanup real, ni sync, ni evento de agente, ni arranque de MCP**.

---

*Auditoría de solo lectura. Sin cambios en el repositorio.*
