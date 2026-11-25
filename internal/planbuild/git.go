package planbuild

// Commit ancestry (plan-build-openspec-odd-align P3): the recorded commit of a
// plan task is graded by git's exit code, so the answer never depends on how
// git prints it.

import (
	"os/exec"
	"regexp"
)

// commitRe guards the argv: the value comes from a markdown table cell, so only
// a sha shape may reach git.
var commitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// CommitIsAncestor reports whether commit is an ancestor of HEAD in root. Any
// command error, any non-zero exit and an empty or malformed value are false.
func CommitIsAncestor(root, commit string) bool {
	if !commitRe.MatchString(commit) {
		return false
	}
	return exec.Command("git", "-C", root, "merge-base", "--is-ancestor", commit, "HEAD").Run() == nil
}
