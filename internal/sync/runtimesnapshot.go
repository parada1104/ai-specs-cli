package sync

// CLI-served recipe runtime (cli-served-recipes T1/T2, S1/S2).
//
// The worktree-flow worktree-gate hooks are no longer distributed as a .sh
// launcher. Sync records the resolved worktree-flow runtime config in the
// project cache (projectcache.WriteRuntimeSnapshot) and every renderer wires the
// CLI verb `ai-specs worktree-gate --hook <id>` instead of a materialized
// script. The CLI reads only that snapshot, never ai-specs.toml (S2), and every
// other hook keeps its script until its own phase (F2/F3).

import (
	"fmt"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
	shared "ai-specs.dev/worktree-gate/shared"
)

// worktreeGateLauncherScript is the retired launcher the worktree-flow hooks
// declare. A hook declaring it is served by the CLI, not materialized.
const worktreeGateLauncherScript = "worktree-gate.sh"

// worktreeGateHookVerb is the CLI verb the renderers invoke for the gate.
const worktreeGateHookVerb = "worktree-gate"

// cliServedHookCommand is the shell command that serves this hook, or "" when
// the hook keeps its materialized script. Only worktree-flow's worktree-gate
// hooks are CLI-served in F1.
func cliServedHookCommand(recipeID string, hook *schema.RuntimeHook) string {
	if recipeID != "worktree-flow" || filepath.Base(hook.Script) != worktreeGateLauncherScript {
		return ""
	}
	// The launcher's resolution order, kept machine-independent: the CLI that
	// ran sync (AI_SPECS_HOME/dist) first, `ai-specs` on PATH second. A missing
	// CLI fails open with exit 0, exactly like the launcher did.
	//
	// The id is shell-quoted: it is recipe data, so an id with spaces or shell
	// metacharacters must reach the CLI as one argument instead of as source.
	return `cli="${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs"; ` +
		`[ -x "$cli" ] || cli=ai-specs; ` +
		`command -v "$cli" >/dev/null 2>&1 || exit 0; ` +
		`exec "$cli" ` + worktreeGateHookVerb + ` --hook ` + shellQuote(hook.ID)
}

// shellQuote renders value as one POSIX sh single-quoted word.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// recipeServesCLIHook reports whether the recipe declares a CLI-served hook.
func recipeServesCLIHook(recipeID string, recipe *schema.Recipe) bool {
	for _, hook := range recipe.RuntimeHooks {
		if cliServedHookCommand(recipeID, hook) != "" {
			return true
		}
	}
	return false
}

// pendingRuntimeSnapshot is the snapshot a run resolved while materializing but
// must not publish yet: it becomes live only when the whole run succeeds, so a
// later failure leaves the previous policy in place (R1-002/R3-001). The spine
// owns the value, the materializer fills it, `(*runner).run` writes it once.
type pendingRuntimeSnapshot struct {
	Snapshot projectcache.RuntimeSnapshot
	Found    bool
}

// runtimeSnapshotFromConfig resolves the snapshot fields exactly as the retired
// launcher's stamps were resolved: the same merged-config keys, the same
// defaults and the same enum refusals (verbatim messages).
func runtimeSnapshotFromConfig(cfg map[string]any) (projectcache.RuntimeSnapshot, error) {
	snap := projectcache.RuntimeSnapshot{
		GateMode:          runtimeGateMode(cfg),
		GateScope:         shared.TemplateConfigOr(cfg, "gate_scope", "auto"),
		RepoTopology:      shared.TemplateConfigOr(cfg, "repo_topology", "auto"),
		Protected:         shared.TemplateConfigOr(cfg, "WORKTREE_GATE_PROTECTED", "main development"),
		WorktreesDir:      shared.TemplateConfigOr(cfg, "worktrees_dir", ".worktrees"),
		IntegrationBranch: shared.TemplateConfigOr(cfg, "integration_branch", "main"),
	}
	switch snap.GateScope {
	case "auto", "superrepo", "subrepo":
	default:
		return projectcache.RuntimeSnapshot{}, fmt.Errorf(
			"invalid gate_scope '%s'; allowed: auto | superrepo | subrepo", snap.GateScope)
	}
	switch snap.RepoTopology {
	case "auto", "standalone", "monorepo-apps", "monorepo-submodules":
	default:
		return projectcache.RuntimeSnapshot{}, fmt.Errorf(
			"invalid repo_topology '%s'; allowed: auto | standalone | monorepo-apps | monorepo-submodules",
			snap.RepoTopology)
	}
	return snap, nil
}

// runtimeGateMode mirrors the launcher's stamped gate mode: a configured value
// is used verbatim (Python's str(merged_cfg.get("gate_mode", "always"))).
func runtimeGateMode(cfg map[string]any) string {
	if value, ok := cfg["gate_mode"]; ok {
		return shared.PyConfigString(value)
	}
	return "always"
}
