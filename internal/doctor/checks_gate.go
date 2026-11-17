package doctor

// Native port of doctor._check_worktree_gate (check 13 of the roster).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkWorktreeGate verifies the installed CLI's worktree-gate route. The
// check retains its legacy roster id and fail-open ERROR posture.
func (d *Doctor) checkWorktreeGate() {
	manifest := d.manifestPath()
	if !isFile(manifest) {
		return
	}
	data := d.manifestData()
	if data == nil {
		return
	}
	recipes, ok := data.Table("recipes")
	if !ok || recipes == nil {
		return
	}
	wf, ok := recipes.Table("worktree-flow")
	if !ok || wf == nil {
		return
	}
	enabled, isBool := wf.Bool("enabled")
	if !isBool || !enabled {
		return
	}
	cfg, _ := wf.Table("config")
	var implRaw any
	if cfg != nil {
		implRaw, _ = cfg.Get("gate_impl")
	}
	impl := pyStrOr(implRaw, "auto")
	launcher := filepath.Join(d.Root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate.sh")
	stampedImpl := ""
	if isFile(launcher) {
		if raw, err := os.ReadFile(launcher); err == nil {
			for _, line := range pySplitLines(string(raw)) {
				if strings.HasPrefix(line, `stamped_gate_impl="`) {
					if parts := strings.SplitN(line, `"`, 3); len(parts) >= 2 {
						stampedImpl = parts[1]
					}
				}
			}
		}
	}
	leftover := filepath.Join(d.Root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate-legacy.sh")
	if isFile(leftover) {
		d.add(INFO, "worktree-gate",
			"leftover worktree-gate-legacy.sh is inert and is not a governed asset",
			"rm ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh")
	}
	if impl == "bash" || stampedImpl == "bash" {
		d.add(ERROR, "worktree-gate",
			"gate_impl=bash is retired; set auto or go, then ai-specs sync",
			"doctor is read-only; set gate_impl to auto or go, then run ai-specs sync")
		return
	}

	binary := installedCLIPath(d.Home)
	if !isExecutable(binary) {
		d.add(ERROR, "worktree-gate",
			fmt.Sprintf("ai-specs CLI is missing or not executable at %s; the gate is failing open", binary),
			"run ai-specs sync")
		return
	}
	if failure := cliWorktreeGateSelftest(binary); failure != "" {
		d.add(ERROR, "worktree-gate",
			fmt.Sprintf("ai-specs CLI at %s failed worktree-gate --selftest: %s; the gate is failing open", binary, failure),
			"run ai-specs sync")
		return
	}
	d.add(OK, "worktree-gate",
		fmt.Sprintf("CLI at %s serves worktree-gate; selftest passed", binary))
}
