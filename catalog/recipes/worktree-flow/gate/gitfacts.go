package main

import (
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

func git(dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	v := gitMemo(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if v == "" {
		v = gitMemo(root, "rev-parse", "--git-common-dir")
	}
	if v == "" {
		return ""
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(root, v)
	}
	return filepath.Clean(v)
}
