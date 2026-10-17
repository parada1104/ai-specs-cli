package tui

// GO-20 U3b-2 advisory remediation (odd/tasks/go-20-u3b-advisories.md). The panel
// pads with fmt's %-*s, whose width counts RUNES, so a byte-based measurement made
// the box wider than its own border as soon as a row carried a non-ASCII rune.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestRenderPanelAlignsNonASCIIRows pins R3-5 (S5): every line of the box has the
// same display width, whatever the rows contain.
func TestRenderPanelAlignsNonASCIIRows(t *testing.T) {
	cases := map[string]PanelData{
		"non-ascii label and text": {
			Version:  "1.0",
			Target:   "/tmp/café",
			Topology: "monorepo",
			Counts:   Counts{OK: 2, Warn: 1},
			Checks:   []PanelCheck{{Label: "topología", Text: "redirigido…"}},
		},
		"ascii only": {
			Version: "1.0",
			Target:  "/tmp/x",
			Counts:  Counts{OK: 3},
		},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(strings.TrimRight(RenderPanel(data, true), "\n"), "\n")
			want := utf8.RuneCountInString(lines[0])
			for _, line := range lines[1:] {
				if got := utf8.RuneCountInString(line); got != want {
					t.Errorf("line width %d, want %d (border): %q", got, want, line)
				}
			}
		})
	}
}
