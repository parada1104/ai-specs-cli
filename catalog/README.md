# Skill catalog (ai-specs-cli)

These directories are **vendored** into your project via `[[deps]]` in `ai-specs/ai-specs.toml`.
Each row is a normal dependency: `ai-specs sync` shallow-clones `source` and copies the skill
from `path` into `ai-specs/skills/<id>/` (gitignored by default).

Upstream repo: `https://github.com/parada1104/ai-specs-cli`

Replace `SOURCE` below with that URL, **or** with an absolute path to a local clone of this repo
(useful in CI or air-gapped workflows — `git clone <local-path>` is supported).

## context-precedence

MVP order for conflicting context sources (docs, skills, memory, proposed output).

```toml
[[deps]]
id = "context-precedence"
source = "SOURCE"
path = "catalog/skills/context-precedence"
scope = ["root"]
license = "MIT"
auto_invoke = ["Resolving conflicts between documentation, skills, memory, and proposed context"]
```

## testing-foundation

Default test commands and evidence expectations for ai-specs-shaped repos.

```toml
[[deps]]
id = "testing-foundation"
source = "SOURCE"
path = "catalog/skills/testing-foundation"
scope = ["root"]
license = "MIT"
auto_invoke = ["Choosing default test commands before merge or PR"]
```

## openspec-sdd-conventions

OpenSpec `config.yaml` shape (spec-driven) plus apply-time commit and archive norms.

```toml
[[deps]]
id = "openspec-sdd-conventions"
source = "SOURCE"
path = "catalog/skills/openspec-sdd-conventions"
scope = ["root"]
license = "MIT"
auto_invoke = ["Editing openspec/config.yaml for spec-driven workflow"]
```

## Example: GitHub source

```toml
[[deps]]
id = "context-precedence"
source = "https://github.com/parada1104/ai-specs-cli.git"
path = "catalog/skills/context-precedence"
scope = ["root"]
license = "MIT"
```

Repeat with `testing-foundation` / `openspec-sdd-conventions` paths as needed, or use
`ai-specs add-dep` with `--subdir` pointing at `catalog/skills/<name>` (same effect).

## Notes

- Each `[[deps]]` entry triggers its own shallow clone today; multiple deps to the same `source`
  are independent clones (a future optimization may dedupe).
- After editing `ai-specs.toml`, run **`ai-specs sync`** to vendor and regenerate `AGENTS.md`.
- `skill-sync` requires `auto_invoke` on vendored skills when `scope` is set — include the
  `auto_invoke` lines below (or pass `--trigger` to `ai-specs add-dep`).

## ai-specs-cli development copy

This repository also keeps **committed** copies under `ai-specs/skills/` for dogfooding and
tests. Treat **`catalog/skills/`** as the public distribution layout for `[[deps]].path`;
keep them in sync when editing policy skills.
