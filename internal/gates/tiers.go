package gates

import "ai-specs.dev/ai-specs/internal/tier"

// The tier vocabulary is the shared derivation's: internal/tier owns both the
// values and the rule, so the guardian and its callers read one contract.
const (
	TierNone     = tier.None
	TierLight    = tier.Light
	TierStandard = tier.Standard

	// TierFull is not a tier any more: it exists only so that a stale `Depth:
	// full` line or `--tier full` can block with the migration message instead
	// of an unhelpful "unknown tier".
	TierFull = tier.Full
)
