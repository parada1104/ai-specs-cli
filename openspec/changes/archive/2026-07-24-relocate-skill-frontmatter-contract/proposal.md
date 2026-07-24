# Proposal: relocate `skill-frontmatter` contract alongside its only consumer

Trello: [#52 — fix(skills): relocate skill-frontmatter contract so it ships with every consumer project](https://trello.com/c/FzgJ1UZr)

## Context

`ai-specs/contracts/skill-frontmatter.md` is the human-owned source of truth for
`SKILL.md` frontmatter (says so in its own header). Executable enforcement lives
in `lib/_internal/skill_contract.py`. A formal Given/When/Then mirror lives at
`openspec/specs/skill-frontmatter-contract/spec.md`. This is product-level
governance content about skill authoring, not something specific to
ai-specs-cli's own dogfood project state.

But `ai-specs/contracts/` is **never scaffolded by `ai-specs init` or sync** for
consumer projects — `grep -rn "contracts" lib/init.sh templates/` returns
nothing. The file exists in THIS repo only because someone hand-created a
dogfood copy back in commit `3f607cd feat: enforce skill frontmatter contract`.

Meanwhile, `bundled-skills/skill-creator/SKILL.md` (lines 69 and 85) and
`bundled-skills/skill-creator/assets/SKILL-TEMPLATE.md` (line 42) — all three
distributed to EVERY consumer project via the CLI-bundled skill mechanism
(resolved from `{cache}/.bundled/skills/skill-creator/` per the four-tier
skill-source precedence shipped in `minimal-project-materialization`) —
reference the contract via a relative link `../../contracts/skill-frontmatter.md`
or a literal path `ai-specs/contracts/skill-frontmatter.md`. In any real
consumer project this file never exists, so the reference is **dead for every
project except this one**. The very skill that teaches contributors how to write
skills points at a contract they cannot read.

Sibling governance follow-up `2026-07-24-relocate-bundled-commands`
(`relocate-bundled-commands/proposal.md`, merged) is the structural template
for this change: same tier — deferred migration completion in the same domain,
low architectural ambiguity.

## Objective

Move the human-readable skill-frontmatter contract to a location that
physically **ships with** the bundled `skill-creator` skill so the contract
reference resolves in EVERY consumer project, not just this dogfooding repo,
without adding a new per-project scaffolded path (which would cut against the
`minimal-project-materialization` goal of shrinking committed project surface).

## Scope — In

- **Relocate the contract file** from `ai-specs/contracts/skill-frontmatter.md`
  (dogfood-only, never scaffolded) to a location that travels with the bundled
  skill-creator distribution. Primary candidate (see **D1** below):
  `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`. The
  contract then rides the existing `{cache}/.bundled/skills/skill-creator/`
  distribution with zero new distribution machinery.
- **Update `bundled-skills/skill-creator/SKILL.md`**: line 69 relative link
  (`../../contracts/skill-frontmatter.md` → `assets/skill-frontmatter-contract.md`
  or the new relative path from the SKILL.md location); line 85 literal path
  (`ai-specs/contracts/skill-frontmatter.md` → the new path or a
  skill-local-relative reference).
- **Update `bundled-skills/skill-creator/assets/SKILL-TEMPLATE.md`**: line 42
  relative link (`../../contracts/skill-frontmatter.md` →
  `skill-frontmatter-contract.md` if colocated in `assets/`).
- **Update `openspec/specs/skill-frontmatter-contract/spec.md`**: scenario on
  line 68 currently says `GIVEN ai-specs/contracts/skill-frontmatter.md` — needs
  to reference the new canonical location (or abstract to "the contract
  document shipped with the skill-creator bundled skill" if D1 chooses the
  bundled-skills location).
- **Retire the dogfood copy** at `ai-specs/contracts/skill-frontmatter.md` (see
  **D3**) and remove the now-empty `ai-specs/contracts/` directory from this
  repo.
- Tests that reference the module (`tests/test_skill_contract.py`,
  `tests/test_manifest_contract_docs.py`) do NOT reference the contract PATH
  and need no changes — verified by grep. `lib/_internal/skill_contract.py` is
  a pure parser with no hardcoded contract path; no changes there.
- `docs/`, `AGENTS.md`, `templates/` contain no references to
  `contracts/skill-frontmatter`; no changes there — verified by grep.

## Scope — Out

- **Archived OpenSpec changes** (`openspec/changes/archive/2026-04-25-definir-contrato-frontmatter-skills/`,
  `openspec/changes/archive/2026-04-30-motor-agents-md-runtime-brief/`): these
  contain historical references to the old path in their own
  `spec.md`/`tasks.md`/`verify-report.md`/`design.md`/`proposal.md` files.
  Archive is an immutable audit trail — do not rewrite historical artifacts.
- **The formal spec at `openspec/specs/skill-frontmatter-contract/spec.md`
  content** beyond the one GIVEN-scenario path reference. The spec's
  requirements and scenarios remain unchanged; only the path citation in the
  "Contract documentation and ownership boundaries" requirement's scenario
  needs updating to the new location.
- **Any new per-project scaffolding** of `ai-specs/contracts/` — ruled out by
  D1/D3 below.
- **Executable enforcement logic** in `lib/_internal/skill_contract.py` —
  pure parser, path-agnostic, not in scope.

## Affected modules

| File | Current state | Change |
|------|---------------|--------|
| `ai-specs/contracts/skill-frontmatter.md` | dogfood-only, never scaffolded | **Delete** (canonical copy moves) |
| `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md` | does not exist | **Create** (new canonical home) |
| `bundled-skills/skill-creator/SKILL.md` | lines 69, 85 reference old path | Update both references |
| `bundled-skills/skill-creator/assets/SKILL-TEMPLATE.md` | line 42 references old path | Update reference |
| `openspec/specs/skill-frontmatter-contract/spec.md` | line 68 GIVEN cites old path | Update path citation |
| `ai-specs/contracts/` | directory with one file | Remove (empty after move) |

Not affected (verified by grep): `lib/_internal/skill_contract.py`,
`tests/test_skill_contract.py`, `tests/test_manifest_contract_docs.py`,
`docs/skills-by-agent.md`, `AGENTS.md`, `templates/`.

## Open decisions

- **D1 — Where does the contract physically live?**
  - **Option A (recommended):** `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md` —
    colocated with the only consumer that references it. Rides the existing
    CLI-bundled skill distribution to `{cache}/.bundled/skills/skill-creator/`;
    zero new distribution machinery; relative path from `SKILL-TEMPLATE.md`
    collapses to `skill-frontmatter-contract.md` (same directory), from
    `SKILL.md` becomes `assets/skill-frontmatter-contract.md`. Conceptually
    acceptable because the contract IS about authoring skills, and
    skill-creator IS the skill-authoring skill.
  - **Option B:** New standalone distribution tier
    (e.g. `bundled-contracts/` at CLI repo root, distributed to
    `{cache}/.bundled/contracts/` alongside `.bundled/skills/` and
    `.bundled/commands/`). Architecturally cleanest — contracts are a
    distinct kind of artifact from skills — but adds a new distribution tier,
    new precedence logic, new doctor checks, new tests, for a single file with
    a single consumer. Over-engineered for the scope.
  - **Option C:** CLI-repo-root `docs/skill-frontmatter-contract.md` — does
    NOT ship to consumer projects at all; reference would need to be an
    absolute URL to the upstream GitHub raw path. Breaks offline/airgapped
    consumers and abandons the "ships with the CLI distribution" principle.
    Reject.
  - **Option D:** Materialize `ai-specs/contracts/skill-frontmatter.md` into
    every consumer project via `init.sh`/sync. Directly contradicts
    `minimal-project-materialization`'s commitment to minimizing committed
    project surface. Reject.
  - Recommendation: **Option A**. Same-domain colocating with the only
    consumer, no new governance tiers, no new project surface.

- **D2 — Does the spec make the human-readable markdown redundant?**
  - **Option A (recommended):** Coexist. The spec at
    `openspec/specs/skill-frontmatter-contract/spec.md` is formal Given/When/Then,
    written for governance and enforcement verification — not agent-friendly
    for a contributor authoring a `SKILL.md`. The human-readable markdown is
    agent-facing and contributor-facing; it summarizes the same rules in
    prose. Keep both; the spec is canonical (enforced requirements, scenarios,
    compatibility window), the markdown is a companion (readable summary,
    examples, checklist). Relocate the markdown, do not retire it.
  - **Option B:** Retire the markdown, point `skill-creator` at the spec.
    The spec is in THIS repo only (under `openspec/specs/`), also not
    distributed to consumers — same dead-reference problem, now pointed at a
    different non-shipping file. Reject unless the spec is also distributed,
    which would be a much larger change.
  - **Option C:** Merge into a single artifact that is both spec AND
    agent-facing doc. Mixes governance and tutorial audiences; neither served
    well. Reject.
  - Recommendation: **Option A** — coexist, spec canonical, markdown
    relocated as the agent-facing companion.

- **D3 — What happens to this repo's own `ai-specs/contracts/skill-frontmatter.md`?**
  - **Option A (recommended):** Retire it. Once the canonical copy moves to
    `bundled-skills/skill-creator/assets/`, the dogfood copy becomes a
    second source of truth. Delete the file and the empty
    `ai-specs/contracts/` directory. This repo consumes its own bundled skill
    through the same distribution mechanism as any other consumer — no
    special case needed.
  - **Option B:** Keep it as a redundant local copy "for convenience." Two
    sources of truth that will drift. Reject.
  - **Option C:** Keep it but mark it as "derived from the bundled asset"
    (e.g. a symlink, or a sync-time copy). Adds machinery for no benefit —
    the bundled skill-creator already resolves from the cache tier in this
    repo just like any consumer. Reject.
  - Recommendation: **Option A** — retire, delete directory.

- **D4 — Doctor check for stale `ai-specs/contracts/skill-frontmatter.md` in
  existing consumer projects?**
  - **Option A (recommended):** No doctor check. The contract was NEVER
    scaffolded by init/sync (verified: `grep -rn "contracts" lib/init.sh
    templates/` returns nothing). No consumer project has this file via any
    official mechanism. The only projects that could have it are ones that
    manually copied it (like this dogfood repo) — and those are maintainer
    decisions, not migration targets. No leftover-cleanup needed.
  - **Option B:** Add a doctor WARN that detects and reports the stale path
    if present (mirror of bundled-skill leftover detection). Defensive but
    solves a non-problem — no consumer should have this file, and if a
    maintainer hand-placed it, they know why. Reject as over-engineering.
  - Recommendation: **Option A** — no doctor check, nothing to migrate.

- **D5 — How does the spec's GIVEN scenario reference the contract?**
  - **Option A (recommended):** Update the path citation in
    `openspec/specs/skill-frontmatter-contract/spec.md` line 68 to the new
    canonical location (e.g. `GIVEN the skill-frontmatter contract shipped
    with the bundled skill-creator skill at
    bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`).
    Concrete, verifiable, matches the sibling relocate-bundled-commands
    pattern of keeping specs concrete.
  - **Option B:** Abstract to "GIVEN the canonical skill-frontmatter contract
    document" without a path. More resilient to future moves, but loses the
    verifiable concreteness that makes Given/When/Then scenarios useful.
    Reject.
  - Recommendation: **Option A** — update with the new concrete path.

## Rollback

Revert the diff: restore `ai-specs/contracts/skill-frontmatter.md`, revert
the three reference updates (`SKILL.md`, `SKILL-TEMPLATE.md`, spec.md),
remove the new `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`.
No data migration, no cache state, no lock changes. A consumer project that
already synced the new location continues to resolve from
`{cache}/.bundled/skills/skill-creator/assets/` — rollback on the CLI side
does not affect already-cached consumers until their next sync.

## Success Criteria

- `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md` exists
  and contains the relocated contract content.
- `bundled-skills/skill-creator/SKILL.md` references resolve to the new
  location (lines 69 and 85); the relative link from `SKILL.md` and the
  literal path both point at the shipped asset.
- `bundled-skills/skill-creator/assets/SKILL-TEMPLATE.md` line 42 reference
  resolves to the colocated asset in the same directory.
- `openspec/specs/skill-frontmatter-contract/spec.md` scenario "Contract
  document describes required and generated fields" cites the new canonical
  location.
- `ai-specs/contracts/skill-frontmatter.md` and the empty `ai-specs/contracts/`
  directory are gone from this repo.
- No archived OpenSpec change (`openspec/changes/archive/...`) is modified —
  historical references to the old path remain as the audit trail.
- A consumer project running `ai-specs init` on a fresh directory does NOT
  scaffold any `contracts/` path.
- A contributor reading the bundled `skill-creator` SKILL.md in any consumer
  project can follow the contract reference to an existing, readable file.
- `./tests/validate.sh` passes.

## Classification

`domain_change (proposal → design → spec → tasks)` — same tier as the sibling
`2026-07-24-relocate-bundled-commands`. Same-domain deferred-migration
completion, low architectural ambiguity, single-file relocation with three
reference updates and one spec path citation update.
