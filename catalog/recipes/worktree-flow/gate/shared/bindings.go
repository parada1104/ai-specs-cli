package shared

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Capability binding resolution and capability conflict grading. This is the
// pure decision core extracted from the gate main package (GO-07 S13 PR1) so the
// gate binary's --resolve-bindings surface and the root native sync authority
// share one implementation. Acquisition (reading recipe.toml) stays per-side;
// only the decision lives here.
//
// The original Python authority (recipe-materialize.resolve_bindings) had the
// same order and the same outcomes:
//
//   - ManifestBinding is one explicit [[bindings]] table (capability -> recipe).
//   - CapabilityConflict is one graded capability conflict, sorted recipes and a
//     severity of "fatal" or "warning".
//   - BindingResolution is the single JSON object emitted on stdout by the gate
//     command. It is one result envelope, not a verdict: errors are data, and
//     the calling bridge decides policy. The command exits 0 whenever the
//     process itself succeeded.
//
// ResolveBindings mirrors recipe-materialize.resolve_bindings step 1 (validate
// explicit bindings, first failure aborts) and step 2 (auto-bind a capability a
// single enabled recipe declares). Explicit bindings override auto-bind.
// CapabilityConflicts mirrors recipe-conflicts.check_capability_conflicts: a
// duplicate explicit binding is fatal, and a capability with more than one
// provider and no explicit binding is a warning naming every provider. The two
// graders are independent, so a duplicate binding is reported both as the
// resolution error that aborted step 1 and as the fatal conflict.

// ManifestBinding is one explicit capability->recipe binding from the manifest.
type ManifestBinding struct {
	Capability string `json:"capability"`
	Recipe     string `json:"recipe"`
}

// CapabilityConflict is one graded capability conflict. Recipes is always
// sorted and deduplicated so the JSON is deterministic.
type CapabilityConflict struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

// BindingResolution is the stdout contract. All three core fields are present on
// every successful run; an absent result is an empty object/list, never null.
// Warnings is the only optional field: it carries best-effort side effects of the
// command itself (the durable witness write), so a write failure is reported
// without demoting or masking a resolution error.
type BindingResolution struct {
	Bindings  map[string]string    `json:"bindings"`
	Conflicts []CapabilityConflict `json:"conflicts"`
	Errors    []string             `json:"errors"`
	Warnings  []string             `json:"warnings,omitempty"`
}

// CapabilityIndex is the one shared read of the enabled recipes' declarations.
// recipeCaps holds the capabilities of every readable recipe, and capOrder lists
// every declared capability in first-seen order with its providers, so conflict
// output does not depend on Go map iteration.
type CapabilityIndex struct {
	enabled    map[string]bool
	recipeCaps map[string][]string
	providers  map[string][]string
	capOrder   []string
}

// BuildCapabilityIndex indexes the acquired declarations. A recipe absent from
// capsByRecipe was unreadable and contributes nothing: it stays enabled (so an
// explicit binding to it fails as undeclared, not as unknown) but provides no
// capability.
func BuildCapabilityIndex(enabledIDs []string, capsByRecipe map[string][]string) CapabilityIndex {
	idx := CapabilityIndex{
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

// ResolveBindings validates the explicit bindings, then auto-binds every
// capability exactly one enabled recipe declares. The first invalid explicit
// binding aborts resolution with a single deterministic error and no bindings,
// matching the exception the Python authority raises.
func ResolveBindings(idx CapabilityIndex, explicit []ManifestBinding) (map[string]string, []string) {
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

// CapabilityConflicts grades capability conflicts. A duplicate explicit binding
// is fatal and returned alone, mirroring the early return of the Python
// authority; otherwise every capability with two or more providers and no
// explicit binding is a warning naming all providers.
func CapabilityConflicts(idx CapabilityIndex, explicit []ManifestBinding) []CapabilityConflict {
	bound := make(map[string]bool, len(explicit))
	boundRecipe := make(map[string]string, len(explicit))
	for _, binding := range explicit {
		if bound[binding.Capability] {
			return []CapabilityConflict{{
				Type:     "capability",
				ID:       binding.Capability,
				Recipes:  SortedUnique([]string{boundRecipe[binding.Capability], binding.Recipe}),
				Severity: "fatal",
			}}
		}
		bound[binding.Capability] = true
		boundRecipe[binding.Capability] = binding.Recipe
	}

	var conflicts []CapabilityConflict
	for _, cap := range idx.capOrder {
		if bound[cap] {
			continue
		}
		providers := idx.providers[cap]
		if len(providers) > 1 {
			conflicts = append(conflicts, CapabilityConflict{
				Type:     "capability",
				ID:       cap,
				Recipes:  SortedUnique(providers),
				Severity: "warning",
			})
		}
	}
	return conflicts
}

// GradeBindings is the pure decision core: index the acquired declarations, run
// both graders, and normalize the result so the JSON contract never emits null.
func GradeBindings(enabledIDs []string, capsByRecipe map[string][]string, explicit []ManifestBinding) BindingResolution {
	idx := BuildCapabilityIndex(enabledIDs, capsByRecipe)
	bindings, resolveErrors := ResolveBindings(idx, explicit)
	out := BindingResolution{
		Bindings:  bindings,
		Conflicts: CapabilityConflicts(idx, explicit),
		Errors:    resolveErrors,
	}
	if out.Bindings == nil {
		out.Bindings = map[string]string{}
	}
	if out.Conflicts == nil {
		out.Conflicts = []CapabilityConflict{}
	}
	if out.Errors == nil {
		out.Errors = []string{}
	}
	return out
}

// ParseManifestBindings decodes the explicit [[bindings]] tables passed as JSON.
// Parsing is as lenient as toml-read.read_bindings: only the capability and
// recipe strings are read and unknown keys are ignored. A blank payload is no
// binding.
func ParseManifestBindings(raw string) ([]ManifestBinding, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	var list []ManifestBinding
	if err := json.Unmarshal([]byte(text), &list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}
