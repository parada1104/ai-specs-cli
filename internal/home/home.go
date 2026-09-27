// Package home resolves the ai-specs installation root (AI_SPECS_HOME).
//
// It mirrors the launcher loop in bin/ai-specs:
//
//	SOURCE="${BASH_SOURCE[0]}"
//	while [[ -L "$SOURCE" ]]; do
//	    DIR="$(cd "$(dirname "$SOURCE")" && pwd)"
//	    SOURCE="$(readlink "$SOURCE")"
//	    [[ "$SOURCE" != /* ]] && SOURCE="$DIR/$SOURCE"
//	done
//	BIN_DIR="$(cd "$(dirname "$SOURCE")" && pwd)"
//	AI_SPECS_HOME="${AI_SPECS_HOME:-$(cd "$BIN_DIR/.." && pwd)}"
//
// i.e. an explicit AI_SPECS_HOME wins verbatim; otherwise the launcher path
// is walked through its symlinks (relative targets joined to the link's
// directory) and the home is the parent of the resolved file's directory.
package home

import (
	"os"
	"path/filepath"
)

// ResolveHome returns the installation root for the launcher at execPath.
// envHome is the AI_SPECS_HOME environment value; when nonempty it is
// returned verbatim, matching the bash "${AI_SPECS_HOME:-...}" fallback.
func ResolveHome(execPath, envHome string) string {
	if envHome != "" {
		return envHome
	}
	cur := execPath
	// Walk symlinks exactly like the bash readlink loop. The loop must
	// terminate on a cycle; cap it far beyond any real chain (the gate
	// module uses the same defensive pattern for ELOOP).
	for i := 0; i < 40; i++ {
		target, err := os.Readlink(cur)
		if err != nil {
			break // not a symlink: final file reached
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cur), target)
		}
		cur = target
	}
	// home = parent of the final file's directory (BIN_DIR/..).
	return filepath.Dir(filepath.Dir(cur))
}
