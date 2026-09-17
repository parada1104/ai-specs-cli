package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// gitMemo caches every git fact by (resolved dir, args): repository root, git
// directory, common dir, current branch and submodule records are each derived
// at most once per directory per invocation (spec "Git facts are memoized
// within an invocation"). This is what lets a four-candidate event issue
// strictly fewer git subprocess invocations than the frozen Bash reference,
// which re-derives facts per candidate. A failed lookup caches "" so a
// repeating failure is not re-invoked either.

var gitCache = struct {
	sync.Mutex
	values map[string]string
}{values: map[string]string{}}

func newGitCommand(dir string, args ...string) *exec.Cmd {
	argv := append([]string{"-C", dir}, args...)
	return exec.Command("git", argv...)
}

func git(dir string, args ...string) string {
	out, err := newGitCommand(dir, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRaw(dir string, args ...string) string {
	out, err := newGitCommand(dir, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func gitRawBytes(dir string, args ...string) []byte {
	out, err := newGitCommand(dir, args...).Output()
	if err != nil {
		return nil
	}
	return out
}

func runGit(dir string, args ...string) error {
	return newGitCommand(dir, args...).Run()
}

func execPatchID(patch []byte) string {
	cmd := exec.Command("git", "patch-id", "--stable")
	cmd.Stdin = bytes.NewReader(patch)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func gitMemo(dir string, args ...string) string {
	key := filepath.Clean(dir) + "\x00" + strings.Join(args, "\x00")
	gitCache.Lock()
	if v, ok := gitCache.values[key]; ok {
		gitCache.Unlock()
		return v
	}
	gitCache.Unlock()
	v := git(dir, args...)
	gitCache.Lock()
	gitCache.values[key] = v
	gitCache.Unlock()
	return v
}

func gitCommon(root string) string {
	return gitCommonWith(root, gitMemo)
}

func gitCommonFresh(root string, fact func(string, ...string) string) string {
	return gitCommonWith(root, fact)
}

func gitCommonWith(root string, fact func(string, ...string) string) string {
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
