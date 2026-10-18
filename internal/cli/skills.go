package cli

// Native `ai-specs skills list` (GO-20 U5a, T1) — the port of
// lib/skills-list.sh (275 lines), which is the contract line for line (S4):
// the usage() heredoc, the argument loop (`--`, unknown flags, the second
// positional, both usage errors exit 2), the `=== ai-specs skills list ===`
// header, the four sections with their exact vocabulary and indentation, and
// the exit codes.
//
// The verb is READ-ONLY (T1/S7): the bundled section resolves through
// internal/projectcache — the ONE cache authority — by path computation only,
// and nothing here creates the cache directory or the project's ai-specs/
// tree. The python heredocs the script shells out to (the front-matter reader
// at skills-list.sh:55, the deps reader, the bundled reader at :145, the
// catalog reader at :208) are ported as internal/skills.Description and
// config.ReadSection(data, "deps") — no second reader is written here.
//
// Documented deviation (deliberate; the frozen exit code is preserved): a
// target that is not an existing directory died inside bash's own `cd` in the
// legacy script (`lib/skills-list.sh: line 48: cd: <path>: <reason>`, exit 1).
// This port keeps exit 1, the `cd: <path>: <reason>` shape and the strerror
// reason, but does not fabricate the script name and line-number prefix — the
// same class of deviation as the doctor port (TestDoctorMissingPathDeviation).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/skills"
	"ai-specs.dev/ai-specs/internal/toml"
)

// skillsListUsage is lib/skills-list.sh's usage() heredoc byte for byte: a
// QUOTED heredoc (`cat <<'EOF'`), so the text is literal and the trailing
// newline after the last line is part of the output.
const skillsListUsage = `Usage: ai-specs skills list [path] [--help]
List vendored ([[deps]]) and local skills for a project.

Shows:
  - Installed deps from ai-specs.toml
  - Bundled skills (CLI-shipped) present in ai-specs/skills/
  - Local skills in ai-specs/skills/ (excluding vendored + bundled)
  - Available catalog skills shipped with the CLI
`

// runSkillsList reproduces skills-list.sh: the argument loop first (usage
// errors exit 2 before anything is printed to stdout), then the header and
// the four sections in script order.
func runSkillsList(args []string, home string, stdout, stderr io.Writer) int {
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
			fmt.Fprint(stdout, skillsListUsage)
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
			fmt.Fprintln(stderr, "Run 'ai-specs skills list --help' for usage.")
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

	tomlPath := filepath.Join(target, "ai-specs", "ai-specs.toml")
	skillsDir := filepath.Join(target, "ai-specs", "skills")
	depsRoot := filepath.Join(target, "ai-specs", ".deps")

	fmt.Fprintln(stdout, "=== ai-specs skills list ===")
	fmt.Fprintf(stdout, "Project: %s\n", target)
	fmt.Fprintln(stdout, "")

	// ── Registered deps from ai-specs.toml ──
	fmt.Fprintln(stdout, "── Registered deps ([[deps]]) ──")
	manifestPresent := regularInitFile(tomlPath)
	var deps []*toml.Table
	depsParsed := false
	if manifestPresent {
		data, err := config.LoadManifest(tomlPath)
		if err != nil {
			// tomllib.TOMLDecodeError branch: diagnostic to stderr, the skip
			// line to stdout, and the listing CONTINUES (the script exits 0
			// from the heredoc). Parse-error detail is diagnostic only (ADR
			// 0002): our parser words its own message.
			fmt.Fprintf(stderr, "  ✗ manifest invalid: %v\n", err)
			fmt.Fprintln(stdout, "  (skipped — fix ai-specs.toml to list deps)")
		} else {
			depsParsed = true
			if section, secErr := config.ReadSection(data, "deps"); secErr == nil {
				deps = depTables(section)
			}
		}
	}
	switch {
	case !manifestPresent:
		fmt.Fprintln(stdout, "  (ai-specs.toml not found — run 'ai-specs init' first)")
	case !depsParsed:
		// both invalid-manifest lines are already out
	case len(deps) == 0:
		fmt.Fprintln(stdout, "  (none)")
	default:
		for _, dep := range deps {
			id := "?"
			if v := depValue(dep, "id"); v != nil {
				id = config.PyStr(v)
			}
			source := "?"
			if v := depValue(dep, "source"); v != nil {
				source = config.PyStr(v)
			}
			subdir := depValue(dep, "path")
			ref := depValue(dep, "ref")
			if !config.Truthy(ref) {
				ref = depValue(dep, "rev")
			}
			// Vendored state: the REAL .deps layout the script checks —
			// a stale copy under ai-specs/skills/ does not count.
			installed := regularInitFile(filepath.Join(depsRoot, id, "skills", id, "SKILL.md"))
			status := "✗ not synced"
			if installed {
				status = "✓ installed"
			}
			fmt.Fprintf(stdout, "  %s\n", id)
			fmt.Fprintf(stdout, "    source:     %s\n", source)
			if config.Truthy(subdir) {
				fmt.Fprintf(stdout, "    subdir:     %s\n", config.PyStr(subdir))
			}
			if config.Truthy(ref) {
				fmt.Fprintf(stdout, "    ref:        %s\n", config.PyStr(ref))
			}
			fmt.Fprintf(stdout, "    status:     %s\n", status)
			if license := depValue(dep, "license"); config.Truthy(license) {
				fmt.Fprintf(stdout, "    license:    %s\n", config.PyStr(license))
			}
			fmt.Fprintln(stdout)
		}
	}
	fmt.Fprintln(stdout, "")

	// Bundled skill ids from the CLI-shipped bundled-skills/ directory; the
	// script falls back to the known pair when that directory is absent (and
	// to NOTHING — not the fallback — when it exists but is empty).
	bundledIDs := []string{"skill-creator", "skill-sync"}
	if bundledSrc := filepath.Join(home, "bundled-skills"); isDirectory(bundledSrc) {
		bundledIDs = localGlobDirs(bundledSrc)
	}

	// ── Bundled skills (CLI-shipped) ──
	// These live in the CLI cache ({cache}/.bundled/skills/), not in the
	// project — the path is derived through projectcache (single cache
	// authority, S7) with no ensure/mkdir anywhere on this route.
	fmt.Fprintln(stdout, "── Bundled skills (CLI-shipped) ──")
	bundledRoot := filepath.Join(projectcache.BundledSkillsRoot(target, home), "skills")
	bundledEntries := bundledCacheDirs(bundledRoot)
	if len(bundledEntries) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	} else {
		for _, name := range bundledEntries {
			fmt.Fprintf(stdout, "  %s\n", name)
			if desc := skills.Description(filepath.Join(bundledRoot, name)); desc != "" {
				fmt.Fprintf(stdout, "    %s\n", desc)
			}
		}
	}
	fmt.Fprintln(stdout, "")

	// ── Local skills in ai-specs/skills/ ──
	fmt.Fprintln(stdout, "── Local skills (ai-specs/skills/) ──")
	if isDirectory(skillsDir) {
		// Registered dep ids to exclude (the script PARSES the manifest here
		// rather than grepping; an invalid manifest silently registers none).
		var registered []string
		if manifestPresent && depsParsed {
			for _, dep := range deps {
				if id, ok := depValue(dep, "id").(string); ok {
					registered = append(registered, id)
				}
			}
		}
		shown := 0
		for _, name := range localGlobDirs(skillsDir) {
			// Skip synced vendored deps and bundled skills: each already has
			// its own section above.
			if slices.Contains(registered, name) || slices.Contains(bundledIDs, name) {
				continue
			}
			shown++
			dir := filepath.Join(skillsDir, name)
			if regularInitFile(filepath.Join(dir, "SKILL.md")) {
				fmt.Fprintf(stdout, "  %s\n", name)
				if desc := skills.Description(dir); desc != "" {
					fmt.Fprintf(stdout, "    %s\n", desc)
				}
			} else {
				fmt.Fprintf(stdout, "  %s  (no SKILL.md)\n", name)
			}
		}
		if shown == 0 {
			fmt.Fprintln(stdout, "  (empty)")
		}
	} else {
		fmt.Fprintln(stdout, "  (not found)")
	}
	fmt.Fprintln(stdout, "")

	// ── Available catalog skills ──
	// Unlike the local glob, a catalog directory WITHOUT SKILL.md is not
	// listed at all (and does not count toward the section's emptiness).
	fmt.Fprintln(stdout, "── Available catalog skills (catalog/skills/) ──")
	catalogDir := filepath.Join(home, "catalog", "skills")
	if isDirectory(catalogDir) {
		count := 0
		for _, name := range localGlobDirs(catalogDir) {
			dir := filepath.Join(catalogDir, name)
			if !regularInitFile(filepath.Join(dir, "SKILL.md")) {
				continue
			}
			count++
			fmt.Fprintf(stdout, "  %s\n", name)
			if desc := skills.Description(dir); desc != "" {
				fmt.Fprintf(stdout, "    %s\n", desc)
			}
		}
		if count == 0 {
			fmt.Fprintln(stdout, "  (empty)")
		}
	} else {
		fmt.Fprintln(stdout, "  (not found)")
	}
	return 0
}

// cdSkillsTarget is the script's `TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"`:
// the argument must be an existing directory (bash's cd checks, errno-style)
// and the result is bash's LOGICAL pwd — an absolute, lexically cleaned path
// with symlinks preserved, which filepath.Abs reproduces.
func cdSkillsTarget(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", syscall.ENOTDIR
	}
	return abs, nil
}

// cdReason is strerror's wording for the cd failure, as bash prints it after
// its own `lib/skills-list.sh: line N: ` prefix (dropped by the deviation).
func cdReason(err error) string {
	switch {
	case os.IsNotExist(err):
		return "No such file or directory"
	case errors.Is(err, syscall.ENOTDIR):
		return "Not a directory"
	case errors.Is(err, syscall.EACCES):
		return "Permission denied"
	default:
		return err.Error()
	}
}

// depValue is dep.get(key): nil only when the key is absent — TOML has no
// null — so the legacy default rules (".get('id', '?')" vs truthiness gates)
// apply verbatim.
func depValue(dep *toml.Table, key string) any {
	v, ok := dep.Get(key)
	if !ok {
		return nil
	}
	return v
}

// depTables flattens config.ReadSection(data, "deps") to the dep tables in
// document order. readDeps is the shared manifest reader this unit reuses
// (config.AppendDepsBlock/RemoveDepSegment are the shared writers); the
// script's own heredoc is the same list shape.
func depTables(section any) []*toml.Table {
	items, ok := section.([]any)
	if !ok {
		return nil
	}
	out := make([]*toml.Table, 0, len(items))
	for _, item := range items {
		if tbl, ok := item.(*toml.Table); ok {
			out = append(out, tbl)
		}
	}
	return out
}

// localGlobDirs is bash's `"$dir"/*/` with its `[[ -d ]]` guard: child
// DIRECTORIES in glob order (hidden names never match a `*` glob), byte
// order under the pinned C collation the differential runs with.
func localGlobDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if isDirectory(filepath.Join(dir, e.Name())) {
			names = append(names, e.Name())
		}
	}
	return names
}

// bundledCacheDirs is the bundled reader's python: sorted(iterdir()) filtered
// to directories that carry a SKILL.md. iterdir() SEES hidden directories
// (this one section is not a glob), and sort.Strings is Python's sorted on
// UTF-8 names (codepoint order).
func bundledCacheDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		child := filepath.Join(root, e.Name())
		if isDirectory(child) && regularInitFile(filepath.Join(child, "SKILL.md")) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
}
