# Convert the coupled test suite to black box

## Tracker

- **card_id**: `6a84e7656f8258f4f6897cfe`
- **shortLink**: `POh1vmd6`
- **url**: https://trello.com/c/POh1vmd6
- **list**: In Progress
- **epic**: https://trello.com/c/qwlHQ7Xa

## Planning depth

**standard.** Multi-file refactor in a known area with named files, but it needs
written requirements: the harness this change produces becomes the verification
authority for cards 03-15, so what counts as a valid conversion has to be
written down rather than left to each implementer's judgement.

## Intent

61 of 87 `tests/test_*.py` files load Python modules directly through
`importlib.util.spec_from_file_location` / `load_module`. They assert against
Python function signatures, not CLI behavior, so every one of them dies the
moment its module is rewritten in Go.

Converting them while the Bash/Python implementation is still the only one turns
the suite from a migration liability into an executable parity contract that
does not care which language implements it.

## Measured surface

Measured on `epic/go-single-binary` @ `0d8e621`:

| Metric | Count |
|---|---|
| `tests/test_*.py` total | 87 |
| Coupled via `spec_from_file_location` / `load_module` | 61 |
| Already driving `bin/ai-specs` | 22 (overlaps coupled — hybrids) |
| Shelling out to `lib/*.sh` directly | 12 |
| Using `tests/_cache_paths.py` / `_fixture_catalog.py` | 18 |

The card description's "73 of 103" predates this branch; these are the numbers
the work is scoped against.

## Two couplings the card's acceptance criteria miss

**`lib/*.sh` is not the process boundary.** 12 files invoke `bash lib/<x>.sh`
directly. That satisfies the literal criterion "no `spec_from_file_location`"
while still dying on the Go port, because `lib/*.sh` disappears too. The real
boundary is `bin/ai-specs <verb>`.

**`tests/_cache_paths.py` is itself coupled**, and 18 test files depend on it.
Converting tests that still import it only relocates the coupling.

## Scope

- Shared black-box helpers: hermetic project fixture, CLI invocation, file-tree
  snapshot with symlink kind, normalized output capture, independent cache-key
  derivation.
- Convert coupled tests to drive `bin/ai-specs` and assert on exit code,
  emitted file tree, and output.
- Preserve every existing assertion's intent; where a test only asserted an
  internal call, replace it with the observable effect it stood in for.
- Triage every assertion with no direct observable equivalent, and bring the
  full set to the human as ONE list for a single approval pass.

## Out of scope

- Any Go code.
- Fixing any behavior recorded in `docs/go-migration-parity-contract.md`. Tests
  freeze CURRENT behavior, defects included; D1-D35 are separate cards.
- New behavioral coverage beyond restoring existing intent.
- Deleting any test. No deletion happens without human approval.

## Success Criteria

- Zero test files import `lib/_internal` modules via `spec_from_file_location`
  or `load_module`, except the single cache-key parity assertion, which compares
  against the implementation and is marked as such.
- No test file invokes `lib/*.sh` directly; `bin/ai-specs` is the only entry point.
- `./tests/validate.sh` passes against the unmodified Bash/Python implementation.
- Every assertion with no direct observable equivalent is triaged with a written
  per-file justification, and no such assertion is deleted without human approval.
- The FROZEN surfaces of `docs/go-migration-parity-contract.md` are covered.

## Risk

Category-3 assertions — those with no observable equivalent — are the whole risk.
Deleting them aggressively leaves the harness with less coverage than it claims,
and cards 07-13 then verify against that weaker harness. Nobody finds out until
something breaks in production weeks later, by which point the harness is no
longer evidence of anything.

Mitigation: no deletion is applied without explicit human approval, delivered as
one list in a single pass. Until approved, such tests stay in place carrying a
`# TRIAGE:` marker.

## Branch and merge policy

Per the epic contract: branched from `epic/go-single-binary`, PR targets
`epic/go-single-binary`, never `development` or `main`. Converted incrementally
across chained PRs — the card explicitly forbids landing 61 files in one review.
