package ledger

import (
	"sort"
	"time"
)

// Remote reconciliation is provider-neutral: the core compares declared
// properties, never provider vocabulary (A7). A recipe declares the property
// names and values it expects and the transport supplies one observation of the
// remote item. Nothing here reads Item.Status, opens the store, or writes the
// remote side: the comparator is pure and its clock is supplied.
const (
	// ReconcileAgree is the only outcome in which the observation may be treated
	// as matching the bound identity and the declared expectations.
	ReconcileAgree = "agree"

	// ReconcileUnboundIdentity: the bound side lacks provider, scope or item, so
	// the observation cannot be bound to anything. It never agrees.
	ReconcileUnboundIdentity = "unbound-identity"

	// ReconcileInvalidClock: the supplied clock is zero, so freshness cannot be
	// validated. It never agrees, and it is never a disabled check.
	ReconcileInvalidClock = "invalid-clock"

	// ReconcileInvalidWindow: the declared freshness window is not positive, so
	// freshness cannot be validated. It never agrees, and it is never a disabled
	// check.
	ReconcileInvalidWindow = "invalid-window"

	// ReconcileInvalidDeclarations: the declared expectations are unusable — no
	// declaration at all, an empty property name, or a repeated property name. It
	// never agrees, whatever the declaration order.
	ReconcileInvalidDeclarations = "invalid-declarations"

	// ReconcileUnavailable: the transport returned no observation. It never agrees.
	ReconcileUnavailable = "unavailable"

	// ReconcileIdentityMismatch: the observation belongs to another provider,
	// scope or item. It never agrees.
	ReconcileIdentityMismatch = "identity-mismatch"

	// ReconcileInvalidObservedAt: the observation carries no parseable RFC3339
	// observed-at stamp. It never agrees.
	ReconcileInvalidObservedAt = "invalid-observed-at"

	// ReconcileFutureObservedAt: the observation is dated after the supplied
	// clock, so it cannot be trusted. It never agrees.
	ReconcileFutureObservedAt = "future-observed-at"

	// ReconcileStaleObservation: the observation is older than the declared
	// freshness window. It never agrees.
	ReconcileStaleObservation = "stale-observation"

	// ReconcileMissingProperty: a declared property was not observed. It never agrees.
	ReconcileMissingProperty = "missing-property"

	// ReconcilePropertyMismatch: a declared property was observed with another
	// value. It never agrees.
	ReconcilePropertyMismatch = "property-mismatch"
)

// Identity facet names a finding may carry. They are neutral labels for the
// bound identity, never provider vocabulary.
const (
	FacetProvider = "provider"
	FacetScope    = "scope"
	FacetItem     = "item"
)

// Finding statuses, one per non-agreeing finding.
const (
	// FindingDifferent: the value was observed but is not the declared one.
	FindingDifferent = "different"
	// FindingMissing: the property was declared but not observed at all.
	FindingMissing = "missing"
	// FindingUnbound: the bound facet is absent, so nothing could be compared.
	FindingUnbound = "unbound"
)

// Expectation is one declared property requirement: a recipe-declared property
// name and the value the remote item must show. Values compare as exact strings,
// so a provider-neutral canonical string is the caller's contract. An empty value
// is a legitimate declared value; an empty or repeated name is invalid input.
type Expectation struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Observation is one provider-neutral snapshot of the remote item as reported by
// the transport. Properties carries declared property names; any undeclared key
// is ignored. ObservedAt is the RFC3339 stamp the transport saw and is never
// generated here.
type Observation struct {
	ProviderID string            `json:"provider_id"`
	Scope      string            `json:"scope"`
	ItemID     string            `json:"item_id"`
	ObservedAt string            `json:"observed_at"`
	Properties map[string]string `json:"properties"`
}

// ReconcileInput is one comparison. The bound facets come from the witness and
// the linked item; Expected comes from the recipe and is never inferred from
// Item.Status. Now is the supplied clock and MaxAge the required freshness
// window: a zero Now and a non-positive MaxAge are invalid input, never a
// disabled check. Expected must declare at least one property, each with a
// non-empty, unique name.
type ReconcileInput struct {
	ProviderID string
	Scope      string
	ItemID     string
	Expected   []Expectation
	Observed   *Observation
	Now        time.Time
	MaxAge     time.Duration
}

// Finding is one non-agreeing comparison result. Property is either a declared
// property name or an identity facet name.
type Finding struct {
	Property string `json:"property"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
	Status   string `json:"status"`
}

// ReconcileResult is the deterministic outcome of one comparison. Outcome is the
// single classification. Findings carries the per-item detail of the identity and
// declared-property outcomes: identity facets in provider/scope/item order, then
// declared properties in property-name order. It is empty when Outcome is
// ReconcileAgree and for every outcome that classifies the whole input or
// observation rather than individual items (the invalid-input outcomes,
// unavailable, and the observed-at outcomes). Unbound identity findings name
// each absent bound facet.
type ReconcileResult struct {
	Outcome  string    `json:"outcome"`
	Findings []Finding `json:"findings"`
}

// Agreed reports whether the observation matched the bound identity and every
// declared expectation.
func (r ReconcileResult) Agreed() bool { return r.Outcome == ReconcileAgree }

// Reconcile compares one transport observation against the bound identity and the
// declared expectations. It is pure: the clock is supplied, nothing is read or
// written, and no remote value is ever inferred. The check order is fixed —
// unbound identity, invalid clock, invalid window, invalid declarations,
// unavailable observation, identity, observed-at, staleness, then declared
// properties — so the same input always yields the same outcome whatever the
// declaration order, and a missing, stale, malformed, future, unavailable or
// invalid input never agrees.
func Reconcile(in ReconcileInput) ReconcileResult {
	if in.ProviderID == "" || in.Scope == "" || in.ItemID == "" {
		return ReconcileResult{Outcome: ReconcileUnboundIdentity, Findings: unboundFindings(in)}
	}
	// Freshness and declarations are validated before any comparison: an input
	// that cannot validate them never reaches agreement. There is no disabled
	// freshness mode and no existence-only mode.
	if in.Now.IsZero() {
		return ReconcileResult{Outcome: ReconcileInvalidClock}
	}
	if in.MaxAge <= 0 {
		return ReconcileResult{Outcome: ReconcileInvalidWindow}
	}
	if !validExpectations(in.Expected) {
		return ReconcileResult{Outcome: ReconcileInvalidDeclarations}
	}
	if in.Observed == nil {
		return ReconcileResult{Outcome: ReconcileUnavailable}
	}
	if findings := identityFindings(in, in.Observed); len(findings) > 0 {
		return ReconcileResult{Outcome: ReconcileIdentityMismatch, Findings: findings}
	}
	observedAt, err := time.Parse(time.RFC3339, in.Observed.ObservedAt)
	if err != nil {
		return ReconcileResult{Outcome: ReconcileInvalidObservedAt}
	}
	if observedAt.After(in.Now) {
		return ReconcileResult{Outcome: ReconcileFutureObservedAt}
	}
	if in.Now.Sub(observedAt) > in.MaxAge {
		return ReconcileResult{Outcome: ReconcileStaleObservation}
	}
	findings := propertyFindings(in.Expected, in.Observed.Properties)
	if len(findings) == 0 {
		return ReconcileResult{Outcome: ReconcileAgree}
	}
	return ReconcileResult{Outcome: propertyOutcome(findings), Findings: findings}
}

// validExpectations reports whether the declared expectations form a usable set:
// at least one declaration, every name present, and no repeated name. The check
// is order-independent, so a repeated name is invalid wherever it appears.
func validExpectations(expected []Expectation) bool {
	if len(expected) == 0 {
		return false
	}
	seen := make(map[string]bool, len(expected))
	for _, exp := range expected {
		if exp.Name == "" || seen[exp.Name] {
			return false
		}
		seen[exp.Name] = true
	}
	return true
}

// unboundFindings names every bound facet that is absent, in facet order.
func unboundFindings(in ReconcileInput) []Finding {
	var findings []Finding
	for _, facet := range []struct{ name, bound string }{
		{FacetProvider, in.ProviderID},
		{FacetScope, in.Scope},
		{FacetItem, in.ItemID},
	} {
		if facet.bound == "" {
			findings = append(findings, Finding{Property: facet.name, Status: FindingUnbound})
		}
	}
	return findings
}

// identityFindings reports every observation facet that differs from the bound
// one, in facet order.
func identityFindings(in ReconcileInput, obs *Observation) []Finding {
	var findings []Finding
	for _, facet := range []struct{ name, bound, observed string }{
		{FacetProvider, in.ProviderID, obs.ProviderID},
		{FacetScope, in.Scope, obs.Scope},
		{FacetItem, in.ItemID, obs.ItemID},
	} {
		if facet.bound != facet.observed {
			findings = append(findings, Finding{
				Property: facet.name, Expected: facet.bound, Observed: facet.observed, Status: FindingDifferent,
			})
		}
	}
	return findings
}

// propertyFindings reports the declared properties that were not observed or hold
// another value. An undeclared observed key is ignored, and the result is sorted
// by property name so the ordering never depends on the declared order or on Go's
// map iteration.
func propertyFindings(expected []Expectation, observed map[string]string) []Finding {
	var findings []Finding
	for _, exp := range expected {
		got, ok := observed[exp.Name]
		switch {
		case !ok:
			findings = append(findings, Finding{Property: exp.Name, Expected: exp.Value, Status: FindingMissing})
		case got != exp.Value:
			findings = append(findings, Finding{
				Property: exp.Name, Expected: exp.Value, Observed: got, Status: FindingDifferent,
			})
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Property < findings[j].Property })
	return findings
}

// propertyOutcome classifies property findings: a missing declaration outranks a
// different value, so the outcome is derived from the findings alone.
func propertyOutcome(findings []Finding) string {
	for _, f := range findings {
		if f.Status == FindingMissing {
			return ReconcileMissingProperty
		}
	}
	return ReconcilePropertyMismatch
}
