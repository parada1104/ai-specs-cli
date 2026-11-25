package planbuild

// Commit ancestry (plan-build-openspec-odd-align P3) and the two diff queries
// the refutations of a `none` change need (S16, S17): the recorded commit of a
// plan task is graded by git's exit code, so the answer never depends on how
// git prints it.

import (
	"os/exec"
	"regexp"
	"strings"
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

// riskTrailerRe is the ODD evidence trailer (S17): `Risk: item N (reason)` with N
// in 1..6. A bare `Risk: none` never matches, and the case-insensitivity covers
// `Risk` alone, not the rest of the line.
var riskTrailerRe = regexp.MustCompile(`(?m)^(?i:Risk):[ \t]*item[ \t]+[1-6]\b`)

// ChangedFiles lists the repository-relative paths the branch changes against
// base: the diff from `git merge-base base HEAD`. The second result is false on
// any git failure (no base, shallow clone, not a repository), so no caller can
// mistake a partial answer for a complete one.
func ChangedFiles(root, base string) ([]string, bool) {
	if !validGitRef(base) {
		return nil, false
	}
	mergeBase, err := gitText(root, "merge-base", base, "HEAD")
	if err != nil {
		return nil, false
	}
	mergeBase = strings.TrimSpace(mergeBase)
	if mergeBase == "" {
		return nil, false
	}
	out, err := gitText(root, "diff", "--name-only", mergeBase)
	if err != nil {
		return nil, false
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, true
}

// CommitsWithRiskTrailer reports whether any commit of base..HEAD carries a
// `Risk: item N (<reason>)` trailer. Any git error is false.
func CommitsWithRiskTrailer(root, base string) bool {
	if !validGitRef(base) {
		return false
	}
	out, err := gitText(root, "log", "--format=%B", base+"..HEAD")
	if err != nil {
		return false
	}
	return riskTrailerRe.MatchString(out)
}

// RefExists reports whether ref resolves to a commit in root. Any git error is
// false, so an unresolvable candidate simply falls through to the next one.
func RefExists(root, ref string) bool {
	if !validGitRef(ref) {
		return false
	}
	return exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run() == nil
}

// validGitRef keeps a ref-shaped value from reaching git as an option.
func validGitRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return false
	}
	return !strings.ContainsAny(ref, " \t\r\n")
}

// gitText runs one read-only git command in root and returns its output.
func gitText(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	return string(out), err
}
