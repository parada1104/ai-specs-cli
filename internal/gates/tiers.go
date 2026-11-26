package gates

// Tiers are derived from the artifacts a change carries (S15), never declared:
// no plan means `none`, which is the guardian's name for an ODD small task; a
// plan alone is `light`; a plan plus a delta is `standard`. A change that
// carries only a delta is `light` as well, because S20 allows a delta with no
// plan.
const (
	TierNone     = "none"
	TierLight    = "light"
	TierStandard = "standard"

	// TierFull is not a tier any more: it exists only so that a stale `Depth:
	// full` line or `--tier full` can block with the migration message instead
	// of an unhelpful "unknown tier".
	TierFull = "full"
)
