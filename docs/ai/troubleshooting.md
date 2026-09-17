# Troubleshooting

Common issues and fixes for the ai-specs manifest pipeline.

## Manifest validation errors

### `Unknown field in manifest`

A key in `ai-specs.toml` is not recognized by the current validator.

**Fix:** Check [`docs/ai-specs-toml.md`](../ai-specs-toml.md) for the canonical
V1 surface. Remove unrecognized sections or move them to a comment.

### Legacy `version` in `[recipes.<id>]`

Per-recipe version pins are no longer required. If a leftover `version` key
remains in the manifest, sync emits a WARN and continues with the catalog
shipped by the installed CLI.

**Fix:** Optional — delete the `version` lines from `[recipes.<id>]`. After
`ai-specs upgrade`, run `ai-specs sync` (no pin bump). Use
`ai-specs recipe list` to see catalog versions as info only.

### `subrepos` path resolution failed

A path listed in `project.subrepos` does not exist relative to the project root.

**Fix:** Remove the invalid entry or create the target directory.

## Runtime brief ownership

If sync reports that `AGENTS.md` was left unchanged as `untracked` or
`user_modified`, ai-specs preserved the existing bytes because it cannot prove
that it owns them. To hand the current file to ai-specs explicitly, run
`ai-specs sync --adopt-brief`; to keep it permanently user-owned, add
`<!-- ai-specs:runtime-brief -->` at the top. `ai-specs doctor` reports the
same state and remedies. See [`runtime-brief-ownership.md`](runtime-brief-ownership.md).

A regular `CLAUDE.md` is not overwritten: it is a relative symlink target and
`make_relative_symlink` refuses to replace a pre-existing regular file. The
runtime-brief ownership decision applies to `AGENTS.md`.

## Sync warnings

### `Multiple recipes provide the same capability`

Two or more enabled recipes declare the same capability ID without an
explicit `[[bindings]]` entry.

**Fix:** Add a `[[bindings]]` block in your manifest to choose which recipe
owns the capability. See [`docs/ai-specs-toml.md`](../ai-specs-toml.md).

### `Unknown config key in [recipes.<id>.config]`

A config override key does not match any field in the recipe's `[config]` schema.

**Fix:** Remove the unknown key or verify the recipe's schema in
[`docs/recipe-schema.md`](../recipe-schema.md).

### `env = ["VAR"]` normalized to `{ VAR = "$VAR" }`

This is informational — the array form is a supported shorthand. The
normalization is correct and does not need a fix.

### CLI version mismatch on sync

`ai-specs sync` failed because `[tool].version` or `[tool].min_version` does not
match the globally installed CLI.

**Fix:** Run `ai-specs upgrade` to update the global CLI, adjust the pin in
`ai-specs.toml`, or use `ai-specs sync --ignore-cli-version` as a break-glass
option. Check `ai-specs doctor` for installed vs pinned vs last-synced values.

## See also

- [`docs/ai-specs-toml.md`](../ai-specs-toml.md) — Canonical manifest reference
- [`docs/recipe-schema.md`](../recipe-schema.md) — Recipe schema reference
