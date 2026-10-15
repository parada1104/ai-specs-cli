package doctor

import (
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/sync"
)

// checkBriefRenderPolicy is doctor._check_brief_render_policy.
func (d *Doctor) checkBriefRenderPolicy() {
	d.loadManifest()
	manifest := d.manifest.data
	if manifest == nil || len(manifest.Keys()) == 0 {
		return
	}
	enabled, err := briefRenderEnabled(manifest)
	if err != nil {
		d.add(ERROR, "brief-render", err.Error(), "use true or false in lowercase")
		return
	}
	if enabled {
		return
	}
	agents := filepath.Join(d.Root, "AGENTS.md")
	if isFile(agents) {
		d.add(INFO, "brief-render", "managed AGENTS.md rendering disabled ([brief].render = false)")
		if data, err := os.ReadFile(agents); err == nil && strings.Contains(string(data), "<!-- ai-specs:runtime-brief -->") {
			d.add(INFO, "brief-render-marker",
				"runtime-brief marker present (redundant with render = false)")
		}
	}
	// The probe is native since GO-07 S18: the resolved-config chain, the
	// render and the classification all run in process. A failure declares
	// nothing, exactly like the op's null result.
	dead, err := sync.BriefHasDeadRecipeFragments(sync.BriefViewRequest{ProjectRoot: d.Root, AISpecsHome: d.Home})
	if err != nil || !dead {
		return
	}
	d.add(WARN, "brief-fragments-unused",
		"enabled recipes declare [provides.brief] but render = false",
		"remove unused recipes or set [brief].render = true")
}

// checkBriefProvenance is doctor._check_brief_provenance.
func (d *Doctor) checkBriefProvenance() {
	if !isFile(d.manifestPath()) {
		return
	}
	if d.briefRenderDisabled() {
		d.add(INFO, "brief-provenance",
			"runtime brief ownership: disabled ([brief].render = false)",
			"set [brief].render = true when ai-specs should render AGENTS.md")
		return
	}
	// The probe is native since GO-07 S18 (the bridge's brief ops are gone):
	// the same resolved-config chain, render and classifier run in process. Any
	// failure keeps the legacy "undetermined" answer.
	state := "undetermined"
	if view, err := sync.BuildBriefView(sync.BriefViewRequest{ProjectRoot: d.Root, AISpecsHome: d.Home}); err == nil && view.State != "" {
		state = view.State
	}
	switch state {
	case "missing":
		d.add(INFO, "brief-provenance", "runtime brief ownership: missing",
			"ai-specs sync will create AGENTS.md")
	case "managed_current":
		d.add(OK, "brief-provenance", "runtime brief ownership: managed_current")
	case "managed_stale":
		d.add(INFO, "brief-provenance", "runtime brief ownership: managed_stale",
			"ai-specs sync will refresh the untouched brief")
	case "marker":
		d.add(INFO, "brief-provenance", "runtime brief ownership: marker (user-owned)",
			"remove the marker only if ai-specs should manage AGENTS.md")
	case "user_modified":
		d.add(WARN, "brief-provenance",
			"runtime brief ownership: user_modified; preserving existing file",
			"ai-specs sync --adopt-brief or add the runtime-brief marker")
	case "untracked":
		d.add(WARN, "brief-provenance",
			"runtime brief ownership: untracked; preserving existing file",
			"ai-specs sync --adopt-brief or add the runtime-brief marker")
	default:
		d.add(WARN, "brief-provenance",
			"runtime brief ownership: "+state+"; preserving existing file",
			"inspect the lock and target, or add the runtime-brief marker")
	}
}
