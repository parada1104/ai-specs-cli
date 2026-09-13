package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

// writeWitness writes raw witness bytes to a temp path and returns that path.
func writeWitness(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "witness.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReadBindingStates pins the four witness states (A4): only bound activates;
// the others are dormant but keep their recorded shape.
func TestReadBindingStates(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantState  string
		wantActive bool
		recipeID   string
		candidates []string
	}{
		{
			name:       "bound activates",
			body:       `{"v":1,"capability":"tracker","state":"bound","recipe_id":"test-tracker-ledger","candidates":[],"written_at":"2026-09-13T00:00:00Z"}`,
			wantState:  WitnessBound,
			wantActive: true,
			recipeID:   "test-tracker-ledger",
		},
		{
			name:       "ambiguous is dormant with candidates",
			body:       `{"v":1,"capability":"tracker","state":"ambiguous","recipe_id":"","candidates":["trello-mcp-workflow","jira-mcp-workflow"]}`,
			wantState:  WitnessAmbiguous,
			wantActive: false,
			candidates: []string{"trello-mcp-workflow", "jira-mcp-workflow"},
		},
		{
			name:       "unbound is dormant",
			body:       `{"v":1,"capability":"tracker","state":"unbound","recipe_id":"","candidates":[]}`,
			wantState:  WitnessUnbound,
			wantActive: false,
		},
		{
			name:       "declared-not-bound is dormant",
			body:       `{"v":1,"capability":"tracker","state":"declared-not-bound","recipe_id":"","candidates":[]}`,
			wantState:  WitnessDeclaredNotBound,
			wantActive: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReadBinding(writeWitness(t, tc.body))
			if got.State != tc.wantState {
				t.Fatalf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Active() != tc.wantActive {
				t.Fatalf("active = %v, want %v", got.Active(), tc.wantActive)
			}
			if got.RecipeID != tc.recipeID {
				t.Fatalf("recipe id = %q, want %q", got.RecipeID, tc.recipeID)
			}
			if len(got.Candidates) != len(tc.candidates) {
				t.Fatalf("candidates = %v, want %v", got.Candidates, tc.candidates)
			}
			for i, want := range tc.candidates {
				if got.Candidates[i] != want {
					t.Fatalf("candidates = %v, want %v", got.Candidates, tc.candidates)
				}
			}
			if got.Reason != "" {
				t.Fatalf("reason = %q, want empty for a readable witness", got.Reason)
			}
		})
	}
}

// TestReadBindingMissingIsDormant pins "missing/unreadable/unknown-v witness is
// dormant with reason witness-missing, never bound and never guessed".
func TestReadBindingMissingIsDormant(t *testing.T) {
	dirPath := t.TempDir() // a directory is an unreadable witness path
	cases := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(t.TempDir(), "witness.json")},
		{"unreadable path", dirPath},
		{"invalid json", writeWitness(t, `{"v":1,`)},
		{"unknown version", writeWitness(t, `{"v":2,"capability":"tracker","state":"bound","recipe_id":"r"}`)},
		{"missing version", writeWitness(t, `{"capability":"tracker","state":"bound","recipe_id":"r"}`)},
		{"other capability", writeWitness(t, `{"v":1,"capability":"jira","state":"bound","recipe_id":"r"}`)},
		{"bound without a recipe id", writeWitness(t, `{"v":1,"capability":"tracker","state":"bound","recipe_id":""}`)},
		{"unknown state", writeWitness(t, `{"v":1,"capability":"tracker","state":"totally-new","recipe_id":"r"}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReadBinding(tc.path)
			if got.Active() {
				t.Fatalf("must never activate on %s: %+v", tc.name, got)
			}
			if got.State != WitnessUnbound {
				t.Fatalf("state = %q, want %q", got.State, WitnessUnbound)
			}
			if got.Reason != ReasonWitnessMissing {
				t.Fatalf("reason = %q, want %q", got.Reason, ReasonWitnessMissing)
			}
			if got.RecipeID != "" {
				t.Fatalf("recipe id = %q, want empty (never guess a provider)", got.RecipeID)
			}
		})
	}
}

// TestWitnessPathIsUnderGitCommonDir pins A3/A4's durable location.
func TestWitnessPathIsUnderGitCommonDir(t *testing.T) {
	common := filepath.Join("/repo", ".git")
	if got := WitnessPath(common); got != filepath.Join(common, "ai-specs", "ledger", "witness.json") {
		t.Fatalf("witness path = %q", got)
	}
}
