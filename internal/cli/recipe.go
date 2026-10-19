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
// READ-ONLY (U6a goal 4, the `recipe list` section below): nothing there
// creates a cache, a manifest or an `ai-specs/` tree — that verb only stats
// and reads. The `recipe add` section (GO-20 U6b) and the `recipe remove`
// section (GO-20 U6c) are the file's writers, and every byte of both writes
// is delegated to internal/config — no second manifest writer lives here.
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

	"ai-specs.dev/ai-specs/internal/config"
	homepkg "ai-specs.dev/ai-specs/internal/home"
	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/recipe"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
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

// ── GO-20 U6b (T2): `recipe add` ──
//
// Native `ai-specs recipe add` — the port of lib/recipe-add.sh (59 lines,
// the wrapper: the argument loop, the missing-id arm, the cd/pwd target
// resolution) plus lib/_internal/recipe-add.py's add_recipe (302, the verb:
// the manifest guard, the internal-test refusal, the catalog read, the
// duplicate refusal, the placeholder append with its restore-on-invalid
// guard, the materialize listing and the non-TTY next-steps guidance).
//
// TTY DECISION (option (a), recorded in L4): on a terminal the WHOLE verb
// stays on the legacy script through runRecipeAdd's documented seam — the
// same delegation pattern as the init wizard's AI_SPECS_INIT_TUI_PY hatch —
// because add_recipe's two interactive branches (recipe-add.py:155-174's
// ensure_deps/questionary PRE-write gate and :262-275's "Configure now?"
// wizard + env_scaffold.offer_harness_env) are not half-ported: the wizard
// core lands in U6d and sync.OfferHarnessEnv exists, but the branch stays
// whole-or-legacy, never split across packages. Off the TTY (piped, CI, and
// both legs of the differential) the port owns the verb end to end — which
// is also what retires recipe-add.sh/recipe-add.py for this route (S9).
//
// Documented deviations (the S12/ADR 0002 class, pinned by tests): a `cd`
// failure keeps exit 1 and the `cd: <path>: <reason>` shape but drops bash's
// own `lib/recipe-add.sh: line N: ` prefix (TestRecipeAddMissingPathDeviation),
// and the guard message's embedded parse-error detail is diagnostic — the
// differential normalizes that parenthesized detail only
// (recipeAddGuardDetailRE), byte-comparing the id, the framing and the
// untouched restored manifest around it.

// recipeAddUsage is lib/recipe-add.sh's usage() heredoc byte for byte: a
// QUOTED heredoc (`cat <<'EOF'`), so the text is literal and the trailing
// newline after the last line is part of the output (stdout for --help,
// stderr after the missing-id error).
const recipeAddUsage = `Usage: ai-specs recipe add <id> [path] [--help]
Add a recipe from catalog/recipes/ to the local manifest.
Arguments:
  id      Recipe identifier (directory name under catalog/recipes/)
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// runRecipeAdd is the native `recipe add` entry: the TTY seam first (the
// whole verb delegating back to lib/recipe.sh — see the header), then
// recipe-add.sh's argument loop (usage errors exit 2 before anything touches
// the filesystem), the missing-id arm, the cd/pwd target resolution, and
// finally add_recipe itself.
//
// exePath is the running dispatcher's own path (selfPath());
// recipe.CatalogHome anchors the catalogue resolver's second candidate on it.
func runRecipeAdd(args []string, home string, stdin io.Reader, stdout, stderr io.Writer, exePath string, ttyNow bool) int {
	if ttyNow {
		// Documented seam (option (a)): recipe.sh receives the post-shift
		// argv exactly as bin/ai-specs's shim would deliver it — args[0]
		// ("recipe") is stripped by runShim, so the legacy case statement
		// sees `add …` and execs recipe-add.sh unchanged.
		return runShim(route{kind: routeShim, script: "recipe.sh"}, args, home, stdin, stdout, stderr, exePath)
	}
	// args[0]=recipe, args[1]=add: recipe.sh shifts the subcommand before it
	// execs recipe-add.sh, so the verb's own loop sees args[2:].
	args = args[2:]

	// The script's while/case loop, in order: --help|-h wins wherever it
	// appears, `--` makes every remaining argument positional (never a flag),
	// an unknown flag prints two stderr lines, and a second/third positional
	// is the other exit-2 usage error. An empty-string positional keeps the
	// slot empty (`[[ -z ]]`), exactly like the script's take_positional.
	recipeID, target := "", ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, recipeAddUsage)
			return 0
		case arg == "--":
			// Positionals after -- are real arguments, not discardable.
			for i++; i < len(args); i++ {
				rest := args[i]
				if recipeID == "" {
					recipeID = rest
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
			fmt.Fprintln(stderr, "Run 'ai-specs recipe add --help' for usage.")
			return 2
		default:
			if recipeID == "" {
				recipeID = arg
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
	if recipeID == "" {
		fmt.Fprintln(stderr, "ERROR: missing recipe id")
		fmt.Fprint(stderr, recipeAddUsage)
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
	return runRecipeAddVerb(resolved, recipeID, home, exePath, stdout, stderr)
}

// runRecipeAddVerb is add_recipe (recipe-add.py:115-296) on the non-TTY
// path: the manifest guard first, the internal-test refusal, the catalog
// read, the duplicate refusal, the placeholder append through
// config.AppendRecipeSegments (the ONE manifest writer, S6), the materialize
// listing, and the next-steps guidance. The TTY branches never reach here:
// runRecipeAdd's seam delegates the whole verb to the legacy script first,
// so the interactive half is never half-ported.
func runRecipeAddVerb(target, recipeID, home, exePath string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(target, "ai-specs", "ai-specs.toml")

	// add_recipe's guard order, verbatim: `if not manifest_path.is_file()`
	// runs BEFORE the internal-test refusal and the catalog read.
	if !regularInitFile(tomlPath) {
		fmt.Fprintln(stderr, "Project not initialized. Run: ai-specs init")
		return 1
	}
	// util.is_internal_test_recipe + util.internal_test_recipe_message.
	if strings.HasPrefix(recipeID, "test-") {
		fmt.Fprintf(stderr, "Recipe '%s' is an internal test fixture and is not part of the public catalog.\n", recipeID)
		return 1
	}

	// Catalog existence: read_recipe's `catalog_dir / recipe_id` is a POSIX
	// path join WITHOUT cleaning (Path never normalizes on join) and an
	// ABSOLUTE id replaces the catalogue root — so the lookup path, every
	// message and the control flow carry the id exactly as Python joins it.
	installRoot := home
	if exePath != "" {
		installRoot = homepkg.ResolveHome(exePath, "")
	}
	catalog := recipe.CatalogDir(recipe.CatalogHome(home, installRoot))
	recipeDir := catalog + string(filepath.Separator) + recipeID
	if filepath.IsAbs(recipeID) {
		recipeDir = recipeID
	}
	notFound := func(detail string) int {
		fmt.Fprintf(stderr, "Recipe '%s' not found in catalog/recipes/: %s\n", recipeID, detail)
		return 1
	}
	if info, err := os.Stat(recipeDir); err != nil || !info.IsDir() {
		return notFound("recipe directory not found: " + recipeDir)
	}
	loaded, err := schema.LoadRecipeToml(recipeDir + string(filepath.Separator) + "recipe.toml")
	if err != nil {
		// read_recipe lets EVERY read/validation exception through the one
		// wrapper (`except Exception as exc: print(f"Recipe … not found …: {exc}")`);
		// the parse-error/validation detail inside is ours (ADR 0002).
		return notFound(err.Error())
	}

	// The duplicate refusal: toml_read.load_toml + read_recipes membership —
	// only a [recipes.<id>] TABLE counts (read_recipes skips non-dict values),
	// and a manifest that does not PARSE is swallowed by recipe-add.py's
	// `except Exception: pass`, so the append proceeds and the write step's
	// guard is what refuses it.
	if data, lerr := config.LoadManifest(tomlPath); lerr == nil {
		if recipes, ok := data.Table("recipes"); ok && recipes != nil {
			for _, id := range recipes.Keys() {
				if id != recipeID {
					continue
				}
				v, present := recipes.Get(id)
				if !present {
					continue
				}
				if _, isTable := v.(*toml.Table); isTable {
					fmt.Fprintf(stderr, "Recipe '%s' is already in the manifest. Use ai-specs sync to materialize it.\n", recipeID)
					return 1
				}
			}
		}
	}

	// has_config / has_mcp_env (recipe-add.py:155-160): standard config
	// FIELDS only (structured tables and extras do not count), and any truthy
	// mcp env entry.
	hasConfig := loaded.ConfigSchema != nil && len(loaded.ConfigSchema.Fields) > 0
	hasMcpEnv := false
	for _, m := range loaded.MCP {
		if config.Truthy(m.Config["env"]) {
			hasMcpEnv = true
			break
		}
	}

	// The placeholder fields; the appender owns Python's sorted() order.
	var fields []config.RecipeConfigField
	if loaded.ConfigSchema != nil {
		fields = make([]config.RecipeConfigField, 0, len(loaded.ConfigSchema.Fields))
		for key, f := range loaded.ConfigSchema.Fields {
			fields = append(fields, config.RecipeConfigField{
				Key: key, Required: f.Required, Default: f.Default,
			})
		}
	}

	// The write: one appender in internal/config owns the read, the literal
	// append, the atomic write and the parse-validate-and-restore guard (S5/S6)
	// — nothing of the surgery is re-implemented here. Its error carries
	// recipe-add.py's frozen guard message; the verb just prints it (exit 1).
	if werr := config.AppendRecipeSegments(tomlPath, recipeID, fields); werr != nil {
		fmt.Fprintln(stderr, werr)
		return 1
	}

	// The frozen success block + the "next sync will materialize" listing
	// (recipe-add.py:222-255): skills/commands/mcp as comma lists,
	// templates/docs one line each, else the primitives placeholder — and
	// hooks never count toward it (`if not any([skills, …, docs])`).
	fmt.Fprintf(stdout, "Recipe '%s' added to the manifest.\n", recipeID)
	fmt.Fprintln(stdout, "The next sync will materialize:")
	if len(loaded.Skills) > 0 {
		ids := make([]string, len(loaded.Skills))
		for i, s := range loaded.Skills {
			ids[i] = s.ID
		}
		fmt.Fprintf(stdout, "  - skills: %s\n", strings.Join(ids, ", "))
	}
	if len(loaded.Commands) > 0 {
		ids := make([]string, len(loaded.Commands))
		for i, c := range loaded.Commands {
			ids[i] = c.ID
		}
		fmt.Fprintf(stdout, "  - commands: %s\n", strings.Join(ids, ", "))
	}
	if len(loaded.MCP) > 0 {
		ids := make([]string, len(loaded.MCP))
		for i, m := range loaded.MCP {
			ids[i] = m.ID
		}
		fmt.Fprintf(stdout, "  - mcp: %s\n", strings.Join(ids, ", "))
	}
	for _, tpl := range loaded.Templates {
		fmt.Fprintf(stdout, "  - template: %s → %s\n", tpl.Source, tpl.Target)
	}
	for _, d := range loaded.Docs {
		fmt.Fprintf(stdout, "  - doc: %s → %s\n", d.Source, d.Target)
	}
	if len(loaded.Skills) == 0 && len(loaded.Commands) == 0 && len(loaded.MCP) == 0 &&
		len(loaded.Templates) == 0 && len(loaded.Docs) == 0 {
		fmt.Fprintln(stdout, "  (no primitives declared)")
	}

	// Non-TTY guidance (recipe-add.py's else branch — the ONLY branch this
	// port takes; the TTY branch stayed on the legacy script):
	// "Next steps:" with one line per present condition, nothing otherwise.
	if hasConfig || hasMcpEnv {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Next steps:")
		if hasConfig {
			fmt.Fprintln(stdout, "  - Configure required values: ai-specs configure-recipes")
		}
		if hasMcpEnv {
			fmt.Fprintln(stdout, "  - Configure MCP environment variables: ai-specs configure-recipes")
		}
	}
	return 0
}

// ── GO-20 U6c (T3): `recipe remove` ──
//
// Native `ai-specs recipe remove` — the port of lib/recipe-remove.sh (203
// lines): the wrapper's argument loop (the usage() heredoc, `<id>` then
// optional `[path]`, `--` semantics, unknown flags, the missing-id arm) and
// the two steps that follow it in order — the manifest surgery through
// config.RemoveRecipeSegments (the ONE remover, S6: the segmenter, the
// valid→invalid guard, the atomic write and the frozen messages are all
// already differential-tested against recipe_remove_ref.py in
// internal/config) and the lock cleanup (recipe-remove.sh:167-203) through
// internal/lock — LoadLock → RemoveRecipeLockEntries → WriteLock only when
// something was removed → the frozen `✓ cleaned` line.
//
// No TTY seam: unlike `recipe add` (L4's option (a)) this script has no
// interactive branch — its heredoc is pure surgery — so the whole verb is
// native end to end, which is what retires the BINARY's use of
// recipe-remove.sh (S9: the file itself stays until U7/U8).
//
// Documented deviations (the S12/ADR 0002 class, pinned by tests): a `cd`
// failure keeps exit 1 and the `cd: <path>: <reason>` shape but drops bash's
// own `lib/recipe-remove.sh: line N: ` prefix (TestRecipeRemoveMissingPathDeviation),
// a corrupt lock file exits 1 like the script's traceback but prints Go's
// load error instead of Python's traceback text, and the valid→invalid guard
// message's embedded parse-error detail is diagnostic — the differential
// normalizes that parenthesized detail only (recipeRemoveGuardDetailRE).

// recipeRemoveUsage is lib/recipe-remove.sh's usage() heredoc byte for byte:
// a QUOTED heredoc (`cat <<'EOF'`), so the text is literal and the trailing
// newline after the last line is part of the output (stdout for --help,
// stderr after the missing-id error).
const recipeRemoveUsage = `Usage: ai-specs recipe remove <id> [path] [--help]
Remove a recipe ([recipes.<id>]) from ai-specs.toml.
Arguments:
  id      Recipe identifier to remove
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// runRecipeRemove is the native `recipe remove` entry: recipe-remove.sh's
// argument loop first (usage errors exit 2 before anything touches the
// filesystem), the missing-id arm, the cd/pwd target resolution, the
// `ERROR: <toml> not found.` guard, then the two write steps in the script's
// own order — manifest surgery, then lock cleanup.
func runRecipeRemove(args []string, stdout, stderr io.Writer) int {
	// The script's while/case loop, in order: --help|-h wins wherever it
	// appears, `--` makes every remaining argument positional (never a flag),
	// an unknown flag prints two stderr lines, and a third positional is the
	// other exit-2 usage error. The take_positional emptiness checks are
	// mirrored literally: an empty-string positional leaves the slot empty, so
	// `remove "" proj` makes proj the ID, exactly like the shell.
	recipeID, target := "", ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, recipeRemoveUsage)
			return 0
		case arg == "--":
			// Positionals after -- are real arguments, not discardable.
			for i++; i < len(args); i++ {
				rest := args[i]
				if recipeID == "" {
					recipeID = rest
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
			fmt.Fprintln(stderr, "Run 'ai-specs recipe remove --help' for usage.")
			return 2
		default:
			if recipeID == "" {
				recipeID = arg
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
	if recipeID == "" {
		fmt.Fprintln(stderr, "ERROR: missing recipe id")
		fmt.Fprint(stderr, recipeRemoveUsage)
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
	tomlPath := filepath.Join(resolved, "ai-specs", "ai-specs.toml")

	// The script's `[[ ! -f "$TOML_PATH" ]]` guard, same message and exit.
	if !regularInitFile(tomlPath) {
		fmt.Fprintf(stderr, "ERROR: %s not found.\n", tomlPath)
		return 1
	}

	// Step 1 — the manifest surgery: the shared remover owns the segmenter,
	// the valid→invalid guard, the atomic temp+rename and the frozen messages
	// (`✓ removed N section(s) …` on stdout, `✗ recipe '<id>' not found in …`
	// and the guard error on stderr) — not a byte of it is re-implemented
	// here. The script's `set -e` stops the tail on any of those failures, so
	// this returns before the lock step, exactly like the heredoc's exit 1.
	_, msg, err := config.RemoveRecipeSegments(tomlPath, recipeID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, msg)

	// Step 2 — the lock cleanup (recipe-remove.sh:167-203): the script's
	// `[[ -f "$LOCK_PATH" ]]` is LoadLock's missing-file shape (a missing or
	// non-regular path loads an empty lock, so nothing is removed and the lock
	// is never rewritten), and internal/lock owns the removal decision, the
	// byte-exact writer and the atomicity. The frozen `✓ cleaned` line prints
	// only when entries were actually dropped.
	lockPath := filepath.Join(resolved, "ai-specs", ".ai-specs.lock")
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		// The script's python dies with a traceback here (exit 1); the exit
		// code survives, the wording is ours (ADR 0002).
		fmt.Fprintln(stderr, err)
		return 1
	}
	if lock.RemoveRecipeLockEntries(lk, recipeID) {
		if err := lock.WriteLock(lockPath, lk); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "  ✓ cleaned lock entries for recipe '%s'\n", recipeID)
	}
	return 0
}
