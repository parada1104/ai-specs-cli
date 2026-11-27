package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPlanIdentityReadNeverMintsTheCreationToken is the read-only contract of T7
// (L5-6): a read must leave the git admin directory byte-identical, so the creation
// token is minted by the write path only. Both forms are derived from the same
// checkout, so the read form's difference is exactly the file it did not write.
func TestPlanIdentityReadNeverMintsTheCreationToken(t *testing.T) {
	dir := repo(t)
	token := filepath.Join(dir, ".git", createdTokenFile)

	read := PlanIdentityReadAt(dir, nil)
	if _, err := os.Stat(token); !os.IsNotExist(err) {
		t.Fatalf("read form wrote %s (stat err = %v)", token, err)
	}
	if read.Created != "" {
		t.Fatalf("read form minted token %q, want no token: a read that mints makes a report a write", read.Created)
	}
	if read.CommonDir == "" || read.Root == "" || read.Branch == "" {
		t.Fatalf("read form = %+v, want the identity facts a report needs", read)
	}

	written := PlanIdentityAt(dir, nil)
	if written.Created == "" {
		t.Fatalf("write form = %+v, want the minted token", written)
	}
	if data, err := os.ReadFile(token); err != nil || string(data) != written.Created+"\n" {
		t.Fatalf("token file = %q (%v), want %q", data, err, written.Created)
	}

	// Once the write path minted the token, a read answers the same identity: the
	// declaration a write recorded is the one a later report finds.
	again := PlanIdentityReadAt(dir, nil)
	if again.Key() != written.Key() {
		t.Fatalf("read key = %q, want the written key %q", again.Key(), written.Key())
	}
}
