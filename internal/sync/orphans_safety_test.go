package sync

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

// A refused record must not leave an earlier record's file already deleted
// while the lock still lists it: validation happens before any removal.
func TestPruneRecipeArtifactsRefusalPreservesFilesAndLock(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	const localRel = "ai-specs/recipes/foo/a.sh"
	const escapedRel = "ai-specs/recipes/foo/z/keep.sh"
	local := filepath.Join(root, localRel)
	external := filepath.Join(outside, "keep.sh")
	spineWrite(t, local, "managed launcher", 0o644)
	spineWrite(t, external, "outside project", 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "ai-specs/recipes/foo/z")); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(root, "ai-specs/.ai-specs.lock")
	lk := lock.NewLock()
	lk.Managed[localRel] = map[string]any{
		"recipe": "foo", "kind": "gate", "sha256": lock.Sha256Bytes([]byte("managed launcher")),
	}
	lk.Managed[escapedRel] = map[string]any{
		"recipe": "foo", "kind": "gate", "sha256": lock.Sha256Bytes([]byte("outside project")),
	}
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := planRecipeArtifactPrune(root, nil, nil); err == nil {
		t.Fatal("expected symlinked ancestor to be refused")
	}
	for path, want := range map[string][]byte{
		local: []byte("managed launcher"), external: []byte("outside project"), lockPath: before,
	} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s changed after refusal: got %q, err %v", path, got, err)
		}
	}
}
