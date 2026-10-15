package sync

import (
	"os"
	"strings"
	"testing"
)

// The gate delegation is gone: the doc comments must not still advertise two
// non-native delegations or a future WU3 port (S14a WU3, S5).
func TestDelegationDocCommentsMatchSingleDelegation(t *testing.T) {
	for _, path := range []string{"materialize.go", "materialize_loop.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, stale := range []string{"two deliberate", "replaced natively by WU3"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still contains stale claim %q", path, stale)
			}
		}
	}
}
