package cli

import (
	"embed"
	"io"
)

// help holds the byte-exact extraction of the help heredoc from bin/ai-specs
// (1698 bytes). Do not regenerate: the parity contract requires the bytes to
// stay identical.
//
//go:embed help.txt
var help embed.FS

// runHelp prints the embedded help bytes verbatim with exit 0.
func runHelp(stdout io.Writer) int {
	data, err := help.ReadFile("help.txt")
	if err != nil {
		// The file is embedded at build time; this is unreachable in a
		// valid build and mirrors bash `cat` failing under set -e.
		return 1
	}
	_, _ = stdout.Write(data)
	return 0
}
