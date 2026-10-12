package shared

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// GitCommonDir resolves the absolute Git common directory of root, or "" when
// root is not inside a repository. It is the `gitCommonWith` rule moved from the
// gate main package (gitfacts.go) as part of GO-07 S13 PR1, so the shared
// witness writer and the main package's memoized seam share one rule.
//
// GitCommonDir uses a plain, unmemoized git subprocess executor; the gate main
// package keeps its memoized gitMemo seam and calls GitCommonDirWith instead.
func GitCommonDir(root string) string {
	return GitCommonDirWith(root, gitFact)
}

// GitCommonDirWith resolves the Git common directory using the supplied git fact
// function. The main package injects its memoizing gitMemo here.
func GitCommonDirWith(root string, fact func(string, ...string) string) string {
	v := fact(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if v == "" {
		v = fact(root, "rev-parse", "--git-common-dir")
	}
	if v == "" {
		return ""
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(root, v)
	}
	return filepath.Clean(v)
}

// gitFact is the default git fact function: one git -C <dir> subprocess, the
// trimmed stdout on success and "" on any failure.
func gitFact(dir string, args ...string) string {
	argv := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", argv...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
