package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// runVersion reproduces lib/version.sh: print <home>/VERSION verbatim
// (cat semantics); a missing file degrades to "unknown" with exit 0 either
// way (the unified post-drift behavior, not the old D32 exit-1).
func runVersion(home string, stdout io.Writer) int {
	data, err := os.ReadFile(filepath.Join(home, "VERSION"))
	if err != nil {
		fmt.Fprintln(stdout, "unknown")
		return 0
	}
	_, _ = stdout.Write(data)
	return 0
}
