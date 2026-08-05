# Delta for Plan-Build-Flow

## MODIFIED Requirements

### Requirement: Change depth classifier

The bundled skill SHALL classify each substantial request into exactly one
planning depth before production edits:

- **Full** — explore → proposal → spec → design → tasks
- **Standard** — spec → tasks (explore/proposal/design optional)
- **Light** — tasks only

Classification SHALL compute a **signal** tier from size/scope heuristics AND
separately detect an **explicit user depth request** when the user names a tier
or clearly equivalent planning depth (including common English and Spanish
phrasings such as "full SDD", "flujo completo", "solo tasks", "tasks only").

The **decided** depth MUST be recorded in `tasks.md`. Direct implementation
verbs on a request with no existing change folder MUST NOT skip planning.

#### Scenario: Full depth for ambiguous scope

- GIVEN a request for a new cross-cutting capability with unclear boundaries
- AND the user did not state a conflicting explicit depth
- WHEN planning starts
- THEN the full planning chain runs
- AND tier minimum artifacts exist before build

#### Scenario: Light depth for scoped fix

- GIVEN a one-file bug fix with an explicit file and expected edit
- AND the user did not state a conflicting explicit depth
- WHEN planning starts
- THEN only `tasks.md` is required
- AND no production code is modified during planning

#### Scenario: Direct implement still plans first

- GIVEN the user says "implement X" with no `openspec/changes/<slug>/` folder
- WHEN the skill evaluates the request
- THEN it classifies depth and runs the plan phase before build
- AND stops for authorization unless the tier is trivially light and inline build is allowed

## ADDED Requirements

### Requirement: Adversarial depth conflict detection

When an explicit user depth request is present, the skill MUST compare it to the
signal tier. If they differ, the skill MUST treat the situation as a **depth
conflict** and MUST NOT silently adopt either value as decided.

Matching request and signal is not a conflict; the skill MAY proceed with that
shared tier.

Absence of an explicit user depth request MUST preserve today's signal-only
classification behavior.

#### Scenario: Explicit request conflicts with signal

- GIVEN the user asks for full planning ("flujo completo SDD" or equivalent)
- AND size/scope signals indicate Standard
- WHEN the classifier runs
- THEN a depth conflict is detected
- AND neither Full nor Standard is silently recorded as decided without resolution

#### Scenario: Explicit request matches signal

- GIVEN the user asks for Light / "solo tasks"
- AND size/scope signals also indicate Light
- WHEN the classifier runs
- THEN no conflict ask is required
- AND planning proceeds at Light

#### Scenario: No explicit request

- GIVEN the user describes work without naming a depth tier
- WHEN the classifier runs
- THEN the signal tier is used as decided
- AND adversarial conflict handling does not block planning

### Requirement: Conflict ask before planning chain

On a depth conflict, the skill MUST ask the user which depth to use (requested
vs signal) before writing the planning artifacts for the decided tier, unless
the same user turn already answered which side wins.

The ask SHOULD briefly state both values and MAY recommend one, but the user
choice (or an explicit same-turn resolution) decides. Until resolution, the
skill MUST NOT implement production code and MUST NOT pretend the conflict is
settled.

#### Scenario: Ask fires on conflict

- GIVEN a depth conflict is detected
- AND the user has not yet chosen requested vs signal
- WHEN planning would otherwise start writing tier artifacts
- THEN the agent asks which depth to use
- AND stops until the user answers

#### Scenario: Same-turn resolution skips repeat ask

- GIVEN the user says both the work description and which depth wins
  (e.g. "use full even if it looks standard")
- WHEN the classifier detects requested ≠ signal
- THEN it adopts the stated resolution without a second ask
- AND records annotation as decided by user

### Requirement: Depth resolution annotation

Whenever a depth conflict was detected (including same-turn resolution),
`tasks.md` MUST annotate at least:

- requested depth
- signal depth
- decided depth
- decision source (`user` or equivalent wording)

The decided depth MUST also appear in the ordinary `Depth: …` line (or
equivalent single decided-tier record) used by existing plan-build consumers.

When there was no conflict, existing `Depth: <tier>` recording remains
sufficient; optional confirmation annotation is allowed but not required.

#### Scenario: Conflict annotated after user chooses requested

- GIVEN requested=Full, signal=Standard, user chooses Full
- WHEN `tasks.md` is written
- THEN it records Depth as Full
- AND it records requested, signal, decided, and that the user decided

#### Scenario: Conflict annotated after user chooses signal

- GIVEN requested=Full, signal=Standard, user chooses Standard
- WHEN `tasks.md` is written
- THEN it records Depth as Standard
- AND it records requested, signal, decided, and that the user decided

### Requirement: Higher decided tier completes its chain

If conflict resolution selects a deeper tier than the signal, the skill MUST
run (or complete) the planning chain required by the decided tier before build
authorization is considered satisfied for that change.

#### Scenario: Upgrade from Standard signal to Full decision

- GIVEN signal was Standard but decided depth is Full
- WHEN planning continues after resolution
- THEN Full-tier minimum artifacts are produced before build
- AND production code remains unmodified during planning
