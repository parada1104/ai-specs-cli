# [Go 07.S9] Conflict library — native port of `recipe-conflicts.py`

Card: `[Go 07.S9]` — https://trello.com/c/tVgXq0NT (parent epic `go-single-binary`)
Branch: `change/go-07-s9-conflicts` · Base: `4b1d512` (S8 merge)

## Scope decision

- New root package `internal/conflicts`: `CheckRecipeConflicts` (skill/command/
  mcp collisions), `CheckTagConflicts`, `CheckCapabilityConflicts`, over the
  already-ported `internal/schema` recipe loader.
- The worktree-gate's `tagconflicts.go`/`primitiveconflicts.go` are
  `package main` in a separately released module (plan Q2 = keep delegating):
  they stay, and the Python bridges in `recipe-materialize.py` keep calling
  them. Retiring the bridges belongs to the materialize slices (S11/S14), which
  will call this package instead.
- Not ported: the `recipe-conflicts.py <catalog> <ids>…` CLI (no production
  caller; `recipe-materialize.py:481` imports the module).
- `internal/schema` gains `NewValidationError` so the "recipe directory not
  found" error is the same type as loader errors.

## Evidence

- RED: `undefined: Conflict` before `conflicts.go` existed.
- GREEN: `TestConflictsDifferential` — 10 cases vs the REAL module (all three
  graders on every case; list order, sorted recipe sets, severity, error text):
  no conflicts, skill+command+tag+capability, reversed order, duplicate inside
  one recipe (registration stops at first collision), mcp collision, explicit
  binding silencing a warning, duplicate explicit binding (fatal, early
  return), bindings with missing keys, missing recipe dir, invalid recipe.
- Mutations killed 6/6 (no break on collision, claim by id instead of name,
  same-recipe duplicate tag, never-fatal tag, duplicate binding continues,
  skipped dir check).
- `go vet ./... && go test ./...` rc 0; `./tests/validate.sh` → see PR.
