// Command ai-specs is the native single-binary dispatcher for the
// ai-specs CLI (Go single-binary migration epic). It reproduces the
// dispatcher contract of bin/ai-specs: `version` and `help` are implemented
// natively, every other verb execs the legacy lib/*.sh implementation via the
// shim in internal/cli.
package main

import (
	"os"

	"ai-specs.dev/ai-specs/internal/cli"
	"ai-specs.dev/ai-specs/internal/home"
)

func main() {
	execPath, err := os.Executable()
	if err != nil {
		execPath = ""
	}
	homeDir := home.ResolveHome(execPath, os.Getenv("AI_SPECS_HOME"))
	os.Exit(cli.Run(os.Args[1:], homeDir, os.Stdin, os.Stdout, os.Stderr))
}
