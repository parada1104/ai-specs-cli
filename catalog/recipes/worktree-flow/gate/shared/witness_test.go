package shared

import (
	"testing"
	"time"
)

// TestWitnessWrittenAtIsUTCSecondPrecision pins the instant format the Python
// writer produced with strftime("%Y-%m-%dT%H:%M:%SZ") — UTC at second precision —
// where the surviving contract now lives: the shared witness producer.
//
// Coverage note (GO-20 U2e T5 PR4a S5): the retired
// tests/test_binding_bridge.py pinned this format through the bridge flag, and
// the only other Go assertion lived in the gate's package-main wrapper test,
// which dies with --resolve-bindings in PR4b. The expectations are the Python
// bridge's cases verbatim.
func TestWitnessWrittenAtIsUTCSecondPrecision(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "utc", now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), want: "2026-01-02T03:04:05Z"},
		{name: "offset is converted to utc", now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("minus3", -3*3600)), want: "2026-01-02T06:04:05Z"},
		{name: "sub-second precision is truncated", now: time.Date(2026, 1, 2, 3, 4, 5, 999999999, time.UTC), want: "2026-01-02T03:04:05Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WitnessWrittenAt(tc.now); got != tc.want {
				t.Fatalf("WitnessWrittenAt(%v) = %q, want %q", tc.now, got, tc.want)
			}
		})
	}
}
