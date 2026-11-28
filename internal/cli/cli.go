// Package cli implements the dispatcher contract of bin/ai-specs: 14 verbs,
// where `version` and `help` are native and every other verb is a passthrough
// shim that execs the legacy lib/*.sh implementation (strangler pattern).
//
// Run is the in-process entry point (the catalog gate module's run(args,
// stdin, stdout, stderr) int pattern) so routing and exit codes are unit
// testable without spawning the binary.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/hub"
	"ai-specs.dev/ai-specs/internal/sync"
	"ai-specs.dev/ai-specs/internal/tty"
	"ai-specs.dev/ai-specs/internal/upgrade"
	"ai-specs.dev/worktree-gate/runner"
)

// routeKind classifies a dispatcher decision.
type routeKind int

const (
	routeVersion           routeKind = iota // native: print <home>/VERSION
	routeHelp                               // native: print embedded help.txt
	routeRefreshBundled                     // native compatibility stub (card [Go 06]): reports embedded provenance
	routeSync                               // native sync spine (card [Go 07.S1])
	routeSyncAgent                          // native sync-agent fan-out (card [Go 07.S15])
	routeDoctor                             // native: `doctor` health diagnostic (card [Go 08])
	routeRulesAudit                         // native: `rules-audit` legacy rules inventory (card [Go 08])
	routeBriefRenderPolicy                  // native internal seam: lib/init.sh's policy gate (S19b)
	routeBaselineBrief                      // native internal seam: lib/init.sh's baseline render (S19b)
	routeUpgrade                            // native: `upgrade` legacy install upgrade (card [Go 12])
	routeHub                                // native: `hub` front door, non-interactive half (GO-20 U3a)
	routeInit                               // native: `init` classic bootstrap; the wizard delegates to the shim (GO-20 U4a)
	routeSkillsList                         // native: `skills list`, port of lib/skills-list.sh (GO-20 U5a)
	routeSkillsRemove                       // native: `skills remove`, port of lib/skills-remove.sh (GO-20 U5b)
	routeSkillsAdd                          // native: `skills add`, port of lib/skills-add.sh (GO-20 U5c)
	routeSkillsValidate                     // native: `skills validate`, port of skill_contract.py sync-metadata (GO-20 U2e U-D)
	routeSkillsField                        // native: `skills field`, the JSON field reader skill-sync's sync.sh uses (GO-20 U2e T5)
	routeAddDep                             // native: `add-dep` — the same verb under bin/ai-specs's alias label (GO-20 U5c)
	routeSkills                             // native: the lib/skills.sh dispatcher remainder (GO-20 U5c)
	routeRecipeList                         // native: `recipe list`, port of lib/recipe-list.sh (GO-20 U6a)
	routeRecipeAdd                          // native: `recipe add`, port of lib/recipe-add.sh (GO-20 U6b)
	routeRecipeRemove                       // native: `recipe remove`, port of lib/recipe-remove.sh (GO-20 U6c)
	routeRecipeConfigure                    // native: `recipe configure`, port of lib/_internal/recipe-configure.py (GO-20 U6d-1)
	routeConfigureRecipes                   // native: `configure-recipes`, port of lib/recipe-config.sh + config_wizard.py (GO-20 U6e)
	routeRecipe                             // native: lib/recipe.sh's dispatcher remainder — usage/bare/unknown (GO-20 U6e)
	routeRecipeInitVerb                     // native: `recipe init`, port of lib/recipe-init.sh (GO-20 U7-2)
	routeSpecs                              // native: the `specs` family remainder — usage/help/unknown (GO-20 U2e U-A)
	routeSpecsPromote                       // native: `specs promote`, port of lib/_internal/spec_promotion.py (GO-20 U2e U-A)
	routeChanges                            // native: the `changes` family remainder — usage/help/unknown (I5/P4)
	routeChangesArchive                     // native: `changes archive`, the mechanical pre-merge archive (I5/P4)
	routeChangesStart                       // native: `changes start --none`, the local no-plan declaration (T14/S19)
	routeChangesWorkstream                  // native: `changes workstream`, the report and the S16 decision (change-ledger-v2 T3)
	routeChangesStatus                      // native: `changes status`, the read-only composite (change-ledger-v2 T7)
	routeGates                              // native: the `gates` family remainder — usage/help/unknown (GO-20 U2e U-B)
	routeGatesPremerge                      // native: `gates premerge`, port of lib/_internal/premerge_guardian.py (GO-20 U2e U-B)
	routeTracker                            // native: the `tracker` family remainder — usage/help/unknown (GO-20 U2e U-C)
	routeTrackerSlug                        // native: `tracker slug`, port of ledger_bridge.change_slug (GO-20 U2e U-C)
	routeTrackerEvidence                    // native: `tracker evidence`, port of ledger_bridge.evidence_payload (GO-20 U2e U-C)
	routeTrackerField                       // native: `tracker field`, JSON value reader for the hook helpers (GO-20 U2e U-F)
	routeTrackerClassify                    // native: `tracker classify`, the hook event classifier (GO-20 U2e U-F2b)
	routeHook                               // native: the `hook` family remainder — usage/help/unknown (GO-20 U2e U-F2a)
	routeHookField                          // native: `hook field`, JSON value reader for the gate hosts (GO-20 U2e U-F2a)
	routeHookNormalizePath                  // native: `hook normalize-path`, canonical hook target (GO-20 U2e U-F2a)
	routeWorktreeGate                       // direct passthrough to the importable worktree-gate runner
	routeWorktreeCleanup                    // native: `worktree-cleanup`, the CLI-served post-merge cleanup (T3)
	routeMCPVaultFS                         // native: `mcp vault-fs`, the CLI-served vault filesystem MCP server (T4)
	routeTrackerCardGate                    // native: `tracker-card-gate`, the CLI-served Tracker card gate (T6)
	routePlanBuildGate                      // native: `plan-build-gate`, the CLI-served planning gate (T7)
	routeUnknown                            // unknown command: usage error, exit 2
)

// route is one dispatcher decision.
type route struct {
	kind routeKind
	// script is always empty: the verb→script map is gone (U7-1 T4) and the
	// bash exec path was deleted with lib/ (GO-20 U2e T5 PR4b). The field is
	// kept because the route table test still pins it as empty for every verb.
	script    string
	invokedAs string // AI_SPECS_INVOKED_AS label (add-dep and skills add, U5c)
}

// NO SHIM MAP (U7-1 T4): the strangler's verb→script table is gone. U6e emptied
// it — every arm of lib/recipe.sh and lib/recipe-config.sh this dispatcher
// reached had a native route — and U7-2 ported the last verb that ever resolved
// through it. The bash EXEC path itself is gone too: GO-20 U2e T5 PR4b deleted
// lib/ and with it the last route that could produce a shim.

// native maps the native verbs and their aliases to their route kind.
// `--help`/`-h` keep ignoring extra arguments; `help` validates its optional
// topic, and version routes reject extra positional arguments.
//
// brief-render-policy and baseline-brief are INTERNAL seams: legacy scripts
// reach them through AI_SPECS_GO_BIN (S19a), so they are routed here and
// deliberately absent from help.txt.
var native = map[string]route{
	// sync and sync-agent are native (cards [Go 07.S1] and [Go 07.S15]).
	"configure-recipes":   {kind: routeConfigureRecipes},
	"refresh-bundled":     {kind: routeRefreshBundled},
	"sync":                {kind: routeSync},
	"sync-agent":          {kind: routeSyncAgent},
	"doctor":              {kind: routeDoctor},
	"rules-audit":         {kind: routeRulesAudit},
	"brief-render-policy": {kind: routeBriefRenderPolicy},
	"baseline-brief":      {kind: routeBaselineBrief},
	"upgrade":             {kind: routeUpgrade},
	"hub":                 {kind: routeHub},
	// init is native for the classic path and delegates the wizard to the shim
	// itself (odd/tasks/go-20-u4-init.md S6).
	"init":              {kind: routeInit},
	"version":           {kind: routeVersion},
	"-v":                {kind: routeVersion},
	"--version":         {kind: routeVersion},
	"help":              {kind: routeHelp},
	"-h":                {kind: routeHelp},
	"--help":            {kind: routeHelp},
	"worktree-gate":     {kind: routeWorktreeGate},
	"tracker-card-gate": {kind: routeTrackerCardGate},
	"plan-build-gate":   {kind: routePlanBuildGate},
	"worktree-cleanup":  {kind: routeWorktreeCleanup},
}

// Route resolves the dispatcher decision for args. Bare invocation (no args)
// is rewritten to `hub`, not to help — a deliberate product decision of the
// legacy launcher.
func Route(args []string) route {
	if len(args) == 0 {
		return native["hub"]
	}
	if r, ok := native[args[0]]; ok {
		return r
	}
	// `skills list` (GO-20 U5a), `skills remove` (GO-20 U5b) and `skills add`
	// (GO-20 U5c) are native: the subcommands are intercepted HERE in
	// lib/skills.sh's own case order (add|list|remove first), and every
	// REMAINING arm of that case statement (`--help|-h|help`, the bare
	// `skills`, unknown subcommands) resolves to the native dispatcher route —
	// the shell dispatcher has no work left for this binary. `add-dep` is the
	// same verb under the label bin/ai-specs's `add-dep)` arm exports.
	if args[0] == "skills" {
		if len(args) > 1 {
			switch args[1] {
			case "add":
				return route{kind: routeSkillsAdd, invokedAs: "ai-specs skills add"}
			case "list":
				return route{kind: routeSkillsList}
			case "remove":
				return route{kind: routeSkillsRemove}
			case "validate":
				return route{kind: routeSkillsValidate}
			case "field":
				return route{kind: routeSkillsField}
			}
		}
		return route{kind: routeSkills}
	}
	if args[0] == "add-dep" {
		return route{kind: routeAddDep, invokedAs: "ai-specs add-dep"}
	}
	// `recipe list` (GO-20 U6a), `recipe add` (GO-20 U6b), `recipe remove`
	// (GO-20 U6c) and `recipe configure` (GO-20 U6d-1) are native: the
	// subcommands are intercepted HERE, in lib/recipe.sh's own case order.
	// U6e finished the dispatcher remainder and U7-2 ported `init` too, so
	// every arm (`--help|-h|help`, the bare `recipe`, unknown subcommands)
	// resolves to the native dispatcher route — usage on stdout, usage on
	// stderr exit 2, the two unknown lines exit 1 (S4).
	// Route cannot see the streams, so `recipe add`'s TTY tail lives in Run:
	// since U6e it is native end to end — wizard, dep gate and env offer all
	// through their own seams, no delegation (pinned by
	// TestRecipeAddTailIsNativeOnATerminal).
	if args[0] == "recipe" {
		if len(args) > 1 {
			switch args[1] {
			case "list":
				return route{kind: routeRecipeList}
			case "add":
				return route{kind: routeRecipeAdd}
			case "remove":
				return route{kind: routeRecipeRemove}
			case "configure":
				return route{kind: routeRecipeConfigure}
			case "init":
				return route{kind: routeRecipeInitVerb}
			}
		}
		return route{kind: routeRecipe}
	}
	// `specs promote` (GO-20 U2e U-A) is the family's only verb; the root
	// dispatcher has no `promote` of its own, and every remaining arm of the
	// family (bare `specs`, `--help|-h|help`, unknown subcommands) resolves to
	// the native family remainder.
	if args[0] == "specs" {
		if len(args) > 1 && args[1] == "promote" {
			return route{kind: routeSpecsPromote}
		}
		return route{kind: routeSpecs}
	}
	// `changes archive` (I5/P4), `changes start` (T14/S19), `changes workstream`
	// (change-ledger-v2 T3) and `changes status` (change-ledger-v2 T7) are the family's
	// verbs; the root dispatcher has no `archive`, `start`, `workstream` or `status` of its
	// own, and every remaining arm of the family (bare `changes`, `--help|-h|help`, unknown
	// subcommands) resolves to the native family remainder.
	if args[0] == "changes" {
		if len(args) > 1 && args[1] == "archive" {
			return route{kind: routeChangesArchive}
		}
		if len(args) > 1 && args[1] == "start" {
			return route{kind: routeChangesStart}
		}
		if len(args) > 1 && args[1] == "workstream" {
			return route{kind: routeChangesWorkstream}
		}
		if len(args) > 1 && args[1] == "status" {
			return route{kind: routeChangesStatus}
		}
		return route{kind: routeChanges}
	}
	// `gates premerge` (GO-20 U2e U-B) is the family's only verb; the root
	// dispatcher has no `premerge` of its own, and every remaining arm of the
	// family (bare `gates`, `--help|-h|help`, unknown subcommands) resolves to
	// the native family remainder.
	if args[0] == "gates" {
		if len(args) > 1 && args[1] == "premerge" {
			return route{kind: routeGatesPremerge}
		}
		return route{kind: routeGates}
	}
	// `mcp vault-fs` (T4/S5) is the family's only verb: it serves the vault
	// filesystem MCP server the recipe used to materialize as a script. Every
	// other arm of the family stays an unknown command.
	if args[0] == "mcp" {
		if len(args) > 1 && args[1] == "vault-fs" {
			return route{kind: routeMCPVaultFS}
		}
		return route{kind: routeUnknown}
	}
	// `tracker slug` and `tracker evidence` (GO-20 U2e U-C) are the family's only
	// verbs; every remaining arm (bare `tracker`, `--help|-h|help`, unknown
	// subcommands) resolves to the native family remainder.
	if args[0] == "hook" {
		if len(args) > 1 {
			switch args[1] {
			case "field":
				return route{kind: routeHookField}
			case "normalize-path":
				return route{kind: routeHookNormalizePath}
			}
		}
		return route{kind: routeHook}
	}
	if args[0] == "tracker" {
		if len(args) > 1 {
			switch args[1] {
			case "slug":
				return route{kind: routeTrackerSlug}
			case "evidence":
				return route{kind: routeTrackerEvidence}
			case "field":
				return route{kind: routeTrackerField}
			case "classify":
				return route{kind: routeTrackerClassify}
			}
		}
		return route{kind: routeTracker}
	}
	return route{kind: routeUnknown}
}

// Run executes the CLI and returns the process exit code.
func Run(args []string, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	r := Route(args)
	switch r.kind {
	case routeWorktreeGate:
		version := "unknown"
		if data, err := os.ReadFile(filepath.Join(home, "VERSION")); err == nil {
			version = strings.TrimRight(string(data), "\r\n")
		}
		if _, hookMode := worktreeGateHookRequest(args[1:]); hookMode {
			return runWorktreeGateHook(home, stdin, stdout, stderr, version)
		}
		return runner.Run(args[1:], stdin, stdout, stderr, version)
	case routeWorktreeCleanup:
		return runWorktreeCleanup(home, args[1:], stdout, stderr)
	case routeTrackerCardGate:
		// Hook mode takes no arguments; the direct host mode takes its own
		// (--root/--checkpoint/--stage) and is selected from the same verb.
		return runTrackerCardGate(home, args[1:], stdin, stdout, stderr)
	case routePlanBuildGate:
		// Pre-tool-use hook only: one event on stdin, no arguments, like the
		// script it replaces.
		return runPlanBuildGate(args[1:], stdin, stdout, stderr)
	case routeMCPVaultFS:
		// args[0]=mcp, args[1]=vault-fs: the verb parses no arguments, like the
		// script it replaces.
		return runVaultFSMCP(stdin, stdout, stderr)
	case routeVersion:
		if len(args) > 1 {
			fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", args[1])
			fmt.Fprintln(stderr, "Run 'ai-specs --help' for usage.")
			return 2
		}
		return runVersion(home, stdout)
	case routeHelp:
		if args[0] == "help" && len(args) > 1 && Route(args[1:2]).kind == routeUnknown {
			fmt.Fprintf(stderr, "ERROR: unknown help topic: %s\n", args[1])
			fmt.Fprintln(stderr, "Run 'ai-specs help' for usage.")
			return 2
		}
		return runHelp(stdout)
	case routeRefreshBundled:
		return runRefreshBundled(args, stdout, stderr)
	case routeSync:
		return sync.Run(args, home, stdin, stdout, stderr)
	case routeSyncAgent:
		return sync.RunAgent(args, home, stdin, stdout, stderr)
	case routeDoctor:
		return runDoctor(args, home, stdout, stderr)
	case routeRulesAudit:
		return runRulesAudit(args, home, stdout, stderr)
	case routeBriefRenderPolicy:
		return runBriefRenderPolicy(args, stdout, stderr)
	case routeBaselineBrief:
		return runBaselineBrief(args, home, stdout, stderr)
	case routeUpgrade:
		// The upgrade engine parses the args AFTER the verb, like the shim
		// does; its exit codes are FROZEN (parity contract section 2).
		rest := args
		if len(rest) > 0 {
			rest = rest[1:]
		}
		return upgrade.Run(rest, upgradeEnv(home), stdout, stderr)
	case routeHub:
		rest := args
		if len(rest) > 0 {
			rest = rest[1:]
		}
		return hub.RunHub(hub.Options{
			Args: rest,
			Home: home,
			TTY:  isTTY(stdin, stdout),
		}, stdin, stdout, stderr)
	case routeInit:
		return runInit(args, home, stdin, stdout, stderr, selfPath())
	case routeSkillsList:
		// args[0]=skills, args[1]=list: skills.sh shifts the subcommand before
		// it execs skills-list.sh, so the verb's own loop sees args[2:].
		return runSkillsList(args[2:], home, stdout, stderr)
	case routeSkillsRemove:
		// args[0]=skills, args[1]=remove: same shift as the list route — the
		// verb's own loop sees args[2:] (skills.sh: `remove) … skills-remove.sh
		// "$@"`).
		return runSkillsRemove(args[2:], stdout, stderr)
	case routeSkillsAdd:
		// args[0]=skills, args[1]=add: skills.sh shifts the subcommand before
		// it execs skills-add.sh with AI_SPECS_INVOKED_AS="ai-specs skills add",
		// so the verb's own loop sees args[2:] under that label.
		return runSkillsAdd(args[2:], r.invokedAs, home, stdin, stdout, stderr)
	case routeAddDep:
		// bin/ai-specs: `add-dep) AI_SPECS_INVOKED_AS="ai-specs add-dep" bash
		// "$LIB_DIR/skills-add.sh" "$@"` — the same verb, one label shift less,
		// and the exact label the legacy alias exported.
		return runSkillsAdd(args[1:], r.invokedAs, home, stdin, stdout, stderr)
	case routeSkillsValidate:
		// args[0]=skills, args[1]=validate: the verb's own grammar sees args[2:].
		return runSkillsValidate(args[2:], stdout, stderr)
	case routeSkillsField:
		// args[0]=skills, args[1]=field: the same shift; the shared reader owns
		// the grammar (one <path>, shared helper, stderr+2 on usage errors).
		return runJSONField(args[2:], stdin, stdout, stderr, "ai-specs skills field", skillsFieldUsage, false)
	case routeSkills:
		// args[0]=skills: skills.sh shifts the verb before its case statement,
		// so the dispatcher's own loop sees args[1:].
		return runSkillsDispatch(args[1:], stdout, stderr)
	case routeRecipeList:
		// args[0]=recipe, args[1]=list: recipe.sh shifts the subcommand before
		// it execs recipe-list.sh, so the verb's own loop sees args[2:].
		return runRecipeList(args[2:], home, selfPath(), stdout, stderr)
	case routeConfigureRecipes:
		// config_wizard.main's whole verb: the wrapper's argument loop (its
		// own usage contract), then the guard, the TTY gate and the loop. The
		// TTY decision needs the streams, so Run computes it like the other
		// interactive routes.
		return runConfigureRecipes(args[1:], home, stdin, stdout, stderr, isTTY(stdin, stdout))
	case routeRecipe:
		// args[0]=recipe: the dispatcher remainder of lib/recipe.sh's case
		// statement, native since U6e — usage on stdout for --help/-h/help,
		// usage on stderr exit 2 for the bare verb, the two unknown lines
		// exit 1.
		return runRecipeDispatch(args[1:], stdout, stderr)
	case routeRecipeInitVerb:
		// args[0]=recipe, args[1]=init: recipe.sh's init arm shifts the
		// subcommand before its wrapper, so the verb's own grammar sees
		// args[2:] (GO-20 U7-2 ported the brief and its wrapper natively).
		return runRecipeInitVerb(args[2:], home, selfPath(), stdout, stderr)
	case routeSpecs:
		// args[0]=specs: the family remainder sees the subcommand slot.
		return runSpecsDispatch(args[1:], stdout, stderr)
	case routeSpecsPromote:
		// args[0]=specs, args[1]=promote: the verb's own grammar sees args[2:].
		return runSpecsPromote(args[2:], stdout, stderr)
	case routeChanges:
		// args[0]=changes: the family remainder sees the subcommand slot.
		return runChangesDispatch(args[1:], stdout, stderr)
	case routeChangesArchive:
		// args[0]=changes, args[1]=archive: the verb's own grammar sees args[2:].
		return runChangesArchive(args[2:], stdout, stderr)
	case routeChangesStart:
		// args[0]=changes, args[1]=start: the verb's own grammar sees args[2:].
		return runChangesStart(args[2:], stdout, stderr)
	case routeChangesWorkstream:
		// args[0]=changes, args[1]=workstream: the verb's own grammar sees args[2:].
		return runChangesWorkstream(args[2:], stdout, stderr)
	case routeChangesStatus:
		// args[0]=changes, args[1]=status: the verb's own grammar sees args[2:].
		return runChangesStatus(args[2:], stdout, stderr)
	case routeGates:
		// args[0]=gates: the family remainder sees the subcommand slot.
		return runGatesDispatch(args[1:], stdout, stderr)
	case routeGatesPremerge:
		// args[0]=gates, args[1]=premerge: the verb's own grammar sees args[2:].
		return runGatesPremerge(args[2:], stdout, stderr)
	case routeTracker:
		// args[0]=tracker: the family remainder sees the subcommand slot.
		return runTrackerDispatch(args[1:], stdout, stderr)
	case routeTrackerSlug:
		// args[0]=tracker, args[1]=slug: the verb's own grammar sees args[2:].
		return runTrackerSlug(args[2:], stdout, stderr)
	case routeTrackerEvidence:
		// args[0]=tracker, args[1]=evidence: the verb's own grammar sees args[2:].
		return runTrackerEvidence(args[2:], stdout, stderr)
	case routeTrackerField:
		// args[0]=tracker, args[1]=field: the verb's own grammar sees args[2:].
		return runTrackerFieldVerb(args[2:], stdin, stdout, stderr)
	case routeTrackerClassify:
		// args[0]=tracker, args[1]=classify: the verb's own grammar sees args[2:].
		return runTrackerClassify(args[2:], stdin, stdout, stderr)
	case routeHook:
		// args[0]=hook: the family remainder sees the subcommand slot.
		return runHookDispatch(args[1:], stdout, stderr)
	case routeHookField:
		// args[0]=hook, args[1]=field: the verb's own grammar sees args[2:].
		return runHookField(args[2:], stdin, stdout, stderr)
	case routeHookNormalizePath:
		// args[0]=hook, args[1]=normalize-path: the verb's grammar sees args[2:].
		return runHookNormalizePath(args[2:], stdin, stdout, stderr)
	case routeRecipeAdd:
		// args[0]=recipe, args[1]=add: recipe.sh shifts the subcommand before
		// it execs recipe-add.sh, so the verb's own loop sees args[2:]. No
		// delegation since U6e: the TTY tail runs natively — wizard, dep-gate
		// seam and env-offer seam (GO-20 U6e, T3b second half).
		return runRecipeAdd(args, home, stdin, stdout, stderr, selfPath(), isTTY(stdin, stdout))
	case routeRecipeRemove:
		// args[0]=recipe, args[1]=remove: recipe.sh shifts the subcommand
		// before it execs recipe-remove.sh, so the verb's own loop sees
		// args[2:]. No TTY seam: the script has no interactive branch.
		return runRecipeRemove(args[2:], stdout, stderr)
	case routeRecipeConfigure:
		// args[0]=recipe, args[1]=configure: recipe.sh's case statement shifts
		// the subcommand before it execs recipe-configure.py, so the verb's own
		// loop sees args[2:]. No TTY seam (the script's arm is a plain
		// python3 exec), no cd/pwd resolution (neither recipe.sh nor
		// recipe-configure.py resolves the target — it takes the path
		// positional and Path.resolve()s it, default ".").
		return runRecipeConfigure(args[2:], home, stdout, stderr)
	case routeUnknown:
		fmt.Fprintf(stderr, "ai-specs: unknown command '%s'\n", args[0])
		fmt.Fprintln(stderr, "Run 'ai-specs help' for usage.")
		return 2
	}
	// Every routeKind has a case above, so this is unreachable; the dispatcher
	// still fails closed rather than falling off the end.
	return 2
}

// isTTY mirrors the shell's `[[ -t 0 && -t 1 ]]` and hub.py's
// `stdin.isatty() and stdout.isatty()`: a character device is a terminal. The
// statistic is what keeps the hub dependency-free — no terminal library, and no
// syscall per platform.
// isTTY is the dispatcher's view of the shell's `[[ -t 0 && -t 1 ]]`. It delegates
// to internal/tty, which asks the KERNEL: a character-device stat is not the same
// question, and answering it that way made the hub's anti-hang guard launch Python
// instead of printing its frozen message (GO-20 U3a-fix).
func isTTY(stdin io.Reader, stdout io.Writer) bool {
	return tty.IsTTY(stdin, stdout)
}

// upgradeEnv resolves the process environment the upgrade engine runs against.
// AI_SPECS_RELEASE_BASE is honored here exactly as the launcher's self-heal
// honors it: without this the dist refresh always hit the default GitHub URL,
// even when the install came from a mirror or a local release base (JD-A-005).
func upgradeEnv(home string) upgrade.Env {
	return upgrade.Env{
		Home:        home,
		UserHome:    os.Getenv("HOME"),
		Executable:  selfPath(),
		ReleaseBase: os.Getenv("AI_SPECS_RELEASE_BASE"),
	}
}

// selfPath is the running dispatcher's own absolute path, exported to shimmed
// children as AI_SPECS_GO_BIN (GO-07 S19a) so a legacy script can reach a
// ported subcommand instead of its legacy Python route. A resolution failure
// leaves the seam unset: the script then keeps its legacy route, which is the
// fail-open direction this seam requires.
func selfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}
