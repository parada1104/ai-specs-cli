package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProveMergedIsTheCleanupsProof pins the read-only surface T7 reports from: the
// branch tip is proven merged only when the cleanup's own local proof holds against a
// resolved base candidate, and an unproven branch stays unmerged so a report never
// invites the removal cleanup would refuse.
func TestProveMergedIsTheCleanupsProof(t *testing.T) {
	root := makeCleanupRepo(t)

	wt := addCleanupWorktree(t, root, "feat-proof")
	if err := os.WriteFile(filepath.Join(wt, "proof.txt"), []byte("proof\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", "-A")
	cleanupGitTest(t, wt, "commit", "-qm", "feat: proof")
	head := cleanupGitTest(t, wt, "rev-parse", "HEAD")

	if proof := ProveMerged(wt, head, "main"); proof.Merged || len(proof.Candidates) == 0 {
		t.Fatalf("proof before the merge = %+v, want a resolved base and no proof", proof)
	}

	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge feat-proof", "feat-proof")
	if proof := ProveMerged(wt, head, "main"); !proof.Merged {
		t.Fatalf("proof after the merge = %+v, want the local proof to hold", proof)
	}

	// No base resolves: nothing is proven, which is the safe direction a report can
	// render as unknown rather than as "not merged".
	if proof := ProveMerged(wt, head, "no-such-base"); proof.Merged || len(proof.Candidates) != 0 {
		t.Fatalf("proof with no base = %+v, want nothing proven", proof)
	}
}
