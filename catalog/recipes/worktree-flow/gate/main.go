// Command worktree-gate is the standalone adapter for the importable runner.
package main

import (
	"os"

	"ai-specs.dev/worktree-gate/runner"
)

// version is injected at build time with -X main.version by scripts/build-gate.sh.
var version = "dev"

func main() {
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version))
}
