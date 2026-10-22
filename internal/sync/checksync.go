package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/cliversion"
	"ai-specs.dev/ai-specs/internal/toml"
)

// CheckSync is the native port of `cli_version.py check-sync`
// (lib/_internal/cli_version.py:238-277). It returns the exit code and the
// stderr text the shim would have written; the caller collapses any nonzero
// code to 1, exactly as the Python exec did.
//
// Exit contract, byte-identical to the Python:
//   - 0 and no stderr: no [tool] policy, or the policy passes.
//   - 0 and one warning line: the policy fails but --ignore-cli-version was
//     passed.
//   - 1 and an "ERROR: ..." line: missing manifest, unparseable [tool], or a
//     failing policy (three lines: reason plus the two guidance lines).
func CheckSync(root, home string, ignoreCliVersion bool) (int, string) {
	// Python resolves both positional paths (Path.resolve) before using them.
	root = resolveSyncPath(root)
	cliHome := resolveSyncPath(home)

	manifestPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	if fi, err := os.Stat(manifestPath); err != nil || !fi.Mode().IsRegular() {
		return 1, fmt.Sprintf("ERROR: %s not found.\n", manifestPath)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return 1, fmt.Sprintf("ERROR: cannot read %s: %v\n", manifestPath, err)
	}
	// Divergence: the Python raises an uncaught traceback on malformed TOML.
	// The port keeps the nonzero exit and prints one diagnostic line instead.
	manifest, err := toml.Parse(data)
	if err != nil {
		return 1, fmt.Sprintf("ERROR: cannot parse %s: %v\n", manifestPath, err)
	}

	policy, policyErr := cliversion.ParseToolPolicy(manifest)
	if policyErr != "" {
		return 1, "ERROR: " + policyErr + "\n"
	}
	if policy == nil {
		return 0, ""
	}

	installed := cliversion.ReadInstalledVersion(cliHome)
	ok, reason := cliversion.CheckPolicy(installed, policy)
	if ok {
		return 0, ""
	}
	if ignoreCliVersion {
		return 0, fmt.Sprintf("⚠ ai-specs: ignoring CLI version policy (%s)\n", reason)
	}
	return 1, fmt.Sprintf("ERROR: %s.\n", reason) +
		"       Run `ai-specs upgrade` or adjust [tool] in ai-specs.toml.\n" +
		"       Pass --ignore-cli-version to sync anyway.\n"
}

// resolveSyncPath resolves symlinks like Python's Path.resolve, falling back to
// the absolute path when the target does not exist yet.
func resolveSyncPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
