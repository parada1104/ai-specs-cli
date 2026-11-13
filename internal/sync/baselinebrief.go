package sync

// The baseline brief render (GO-07 S19b).
//
// It is the chain lib/init.sh has always run through Python: the materializer
// with only the resolved-config output requested, then the AGENTS.md renderer
// with --preserve-if-runtime-brief and the optional --adopt-brief. Both pieces
// are the ones the sync pipeline already owns (MaterializeRecipes,
// RenderAgentsFile); this file only composes them for the internal verb the
// script reaches through AI_SPECS_GO_BIN.
//
// It owns no policy: the [brief].render gate is the caller's, exactly as it is
// the shell's today, so the disabled and failure branches keep printing what
// they always printed.

import (
	"io"
	"path/filepath"
)

// BaselineBriefRequest is one baseline render.
type BaselineBriefRequest struct {
	ProjectRoot       string
	AISpecsHome       string
	ResolvedConfigOut string
	AdoptBrief        bool
	Stdout            io.Writer
	Stderr            io.Writer
}

// RenderBaselineBrief runs the chain and returns the status the shell's
// `materialize && render` produces: a failing materialize short-circuits the
// render, and the first failure's status is the one reported.
func RenderBaselineBrief(req BaselineBriefRequest) int {
	stdout := req.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	resolvedOut := req.ResolvedConfigOut
	if _, err := MaterializeRecipes(MaterializeRequest{
		ProjectRoot:       req.ProjectRoot,
		AISpecsHome:       req.AISpecsHome,
		ResolvedConfigOut: &resolvedOut,
		Stdout:            stdout,
		Stderr:            stderr,
	}); err != nil {
		return 1
	}

	_, _, rc := RenderAgentsFile(
		filepath.Join(req.ProjectRoot, "ai-specs", "ai-specs.toml"),
		filepath.Join(req.ProjectRoot, "AGENTS.md"),
		RenderAgentsOptions{
			// --preserve-if-runtime-brief is inert by design (D5): the marker
			// preserves unconditionally, so setting it cannot suppress a write.
			PreserveIfRuntimeBrief: true,
			AdoptBrief:             req.AdoptBrief,
			ResolvedConfigPath:     req.ResolvedConfigOut,
		},
		stdout,
		stderr,
	)
	return rc
}
