package planbuild

// Commit ancestry (plan-build-openspec-odd-align P3) and the two diff queries
// the refutations of a `none` change need (S16, S17): the recorded commit of a
// plan task is graded by git's exit code, so the answer never depends on how
// git prints it.

import (
	"os/exec"
	"regexp"
	"strconv"
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

// riskTrailerRe matches one trailer value of the ODD evidence trailer (S17):
// `item N (reason)`, N in 1..6, with a non-empty reason. Git's trailer parser has
// already separated values from prose, so nothing else can match.
var riskTrailerRe = regexp.MustCompile(`(?i)^item[ \t]+[1-6][ \t]+\(\s*\S[^)]*\)$`)

// DiffEntry is one path of the diff and the status git reports for it, so a
// caller can tell a path this diff contributes from one it removes.
type DiffEntry struct {
	Status string // git's status letter: A added, M modified, D deleted, T type change
	Path   string
}

// Adds reports whether the diff contributes this path: an added or modified
// file. A deletion, or a rename away, is never evidence of anything (S16).
func (entry DiffEntry) Adds() bool {
	return entry.Status == "A" || entry.Status == "M"
}

// DiffEntries lists what the branch changes against base, with git's status for
// each path: the diff from `git merge-base base HEAD`. `-z --no-renames` keeps
// every path raw and unquoted and reports both sides of a rename as a deletion
// plus an addition, so no prefix rule is defeated by git's display conventions.
// The second result is false on any git failure, so no caller can mistake a
// partial answer for a complete one.
func DiffEntries(root, base string) ([]DiffEntry, bool) {
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
	out, err := gitText(root, "diff", "--name-status", "-z", "--no-renames", mergeBase)
	if err != nil {
		return nil, false
	}
	// -z prints `<status> NUL <path> NUL` per entry; a path never contains NUL.
	fields := strings.Split(out, "\x00")
	var entries []DiffEntry
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "" || fields[i+1] == "" {
			continue
		}
		entries = append(entries, DiffEntry{Status: fields[i][:1], Path: fields[i+1]})
	}
	return entries, true
}

// DiffLines counts the changed lines of every path the branch changes against base:
// added plus deleted lines from `git diff --numstat` over the same merge base and
// flags as DiffEntries. A binary file counts zero lines; it still appears, so a
// caller can count its path. The second result is false on any git failure.
func DiffLines(root, base string) (map[string]int, bool) {
	if !validGitRef(base) {
		return nil, false
	}
	mergeBase, err := gitText(root, "merge-base", base, "HEAD")
	if err != nil || strings.TrimSpace(mergeBase) == "" {
		return nil, false
	}
	out, err := gitText(root, "diff", "--numstat", "-z", "--no-renames", strings.TrimSpace(mergeBase))
	if err != nil {
		return nil, false
	}
	// -z prints `<added> TAB <deleted> TAB <path> NUL` per entry; `-` marks a binary.
	lines := map[string]int{}
	for _, record := range strings.Split(out, "\x00") {
		parts := strings.SplitN(record, "\t", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		added, _ := strconv.Atoi(parts[0])
		deleted, _ := strconv.Atoi(parts[1])
		lines[parts[2]] = added + deleted
	}
	return lines, true
}

// UntrackedPaths lists the files of the working tree git does not track and no
// ignore rule covers. DiffEntries compares the merge base against the working
// tree, so it already reports an uncommitted edit to a tracked file; a file the
// branch never added is the divergence only this answers. The second result is
// false on any git failure, so no caller can mistake an unanswered list for an
// empty one.
func UntrackedPaths(root string) ([]string, bool) {
	out, err := gitText(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, false
	}
	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, true
}

// CommitsWithRiskTrailer reports whether any commit of base..HEAD carries a
// `Risk: item N (<reason>)` trailer. Git supplies the values, so only a trailer
// can satisfy it, never prose. Any git error is false.
func CommitsWithRiskTrailer(root, base string) bool {
	if !validGitRef(base) {
		return false
	}
	out, err := gitText(root, "log", "--format=%(trailers:key=Risk,valueonly)", base+"..HEAD")
	if err != nil {
		return false
	}
	for _, value := range strings.Split(out, "\n") {
		if riskTrailerRe.MatchString(strings.TrimRight(value, " \t\r")) {
			return true
		}
	}
	return false
}

// FileAt reads one repository-relative path from a revision (`git show
// <base>:<rel>`), so a policy is graded at the trusted revision, not the
// candidate. false on any failure or unusable ref.
func FileAt(root, base, rel string) (string, bool) {
	if !validGitRef(base) || rel == "" || strings.HasPrefix(rel, "-") {
		return "", false
	}
	out, err := gitText(root, "show", base+":"+rel)
	if err != nil {
		return "", false
	}
	return out, true
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
