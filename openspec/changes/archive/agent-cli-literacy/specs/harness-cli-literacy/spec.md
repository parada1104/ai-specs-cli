# harness-cli-literacy Specification

## Purpose

Define always-on agent literacy for operating the public `ai-specs` CLI in
consumer projects: focused bundled skills that teach when and how to run harness
operations, without relying on humans reading README/`ai-specs help`.

## Non-Goals

- Changing public CLI subcommand contracts, flags, or exit codes.
- Shipping literacy primarily as an opt-in catalog recipe.
- MCP wrappers or slash-command surfaces (optional follow-ups).
- Per-project `ai-specs/bin` shims for locating the CLI binary.
- Duplicating the full human README inside skills.

---

## Requirements

### Requirement: Always-on bundled literacy skills

The CLI installation MUST ship three literacy skills under `bundled-skills/`:

- `harness-lifecycle`
- `harness-recipes`
- `harness-skills-deps`

Each MUST include a valid `SKILL.md` that satisfies the skill frontmatter
contract (`metadata.scope` includes `root`, and intent-specific
`metadata.auto_invoke` phrases).

#### Scenario: Bundled skill directories exist

- GIVEN a normal CLI checkout / install tree
- WHEN `bundled-skills/` is inspected
- THEN directories `harness-lifecycle`, `harness-recipes`, and `harness-skills-deps`
  MUST each contain a `SKILL.md`

#### Scenario: Frontmatter is sync-valid

- GIVEN each harness literacy `SKILL.md`
- WHEN skill frontmatter validation runs
- THEN validation MUST succeed for `scope` and `auto_invoke`

---

### Requirement: Refresh-bundled materializes literacy skills into projects

`refresh-bundled` / init / sync MUST copy the three harness literacy skills into
consumer `ai-specs/skills/<name>/` using the same lock-aware rules as other
bundled skills (including `.new` sidecars and user-delete opt-out).

#### Scenario: Fresh project receives literacy skills

- GIVEN a temporary project without prior harness literacy skills
- WHEN `refresh-bundled` (or init path that installs bundled skills) runs
- THEN `ai-specs/skills/harness-lifecycle/SKILL.md`,
  `ai-specs/skills/harness-recipes/SKILL.md`, and
  `ai-specs/skills/harness-skills-deps/SKILL.md` MUST exist

#### Scenario: User-deleted literacy skill is not force-restored

- GIVEN a project lock that records a harness literacy skill as user-removed
  (or the existing permanent opt-out mechanism for deleted bundled skills)
- WHEN `refresh-bundled` runs again
- THEN the CLI MUST honor the existing bundled-skills opt-out semantics
  (no silent forced restore beyond current refresh-bundled behavior)

---

### Requirement: Domain coverage across the three skills

Together, the three skills MUST provide operating guidance for the public
command surface:

- lifecycle: `init`, `sync`, `sync-agent`, `refresh-bundled`, `doctor`,
  `upgrade`, `hub`, and `configure-recipes` as a lifecycle step
- recipes: `recipe list`, `recipe add`, `recipe init`, and recipe-oriented
  `configure-recipes`
- skills/deps: local skill creation posture (cross-link `skill-creator`),
  `skills add`, `skills list`, `skills remove`, `add-dep`, and cross-link
  `skill-sync`

Guidance MUST emphasize order-of-ops and pitfalls, not a dump of README text.

#### Scenario: Recipe add flow is documented

- GIVEN `harness-recipes/SKILL.md`
- WHEN an agent follows the skill for installing a catalog recipe
- THEN the skill MUST describe the sequence `recipe add` → (optional)
  `recipe init` / `configure-recipes` → `sync`

#### Scenario: External dep install is documented

- GIVEN `harness-skills-deps/SKILL.md`
- WHEN an agent follows the skill for adding an external skill dependency
- THEN the skill MUST document `skills add` / `add-dep` and that sync follows

#### Scenario: Sync/doctor lifecycle is documented

- GIVEN `harness-lifecycle/SKILL.md`
- WHEN an agent follows the skill after manifest changes
- THEN the skill MUST document `sync` and `doctor` as primary verification ops

---

### Requirement: Path resolution is a footnote, not a project bin

Literacy skills MAY include a one-line path-resolution footnote using
`command -v ai-specs` with fallback to
`${AI_SPECS_HOME:-$HOME/.ai-specs}/bin/ai-specs`.

Literacy MUST NOT introduce a per-project `ai-specs/bin/ai-specs` shim as the
delivery mechanism.

#### Scenario: No literacy shim under ai-specs/bin

- GIVEN this change's deliverables
- WHEN the repository and recipe templates are inspected for literacy delivery
- THEN there MUST NOT be a new template or materializer that writes
  `ai-specs/bin/ai-specs` solely to teach CLI location

---

### Requirement: Playbook commands stay aligned with the public CLI

Referenced `ai-specs <subcommand>` invocations in the three literacy skills
MUST correspond to subcommands advertised by `bin/ai-specs` help (or aliases
explicitly documented there, e.g. `add-dep`).

#### Scenario: Unknown subcommand references fail CI

- GIVEN the three harness literacy `SKILL.md` files
- WHEN the literacy alignment test runs
- THEN every extracted `ai-specs <cmd>` token MUST be in the known public
  command set derived from `bin/ai-specs` help
