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

	"ai-specs.dev/ai-specs/internal/hub"
	"ai-specs.dev/ai-specs/internal/sync"
	"ai-specs.dev/ai-specs/internal/tty"
	"ai-specs.dev/ai-specs/internal/upgrade"
)

// routeKind classifies a dispatcher decision.
type routeKind int

const (
	routeShim           routeKind = iota // exec bash <home>/lib/<script>
	routeVersion                         // native: print <home>/VERSION
	routeHelp                            // native: print embedded help.txt
	routeRefreshBundled                  // native compatibility stub (card [Go 06]): reports embedded provenance
	routeSync                            // native sync spine (card [Go 07.S1])
	routeSyncAgent                       // native sync-agent fan-out (card [Go 07.S15])
	routeDoctor                          // native: `doctor` health diagnostic (card [Go 08])
	routeRulesAudit                      // native: `rules-audit` legacy rules inventory (card [Go 08])
	routeBriefRenderPolicy               // native internal seam: lib/init.sh's policy gate (S19b)
	routeBaselineBrief                   // native internal seam: lib/init.sh's baseline render (S19b)
	routeUpgrade                         // native: `upgrade` legacy install upgrade (card [Go 12])
	routeHub                             // native: `hub` front door, non-interactive half (GO-20 U3a)
	routeUnknown                         // unknown command: usage error, exit 2
)

// route is one dispatcher decision.
type route struct {
	kind      routeKind
	script    string // lib script name for shims, e.g. "hub.sh"
	invokedAs string // AI_SPECS_INVOKED_AS override (add-dep only)
}

// shims maps every shimmed verb to its legacy lib script, mirroring the case
// statement of bin/ai-specs exactly.
var shims = map[string]route{
	"init":              {kind: routeShim, script: "init.sh"},
	"add-dep":           {kind: routeShim, script: "skills-add.sh", invokedAs: "ai-specs add-dep"},
	"skills":            {kind: routeShim, script: "skills.sh"},
	"recipe":            {kind: routeShim, script: "recipe.sh"},
	"configure-recipes": {kind: routeShim, script: "recipe-config.sh"},
}

// native maps the native verbs and their aliases to their route kind.
// Like the legacy dispatcher, version and help ignore any extra arguments
// (the verb runs and the rest is dropped).
//
// brief-render-policy and baseline-brief are INTERNAL seams: legacy scripts
// reach them through AI_SPECS_GO_BIN (S19a), so they are routed here and
// deliberately absent from help.txt.
var native = map[string]route{
	// sync and sync-agent are native (cards [Go 07.S1] and [Go 07.S15]).
	"refresh-bundled":     {kind: routeRefreshBundled},
	"sync":                {kind: routeSync},
	"sync-agent":          {kind: routeSyncAgent},
	"doctor":              {kind: routeDoctor},
	"rules-audit":         {kind: routeRulesAudit},
	"brief-render-policy": {kind: routeBriefRenderPolicy},
	"baseline-brief":      {kind: routeBaselineBrief},
	"upgrade":             {kind: routeUpgrade},
	"hub":                 {kind: routeHub},
	"version":             {kind: routeVersion},
	"-v":                  {kind: routeVersion},
	"--version":           {kind: routeVersion},
	"help":                {kind: routeHelp},
	"-h":                  {kind: routeHelp},
	"--help":              {kind: routeHelp},
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
	if r, ok := shims[args[0]]; ok {
		return r
	}
	return route{kind: routeUnknown}
}

// Run executes the CLI and returns the process exit code.
func Run(args []string, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	r := Route(args)
	switch r.kind {	case routeVersion:
		return runVersion(home, stdout)
	case routeHelp:
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
		return upgrade.Run(rest, upgrade.Env{
			Home:       home,
			UserHome:   os.Getenv("HOME"),
			Executable: selfPath(),
		}, stdout, stderr)
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
	case routeUnknown:
		fmt.Fprintf(stderr, "ai-specs: unknown command '%s'\n", args[0])
		fmt.Fprintln(stderr, "Run 'ai-specs help' for usage.")
		return 2
	default:
		return runShim(r, args, home, stdin, stdout, stderr, selfPath())
	}
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
