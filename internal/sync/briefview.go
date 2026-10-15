package sync

// Read-only brief view for consumers outside the sync pipeline (GO-07 S18).
//
// The doctor's brief checks used to run through the Python bridge
// (internal/doctor/bridge.py): its `brief_state` and `brief_dead_fragments` ops
// rebuilt the resolved-config graph with recipe-materialize.py and rendered the
// would-write brief with agents-render.py / brief-render-policy.py. This file
// is that chain in process, on top of the pieces the pipeline already owns (the
// projection, the S14b catalog enrichment, the binding authority and the
// render/classification helpers), so a read-only consumer never spawns Python
// for it.
//
// Two chains, deliberately kept apart because Python keeps them apart:
//
//   - BuildBriefView is the bridge's `_resolved_for_brief`: the projection, the
//     catalog defaults, the brief fragments, the auto-bindings under the
//     enabled guard, then `_render_lines` and `brief_effective_state`.
//   - BriefHasDeadRecipeFragments is the bridge's `brief_dead_fragments`: the
//     projection plus the brief fragments only — no catalog defaults and no
//     auto-bindings — then `has_dead_recipe_fragments`.
//
// Neither function writes anything: no lock write, no AGENTS.md write, no
// cache write.

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"

	"ai-specs.dev/ai-specs/internal/config"
	shared "ai-specs.dev/worktree-gate/shared"
)

// BriefViewRequest is the input of one read-only brief view: the project root
// the manifest is read from and the CLI home the catalog is read from.
type BriefViewRequest struct {
	ProjectRoot string
	AISpecsHome string
}

// BriefView is one read-only brief projection: exactly the bytes sync would
// write to AGENTS.md and the state sync's own classifier would act on.
type BriefView struct {
	WouldWrite []byte
	State      string
}

// errNotObjectDocument reports a projection that is not a JSON object. The
// shared core always produces one; the guard keeps the decoder honest.
var errNotObjectDocument = errors.New("the resolved-config projection is not an object")

// BuildBriefView renders the brief this project would get and classifies it,
// exactly as the bridge's `brief_state` op did. Any failure is returned as an
// error; the caller maps that onto the legacy degradation (the doctor reports
// the state as "undetermined").
func BuildBriefView(req BriefViewRequest) (BriefView, error) {
	tomlPath := materializeManifestPath(req.ProjectRoot)
	catalogDir := filepath.Join(req.AISpecsHome, "catalog", "recipes")

	resolved, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: req.ProjectRoot})
	if err != nil {
		return BriefView{}, err
	}
	doc, err := orderedResolvedDocument(resolved)
	if err != nil {
		return BriefView{}, err
	}
	mergeCatalogDefaultsIntoResolved(doc, resolved.Enabled, catalogDir)
	attachBriefFragmentsToResolved(doc, resolved.Enabled, catalogDir)
	if len(resolved.Enabled) > 0 {
		if err := attachAutoBindings(doc, resolved.Enabled, catalogDir, req.ProjectRoot); err != nil {
			return BriefView{}, err
		}
	}

	manifest, err := config.LoadManifest(tomlPath)
	if err != nil {
		return BriefView{}, err
	}
	wouldWrite := RenderAgentsMarkdown(manifest, mcpPlainMap(doc))
	if wouldWrite == nil {
		// _render_lines validates the [brief] mode keys first and raises; the
		// op answered any exception as "undetermined".
		brief, _ := manifest.Table("brief")
		if verr := ValidateBriefModes(brief); verr != nil {
			return BriefView{}, verr
		}
		return BriefView{}, errors.New("the brief could not be rendered")
	}
	return BriefView{
		WouldWrite: wouldWrite,
		State: briefEffectiveState(
			tomlPath,
			filepath.Join(req.ProjectRoot, "AGENTS.md"),
			wouldWrite,
			briefLockPath(tomlPath),
			nil,
		),
	}, nil
}

// BriefHasDeadRecipeFragments is the bridge's `brief_dead_fragments` op: true
// when an enabled recipe declares a truthy [provides.brief] section. Any
// failure is returned as an error; the caller emits no check for it, exactly
// like the op's null result.
func BriefHasDeadRecipeFragments(req BriefViewRequest) (bool, error) {
	catalogDir := filepath.Join(req.AISpecsHome, "catalog", "recipes")
	resolved, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: req.ProjectRoot})
	if err != nil {
		return false, err
	}
	doc, err := orderedResolvedDocument(resolved)
	if err != nil {
		return false, err
	}
	attachBriefFragmentsToResolved(doc, resolved.Enabled, catalogDir)
	return briefHasDeadRecipeFragments(doc), nil
}

// briefHasDeadRecipeFragments is has_dead_recipe_fragments
// (lib/_internal/brief-render-policy.py:44): for every enabled recipe, an
// object `brief_fragments` with any truthy section value. An absent entry, an
// absent key and an empty section all declare nothing; a `brief_fragments`
// value that is not an object declares nothing either, exactly like Python's
// `isinstance(frags, dict)` skip. The projection guarantees object recipe
// entries (the Python authority would raise on a truthy non-object there, which
// its caller turns into no check); this predicate stays total and treats that
// unreachable shape as declaring nothing.
func briefHasDeadRecipeFragments(doc *mcpObj) bool {
	if doc == nil {
		return false
	}
	enabledValue, _ := doc.get("enabled")
	recipesValue, _ := doc.get("recipes")
	recipes, _ := recipesValue.(*mcpObj)
	for _, rid := range mcpStringList(enabledValue) {
		if recipes == nil {
			continue
		}
		entryValue, ok := recipes.get(rid)
		if !ok {
			continue
		}
		entry, isObj := entryValue.(*mcpObj)
		if !isObj || entry == nil {
			continue
		}
		fragmentsValue, ok := entry.get("brief_fragments")
		if !ok {
			continue
		}
		fragments, isObj := fragmentsValue.(*mcpObj)
		if !isObj || fragments == nil {
			continue
		}
		for _, section := range fragments.keys {
			if pyTruthy(fragments.vals[section]) {
				return true
			}
		}
	}
	return false
}

// attachAutoBindings is the bridge's auto-binding step: re-read the manifest for
// its explicit [[bindings]], resolve them through the same authority the
// pipeline uses, and replace the document's bindings when the resolution
// produced any. A resolution error is a failure, because Python's
// resolve_bindings raises there and the op turns any exception into
// "undetermined"; so is an unreadable manifest, because Python's
// load_bindings_from_manifest raises.
func attachAutoBindings(doc *mcpObj, enabled []string, catalogDir, projectRoot string) error {
	manifest, err := config.LoadManifest(materializeManifestPath(projectRoot))
	if err != nil {
		return err
	}
	resolution := ResolveBindings(BindingRequest{
		EnabledRecipeIDs: enabled,
		CatalogDir:       catalogDir,
		Bindings:         loadManifestBindings(manifest),
	})
	if len(resolution.Errors) > 0 {
		return errors.New(resolution.Errors[0])
	}
	if len(resolution.Bindings) == 0 {
		return nil
	}
	keys := make([]string, 0, len(resolution.Bindings))
	for key := range resolution.Bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bindings := newMCPObj()
	for _, key := range keys {
		bindings.set(key, resolution.Bindings[key])
	}
	doc.set("bindings", bindings)
	return nil
}

// orderedResolvedDocument marshals the projection and decodes it into the
// ordered model — the same JSON round trip the pipeline performs, so the values
// carry the gate envelope's rendering (a manifest 1.0 is 1 there too while the
// catalog values keep their own types).
func orderedResolvedDocument(resolved shared.ResolvedConfig) (*mcpObj, error) {
	raw, err := json.Marshal(resolved)
	if err != nil {
		return nil, err
	}
	decoded, err := mcpParseJSONOrdered(raw)
	if err != nil {
		return nil, err
	}
	doc, ok := decoded.(*mcpObj)
	if !ok {
		return nil, errNotObjectDocument
	}
	return doc, nil
}

// mcpPlainMap converts the ordered document into the plain map domain
// renderLines consumes. It converts structurally rather than through text JSON
// so value kinds survive: a wide integer stays a big integer and a TOML
// date/time stays typed instead of degrading to a float64 or a string.
func mcpPlainMap(doc *mcpObj) map[string]any {
	if doc == nil {
		return map[string]any{}
	}
	out := make(map[string]any, doc.len())
	for _, key := range doc.keys {
		out[key] = mcpPlainValue(doc.vals[key])
	}
	return out
}

// mcpPlainValue converts one ordered value structurally (see mcpPlainMap).
func mcpPlainValue(v any) any {
	switch x := v.(type) {
	case *mcpObj:
		return mcpPlainMap(x)
	case []any:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = mcpPlainValue(e)
		}
		return arr
	default:
		return v
	}
}

// mcpStringList returns the string elements of a JSON array value in order,
// skipping non-strings (Python iterates the list and compares each element, so
// a non-string id simply matches no recipe entry).
func mcpStringList(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, isStr := item.(string); isStr {
			out = append(out, s)
		}
	}
	return out
}
