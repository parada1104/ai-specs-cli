package planbuild

// The local no-plan declaration (plan-build-openspec-odd-align S19): in
// openspec mode a branch can declare that its change needs no plan. The
// declaration is one git config entry, read and written through git, so it
// never travels and never becomes an artifact.

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	// CeremonyKey is the git config variable of the declaration, under
	// branch.<branch>.
	CeremonyKey = "aiSpecsCeremony"
	// CeremonyNoneValue is the only value that declares "no plan needed".
	CeremonyNoneValue = "none"
)

// ceremonyConfigKey is the full git config key of one branch.
func ceremonyConfigKey(branch string) string {
	return "branch." + branch + "." + CeremonyKey
}

// CeremonyNone reports whether branch declared that its change needs no plan.
// An empty branch, a git failure and any stored value other than none are
// false, so an unreadable declaration never allows a write silently.
func CeremonyNone(root, branch string) bool {
	if !validGitRef(branch) {
		return false
	}
	out, err := gitText(root, "config", "--get", ceremonyConfigKey(branch))
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == CeremonyNoneValue
}

// DeclareCeremonyNone writes the declaration for branch into the local
// repository config. Writing it twice is idempotent.
func DeclareCeremonyNone(root, branch string) error {
	if !validGitRef(branch) {
		return fmt.Errorf("cannot declare a no-plan ceremony for an empty or invalid branch name %q", branch)
	}
	out, err := exec.Command("git", "-C", root, "config", "--local", ceremonyConfigKey(branch), CeremonyNoneValue).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot write %s in %s: %s", ceremonyConfigKey(branch), root, strings.TrimSpace(string(out)))
	}
	return nil
}
