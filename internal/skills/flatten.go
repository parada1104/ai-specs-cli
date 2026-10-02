package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// Flatten mirrors flatten-resolved-skills.py's main() after argv parsing:
// both paths are resolved, dest is wiped (shutil.rmtree) and recreated, and
// every resolved skill is copytree'd to dest/<id> in sorted id order.
//
// Exit codes: 0 on success; 1 where Python raises an uncaught traceback
// (rmtree/mkdir/copytree failure — the first failing skill stops the loop,
// leaving the skills copied before it) and when AI_SPECS_HOME is unset (the
// S6 accepted deviation: Python falls back to its module repo root, which has
// no Go source-layout equivalent). Tracebacks are not byte-reproducible, so a
// failure writes one "error: <msg>" line instead. The argv usage branch
// (exit 2) stays with the caller.
func Flatten(projectRoot, destDir, cliHome string, stdout, stderr io.Writer) int {
	if projectcache.AISpecsHome(cliHome) == "" {
		fmt.Fprintln(stderr, "error: AI_SPECS_HOME is not set")
		return 1
	}
	root := projectcache.ResolvePath(projectRoot)
	dest := projectcache.ResolvePath(destDir)

	resolved := CollectSkillsTo(root, cliHome, stderr)

	if _, err := os.Stat(dest); err == nil {
		if err := projectcache.RemoveTreePy(dest); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}
	if err := os.MkdirAll(dest, 0o777); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	ids := make([]string, 0, len(resolved))
	for id := range resolved {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := projectcache.CopyTree(resolved[id].Path, filepath.Join(dest, id)); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "  \u2713 flattened %d skill(s) to %s\n", len(resolved), dest)
	return 0
}
