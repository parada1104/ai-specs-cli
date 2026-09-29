package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// checkEnabledAgents is doctor._check_enabled_agents.
//
// Order is frozen: the `mcp` WARN (only when the manifest declares no
// [mcp.*] entries), then `agents`, then per enabled agent in manifest order.
func (d *Doctor) checkEnabledAgents() {
	data := d.manifestData()
	var agents *toml.Table
	if data != nil {
		agents, _ = data.Table("agents")
	}
	var enabled []any
	if agents != nil {
		if raw, ok := agents.Get("enabled"); ok {
			enabled = asPlainList(raw)
		}
	}

	mcpCount := mcpServerCount(data)
	if mcpCount == 0 {
		d.add(WARN, "mcp", "no [mcp.*] entries declared",
			"add MCP servers to ai-specs.toml if needed")
	}

	if len(enabled) == 0 {
		d.add(WARN, "agents", "no agents enabled in ai-specs.toml",
			"set [agents].enabled")
		return
	}

	names := make([]string, len(enabled))
	for i, item := range enabled {
		names[i] = pyStrValue(item)
	}
	d.add(OK, "agents", "enabled: "+strings.Join(names, ", "))

	for _, item := range enabled {
		agent := pyStrValue(item)
		plat, ok := platform[agent]
		if !ok {
			d.add(ERROR, "agents", "unsupported agent: "+agent,
				"supported: "+strings.Join(sortedPlatformAgents(), ", "))
			continue
		}
		d.checkAgentOutputs(agent, plat, mcpCount)
	}
}

// mcpServerCount is doctor._mcp_server_count: len of the [mcp] table, or 0 for
// an absent / non-table value.
func mcpServerCount(data *toml.Table) int {
	if data == nil {
		return 0
	}
	mcp, ok := data.Table("mcp")
	if !ok || mcp == nil {
		return 0
	}
	return len(mcp.Keys())
}

// checkAgentOutputs is doctor._check_agent_outputs: instructions symlink,
// skills dir, commands dir, then the MCP config (only when mcp_count > 0).
func (d *Doctor) checkAgentOutputs(agent string, plat platformInfo, mcpCount int) {
	// Instruction symlink
	if plat.instructionsPath != "" {
		instrPath := filepath.Join(d.Root, plat.instructionsPath)
		switch {
		case isSymlink(instrPath):
			target := resolvePy(instrPath)
			agentsMD := resolvePy(filepath.Join(d.Root, "AGENTS.md"))
			if target == agentsMD {
				d.add(OK, plat.instructionsPath, "symlink valid → AGENTS.md")
			} else {
				d.add(ERROR, plat.instructionsPath, "symlink points elsewhere",
					"run ai-specs sync")
			}
		case exists(instrPath):
			d.add(ERROR, plat.instructionsPath, "not a symlink", "run ai-specs sync")
		default:
			d.add(ERROR, plat.instructionsPath, "missing; run ai-specs sync", "ai-specs sync")
		}
	}

	// Skills
	if plat.skillsDir != "" {
		skillsPath := filepath.Join(d.Root, plat.skillsDir)
		if plat.skillsCopy {
			// OpenCode: copied skill dirs, not symlinks.
			if isDir(skillsPath) && dirNonEmpty(skillsPath) {
				d.add(OK, plat.skillsDir, "copied skill directory present")
			} else {
				d.add(ERROR, plat.skillsDir, "missing or empty", "ai-specs sync")
			}
		} else {
			d.checkSkillsSymlink(plat.skillsDir, skillsPath)
		}
	}

	// Commands
	if plat.commandsDir != "" {
		d.checkAgentCommands(plat.commandsDir)
	}

	// MCP config
	if plat.mcpConfigPath != "" && mcpCount > 0 {
		if isFile(filepath.Join(d.Root, plat.mcpConfigPath)) {
			d.add(OK, "mcp-"+agent, plat.mcpConfigPath+" present")
		} else {
			d.add(ERROR, "mcp-"+agent, plat.mcpConfigPath+" missing", "ai-specs sync")
		}
	}
}

// checkSkillsSymlink is the symlink half of _check_agent_outputs' skills
// branch, ported faithfully from the legacy acceptance rule.
//
// A symlink target (already resolved, like Path.resolve()) is accepted when
// ANY of these hold against one of the three candidate roots: ai-specs/skills;
// ai-specs/.internal/resolved-skills (legacy in-project layout); or the
// per-project CLI cache {cache}/resolved-skills.
//
// Acceptance rule (any one is enough):
//
//  1. resolved target equals the candidate (string equality);
//  2. both exist and os.SameFile reports them identical (inode/dev, tolerating
//     macOS /var vs /private/var and symlinked cache roots);
//  3. their realpaths compare equal; or
//  4. (extra legacy fallback) the target exists and its path segments contain
//     "cache" and "projects" and its basename is "resolved-skills", which
//     accepts any existing cache resolved-skills target even when the computed
//     cache key differs.
//
// A target failing all four is an ERROR with `run ai-specs sync` guidance.
func (d *Doctor) checkSkillsSymlink(name, skillsPath string) {
	if isSymlink(skillsPath) {
		target := resolvePy(skillsPath)
		aiSpecsSkills := resolvePy(filepath.Join(d.Root, "ai-specs", "skills"))
		resolvedSkills := resolvePy(filepath.Join(d.Root, "ai-specs", ".internal", "resolved-skills"))
		cacheResolved := resolvePy(d.resolvedSkillsDir())
		candidates := []string{aiSpecsSkills, resolvedSkills, cacheResolved}

		matched := false
		for _, candidate := range candidates {
			if target == candidate {
				matched = true
				break
			}
			tInfo, tErr := os.Stat(target)
			cInfo, cErr := os.Stat(candidate)
			if tErr == nil && cErr == nil && os.SameFile(tInfo, cInfo) {
				matched = true
				break
			}
			if rt, err := filepath.EvalSymlinks(target); err == nil {
				if rc, err2 := filepath.EvalSymlinks(candidate); err2 == nil && rt == rc {
					matched = true
					break
				}
			}
		}
		if !matched && exists(target) {
			parts := strings.Split(filepath.ToSlash(target), "/")
			if containsString(parts, "cache") && containsString(parts, "projects") &&
				filepath.Base(target) == "resolved-skills" {
				matched = true
			}
		}

		if matched {
			display := target
			if isRelativeTo(d.Root, target) {
				display = relTo(d.Root, target)
			}
			d.add(OK, name, "symlink valid → "+display)
		} else {
			d.add(ERROR, name, "symlink points elsewhere", "run ai-specs sync")
		}
		return
	}
	if exists(skillsPath) {
		d.add(ERROR, name, "not a symlink", "run ai-specs sync")
		return
	}
	d.add(ERROR, name, "missing; run ai-specs sync", "ai-specs sync")
}

// checkAgentCommands is the commands branch of _check_agent_outputs. Expected
// = hand-authored ai-specs/commands/ ∪ cache commands/ ∪ CLI-bundled cache
// commands (all three are CLI-driven or merge inputs).
func (d *Doctor) checkAgentCommands(name string) {
	commandsPath := filepath.Join(d.Root, name)
	if !isDir(commandsPath) {
		d.add(WARN, name, "directory missing", "ai-specs sync")
		return
	}
	expected := map[string]bool{}
	if aiSpecsCommands := filepath.Join(d.Root, "ai-specs", "commands"); isDir(aiSpecsCommands) {
		for k := range mdNames(aiSpecsCommands) {
			expected[k] = true
		}
	}
	for k := range d.cacheCommandNames() {
		expected[k] = true
	}
	for k := range d.bundledCacheCommandNames() {
		expected[k] = true
	}
	actual := mdNames(commandsPath)
	missing := setDiff(expected, actual)
	extra := setDiff(actual, expected)

	switch {
	case len(actual) == 0 && len(expected) == 0:
		d.add(OK, name, "no commands configured")
	case len(actual) == 0:
		d.add(WARN, name, "directory empty", "ai-specs sync to populate")
	case len(missing) > 0:
		d.add(ERROR, name,
			fmt.Sprintf("missing %d command(s): %s", len(missing), strings.Join(sortedSet(missing), ", ")),
			"run ai-specs sync")
	case len(extra) > 0:
		d.add(WARN, name,
			fmt.Sprintf("%d stale command(s): %s", len(extra), strings.Join(sortedSet(extra), ", ")),
			"run ai-specs sync")
	default:
		d.add(OK, name, fmt.Sprintf("%d command(s) in sync", len(actual)))
	}
}

// cacheCommandNames is doctor._cache_command_names: the .md filenames in the
// per-project CLI cache commands dir.
func (d *Doctor) cacheCommandNames() map[string]bool {
	return mdNames(d.commandsDir())
}

// bundledCacheCommandNames is doctor._bundled_command_names: the .md filenames
// flattened into {cache}/.bundled/commands/.
func (d *Doctor) bundledCacheCommandNames() map[string]bool {
	return mdNames(d.bundledCommandsRoot())
}

// --- small helpers --------------------------------------------------------

// isSymlink mirrors Path.is_symlink (Lstat: true even for a broken link).
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// dirNonEmpty mirrors `any(dir.iterdir())`.
func dirNonEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

// asPlainList mirrors `isinstance(x, list)` for the values internal/toml can
// produce.
func asPlainList(raw any) []any {
	switch x := raw.(type) {
	case []any:
		return x
	case []*toml.Table:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = e
		}
		return out
	}
	return nil
}

// pyStrValue mirrors Python str(value) for the TOML value types.
func pyStrValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return pythonRepr(v)
}

// sortedPlatformAgents is sorted(PLATFORM) for the unsupported-agent guidance.
func sortedPlatformAgents() []string {
	names := make([]string, 0, len(platform))
	for name := range platform {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isRelativeTo mirrors PurePath.is_relative_to: a lexical containment test.
func isRelativeTo(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// setDiff returns the keys present in a but not b.
func setDiff(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		if !b[k] {
			out[k] = true
		}
	}
	return out
}

// sortedSet returns the keys in ascending order.
func sortedSet(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
