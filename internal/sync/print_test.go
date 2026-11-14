package sync

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeTemp writes content to a fresh temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPrintStepOutputCompactDropsGlyphsAndBlanks pins the FROZEN compact
// contract of print_step_output: blank lines and lines whose first
// non-whitespace char is ✓ · ⇢ ▸ are dropped; every other non-blank line is
// replayed byte-identically (including its leading whitespace) on its stream.
func TestPrintStepOutputCompactDropsGlyphsAndBlanks(t *testing.T) {
	path := writeTemp(t, "    ✓ bundled skill worktree-flow\n"+
		"    · symlink ok\n"+
		"    ⇢ flattened 1\n"+
		"    ▸ recipe session-context\n"+
		"\n"+
		"   \n"+
		"keep-me\n"+
		"  ! warning\n"+
		"  ✗ error\n"+
		"  ℹ notice\n")
	var out bytes.Buffer
	printStepOutput(&out, path, false)
	want := "keep-me\n  ! warning\n  ✗ error\n  ℹ notice\n"
	if out.String() != want {
		t.Errorf("compact output = %q, want %q", out.String(), want)
	}
}

// TestPrintStepOutputVerboseIsByteExact pins the FROZEN verbose contract:
// the capture is replayed byte-for-byte, including trailing blank lines.
func TestPrintStepOutputVerboseIsByteExact(t *testing.T) {
	path := writeTemp(t, "detail\n\n\n")
	var out bytes.Buffer
	printStepOutput(&out, path, true)
	if out.String() != "detail\n\n\n" {
		t.Errorf("verbose output = %q, want %q", out.String(), "detail\n\n\n")
	}
}

// TestPrintStepOutputEmptyAndMissingAreSilent matches the `[[ -s file ]]`
// guard: an empty or missing capture prints nothing.
func TestPrintStepOutputEmptyAndMissingAreSilent(t *testing.T) {
	empty := writeTemp(t, "")
	for _, path := range []string{empty, filepath.Join(t.TempDir(), "nope")} {
		for _, verbose := range []bool{false, true} {
			var out bytes.Buffer
			printStepOutput(&out, path, verbose)
			if out.Len() != 0 {
				t.Errorf("path=%q verbose=%v: output = %q, want empty", path, verbose, out.String())
			}
		}
	}
}

// TestPrintStepOutputFinalLineWithoutNewline mirrors bash `read`'s
// `|| [[ -n "$line" ]]` tail handling: a final line without a newline is
// still replayed, with one newline appended by printf.
func TestPrintStepOutputFinalLineWithoutNewline(t *testing.T) {
	path := writeTemp(t, "a\nb")
	var out bytes.Buffer
	printStepOutput(&out, path, false)
	if out.String() != "a\nb\n" {
		t.Errorf("output = %q, want %q", out.String(), "a\nb\n")
	}
	// A lone newline is non-empty but carries no printable line.
	path = writeTemp(t, "\n")
	out.Reset()
	printStepOutput(&out, path, false)
	if out.Len() != 0 {
		t.Errorf("lone newline output = %q, want empty", out.String())
	}
}
