package sync

// The gate provenance contract is user-facing documentation, not only code: the
// worktree-flow README, the trello-mcp-workflow README and docs/recipes-catalog.md
// describe when a gate is refreshed, preserved or backed up. The legacy suite
// pinned that prose (tests/test_override_ownership.py::
// test_gate_provenance_policy_is_documented); it moves here with the behavior it
// documents, which is native (shared.HookGateActuator).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateProvenanceDocsStayAccurate(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var surface strings.Builder
	for _, rel := range []string{
		"catalog/recipes/worktree-flow/README.md",
		"catalog/recipes/trello-mcp-workflow/README.md",
		"docs/recipes-catalog.md",
	} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		surface.WriteString(strings.ToLower(string(raw)))
		surface.WriteString("\n")
	}
	docs := surface.String()
	for _, phrase := range []string{
		"gate provenance",
		"records a baseline",
		"byte mismatch",
		"missing baseline",
		"ai-specs sync --refresh-gates",
		"cache-only immutable backup",
		"runtime hook scripts are no longer rewritten unconditionally",
	} {
		if !strings.Contains(docs, phrase) {
			t.Errorf("the gate provenance docs no longer state %q", phrase)
		}
	}
	// The retired claim: runtime hooks are NOT rewritten unconditionally.
	if strings.Contains(docs, "always rewritten") {
		t.Error(`the docs still claim runtime hooks are "always rewritten"`)
	}
}
