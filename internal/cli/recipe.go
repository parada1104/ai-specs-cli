package cli

// Native `ai-specs recipe list` (GO-20 U6a, T1) — the port of lib/recipe-list.sh
// (49 lines) plus lib/_internal/recipe-list.py (129), which are the contract
// line for line (S4): the usage() heredoc, the argument loop (`--` makes the
// rest positional, an unknown flag and a second positional both exit 2,
// `--help|-h` wins wherever it appears), the `cd … && pwd` target resolution,
// recipe-list.py main()'s guard order (manifest check before the listing), and
// the exact row format of recipe-list.py:120.
//
// The CATALOGUE read is shared, never copied (S6): recipe.CatalogHome applies
// recipe-list.py:30-41's home-first resolution and recipe.List owns the scan,
// the internal `test-`/dot filter, the statuses and the [recipes.*] manifest
// read through internal/config — this file contains only the verb's shell
// contract on top of that reader, so there is no second resolver and no
// second manifest reader.
//
// READ-ONLY (U6a goal 4): nothing here creates a cache, a manifest or an
// `ai-specs/` tree — the manifest is only stat'ed and read.
//
// Documented deviations (the S12/ADR 0002 class, pinned by tests): a `cd`
// failure keeps exit 1 and the `cd: <path>: <reason>` shape but drops bash's
// own `lib/recipe-list.sh: line 47: ` prefix (the U5a deviation), and a
// catalogue that exists but cannot be read words Go's own error where Python
// printed a traceback — both keep the script's exit code.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	homepkg "ai-specs.dev/ai-specs/internal/home"
	"ai-specs.dev/ai-specs/internal/recipe"
)

// recipeListUsage is lib/recipe-list.sh's usage() heredoc byte for byte: a
// QUOTED heredoc (`cat <<'EOF'`), so the text is literal and the trailing
// newline after the last line is part of the output.
const recipeListUsage = `Usage: ai-specs recipe list [path] [--help]
List available recipes in catalog/recipes/ and their installation status.
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// runRecipeList reproduces recipe-list.sh: the argument loop first (usage
// errors exit 2 before anything touches the filesystem), then the cd/pwd
// target resolution, then recipe-list.py main(): the not-initialized guard
// and the rows.
//
// exePath is the running dispatcher's own path (selfPath()); recipe.CatalogHome
// anchors the catalogue resolver's second candidate on it.
func runRecipeList(args []string, home, exePath string, stdout, stderr io.Writer) int {
	// The script's while/case loop, in order: --help|-h wins wherever it
	// appears, `--` makes every remaining argument positional (never a flag),
	// an unknown flag prints two stderr lines, and a second positional is the
	// other exit-2 usage error. An empty-string positional keeps TARGET_PATH
	// empty (`[[ -z ]]`), exactly like the script's take_positional.
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, recipeListUsage)
			return 0
		case arg == "--":
			// Positionals after -- are real arguments, not discardable.
			for i++; i < len(args); i++ {
				if target == "" {
					target = args[i]
					continue
				}
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", args[i])
				return 2
			}
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs recipe list --help' for usage.")
			return 2
		default:
			if target == "" {
				target = arg
			} else {
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
				return 2
			}
		}
	}
	if target == "" {
		if cwd, err := os.Getwd(); err == nil {
			target = cwd
		}
		// A failing `pwd` killed the script under set -e; the cd below
		// reports the same frozen exit 1 instead.
	}
	resolved, err := cdSkillsTarget(target)
	if err != nil {
		fmt.Fprintf(stderr, "cd: %s: %s\n", target, cdReason(err))
		return 1
	}
	target = resolved

	// recipe-list.py main(): the manifest guard runs BEFORE the listing
	// (`if not manifest_path.is_file()`).
	if !regularInitFile(filepath.Join(target, "ai-specs", "ai-specs.toml")) {
		fmt.Fprintln(stderr, "Project not initialized. Run: ai-specs init")
		return 1
	}

	installRoot := home
	if exePath != "" {
		installRoot = homepkg.ResolveHome(exePath, "")
	}
	entries, err := recipe.List(target, recipe.CatalogHome(home, installRoot))
	if err != nil {
		// Python died with a traceback here (exit 1); the script's exit code
		// survives, the wording is ours (ADR 0002).
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "No recipes available.")
		return 0
	}
	for _, e := range entries {
		// recipe-list.py:120 — f"[{status:12s}]  {id:24s}  {version:8s}  {name}"
		fmt.Fprintf(stdout, "[%-12s]  %-24s  %-8s  %s\n", e.Status, e.ID, e.Version, e.Name)
	}
	return 0
}
