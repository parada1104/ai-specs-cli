# [Go 07.S7] Skill resolution + flatten — Go `Flatten` + single cache-key owner

Card: `[Go 07.S7]` — https://trello.com/c/AKU5lg7s (parent epic `go-single-binary`)
Branch: `change/go-07-s7-skills` · Worktree: `.worktrees/go-07-s7-skills`
Base: `d4b1afb` (S6 merge) · Plan: `odd/tasks/go-07-sync-plan.md` (S7 row)

## Goal

Port `lib/_internal/flatten-resolved-skills.py` (56 lines) to Go and close the
`skill-resolution.py` port, proven by a differential against the REAL script.

## Scope decision

- `collect_skills` and its four scanners were already ported in Go 08
  (`internal/skills/resolve.go`, pinned by `TestDifferentialCollectSkills`).
- `flatten-resolved-skills.py` has one caller, `lib/sync-agent.sh:287`. The Go
  spine still execs `sync-agent.sh` until S15, so S7 ships the library +
  differential only (S2/S4/S5/S6 precedent): no flag, no wiring, the Python
  stays alive.
- **Not ported (YAGNI)**: `load_skill_config`, `resolve_skill_template`,
  `resolve_skill` and the `skill-resolution.py <root> [--json]` CLI. They have
  zero production callers (only `tests/test_external_dirs.py` probes the module
  functions; `hub.py` and `rules-inventory.py` use `collect_skills` only). They
  die with the Python in S16; port them only if a Go caller appears.
- **Dedupe**: Go 08 wrote a second copy of the frozen project-cache derivation
  in `internal/skills` (cache key, cache root, `*_skills_root`, realpath) before
  S6 ported `project-cache.py`; S6 had to fix both twins. `internal/skills`,
  `internal/doctor` and `internal/rulesaudit` now derive every cache path
  through `internal/projectcache` (−312/+27 lines).

## Tasks

- [x] T1 — `refactor(skills)`: route cache roots through `internal/projectcache`,
  delete the duplicate + its duplicated tests (covered by `projectcache_test`).
- [x] T2 — `feat(skills)`: `skills.Flatten` + `TestFlattenDifferential`.
- [x] T3 — Evidence in this doc.

## Design

`Flatten(projectRoot, destDir, cliHome string, stdout, stderr io.Writer) int`
mirrors `main()` after argv parsing: resolve both paths, `collect_skills`
(warnings on stderr), `rmtree` an existing dest, `mkdir -p`, `copytree` each
skill in sorted id order, print `  ✓ flattened N skill(s) to <dest>`.

- Reuses projectcache's shutil mirrors, now exported: `CopyTree`
  (`copytree(symlinks=False)`: link targets copied as real files/dirs, per-entry
  errors accumulated, copystat) and `RemoveTreePy` (`rmtree` refuses symlinks
  and non-directories).
- The first failing skill stops the loop (Python's uncaught exception), keeping
  the skills copied before it and the partial failing skill.
- Python's per-skill `if target.exists(): rmtree(target)` is unreachable (fresh
  dest, unique ids) and is not ported.
- Tracebacks are not byte-reproducible: failures write `error: <msg>` and rc 1
  (S5/S6 traceback mode — rc, stdout and written tree are pinned).
- `AI_SPECS_HOME` unset → `error: AI_SPECS_HOME is not set`, rc 1, nothing
  written (S6 accepted deviation; the shim always pins it).

## Evidence

### RED

```
go test ./internal/skills -run 'TestFlatten' -count=1
internal/skills/flatten_test.go:84:8: undefined: Flatten
FAIL	ai-specs.dev/ai-specs/internal/skills [build failed]
```

### GREEN

```
go test ./internal/skills -run 'TestFlatten' -count=1 -v
--- PASS: TestFlattenDifferential
    four-tiers-duplicates-nested-modes-links, stale-dest-is-wiped, no-skills,
    dest-through-symlinked-parent, dangling-link-stops-after-failing-skill,
    dest-is-a-file — all PASS
--- PASS: TestFlattenRequiresHome
```

The differential runs both legs against the SAME sandbox (frozen cache key
hashes the realpath) and pins rc, stdout, stderr (success cases) and the dest
projection: kind, mode, content and mtime of every entry (dest root mtime
excluded — created by the run, not copied).

### Mutation check (each mutation killed)

| Mutation | Failing cases |
|---|---|
| continue past a copy error | dangling-link |
| skip dest resolve | stale-dest, no-skills, symlinked-parent |
| skip dest wipe | stale-dest, symlinked-parent |
| drop collect_skills warnings | four-tiers |
| reverse id order | dangling-link |

### Suites

- `go vet ./... && go test ./... -count=1` → rc 0.
- `./tests/validate.sh` → see PR (parity `multi-agent-fanout` included).
