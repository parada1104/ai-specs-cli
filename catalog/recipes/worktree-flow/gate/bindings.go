package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

// Capability binding resolution and capability conflict grading, moved from the
// Python authority with the same order and the same outcomes:
//
//   - manifestBinding is one explicit [[bindings]] table (capability -> recipe).
//   - capabilityConflict is one graded capability conflict, sorted recipes and a
//     severity of "fatal" or "warning".
//   - bindingResolution is the single JSON object emitted on stdout. It is one
//     result envelope, not a verdict: errors are data, and the calling bridge
//     decides policy. The command exits 0 whenever the process itself succeeded.
//
// resolveBindings mirrors recipe-materialize.resolve_bindings step 1 (validate
// explicit bindings, first failure aborts) and step 2 (auto-bind a capability a
// single enabled recipe declares). Explicit bindings override auto-bind.
// capabilityConflicts mirrors recipe-conflicts.check_capability_conflicts: a
// duplicate explicit binding is fatal, and a capability with more than one
// provider and no explicit binding is a warning naming every provider. The two
// graders are independent, so a duplicate binding is reported both as the
// resolution error that aborted step 1 and as the fatal conflict.

// manifestBinding is one explicit capability->recipe binding from the manifest.
type manifestBinding struct {
	Capability string `json:"capability"`
	Recipe     string `json:"recipe"`
}

// capabilityConflict is one graded capability conflict. recipes is always
// sorted and deduplicated so the JSON is deterministic.
type capabilityConflict struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

// bindingResolution is the stdout contract. All three fields are present on
// every successful run; an absent result is an empty object/list, never null.
type bindingResolution struct {
	Bindings  map[string]string    `json:"bindings"`
	Conflicts []capabilityConflict `json:"conflicts"`
	Errors    []string             `json:"errors"`
}

// capabilityIndex is the one shared read of the enabled recipes' declarations.
// recipeCaps holds the capabilities of every readable recipe, and capOrder lists
// every declared capability in first-seen order with its providers, so conflict
// output does not depend on Go map iteration.
type capabilityIndex struct {
	enabled    map[string]bool
	recipeCaps map[string][]string
	providers  map[string][]string
	capOrder   []string
}

// buildCapabilityIndex indexes the acquired declarations. A recipe absent from
// capsByRecipe was unreadable and contributes nothing: it stays enabled (so an
// explicit binding to it fails as undeclared, not as unknown) but provides no
// capability.
func buildCapabilityIndex(enabledIDs []string, capsByRecipe map[string][]string) capabilityIndex {
	idx := capabilityIndex{
		enabled:    make(map[string]bool, len(enabledIDs)),
		recipeCaps: make(map[string][]string, len(enabledIDs)),
		providers:  make(map[string][]string),
	}
	for _, rid := range enabledIDs {
		idx.enabled[rid] = true
		caps, readable := capsByRecipe[rid]
		if !readable {
			continue
		}
		idx.recipeCaps[rid] = caps
		for _, cap := range caps {
			if _, seen := idx.providers[cap]; !seen {
				idx.capOrder = append(idx.capOrder, cap)
			}
			idx.providers[cap] = append(idx.providers[cap], rid)
		}
	}
	return idx
}

// resolveBindings validates the explicit bindings, then auto-binds every
// capability exactly one enabled recipe declares. The first invalid explicit
// binding aborts resolution with a single deterministic error and no bindings,
// matching the exception the Python authority raises.
func resolveBindings(idx capabilityIndex, explicit []manifestBinding) (map[string]string, []string) {
	bindings := make(map[string]string, len(explicit))
	seen := make(map[string]bool, len(explicit))
	for _, binding := range explicit {
		if seen[binding.Capability] {
			return nil, []string{fmt.Sprintf("duplicate explicit binding for capability '%s'", binding.Capability)}
		}
		seen[binding.Capability] = true
		if !idx.enabled[binding.Recipe] {
			return nil, []string{fmt.Sprintf(
				"explicit binding for capability '%s' references disabled/unknown recipe '%s'",
				binding.Capability, binding.Recipe)}
		}
		if !slices.Contains(idx.recipeCaps[binding.Recipe], binding.Capability) {
			return nil, []string{fmt.Sprintf(
				"explicit binding for capability '%s' references recipe '%s' which does not declare that capability",
				binding.Capability, binding.Recipe)}
		}
		bindings[binding.Capability] = binding.Recipe
	}

	for _, cap := range idx.capOrder {
		if _, bound := bindings[cap]; bound {
			continue
		}
		if providers := idx.providers[cap]; len(providers) == 1 {
			bindings[cap] = providers[0]
		}
	}
	return bindings, nil
}

// capabilityConflicts grades capability conflicts. A duplicate explicit binding
// is fatal and returned alone, mirroring the early return of the Python
// authority; otherwise every capability with two or more providers and no
// explicit binding is a warning naming all providers.
func capabilityConflicts(idx capabilityIndex, explicit []manifestBinding) []capabilityConflict {
	bound := make(map[string]bool, len(explicit))
	boundRecipe := make(map[string]string, len(explicit))
	for _, binding := range explicit {
		if bound[binding.Capability] {
			return []capabilityConflict{{
				Type:     "capability",
				ID:       binding.Capability,
				Recipes:  sortedUnique([]string{boundRecipe[binding.Capability], binding.Recipe}),
				Severity: "fatal",
			}}
		}
		bound[binding.Capability] = true
		boundRecipe[binding.Capability] = binding.Recipe
	}

	var conflicts []capabilityConflict
	for _, cap := range idx.capOrder {
		if bound[cap] {
			continue
		}
		providers := idx.providers[cap]
		if len(providers) > 1 {
			conflicts = append(conflicts, capabilityConflict{
				Type:     "capability",
				ID:       cap,
				Recipes:  sortedUnique(providers),
				Severity: "warning",
			})
		}
	}
	return conflicts
}

// gradeBindings is the pure decision core: index the acquired declarations, run
// both graders, and normalize the result so the JSON contract never emits null.
func gradeBindings(enabledIDs []string, capsByRecipe map[string][]string, explicit []manifestBinding) bindingResolution {
	idx := buildCapabilityIndex(enabledIDs, capsByRecipe)
	bindings, resolveErrors := resolveBindings(idx, explicit)
	out := bindingResolution{
		Bindings:  bindings,
		Conflicts: capabilityConflicts(idx, explicit),
		Errors:    resolveErrors,
	}
	if out.Bindings == nil {
		out.Bindings = map[string]string{}
	}
	if out.Conflicts == nil {
		out.Conflicts = []capabilityConflict{}
	}
	if out.Errors == nil {
		out.Errors = []string{}
	}
	return out
}

// parseManifestBindings decodes the explicit [[bindings]] tables passed as JSON.
// Parsing is as lenient as toml-read.read_bindings: only the capability and
// recipe strings are read and unknown keys are ignored. A blank payload is no
// binding.
func parseManifestBindings(raw string) ([]manifestBinding, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	var list []manifestBinding
	if err := json.Unmarshal([]byte(text), &list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}

// bindingsOptions is the parsed --resolve-bindings flag surface.
type bindingsOptions struct {
	catalogDir string
	recipeIDs  []string
	bindings   string
}

// runResolveBindings is the --resolve-bindings command: acquire the enabled
// recipes' capabilities through the TOML seam, grade bindings and conflicts, and
// print one JSON object on stdout. Resolution errors are data (exit 0); only a
// process-level failure — unusable flags or an unavailable parser — exits 2.
func runResolveBindings(opts bindingsOptions, stdout, stderr io.Writer) int {
	if opts.catalogDir == "" {
		fmt.Fprintln(stderr, "worktree-gate: --resolve-bindings requires --catalog-dir")
		return 2
	}
	explicit, err := parseManifestBindings(opts.bindings)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-bindings: invalid --bindings JSON: %v\n", err)
		return 2
	}
	capsByRecipe, err := loadRecipeCapabilities(opts.catalogDir, opts.recipeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-bindings: %v\n", err)
		return 2
	}
	payload, err := json.Marshal(gradeBindings(opts.recipeIDs, capsByRecipe, explicit))
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-bindings: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

// sortedUnique returns values as a sorted, deduplicated copy. It is the shape
// both conflict graders emit, so a set-like recipe group has one stable order.
func sortedUnique(values []string) []string {
	unique := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if unique[value] {
			continue
		}
		unique[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
