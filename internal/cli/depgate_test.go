package cli

// The native CLI-dep gate's CLI half (odd/tasks/go-20-u7-dep-gate.md T3).
// internal/deps owns the gate and its own suite; what is left to pin here is
// the one answer depgate.go invents on its own — the adapter for `recipe add`
// and `configure-recipes` needs a terminal it can open, and a stdin that is
// not a file has none. That arm replaced S37's refusal: the driver answered
// rc 3 there, and the native gate has no driver to answer with.

import (
	"bytes"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

// TestRecipeDepGateNeedsFileStdin pins recipeDepGate's refusal over a
// non-*os.File stdin (a pipe, a strings.Reader, a test): the outcome carries
// the error and not Proceed, so every caller words it through its existing
// failure arm instead of prompting on a terminal that does not exist.
func TestRecipeDepGateNeedsFileStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	outcome := recipeDepGate(t.TempDir(), strings.NewReader(""), &stdout, &stderr, &schema.Recipe{})
	if outcome.Proceed {
		t.Errorf("Proceed = true over a non-file stdin, want false: %+v", outcome)
	}
	if outcome.Err == nil {
		t.Fatal("Err = nil, want the terminal refusal")
	}
	if want := "the CLI-dep gate needs a terminal stdin"; outcome.Err.Error() != want {
		t.Errorf("Err = %q, want %q", outcome.Err, want)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("the refusal must print nothing before it returns: stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
}
