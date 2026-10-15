package sync

// Native --resolved-config-only mode (GO-07 S14c).
//
// Ports build_resolved_config_only (lib/_internal/recipe-materialize.py:4143-4205),
// the lightweight mode the standalone sync-agent fan-out uses to enrich a
// subrepo's AGENTS.md: build the resolved-config projection, auto-bind
// capabilities through the same native authority, apply the S14b catalog
// enrichment, and write the document. No cache ensure, no legacy-origin
// cleanup, no per-recipe loop, no hooks, no lock writes, no orphan cleanup and
// no recipe-MCP temp file — the Python docstring's "no side effects" contract.
//
// Stream contract: nothing is printed on success. A manifest-validation failure
// prints Python's "ERROR: binding validation failed: <msg>" and fails; any
// other failure prints "WARNING: resolved-config generation failed: <msg>" and
// fails; a manifest that cannot be re-read for the auto-binding step degrades
// silently to the explicit bindings, exactly like the Python inner
// `except Exception: pass`.

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/toml"
	shared "ai-specs.dev/worktree-gate/shared"
)

// ResolvedConfigOnlyRequest is the root boundary input for one
// --resolved-config-only run. Stdout is accepted because the Python mode owns
// both streams at its call sites; it stays empty on success.
type ResolvedConfigOnlyRequest struct {
	ProjectRoot string
	AISpecsHome string
	Out         string
	Stdout      io.Writer
	Stderr      io.Writer
}

// BuildResolvedConfigOnly writes the resolved-config document to Out and
// returns the failure the caller maps to exit status 1, mirroring the Python
// mode's `return 1` paths.
func BuildResolvedConfigOnly(req ResolvedConfigOnlyRequest) error {
	stdout := req.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	_ = stdout // the Python mode prints nothing on success

	catalogDir := filepath.Join(req.AISpecsHome, "catalog", "recipes")

	resolved, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: req.ProjectRoot})
	if err != nil {
		fmt.Fprintf(stderr, "WARNING: resolved-config generation failed: %s\n", err)
		return err
	}

	// `if enabled_ids:` guard and then the auto-binding assignment `if
	// auto_bindings:`. The manifest is re-read here exactly as Python re-reads
	// it inside the guarded block; a read failure degrades silently.
	if manifest, readErr := config.LoadManifest(materializeManifestPath(req.ProjectRoot)); readErr == nil {
		enabledIDs, bindings := resolvedConfigOnlyInputs(manifest)
		if len(enabledIDs) > 0 {
			resolution := ResolveBindings(BindingRequest{
				EnabledRecipeIDs: enabledIDs,
				CatalogDir:       catalogDir,
				Bindings:         bindings,
			})
			if len(resolution.Errors) > 0 {
				message := resolution.Errors[0]
				fmt.Fprintf(stderr, "ERROR: binding validation failed: %s\n", message)
				return errors.New(message)
			}
			if len(resolution.Bindings) > 0 {
				resolved.Bindings = resolution.Bindings
			}
		}
	}

	document, err := materializeEnrichedResolvedConfigDocument(resolved, catalogDir)
	if err != nil {
		fmt.Fprintf(stderr, "WARNING: resolved-config generation failed: %s\n", err)
		return err
	}
	if err := writeMaterializeDocument(req.Out, document); err != nil {
		fmt.Fprintf(stderr, "WARNING: resolved-config generation failed: %s\n", err)
		return err
	}
	return nil
}

// resolvedConfigOnlyInputs extracts the enabled recipe ids in manifest order
// (Python's `val.get("enabled") is True`, an identity check on the boolean) and
// the explicit [[bindings]] entries through the same acquisition the projection
// uses.
func resolvedConfigOnlyInputs(manifest *toml.Table) ([]string, []shared.ManifestBinding) {
	payload := acquireResolvedConfigPayload(manifest)
	enabledIDs := []string{}
	for _, entry := range payload.Recipes {
		if enabled, ok := entry.Table["enabled"].(bool); ok && enabled {
			enabledIDs = append(enabledIDs, entry.ID)
		}
	}
	return enabledIDs, loadManifestBindings(manifest)
}
