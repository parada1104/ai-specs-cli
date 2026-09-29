package doctor

// Native port of doctor._check_repo_topology (check 14). The topology decision
// is owned once by internal/target (ProjectRepoTopology); this file keeps only
// the doctor check and its message building.

import (
	"fmt"

	"ai-specs.dev/ai-specs/internal/target"
)

// topologySourceDefault is the target package's "default" source value; the
// legacy alias only reports while worktree-flow is enabled.
const topologySourceDefault = "default"

// checkRepoTopology is doctor._check_repo_topology.
func (d *Doctor) checkRepoTopology() {
	if !isFile(d.manifestPath()) {
		return
	}
	data := d.manifestData()
	if data == nil {
		return
	}

	wfEnabled := false
	if recipes, ok := data.Table("recipes"); ok && recipes != nil {
		if wf, ok := recipes.Table("worktree-flow"); ok && wf != nil {
			if enabled, isBool := wf.Bool("enabled"); isBool {
				wfEnabled = enabled
			}
		}
	}

	topo := target.ProjectRepoTopology(d.Root, data)
	// A project-owned value always reports; the legacy recipe alias only
	// reports while worktree-flow is enabled (previous behavior).
	if topo.Source == topologySourceDefault && !wfEnabled {
		return
	}

	d.add(INFO, "repo-topology",
		fmt.Sprintf("%s (via %s; source: %s; %d initialized submodule(s))",
			topo.Resolved, topo.Via, topo.Source, len(topo.Submodules)))
	if topo.Deprecation != "" {
		d.add(WARN, "repo-topology-deprecated", topo.Deprecation)
	}
}
