# [Go 06] Embed catalog, templates and bundled assets (go:embed)

Card: Trello `6a84e79a418782ae1427b52a` — branch `change/go-06-embed-assets`
(cut from epic tip `1bbbc6f`). Trello MCP was down at session start; the card
scope was taken from the orchestrator-provided card text (inline, binding).

## Goal

Catalog, templates, bundled skills and bundled commands become embedded assets
of the single binary (stdlib `go:embed`, zero third-party deps,
CGO_ENABLED=0). A Go asset accessor replaces filesystem lookups under
`$AI_SPECS_HOME` for CLI-BUNDLED assets only; user-vendored deps stay on disk
(card 11). `refresh-bundled` is reduced to a compatibility stub reporting
embedded provenance. The legacy cache machinery is NOT deleted (card 16).

## Asset inventory (runtime-read paths and consumers)

All paths below are resolved under `$AI_SPECS_HOME` (the CLI install root).
Inventory method: `grep -rn '"catalog"|"templates"|"bundled-skills"|"bundled-commands"' lib/ bin/`
plus manual reading of each consumer.

### 1. `catalog/` (recipes + bundled catalog skills)

| Path | Consumers (lib/) | Use |
|---|---|---|
| `catalog/recipes/<id>/recipe.toml` | recipe-read.py, recipe-list.py, recipe-add.py, recipe-configure.py, recipe-init.py, recipe-materialize.py, recipe-conflicts.py (via recipe_schema.py) | recipe schema parse, add/init/list/configure, materialization |
| `catalog/recipes/<id>/{bin,hooks,templates,docs}/**` | recipe-materialize.py (via recipe_schema provides), gate_binary.py | materialize recipe artifacts, gate binary acquisition |
| `catalog/recipes/worktree-flow/gate/**` | gate_binary.py | builds/locates the worktree-gate Go module (nested module with own go.mod) |
| `catalog/recipes/.gitkeep` | none (placeholder) | — |
| `catalog/skills/{context-precedence,testing-foundation}` | none found in lib/ runtime reads | shipped catalog skills (documented; consumed via skill docs, not runtime I/O) |
| `catalog/README.md` | none (docs) | — |

Consumers listing `"catalog"`: config_wizard.py, dep_check.py, doctor.py,
env_scaffold.py, gate_binary.py, init_tui.py, recipe-add.py,
recipe-configure.py, recipe-init.py, recipe-list.py, recipe-materialize.py,
skills-list.sh.

### 2. `templates/`

| Path | Consumers | Use |
|---|---|---|
| `templates/ai-specs.toml.tmpl` | init.sh:231 (via `$TEMPLATES_DIR`) | first-init manifest template |
| `templates/gitignore-root.tmpl` | init.sh:270, sync.sh:218 (gitignore-root-refresh.py argv) | root .gitignore agent block |
| `templates/openspec/config-fragment.yaml` | NONE FOUND | no runtime consumer located at this revision; embedded verbatim with the root |

Consumers listing `"templates"`: doctor.py, recipe_schema.py, recipe-add.py,
recipe-read.py, skill-resolution.py (note: skill-resolution's "templates"
references are recipe override dirs, not the CLI templates root).

### 3. `bundled-skills/` (harness-lifecycle, harness-recipes, harness-skills-deps, skill-creator, skill-sync)

| Consumers | Use |
|---|---|
| refresh-bundled.py:68 | flatten source → `{cache}/.bundled/skills/` |
| project-cache.py:197,369 | leftover cleanup source of truth; bundled skill id listing |
| doctor.py:26 | verify bundled skills present in install |
| skill-resolution.py:120 (via project-cache) | tier-4 resolution: `{cache}/.bundled/skills/` flattened from here |
| init.sh (BUNDLED_SKILLS_DIR) | init docs/guidance text |

### 4. `bundled-commands/` (rules-audit.md, skills-as-rules.md)

| Consumers | Use |
|---|---|
| refresh-bundled.py:90 | flatten source → `{cache}/.bundled/commands/` |
| project-cache.py:274,380 | leftover cleanup; bundled command id listing |
| doctor.py:42 | verify bundled commands present in install |

## Design

- **Embed location**: root module package `assets` (repo root, `assets.go`) —
  `go:embed` patterns are relative to the package directory, so the embed
  package must sit at the module root where `catalog/`, `templates/`,
  `bundled-skills/`, `bundled-commands/` live. `all:` prefixes include
  `.gitkeep`.
- **Accessor API** (`package assets`):
  - Roots: `Catalog()`, `Templates()`, `BundledSkills()`, `BundledCommands()`
    returning `fs.FS` rooted at each asset tree.
  - `Resolve(root, rel)` / digest helpers for verification.
  - Dev-mode override: env `AI_SPECS_ASSETS_DIR` — when set and
    `<dir>/<root>` exists as a directory, that root is served from disk
    (per-root override; missing roots fall back to embedded). Lets
    contributors point at on-disk assets without rebuilding.
- **Version isolation guarantee** (cache layout mapping): today the cache
  (`{install-root}/cache/`) is per-install-root, so each CLI version has its
  own `.bundled` tier pinned to its own shipped assets. With embedding, the
  asset source of truth is the binary itself: every binary carries exactly
  the asset set of its own build, so the per-version isolation is preserved
  by construction (stronger than the cache layout: no cache to populate or
  repair). Documented in the package doc and the PR.
- **refresh-bundled stub**: native route in internal/cli (routeNative),
  no-op; prints embedded provenance; frozen cross-cutting exit codes kept
  (0 ok/help, 2 unknown flag/unexpected positional). Legacy
  `lib/refresh-bundled.sh` + `lib/_internal/refresh-bundled.py` stay
  untouched — sync's internal flatten path keeps working (parity harness
  compares against it).

## Tasks

1. [x] Baseline binary size
2. [x] RED: acceptance tests (digest parity, no-home resolution, dev override, stub contract)
3. [x] GREEN: assets package with go:embed + accessor + dev override
4. [x] GREEN: refresh-bundled native stub
5. [x] Full verification (build/vet/test, parity, ./tests/run.sh)
6. [x] Binary size after + evidence

## Evidence

All measurements from this worktree (`change/go-06-embed-assets`).

### Go toolchain (this session, after final gofmt fix)

| command | result | exit |
|---|---|---|
| `gofmt -l .` | no unformatted files (see Findings) | 0 |
| `go vet ./...` | no findings | 0 |
| `go test ./...` | all 8 packages `ok` / no-test-files | 0 |
| `CGO_ENABLED=0 go build -o <tmp>/ai-specs-go ./cmd/ai-specs` | built | 0 |

### Full-suite evidence (earlier run, orchestrator-provided)

- `./tests/run.sh`: **Ran 2372 tests**, `suite_exit=0`.
- Parity harness: **8 fixtures, 0 failing**.

### Binary size

| | bytes |
|---|---|
| before | 2,635,106 |
| after | 3,123,234 |
| delta | +488,128 (+18.5%) |

`after` re-measured this session from a fresh `CGO_ENABLED=0` build of
`./cmd/ai-specs`; it reproduces the prior figure exactly (3,123,234 bytes).
The growth is the four embedded asset trees (`catalog`, `templates`,
`bundled-skills`, `bundled-commands`) linked into the binary.

### Changed paths (uncommitted)

```
 M internal/cli/cli.go
 M internal/cli/cli_test.go
?? assets.go
?? assets_test.go
?? internal/cli/refresh_bundled.go
?? internal/cli/refresh_bundled_test.go
?? odd/tasks/go-06-embed-assets.md
```

## Findings / surprises

- **gofmt caught a real defect**: `internal/cli/cli_test.go` had misaligned
  map entries in the `natives` literal after adding `refresh-bundled`
  (`gofmt -l` listed it). Fixed with `gofmt -w`; `gofmt -l .` is now clean.
- **Nested Go module is structurally un-embeddable**: `go:embed` excludes
  `catalog/recipes/worktree-flow/gate/` (own `go.mod`). The digest-parity
  test therefore skips that directory on the disk walk; the gate's runtime
  trust root `.../worktree-flow/bin/SHA256SUMS` *is* embedded. Documented in
  both `assets.go` and the test doc comment.
- **Cache-tier version isolation is replaced by construction**, not merely
  preserved: each binary carries exactly its own build's asset tree, so
  there is no `{install-root}/cache/.bundled` tier to populate or repair.
- **Legacy flatten path intentionally untouched**: `lib/refresh-bundled.sh`
  and `lib/_internal/refresh-bundled.py` remain, because `sync` still calls
  the Python internally and the parity harness compares against it. Only the
  dispatcher-level verb became a native no-op stub (frozen exit codes 0/2
  preserved).

## What remains (later cards)

- Rewire asset-consuming verbs to read through the accessor (later cards in
  the epic). Nothing consumes `package assets` yet except the
  `refresh-bundled` stub and its tests.
- Delete the legacy cache/flatten machinery (card 16).
- Revert `base_branch`/`integration_branch` config values before the
  promotion PR (card 16).
