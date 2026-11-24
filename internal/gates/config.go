package gates

// Verification commands (plan-build-openspec-odd-align D1): `openspec/config.yaml`
// is an optional project file that OpenSpec and agents may read, but the CLI
// neither requires nor grades it. The OpenSpec CLI is not a dependency of the
// premerge, so the former config validation, the `testing.runner` freshness
// matching and the missing-config blocker are gone.
//
// One rule survives: every command recorded in a `## Verification` row must
// resolve here -- a relative or absolute path must exist on disk, a bare word
// must be on PATH. Light stays advisory, so the rule is graded from standard up.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// checkVerificationCommands resolves the command of every graded row against the
// machine that records it. A row without a command is already refused by the
// plan grammar, so only rows that record one are resolved here.
func checkVerificationCommands(repoRoot, planPath, tier string) []string {
	if tier != TierStandard {
		return nil
	}
	plan, _ := ParsePlan(readText(planPath))
	location := repoRelative(repoRoot, planPath)
	var blockers []string
	for _, row := range plan.Verification {
		// An N/A row records no run (C3).
		if row.Verdict == "N/A" {
			continue
		}
		command, ok := verificationCommand(row.Evidence)
		if !ok {
			continue
		}
		if word := firstWord(command); !commandResolves(repoRoot, word) {
			blockers = append(blockers, location+`: verification for `+row.ID+
				` names command "`+command+`" which does not exist: record the real command`)
		}
	}
	return blockers
}

// repoRelative is the repository-relative path the blockers name.
func repoRelative(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}

// firstWord is the shell word that decides whether a command can run.
func firstWord(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// commandResolves reports whether the first shell word of a recorded command can
// run here: a path is stat'ed inside the repository, a bare word is looked up.
func commandResolves(root, word string) bool {
	if word == "" {
		return false
	}
	if strings.HasPrefix(word, "./") || strings.HasPrefix(word, "/") {
		path := word
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, word)
		}
		info, err := os.Stat(path)
		return err == nil && info.Mode().IsRegular()
	}
	_, err := exec.LookPath(word)
	return err == nil
}

// removedTierBlocker is the migration message for a change that declares the
// full tier this contract removed. It points at standard so nobody is downgraded
// silently.
func removedTierBlocker(location string) string {
	return location + ": the full tier was removed: migrate the change to `Depth: standard` " +
		"(the plan plus at least one `specs/` delta)"
}
