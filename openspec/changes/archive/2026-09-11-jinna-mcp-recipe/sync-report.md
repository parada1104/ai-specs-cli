# Sync Report: jinna-mcp-recipe

- **Status**: PASS — synced (canonical specs updated; change folder kept active, not archived)
- **Date**: 2026-09-11
- **Mode**: `openspec` (file-backed canonical merge)
- **Change root**: `openspec/changes/jinna-mcp-recipe/`
- **Worktree / branch**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/jinna-mcp-recipe` @
  `parada1104/jinna-mcp-recipe`, HEAD `b325df8a422ecc7b0bab447372ea8bf46d93cb0f` (includes merge of
  `development` `f6fe535`)
- **Writes performed by this phase**: `openspec/specs/jinna-mcp-recipe/spec.md` (created) and this
  file. No commits, no pushes, no `ai-specs sync`, no other repository path touched.

## Evidence contract consumed

Source: `openspec/changes/jinna-mcp-recipe/verify-report.md`.

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:9f211b849e3217d515901dc265c92fbf77334238a4777c62c647b3bfa4b47e28
verdict: pass
blockers: 0
critical_findings: 0
requirements: 11/11
scenarios: 29/29
```

The verify report states `Spec coverage total: 11/11 requirements, 29/29 scenarios PASS`, records no
verification blocker, and names no unresolved `FAIL` / `BLOCKED` / `CRITICAL` finding. Its recorded
risks (pre-existing bitbucket golden-test failure on the untouched `development` base, two accepted
test-strength caveats, the pending delivery decision, and unticked `8.5`/`8.6`) are gates and
follow-ups, not sync blockers.

## Native status and `actionContext`

Manual fallback status supplied by the parent (`gentle-ai` CLI not installed; `sdd-status-contract.md`
permits the fallback) was consumed as authoritative: `artifactStore: openspec`,
`artifacts.verifyReport: done`, `artifacts.syncReport: missing`, `applyState: all_done`,
`dependencies.sync: ready`, `nextRecommended: sync`, `isNonAuthoritative: false`,
`taskArtifactErrors: []`, `taskProgress: 41/42` with the single unchecked row
`### [ ] 8.6 Archive before merge` (owned by the archive phase, not by sync).

`actionContext`: `mode: repo-local`, `workspaceRoot` = the worktree above,
`allowedEditRoots: [.../jinna-mcp-recipe/openspec]`. Both write targets are inside that root — no
workspace-planning restriction, no out-of-root path, no ambiguity in active change selection
(`jinna-mcp-recipe` is the only active change folder).

## Domains synced

| Domain | Action | Result |
|--------|--------|--------|
| `jinna-mcp-recipe` | promote delta → `openspec/specs/jinna-mcp-recipe/spec.md` | OK (new canonical capability) |

- **Source delta**: `openspec/changes/jinna-mcp-recipe/specs/jinna-mcp-recipe/spec.md` (350 lines)
- **Canonical target**: `openspec/specs/jinna-mcp-recipe/spec.md` (355 lines, newly created — the
  capability did not exist in the store, which previously held 45 domain specs and now holds 46)

## Delta operations applied

ADDED (all 11 — every requirement is a first-time promotion into a new canonical spec):

1. Declare the provider dependency
2. Detect and reuse the provider
3. Offer an explicit GitHub Release installation
4. Select and verify the provider release
5. Validate archive contents and install atomically
6. Resolve the provider command for MCP materialization
7. Configure OpenProject safely
8. Preserve protocol and operational boundaries
9. Preserve compatibility and explain recovery
10. Evidence and provider readiness gate
11. Validate declared environment values

MODIFIED: none. REMOVED: none. RENAMED: none.

## Sync-record shape (promoted headings, 11 requirements / 29 scenarios)

| Requirement (canonical) | Scenarios promoted |
|-------------------------|--------------------|
| Declare the provider dependency | 3 |
| Detect and reuse the provider | 3 |
| Offer an explicit GitHub Release installation | 3 |
| Select and verify the provider release | 3 |
| Validate archive contents and install atomically | 3 |
| Resolve the provider command for MCP materialization | 3 |
| Configure OpenProject safely | 2 |
| Preserve protocol and operational boundaries | 2 |
| Preserve compatibility and explain recovery | 2 |
| Evidence and provider readiness gate | 2 |
| Validate declared environment values | 3 |
| **Total** | **29** |

## Deliberate restructuring (recorded, not silent)

The canonical spec is a promotion of the change spec into the store's established shape; the only
changes are structural:

1. **Title**: `# Specification: Provider installation and MCP recipe integration` →
   `# jinna-mcp-recipe Specification` (store convention; 35/45 existing canonical specs use
   `<capability> Specification`).
2. **`## Conventions` → `## Purpose`**: the delta's non-requirement front-matter section was condensed
   into the canonical `## Purpose` paragraph. All normative content survived: RFC 2119 terminology, the
   statement that the scenario examples are normative, and the four defined terms `provider`
   (`parada1104/jinna-provider`), `provider binary` (`jinna`), `OpenProject` (remote Self-Hosted
   service), and `MCP host` (AI runtime launching the local stdio server), plus the
   provider-readiness-is-external-prerequisite clause. No normativity was weakened or added.
3. **Numbered requirements → unnumbered canonical headings**: `## Requirement N: <name>` →
   `### Requirement: <name>`. Numbering was dropped because the store keys requirements by exact name;
   all 11 names and their order are preserved.
4. **Scenario heading level bumped**: `### Scenario: <name>` → `#### Scenario: <name>` so scenarios nest
   under their requirement at the canonical depth.
5. **Requirement order preserved** as R1…R11; no requirement was merged, split, reordered, reworded,
   added, or dropped; no scenario was added or removed.
6. **Scenario bullet text preserved verbatim**, including the delta's `**Given** / **When** / **Then** /
   **And**` keyword casing and the fenced ```text``` env-reference block in *Configure OpenProject
   safely*. (Some older canonical specs, `recipe-schema` included, render keywords as
   `**WHEN** / **THEN**`; uppercasing here would be a content edit, so the delta text was carried over
   byte-for-byte.)
7. **`## Requirements` immediately precedes the first `### Requirement:` with no blank line**, matching
   the parent-named reference `openspec/specs/recipe-schema/spec.md`. Note for the record: 34 of the 45
   pre-existing canonical specs do include a blank line there. This is whitespace-only and affects no
   parser or heading-keyed operation. For the same reason, the delta's blank line between each
   `### Requirement:` heading and its normative text was kept (`recipe-schema` places the text on the
   line directly after the heading); both forms render identically and neither is heading- or
   name-keyed.

## Guardrails

- **Legacy flat spec**: not applicable — the change uses the domain layout
  `specs/jinna-mcp-recipe/spec.md`, not a flat `spec.md`.
- **MODIFIED/REMOVED targeting a missing canonical requirement**: none — no MODIFIED or REMOVED
  operation exists.
- **Destructive sync (large REMOVED / large MODIFIED)**: none — this is a pure additive promotion of a
  brand-new capability, so no destructive-sync approval was required or claimed.
- **RENAMED**: absent from the delta, so the unsupported-RENAMED block did not trigger.
- **Same-domain active-change collision**: none. `jinna-mcp-recipe` is the only active change folder
  under `openspec/changes/` in this worktree, and no other active change declares
  `specs/jinna-mcp-recipe/`. No archive/sync ordering decision is needed.
- **Review authority**: none claimed — sync is a canonical-store write, not a review or an approval.

## Checks performed

All checks are read-only greps/diffs run in the worktree; `D` = change delta, `C` = canonical target.

1. `grep -c '^### Requirement: ' $C` → `11`; `grep -c '^#### Scenario: ' $C` → `29` (delta baseline:
   `grep -c '^## Requirement [0-9]*: ' $D` → 11, `grep -c '^### Scenario: ' $D` → 29).
2. Requirement-name set equality —
   `diff <(grep '^## Requirement [0-9]*: ' $D | sed -E 's/^## Requirement [0-9]+: //') <(grep '^### Requirement: ' $C | sed -E 's/^### Requirement: //')`
   → empty output, so the 11 canonical names match the delta's exactly.
3. Scenario-name set equality —
   `diff <(grep '^### Scenario: ' $D | sed 's/^### //') <(grep '^#### Scenario: ' $C | sed 's/^#### //')`
   → empty output, so all 29 scenario names match.
4. Verbatim-content check — `diff` of every non-heading line in the delta body (from the first
   requirement heading) against every non-heading line under `## Requirements` in the canonical spec
   → empty output, proving no normative text was reworded, dropped, or invented.
5. `git status --short` in the worktree → only `openspec/specs/jinna-mcp-recipe/spec.md` (new) and this
   report (new) are added; no tracked file is modified or deleted.

No test or build command was re-run by this phase: sync is a documentation-store promotion and does
not touch production code or tests. Apply-phase evidence remains the verify report's `./tests/run.sh`
/ `./tests/validate.sh` records and the strict-TDD gate above (11/11, 29/29).

## Known limitations carried forward

- Task `8.6 Archive before merge` stays unticked — archive is the next phase's action, and this phase
  is forbidden from moving the change folder.
- The verify report's pre-existing environmental failure
  (`test_apply_progress_omits_absolute_host_and_worktree_paths`) is not a sync blocker and is left to
  its recorded harness follow-up.
- The delivery decision (`ask-on-risk`, ~5.2k changed lines against a 400-line review budget, chained
  PRs recommended) is still pending with the parent; sync neither makes nor defers that decision.

## Next recommended phase

`sdd-archive` — sync is clean, additive, non-destructive, collision-free, and the change folder is
kept active per the sync/archive distinction. Archive-tail (task `8.6`) may proceed once the parent
records the delivery decision; nothing here authorizes a merge or a push.
