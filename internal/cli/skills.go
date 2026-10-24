package cli

// Native `ai-specs skills list` (GO-20 U5a, T1), `ai-specs skills remove`
// (GO-20 U5b, T3) and `ai-specs skills add` / `add-dep` + the skills
// dispatcher (GO-20 U5c, T5) — the ports of lib/skills-list.sh (275 lines),
// lib/skills-remove.sh (217 lines) and lib/skills-add.sh (222 lines) plus
// lib/skills.sh's case statement (39), which are the contract line for line
// (S4): the usage() heredocs, the argument loops (`--`, unknown flags, extra
// positionals, both usage errors exit 2), the exact output vocabulary and
// indentation, and the exit codes.
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
	"regexp"
	"slices"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/skills"
	"ai-specs.dev/ai-specs/internal/sync"
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

// skillsRemoveUsage is lib/skills-remove.sh's usage() heredoc byte for byte:
// a QUOTED heredoc (`cat <<'EOF'`), so the text is literal and the trailing
// newline after the last line is part of the output (stdout for --help,
// stderr after the missing-id error).
const skillsRemoveUsage = `Usage: ai-specs skills remove <id> [path] [--help]
Remove a vendored skill ([[deps]]) from ai-specs.toml.

Removes the [[deps]] block and prunes the vendored copy under
ai-specs/.deps/<id>/ (gitignored).

Arguments:
  id      Skill identifier matching [[deps]].id
  path    Target project root (default: current directory)
`

// runSkillsRemove reproduces skills-remove.sh: the argument loop first (usage
// errors exit 2 before anything reaches stdout), then the cd resolution, the
// C2-style dep-id guard, the manifest existence check, the atomic [[deps]]
// removal (config.RemoveDepSegment — the ONE manifest writer of this unit,
// already owning the segmenter, the valid→invalid guard, the atomic write and
// the frozen success/not-found lines), the .deps prune, and finally the lock
// step (internal/lock — the port of the lock.py heredoc D17).
//
// Documented deviations from the script (pinned by tests, reported in the
// unit document's Log):
//   - a failing cd keeps exit 1 and the `cd: <path>: <reason>` shape but
//     drops bash's `lib/skills-remove.sh: line N: ` prefix (the U5a/doctor
//     S12 class) — TestSkillsRemoveMissingPathDeviation;
//   - a READ-ONLY manifest: the script's mkstemp+os.replace bypasses the file
//     mode (rename needs write on the directory) and completes the whole
//     tail, while RemoveDepSegment's shared atomicWrite refuses it
//     (R4-atomicwrite-readonly-refusal, pinned in internal/config) —
//     TestSkillsRemoveReadOnlyDeviation records both behaviours. The refusal
//     lives outside this unit's edit surface;
//   - a corrupt lock file exits 1 like the script's traceback but prints the
//     Go load error instead of Python's traceback text (diagnostic class,
//     ADR 0002);
//   - an rm/lock I/O failure prints Go's error text where rm/Python word
//     their own; the frozen exit code 1 is preserved.
func runSkillsRemove(args []string, stdout, stderr io.Writer) int {
	// The script's while/case loop, in order: --help|-h wins wherever it
	// appears, `--` makes every remaining argument positional (never a flag),
	// an unknown flag prints two stderr lines, and a third positional is the
	// other exit-2 usage error. The take_positional emptiness checks are
	// mirrored literally: an empty-string positional leaves the slot empty, so
	// `remove "" proj` makes proj the ID, exactly like the shell.
	depID, target := "", ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, skillsRemoveUsage)
			return 0
		case arg == "--":
			// Positionals after -- are real arguments, not discardable.
			for i++; i < len(args); i++ {
				rest := args[i]
				if depID == "" {
					depID = rest
					continue
				}
				if target == "" {
					target = rest
					continue
				}
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", rest)
				return 2
			}
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs skills remove --help' for usage.")
			return 2
		default:
			if depID == "" {
				depID = arg
				continue
			}
			if target == "" {
				target = arg
				continue
			}
			fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
			return 2
		}
	}
	if depID == "" {
		fmt.Fprintln(stderr, "ERROR: missing skill id")
		fmt.Fprint(stderr, skillsRemoveUsage)
		return 2
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

	// C2-style guard, exactly the script's order: ids that could escape the
	// managed ai-specs/.deps tree are refused BEFORE the manifest is touched
	// (a hand-edited manifest can carry a traversal id; removal must never
	// turn it into an rm -rf outside .deps/). The message runs the id through
	// the script's `LC_ALL=C tr -c ' -~' '?'` sanitization: every byte outside
	// 0x20–0x7E becomes '?'.
	if sanitized, invalid := invalidDepID(depID); invalid {
		fmt.Fprintf(stderr, "ERROR: refusing invalid dep id: '%s'\n", sanitized)
		return 2
	}

	if !regularInitFile(tomlPath) {
		fmt.Fprintf(stderr, "ERROR: %s not found.\n", tomlPath)
		return 1
	}

	// The manifest step: the shared writer owns the segmenter, the
	// valid→invalid guard, the atomic temp+rename and the frozen messages —
	// not a byte of it is re-implemented here.
	msg, err := config.RemoveDepSegment(tomlPath, depID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, msg)

	// Prune the vendored copy (D18): the in-project .deps tree is
	// regenerable from the declared source, so removal deletes it instead of
	// orphaning it. `-d` follows symlinks; RemoveAll unlinks a symlink itself
	// without recursing, like `rm -rf`.
	depsDir := filepath.Join(target, "ai-specs", ".deps", depID)
	if isDirectory(depsDir) {
		if err := os.RemoveAll(depsDir); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "  ✓ pruned ai-specs/.deps/%s/\n", depID)
	}

	// Drop the dep's recorded content hashes from the lock (D17 schema). The
	// script's heredoc exits silently when the lock is not a file, and
	// writes+prints only when the entry was actually present — LoadLock's
	// missing-file shape makes the same branches.
	lockPath := filepath.Join(target, "ai-specs", ".ai-specs.lock")
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if lock.RemoveDepLockEntries(lk, depID) {
		if err := lock.WriteLock(lockPath, lk); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "  ✓ removed lock hashes for '%s'\n", depID)
	}
	return 0
}

// invalidDepID is the script's invalid_dep_id guard: empty, ".", "..", or
// containing '/', '\\', or any byte outside 0x20–0x7E (the case pattern plus
// `LC_ALL=C grep -q '[^ -~]'`). The returned sanitized id mirrors
// `LC_ALL=C tr -c ' -~' '?'` byte for byte — non-ASCII UTF-8 becomes one '?'
// per byte, like the shell sees it under the C locale.
func invalidDepID(id string) (sanitized string, invalid bool) {
	invalid = id == "" || id == "." || id == ".." ||
		strings.Contains(id, "/") || strings.Contains(id, "\\")
	b := []byte(id)
	if !invalid {
		for _, c := range b {
			if c < 0x20 || c > 0x7e {
				invalid = true
				break
			}
		}
	}
	if !invalid {
		return id, false
	}
	sb := make([]byte, len(b))
	for i, c := range b {
		if c < 0x20 || c > 0x7e {
			sb[i] = '?'
		} else {
			sb[i] = c
		}
	}
	return string(sb), true
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

// ── GO-20 U5c (T5): `skills add` / `add-dep` and the skills dispatcher ──

// skillsUsage is lib/skills.sh's usage() heredoc byte for byte: a QUOTED
// heredoc (`cat <<'EOF'`), so the text is literal and the trailing newline
// after the last line is part of the output (stdout for `--help|-h|help`,
// stderr after the bare `skills`). Pinned byte for byte by the U5c
// differential's dispatcher rows.
const skillsUsage = `Usage: ai-specs skills <subcommand> [args]
Subcommands:
  add <url> [path] [flags]   Register a vendored skill ([[deps]]) and sync
                             Flags: --id, --subdir, --scope, --license,
                                    --attribution, --trigger, --no-sync
  list [path]                List registered and installed skills
  remove <id> [path]         Remove a vendored skill and prune ai-specs/.deps/<id>/
  validate <skill_md_path>   Validate one SKILL.md's sync metadata (JSON)
Path defaults to current directory. Run 'ai-specs skills add --help' for
full add flags.
`

// skillsAddUsage is lib/skills-add.sh's usage() heredoc — an UNQUOTED heredoc
// whose only runtime substitution is ${CMD_LABEL} (line 13). The label is
// "ai-specs skills add" on the skills route and "ai-specs add-dep" behind
// bin/ai-specs's alias (the AI_SPECS_INVOKED_AS value the legacy dispatcher
// exports), which is what makes `add-dep --help` name ITSELF.
func skillsAddUsage(label string) string {
	return `Usage: ` + label + ` <git-url> [path] [flags]

Register a vendored skill in ai-specs.toml (under [[deps]]) and run sync.

Arguments:
  git-url               Source URL (e.g. https://github.com/obra/superpowers)
  path                  Project root (default: current directory)

Flags:
  --id <id>             Skill ID (default: last URL path component, .git stripped)
  --subdir <subpath>    Subdir within the repo where SKILL.md lives (multi-skill repos)
  --scope <s1,s2,...>   Comma-list for metadata.scope (default: root)
  --license <license>   License string (default: empty)
  --attribution <auth>  vendor_attribution (default: URL author)
  --ref <ref>           Git ref to pin (tag/branch/sha; default: none)
  --trigger <text>      auto_invoke entry (default: "When working on <id>")
  --no-sync             Don't run 'ai-specs sync' after registering
`
}

// runSkillsDispatch reproduces lib/skills.sh's case statement for every arm
// Route did not intercept: `add|list|remove` are subcommand routes (the
// script's first three arms, intercepted in Route), so what reaches here is
// `--help|-h|help` (usage → stdout, exit 0), the bare `skills` (usage →
// stderr, exit 2) and any other subcommand (the two unknown-subcommand lines
// on stderr, exit 2) — with the rest of the argv ignored, exactly like
// `subcmd="${1:-}"; shift || true` before the case.
func runSkillsDispatch(args []string, stdout, stderr io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}
	switch subcmd {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, skillsUsage)
		return 0
	case "":
		fmt.Fprint(stderr, skillsUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs skills: unknown subcommand '%s'\n", subcmd)
		fmt.Fprintln(stderr, "Run 'ai-specs skills --help' for usage.")
		return 2
	}
}

// skillsAddValueFlags are lib/skills-add.sh's seven value-taking flags, in
// the script's case order (each supports `--flag value` and `--flag=value`).
var skillsAddValueFlags = [...]string{
	"--id", "--subdir", "--scope", "--license", "--attribution", "--ref", "--trigger",
}

// kebabCaseID is the script's `[[ "$ID" =~ ^[a-z0-9][a-z0-9_-]*$ ]]`.
// Non-ASCII and control bytes never reach it — invalidDepID has already
// refused them — so the ASCII-only class matches bash's C-locale ERE here.
var kebabCaseID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// skillsAddOptions is the legacy argument contract of skills-add.sh.
type skillsAddOptions struct {
	url, target string
	id, subdir  string
	scope       string // default "root"; `--scope=` explicitly empties it and
	// nothing re-fills it (the echo prints empty, the writer falls back to root)
	license string
	auth    string // --attribution
	ref     string
	trigger string
	runSync bool // default true; --no-sync clears it
}

// setAddValue routes a parsed `--flag value` / `--flag=value` to its field.
func (o *skillsAddOptions) setAddValue(flag, value string) {
	switch flag {
	case "--id":
		o.id = value
	case "--subdir":
		o.subdir = value
	case "--scope":
		o.scope = value
	case "--license":
		o.license = value
	case "--attribution":
		o.auth = value
	case "--ref":
		o.ref = value
	case "--trigger":
		o.trigger = value
	}
}

// skillsAddFlagArg classifies arg against the seven value-taking flags. It
// reports which flag matched, the carried value (eq=true for `--flag=…`,
// which may be EMPTY and never consumes the next argument) and whether the
// separate form needs args[i+1].
func skillsAddFlagArg(arg string) (flag, value string, eq, ok bool) {
	for _, f := range skillsAddValueFlags {
		if arg == f {
			return f, "", false, true
		}
		if strings.HasPrefix(arg, f+"=") {
			return f, strings.TrimPrefix(arg, f+"="), true, true
		}
	}
	return "", "", false, false
}

// parseSkillsAddArgs mirrors the script's while/case loop plus its two
// post-loop take_positional lines. done reports that the loop terminated the
// verb (help, usage error); the caller returns rc immediately.
//
// Contract notes pinned by the differential:
//   - `-h|--help` wins wherever it appears (bash case order, no overlap with
//     the other arms);
//   - a trailing value-taking flag has NO value: bash expands `$2` under
//     `set -u` and dies at exit 1 with its own line-number artifact. The port
//     keeps the observed exit 1 and prints the script's OWN dead guard text
//     (`ERROR: --<flag> requires a value`, the message behind the
//     unreachable `shift 2 || …` fallback) — S12/init's JD-B-001 precedent;
//   - an unknown flag prints two stderr lines; a THIRD positional errors,
//     but one after `--` fills only an empty slot and is otherwise silently
//     DROPPED (the script's post-loop consumption never re-checks `$@`);
//   - an empty-string positional keeps the slot empty (`[[ -z ]]`), so
//     `add "" proj` makes proj the URL, exactly like the shell.
func parseSkillsAddArgs(args []string, label string, stdout, stderr io.Writer) (skillsAddOptions, int, bool) {
	opts := skillsAddOptions{scope: "root", runSync: true}
	rest := []string(nil)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			fmt.Fprint(stdout, skillsAddUsage(label))
			return opts, 0, true
		case arg == "--no-sync":
			opts.runSync = false
		case arg == "--":
			// `shift; break`: the remainder is positional-only, consumed by
			// the take_positional lines below after the loop ends.
			rest = args[i+1:]
			i = len(args)
		default:
			if flag, value, eq, matched := skillsAddFlagArg(arg); matched {
				if !eq {
					if i+1 >= len(args) {
						fmt.Fprintf(stderr, "ERROR: %s requires a value\n", flag)
						return opts, 1, true
					}
					value = args[i+1]
					i++
				}
				opts.setAddValue(flag, value)
				continue
			}
			// bash's `-*` arm matches ANY flag-shaped token, `-` alone included.
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
				fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", label)
				return opts, 2, true
			}
			if opts.url == "" {
				opts.url = arg
				continue
			}
			if opts.target == "" {
				opts.target = arg
				continue
			}
			fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
			return opts, 2, true
		}
	}
	// `[[ -z "$URL" && $# -gt 0 ]] && URL="$1" && shift` (and the
	// TARGET_PATH twin): only a `--` break leaves a remainder, the empty
	// slots win first, and anything left over is never looked at again.
	if opts.url == "" && len(rest) > 0 {
		opts.url, rest = rest[0], rest[1:]
	}
	if opts.target == "" && len(rest) > 0 {
		opts.target, rest = rest[0], rest[1:]
	}
	return opts, 0, false
}

// runSkillsAdd is the native `skills add` / `add-dep` entry: the port of
// lib/skills-add.sh. label is the CMD_LABEL the legacy script reads from
// AI_SPECS_INVOKED_AS — Route supplies "ai-specs skills add" (skills.sh's
// `add)` arm) or "ai-specs add-dep" (bin/ai-specs's alias arm), so every
// message names the command the user actually typed.
//
// The stages, in the script's order: the argument loop (usage errors exit 2
// before anything else), the missing-URL error (two stderr lines + usage →
// STDOUT, exit 2), the cd and manifest-existence checks, the URL
// derivations (id, trigger, attribution), the C2-style dep-id guard and the
// kebab-case check, the duplicate-id check, the eight-line summary block,
// the atomic [[deps]] append (config.AppendDepsBlock — the ONE manifest
// writer of this unit, shared with the remove port), and finally the sync
// delegation (S8).
//
// Documented deviations from the script (each pinned by a test):
//   - a failing cd keeps exit 1 and `cd: <path>: <reason>` but drops bash's
//     `lib/skills-add.sh: line 94: ` prefix (the U5a/U5b S12 class) —
//     TestSkillsAddMissingPathDeviation;
//   - a trailing value-taking flag keeps the observed exit 1 and prints the
//     script's dead-guard text instead of `$2: unbound variable` —
//     TestSkillsAddTrailingFlagDeviation;
//   - a manifest whose `deps` is a TABLE (or a truthy non-list) crashes the
//     script's python reader at exit 1 with a traceback; the port keeps exit
//     1, empty stdout and the untouched manifest, and words the script's own
//     frozen `ERROR: failed to read manifest` line (ADR 0002) —
//     TestSkillsAddNonListDepsDeviation. A manifest that does not PARSE dies
//     silently at exit 2 in BOTH legs (the script's substitution failure
//     under set -e), reproduced byte for byte in the differential;
//   - a READ-ONLY manifest: the script appends with pathlib.write_text(),
//     which needs the FILE's write bit and dies with a PermissionError
//     traceback after the summary (exit 1), while the port reaches the
//     shared atomicWrite's R4 refusal (rename needs the DIRECTORY's write
//     bit) — both legs exit 1 with the identical summary on stdout and
//     nothing written; only the failure wording differs —
//     TestSkillsAddReadOnlyDeviation (note the inversion versus U5b's
//     remove, whose mkstemp+replace heredoc bypassed the file mode);
//   - an append I/O failure words Go's error where python words its
//     traceback; the frozen exit code 1 is preserved.
func runSkillsAdd(args []string, label, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, rc, done := parseSkillsAddArgs(args, label, stdout, stderr)
	if done {
		return rc
	}
	if opts.url == "" {
		fmt.Fprintln(stderr, "ERROR: <git-url> is required.")
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", label)
		fmt.Fprint(stdout, skillsAddUsage(label))
		return 2
	}
	target := opts.target
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
	if !regularInitFile(tomlPath) {
		fmt.Fprintf(stderr, "ERROR: %s not found. Run 'ai-specs init %s' first.\n", tomlPath, target)
		return 1
	}

	// Derive defaults from the URL, exactly the script's expansion order
	// (basename → strip a trailing .git → id, then trigger, then
	// attribution): ${URL##*/}, ${basename%.git}, ${v#*://}, ${v#*/}, ${v%%/*}.
	urlBasename := afterLast(opts.url, "/")
	urlBasename = strings.TrimSuffix(urlBasename, ".git")
	id := opts.id
	if id == "" {
		id = urlBasename
	}
	trigger := opts.trigger
	if trigger == "" {
		trigger = "When working on " + id
	}
	auth := opts.auth
	if auth == "" {
		noProto := removeThroughFirst(opts.url, "://")
		noHost := removeThroughFirst(noProto, "/")
		auth = uptoFirst(noHost, "/")
	}

	// Refuse ids that could escape the managed ai-specs/.deps tree, then the
	// kebab-case validation — both BEFORE the manifest is read or written,
	// exactly like the script (guards → regex → dup check → summary).
	if sanitized, invalid := invalidDepID(id); invalid {
		fmt.Fprintf(stderr, "ERROR: refusing invalid dep id: '%s'\n", sanitized)
		return 2
	}
	if !kebabCaseID.MatchString(id) {
		fmt.Fprintf(stderr, "ERROR: derived/provided id is not kebab-case: '%s' — pass --id explicitly.\n", id)
		return 2
	}

	// Confirm not already registered. See the deviation list above for the
	// two read-failure families (silent exit 2 / frozen message at exit 1).
	data, err := config.LoadManifest(tomlPath)
	if err != nil {
		return 2 // the script's `existing="$(python3 …)"` dies under set -e with the child's status 2, silently
	}
	ids, ok := skillsAddDupIDs(data)
	if !ok {
		fmt.Fprintln(stderr, "ERROR: failed to read manifest — fix ai-specs.toml first.")
		return 1
	}
	if slices.Contains(ids, id) {
		fmt.Fprintf(stderr, "ERROR: dep with id '%s' already exists in %s\n", id, tomlPath)
		return 1
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, label)
	fmt.Fprintf(stdout, "  url:         %s\n", opts.url)
	fmt.Fprintf(stdout, "  id:          %s\n", id)
	fmt.Fprintf(stdout, "  subdir:      %s\n", addOrNone(opts.subdir))
	fmt.Fprintf(stdout, "  scope:       %s\n", opts.scope)
	fmt.Fprintf(stdout, "  license:     %s\n", addOrNone(opts.license))
	fmt.Fprintf(stdout, "  attribution: %s\n", auth)
	fmt.Fprintf(stdout, "  ref:         %s\n", addOrNone(opts.ref))
	fmt.Fprintln(stdout, "")

	msg, err := config.AppendDepsBlock(tomlPath, id, opts.url, opts.subdir, opts.scope, trigger, opts.license, auth, opts.ref)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, msg)

	fmt.Fprintln(stdout, "")
	if !opts.runSync {
		fmt.Fprintf(stdout, "✓ dep registered. Run 'ai-specs sync %s' to vendor it.\n", target)
		return 0
	}
	fmt.Fprintf(stdout, "▸ ai-specs sync %s\n", target)
	// S8: the one step of this unit that stays a call into another verb. The
	// script runs sync as its LAST command under set -e, so sync's exit code
	// IS the verb's exit code — here it is the return value itself.
	return sync.Run([]string{"sync", target}, home, stdin, stdout, stderr)
}

// skillsAddDupIDs is the id list the script's dup-check heredoc computes:
// `ids = [d.get("id") for d in (data.get("deps", []) or [])]`. Python
// truthiness gates the raw value first (absent, [], {}, "", 0, false → no
// ids, registration proceeds), then the value must be a list whose items are
// all tables — a truthy non-list or a non-table item crashes the heredoc at
// exit 1 before ANY write (ok=false). Python's `in` compares against the
// STRING argv only, so non-string id values can never match; they are
// collected as nothing.
func skillsAddDupIDs(data *toml.Table) (ids []string, ok bool) {
	raw, present := data.Get("deps")
	if !present || !config.Truthy(raw) {
		return nil, true
	}
	var items []any
	switch x := raw.(type) {
	case []any:
		items = x
	case []*toml.Table:
		for _, tbl := range x {
			items = append(items, tbl)
		}
	default:
		return nil, false // truthy non-list: `for d in …` dies (keys/scalar)
	}
	for _, item := range items {
		tbl, isTable := item.(*toml.Table)
		if !isTable {
			return nil, false // `d.get` on a non-dict crashes the heredoc
		}
		if v, has := tbl.Get("id"); has {
			if s, isString := v.(string); isString {
				ids = append(ids, s)
			}
		}
	}
	return ids, true
}

// afterLast is `${v##<sep>*}`… concretely `${v##*/}`: everything after the
// LAST occurrence of sep (unchanged when sep is absent). Used for the URL
// basename the id is derived from.
func afterLast(v, sep string) string {
	if idx := strings.LastIndex(v, sep); idx >= 0 {
		return v[idx+len(sep):]
	}
	return v
}

// removeThroughFirst is `${v#*<sep>}`: drop the shortest prefix ending in the
// FIRST occurrence of sep (unchanged when sep is absent) — bash's shortest-
// match `#` removal, used for the scheme and the host of the URL.
func removeThroughFirst(v, sep string) string {
	if idx := strings.Index(v, sep); idx >= 0 {
		return v[idx+len(sep):]
	}
	return v
}

// uptoFirst is `${v%%<sep>*}`: v truncated before its FIRST occurrence of
// sep (unchanged when sep is absent) — bash's longest-match `%%` removal,
// used for the URL author part of the attribution default.
func uptoFirst(v, sep string) string {
	if idx := strings.Index(v, sep); idx >= 0 {
		return v[:idx]
	}
	return v
}

// addOrNone is the summary block's `${VAR:-(none)}`: unset and empty are the
// same value in the shell's default expansion.
func addOrNone(v string) string {
	if v == "" {
		return "(none)"
	}
	return v
}
