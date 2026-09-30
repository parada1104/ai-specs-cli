package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func symlinkTo(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(link), err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestPlatformTable pins doctor.PLATFORM's keys and per-agent values.
func TestPlatformTable(t *testing.T) {
	want := map[string]platformInfo{
		"claude":   {instructionsPath: "CLAUDE.md", skillsDir: ".claude/skills", mcpConfigPath: ".mcp.json", mcpKey: "mcpServers", commandsDir: ".claude/commands"},
		"cursor":   {instructionsPath: "", skillsDir: ".cursor/skills", mcpConfigPath: ".cursor/mcp.json", mcpKey: "mcpServers", commandsDir: ".cursor/commands"},
		"opencode": {instructionsPath: "", skillsDir: ".opencode/skills", mcpConfigPath: "opencode.json", mcpKey: "mcp", commandsDir: ".opencode/commands", skillsCopy: true},
		"pi":       {instructionsPath: "", skillsDir: ".pi/skills", mcpConfigPath: ".mcp.json", mcpKey: "mcpServers", commandsDir: ""},
		"omp":      {instructionsPath: ".omp/AGENTS.md", skillsDir: ".omp/skills", mcpConfigPath: ".omp/mcp.json", mcpKey: "mcpServers", commandsDir: ".omp/commands"},
		"codex":    {instructionsPath: "", skillsDir: "", mcpConfigPath: ".codex/config.toml", mcpKey: "mcp_servers", commandsDir: ""},
		"copilot":  {instructionsPath: ".github/copilot-instructions.md", skillsDir: "", mcpConfigPath: "", mcpKey: "", commandsDir: ""},
		"gemini":   {instructionsPath: "GEMINI.md", skillsDir: ".gemini/skills", mcpConfigPath: ".gemini/settings.json", mcpKey: "mcpServers", commandsDir: ""},
	}
	if len(platform) != len(want) {
		t.Fatalf("platform has %d agents, want %d", len(platform), len(want))
	}
	for agent, wantInfo := range want {
		if got := platform[agent]; got != wantInfo {
			t.Errorf("platform[%q] = %+v, want %+v", agent, got, wantInfo)
		}
	}
}

func TestSortedPlatformAgents(t *testing.T) {
	want := []string{"claude", "codex", "copilot", "cursor", "gemini", "omp", "opencode", "pi"}
	if got := sortedPlatformAgents(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sortedPlatformAgents() = %v, want %v", got, want)
	}
}

func TestMcpServerCount(t *testing.T) {
	cases := map[string]int{
		"":                             0,
		"[project]\nname = \"x\"\n":    0,
		"mcp = 5\n":                    0,
		"[mcp.one]\ncommand = \"a\"\n": 1,
		"[mcp.one]\n[mcp.two]\n":       2,
	}
	for src, want := range cases {
		got := mcpServerCount(parseTOML(t, src))
		if got != want {
			t.Errorf("mcpServerCount(%q) = %d, want %d", src, got, want)
		}
	}
	if got := mcpServerCount(nil); got != 0 {
		t.Errorf("mcpServerCount(nil) = %d, want 0", got)
	}
}

// TestCheckEnabledAgentsNoMcpNoAgents pins the two leading WARNs and the early
// return when nothing is enabled.
func TestCheckEnabledAgentsNoMcpNoAgents(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project]\nname = \"x\"\n")
	d := New(root, t.TempDir())
	d.checkEnabledAgents()
	assertChecks(t, d, []Check{
		{WARN, "mcp", "no [mcp.*] entries declared", "add MCP servers to ai-specs.toml if needed"},
		{WARN, "agents", "no agents enabled in ai-specs.toml", "set [agents].enabled"},
	})
}

// TestCheckEnabledAgentsEnabled pins the roster order for a synced-style
// manifest with no MCP entries.
func TestCheckEnabledAgentsEnabled(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[agents]\nenabled = [\"claude\", \"pi\"]\n")
	d := New(root, t.TempDir())
	d.checkEnabledAgents()
	assertChecks(t, d, []Check{
		{WARN, "mcp", "no [mcp.*] entries declared", "add MCP servers to ai-specs.toml if needed"},
		{OK, "agents", "enabled: claude, pi", ""},
		{ERROR, "CLAUDE.md", "missing; run ai-specs sync", "ai-specs sync"},
		{ERROR, ".claude/skills", "missing; run ai-specs sync", "ai-specs sync"},
		{WARN, ".claude/commands", "directory missing", "ai-specs sync"},
		{ERROR, ".pi/skills", "missing; run ai-specs sync", "ai-specs sync"},
	})
}

// TestCheckEnabledAgentsUnsupported pins the unsupported-agent ERROR and its
// sorted supported list.
func TestCheckEnabledAgentsUnsupported(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[agents]\nenabled = [\"bogus\"]\n")
	d := New(root, t.TempDir())
	d.checkEnabledAgents()
	assertChecks(t, d, []Check{
		{WARN, "mcp", "no [mcp.*] entries declared", "add MCP servers to ai-specs.toml if needed"},
		{OK, "agents", "enabled: bogus", ""},
		{ERROR, "agents", "unsupported agent: bogus",
			"supported: claude, codex, copilot, cursor, gemini, omp, opencode, pi"},
	})
}

// TestCheckEnabledAgentsWithMcp pins that a declared [mcp.*] entry suppresses
// the mcp WARN and gates the per-agent mcp config check.
func TestCheckEnabledAgentsWithMcp(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[agents]\nenabled = [\"claude\"]\n[mcp.demo]\ncommand = \"x\"\n")
	d := New(root, t.TempDir())
	d.checkEnabledAgents()
	// No mcp WARN; the mcp-claude check fires because mcp_count > 0.
	var names []string
	for _, c := range d.Checks {
		names = append(names, c.Name)
	}
	want := []string{"agents", "CLAUDE.md", ".claude/skills", ".claude/commands", "mcp-claude"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("check names = %v, want %v", names, want)
	}
	last := d.Checks[len(d.Checks)-1]
	if last != (Check{ERROR, "mcp-claude", ".mcp.json missing", "ai-specs sync"}) {
		t.Errorf("mcp-claude = %+v", last)
	}
	// Present -> OK.
	writeFile(t, filepath.Join(root, ".mcp.json"), "{}\n")
	d2 := New(root, t.TempDir())
	d2.checkEnabledAgents()
	last = d2.Checks[len(d2.Checks)-1]
	if last != (Check{OK, "mcp-claude", ".mcp.json present", ""}) {
		t.Errorf("mcp-claude present = %+v", last)
	}
}

// TestCheckAgentOutputsInstructions covers the four instruction-symlink states.
func TestCheckAgentOutputsInstructions(t *testing.T) {
	plat := platform["claude"]

	t.Run("valid", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "AGENTS.md"), "# agents\n")
		symlinkTo(t, "AGENTS.md", filepath.Join(root, "CLAUDE.md"))
		d := New(root, t.TempDir())
		d.checkAgentOutputs("claude", plat, 0)
		if d.Checks[0] != (Check{OK, "CLAUDE.md", "symlink valid → AGENTS.md", ""}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("elsewhere", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "OTHER.md"), "x\n")
		symlinkTo(t, "OTHER.md", filepath.Join(root, "CLAUDE.md"))
		d := New(root, t.TempDir())
		d.checkAgentOutputs("claude", plat, 0)
		if d.Checks[0] != (Check{ERROR, "CLAUDE.md", "symlink points elsewhere", "run ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("dangling-to-agents", func(t *testing.T) {
		// AGENTS.md does not exist: Path.resolve() is non-strict, so the
		// dangling CLAUDE.md -> AGENTS.md still resolves to <root>/AGENTS.md.
		root := t.TempDir()
		symlinkTo(t, "AGENTS.md", filepath.Join(root, "CLAUDE.md"))
		d := New(root, t.TempDir())
		d.checkAgentOutputs("claude", plat, 0)
		if d.Checks[0] != (Check{OK, "CLAUDE.md", "symlink valid → AGENTS.md", ""}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("regular-file", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "CLAUDE.md"), "# nope\n")
		d := New(root, t.TempDir())
		d.checkAgentOutputs("claude", plat, 0)
		if d.Checks[0] != (Check{ERROR, "CLAUDE.md", "not a symlink", "run ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("missing", func(t *testing.T) {
		d := New(t.TempDir(), t.TempDir())
		d.checkAgentOutputs("claude", plat, 0)
		if d.Checks[0] != (Check{ERROR, "CLAUDE.md", "missing; run ai-specs sync", "ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
}

// TestCheckSkillsSymlink covers the acceptance rule branches.
func TestCheckSkillsSymlink(t *testing.T) {
	t.Run("cache-resolved", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		d := New(root, home)
		cacheDir := d.resolvedSkillsDir()
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}
		link := filepath.Join(root, ".claude", "skills")
		symlinkTo(t, cacheDir, link)
		d.checkSkillsSymlink(".claude/skills", link)
		want := Check{OK, ".claude/skills", "symlink valid → " + resolvePy(cacheDir), ""}
		if len(d.Checks) != 1 || d.Checks[0] != want {
			t.Errorf("checks = %+v, want %+v", d.Checks, want)
		}
	})
	t.Run("cache-projects-fallback", func(t *testing.T) {
		root := t.TempDir()
		// A resolved-skills path under any cache/projects tree is accepted even
		// when it is not the computed cache key.
		fallback := filepath.Join(t.TempDir(), "cache", "projects", "other-key", "resolved-skills")
		if err := os.MkdirAll(fallback, 0o755); err != nil {
			t.Fatalf("mkdir fallback: %v", err)
		}
		d := New(root, t.TempDir())
		link := filepath.Join(root, "ai-specs", ".internal", "resolved-skills") // arbitrary link location
		symlinkTo(t, fallback, link)
		d.checkSkillsSymlink(".pi/skills", link)
		want := Check{OK, ".pi/skills", "symlink valid → " + resolvePy(fallback), ""}
		if len(d.Checks) != 1 || d.Checks[0] != want {
			t.Errorf("checks = %+v, want %+v", d.Checks, want)
		}
	})
	t.Run("in-project-resolved-skills", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "ai-specs", ".internal", "resolved-skills")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		d := New(root, t.TempDir())
		link := filepath.Join(root, ".claude", "skills")
		symlinkTo(t, target, link)
		d.checkSkillsSymlink(".claude/skills", link)
		// Display is project-relative because the target is under root.
		want := Check{OK, ".claude/skills", "symlink valid → ai-specs/.internal/resolved-skills", ""}
		if len(d.Checks) != 1 || d.Checks[0] != want {
			t.Errorf("checks = %+v, want %+v", d.Checks, want)
		}
	})
	t.Run("elsewhere", func(t *testing.T) {
		root := t.TempDir()
		other := t.TempDir()
		link := filepath.Join(root, ".claude", "skills")
		symlinkTo(t, other, link)
		d := New(root, t.TempDir())
		d.checkSkillsSymlink(".claude/skills", link)
		if d.Checks[0] != (Check{ERROR, ".claude/skills", "symlink points elsewhere", "run ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("regular-dir", func(t *testing.T) {
		root := t.TempDir()
		link := filepath.Join(root, ".claude", "skills")
		if err := os.MkdirAll(link, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		d := New(root, t.TempDir())
		d.checkSkillsSymlink(".claude/skills", link)
		if d.Checks[0] != (Check{ERROR, ".claude/skills", "not a symlink", "run ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("missing", func(t *testing.T) {
		root := t.TempDir()
		d := New(root, t.TempDir())
		d.checkSkillsSymlink(".claude/skills", filepath.Join(root, ".claude", "skills"))
		if d.Checks[0] != (Check{ERROR, ".claude/skills", "missing; run ai-specs sync", "ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
}

// TestCheckAgentOutputsCopiedSkills covers OpenCode's skills_copy branch.
func TestCheckAgentOutputsCopiedSkills(t *testing.T) {
	plat := platform["opencode"]
	t.Run("present", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, ".opencode", "skills", "one", "SKILL.md"), "x\n")
		d := New(root, t.TempDir())
		d.checkAgentOutputs("opencode", plat, 0)
		if d.Checks[0] != (Check{OK, ".opencode/skills", "copied skill directory present", ""}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("empty", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".opencode", "skills"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		d := New(root, t.TempDir())
		d.checkAgentOutputs("opencode", plat, 0)
		if d.Checks[0] != (Check{ERROR, ".opencode/skills", "missing or empty", "ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
}

// TestCheckAgentCommands covers every branch of the commands comparison.
func TestCheckAgentCommands(t *testing.T) {
	t.Run("directory-missing", func(t *testing.T) {
		d := New(t.TempDir(), t.TempDir())
		d.checkAgentCommands(".claude/commands")
		if d.Checks[0] != (Check{WARN, ".claude/commands", "directory missing", "ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("no-commands-configured", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude", "commands"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		d := New(root, t.TempDir())
		d.checkAgentCommands(".claude/commands")
		if d.Checks[0] != (Check{OK, ".claude/commands", "no commands configured", ""}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("directory-empty", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".claude", "commands"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeFile(t, filepath.Join(root, "ai-specs", "commands", "alpha.md"), "x\n")
		d := New(root, t.TempDir())
		d.checkAgentCommands(".claude/commands")
		if d.Checks[0] != (Check{WARN, ".claude/commands", "directory empty", "ai-specs sync to populate"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("missing-commands", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "ai-specs", "commands", "alpha.md"), "x\n")
		writeFile(t, filepath.Join(root, "ai-specs", "commands", "beta.md"), "x\n")
		writeFile(t, filepath.Join(root, ".claude", "commands", "alpha.md"), "x\n")
		d := New(root, t.TempDir())
		d.checkAgentCommands(".claude/commands")
		if d.Checks[0] != (Check{ERROR, ".claude/commands", "missing 1 command(s): beta.md", "run ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("stale-commands", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "ai-specs", "commands", "alpha.md"), "x\n")
		writeFile(t, filepath.Join(root, ".claude", "commands", "alpha.md"), "x\n")
		writeFile(t, filepath.Join(root, ".claude", "commands", "old.md"), "x\n")
		d := New(root, t.TempDir())
		d.checkAgentCommands(".claude/commands")
		if d.Checks[0] != (Check{WARN, ".claude/commands", "1 stale command(s): old.md", "run ai-specs sync"}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
	t.Run("in-sync-with-cache-and-bundled", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		d := New(root, home)
		writeFile(t, filepath.Join(root, "ai-specs", "commands", "alpha.md"), "x\n")
		writeFile(t, filepath.Join(d.commandsDir(), "cached.md"), "x\n")
		writeFile(t, filepath.Join(d.bundledCommandsRoot(), "bundled.md"), "x\n")
		for _, name := range []string{"alpha.md", "cached.md", "bundled.md"} {
			writeFile(t, filepath.Join(root, ".claude", "commands", name), "x\n")
		}
		d.checkAgentCommands(".claude/commands")
		if d.Checks[0] != (Check{OK, ".claude/commands", "3 command(s) in sync", ""}) {
			t.Errorf("check = %+v", d.Checks[0])
		}
	})
}

// TestMdNames pins glob("*.md") semantics: dotfiles excluded, directories
// included (Python's glob matches a directory whose name ends in .md).
func TestMdNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.md"), "x\n")
	writeFile(t, filepath.Join(dir, "two.txt"), "x\n")
	writeFile(t, filepath.Join(dir, ".hidden.md"), "x\n")
	if err := os.MkdirAll(filepath.Join(dir, "nested.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := sortedSet(mdNames(dir))
	want := []string{"nested.md", "one.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("mdNames() = %v, want %v", got, want)
	}
	// A missing directory yields an empty set, not a panic.
	if got := mdNames(filepath.Join(dir, "nope")); len(got) != 0 {
		t.Errorf("mdNames(missing) = %v, want empty", got)
	}
}

// TestResolvePyNonStrict pins resolvePy against Path.resolve(): a dangling
// symlink resolves to its target path (non-strict), and a chain resolves to
// the real path of the final target.
func TestResolvePyNonStrict(t *testing.T) {
	t.Run("relative-dangling", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, "dir"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(base, "dir", "link")
		symlinkTo(t, "../target/missing", link)
		want := resolvePy(filepath.Join(base, "target", "missing"))
		if got := resolvePy(link); got != want {
			t.Errorf("resolvePy(%s) = %s, want %s", link, got, want)
		}
	})
	t.Run("chain", func(t *testing.T) {
		dir := t.TempDir()
		final := filepath.Join(dir, "final")
		writeFile(t, final, "x\n")
		a := filepath.Join(dir, "a")
		b := filepath.Join(dir, "b")
		symlinkTo(t, b, a)     // a -> b
		symlinkTo(t, final, b) // b -> final (absolute)
		want, err := filepath.EvalSymlinks(final)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		if got := resolvePy(a); got != want {
			t.Errorf("resolvePy(%s) = %s, want %s", a, got, want)
		}
	})
}

// TestIsRelativeTo pins the lexical containment used for display paths.
func TestIsRelativeTo(t *testing.T) {
	cases := []struct {
		base, target string
		want         bool
	}{
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/bc", false},
		{"/a/b", "/a", false},
		{"/a/b", "/x/y", false},
	}
	for _, tc := range cases {
		if got := isRelativeTo(tc.base, tc.target); got != tc.want {
			t.Errorf("isRelativeTo(%q, %q) = %v, want %v", tc.base, tc.target, got, tc.want)
		}
	}
}
