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
)

// routeKind classifies a dispatcher decision.
type routeKind int

const (
	routeShim    routeKind = iota // exec bash <home>/lib/<script>
	routeVersion                  // native: print <home>/VERSION
	routeHelp                     // native: print embedded help.txt
	routeUnknown                  // unknown command: usage error, exit 2
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
	"hub":               {kind: routeShim, script: "hub.sh"},
	"init":              {kind: routeShim, script: "init.sh"},
	"sync":              {kind: routeShim, script: "sync.sh"},
	"sync-agent":        {kind: routeShim, script: "sync-agent.sh"},
	"refresh-bundled":   {kind: routeShim, script: "refresh-bundled.sh"},
	"add-dep":           {kind: routeShim, script: "skills-add.sh", invokedAs: "ai-specs add-dep"},
	"skills":            {kind: routeShim, script: "skills.sh"},
	"doctor":            {kind: routeShim, script: "doctor.sh"},
	"rules-audit":       {kind: routeShim, script: "rules-audit.sh"},
	"recipe":            {kind: routeShim, script: "recipe.sh"},
	"configure-recipes": {kind: routeShim, script: "recipe-config.sh"},
	"upgrade":           {kind: routeShim, script: "upgrade.sh"},
}

// native maps the version/help verbs and their aliases to their route kind.
// Like the legacy dispatcher, version and help ignore any extra arguments
// (the verb runs and the rest is dropped).
var native = map[string]route{
	"version":   {kind: routeVersion},
	"-v":        {kind: routeVersion},
	"--version": {kind: routeVersion},
	"help":      {kind: routeHelp},
	"-h":        {kind: routeHelp},
	"--help":    {kind: routeHelp},
}

// Route resolves the dispatcher decision for args. Bare invocation (no args)
// is rewritten to `hub`, not to help — a deliberate product decision of the
// legacy launcher.
func Route(args []string) route {
	if len(args) == 0 {
		return shims["hub"]
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
	switch r.kind {
	case routeVersion:
		return runVersion(home, stdout)
	case routeHelp:
		return runHelp(stdout)
	case routeUnknown:
		fmt.Fprintf(stderr, "ai-specs: unknown command '%s'\n", args[0])
		fmt.Fprintln(stderr, "Run 'ai-specs help' for usage.")
		return 2
	default:
		return runShim(r, args, home, stdin, stdout, stderr)
	}
}
