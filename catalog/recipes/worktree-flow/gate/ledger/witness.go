package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// WitnessVersion is the only witness schema version this build reads. An
	// unknown version is dormant, never interpreted (A4).
	WitnessVersion = 1

	// CapabilityTracker is the one capability this ledger grades (D1).
	CapabilityTracker = "tracker"

	// Witness states. Only bound activates the ledger (D6).
	WitnessBound            = "bound"
	WitnessAmbiguous        = "ambiguous"
	WitnessUnbound          = "unbound"
	WitnessDeclaredNotBound = "declared-not-bound"

	// ReasonWitnessMissing marks a dormant binding caused by a missing,
	// unreadable, or unknown-version witness (A4).
	ReasonWitnessMissing = "witness-missing"
)

// Binding is the normalized activation outcome read from the witness.
type Binding struct {
	State      string
	RecipeID   string
	Candidates []string
	// Reason is set only when dormancy came from an unusable witness file.
	Reason string
}

// Active reports whether the witness activates the ledger. Only the bound state
// activates; a declaration alone is supply, not activation (D6).
func (b Binding) Active() bool { return b.State == WitnessBound }

// witness is the on-disk witness written by sync (A4).
type witness struct {
	V          int      `json:"v"`
	Capability string   `json:"capability"`
	State      string   `json:"state"`
	RecipeID   string   `json:"recipe_id"`
	Candidates []string `json:"candidates"`
	WrittenAt  string   `json:"written_at"`
}

// WitnessPath is the durable witness location: <git-common-dir>/ai-specs/ledger/witness.json (A4).
func WitnessPath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledger", "witness.json")
}

// ReadBinding decodes the witness at path read-only. A missing, unreadable,
// unknown-version, wrong-capability, or incomplete witness is dormant with
// reason witness-missing; the ledger never re-derives catalog bindings and never
// guesses a provider (A4, D6).
func ReadBinding(path string) Binding {
	data, err := os.ReadFile(path)
	if err != nil {
		return dormant()
	}
	var decoded witness
	if err := json.Unmarshal(data, &decoded); err != nil {
		return dormant()
	}
	if decoded.V != WitnessVersion || decoded.Capability != CapabilityTracker {
		return dormant()
	}
	switch decoded.State {
	case WitnessBound:
		// A bound witness without a recipe id names no provider, so it cannot
		// activate anything: guessing one is exactly what D6 forbids.
		if decoded.RecipeID == "" {
			return dormant()
		}
		return Binding{State: WitnessBound, RecipeID: decoded.RecipeID, Candidates: decoded.Candidates}
	case WitnessAmbiguous, WitnessUnbound, WitnessDeclaredNotBound:
		return Binding{State: decoded.State, RecipeID: decoded.RecipeID, Candidates: decoded.Candidates}
	default:
		return dormant()
	}
}

// dormant is the unusable-witness outcome: inactive, unbound, never guessed.
func dormant() Binding {
	return Binding{State: WitnessUnbound, Reason: ReasonWitnessMissing}
}
