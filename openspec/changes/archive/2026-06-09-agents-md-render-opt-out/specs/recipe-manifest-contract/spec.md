## ADDED Requirements

### Requirement: [brief].render controls managed AGENTS.md generation

The manifest `[brief]` table MAY include an optional `render` key of type boolean.
When absent, the default SHALL be `true`. When `render = false`, the project
opts out of managed `AGENTS.md` generation: neither manifest `[brief]` prose nor
enabled recipe `[provides.brief]` fragments SHALL be written to `AGENTS.md` during
sync or init.

The `render` key is independent of other `[brief]` keys: prose fields and
`<section>_mode` keys MAY remain in the manifest for documentation or for use if
rendering is re-enabled later, but they MUST NOT affect `AGENTS.md` on disk while
`render = false`.

#### Scenario: render omitted defaults to enabled

- **GIVEN** a manifest `[brief]` table without a `render` key
- **WHEN** the manifest is validated and sync runs
- **THEN** validation SHALL pass
- **AND** managed AGENTS.md generation SHALL proceed as when `render = true`

#### Scenario: render false disables managed output

- **GIVEN** a manifest declaring:
  ```toml
  [brief]
  render = false
  intro = "Manual project voice."
  workflow_rules = ["This rule must not appear in AGENTS.md while render is false."]
  ```
- **AND** enabled recipes contribute `[provides.brief]` fragments
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST NOT be updated with `intro`, `workflow_rules`, or recipe fragments
- **AND** sync MUST NOT fail solely because `[brief]` contains prose keys

#### Scenario: render true with prose and recipes behaves as today

- **GIVEN** a manifest declaring `[brief] render = true` (or omitting `render`)
- **AND** enabled recipes contribute fragments
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST be generated with merged recipe fragments and manifest prose
- **AND** behavior MUST match the pre-change contract

---

### Requirement: [brief].render validation

The value of `[brief].render` MUST be a TOML boolean (`true` or `false`).
Non-boolean values (including capitalized `True`/`False` or string `"false"`)
SHALL be rejected during validation with an explicit error naming `[brief].render`
and listing the accepted boolean forms.

#### Scenario: Lowercase boolean accepted

- **GIVEN** a manifest declaring `[brief] render = false`
- **WHEN** the manifest is validated
- **THEN** validation SHALL pass

#### Scenario: Invalid boolean rejected

- **GIVEN** a manifest declaring `[brief] render = "false"` (string)
- **WHEN** validation runs (doctor or sync preflight)
- **THEN** validation SHALL fail with an error referencing `[brief].render`
- **AND** the error MUST indicate that a boolean is required

#### Scenario: Capitalized True rejected at parse time

- **GIVEN** a manifest file containing `render = True` (invalid TOML boolean)
- **WHEN** the manifest is parsed
- **THEN** parsing SHALL fail with a TOML decode error
- **OR** if caught by doctor, report an explicit boolean-format guidance message

---

### Requirement: render false propagates to subrepo sync targets

When the root manifest declares `[brief] render = false`, subrepo targets
resolved from `[project].subrepos` MUST inherit the same hands-off policy for
`AGENTS.md`. Subrepo sync MUST NOT invoke managed rendering using the root
manifest's `[brief]` or recipe fragments.

Per-subrepo override of `[brief].render` is out of scope for V1 (subrepos do not
carry their own manifest).

#### Scenario: Root render false applies to subrepo fan-out

- **GIVEN** the root manifest declares `[brief] render = false`
- **AND** `[project].subrepos` includes a wired subrepo path
- **AND** the subrepo has an existing `AGENTS.md`
- **WHEN** root `ai-specs sync` fans out to the subrepo
- **THEN** the subrepo's `AGENTS.md` MUST NOT be regenerated
- **AND** other subrepo artifacts (skills, commands) MUST still sync

---

### Requirement: Doctor guidance for render disabled configurations

`ai-specs doctor` MUST surface configuration guidance when `[brief].render = false`:

- INFO when render is disabled (sync will not update AGENTS.md)
- ERROR when render is disabled and `AGENTS.md` is missing
- WARN when render is disabled and any enabled recipe contributes non-empty
  `[provides.brief]` fragments (dead configuration weight)

#### Scenario: Doctor ERROR when render false and AGENTS.md missing

- **GIVEN** a project with `[brief] render = false`
- **AND** no `AGENTS.md` at the project root
- **WHEN** `ai-specs doctor` runs
- **THEN** doctor MUST report an ERROR for the missing AGENTS.md
- **AND** guidance MUST suggest creating a manual brief or enabling render

#### Scenario: Doctor WARN when recipe fragments unused

- **GIVEN** a project with `[brief] render = false`
- **AND** an enabled recipe declares `[provides.brief]` fragments
- **WHEN** `ai-specs doctor` runs
- **THEN** doctor MUST report a WARN indicating recipe brief fragments will not be applied
- **AND** doctor MUST NOT fail with a non-zero exit solely for this WARN

#### Scenario: Doctor INFO when render disabled with AGENTS.md present

- **GIVEN** a project with `[brief] render = false` and an existing `AGENTS.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** doctor MUST report INFO that managed rendering is disabled
- **AND** doctor exit code MUST remain 0 unless other checks fail
