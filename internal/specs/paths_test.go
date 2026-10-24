package specs

// Behaviour tests for the canonical spec promoter, the port of
// lib/_internal/spec_promotion.py (GO-20 U2e, unit U-A). Each case maps to one
// case of tests/test_spec_promotion.py; the expected bytes come from the Python
// implementation, so the port is compared against its oracle, not against a
// re-description of it.
//
// The promoter writes files, so every case runs on its own t.TempDir() tree:
// openspec/changes/<slug>/specs/<domain>/spec.md (the delta) is composed into
// openspec/specs/<domain>/spec.md (the canonical spec).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fixture vocabulary (the Python module's _requirement/_canonical/_delta) ---

func TestSlugTraversalAndRootEscapeAreRejected(t *testing.T) {
	root := newRepo(t)
	for _, slug := range []string{"../escape", "escape/../..", "/abs"} {
		t.Run(slug, func(t *testing.T) {
			report := PromoteChange(root, slug, false)
			wantOneBlocker(t, report,
				"invalid change slug '"+slug+"': a slug is one path segment with no separators")
		})
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); !os.IsNotExist(err) {
		t.Errorf("promotion escaped the planning root (err = %v)", err)
	}
}

func TestNonPlanningRootIsRejected(t *testing.T) {
	root := t.TempDir()

	report := PromoteChange(root, "any-slug", false)

	wantOneBlocker(t, report, "--root "+root+" is not a planning root: "+
		filepath.Join(root, "openspec")+" is missing; pass the resolved planning root, never the process cwd")
}

func TestSymlinkedCanonicalDomainIsRejected(t *testing.T) {
	root := newRepo(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "openspec", "specs", "capability")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeDelta(t, root, "symlink", "capability", deltaDoc(addedSection(addedBlock)))

	report := PromoteChange(root, "symlink", false)

	if len(report.Blockers) != 1 || !strings.Contains(report.Blockers[0], "escapes the repository planning tree") {
		t.Fatalf("Blockers = %q, want the symlink-escape blocker", report.Blockers)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("promotion wrote through the symlink: %v", entries)
	}
}

func TestSymlinkedOpenspecAncestorIsRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "openspec")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeDelta(t, root, "symlink-ancestor", "capability", deltaDoc(addedSection(addedBlock)))

	report := PromoteChange(root, "symlink-ancestor", false)

	if len(report.Blockers) != 1 || !strings.Contains(report.Blockers[0], "refusing to promote through symlinked path") {
		t.Fatalf("Blockers = %q, want the symlinked-ancestor blocker", report.Blockers)
	}
	entries, err := os.ReadDir(filepath.Join(outside, "specs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("promotion wrote through the symlinked ancestor: %v", entries)
	}
}
