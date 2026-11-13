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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"ai-specs.dev/ai-specs/internal/cliversion"
	"ai-specs.dev/ai-specs/internal/config"
	homepkg "ai-specs.dev/ai-specs/internal/home"
	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/recipe"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/sync"
	"ai-specs.dev/ai-specs/internal/target"
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
// TTY DECISION — FLIPPED (U6e, T3b second half): there is no delegation
// left. Off the TTY (piped, CI, and both legs of the differential) the port
// owns the verb end to end, unchanged. On a terminal the whole tail runs
// through the injected recipeTailSeams: recipe-add.py:247-275's "Configure
// now?" confirm (U6d-2's runConfigWizard on internal/tui), the CLI dep gate
// at the script's TWO positions (:258's own gate before the guidance block,
// and the gate inside configure_selected_recipes before the wizard — both
// NATIVE since U7-1 T3: internal/deps.Gate through depgate.go's
// recipeDepGate adapter, which opens its own raw-mode window per question),
// and the env offer at :269 through sync.OfferHarnessEnv (U4d-2). lib/recipe.sh
// never runs for this verb anymore — pinned by
// TestRecipeAddTailIsNativeOnATerminal.
//
// The legacy PRE-write gate recipe-add.py:155-181 (ensure_deps' pip offer
// for rich/questionary, then the questionary import) has NO native
// counterpart and does not need one: the native wizard's only TUI dependency
// is internal/tui itself, which cannot be missing from the running binary,
// so nothing is vendored and no run is refused for Python packages (recorded
// in L9; the native gate has no vendor tree, so S37's rc-3 arm died with the
// driver — L10). What retires for this route (S9): the BINARY's use of
// recipe-add.sh/recipe-add.py on EVERY run, TTY included — nothing in this
// verb execs a legacy file anymore, and physical deletion stays U7/U8.
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

// runRecipeAdd is the native `recipe add` entry: recipe-add.sh's argument
// loop first (usage errors exit 2 before anything touches the filesystem),
// the missing-id arm, the cd/pwd target resolution, then add_recipe itself —
// whose TTY tail (U6e-flipped, see the header) lives inside the verb, after
// the recipe is loaded and the tail's needs are known.
//
// exePath is the running dispatcher's own path (selfPath());
// recipe.CatalogHome anchors the catalogue resolver's second candidate on it.
func runRecipeAdd(args []string, home string, stdin io.Reader, stdout, stderr io.Writer, exePath string, ttyNow bool) int {
	return runRecipeAddFlow(args, home, stdin, stdout, stderr, exePath, ttyNow,
		productionRecipeTailSeams(home, stdin, stdout, stderr))
}

// runRecipeAddFlow is runRecipeAdd with the TTY tail's seams injected — the
// runInit/openInitWizardKeys pattern: the dispatcher keeps calling
// runRecipeAdd, which supplies the PRODUCTION seams (recipeDepGate over the
// native gate, openRecipeWizardPrompts over the raw-mode terminal, and the
// sync.OfferHarnessEnv offer), while a test scripts the gate answers, the
// wizard's answers and the env offer instead of driving a PTY.
func runRecipeAddFlow(args []string, home string, stdin io.Reader, stdout, stderr io.Writer, exePath string, ttyNow bool, seams recipeTailSeams) int {
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
	return runRecipeAddVerb(resolved, recipeID, home, exePath, stdin, stdout, stderr, ttyNow, seams)
}

// runRecipeAddVerb is add_recipe (recipe-add.py:115-296): the manifest guard
// first, the internal-test refusal, the catalog read, the duplicate refusal,
// the placeholder append through config.AppendRecipeSegments (the ONE
// manifest writer, S6), the materialize listing, and — see the header — the
// TTY tail through the injected seams (confirm → gate → wizard → env offer,
// each at the script's own position) or the non-TTY next-steps guidance.
func runRecipeAddVerb(target, recipeID, home, exePath string, stdin io.Reader, stdout, stderr io.Writer, ttyNow bool, seams recipeTailSeams) int {
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

	// recipe-add.py:245-275's guidance split, every leg native since U6e
	// (see the header): a blank line, the "Configure now?" confirm — a
	// cancel/abort wording the frozen decline — the gate at its two script
	// positions, the wizard + write-back when config fields exist, and the
	// env offer last. Off the TTY, or with nothing to configure, it is the
	// frozen "Next steps:" block, unchanged.
	if ttyNow && (hasConfig || hasMcpEnv) {
		fmt.Fprintln(stdout)
		prompts, closeKeys, rc := seams.openWizard(stdin, stdout)
		if rc != 0 || prompts == nil || closeKeys == nil {
			// The TUI cannot open: questionary's `.ask()` answers None at the
			// same point, which the script words as a declined confirm — the
			// recorded L9 deviation (the Python would traceback uncaught;
			// never exercised on a real terminal).
			fmt.Fprintln(stdout, "You can configure later with: ai-specs configure-recipes")
			return 0
		}
		answer, ok := prompts.Confirm("Configure now?", true)
		closeKeys()
		if !ok || !answer {
			fmt.Fprintln(stdout, "You can configure later with: ai-specs configure-recipes")
			return 0
		}

		// The dep gate, at recipe-add.py's own positions: :258 gates in the
		// tail itself when there are no config fields (a decline prints the
		// guidance block), and configure_selected_recipes gates internally
		// when there are (a decline prints ITS frozen Skipped line on stderr).
		// internal/deps.Gate owns both; an erroring gate (its terminal could
		// not be opened) words each call site's own frozen catch. The rc-3
		// arm (the driver's vendor bootstrap) and the rc-11 arm (the driver's
		// own read failure) died with the driver: the native gate has neither
		// a vendor tree to refuse over nor a second read to fail.
		configureNow := hasConfig
		if len(loaded.CliDeps) > 0 {
			outcome := seams.depGate(loaded)
			// The error arm goes FIRST: gateOutcome reports a failure with
			// Proceed still false, so testing !Proceed first would word every
			// gate error as a decline and report success the gate never
			// granted.
			switch {
			case outcome.Err != nil:
				if hasConfig {
					// recipe-add.py:266-267 wraps configure_selected_recipes in
					// `except Exception` — one line, no detail.
					fmt.Fprintln(stdout, "  ! could not open the configuration wizard")
					configureNow = false
				} else {
					// recipe-add.py:76's _run_cli_dep_gate catch — its own line.
					fmt.Fprintf(stdout, "  ! CLI dependency check skipped: %v\n", outcome.Err)
				}
			case !outcome.Proceed:
				if hasConfig {
					fmt.Fprintf(stderr, "Skipped %s (missing CLI deps)\n", recipeID)
					configureNow = false
				} else {
					recipePrintCLIDepGuidance(loaded, stdout)
				}
			}
		}

		if configureNow {
			prompts, closeKeys, rc = seams.openWizard(stdin, stdout)
			if rc != 0 || prompts == nil || closeKeys == nil {
				// questionary raising inside run_config_wizard: add's catch.
				fmt.Fprintln(stdout, "  ! could not open the configuration wizard")
			} else {
				// configure_selected_recipes' existing-config read: the manifest
				// exactly as the append above left it (Python re-reads it too).
				existing := map[string]any{}
				if data, lerr := config.LoadManifest(tomlPath); lerr == nil {
					existing = wizardExistingConfig(data, recipeID)
				}
				values, wok, werr := runConfigWizard(recipeID, loaded.ConfigSchema, existing, prompts)
				closeKeys()
				switch {
				case werr != nil:
					// recipe-add.py:266-267's `except Exception` prints this one
					// line for ANY failure inside the wizard call.
					fmt.Fprintln(stdout, "  ! could not open the configuration wizard")
				case wok && len(values) > 0:
					if _, refusal, uerr := config.UpdateRecipeConfig(tomlPath, recipeID, sortedWizardConfigValues(values)); uerr != nil || refusal != "" {
						fmt.Fprintln(stdout, "  ! could not open the configuration wizard")
					}
				}
				// A cancelled or all-blank wizard is Python's falsy `values`:
				// nothing written, nothing printed.
			}
		}

		// recipe-add.py:269-274's env leg: collect+offer inside
		// sync.OfferHarnessEnv (U4d-2), scoped to THIS recipe id since U7-3
		// (S5 — recipe-add.py:273's `recipe_ids=[recipe_id]`), its failure
		// wording this one frozen line and leaving the append standing.
		if hasMcpEnv {
			if err := seams.envOffer(target, []string{recipeID}); err != nil {
				fmt.Fprintln(stdout, "  ! could not configure environment variables")
			}
		}
		return 0
	}

	// Non-TTY guidance (recipe-add.py's else branch): "Next steps:" with one
	// line per present condition, nothing otherwise — unchanged by the seam
	// narrowing above.
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

// ── GO-20 U6d-1 (T4): `recipe configure` ──
//
// Native `ai-specs recipe configure` — the port of lib/recipe.sh's
// `configure` arm, which execs lib/_internal/recipe-configure.py directly, so
// recipe-configure.py IS the contract end to end: argparse's argument surface
// (`-h/--help`, `--inspect`, `--set KEY=VALUE` repeatable, `--dry-run`,
// `--sync`, `--ignore-cli-version`, `--json`, two positional slots, and the
// error arms), `_schema_for`'s wrapped messages, the PROJECT_OWNED_KEYS
// routing of `repo_topology` to `[project]`, and the JSON report —
// `json.dumps(document, ensure_ascii=False, …)` with `indent=2` or the
// compact `(",", ":")` separators — byte for byte. The argparse surface is
// emulated (not invoked): help/usage rendering reproduces argparse's width
// algorithm (COLUMNS → stdout TIOCGWINSZ → 80, minus 2), the registration
// order drives prefix abbreviation and the ambiguous-option message, and the
// parse-error precedence (help during the walk, then required, then
// unrecognized) matches parser.parse_args. The reference the port FOLLOWS is
// Python 3.14 — the interpreter the differential oracle runs on this machine
// (PATH=/tmp/pybin/python3 is 3.14.7), so S3's byte comparison is measured
// against it. Python 3.11 was probed side by side: help/usage (default and
// COLUMNS-pinned), the unknown/ambiguous/expected-one-argument/required arms
// are byte-identical, and ONE form diverges — an interleaved trailing
// positional (`recipe configure cfg --inspect /path` and `… --set k=v /path`)
// is `unrecognized arguments` + exit 2 on 3.11 and accepted on 3.14. The port
// accepts the 3.14 form; the differential case "interleaved positional fills
// the path slot" pins it (it fails against a ≤3.11 oracle, recording the
// divergence). Pinned differentially against the legacy route below.
//
// The write half is ROUTED, not re-implemented (S5/S6): config.ParseConfigAssignment,
// config.ValidateConfigValues, config.UpdateRecipeConfig (the promoted gate
// writer) and config.WriteProjectKey own the value domain and both writers —
// this file owns only the verb's contract around them: the argument surface,
// the report document, the preflight (internal/cliversion), the topology
// grounding (internal/target) and the --sync leg.
//
// The --sync leg, decided (L6): apply_project runs `<home>/bin/ai-specs sync
// <root> [--ignore-cli-version]` and then `<home>/bin/ai-specs doctor <root>`
// as SUBPROCESSES and parses their captured output — the last `syncing …`
// line for sync.failed_step, the last `Summary: N OK, N INFO, N WARN, N
// ERROR` line for verify. The leg is therefore ported IN FULL by spawning the
// SAME front-door command the script spawns (never Python directly, S1), and
// both parse targets are printed identically by the native sync and doctor
// ports (internal/sync/step.go's `  syncing <label>` and
// internal/doctor/doctor.go's `Summary: %d OK, …`), so the regexes keep
// working whatever implementation sits behind bin/ai-specs today (bash →
// lib/*.sh, the Go binary after U2's launcher flip). It is NOT half-ported
// and NOT moved to a legacy seam; the differential does not run --sync over
// the real spine (that would measure sync's writes, not this verb's report),
// so the leg's parsing, report and exit codes are pinned byte-for-byte
// against the Python reference over a STUB front door by
// TestRecipeConfigureSyncLegDifferential.
//
// Documented deviations (the S12/ADR 0002 class): parse-error DETAIL inside
// `invalid manifest:` / `invalid TOML value for …` / `invalid TOML after
// config write:` comes from internal/toml instead of tomllib (the frozen
// framings are byte-identical; recipeconfig_test.go pins Go's own guard);
// a front door that cannot be spawned prints Go's exec error and exits 1 with
// no report, where Python dies with an uncaught FileNotFoundError traceback
// (same exit code, no report either way); a value json.dumps cannot serialize
// (a TOML datetime in config) prints Go's serialization error and exits 1
// where Python dies with a TypeError traceback (both unexercised); and a
// catalog id whose path spelling Python's Path keeps uncleaned (interior
// `..`) is joined without cleaning here, so the `unknown recipe` message
// carries Python's spelling.

// recipeConfigureProg is recipe-configure.py's argparse `prog`.
const recipeConfigureProg = "ai-specs recipe configure"

// recipeConfigureProjectOwnedKeys is recipe-configure.py's
// PROJECT_OWNED_KEYS: values the project manifest owns, written to [project]
// through config.WriteProjectKey instead of the recipe's config block.
var recipeConfigureProjectOwnedKeys = map[string]bool{"repo_topology": true}

// recipeConfigureOption is one registered argparse option.
type recipeConfigureOption struct {
	name   string // canonical option string: "--set", "--inspect", …
	joined string // the message spelling: argparse joins all option strings with "/"
	value  bool   // takes one value (--set)
	help   bool   // the -h/--help action
	short  bool   // reachable as a single-dash group prefix ("-h…")
}

// recipeConfigureOptionOrder is argparse's registration order: -h/--help is
// registered first (by ArgumentParser), then the add_argument calls in
// recipe-configure.py's order. Prefix abbreviation candidates and the
// ambiguous-option message read this order, and the help section prints
// rows in it.
var recipeConfigureOptionOrder = []recipeConfigureOption{
	{name: "-h", joined: "-h/--help", help: true, short: true},
	{name: "--help", joined: "-h/--help", help: true},
	{name: "--inspect", joined: "--inspect"},
	{name: "--set", joined: "--set", value: true},
	{name: "--dry-run", joined: "--dry-run"},
	{name: "--sync", joined: "--sync"},
	{name: "--ignore-cli-version", joined: "--ignore-cli-version"},
	{name: "--json", joined: "--json"},
}

func recipeConfigureExactOption(base string) (*recipeConfigureOption, bool) {
	for i := range recipeConfigureOptionOrder {
		opt := &recipeConfigureOptionOrder[i]
		if opt.name == base {
			return opt, true
		}
	}
	return nil, false
}

// recipeConfigureOptionLike mirrors argparse's "looks like an option" test:
// starts with a prefix character and is not the single "-" (a lone dash is a
// positional; negative-number-looking tokens are positionals too).
func recipeConfigureOptionLike(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	return !recipeConfigureNegativeNumber(arg)
}

// recipeConfigureNegativeNumber mirrors argparse's
// _negative_number_matcher (^-\d+$|^-\d*\.\d+$): such tokens are positionals.
func recipeConfigureNegativeNumber(arg string) bool {
	s := arg[1:]
	isDigits := func(x string, allowEmpty bool) bool {
		if x == "" {
			return allowEmpty
		}
		for i := 0; i < len(x); i++ {
			if x[i] < '0' || x[i] > '9' {
				return false
			}
		}
		return true
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		if strings.Contains(s[i+1:], ".") {
			return false
		}
		return isDigits(s[:i], true) && isDigits(s[i+1:], false)
	}
	return isDigits(s, false)
}

// recipeConfigureParseKind classifies the argument contract's outcomes.
type recipeConfigureParseKind int

const (
	recipeConfigureParseRun recipeConfigureParseKind = iota
	recipeConfigureParseHelp
	recipeConfigureParseError
)

// recipeConfigureOptions is argparse's parsed namespace for the verb.
type recipeConfigureOptions struct {
	recipeID         string
	path             string
	hasPath          bool
	inspect          bool
	dryRun           bool
	sync             bool
	ignoreCLIVersion bool
	asJSON           bool
	assignments      []string
}

// recipeConfigureParse is one walk of the argument list.
type recipeConfigureParse struct {
	kind   recipeConfigureParseKind
	opts   recipeConfigureOptions
	errMsg string
}

// parseRecipeConfigure reproduces argparse's parse for this parser:
// left-to-right consumption (help, ambiguity and the value-taking refusals
// fire WHEN REACHED), then the required-positional check, then the
// unrecognized-argument leftover — parser.parse_args' own precedence.
// `--` makes every remaining argument positional; unknown option tokens are
// leftovers, never consumed as values.
func parseRecipeConfigure(args []string) recipeConfigureParse {
	res := recipeConfigureParse{kind: recipeConfigureParseRun}
	opts := &res.opts
	type positional struct {
		order int
		text  string
	}
	type leftover struct {
		order int
		text  string
	}
	var pos []positional
	var extras []leftover
	fail := func(msg string) recipeConfigureParse {
		res.kind = recipeConfigureParseError
		res.errMsg = msg
		return res
	}
	ignoredExplicit := func(joined, value string) recipeConfigureParse {
		return fail(fmt.Sprintf("argument %s: ignored explicit argument '%s'", joined, value))
	}

	afterDash := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if afterDash {
			pos = append(pos, positional{i, arg})
			continue
		}
		if arg == "--" {
			afterDash = true
			continue
		}
		if !recipeConfigureOptionLike(arg) {
			pos = append(pos, positional{i, arg})
			continue
		}

		base, explicit, hasExplicit := strings.Cut(arg, "=")
		opt, exact := recipeConfigureExactOption(base)
		matchedByShort := false
		if !exact {
			var matches []string
			if strings.HasPrefix(arg, "--") {
				for _, cand := range recipeConfigureOptionOrder {
					if strings.HasPrefix(cand.name, base) {
						matches = append(matches, cand.name)
					}
				}
				if len(matches) == 0 {
					extras = append(extras, leftover{i, arg})
					continue
				}
				if len(matches) > 1 {
					return fail(fmt.Sprintf("ambiguous option: %s could match %s",
						base, strings.Join(matches, ", ")))
				}
				opt, _ = recipeConfigureExactOption(matches[0])
			} else {
				// Single-dash group: "-h…" resolves through the two-character
				// prefix (argparse's short_option_prefix); the tail is only a
				// value when the action takes one, and -h is the only
				// single-dash option this parser registers.
				shortPrefix := arg[:2]
				for i := range recipeConfigureOptionOrder {
					cand := &recipeConfigureOptionOrder[i]
					if cand.name == shortPrefix {
						opt = cand
						break
					}
				}
				if opt == nil {
					extras = append(extras, leftover{i, arg})
					continue
				}
				matchedByShort = true
			}
		}
		if opt.help {
			if matchedByShort {
				// argparse parses more single-dash options out of the tail;
				// the help action fires first and exits.
				res.kind = recipeConfigureParseHelp
				return res
			}
			if hasExplicit {
				return ignoredExplicit(opt.joined, explicit)
			}
			res.kind = recipeConfigureParseHelp
			return res
		}
		if !opt.value {
			if hasExplicit {
				return ignoredExplicit(opt.joined, explicit)
			}
			switch opt.name {
			case "--inspect":
				opts.inspect = true
			case "--dry-run":
				opts.dryRun = true
			case "--sync":
				opts.sync = true
			case "--ignore-cli-version":
				opts.ignoreCLIVersion = true
			case "--json":
				opts.asJSON = true
			}
			continue
		}
		// --set: an explicit "=value" (possibly empty) or the next token when
		// that token is a positional for argparse — never an option-like one.
		if hasExplicit {
			opts.assignments = append(opts.assignments, explicit)
			continue
		}
		if i+1 >= len(args) || recipeConfigureOptionLike(args[i+1]) {
			return fail("argument --set: expected one argument")
		}
		i++
		opts.assignments = append(opts.assignments, args[i])
	}

	if len(pos) == 0 {
		return fail("the following arguments are required: recipe_id")
	}
	// Leftovers beyond the two positional slots join the unknown options in
	// argument order (argparse's `unrecognized arguments: ' '.join(argv)`).
	for i := 2; i < len(pos); i++ {
		extras = append(extras, leftover{pos[i].order, pos[i].text})
	}
	if len(extras) > 0 {
		sort.SliceStable(extras, func(a, b int) bool { return extras[a].order < extras[b].order })
		texts := make([]string, 0, len(extras))
		for _, ex := range extras {
			texts = append(texts, ex.text)
		}
		return fail("unrecognized arguments: " + strings.Join(texts, " "))
	}
	opts.recipeID = pos[0].text
	if len(pos) > 1 {
		opts.path = pos[1].text
		opts.hasPath = true
	}
	return res
}

// recipeConfigureColumns mirrors shutil.get_terminal_size as argparse's
// HelpFormatter reads it: a positive integer COLUMNS wins, else the stdout
// terminal size (TIOCGWINSZ), else the 80-column fallback.
func recipeConfigureColumns() int {
	if c := os.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(c)); err == nil && n > 0 {
			return n
		}
	}
	var ws struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno == 0 && ws.Col > 0 {
		return int(ws.Col)
	}
	return 80
}

// recipeConfigureUsageLines is argparse HelpFormatter's usage block for this
// parser: `usage: ` + prog + the option parts + the positional parts, wrapped
// by get_lines' greedy rule at textWidth = columns-2, with the prog-followed
// indent when `len(prefix)+len(prog) <= 0.75*textWidth` and prog on its own
// line otherwise. Lines carry no trailing newline.
func recipeConfigureUsageLines(textWidth int) []string {
	const prefix = "usage: "
	prog := recipeConfigureProg
	optParts := []string{"[-h]", "[--inspect]", "[--set ASSIGNMENTS]", "[--dry-run]",
		"[--sync]", "[--ignore-cli-version]", "[--json]"}
	posParts := []string{"recipe_id", "[path]"}
	joined := strings.Join(append(append([]string{prog}, optParts...), posParts...), " ")
	if len(prefix)+len(joined) <= textWidth {
		return []string{prefix + joined}
	}

	getLines := func(parts []string, indent string, firstIsPrefix bool) []string {
		var out []string
		var line []string
		lineLen := len(indent) - 1
		if firstIsPrefix {
			lineLen = len(prefix) - 1
		}
		for _, part := range parts {
			if lineLen+1+len(part) > textWidth && len(line) > 0 {
				content := strings.Join(line, " ")
				if firstIsPrefix && len(out) == 0 {
					out = append(out, prefix+content)
				} else {
					out = append(out, indent+content)
				}
				line = nil
				lineLen = len(indent) - 1
			}
			line = append(line, part)
			lineLen += len(part) + 1
		}
		if len(line) > 0 {
			content := strings.Join(line, " ")
			if firstIsPrefix && len(out) == 0 {
				out = append(out, prefix+content)
			} else {
				out = append(out, indent+content)
			}
		}
		return out
	}

	if float64(len(prefix)+len(prog)) <= 0.75*float64(textWidth) {
		indent := strings.Repeat(" ", len(prefix)+len(prog)+1)
		lines := getLines(append([]string{prog}, optParts...), indent, true)
		return append(lines, getLines(posParts, indent, false)...)
	}
	indent := strings.Repeat(" ", len(prefix))
	lines := getLines(append(append([]string{}, optParts...), posParts...), indent, false)
	if len(lines) > 1 {
		lines = append(getLines(optParts, indent, false), getLines(posParts, indent, false)...)
	}
	return append([]string{prog}, lines...)
}

// recipeConfigureWrapWords greedily wraps one help string (argparse routes
// help text through textwrap.wrap; every word of this parser's only help
// string is shorter than the minimum help width, so no long-word break is
// reachable).
func recipeConfigureWrapWords(text string, width int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(text) {
		switch {
		case cur == "":
			cur = word
		case len(cur)+1+len(word) <= width:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// recipeConfigureHelpText is parser.format_help() byte for byte: the usage
// block, a blank line, the `positional arguments:` section, a blank line and
// the `options:` section (Python ≥3.10's group title), with the -h row padded
// to argparse's help column (min(action_max_length+2, max_help_position),
// where the group indent makes action_max_length the longest invocation + 2).
func recipeConfigureHelpText(columns int) string {
	textWidth := columns - 2
	var sb strings.Builder
	sb.WriteString(strings.Join(recipeConfigureUsageLines(textWidth), "\n"))
	sb.WriteString("\n\n")
	sb.WriteString("positional arguments:\n  recipe_id\n  path\n\n")
	sb.WriteString("options:\n")
	rows := []struct{ inv, help string }{
		{"-h, --help", "show this help message and exit"},
		{"--inspect", ""},
		{"--set ASSIGNMENTS", ""},
		{"--dry-run", ""},
		{"--sync", ""},
		{"--ignore-cli-version", ""},
		{"--json", ""},
	}
	maxInvocation := len("recipe_id") // the positionals measure too
	for _, r := range rows {
		maxInvocation = max(maxInvocation, len(r.inv))
	}
	maxHelpPosition := min(24, max(textWidth-20, 4))
	helpPosition := min(maxInvocation+2+2, maxHelpPosition)
	actionWidth := helpPosition - 4 // group indent 2, then the two pad spaces
	for _, r := range rows {
		if r.help == "" {
			sb.WriteString("  " + r.inv + "\n")
			continue
		}
		if len(r.inv) <= actionWidth {
			sb.WriteString("  " + r.inv + strings.Repeat(" ", actionWidth-len(r.inv)) + "  " + r.help + "\n")
			continue
		}
		sb.WriteString("  " + r.inv + "\n")
		helpWidth := max(textWidth-helpPosition, 11)
		for _, line := range recipeConfigureWrapWords(r.help, helpWidth) {
			sb.WriteString(strings.Repeat(" ", helpPosition) + line + "\n")
		}
	}
	return sb.String()
}

// recipeConfigureErrorText is argparse's error() surface: the usage block on
// its own (format_usage strips the trailing blank line) then the frozen
// `prog: error: message` line, all to stderr with exit 2.
func recipeConfigureErrorText(columns int, msg string) string {
	return strings.Join(recipeConfigureUsageLines(columns-2), "\n") + "\n" +
		recipeConfigureProg + ": error: " + msg + "\n"
}

// ── the report document ──
//
// recipe-configure.py builds plain dicts and prints them through
// json.dumps(document, ensure_ascii=False, indent=2) — or the compact
// (",", ":") separators with --json — with insertion order preserved
// (sort_keys=False). rcJSON is that document: an insertion-ordered object,
// an array, a pre-rendered number literal, a raw JSON fragment (the schema
// shape, straight out of schema.ConfigTable.ShapeJSON) or a string; the
// writer escapes only what json's ensure_ascii=False encoder escapes (the
// quotes, the backslash and the control characters below 0x20) and leaves
// every other rune — 0x7f included — raw.

type rcJSONKind int

const (
	rcJSONNull rcJSONKind = iota
	rcJSONBool
	rcJSONNumber // pre-rendered literal
	rcJSONString // value, escaped at write time
	rcJSONArray
	rcJSONObject
)

type rcJSON struct {
	kind rcJSONKind
	b    bool
	num  string
	str  string
	arr  []rcJSON
	keys []string
	obj  map[string]rcJSON
}

func rcNull() rcJSON { return rcJSON{kind: rcJSONNull} }

func rcBool(v bool) rcJSON { return rcJSON{kind: rcJSONBool, b: v} }

func rcNumber(lit string) rcJSON { return rcJSON{kind: rcJSONNumber, num: lit} }

func rcInt(v int) rcJSON { return rcJSON{kind: rcJSONNumber, num: strconv.Itoa(v)} }

func rcString(v string) rcJSON { return rcJSON{kind: rcJSONString, str: v} }

func rcNewArray(items ...rcJSON) rcJSON {
	if items == nil {
		items = []rcJSON{}
	}
	return rcJSON{kind: rcJSONArray, arr: items}
}

func rcNewObject() rcJSON {
	return rcJSON{kind: rcJSONObject, obj: map[string]rcJSON{}}
}

// set stores a key, preserving Python dict semantics: an existing key keeps
// its position, a new key is appended.
func (n *rcJSON) set(key string, v rcJSON) {
	if n.kind != rcJSONObject {
		return
	}
	if _, exists := n.obj[key]; !exists {
		n.keys = append(n.keys, key)
	}
	n.obj[key] = v
}

func (n rcJSON) get(key string) rcJSON {
	if n.kind != rcJSONObject {
		return rcNull()
	}
	v, ok := n.obj[key]
	if !ok {
		return rcNull()
	}
	return v
}

// rcQuoteJSON is json.dumps(s, ensure_ascii=False): the short escapes, \u00xx
// for the remaining control characters (lowercase hex, like json's), raw
// everything at or above 0x20 — including 0x7f and all non-ASCII.
func rcQuoteJSON(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// writeJSON renders the document like Python's json.dumps: compact with the
// (",", ":") separators, or indent=2 with each entry on its own line (empty
// containers stay `{}`/`[]`, like the C encoder).
func writeJSON(sb *strings.Builder, n rcJSON, depth int, compact bool) {
	switch n.kind {
	case rcJSONNull:
		sb.WriteString("null")
	case rcJSONBool:
		if n.b {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case rcJSONNumber:
		sb.WriteString(n.num)
	case rcJSONString:
		sb.WriteString(rcQuoteJSON(n.str))
	case rcJSONArray:
		if len(n.arr) == 0 {
			sb.WriteString("[]")
			return
		}
		if compact {
			sb.WriteByte('[')
			for i, item := range n.arr {
				if i > 0 {
					sb.WriteByte(',')
				}
				writeJSON(sb, item, depth, true)
			}
			sb.WriteByte(']')
			return
		}
		sb.WriteString("[\n")
		for i, item := range n.arr {
			sb.WriteString(strings.Repeat("  ", depth+1))
			writeJSON(sb, item, depth+1, false)
			if i < len(n.arr)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.Repeat("  ", depth) + "]")
	case rcJSONObject:
		if len(n.keys) == 0 {
			sb.WriteString("{}")
			return
		}
		if compact {
			sb.WriteByte('{')
			for i, k := range n.keys {
				if i > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(rcQuoteJSON(k))
				sb.WriteByte(':')
				writeJSON(sb, n.obj[k], depth, true)
			}
			sb.WriteByte('}')
			return
		}
		sb.WriteString("{\n")
		for i, k := range n.keys {
			sb.WriteString(strings.Repeat("  ", depth+1))
			sb.WriteString(rcQuoteJSON(k))
			sb.WriteString(": ")
			writeJSON(sb, n.obj[k], depth+1, false)
			if i < len(n.keys)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.Repeat("  ", depth) + "}")
	}
}

// rcFromTOML converts one TOML-domain value into the report's JSON domain the
// way json.dumps serializes tomllib's dicts. A value json.dumps refuses (a
// datetime) is an error here; Python dies with a TypeError traceback at emit
// time (recorded deviation — same exit, no report).
func rcFromTOML(v any) (rcJSON, error) {
	switch t := v.(type) {
	case nil:
		return rcNull(), nil
	case bool:
		return rcBool(t), nil
	case string:
		return rcString(t), nil
	case int:
		return rcNumber(strconv.FormatInt(int64(t), 10)), nil
	case int64:
		return rcNumber(strconv.FormatInt(t, 10)), nil
	case toml.BigInt:
		return rcNumber(string(t)), nil
	case float64:
		return rcNumber(config.FormatPythonJSONFloat(t)), nil
	case []any:
		out := rcNewArray(make([]rcJSON, 0, len(t))...)
		for _, item := range t {
			converted, err := rcFromTOML(item)
			if err != nil {
				return rcNull(), err
			}
			out.arr = append(out.arr, converted)
		}
		return out, nil
	case []*toml.Table:
		out := rcNewArray(make([]rcJSON, 0, len(t))...)
		for _, item := range t {
			converted, err := rcFromTOML(item)
			if err != nil {
				return rcNull(), err
			}
			out.arr = append(out.arr, converted)
		}
		return out, nil
	case *toml.Table:
		out := rcNewObject()
		if t != nil {
			for _, k := range t.Keys() {
				value, _ := t.Get(k)
				converted, err := rcFromTOML(value)
				if err != nil {
					return rcNull(), err
				}
				out.set(k, converted)
			}
		}
		return out, nil
	case toml.DateTime:
		return rcNull(), fmt.Errorf("Object of type %s is not JSON serializable", t.PyTypeName())
	default:
		return rcNull(), fmt.Errorf("Object of type %T is not JSON serializable", v)
	}
}

// rcFromJSONFragment decodes the ordered JSON fragment
// schema.ConfigTable.ShapeJSON produces into the report's document tree: the
// shape is embedded as DATA (json.dumps renders the dict at the report's own
// indent, or through the compact separators under --json), never as
// pre-rendered text. The grammar is the shape serializer's own — objects,
// arrays, strings and JSON literals — walked sequentially so declaration
// order survives.
func rcFromJSONFragment(s string) (rcJSON, error) {
	d := &rcJSONDecoder{src: s}
	v, err := d.decodeValue()
	if err != nil {
		return rcNull(), err
	}
	d.skipSpace()
	if d.pos != len(d.src) {
		return rcNull(), fmt.Errorf("trailing data after JSON fragment")
	}
	return v, nil
}

type rcJSONDecoder struct {
	src string
	pos int
}

func (d *rcJSONDecoder) skipSpace() {
	for d.pos < len(d.src) {
		switch d.src[d.pos] {
		case ' ', '\t', '\n', '\r':
			d.pos++
		default:
			return
		}
	}
}

func (d *rcJSONDecoder) decodeValue() (rcJSON, error) {
	d.skipSpace()
	if d.pos >= len(d.src) {
		return rcNull(), fmt.Errorf("unexpected end of JSON fragment")
	}
	switch c := d.src[d.pos]; {
	case c == '{':
		return d.decodeObject()
	case c == '[':
		return d.decodeArray()
	case c == '"':
		s, err := d.decodeString()
		if err != nil {
			return rcNull(), err
		}
		return rcString(s), nil
	case c == 't':
		return d.decodeLiteral("true", rcBool(true))
	case c == 'f':
		return d.decodeLiteral("false", rcBool(false))
	case c == 'n':
		return d.decodeLiteral("null", rcNull())
	default:
		start := d.pos
		for d.pos < len(d.src) && !strings.ContainsRune(",]} \t\n\r", rune(d.src[d.pos])) {
			d.pos++
		}
		if d.pos == start {
			return rcNull(), fmt.Errorf("invalid JSON token at %d", d.pos)
		}
		return rcNumber(d.src[start:d.pos]), nil
	}
}

func (d *rcJSONDecoder) decodeLiteral(word string, v rcJSON) (rcJSON, error) {
	if !strings.HasPrefix(d.src[d.pos:], word) {
		return rcNull(), fmt.Errorf("invalid JSON literal at %d", d.pos)
	}
	d.pos += len(word)
	return v, nil
}

func (d *rcJSONDecoder) decodeObject() (rcJSON, error) {
	d.pos++ // '{'
	out := rcNewObject()
	d.skipSpace()
	if d.pos < len(d.src) && d.src[d.pos] == '}' {
		d.pos++
		return out, nil
	}
	for {
		d.skipSpace()
		key, err := d.decodeString()
		if err != nil {
			return rcNull(), err
		}
		d.skipSpace()
		if d.pos >= len(d.src) || d.src[d.pos] != ':' {
			return rcNull(), fmt.Errorf("expected ':' at %d", d.pos)
		}
		d.pos++
		value, err := d.decodeValue()
		if err != nil {
			return rcNull(), err
		}
		out.set(key, value)
		d.skipSpace()
		if d.pos >= len(d.src) {
			return rcNull(), fmt.Errorf("unterminated JSON object")
		}
		switch d.src[d.pos] {
		case ',':
			d.pos++
		case '}':
			d.pos++
			return out, nil
		default:
			return rcNull(), fmt.Errorf("expected ',' or '}' at %d", d.pos)
		}
	}
}

func (d *rcJSONDecoder) decodeArray() (rcJSON, error) {
	d.pos++ // '['
	out := rcNewArray()
	d.skipSpace()
	if d.pos < len(d.src) && d.src[d.pos] == ']' {
		d.pos++
		return out, nil
	}
	for {
		value, err := d.decodeValue()
		if err != nil {
			return rcNull(), err
		}
		out.arr = append(out.arr, value)
		d.skipSpace()
		if d.pos >= len(d.src) {
			return rcNull(), fmt.Errorf("unterminated JSON array")
		}
		switch d.src[d.pos] {
		case ',':
			d.pos++
		case ']':
			d.pos++
			return out, nil
		default:
			return rcNull(), fmt.Errorf("expected ',' or ']' at %d", d.pos)
		}
	}
}

func (d *rcJSONDecoder) decodeString() (string, error) {
	if d.pos >= len(d.src) || d.src[d.pos] != '"' {
		return "", fmt.Errorf("expected string at %d", d.pos)
	}
	d.pos++
	var sb strings.Builder
	for d.pos < len(d.src) {
		c := d.src[d.pos]
		switch {
		case c == '"':
			d.pos++
			return sb.String(), nil
		case c == '\\':
			d.pos++
			if d.pos >= len(d.src) {
				return "", fmt.Errorf("unterminated escape")
			}
			esc := d.src[d.pos]
			d.pos++
			switch esc {
			case '"', '\\', '/':
				sb.WriteByte(esc)
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				if d.pos+4 > len(d.src) {
					return "", fmt.Errorf("short \\u escape")
				}
				code, err := strconv.ParseUint(d.src[d.pos:d.pos+4], 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid \\u escape: %v", err)
				}
				d.pos += 4
				sb.WriteRune(rune(code))
			default:
				return "", fmt.Errorf("invalid escape %q", esc)
			}
		default:
			sb.WriteByte(c)
			d.pos++
		}
	}
	return "", fmt.Errorf("unterminated string")
}

// ── recipe-configure.py's read surface ──

// configureLoadManifest is _load_manifest: the path spelling Python's
// Path.resolve() prints (recipeConfigureResolve owns the resolution) and the
// two frozen messages — `manifest not found:` for a missing/non-file path,
// `invalid manifest:` with diagnostic parse detail (ADR 0002) for a manifest
// tomllib would reject.
func configureLoadManifest(root string) (*toml.Table, error) {
	path := filepath.Join(root, "ai-specs", "ai-specs.toml")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return nil, fmt.Errorf("manifest not found: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest not found: %s", path)
	}
	parsed, perr := toml.Parse(data)
	if perr != nil {
		return nil, fmt.Errorf("invalid manifest: %s", perr)
	}
	return parsed, nil
}

// configureCatalogDir is _cli_home() / "catalog" / "recipes": the home
// verbatim (recipe-configure.py reads ONLY this candidate — no install-root
// fallback like recipe-list.py's second one).
func configureCatalogDir(home string) string {
	return filepath.Join(home, "catalog", "recipes")
}

// configureRecipeDir is `catalog_dir / recipe_id`: an absolute id replaces
// the catalogue root (Path join semantics) and the id's own spelling is kept
// uncleaned (interior `..` included), so messages carry Python's path text.
func configureRecipeDir(home, recipeID string) string {
	if filepath.IsAbs(recipeID) {
		return recipeID
	}
	return configureCatalogDir(home) + "/" + recipeID
}

// configureSchemaFor is recipe-configure.py's _schema_for: every failure is
// wrapped as `unknown recipe '<id>': <detail>`.
func configureSchemaFor(home, recipeID string) (*schema.Recipe, error) {
	dir := configureRecipeDir(home, recipeID)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("unknown recipe '%s': recipe directory not found: %s", recipeID, dir)
	}
	recipe, err := schema.LoadRecipeToml(dir + "/recipe.toml")
	if err != nil {
		return nil, fmt.Errorf("unknown recipe '%s': %s", recipeID, err)
	}
	return recipe, nil
}

// configureConfigDeclarationOrder re-reads recipe.toml for the ORDER of the
// [config] keys: schema.ConfigSchema exposes fields/tables as maps, but
// _schema_document emits them in TOML declaration order (fields first, then
// tables, each in relative declaration order) — the same re-read pattern
// internal/sync/buildrecipemcp.go uses for [[provides.mcp]] key order. A
// parse failure (impossible after configureSchemaFor succeeded) falls back to
// sorted keys; any key the maps do not carry (extras) is skipped.
func configureConfigDeclarationOrder(home, recipeID string) []string {
	data, err := os.ReadFile(configureRecipeDir(home, recipeID) + "/recipe.toml")
	if err != nil {
		return nil
	}
	parsed, err := toml.Parse(data)
	if err != nil {
		return nil
	}
	cfg, ok := parsed.Table("config")
	if !ok || cfg == nil {
		return nil
	}
	return cfg.Keys()
}

// configurePreflight is recipe-configure.py's _preflight over
// internal/cliversion (cli_version.py's port): the installed version, the
// [tool] policy and its exact-message failure, the lock's cli_version and the
// derived lock_state.
type configurePreflight struct {
	installed     string
	pin           string
	pinSet        bool
	pinKind       string
	pinKindSet    bool
	policyOK      bool
	lockCLI       string
	lockCLISet    bool
	lockState     string
	blockedReason string
}

func configureBuildPreflight(root string, manifest *toml.Table, home string) configurePreflight {
	pf := configurePreflight{installed: cliversion.ReadInstalledVersion(home), policyOK: true}
	policy, policyErr := cliversion.ParseToolPolicy(manifest)
	lockMeta := cliversion.ReadLockMeta(filepath.Join(root, "ai-specs", ".ai-specs.lock"))
	reason := policyErr
	if policyErr != "" {
		pf.policyOK = false
	}
	if policy != nil {
		pf.pin, pf.pinSet = policy.Version, true
		pf.pinKind, pf.pinKindSet = policy.Kind, true
		if policyErr == "" {
			ok, checkReason := cliversion.CheckPolicy(pf.installed, policy)
			pf.policyOK = ok
			reason = checkReason
		}
	}
	if v, ok := lockMeta["cli_version"]; ok {
		pf.lockCLI, pf.lockCLISet = v, true
		switch {
		case v != pf.installed:
			pf.lockState = "stale"
		default:
			pf.lockState = "current"
		}
	} else {
		pf.lockState = "unknown"
	}
	if !pf.policyOK {
		pf.blockedReason = reason
	}
	return pf
}

// configureCLIVersionNode is preflight["cli_version"]'s fixed key order.
func (pf configurePreflight) configureCLIVersionNode() rcJSON {
	node := rcNewObject()
	node.set("installed", rcString(pf.installed))
	if pf.pinSet {
		node.set("pin", rcString(pf.pin))
	} else {
		node.set("pin", rcNull())
	}
	if pf.pinKindSet {
		node.set("pin_kind", rcString(pf.pinKind))
	} else {
		node.set("pin_kind", rcNull())
	}
	node.set("policy_ok", rcBool(pf.policyOK))
	if pf.lockCLISet {
		node.set("lock_cli_version", rcString(pf.lockCLI))
	} else {
		node.set("lock_cli_version", rcNull())
	}
	node.set("lock_state", rcString(pf.lockState))
	return node
}

// configureTopology is recipe-configure.py's _topology_grounding: nil (no
// topology key in the recipe) or the grounding dict plus the assumptions in
// their fixed order.
func configureTopology(root string, recipe *schema.Recipe, manifest *toml.Table) (rcJSON, []string) {
	if recipe.ConfigSchema == nil {
		return rcNull(), nil
	}
	if _, ok := recipe.ConfigSchema.Fields["repo_topology"]; !ok {
		return rcNull(), nil
	}
	pt := target.ProjectRepoTopology(root, manifest)
	submodules := append([]string(nil), pt.Submodules...)
	sort.Strings(submodules)
	node := rcNewObject()
	node.set("resolved", rcString(pt.Resolved))
	node.set("configured", rcString(pt.Configured))
	node.set("via", rcString(pt.Via))
	node.set("source", rcString(pt.Source))
	node.set("submodules", rcNewArray(func() []rcJSON {
		out := make([]rcJSON, 0, len(submodules))
		for _, s := range submodules {
			out = append(out, rcString(s))
		}
		return out
	}()...))
	node.set("gitmodules_present", rcBool(pt.GitmodulesPresent))
	var assumptions []string
	if !pt.GitmodulesPresent && pt.Configured == "auto" {
		assumptions = append(assumptions,
			"auto detection cannot distinguish monorepo-apps from standalone; ask the user which topology applies")
	}
	if pt.Via == "auto" && !pt.GitmodulesPresent {
		assumptions = append(assumptions,
			"topology detection had no .gitmodules signal and may have degraded to standalone")
	}
	if pt.Deprecation != "" {
		assumptions = append(assumptions, pt.Deprecation)
	}
	return node, assumptions
}

// configureManifestMCP is `manifest["mcp"] if isinstance(..., dict) else {}`:
// only a [mcp] table counts — [[mcp]] array-of-tables (the list form) does
// not, exactly like toml-read.py's read_mcp.
func configureManifestMCP(manifest *toml.Table) map[string]bool {
	out := map[string]bool{}
	if manifest == nil {
		return out
	}
	raw, ok := manifest.Get("mcp")
	if !ok {
		return out
	}
	tbl, isTable := raw.(*toml.Table)
	if !isTable || tbl == nil {
		return out
	}
	for _, name := range tbl.Keys() {
		out[name] = true
	}
	return out
}

// configureGrounding is inspect_project's grounding block (mcp/init/cli_deps,
// then topology when the recipe declares it) plus the assumptions list.
func configureGrounding(root string, recipe *schema.Recipe, manifest *toml.Table) (rcJSON, []string) {
	requiredMCP := make([]rcJSON, 0, len(recipe.MCP))
	manifestMCP := configureManifestMCP(manifest)
	envVars := map[string]bool{}
	requiredIDs := make([]string, 0, len(recipe.MCP))
	for _, preset := range recipe.MCP {
		requiredIDs = append(requiredIDs, preset.ID)
		requiredMCP = append(requiredMCP, rcString(preset.ID))
		if env, ok := preset.Config["env"].(*toml.Table); ok && env != nil {
			for _, name := range env.Keys() {
				envVars[name] = true
			}
		}
	}
	// B-001 (dual-blind review): `present` is Python's
	// sorted(name for name in required_mcp if name in manifest_mcp) —
	// SORTED, never the recipe's declaration order (a fixture whose
	// [[provides.mcp]] presets are declared unsorted pins it in the
	// differential).
	var presentIDs []string
	for _, id := range requiredIDs {
		if manifestMCP[id] {
			presentIDs = append(presentIDs, id)
		}
	}
	sort.Strings(presentIDs)
	present := make([]rcJSON, 0, len(presentIDs))
	for _, id := range presentIDs {
		present = append(present, rcString(id))
	}

	grounding := rcNewObject()
	mcpNode := rcNewObject()
	mcpNode.set("required", rcBool(len(requiredMCP) > 0))
	mcpNode.set("present", rcNewArray(present...))
	mcpNode.set("env_vars", rcNewArray(sortedStringKeys(envVars, func(k string) rcJSON { return rcString(k) })...))
	grounding.set("mcp", mcpNode)

	initNode := rcNewObject()
	initNode.set("present", rcBool(recipe.Init != nil))
	if recipe.Init != nil {
		initNode.set("needs_manifest", rcBool(recipe.Init.NeedsManifest))
		needsMCP := append([]string(nil), recipe.Init.NeedsMCP...)
		sort.Strings(needsMCP)
		initNode.set("needs_mcp", rcNewArray(func() []rcJSON {
			out := make([]rcJSON, 0, len(needsMCP))
			for _, s := range needsMCP {
				out = append(out, rcString(s))
			}
			return out
		}()...))
	} else {
		initNode.set("needs_manifest", rcBool(false))
		initNode.set("needs_mcp", rcNewArray())
	}
	grounding.set("init", initNode)

	cliDeps := append([]*schema.CliDep(nil), recipe.CliDeps...)
	sort.SliceStable(cliDeps, func(i, j int) bool { return cliDeps[i].Binary < cliDeps[j].Binary })
	depNodes := make([]rcJSON, 0, len(cliDeps))
	for _, dep := range cliDeps {
		node := rcNewObject()
		node.set("name", rcString(dep.Binary))
		_, presentErr := exec.LookPath(dep.Binary)
		node.set("present", rcBool(presentErr == nil))
		depNodes = append(depNodes, node)
	}
	grounding.set("cli_deps", rcNewArray(depNodes...))

	topology, assumptions := configureTopology(root, recipe, manifest)
	if topology.kind != rcJSONNull {
		grounding.set("topology", topology)
	}
	return grounding, assumptions
}

// sortedStringKeys returns the map's keys sorted (Python's sorted()) through
// the value mapper.
func sortedStringKeys(set map[string]bool, to func(string) rcJSON) []rcJSON {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]rcJSON, 0, len(keys))
	for _, k := range keys {
		out = append(out, to(k))
	}
	return out
}

// configureSchemaDocument is _schema_document: every standard field in TOML
// declaration order (fields first), then every structured table in its
// declaration order, each row in the Python dict's fixed key order; the table
// rows carry the declared shape verbatim (schema.ConfigTable.ShapeJSON — the
// same JSON STRUCTURED_CONFIG_SHAPES serializes to on the Python side).
func configureSchemaDocument(home, recipeID string, recipe *schema.Recipe) (rcJSON, error) {
	fields := map[string]*schema.ConfigField{}
	tables := map[string]*schema.ConfigTable{}
	if recipe.ConfigSchema != nil {
		fields = recipe.ConfigSchema.Fields
		tables = recipe.ConfigSchema.Tables
	}
	order := configureConfigDeclarationOrder(home, recipeID)
	appendSortedMissing := func(keys []string, present map[string]bool) []string {
		var missing []string
		for _, k := range keys {
			if !present[k] {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		return append(keys, missing...)
	}
	fieldKeys := make([]string, 0, len(fields))
	tableKeys := make([]string, 0, len(tables))
	inFields := map[string]bool{}
	inTables := map[string]bool{}
	for _, k := range order {
		if _, ok := fields[k]; ok {
			fieldKeys = append(fieldKeys, k)
			inFields[k] = true
		}
		if _, ok := tables[k]; ok {
			tableKeys = append(tableKeys, k)
			inTables[k] = true
		}
	}
	allFields := make([]string, 0, len(fields))
	for k := range fields {
		allFields = append(allFields, k)
	}
	allTables := make([]string, 0, len(tables))
	for k := range tables {
		allTables = append(allTables, k)
	}
	if order == nil {
		sort.Strings(allFields)
		sort.Strings(allTables)
		fieldKeys, tableKeys = allFields, allTables
	} else {
		fieldKeys = appendSortedMissing(fieldKeys, inFields)
		tableKeys = appendSortedMissing(tableKeys, inTables)
	}

	document := rcNewObject()
	rows := make([]rcJSON, 0, len(fieldKeys)+len(tableKeys))
	for _, key := range fieldKeys {
		field := fields[key]
		row := rcNewObject()
		row.set("key", rcString(key))
		row.set("type", rcString(config.RecipeConfigFieldType(field)))
		row.set("required", rcBool(field.Required))
		if field.Enum != nil {
			enum := make([]rcJSON, 0, len(field.Enum))
			for _, v := range field.Enum {
				enum = append(enum, rcString(v))
			}
			row.set("enum", rcNewArray(enum...))
		} else {
			row.set("enum", rcNull())
		}
		def, err := rcFromTOML(field.Default)
		if err != nil {
			return rcNull(), err
		}
		row.set("default", def)
		row.set("help_text", rcString(field.HelpText))
		rows = append(rows, row)
	}
	for _, key := range tableKeys {
		shape, err := tables[key].ShapeJSON()
		if err != nil {
			return rcNull(), err
		}
		row := rcNewObject()
		row.set("key", rcString(key))
		row.set("type", rcString("table"))
		row.set("required", rcBool(false))
		row.set("enum", rcNull())
		row.set("default", rcNull())
		row.set("help_text", rcString(""))
		shapeTree, shapeErr := rcFromJSONFragment(string(shape))
		if shapeErr != nil {
			return rcNull(), shapeErr
		}
		row.set("shape", shapeTree)
		rows = append(rows, row)
	}
	document.set("fields", rcNewArray(rows...))
	return document, nil
}

// configureStateNode is _recipe_state rendered: the config table as the
// sorted `current_config` dict.
func configureCurrentConfig(current *toml.Table) (rcJSON, error) {
	out := rcNewObject()
	if current == nil {
		return out, nil
	}
	keys := append([]string(nil), current.Keys()...)
	sort.Strings(keys)
	for _, k := range keys {
		value, _ := current.Get(k)
		converted, err := rcFromTOML(value)
		if err != nil {
			return rcNull(), err
		}
		out.set(k, converted)
	}
	return out, nil
}

// configureInspect is inspect_project: manifest, schema, state, grounding,
// the lock meta, unknown keys and assumptions — the document dict in its
// fixed key order.
func configureInspect(root, recipeID, home string) (rcJSON, error) {
	manifest, err := configureLoadManifest(root)
	if err != nil {
		return rcNull(), err
	}
	recipe, err := configureSchemaFor(home, recipeID)
	if err != nil {
		return rcNull(), err
	}
	present, enabled, current := config.RecipeConfigState(manifest, recipeID)

	schemaDocument, err := configureSchemaDocument(home, recipeID, recipe)
	if err != nil {
		return rcNull(), err
	}
	currentConfig, err := configureCurrentConfig(current)
	if err != nil {
		return rcNull(), err
	}
	grounding, assumptions := configureGrounding(root, recipe, manifest)

	// _preflight(project_root, manifest)["cli_version"] — the FULL
	// cli_version block (the Python local that receives it is named
	// `lock_meta`, but _preflight's dict is what it holds; the lock meta
	// feeds it inside configureBuildPreflight).
	pf := configureBuildPreflight(root, manifest, home)
	preflightNode := rcNewObject()
	preflightNode.set("cli_version", pf.configureCLIVersionNode())

	unknown := []string{}
	if current != nil {
		addressable := map[string]bool{}
		if recipe.ConfigSchema != nil {
			for k := range recipe.ConfigSchema.Fields {
				addressable[k] = true
			}
			for k := range recipe.ConfigSchema.Tables {
				addressable[k] = true
			}
		}
		for _, k := range current.Keys() {
			if !addressable[k] {
				unknown = append(unknown, k)
			}
		}
	}
	sort.Strings(unknown)
	unknownNodes := make([]rcJSON, 0, len(unknown))
	for _, k := range unknown {
		unknownNodes = append(unknownNodes, rcString(k))
	}
	assumptionNodes := make([]rcJSON, 0, len(assumptions))
	for _, a := range assumptions {
		assumptionNodes = append(assumptionNodes, rcString(a))
	}

	doc := rcNewObject()
	doc.set("schema_version", rcNumber("1"))
	recipeNode := rcNewObject()
	recipeNode.set("id", rcString(recipeID))
	recipeNode.set("enabled", rcBool(enabled))
	recipeNode.set("present_in_manifest", rcBool(present))
	doc.set("recipe", recipeNode)
	doc.set("schema", schemaDocument)
	doc.set("current_config", currentConfig)
	doc.set("grounding", grounding)
	doc.set("preflight", preflightNode)
	doc.set("unknown_keys", rcNewArray(unknownNodes...))
	doc.set("assumptions", rcNewArray(assumptionNodes...))
	return doc, nil
}

// ── recipe-configure.py's apply_project report ──

// configureBaseReport is _base_report: the fixed key order (report_version …
// gaps), with the preflight's cli_version object, the optional
// blocked_reason key (the error paths' `{"cli_version": {}}` preflight does
// NOT carry it — Python's dict literal lacks the key entirely) and
// ignore_cli_version appended last inside preflight.
func configureBaseReport(recipeID string, cliVersion rcJSON, blockedReason rcJSON, hasBlockedKey bool, ignore bool) rcJSON {
	preflight := rcNewObject()
	preflight.set("cli_version", cliVersion)
	if hasBlockedKey {
		preflight.set("blocked_reason", blockedReason)
	}
	preflight.set("ignore_cli_version", rcBool(ignore))

	syncNode := rcNewObject()
	syncNode.set("ran", rcBool(false))
	syncNode.set("exit_code", rcNull())
	syncNode.set("failed_step", rcNull())
	syncNode.set("rolled_back", rcBool(false))
	syncNode.set("lock_stamped", rcBool(false))

	verifyNode := rcNewObject()
	verifyNode.set("doctor_exit", rcNull())
	verifyNode.set("parsed", rcBool(false))
	verifyNode.set("ok", rcNull())
	verifyNode.set("info", rcNull())
	verifyNode.set("warn", rcNull())
	verifyNode.set("error", rcNull())

	applied := rcNewObject()
	applied.set("changed", rcNewArray())
	applied.set("unchanged", rcNewArray())
	applied.set("preserved", rcNewArray())

	report := rcNewObject()
	report.set("report_version", rcNumber("1"))
	report.set("status", rcString("rejected"))
	report.set("recipe", rcString(recipeID))
	report.set("applied", applied)
	report.set("preflight", preflight)
	report.set("sync", syncNode)
	report.set("verify", verifyNode)
	report.set("assumptions", rcNewArray())
	report.set("drift", rcNewArray())
	report.set("gaps", rcNewArray())
	return report
}

// configureRejectedReport is every ConfigureError arm: the
// `{"cli_version": {}}` preflight and the reason appended after gaps.
func configureRejectedReport(recipeID, reason string, ignore bool) rcJSON {
	report := configureBaseReport(recipeID, rcNewObject(), rcNull(), false, ignore)
	report.set("reason", rcString(reason))
	return report
}

// configureFrontDoor runs `<home>/bin/ai-specs <argv…>` exactly like
// recipe-configure.py's _run_command: cwd = the project root, AI_SPECS_HOME
// pinned to the CLI home, stdout and stderr captured separately and returned
// as stdout+stderr concatenated (Python's `(proc.stdout or "") +
// (proc.stderr or "")`). A process that cannot be spawned is a fatal error:
// Python dies there with an uncaught traceback (exit 1, no report).
// reentryEnv is the environment of a Go->launcher re-entry: AI_SPECS_HOME pinned
// to the CLI home, and AI_SPECS_GO_BIN pinned to the running binary (self) so the
// launcher execs this build instead of looking for dist/ (GO-20 U2e T2).
func reentryEnv(home, self string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AI_SPECS_HOME=") || strings.HasPrefix(kv, "AI_SPECS_GO_BIN=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "AI_SPECS_HOME="+home)
	if self != "" {
		env = append(env, "AI_SPECS_GO_BIN="+self)
	}
	return env
}

func configureFrontDoor(home, root string, argv ...string) (int, string, error) {
	cmd := exec.Command(filepath.Join(home, "bin", "ai-specs"), argv...)
	cmd.Dir = root
	cmd.Env = reentryEnv(home, selfPath())
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	combined := outBuf.String() + errBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), combined, nil
		}
		return 0, "", err
	}
	return 0, combined, nil
}

// configureSummaryRE is recipe-configure.py's SUMMARY_RE.
var configureSummaryRE = regexp.MustCompile(`(?i)Summary:\s*(\d+)\s+OK,\s*(\d+)\s+INFO,\s*(\d+)\s+WARN,\s*(\d+)\s+ERROR`)

// configureFailedStepRE is recipe-configure.py's _failed_step pattern; the
// LAST `syncing …` match names the step that stopped sync.
var configureFailedStepRE = regexp.MustCompile(`(?i)syncing\s+([^\n\r]+)`)

// configureVerifyNode is parse_doctor_summary + the doctor_exit stamp, in the
// Python dict's key order.
func configureVerifyNode(output string, doctorExit int) rcJSON {
	node := rcNewObject()
	node.set("doctor_exit", rcInt(doctorExit))
	matches := configureSummaryRE.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		node.set("parsed", rcBool(false))
		node.set("ok", rcNull())
		node.set("info", rcNull())
		node.set("warn", rcNull())
		node.set("error", rcNull())
		return node
	}
	last := matches[len(matches)-1]
	node.set("parsed", rcBool(true))
	for i, key := range []string{"ok", "info", "warn", "error"} {
		node.set(key, rcNumber(last[i+1]))
	}
	return node
}

// configureFailedStep is _failed_step over the captured sync output.
func configureFailedStep(output string) rcJSON {
	matches := configureFailedStepRE.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return rcNull()
	}
	return rcString(strings.TrimSpace(matches[len(matches)-1][1]))
}

// configureAssumptions is what apply_project's
// `report["assumptions"] = inspect_project(...)["assumptions"]` reads: the
// manifest re-read, the schema re-read and the topology grounding — the same
// helpers inspect runs, so the two can never disagree (S6).
func configureAssumptions(root, recipeID, home string) ([]string, error) {
	manifest, err := configureLoadManifest(root)
	if err != nil {
		return nil, err
	}
	recipe, err := configureSchemaFor(home, recipeID)
	if err != nil {
		return nil, err
	}
	_, assumptions := configureGrounding(root, recipe, manifest)
	return assumptions, nil
}

// configureApply is apply_project: validation → preflight (blocked exit 4) →
// the changed/unchanged/preserved computation → the dry-run/no-op exits →
// both writes (project-owned keys first, then the recipe config block) → the
// --sync leg. The returned report is emitted by the caller; the third result
// is the fatal front-door/spawn error Python would die on (no report).
func configureApply(root, recipeID, home string, values []config.ConfigValue, sync, ignore, dryRun bool) (rcJSON, int, error) {
	manifest, merr := configureLoadManifest(root)
	recipe, rerr := configureSchemaFor(home, recipeID)
	if merr != nil {
		return configureRejectedReport(recipeID, merr.Error(), ignore), 3, nil
	}
	if rerr != nil {
		return configureRejectedReport(recipeID, rerr.Error(), ignore), 3, nil
	}
	if verr := config.ValidateConfigValues(recipe, values); verr != nil {
		return configureRejectedReport(recipeID, verr.Error(), ignore), 3, nil
	}

	pf := configureBuildPreflight(root, manifest, home)
	blocked := rcNull()
	if !pf.policyOK {
		blocked = rcString(pf.blockedReason)
	}
	report := configureBaseReport(recipeID, pf.configureCLIVersionNode(), blocked, true, ignore)
	if !pf.policyOK && !ignore {
		report.set("status", rcString("blocked"))
		return report, 4, nil
	}
	if pf.lockState == "stale" {
		report.set("gaps", rcNewArray(rcString(fmt.Sprintf(
			"lock cli_version %s != installed %s (next sync restamps)",
			pf.lockCLI, pf.installed))))
	}

	_, _, current := config.RecipeConfigState(manifest, recipeID)
	var project *toml.Table
	if raw, ok := manifest.Get("project"); ok {
		if tbl, isTable := raw.(*toml.Table); isTable {
			project = tbl
		}
	}
	getProject := func(key string) any {
		if project == nil {
			return nil
		}
		v, _ := project.Get(key)
		return v
	}
	getCurrent := func(key string) (any, bool) {
		if current == nil {
			return nil, false
		}
		v, ok := current.Get(key)
		return v, ok
	}

	sortedKeys := make([]string, 0, len(values))
	valueOf := map[string]any{}
	for _, cv := range values {
		sortedKeys = append(sortedKeys, cv.Key)
		valueOf[cv.Key] = cv.Value
	}
	sort.Strings(sortedKeys)

	changed := rcNewArray()
	unchanged := []rcJSON{}
	for _, key := range sortedKeys {
		value := valueOf[key]
		var from any
		equal := false
		switch {
		case recipeConfigureProjectOwnedKeys[key]:
			from = getProject(key)
			equal = config.EqualPythonValue(from, value)
		case strings.Contains(key, "."):
			var ok bool
			from, ok = config.LookupNestedConfig(current, key)
			equal = ok && config.EqualPythonValue(from, value)
		default:
			var ok bool
			from, ok = getCurrent(key)
			equal = ok && config.EqualPythonValue(from, value)
		}
		if equal {
			unchanged = append(unchanged, rcString(key))
			continue
		}
		fromNode, err := rcFromTOML(from)
		if err != nil {
			return rcNull(), 0, err
		}
		toNode, err := rcFromTOML(value)
		if err != nil {
			return rcNull(), 0, err
		}
		entry := rcNewObject()
		entry.set("key", rcString(key))
		entry.set("from", fromNode)
		entry.set("to", toNode)
		changed.arr = append(changed.arr, entry)
	}

	var recipeValues []config.ConfigValue
	touched := map[string]bool{}
	for _, cv := range values {
		if recipeConfigureProjectOwnedKeys[cv.Key] {
			continue
		}
		recipeValues = append(recipeValues, cv)
		rootKey := strings.Split(cv.Key, ".")[0]
		touched[rootKey] = true
	}
	preserved := []rcJSON{}
	if current != nil {
		keys := append([]string(nil), current.Keys()...)
		sort.Strings(keys)
		for _, k := range keys {
			if !touched[k] {
				preserved = append(preserved, rcString(k))
			}
		}
	}
	applied := report.get("applied")
	applied.set("changed", changed)
	applied.set("unchanged", rcNewArray(unchanged...))
	applied.set("preserved", rcNewArray(preserved...))

	assumptions, aerr := configureAssumptions(root, recipeID, home)
	if aerr != nil {
		// apply_project's assumptions line is outside its guarded block: the
		// error reaches main()'s except and rebuilds the rejected report with
		// the EMPTY preflight.
		return configureRejectedReport(recipeID, aerr.Error(), ignore), 3, nil
	}
	assumptionNodes := make([]rcJSON, 0, len(assumptions))
	for _, a := range assumptions {
		assumptionNodes = append(assumptionNodes, rcString(a))
	}
	report.set("assumptions", rcNewArray(assumptionNodes...))

	if len(changed.arr) == 0 || dryRun {
		if len(changed.arr) == 0 {
			report.set("status", rcString("no-op"))
		} else {
			report.set("status", rcString("ok"))
		}
		report.set("dry_run", rcBool(dryRun))
		return report, 0, nil
	}

	// Write phase: project-owned keys first (sorted), then the recipe block.
	manifestPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	for _, key := range sortedKeys {
		if !recipeConfigureProjectOwnedKeys[key] {
			continue
		}
		if config.EqualPythonValue(getProject(key), valueOf[key]) {
			continue
		}
		if werr := config.WriteProjectKey(manifestPath, key, valueOf[key]); werr != nil {
			report.set("status", rcString("failed"))
			report.set("reason", rcString(werr.Error()))
			return report, 1, nil
		}
	}
	_, refusal, werr := config.UpdateRecipeConfig(manifestPath, recipeID, recipeValues)
	if werr != nil {
		report.set("status", rcString("failed"))
		report.set("reason", rcString(werr.Error()))
		return report, 1, nil
	}
	if refusal != "" {
		report.set("status", rcString("failed"))
		report.set("reason", rcString(refusal))
		return report, 1, nil
	}

	if !sync {
		report.set("status", rcString("ok"))
		return report, 0, nil
	}

	// The --sync leg: the same front-door command, env and parse seams the
	// script uses (see the section header for the decision).
	syncArgs := []string{"sync", root}
	if ignore {
		syncArgs = append(syncArgs, "--ignore-cli-version")
	}
	syncExit, syncOutput, ferr := configureFrontDoor(home, root, syncArgs...)
	if ferr != nil {
		return rcNull(), 0, ferr
	}
	syncNode := report.get("sync")
	syncNode.set("ran", rcBool(true))
	syncNode.set("exit_code", rcInt(syncExit))
	if syncExit != 0 {
		syncNode.set("failed_step", configureFailedStep(syncOutput))
		report.set("status", rcString("partial"))
		report.set("reason", rcString(
			"sync failed; previous writes were not rolled back and lock CLI version was not stamped"))
		return report, 1, nil
	}
	syncNode.set("lock_stamped", rcBool(true))

	doctorExit, doctorOutput, derr := configureFrontDoor(home, root, "doctor", root)
	if derr != nil {
		return rcNull(), 0, derr
	}
	report.set("verify", configureVerifyNode(doctorOutput, doctorExit))
	if doctorExit != 0 {
		report.set("status", rcString("failed"))
		report.set("reason", rcString("doctor verification failed"))
		return report, 1, nil
	}
	report.set("status", rcString("ok"))
	return report, 0, nil
}

// recipeConfigureResolve is Path.resolve(): absolute, symlinks resolved as
// far as the path exists (Python's realpath semantics — the tail after a
// missing component is appended unresolved), which is what puts the
// macOS `/private/var/...` spelling into `manifest not found:`.
func recipeConfigureResolve(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	cur := abs
	var missing []string
	for {
		resolved, rerr := filepath.EvalSymlinks(cur)
		if rerr == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		if !os.IsNotExist(rerr) {
			return abs
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		missing = append([]string{filepath.Base(cur)}, missing...)
		cur = parent
	}
}

// configureParseAssignments is `dict(_parse_assignment(recipe, item) for
// item in args.assignments)`: parsed in order (a failure aborts before any
// report is built), deduplicated Python-dict style — the LAST value at the
// FIRST occurrence's position.
func configureParseAssignments(recipe *schema.Recipe, assignments []string) ([]config.ConfigValue, error) {
	var order []string
	byKey := map[string]any{}
	for _, assignment := range assignments {
		cv, err := config.ParseConfigAssignment(recipe, assignment)
		if err != nil {
			return nil, err
		}
		if _, seen := byKey[cv.Key]; !seen {
			order = append(order, cv.Key)
		}
		byKey[cv.Key] = cv.Value
	}
	values := make([]config.ConfigValue, 0, len(order))
	for _, key := range order {
		values = append(values, config.ConfigValue{Key: key, Value: byKey[key]})
	}
	return values, nil
}

// emitConfigureReport is recipe-configure.py's _emit: one line of JSON on
// stdout — indent=2 by default, the compact separators under --json — and the
// trailing newline print() adds.
func emitConfigureReport(w io.Writer, doc rcJSON, compact bool) error {
	var sb strings.Builder
	writeJSON(&sb, doc, 0, compact)
	sb.WriteByte('\n')
	_, err := io.WriteString(w, sb.String())
	return err
}

// runRecipeConfigure is the native `recipe configure` entry —
// recipe-configure.py's main(): the argparse surface first (usage errors exit
// 2 before anything touches the filesystem), then Path.resolve, then the two
// parser.error() arm combinations, then _schema_for (every failure is the
// rejected report, exit 3), then --inspect or the apply/report path.
func runRecipeConfigure(args []string, home string, stdout, stderr io.Writer) int {
	columns := recipeConfigureColumns()
	parsed := parseRecipeConfigure(args)
	switch parsed.kind {
	case recipeConfigureParseHelp:
		fmt.Fprint(stdout, recipeConfigureHelpText(columns))
		return 0
	case recipeConfigureParseError:
		fmt.Fprint(stderr, recipeConfigureErrorText(columns, parsed.errMsg))
		return 2
	}
	opts := parsed.opts

	// parser.error() arms after parse_args + Path(args.path).resolve().
	if opts.inspect && len(opts.assignments) > 0 {
		fmt.Fprint(stderr, recipeConfigureErrorText(columns, "--inspect cannot be combined with --set"))
		return 2
	}
	if !opts.inspect && len(opts.assignments) == 0 {
		fmt.Fprint(stderr, recipeConfigureErrorText(columns, "one of --inspect or --set is required"))
		return 2
	}
	pathArg := "."
	if opts.hasPath {
		pathArg = opts.path
	}
	root := recipeConfigureResolve(pathArg)

	emit := func(doc rcJSON, code int) int {
		if doc.kind == rcJSONNull {
			return code
		}
		if err := emitConfigureReport(stdout, doc, opts.asJSON); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return code
	}
	rejected := func(reason string) int {
		return emit(configureRejectedReport(opts.recipeID, reason, opts.ignoreCLIVersion), 3)
	}

	// JD-R1-path-traversal-parity (U6d-1): the Python joins the id verbatim, so an
	// id carrying `..` or an absolute path resolved the recipe read OUTSIDE the
	// catalog root and `--inspect` then reported an arbitrary directory's
	// recipe.toml. skills remove already refuses an invalid id with the same
	// guard; this verb does too, and the refusal is a deliberate fail-closed
	// divergence from the script (S5's guard family), pinned by a test.
	if sanitized, bad := invalidDepID(opts.recipeID); bad {
		fmt.Fprintf(stderr, "ERROR: refusing invalid recipe id: '%s'\n", sanitized)
		return 2
	}

	recipe, err := configureSchemaFor(home, opts.recipeID)
	if err != nil {
		return rejected(err.Error())
	}
	if opts.inspect {
		doc, ierr := configureInspect(root, opts.recipeID, home)
		if ierr != nil {
			return rejected(ierr.Error())
		}
		return emit(doc, 0)
	}
	values, verr := configureParseAssignments(recipe, opts.assignments)
	if verr != nil {
		return rejected(verr.Error())
	}
	report, code, fatal := configureApply(root, opts.recipeID, home, values,
		opts.sync, opts.ignoreCLIVersion, opts.dryRun)
	if fatal != nil {
		fmt.Fprintln(stderr, fatal)
		return 1
	}
	return emit(report, code)
}

// ── GO-20 U6e (T5): `configure-recipes` and the recipe dispatcher ──
//
// Native `ai-specs configure-recipes` — the port of lib/recipe-config.sh (49
// lines, the wrapper's argument contract) plus lib/_internal/config_wizard.py's
// main (the verb: the manifest guard, the TTY gate, the enabled-recipe
// enumeration, the whole-project loop and the env offer). And the recipe
// DISPATCHER: lib/recipe.sh's usage/bare/unknown arms natively (S4), and
// since U7-2 `init` is native too — internal/recipe owns the brief and the
// wrapper grammar, so recipe.sh and recipe-init.sh both retire from the
// binary (S9).
//
// THE CLI-DEP GATE (U4b-2's S37 seam, goal 4): NATIVE since U7-1 T3.
// internal/deps.Gate owns `_dep_gate` plus dep_install's OFFER half (the
// brew/apt plans, the consent prompt, the recheck, the provider install),
// and depgate.go's recipeDepGate adapter wires it to all THREE callers
// (init's wizard, this loop, recipe add's tail) — S37's python3 driver and
// its initwizard.go entry point are gone. The gate is therefore not removed
// from any route: every path Python gated reaches it, in-process.
//
// NO ensure_deps (L9 deviation 1's class): nothing before the first gated
// recipe needs Python in this port — the wizard is internal/tui, the env
// offer is internal/sync, and the gate is internal/deps. config_wizard.main's
// ensure_deps would exit 3 silently when rich/questionary are missing; this
// port had no such precondition to inherit, and S37's rc-3 answer (the
// driver's own vendor bootstrap) died with the driver: the native gate never
// refuses a run over Python packages, so this loop has no rc-3 arm either.
//
// Documented deviations (the S12/ADR 0002 class, pinned by tests): a `cd`
// failure keeps exit 1 and the `cd: <path>: <reason>` shape but drops bash's
// own `lib/recipe-config.sh: line N: ` prefix
// (TestConfigureRecipesMissingPathDeviation); a manifest that does not parse
// exits 1 on our own error line where Python printed a traceback; a wizard
// prompt that cannot open skips that recipe (L9's deviation class) where
// Python would traceback uncaught; rich's TTY colouring of the skip lines is
// not reproduced (plain bytes, the init port's precedent); and `recipe add`'s
// env offer prompts the project-wide variable set where recipe-add.py scoped
// collection to the added id (internal/sync owns the scoping and is outside
// this slice's edit edges). The TTY surface is pinned in-process through the
// seams; only the wrapper, guard and gate bytes are differentially
// byte-compared (no PTY in the table, L4's constraint unchanged).

// configureRecipesUsage is lib/recipe-config.sh's usage() heredoc byte for
// byte: a QUOTED heredoc (`cat <<'EOF'`), so the text is literal and the
// trailing newline after the last line is part of the output.
const configureRecipesUsage = `Usage: ai-specs configure-recipes [path] [--help]
Configure enabled recipes (config fields, CLI deps, ai-specs.env + root .envrc).
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// recipeUsage is lib/recipe.sh's usage() heredoc byte for byte (stdout for
// --help/-h/help, stderr after the bare-verb exit 2).
const recipeUsage = `Usage: ai-specs recipe <subcommand> [args]
Subcommands:
  list [path]      List available and installed recipes
  add <id> [path]  Add a recipe to the manifest
  init <id> [path]   Print a read-only initialization brief
  configure <id> [path] [options]  Inspect or apply recipe config non-interactively
  remove <id> [path]  Remove a recipe from the manifest
Path defaults to current directory.
`

// runRecipeInitVerb is the native `recipe init` (GO-20 U7-2): the wrapper
// grammar and the brief both live in internal/recipe, so this seam only
// supplies the two catalogue anchors — AI_SPECS_HOME and the CLI install root
// that backs S9's fallback.
func runRecipeInitVerb(args []string, home, exePath string, stdout, stderr io.Writer) int {
	installRoot := home
	if exePath != "" {
		installRoot = homepkg.ResolveHome(exePath, "")
	}
	return recipe.RunInit(args, recipe.InitOptions{Home: home, InstallRoot: installRoot}, stdout, stderr)
}

// runRecipeDispatch is lib/recipe.sh's case statement REMAINDER (S4's three
// dispatcher failure arms): `--help|-h|help` prints usage on stdout exit 0
// and ignores the rest of the argv, the bare `recipe` prints the same usage
// on stderr exit 2, and anything else is the two unknown lines on stderr exit
// 1 — `subcmd="${1:-}"` then the `*` arm, argv[0] being the subcommand.
func runRecipeDispatch(args []string, stdout, stderr io.Writer) int {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, recipeUsage)
		return 0
	case "":
		fmt.Fprint(stderr, recipeUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs recipe: unknown subcommand '%s'\n", sub)
		fmt.Fprintln(stderr, "Run 'ai-specs recipe --help' for usage.")
		return 1
	}
}

// recipeTailSeams is U6e's injected TTY tail, shared by `recipe add`'s leg
// and the configure-recipes loop (the runInit pattern): production wires the
// three handlers below, a test scripts each one. The dep gate and the env
// offer are what U6b's narrowed seam still delegated WHOLE to recipe.sh;
// wiring both here is what lets the shim map go empty (L10).
type recipeTailSeams struct {
	// openWizard yields the config-wizard prompts over the streams
	// (production openRecipeWizardPrompts); `recipe add`'s confirm uses the
	// same opener, opening and closing per prompt like questionary does.
	openWizard recipeAddWizardOpener
	// depGate runs the CLI-dep gate for one already-read recipe (production:
	// recipeDepGate over internal/deps.Gate — the native gate, U7-1 T3). The
	// recipe travels by value because the caller has already parsed it; the
	// old seam took an id only because it was a separate process that had to
	// re-read the catalog.
	depGate func(recipe *schema.Recipe) wizardDepGateOutcome
	// envOffer runs the harness env offer against the project root, returning
	// the error its caller words (production: openInitWizardKeys +
	// sync.OfferHarnessEnv — which soft-fails internally, so the error only
	// surfaces when the terminal cannot be taken over or an exception
	// escapes: Python's `_offer_envrc` outer `except Exception`).
	// recipeIDs is the offer's scope (U7-3, S5): `recipe add` passes the added
	// id, the configure-recipes loop passes nil for the aggregate set — the
	// same split as recipe-add.py:273 vs config_wizard.py:264.
	envOffer func(root string, recipeIDs []string) error
}

// productionRecipeTailSeams wires the tail for the running CLI over the
// dispatcher's own streams.
func productionRecipeTailSeams(home string, stdin io.Reader, stdout, stderr io.Writer) recipeTailSeams {
	return recipeTailSeams{
		openWizard: openRecipeWizardPrompts,
		depGate: func(loaded *schema.Recipe) wizardDepGateOutcome {
			// No raw-mode window is open at either call site (`recipe add`
			// closes the confirm's first, this loop opens per recipe), so
			// recipeDepGate opens ONE for the gate's own questions and closes
			// it before returning — questionary's per-prompt takeover.
			return recipeDepGate(home, stdin, stdout, stderr, loaded)
		},
		envOffer: func(root string, recipeIDs []string) (err error) {
			defer func() {
				if r := recover(); r != nil {
					// Python's `except Exception` around offer_harness_env.
					err = fmt.Errorf("%v", r)
				}
			}()
			keys, closeKeys, rc := openInitWizardKeys(stdin, stdout)
			if rc != 0 {
				return errors.New("the interactive terminal is unavailable")
			}
			defer closeKeys()
			sync.OfferHarnessEnv(root, home, recipeIDs, &wizardEnvPrompts{out: stdout, keys: keys}, stdout, stderr)
			return nil
		},
	}
}

// runConfigureRecipes is the native `configure-recipes` entry:
// recipe-config.sh's argument loop first (usage errors exit 2 before anything
// touches the filesystem), then config_wizard.main itself.
func runConfigureRecipes(args []string, home string, stdin io.Reader, stdout, stderr io.Writer, ttyNow bool) int {
	return runConfigureRecipesFlow(args, home, stdin, stdout, stderr, ttyNow,
		productionRecipeTailSeams(home, stdin, stdout, stderr))
}

// runConfigureRecipesFlow is runConfigureRecipes with the TTY tail's seams
// injected (and ttyNow, which the kernel detects in production — an
// in-process test cannot fake a TTY) — the interactive loop is pinned here,
// never through a PTY.
func runConfigureRecipesFlow(args []string, home string, stdin io.Reader, stdout, stderr io.Writer, ttyNow bool, seams recipeTailSeams) int {
	// recipe-config.sh's while/case loop, in order: --help|-h wins wherever
	// it appears, `--` makes every remaining argument positional (never a
	// flag), an unknown flag prints two stderr lines, and a second positional
	// is the other exit-2 usage error. An empty-string positional keeps
	// TARGET_PATH empty (`[[ -z ]]`), falling into the cwd default exactly
	// like the script's take_positional.
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, configureRecipesUsage)
			return 0
		case arg == "--":
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
			fmt.Fprintln(stderr, "Run 'ai-specs configure-recipes --help' for usage.")
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
	// config_wizard.main(): `Path(args.path).resolve()` — the physical
	// spelling after the wrapper's cd/pwd (U6d-1's recipeConfigureResolve is
	// Path.resolve() with a tolerated missing tail).
	root := recipeConfigureResolve(resolved)
	manifestPath := filepath.Join(root, "ai-specs", "ai-specs.toml")

	// main's FIRST check — before ensure_deps, the TTY gate and the recipe
	// enumeration (its frozen message, exit 1).
	if !regularInitFile(manifestPath) {
		fmt.Fprintf(stderr, "Project not initialized: missing %s\n", manifestPath)
		return 1
	}
	if !ttyNow {
		// main's TTY gate, its frozen message on stderr, exit 3 (config_wizard
		// runs ensure_deps before it; this port has no equivalent need — the
		// header).
		fmt.Fprintln(stderr, "configure-recipes requires an interactive TTY")
		return 3
	}
	// One manifest read standing in for Python's TWO (main's unguarded
	// _enabled_recipe_ids, then configure_selected_recipes's try/except →
	// {}): nothing writes between them, so one read feeds both identically.
	// The unguarded load's failure is Python's traceback → exit 1; the exit
	// code is kept, the wording is ours (ADR 0002).
	data, lerr := config.LoadManifest(manifestPath)
	if lerr != nil {
		fmt.Fprintf(stderr, "ERROR: configure-recipes failed: %v\n", lerr)
		return 1
	}
	ids := configureRecipesEnabledIDs(data)
	if len(ids) == 0 {
		// `_enabled_recipe_ids` empty → the frozen line on stdout (rich
		// Console() default), exit 0, before the loop and the env offer.
		fmt.Fprintln(stdout, "No enabled recipes found in the manifest.")
		return 0
	}
	// configure_selected_recipes' existing_by_id — a PRE-loop snapshot (the
	// loop's own writes never feed back into it, exactly like Python's dict).
	existingByID := map[string]map[string]any{}
	for _, rid := range ids {
		existingByID[rid] = wizardExistingConfig(data, rid)
	}

	// The loop, in _enabled_recipe_ids' manifest order (the Python iterates
	// read_recipes' insertion order — the prompt order is the contract).
	for _, rid := range ids {
		// read_recipe(catalog, rid): the Path join verbatim (an absolute id
		// replaces the catalogue root, interior `..` kept) and the dir check
		// before the parse — configureRecipeDir is U6d-1's shared helper for
		// exactly this join. The catalogue is ai_specs_home()/catalog/recipes,
		// the dispatcher's own home — this route has NO module-path fallback
		// (recipe-add._resolve_catalog_dir's is_dir branch is a different
		// function; Python's configure_selected_recipes does not use it).
		recipeDir := configureRecipeDir(home, rid)
		if info, serr := os.Stat(recipeDir); serr != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "skip %s: recipe directory not found: %s\n", rid, recipeDir)
			continue
		}
		loaded, rerr := schema.LoadRecipeToml(recipeDir + "/recipe.toml")
		if rerr != nil {
			// `console.print(f"[yellow]skip {rid}: {exc}[/yellow]")` with the
			// markup stripped — plain bytes, the init port's precedent.
			fmt.Fprintf(stderr, "skip %s: %v\n", rid, rerr)
			continue
		}
		// The gate BEFORE the fields check — configure_selected_recipes'
		// order, even for a recipe that declares no config at all.
		if len(loaded.CliDeps) > 0 {
			outcome := seams.depGate(loaded)
			switch {
			case outcome.Err != nil:
				// configure_selected_recipes is NOT wrapped in main (Python:
				// traceback, exit 1) — our own line, same code.
				fmt.Fprintf(stderr, "ERROR: configure-recipes failed: %v\n", outcome.Err)
				return 1
			case !outcome.Proceed:
				fmt.Fprintf(stderr, "Skipped %s (missing CLI deps)\n", rid)
				continue
			}
		}
		if loaded.ConfigSchema == nil || len(loaded.ConfigSchema.Fields) == 0 {
			continue
		}
		prompts, closeKeys, rc := seams.openWizard(stdin, stdout)
		if rc != 0 || prompts == nil || closeKeys == nil {
			// Recorded L9-class deviation: questionary raising here would take
			// main down with a traceback (exit 1); the port skips the recipe
			// instead — never exercised on a real terminal, since the loop is
			// behind the gate.
			continue
		}
		values, wok, werr := runConfigWizard(rid, loaded.ConfigSchema, existingByID[rid], prompts)
		closeKeys()
		if werr != nil {
			// Python's re.error escaping run_config_wizard → uncaught → exit 1.
			fmt.Fprintf(stderr, "ERROR: configure-recipes failed: %v\n", werr)
			return 1
		}
		if !wok || len(values) == 0 {
			continue // Python's `if not values: continue`
		}
		// The write through the ONE writer (S6): update_recipe_config's
		// refusal detail travels verbatim, exit 1 like the reference's
		// RecipeConfigWriteError/traceback.
		if _, refusal, werr := config.UpdateRecipeConfig(manifestPath, rid, sortedWizardConfigValues(values)); werr != nil {
			fmt.Fprintf(stderr, "ERROR: configure-recipes failed: %v\n", werr)
			return 1
		} else if refusal != "" {
			fmt.Fprintf(stderr, "ERROR: configure-recipes failed: %s\n", refusal)
			return 1
		}
	}

	// _offer_envrc, soft-failing like Python's `except Exception`: the loop's
	// writes stand, the yellow line (rich Console() → stdout) prints, exit 0.
	// config_wizard.py:264 passes no recipe_ids, so this offer stays aggregate.
	if serr := seams.envOffer(root, nil); serr != nil {
		fmt.Fprintf(stdout, "Harness env offer skipped: %v\n", serr)
	}
	return 0
}

// configureRecipesEnabledIDs is config_wizard._enabled_recipe_ids over the
// already-loaded manifest: [recipes.*] TABLES in file order (read_recipes
// preserves insertion order — the loop's prompt order), kept only when
// `enabled` is a real boolean true. The predicate is the one internal/recipe's
// enabledRecipes already grades for the list route (bool-only: `enabled =
// "true"` stays disabled); this walks the table instead of collecting a map
// because the ORDER is the contract here.
func configureRecipesEnabledIDs(manifest *toml.Table) []string {
	recipes, ok := manifest.Table("recipes")
	if !ok || recipes == nil {
		return nil
	}
	var out []string
	for _, id := range recipes.Keys() {
		table, ok := recipes.Table(id)
		if !ok || table == nil {
			continue // read_recipes skips non-dict values
		}
		if on, _ := table.Bool("enabled"); on {
			out = append(out, id)
		}
	}
	return out
}

// depInstallPackageMap is dep_install._PACKAGE_MAP: binary → (brew formula,
// apt package). Empty strings mean guidance-only for that side.
var depInstallPackageMap = map[string][2]string{
	"gh":     {"gh", "gh"},
	"glab":   {"glab", "glab"},
	"jq":     {"jq", "jq"},
	"direnv": {"direnv", "direnv"},
	"git":    {"git", "git"},
	"bb":     {"bb-cli", ""},
}

// depGuidanceOnly is dep_install._GUIDANCE_ONLY — always guidance, never a
// blind Node install.
var depGuidanceOnly = map[string]bool{"npx": true}

// depGuidanceDisplay is dep_install.resolve_install_plan's GUIDANCE half for
// `_print_cli_dep_guidance`'s one line: guidance-only or unknown binaries
// fall to the install_url (or `install '<binary>' manually`), a known formula
// renders `brew install <formula>` when brew is on PATH (Darwin/Linux) or
// `sudo apt-get install -y <pkg>` on Linux when apt-get is. This is the
// GUIDANCE half only: since U7-1 T3 the OFFER half (command execution, the
// GitHub-release install) lives in internal/deps, which owns it natively.
func depGuidanceDisplay(binary, installURL string) string {
	if depGuidanceOnly[binary] {
		if installURL != "" {
			return installURL
		}
		return "install '" + binary + "' manually"
	}
	formula, known := depInstallPackageMap[binary]
	if !known {
		if installURL != "" {
			return installURL
		}
		return "install '" + binary + "' manually"
	}
	// platform.system() → runtime.GOOS; shutil.which → exec.LookPath.
	if formula[0] != "" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		if _, err := exec.LookPath("brew"); err == nil {
			return "brew install " + formula[0]
		}
	}
	if formula[1] != "" && runtime.GOOS == "linux" {
		if _, err := exec.LookPath("apt-get"); err == nil {
			return "sudo apt-get install -y " + formula[1]
		}
	}
	if installURL != "" {
		return installURL
	}
	return "install '" + binary + "' manually"
}

// recipePrintCLIDepGuidance is recipe-add.py:68-99
// (_print_cli_dep_guidance): a blank line, one `  - <binary>: <target>` line
// per REQUIRED dep (dep_install's display through depGuidanceDisplay above),
// then the two frozen lines. Python's dep_install import failure falls back
// to the raw install_url — in-process this port is the load's constant
// success side, so only the display branch is reachable.
func recipePrintCLIDepGuidance(r *schema.Recipe, stdout io.Writer) {
	fmt.Fprintln(stdout)
	for _, dep := range r.CliDeps {
		if !dep.Required {
			continue
		}
		target := depGuidanceDisplay(dep.Binary, dep.InstallURL)
		if target != "" {
			fmt.Fprintf(stdout, "  - %s: %s\n", dep.Binary, target)
			continue
		}
		fmt.Fprintf(stdout, "  - %s\n", dep.Binary)
	}
	fmt.Fprintln(stdout, "required CLI dependencies are still missing; install them and re-run, or continue with:")
	fmt.Fprintln(stdout, "  ai-specs configure-recipes")
}
