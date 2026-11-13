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

func requirement(name, body string) string { return requirementScenario(name, body, "behaviour") }

func requirementScenario(name, body, scenario string) string {
	return "### Requirement: " + name + "\n\n" + body + "\n\n#### Scenario: " + scenario + "\n\n" +
		"- **WHEN** " + scenario + "\n- **THEN** " + scenario + "\n"
}

func canonicalDoc(blocks ...string) string {
	return "# capability Specification\n\n## Purpose\n\nExisting purpose.\n\n## Requirements\n\n" +
		strings.Join(blocks, "\n")
}

func deltaDoc(sections ...string) string {
	return "# Delta for capability\n\n" + strings.Join(sections, "\n")
}

func addedSection(blocks ...string) string {
	return "## ADDED Requirements\n\n" + strings.Join(blocks, "\n")
}

func modifiedSection(blocks ...string) string {
	return "## MODIFIED Requirements\n\n" + strings.Join(blocks, "\n")
}

func removedSection(blocks ...string) string {
	return "## REMOVED Requirements\n\n" + strings.Join(blocks, "\n")
}

var (
	existingBlock  = requirement("Existing requirement", "Existing body.")
	untouchedBlock = requirement("Untouched requirement", "Untouched body.")
	addedBlock     = requirement("Added requirement", "Added body.")
)

// --- filesystem helpers ---

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"openspec/changes/archive", "openspec/specs"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func canonicalPath(root, domain string) string {
	return filepath.Join(root, "openspec", "specs", domain, "spec.md")
}

// writeDelta creates openspec/changes/<slug>/specs/<domain>/spec.md and returns
// the change folder.
func writeDelta(t *testing.T, root, slug, domain, text string) string {
	t.Helper()
	dir := filepath.Join(root, "openspec", "changes", slug, "specs", domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "openspec", "changes", slug)
}

func writeCanonical(t *testing.T, root, domain, text string) string {
	t.Helper()
	path := canonicalPath(root, domain)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustPromote(t *testing.T, root, slug string) PromotionReport {
	t.Helper()
	report := PromoteChange(root, slug, false)
	if len(report.Blockers) != 0 {
		t.Fatalf("PromoteChange(%q).Blockers = %q, want none", slug, report.Blockers)
	}
	return report
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// wantOneBlocker asserts the exact single blocker, matching the Python message.
func wantOneBlocker(t *testing.T, report PromotionReport, want string) {
	t.Helper()
	if len(report.Blockers) != 1 || report.Blockers[0] != want {
		t.Fatalf("Blockers = %q, want [%q]", report.Blockers, want)
	}
}

// --- composition -----------------------------------------------------------
