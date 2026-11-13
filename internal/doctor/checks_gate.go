package doctor

// Native port of doctor._check_worktree_gate (check 13 of the roster).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkWorktreeGate is doctor._check_worktree_gate.
//
// Severity table:
//
//	OK    Go binary resolved, version matches the stamp, selftest passes
//	ERROR gate_impl=bash configured (retired value)
//	ERROR no usable binary (auto and go both fail open)
//	WARN  binary version does not match the stamped version
//	ERROR digest mismatch recorded at the last acquisition
//	INFO  leftover worktree-gate-legacy.sh on disk (inert; manual rm)
//
// A fail-open gate is invisible by construction, so this ERROR is the only
// place a user can discover it.
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
	// Python identity: `wf.get("enabled") is not True` -- only the bool true
	// enables the check (a truthy non-bool is not True).
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
	stampedVersion := ""
	stampedImpl := ""
	if isFile(launcher) {
		if raw, err := os.ReadFile(launcher); err == nil {
			for _, line := range pySplitLines(string(raw)) {
				switch {
				case strings.HasPrefix(line, `stamped_gate_version="`):
					if parts := strings.SplitN(line, `"`, 3); len(parts) >= 2 {
						stampedVersion = parts[1]
					}
				case strings.HasPrefix(line, `stamped_gate_impl="`):
					if parts := strings.SplitN(line, `"`, 3); len(parts) >= 2 {
						stampedImpl = parts[1]
					}
				}
			}
		}
	}

	// Recorded digest mismatch from the last acquisition -> ERROR.
	mismatchPath := gateDigestMismatchRecordPath(d.Home)
	if isFile(mismatchPath) {
		mismatchText := ""
		if raw, err := os.ReadFile(mismatchPath); err == nil {
			mismatchText = strings.TrimSpace(string(raw))
		}
		if mismatchText == "" {
			mismatchText = "gate binary digest mismatch recorded at last acquisition"
		}
		d.add(ERROR, "worktree-gate", mismatchText,
			"run ai-specs sync to re-acquire; the rejected artifact was never executed")
		return
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

	goos, goarch := detectGatePlatform()
	binary := gateCacheBinPath(d.Home, goos, goarch)
	if !isFile(binary) || !isExecutable(binary) {
		d.add(ERROR, "worktree-gate",
			fmt.Sprintf("gate_impl=%s and no usable binary at %s; the gate is failing open", impl, binary),
			"run ai-specs sync or ai-specs sync --refresh-gates")
		return
	}

	version := gateBinaryVersion(binary)
	selftest := gateSelftest(binary)
	if selftest != "" {
		d.add(ERROR, "worktree-gate",
			fmt.Sprintf("gate binary at %s failed --selftest: %s; the gate is not enforcing", binary, selftest),
			"run ai-specs sync to re-acquire or re-build")
		return
	}

	if stampedVersion != "" && version != stampedVersion {
		d.add(WARN, "worktree-gate",
			fmt.Sprintf("gate binary version %s does not match the stamped version %s", version, stampedVersion),
			"run ai-specs sync to re-acquire for the installed CLI version")
		return
	}

	sizeKB := gateCacheSize(d.Home) / 1024
	d.add(OK, "worktree-gate",
		fmt.Sprintf("Go binary %s at %s; selftest passed (cache %d KiB)", version, binary, sizeKB))
}
