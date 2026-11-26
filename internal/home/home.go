// Package home resolves the ai-specs installation root (AI_SPECS_HOME).
//
// Resolution order:
//
//  1. an explicit AI_SPECS_HOME wins verbatim, matching the launcher's
//     "${AI_SPECS_HOME:-...}" fallback;
//  2. the launcher path is walked through its symlinks (relative targets joined
//     to the link's directory) and the home is the parent of the resolved
//     file's directory — but only when that guess really is an installation
//     root (it has a VERSION file);
//  3. otherwise the default install root $HOME/.ai-specs, when it is one;
//  4. otherwise the geometric guess, so a binary installed anywhere still
//     returns something usable.
//
// Step 2 alone is the legacy launcher loop:
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
// It is correct for the checkout launcher ($AI_SPECS_HOME/bin/ai-specs) but not
// for an installed binary: install.sh puts a REAL file at $INSTALL_BIN/ai-specs
// (default ~/.local/bin/ai-specs) while the data home stays ~/.ai-specs, so
// BIN_DIR/.. is ~/.local, a directory with no VERSION, no templates and no
// trust anchor. Steps 3 and 4 turn that silent wrong answer into the documented
// default, and $HOME/.ai-specs is the same default the rest of the CLI already
// assumes (see internal/sync/worktreecleanup.go, internal/sync/runtimesnapshot.go
// and upgrade.ExpectedHome).
package home

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// VersionFile marks an installation root; every verb reads it, and its
	// absence is what made a fresh install report "unknown".
	VersionFile = "VERSION"
	// DefaultDirName is the install root install.sh creates under $HOME.
	DefaultDirName = ".ai-specs"
)

// ResolveHome returns the installation root for the launcher at execPath.
// envHome is the AI_SPECS_HOME environment value; when nonempty it is
// returned verbatim.
func ResolveHome(execPath, envHome string) string {
	if envHome != "" {
		return envHome
	}
	guess := filepath.Dir(filepath.Dir(walkSymlinks(execPath)))
	if isHome(guess) {
		return guess
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		if def := filepath.Join(userHome, DefaultDirName); isHome(def) {
			return def
		}
	}
	// Not a home and no default install: keep the legacy answer rather than
	// inventing a path that may not exist.
	return guess
}

// isHome reports whether dir is an ai-specs installation root. The marker is a
// VERSION file holding a single version field: a checkout without one cannot
// serve any verb, while a plain PATH directory such as ~/.local never has one.
//
// The shape matters, not just the name. <home>/dist/VERSION is a cache stamp
// ("<version> <sha256> <size>:<mtime>:<ctime>:<inode>", written by install.sh and
// bin/ai-specs), so an INSTALL_BIN of <home>/dist/bin would otherwise make dist/
// look like the home.
func isHome(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, VersionFile))
	if err != nil {
		return false
	}
	version := strings.TrimSpace(string(data))
	return version != "" && !strings.ContainsAny(version, " \t\n\r")
}

// walkSymlinks resolves the final file behind execPath. The loop must terminate
// on a cycle; cap it far beyond any real chain (the gate module uses the same
// defensive pattern for ELOOP).
func walkSymlinks(execPath string) string {
	cur := execPath
	for range 40 {
		target, err := os.Readlink(cur)
		if err != nil {
			break // not a symlink: final file reached
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cur), target)
		}
		cur = target
	}
	return cur
}
