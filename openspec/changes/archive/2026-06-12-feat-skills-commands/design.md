# Design: `ai-specs skills` subcommand group

## Architecture

### File layout (new)

```
lib/
├── skills.sh              # dispatcher: skills add | list | remove
├── skills-add.sh          # register [[deps]] (migrated from add-dep.sh logic)
├── skills-list.sh         # list deps + local + catalog skills
└── skills-remove.sh       # remove [[deps]] by id
bin/
└── ai-specs               # +skills case; add-dep → skills-add.sh
```

### Pattern: sub-dispatcher

Identical to `recipe.sh`:

```bash
# lib/skills.sh
subcmd="${1:-}"; shift
case "$subcmd" in
    add)    bash lib/skills-add.sh "$@" ;;
    list)   bash lib/skills-list.sh "$@" ;;
    remove) bash lib/skills-remove.sh "$@" ;;
esac
```

### CLI integration

In `bin/ai-specs`, the `case` statement gets two changes:

```bash
case "$cmd" in
    add-dep) bash "$LIB_DIR/skills-add.sh" "$@" ;;   # was add-dep.sh
    skills)  bash "$LIB_DIR/skills.sh" "$@" ;;        # new
    ...
```

The old `add-dep.sh` is preserved as a standalone script but no longer called
from the CLI. This avoids breaking any external references.

### `skills-add.sh`

Identical logic to `add-dep.sh`:
- Parse `--id`, `--subdir`, `--scope`, `--license`, `--attribution`, `--trigger`, `--no-sync`
- Derive defaults from URL
- Validate ID (kebab-case)
- Check duplicate via Python/tomllib
- Append `[[deps]]` block via Python
- Run `ai-specs sync` (unless `--no-sync`)

Difference from `add-dep.sh`: usage header says "ai-specs skills add" instead
of "ai-specs add-dep".

### `skills-list.sh`

Reads three data sources and prints tagged sections:

1. `[[deps]]` from `ai-specs.toml` — parsed via Python/tomllib
2. Local skills — `ls ai-specs/skills/*/SKILL.md`, extract frontmatter description
3. Catalog skills — `ls $AI_SPECS_HOME/catalog/skills/*/SKILL.md`

Uses inline Python for the TOML parsing; bash `head` + Python for YAML
frontmatter extraction (same pattern as existing scripts).

### `skills-remove.sh`

Removes a `[[deps]]` block by matching the `id = "<name>"` line inside a
`[[deps]]` ... (next section header) block. Uses Python `re.subn()` with a
pattern that captures from `[[deps]]` up to (but not including) the next
section header or EOF.

Does NOT delete the on-disk `ai-specs/skills/<id>/` directory — that's a
deliberate safety choice (user can `rm -rf` manually if desired).

## Decisions and rationale

| Decision | Rationale |
|----------|-----------|
| Preserve `add-dep.sh` on disk | No breaking changes for anyone referencing it directly |
| `add-dep` → `skills-add.sh` | Single implementation; add-dep becomes a lightweight alias |
| Remove doesn't delete on disk | Safety: prevent accidental data loss |
| Inline Python for TOML | Existing pattern in codebase (add-dep.sh, recipe-list.py) |
| YAML frontmatter extraction | Lightweight, no extra deps, works for the 1-5 lines needed |
