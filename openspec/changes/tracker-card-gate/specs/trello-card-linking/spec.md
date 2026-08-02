# trello-card-linking (delta)

## ADDED Requirements

### Requirement: Canonical trello.md link artifact

When the `trello-mcp-workflow` recipe is enabled, every active (non-archive)
OpenSpec change that is linked to a Trello card SHALL persist that link as
`openspec/changes/<slug>/trello.md`. This file is the sole card-link contract
for enforcement surfaces (skill, doctor, gate, evals). The capability MUST NOT
treat a folder-schema field named `trello_card_id` as a real required schema
key; any skill vocabulary that still says `trello_card_id` SHALL mean "the
`card_id` recorded in `trello.md`".

A valid `trello.md` SHALL be frontmatter-free markdown and MUST include
non-empty values for:

- `card_id` (Trello card id)
- `url` (Trello card URL)

Optional keys MAY include `shortLink`, `list`, and `pr`. Parsers SHALL accept
the de-facto bold-key list form:

```markdown
# Trello link

- **card_id**: `<id>`
- **url**: https://trello.com/c/...
```

#### Scenario: Linking writes trello.md with required keys

- **GIVEN** the `trello-mcp-workflow` recipe is enabled
- **AND** a new or existing active change `<slug>` is linked to a Trello card
- **WHEN** the `trello-card-linking` capability completes successfully
- **THEN** `openspec/changes/<slug>/trello.md` MUST exist
- **AND** it MUST contain a non-empty `card_id` value
- **AND** it MUST contain a non-empty `url` value

#### Scenario: Skill vocabulary maps trello_card_id to trello.md

- **GIVEN** recipe skill or command text mentions `trello_card_id`
- **WHEN** an agent resolves where to read or write the linked card id
- **THEN** it MUST use `openspec/changes/<slug>/trello.md`
- **AND** it MUST NOT require a `.openspec.yaml` / folder-schema `trello_card_id` field

### Requirement: Narrow tracker:none exemption

The broad free-skip hatch that allowed an OpenSpec change to exist without a
Trello card is removed. The only documented exemption is an explicit
`tracker:none` marker for the active change.

The exemption marker SHALL be the file
`openspec/changes/<slug>/tracker.none` (the on-disk form of `tracker:none`).
When the marker is created or relied upon, the skill/capability MUST log the
exemption (stderr and/or the recipe warnings log) naming the change slug and
that card linking was intentionally skipped.

A change with a present `tracker.none` marker is exempt from card-link
enforcement that would otherwise require a valid `trello.md`. Archives are
grandfathered and are out of scope for this exemption contract.

#### Scenario: tracker:none allows change without trello.md

- **GIVEN** the `trello-mcp-workflow` recipe is enabled
- **AND** active change `<slug>` has `openspec/changes/<slug>/tracker.none`
- **AND** `openspec/changes/<slug>/trello.md` is absent
- **WHEN** card-link enforcement evaluates the change
- **THEN** the missing `trello.md` MUST NOT be treated as a contract violation
- **AND** the exemption MUST be logged naming `<slug>`

#### Scenario: Declining card creation without tracker:none is not a free pass

- **GIVEN** the `trello-mcp-workflow` recipe is enabled
- **AND** no linked card exists for active change `<slug>`
- **AND** `tracker.none` is absent
- **WHEN** the agent declines to create or link a card
- **THEN** the capability MUST NOT treat that decline as permission to proceed
  without a link artifact
- **AND** the agent MUST either create/link a card and write valid `trello.md`,
  or write the `tracker:none` exemption marker with a logged reason

### Requirement: Availability failure vs missing link artifact

Design Decision #7 ("Trello failures never block") is narrowed. The capability
SHALL distinguish failure classes:

| Failure class | Policy |
|---------------|--------|
| MCP / network / API **unavailable** while attempting create/link/query | degrade: warn and continue; do not claim a link exists |
| Recipe disabled or bootstrap marker absent | card-link hard enforcement inactive |
| **Missing** `trello.md` (and no `tracker:none`) while recipe + marker are active | enforceable by doctor WARN and by the tracker-card gate under `warn`/`always` |
| Explicit `tracker:none` | allow with logged exemption |

Agents MUST NOT classify a missing link artifact as "Trello unavailable".

#### Scenario: MCP unavailable degrades without inventing a link

- **GIVEN** the recipe is enabled and bootstrap marker is present
- **AND** Trello MCP create/link calls fail due to network or API unavailability
- **WHEN** the `trello-card-linking` capability runs
- **THEN** it MUST emit a warning and continue without blocking the session
- **AND** it MUST NOT write a fabricated `trello.md`
- **AND** it MUST NOT claim the change is linked

#### Scenario: Missing artifact is not an availability failure

- **GIVEN** the recipe is enabled and bootstrap marker is present
- **AND** Trello MCP is reachable
- **AND** active change `<slug>` has neither valid `trello.md` nor `tracker.none`
- **WHEN** an enforcement surface evaluates card-link presence
- **THEN** the condition MUST be classified as a missing link artifact
- **AND** MUST NOT be reported as Trello unavailability

### Requirement: Brief rules require link artifact before apply

The `trello-mcp-workflow` recipe `[provides.brief].workflow_rules` SHALL require
that, when the recipe is enabled, agents establish a valid `trello.md` (or an
explicit `tracker:none` exemption) for the active change before apply-time
production work. Brief text MUST name the canonical artifact path
`openspec/changes/<slug>/trello.md`.

#### Scenario: Brief names trello.md precondition

- **GIVEN** the catalog `trello-mcp-workflow` recipe after this change
- **WHEN** `[provides.brief].workflow_rules` is read
- **THEN** the rules MUST require a link artifact (or `tracker:none`) before
  apply-time production work
- **AND** MUST name `trello.md` as the canonical artifact

## MODIFIED Requirements

### Requirement: Create card from template when absent

When no Trello card is linked, the capability SHALL offer to create a card from
a bundled template (or link an existing card). On accept, behavior is unchanged:
create from template in `default_list`, post the initial linking comment, and
persist the link via `trello.md`.

On decline, the capability SHALL NOT allow the change to proceed as an unlinked
change unless the agent writes the `tracker:none` exemption marker and logs it.
(Previously: declining card creation skipped linking and allowed the OpenSpec
change to exist without a Trello card.)

#### Scenario: Offer card creation when no card exists

- **WHEN** no linked Trello card is detected for the current change
- **THEN** the capability SHALL offer the agent a choice to create a card using
  one of the bundled templates (or to link an existing card)

#### Scenario: Card creation from bundled template writes trello.md

- **WHEN** the agent accepts the card creation offer
- **THEN** the capability SHALL create a Trello card using the selected template
  (feature, bug, spike, epic, or handoff)
- **AND** the card SHALL be placed in the configured `default_list`
- **AND** the capability SHALL post an initial linking comment on the new card
- **AND** the capability SHALL write valid `openspec/changes/<slug>/trello.md`

#### Scenario: Decline requires tracker:none instead of silent skip

- **GIVEN** no linked card exists for the change
- **WHEN** the agent declines to create or link a card
- **THEN** the capability MUST require an explicit `tracker:none` exemption
  (`tracker.none` marker) before treating the change as intentionally unlinked
- **AND** MUST log that exemption

## REMOVED Requirements

### Requirement: Skip card creation free pass

**Reason**: Replaced by the narrow `tracker:none` exemption. The previous
scenario under "Create card from template when absent" that allowed declining
card creation and continuing with no Trello card is removed.

#### Scenario: Silent skip no longer permitted

- **GIVEN** the `trello-mcp-workflow` recipe is enabled
- **AND** the agent declines card creation without writing `tracker.none`
- **WHEN** card-link policy is evaluated
- **THEN** the change MUST NOT be considered compliantly unlinked
