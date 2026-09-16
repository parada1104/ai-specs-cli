package ledger

import (
	"reflect"
	"testing"
	"time"
)

// rNow is the supplied clock every reconciliation case uses. The comparator is
// pure, so no test may read the wall clock.
var rNow = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// rInput is the agreeing baseline: bound provider/scope/item, a fresh
// observation, and two declared properties that both hold.
func rInput() ReconcileInput {
	return ReconcileInput{
		ProviderID: "recipe-a",
		Scope:      "scope-1",
		ItemID:     "remote-1",
		Now:        rNow,
		MaxAge:     time.Hour,
		Expected: []Expectation{
			{Name: "stage", Value: "review"},
			{Name: "finished", Value: "false"},
		},
		Observed: &Observation{
			ProviderID: "recipe-a",
			Scope:      "scope-1",
			ItemID:     "remote-1",
			ObservedAt: rNow.Add(-time.Minute).Format(time.RFC3339),
			Properties: map[string]string{"stage": "review", "finished": "false"},
		},
	}
}

// TestReconcileOutcomes is the T1 acceptance table: equal, different and missing
// declared properties, identity/scope/provider mismatches, unbound identity,
// unavailable observation, malformed/future/stale timestamps, the declared
// window edge, the disabled window, and the deterministic check precedence.
func TestReconcileOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(in *ReconcileInput)
		outcome  string
		findings []Finding
	}{
		{
			name:    "equal declared properties agree",
			mutate:  func(*ReconcileInput) {},
			outcome: ReconcileAgree,
		},
		{
			name: "an undeclared observed property is ignored",
			mutate: func(in *ReconcileInput) {
				in.Observed.Properties["undeclared"] = "whatever"
			},
			outcome: ReconcileAgree,
		},
		{
			name: "a different declared property does not agree",
			mutate: func(in *ReconcileInput) {
				in.Observed.Properties["stage"] = "done"
			},
			outcome: ReconcilePropertyMismatch,
			findings: []Finding{{
				Property: "stage", Expected: "review", Observed: "done", Status: FindingDifferent,
			}},
		},
		{
			name: "a declared property observed as empty is different, not missing",
			mutate: func(in *ReconcileInput) {
				in.Observed.Properties["stage"] = ""
			},
			outcome: ReconcilePropertyMismatch,
			findings: []Finding{{
				Property: "stage", Expected: "review", Status: FindingDifferent,
			}},
		},
		{
			name: "a missing declared property does not agree",
			mutate: func(in *ReconcileInput) {
				delete(in.Observed.Properties, "finished")
			},
			outcome: ReconcileMissingProperty,
			findings: []Finding{{
				Property: "finished", Expected: "false", Status: FindingMissing,
			}},
		},
		{
			name: "missing and different findings are ordered by property name",
			mutate: func(in *ReconcileInput) {
				in.Expected = []Expectation{
					{Name: "zeta", Value: "z"},
					{Name: "beta", Value: "b"},
					{Name: "alpha", Value: "a"},
				}
				in.Observed.Properties = map[string]string{"zeta": "not-z", "alpha": "a"}
			},
			outcome: ReconcileMissingProperty,
			findings: []Finding{
				{Property: "beta", Expected: "b", Status: FindingMissing},
				{Property: "zeta", Expected: "z", Observed: "not-z", Status: FindingDifferent},
			},
		},
		{
			name:    "a provider mismatch does not agree",
			mutate:  func(in *ReconcileInput) { in.Observed.ProviderID = "recipe-b" },
			outcome: ReconcileIdentityMismatch,
			findings: []Finding{{
				Property: FacetProvider, Expected: "recipe-a", Observed: "recipe-b", Status: FindingDifferent,
			}},
		},
		{
			name:    "a scope mismatch does not agree",
			mutate:  func(in *ReconcileInput) { in.Observed.Scope = "scope-2" },
			outcome: ReconcileIdentityMismatch,
			findings: []Finding{{
				Property: FacetScope, Expected: "scope-1", Observed: "scope-2", Status: FindingDifferent,
			}},
		},
		{
			name:    "an item mismatch does not agree even when the properties match",
			mutate:  func(in *ReconcileInput) { in.Observed.ItemID = "remote-2" },
			outcome: ReconcileIdentityMismatch,
			findings: []Finding{{
				Property: FacetItem, Expected: "remote-1", Observed: "remote-2", Status: FindingDifferent,
			}},
		},
		{
			name:    "an unbound provider never agrees",
			mutate:  func(in *ReconcileInput) { in.ProviderID = "" },
			outcome: ReconcileUnboundIdentity,
			findings: []Finding{{
				Property: FacetProvider, Status: FindingUnbound,
			}},
		},
		{
			name:    "an unbound scope never agrees",
			mutate:  func(in *ReconcileInput) { in.Scope = "" },
			outcome: ReconcileUnboundIdentity,
			findings: []Finding{{
				Property: FacetScope, Status: FindingUnbound,
			}},
		},
		{
			name:    "an unbound item never agrees",
			mutate:  func(in *ReconcileInput) { in.ItemID = "" },
			outcome: ReconcileUnboundIdentity,
			findings: []Finding{{
				Property: FacetItem, Status: FindingUnbound,
			}},
		},
		{
			name:    "an unavailable observation never agrees",
			mutate:  func(in *ReconcileInput) { in.Observed = nil },
			outcome: ReconcileUnavailable,
		},
		{
			name: "a malformed observed-at never agrees",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = "not-a-timestamp"
			},
			outcome: ReconcileInvalidObservedAt,
		},
		{
			name:    "a missing observed-at never agrees",
			mutate:  func(in *ReconcileInput) { in.Observed.ObservedAt = "" },
			outcome: ReconcileInvalidObservedAt,
		},
		{
			name: "an observed-at after the supplied clock never agrees",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = rNow.Add(time.Second).Format(time.RFC3339)
			},
			outcome: ReconcileFutureObservedAt,
		},
		{
			name: "an observation older than the declared window never agrees",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = rNow.Add(-2 * time.Hour).Format(time.RFC3339)
			},
			outcome: ReconcileStaleObservation,
		},
		{
			name: "an observation exactly at the window edge still agrees",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = rNow.Add(-time.Hour).Format(time.RFC3339)
			},
			outcome: ReconcileAgree,
		},
		{
			name: "an observation at the supplied clock still agrees",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = rNow.Format(time.RFC3339)
			},
			outcome: ReconcileAgree,
		},
		{
			name:    "a zero supplied clock is invalid input",
			mutate:  func(in *ReconcileInput) { in.Now = time.Time{} },
			outcome: ReconcileInvalidClock,
		},
		{
			name:    "a zero freshness window is invalid input",
			mutate:  func(in *ReconcileInput) { in.MaxAge = 0 },
			outcome: ReconcileInvalidWindow,
		},
		{
			name:    "a negative freshness window is invalid input",
			mutate:  func(in *ReconcileInput) { in.MaxAge = -time.Hour },
			outcome: ReconcileInvalidWindow,
		},
		{
			name:    "no declared expectations are invalid input",
			mutate:  func(in *ReconcileInput) { in.Expected = nil },
			outcome: ReconcileInvalidDeclarations,
		},
		{
			name: "an empty declared property name is invalid input",
			mutate: func(in *ReconcileInput) {
				in.Expected = append(in.Expected, Expectation{Name: "", Value: "review"})
			},
			outcome: ReconcileInvalidDeclarations,
		},
		{
			name: "an identical duplicate declared property name is invalid input",
			mutate: func(in *ReconcileInput) {
				in.Expected = []Expectation{
					{Name: "stage", Value: "review"},
					{Name: "stage", Value: "review"},
				}
			},
			outcome: ReconcileInvalidDeclarations,
		},
		{
			name: "a conflicting duplicate declared property name is invalid input",
			mutate: func(in *ReconcileInput) {
				in.Expected = []Expectation{
					{Name: "stage", Value: "review"},
					{Name: "stage", Value: "done"},
				}
			},
			outcome: ReconcileInvalidDeclarations,
		},
		{
			name: "an empty declared value is a legitimate value",
			mutate: func(in *ReconcileInput) {
				in.Expected = []Expectation{{Name: "stage", Value: ""}}
				in.Observed.Properties = map[string]string{"stage": ""}
			},
			outcome: ReconcileAgree,
		},
		{
			name: "an empty declared value observed differently does not agree",
			mutate: func(in *ReconcileInput) {
				in.Expected = []Expectation{{Name: "stage", Value: ""}}
				in.Observed.Properties = map[string]string{"stage": "done"}
			},
			outcome: ReconcilePropertyMismatch,
			findings: []Finding{{
				Property: "stage", Observed: "done", Status: FindingDifferent,
			}},
		},
		{
			name: "an invalid clock outranks invalid declarations",
			mutate: func(in *ReconcileInput) {
				in.Now = time.Time{}
				in.Expected = nil
			},
			outcome: ReconcileInvalidClock,
		},
		{
			name: "an invalid window outranks invalid declarations",
			mutate: func(in *ReconcileInput) {
				in.MaxAge = 0
				in.Expected = nil
			},
			outcome: ReconcileInvalidWindow,
		},
		{
			name: "an unbound identity outranks an invalid clock",
			mutate: func(in *ReconcileInput) {
				in.ProviderID = ""
				in.Now = time.Time{}
			},
			outcome:  ReconcileUnboundIdentity,
			findings: []Finding{{Property: FacetProvider, Status: FindingUnbound}},
		},
		{
			name: "invalid declarations outrank an unavailable observation",
			mutate: func(in *ReconcileInput) {
				in.Expected = nil
				in.Observed = nil
			},
			outcome: ReconcileInvalidDeclarations,
		},
		{
			name: "an identity mismatch outranks a malformed observed-at",
			mutate: func(in *ReconcileInput) {
				in.Observed.ItemID = "remote-2"
				in.Observed.ObservedAt = "not-a-timestamp"
			},
			outcome: ReconcileIdentityMismatch,
			findings: []Finding{{
				Property: FacetItem, Expected: "remote-1", Observed: "remote-2", Status: FindingDifferent,
			}},
		},
		{
			name: "a malformed observed-at outranks a declared property",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = "not-a-timestamp"
				in.Observed.Properties["stage"] = "done"
			},
			outcome: ReconcileInvalidObservedAt,
		},
		{
			name: "a stale observation outranks a declared property",
			mutate: func(in *ReconcileInput) {
				in.Observed.ObservedAt = rNow.Add(-2 * time.Hour).Format(time.RFC3339)
				in.Observed.Properties["stage"] = "done"
			},
			outcome: ReconcileStaleObservation,
		},
		{
			name: "a missing declared property outranks a different one",
			mutate: func(in *ReconcileInput) {
				in.Observed.Properties["stage"] = "done"
				delete(in.Observed.Properties, "finished")
			},
			outcome: ReconcileMissingProperty,
			findings: []Finding{
				{Property: "finished", Expected: "false", Status: FindingMissing},
				{Property: "stage", Expected: "review", Observed: "done", Status: FindingDifferent},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := rInput()
			tc.mutate(&in)
			got := Reconcile(in)
			if got.Outcome != tc.outcome {
				t.Fatalf("outcome = %q, want %q (%+v)", got.Outcome, tc.outcome, got)
			}
			if !reflect.DeepEqual(got.Findings, tc.findings) {
				t.Fatalf("findings = %+v, want %+v", got.Findings, tc.findings)
			}
			if got.Agreed() != (tc.outcome == ReconcileAgree) {
				t.Fatalf("Agreed() = %v with outcome %q", got.Agreed(), got.Outcome)
			}
		})
	}
}

// TestReconcileOrderIsDeterministic pins that reordering the declared
// expectations cannot change either the outcome or the finding order.
func TestReconcileOrderIsDeterministic(t *testing.T) {
	in := rInput()
	in.Expected = []Expectation{{Name: "beta", Value: "1"}, {Name: "alpha", Value: "1"}}
	in.Observed.Properties = map[string]string{"alpha": "2", "beta": "2"}

	first := Reconcile(in)
	in.Expected[0], in.Expected[1] = in.Expected[1], in.Expected[0]
	second := Reconcile(in)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("declared order changed the result: %+v vs %+v", first, second)
	}
	if first.Outcome != ReconcilePropertyMismatch {
		t.Fatalf("outcome = %q, want %q", first.Outcome, ReconcilePropertyMismatch)
	}
	if len(first.Findings) != 2 || first.Findings[0].Property != "alpha" || first.Findings[1].Property != "beta" {
		t.Fatalf("findings = %+v, want alpha then beta", first.Findings)
	}
}

// TestReconcileWithoutDeclaredProperties pins that a comparison with nothing
// declared cannot validate anything and is invalid input, never agreement.
func TestReconcileWithoutDeclaredProperties(t *testing.T) {
	in := rInput()
	in.Expected = nil
	in.Observed.Properties = nil
	got := Reconcile(in)
	if got.Outcome != ReconcileInvalidDeclarations {
		t.Fatalf("outcome = %q, want %q", got.Outcome, ReconcileInvalidDeclarations)
	}
	if len(got.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", got.Findings)
	}
}

// TestReconcileDuplicateDeclarationOrderIsDeterministic pins that a repeated
// property name is invalid input regardless of where it appears, so validation
// never depends on the declaration order.
func TestReconcileDuplicateDeclarationOrderIsDeterministic(t *testing.T) {
	orders := [][]Expectation{
		{{Name: "alpha", Value: "1"}, {Name: "beta", Value: "1"}, {Name: "alpha", Value: "1"}},
		{{Name: "alpha", Value: "1"}, {Name: "alpha", Value: "1"}, {Name: "beta", Value: "1"}},
		{{Name: "beta", Value: "1"}, {Name: "alpha", Value: "1"}, {Name: "alpha", Value: "1"}},
	}
	for _, expected := range orders {
		in := rInput()
		in.Expected = expected
		in.Observed.Properties = map[string]string{"alpha": "1", "beta": "1"}
		got := Reconcile(in)
		if got.Outcome != ReconcileInvalidDeclarations {
			t.Fatalf("declarations %+v: outcome = %q, want %q", expected, got.Outcome, ReconcileInvalidDeclarations)
		}
		if len(got.Findings) != 0 {
			t.Fatalf("declarations %+v: findings = %+v, want none", expected, got.Findings)
		}
	}
}
