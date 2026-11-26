// Package tier is the one derivation of a change's tier: its artifacts decide
// it and no declaration does (change-ledger-v2 S1/S2). Both modes compute the
// same three values here, so an ODD and an OpenSpec change escalate alike.
package tier

// The tier vocabulary. `full` is not a tier any more: it exists only so that a
// stale `Depth: full` line or `--tier full` blocks with the migration message
// instead of an unhelpful "unknown tier".
const (
	None     = "none"
	Light    = "light"
	Standard = "standard"
	Full     = "full"
)

// Derive reads the two artifacts a change can carry: the plan is the artifact
// that asks for ceremony, and a delta alone is a light change because a delta
// with no plan is allowed (S20).
func Derive(hasPlan, hasDelta bool) string {
	switch {
	case !hasPlan && !hasDelta:
		return None
	case hasPlan && hasDelta:
		return Standard
	default:
		return Light
	}
}
