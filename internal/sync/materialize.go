package sync

// Native materializer spine (GO-07 S14a WU1), added and NOT wired: no pipeline
// call site invokes MaterializeRecipes yet (S14c owns the wiring), and the
// frozen stdout markers stay byte-identical to the Python authority
// lib/_internal/recipe-materialize.py materialize_recipes (:3858-4142).
//
// WU1 owns the orchestration order up to and including the temp-document
// writes: cache ensure, manifest load and enabled filter, legacy-origin
// cleanup, the internal-test guard, deps/hooks init, the no-enabled branch,
// binding resolution plus witness, the stamp step, tag and primitive conflict
// checks, the recipe-MCP build, the provenance loop, orphan cleanup, the
// per-recipe frame (print + legacy-version warning + config merge) and the
// recipe-MCP / resolved-config / resolved-hooks documents.
//
// The per-recipe artifact actuation (skills, commands, templates, docs, hooks,
// runtime hooks, gate-binary acquisition) is WU2 (T4) and is deliberately
// absent here. The two known non-ported steps are documented in the handoff:
// dependency-skill vendoring (its own card) and the worktree-flow gate-binary
// acquisition (WU2's loop; internal/doctor exposes no acquisition entry point
// and the WU1 edit surface forbids extending it).
//
// The only decision logic in this file is the orchestration itself; every
// grader is the merged native authority: ResolveBindings, PlanReconcileStamps
// plus the promoted shared writer, PlanTagConflicts, shared.CheckPrimitiveConflicts,
// PlanMergeConfig, BuildRecipeMCP, CleanOrphans and BuildResolvedConfig.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/toml"
	shared "ai-specs.dev/worktree-gate/shared"
)

// MaterializeRequest mirrors the Python flags as optional fields. The three
// output paths are optional exactly as the Python None defaults; RefreshGates
// mirrors --refresh-gates. Stdout/Stderr receive the verbatim spine output.
type MaterializeRequest struct {
	ProjectRoot       string
	AISpecsHome       string
	RecipeMCPOut      *string
	ResolvedConfigOut *string
	ResolvedHooksOut  *string
	RefreshGates      bool
	Stdout            io.Writer
	Stderr            io.Writer
}

// MaterializeResult carries the paths and serialized documents the caller
// needs. A document is empty when its output path was absent (the recipe-MCP
// document is always serialized because the marker prints its temp path).
type MaterializeResult struct {
	RecipeMCPPath          string
	RecipeMCPDocument      string
	ResolvedConfigPath     string
	ResolvedConfigDocument string
	ResolvedHooksPath      string
	ResolvedHooksDocument  string
	ResolvedBindings       map[string]string
	EnabledRecipeIDs       []string
}

// materializeFatal mirrors the Python fail() helper: "  ✗ <msg>" on stderr,
// then a returned error so the caller can exit non-zero. Python exits
// immediately, so only the first fatal is ever printed.
func materializeFatal(stderr io.Writer, msg string) error {
	fmt.Fprintf(stderr, "  ✗ %s\n", msg)
	return errors.New(msg)
}

// MaterializeRecipes is the native materializer spine. It returns the paths and
// documents the caller needs; all user-visible output goes to the request's
// writers through the frozen verbatim lines.
func MaterializeRecipes(req MaterializeRequest) (MaterializeResult, error) {
	var result MaterializeResult
	stdout := req.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	catalogDir := filepath.Join(req.AISpecsHome, "catalog", "recipes")
	manifestPath := materializeManifestPath(req.ProjectRoot)
	cliHome := req.AISpecsHome

	if _, err := projectcache.EnsureCache(req.ProjectRoot, cliHome); err != nil {
		return result, materializeFatal(stderr, err.Error())
	}

	manifest, err := loadMaterializeManifest(manifestPath)
	if err != nil {
		return result, materializeFatal(stderr, err.Error())
	}

	recipes := loadMaterializeRecipes(manifest)
	enabled := make([]materializeRecipe, 0, len(recipes))
	for _, recipe := range recipes {
		if recipe.Enabled {
			enabled = append(enabled, recipe)
		}
	}
	enabledIDs := materializeRecipeIDs(enabled)
	result.EnabledRecipeIDs = enabledIDs

	projectcache.RemoveLegacyOrigin(req.ProjectRoot, cliHome, stdout, stderr)

	allowInternal := os.Getenv("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES") == "1"
	blocked := []string{}
	for _, recipe := range enabled {
		if strings.HasPrefix(recipe.ID, "test-") {
			blocked = append(blocked, recipe.ID)
		}
	}
	sort.Strings(blocked)
	if len(blocked) > 0 && !allowInternal {
		msg := fmt.Sprintf(
			"Recipe '%s' is an internal test fixture and is not part of the public catalog.",
			blocked[0],
		)
		return result, materializeFatal(stderr, msg)
	}

	// Runtime hooks collected across enabled recipes (for hooks-render.py).
	// WU2 fills this from the per-recipe runtime-hook loop; WU1 writes the
	// empty list the spine starts with.
	resolvedHooks := []any{}

	expectedDepIDs := materializeStringSet(loadManifestDepIDs(manifest))

	if len(enabled) == 0 {
		projectcache.RemoveRecipeCommandLeftovers(req.ProjectRoot, cliHome, nil, nil, stdout, stderr)
		planInput, roots := materializeOrphanInputs(req.ProjectRoot, cliHome, []string{}, materializeSortedSet(expectedDepIDs))
		if _, err := cleanOrphansNative(planInput, roots, req.ProjectRoot, stdout); err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		binding := ResolveBindings(BindingRequest{
			EnabledRecipeIDs: []string{},
			CatalogDir:       catalogDir,
			Bindings:         nil,
			ProjectRoot:      req.ProjectRoot,
			WriteWitness:     true,
		})
		if fatal := materializeBindingWarnings(stderr, binding); fatal != nil {
			return result, materializeFatal(stderr, fatal.Error())
		}
		fmt.Fprintln(stdout, "  (no [recipes.*] enabled — skipping)")
		if req.ResolvedConfigOut != nil {
			resolved, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: req.ProjectRoot})
			if err != nil {
				return result, materializeFatal(stderr, err.Error())
			}
			document, err := materializeResolvedConfigDocument(resolved)
			if err != nil {
				return result, materializeFatal(stderr, err.Error())
			}
			if err := writeMaterializeDocument(*req.ResolvedConfigOut, document); err != nil {
				return result, materializeFatal(stderr, err.Error())
			}
			result.ResolvedConfigPath = *req.ResolvedConfigOut
			result.ResolvedConfigDocument = document
			fmt.Fprintln(stdout, "  ✓ wrote resolved-config (0 enabled recipe(s))")
		}
		if req.ResolvedHooksOut != nil {
			document := materializeResolvedHooksDocument(enabledAgentNames(manifest), []any{})
			if err := writeMaterializeDocument(*req.ResolvedHooksOut, document); err != nil {
				return result, materializeFatal(stderr, err.Error())
			}
			result.ResolvedHooksPath = *req.ResolvedHooksOut
			result.ResolvedHooksDocument = document
			fmt.Fprintln(stdout, "  ✓ wrote resolved-hooks (0 hook(s))")
		}
		return result, nil
	}

	manifestBindings := loadManifestBindings(manifest)

	binding := ResolveBindings(BindingRequest{
		EnabledRecipeIDs: enabledIDs,
		CatalogDir:       catalogDir,
		Bindings:         manifestBindings,
		ProjectRoot:      req.ProjectRoot,
		WriteWitness:     true,
	})
	if fatal := materializeBindingWarnings(stderr, binding); fatal != nil {
		return result, materializeFatal(stderr, fatal.Error())
	}
	for _, conflict := range binding.Conflicts {
		recipes := strings.Join(conflict.Recipes, ", ")
		if conflict.Severity == "fatal" {
			msg := fmt.Sprintf(
				"capability conflict: %s.id='%s' claimed by %s. Resolve manually in ai-specs.toml.",
				conflict.Type, conflict.ID, recipes,
			)
			return result, materializeFatal(stderr, msg)
		}
		warnf(stderr, "capability ambiguity: %s.id='%s' declared by %s. Add an explicit [[bindings]] entry to resolve.", conflict.Type, conflict.ID, recipes)
	}

	if err := stampReconcileDefaults(manifestPath, catalogDir, enabledIDs, stderr); err != nil {
		return result, materializeFatal(stderr, err.Error())
	}

	tagConflicts, err := PlanTagConflicts(catalogDir, enabledIDs)
	if err != nil {
		return result, materializeFatal(stderr, err.Error())
	}
	for _, conflict := range tagConflicts {
		recipes := strings.Join(conflict.Recipes, ", ")
		if conflict.Severity == "fatal" {
			warnf(stderr, "tag conflict: recipes %s share tag '%s' and declare an explicit conflicts_with. Review whether both should be enabled.", recipes, conflict.Tag)
		} else {
			warnf(stderr, "tag overlap: recipes %s share tag '%s' (same capability category).", recipes, conflict.Tag)
		}
	}

	primitives, err := shared.LoadRecipePrimitives(catalogDir, enabledIDs)
	if err != nil {
		return result, materializeFatal(stderr, err.Error())
	}
	if conflicts := shared.CheckPrimitiveConflicts(primitives); len(conflicts) > 0 {
		conflict := conflicts[0]
		msg := fmt.Sprintf(
			"recipe conflict: %s.id='%s' claimed by %s. Resolve manually in ai-specs.toml.",
			conflict.Type, conflict.ID, strings.Join(conflict.Recipes, ", "),
		)
		return result, materializeFatal(stderr, msg)
	}

	manifestMCP := mcpReadServers(manifest)
	recipeMCP, err := BuildRecipeMCP(catalogDir, enabledIDs, manifestMCP, &cliHome, stderr)
	if err != nil {
		return result, materializeFatal(stderr, err.Error())
	}

	commandSources, depSkillIDs, err := recipeCommandSources(catalogDir, enabled)
	if err != nil {
		return result, materializeFatal(stderr, err.Error())
	}
	for _, id := range depSkillIDs {
		expectedDepIDs[id] = true
	}

	planInput, roots := materializeOrphanInputs(req.ProjectRoot, cliHome, enabledIDs, materializeSortedSet(expectedDepIDs))
	if _, err := cleanOrphansNative(planInput, roots, req.ProjectRoot, stdout); err != nil {
		return result, materializeFatal(stderr, err.Error())
	}
	projectcache.RemoveRecipeCommandLeftovers(req.ProjectRoot, cliHome, nil, commandSources, stdout, stderr)

	for _, recipe := range enabled {
		fmt.Fprintf(stdout, "  ▸ recipe %s\n", recipe.ID)
		if recipe.Version != "" {
			warnf(stderr, "recipe '%s' has legacy version='%s' in ai-specs.toml; pins are ignored — sync uses the installed CLI catalog", recipe.ID, recipe.Version)
		}
		merge, err := PlanMergeConfig(MergeConfigRequest{
			RecipePath:   filepath.Join(catalogDir, recipe.ID, "recipe.toml"),
			ManifestPath: manifestPath,
			RecipeID:     recipe.ID,
		})
		if err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		for _, warning := range merge.Warnings {
			warnf(stderr, "%s", warning)
		}
		if merge.Error != "" {
			return result, materializeFatal(stderr, merge.Error)
		}
		// WU2 (T4) owns the per-recipe artifact actuation here: bundled/dep
		// skills, commands, templates, docs, hooks, runtime-hooks collection
		// and the worktree-flow gate-binary acquisition. The merged config is
		// consumed only by those steps, so WU1 stops at the config decision.
	}

	recipeMCPDocument, err := mcpJSONDumpsIndent2(recipeMCP)
	if err != nil {
		return result, materializeFatal(stderr, err.Error())
	}
	recipeMCPDocument += "\n"
	recipeMCPPath := ""
	recipeMCPTemporary := req.RecipeMCPOut == nil
	if req.RecipeMCPOut != nil {
		recipeMCPPath = *req.RecipeMCPOut
	} else {
		temp, err := materializeTempFile("ai-specs-recipe-mcp-*.json")
		if err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		recipeMCPPath = temp
	}
	if err := writeMaterializeDocument(recipeMCPPath, recipeMCPDocument); err != nil {
		return result, materializeFatal(stderr, err.Error())
	}
	result.RecipeMCPPath = recipeMCPPath
	result.RecipeMCPDocument = recipeMCPDocument
	fmt.Fprintf(stdout, "  ✓ wrote recipe MCP temp (%d server(s))\n", recipeMCP.len())
	if recipeMCPTemporary {
		fmt.Fprintf(stdout, "RECIPE_MCP_TEMP:%s\n", recipeMCPPath)
	}

	if req.ResolvedConfigOut != nil {
		resolved, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: req.ProjectRoot})
		if err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		resolved.Bindings = binding.Bindings
		document, err := materializeResolvedConfigDocument(resolved)
		if err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		if err := writeMaterializeDocument(*req.ResolvedConfigOut, document); err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		result.ResolvedConfigPath = *req.ResolvedConfigOut
		result.ResolvedConfigDocument = document
		result.ResolvedBindings = binding.Bindings
		fmt.Fprintf(stdout, "  ✓ wrote resolved-config (%d recipe(s))\n", len(resolved.Recipes))
	}

	if req.ResolvedHooksOut != nil {
		document := materializeResolvedHooksDocument(enabledAgentNames(manifest), resolvedHooks)
		if err := writeMaterializeDocument(*req.ResolvedHooksOut, document); err != nil {
			return result, materializeFatal(stderr, err.Error())
		}
		result.ResolvedHooksPath = *req.ResolvedHooksOut
		result.ResolvedHooksDocument = document
		fmt.Fprintf(stdout, "  ✓ wrote resolved-hooks (%d hook(s))\n", len(resolvedHooks))
	}

	return result, nil
}

// loadMaterializeManifest reads the manifest exactly as the Python
// load_recipes_from_manifest path does. A missing manifest is a fatal
// acquisition error.
func loadMaterializeManifest(path string) (*toml.Table, error) {
	return config.LoadManifest(path)
}

// materializeBindingWarnings prints the best-effort warnings carried by a
// binding resolution and returns the first resolution error as a fatal, or nil
// when resolution succeeded. This reproduces the Python order: envelope
// warnings are emitted before _bindings_from_envelope raises the first error.
func materializeBindingWarnings(stderr io.Writer, resolution shared.BindingResolution) error {
	for _, warning := range resolution.Warnings {
		warnf(stderr, "%s", warning)
	}
	if len(resolution.Errors) > 0 {
		return errors.New(resolution.Errors[0])
	}
	return nil
}

// stampReconcileDefaults mirrors stamp_recipe_reconcile_defaults: plan once
// with the shared authority, then write each planned stamp through the promoted
// shared writer. A write failure warns (never fails the sync), mirroring the
// Python authority's per-entry try/except.
func stampReconcileDefaults(manifestPath, catalogDir string, enabledIDs []string, stderr io.Writer) error {
	for _, entry := range PlanReconcileStamps(catalogDir, enabledIDs) {
		ordered, err := materializeStampOrdered(entry)
		if err != nil {
			warnReconcileStampFailure(stderr, catalogDir, entry.ID, err)
			continue
		}
		_, refusal, err := shared.ApplyRecipeConfigWrite(manifestPath, entry.ID, ordered)
		if err != nil {
			warnReconcileStampFailure(stderr, catalogDir, entry.ID, err)
			continue
		}
		if refusal != "" {
			warnReconcileStampFailure(stderr, catalogDir, entry.ID, errors.New(refusal))
			continue
		}
	}
	return nil
}

// warnReconcileStampFailure mirrors the Python warning shape:
// recipe '<name>': reconcile defaults not stamped (<Type>: <exc>).
func warnReconcileStampFailure(stderr io.Writer, catalogDir, recipeID string, err error) {
	name := recipeID
	if loaded := reconcileStampRecipeName(catalogDir, recipeID); loaded != "" {
		name = loaded
	}
	warnf(stderr, "recipe '%s': reconcile defaults not stamped (RuntimeError: %v)", name, err)
}

// cleanOrphansNative runs the shared orphan plan/apply through the root
// CleanOrphans boundary, which owns the lock read, the success lines and the
// stale-lock prune. The lock path is the project lock.
func cleanOrphansNative(planInput shared.OrphanPlanInput, roots shared.OrphanApplyRoots, projectRoot string, stdout io.Writer) (shared.OrphanApplyOutcome, error) {
	lockPath := filepath.Join(projectRoot, "ai-specs", ".ai-specs.lock")
	return CleanOrphans(OrphanCleanupInput{
		PlanInput: planInput,
		Roots:     roots,
		LockPath:  lockPath,
		Output:    stdout,
	}, os.RemoveAll)
}

// materializeRecipeIDs returns the enabled recipe ids in manifest order.
func materializeRecipeIDs(recipes []materializeRecipe) []string {
	out := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		out = append(out, recipe.ID)
	}
	return out
}

// materializeStringSet build a membership set from a slice.
func materializeStringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

// materializeSortedSet returns the sorted keys of a membership set.
func materializeSortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// writeMaterializeDocument writes one serialized document byte for byte.
func writeMaterializeDocument(path, document string) error {
	return os.WriteFile(path, []byte(document), 0o644)
}

// materializeTempFile creates an empty temp file with the Python mkstemp naming
// shape and returns its path.
func materializeTempFile(pattern string) (string, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	return name, nil
}
