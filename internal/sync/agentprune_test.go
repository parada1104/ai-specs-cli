package sync

// T7 (S3): an agent removed from [agents].enabled loses every artifact ai-specs
// generated for it — links, recorded command copies, recorded MCP servers, hook
// wiring and shims, and their lock records — while user content in the same
// files stays byte-identical.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// agentLeftoverFixture installs one command + hook recipe in an isolated CLI
// home and returns the home and a project enabling the given agents.
func agentLeftoverFixture(t *testing.T, agents string) (home, project string) {
	t.Helper()
	source, _ := spineEnv(t)
	home = t.TempDir()
	for _, name := range []string{"templates", "bundled-skills", "bundled-commands", "VERSION"} {
		if err := os.Symlink(filepath.Join(source, name), filepath.Join(home, name)); err != nil {
			t.Fatal(err)
		}
	}
	recipe := filepath.Join(home, "catalog/recipes/agentdemo")
	spineWrite(t, filepath.Join(recipe, "recipe.toml"),
		"[recipe]\nid = 'agentdemo'\nname = 'Demo'\ndescription = 'Fixture'\nversion = '1.0.0'\n"+
			"[provides]\ncommands = [{ id = 'go', path = 'commands/go.md' }]\n"+
			"[[provides.hooks]]\nid = 'main'\nevent = 'pre-tool-use'\nscript = 'hooks/main.sh'\nmatcher = 'Bash'\n", 0o644)
	spineWrite(t, filepath.Join(recipe, "hooks/main.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	spineWrite(t, filepath.Join(recipe, "commands/go.md"), "# go\n", 0o644)
	project = spineProject(t, "[project]\nname = 'retired-agents'\n[brief]\nrender = false\n"+
		"[agents]\nenabled = "+agents+"\n[mcp.demo]\ncommand = 'demo'\n"+
		"[recipes.agentdemo]\nenabled = true\n")
	return home, project
}

// retireAgents rewrites the manifest's agent list, keeping the rest.
func retireAgents(t *testing.T, project, agents string) {
	t.Helper()
	path := filepath.Join(project, "ai-specs", "ai-specs.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start := strings.Index(text, "enabled = [")
	end := strings.Index(text[start:], "]\n")
	spineWrite(t, path, text[:start]+"enabled = "+agents+text[start+end+1:], 0o644)
}

func mustLstat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s should be gone (err=%v)", path, err)
	}
}

// addMCPServer merges one user server into a JSON MCP target, as a user would.
func addMCPServer(t *testing.T, path, name string) {
	t.Helper()
	var doc map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	servers[name] = map[string]any{"command": "manual"}
	doc["mcpServers"] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	spineWrite(t, path, string(out)+"\n", 0o644)
}

func mcpServerNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	names := map[string]bool{}
	for name := range doc.Servers {
		names[name] = true
	}
	return names
}

func TestRetiredAgentsOnlyWithoutEnabledList(t *testing.T) {
	source, _ := spineEnv(t)
	project := spineProject(t, "[project]\nname = 'retired'\n[agents]\nenabled = ['claude', 'codex']\n")
	_ = source
	retired, err := retiredAgents(project)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"copilot", "cursor", "gemini", "omp", "opencode", "pi"}
	if strings.Join(retired, ",") != strings.Join(want, ",") {
		t.Fatalf("retired = %v, want %v", retired, want)
	}
	// An empty list declares no agent at all, so it retires nothing: the run
	// itself writes nothing for any agent and says so.
	spineWrite(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = 'retired'\n[agents]\nenabled = []\n", 0o644)
	retired, err = retiredAgents(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(retired) != 0 {
		t.Fatalf("retired = %v, want none for an empty enabled list", retired)
	}
}

func TestSyncPrunesRetiredAgentLeftovers(t *testing.T) {
	home, project := agentLeftoverFixture(t, "['claude', 'cursor', 'pi']")
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("seed sync stderr = %q", stderr)
	}
	// A user server in each retired agent's MCP target and a user hook in the
	// shared settings file must survive the prune byte-for-byte.
	addMCPServer(t, filepath.Join(project, ".mcp.json"), "manual")
	addMCPServer(t, filepath.Join(project, ".pi", "mcp.json"), "manual")
	settings := filepath.Join(project, ".claude", "settings.json")
	var doc map[string]any
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	hooks["PreToolUse"] = append(hooks["PreToolUse"].([]any), map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "echo user"}}})
	doc["hooks"] = hooks
	merged, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	spineWrite(t, settings, string(merged)+"\n", 0o644)

	retireAgents(t, project, "['cursor']")
	out, stderr := runSyncCaseOutput(t, home, project)

	for _, rel := range []string{
		"CLAUDE.md", ".claude/skills", ".claude/commands/go.md",
		".pi/skills", ".pi/extensions/agentdemo-main.ts",
	} {
		mustNotExist(t, filepath.Join(project, rel))
	}
	for _, line := range []string{
		"  ✓ removed retired agent link CLAUDE.md",
		"  ✓ removed retired agent link .pi/skills",
		"  ✓ removed retired agent command .claude/commands/go.md",
		"  ✓ removed retired agent claude MCP record .mcp.json",
		"  ✓ removed retired agent pi MCP record .pi/mcp.json",
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("stdout missing %q:\n%s", line, out)
		}
	}
	// The renderer names the target by its resolved path.
	if !strings.Contains(out, "    ✓ removed managed MCP server \"demo\" from ") ||
		!strings.Contains(out, ".pi/mcp.json\n") {
		t.Fatalf("stdout does not report the pruned user-visible server:\n%s", out)
	}
	if strings.Contains(stderr, "agentdemo") {
		t.Fatalf("stderr names a managed artifact: %q", stderr)
	}
	// Recorded MCP servers are gone, the user's are not.
	for _, rel := range []string{".mcp.json", ".pi/mcp.json"} {
		names := mcpServerNames(t, filepath.Join(project, rel))
		if names["demo"] || !names["manual"] {
			t.Fatalf("%s servers = %v, want manual only", rel, names)
		}
	}
	// The user's own hook survives; the managed wiring is gone.
	data, err = os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "echo user") {
		t.Fatalf("user hook was removed from %s:\n%s", settings, data)
	}
	if strings.Contains(string(data), "ai-specs:hooks:") {
		t.Fatalf("managed hook wiring survived in %s:\n%s", settings, data)
	}
	// Lock: the retired targets, copies and links lost their records, the
	// enabled agent keeps every one of them.
	lockText := readLockText(t, project)
	for _, gone := range []string{`".pi/mcp.json"`, `"CLAUDE.md"`, `".claude/skills"`, `".claude/commands/go.md"`, `".pi/extensions/agentdemo-main.ts"`, `".mcp.json"`} {
		if strings.Contains(lockText, gone) {
			t.Fatalf("lock still records %s:\n%s", gone, lockText)
		}
	}
	for _, kept := range []string{`".cursor/skills"`, `".cursor/hooks/agentdemo-main.sh"`, `".cursor/mcp.json"`, `".cursor/commands/go.md"`} {
		if !strings.Contains(lockText, kept) {
			t.Fatalf("lock lost the enabled agent's record %s:\n%s", kept, lockText)
		}
	}
	if !strings.Contains(lockText, `[links]`) || !strings.Contains(lockText, `".cursor/skills" = "ai-specs:link"`) {
		t.Fatalf("lock does not record the link sync created:\n%s", lockText)
	}
	// The enabled agent's artifacts are untouched.
	if info := mustLstat(t, filepath.Join(project, ".cursor", "skills")); info.Mode()&os.ModeSymlink == 0 {
		t.Fatal(".cursor/skills is no longer a symlink")
	}
	if !mcpServerNames(t, filepath.Join(project, ".cursor", "mcp.json"))["demo"] {
		t.Fatal("the enabled agent lost its managed MCP server")
	}
}

func TestSyncRetiredAgentDropsRecordOfAMissingMCPTarget(t *testing.T) {
	home, project := agentLeftoverFixture(t, "['claude', 'pi']")
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("seed sync stderr = %q", stderr)
	}
	// The target is gone, so only the ownership record is left behind: it goes
	// too, without a render.
	if err := os.Remove(filepath.Join(project, ".pi", "mcp.json")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readLockText(t, project), `[mcp.".pi/mcp.json"]`) {
		t.Fatal("seed lock does not record the target")
	}
	retireAgents(t, project, "['claude']")
	out, _ := runSyncCaseOutput(t, home, project)

	if strings.Contains(readLockText(t, project), `.pi/mcp.json`) {
		t.Fatal("the record of the missing target survived")
	}
	if !strings.Contains(out, "  ✓ removed retired agent pi MCP record .pi/mcp.json") {
		t.Fatalf("stdout = %s", out)
	}
}

// runSyncCaseOutput is runSyncCase plus stdout.
func runSyncCaseOutput(t *testing.T, home, project string) (string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &stderr); rc != 0 {
		t.Fatalf("sync rc=%d, stderr=%q", rc, stderr.String())
	}
	return out.String(), stderr.String()
}

func TestSyncRetiredAgentKeepsUnrecordedContent(t *testing.T) {
	home, project := agentLeftoverFixture(t, "['claude', 'cursor', 'pi']")
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("seed sync stderr = %q", stderr)
	}
	// Not ai-specs' output: a user shim without the generated marker, a
	// generated-looking shim with no ownership record, and a user command copy
	// with no ownership record.
	userShim := filepath.Join(project, ".pi", "extensions", "mine.ts")
	spineWrite(t, userShim, "// user extension\nexport {};\n", 0o644)
	legacyShim := filepath.Join(project, ".pi", "extensions", "orphan.ts")
	spineWrite(t, legacyShim, hooksTSHeader+"orphan\n", 0o644)
	userCommand := filepath.Join(project, ".cursor", "commands", "notes.md")
	spineWrite(t, userCommand, "# my notes\n", 0o644)
	// A target holding servers ai-specs never recorded keeps them all.
	unrecorded := filepath.Join(project, ".omp")
	spineWrite(t, filepath.Join(unrecorded, "mcp.json"), "{\"mcpServers\": {\"theirs\": {\"command\": \"x\"}}}\n", 0o644)

	retireAgents(t, project, "['claude']")
	_, stderr := runSyncCaseOutput(t, home, project)

	if body, err := os.ReadFile(userShim); err != nil || string(body) != "// user extension\nexport {};\n" {
		t.Fatalf("user shim changed: %q (err=%v)", body, err)
	}
	if body, err := os.ReadFile(userCommand); err != nil || string(body) != "# my notes\n" {
		t.Fatalf("user command changed: %q (err=%v)", body, err)
	}
	if names := mcpServerNames(t, filepath.Join(unrecorded, "mcp.json")); !names["theirs"] || len(names) != 1 {
		t.Fatalf(".omp/mcp.json servers = %v, want the user's only", names)
	}
	want := "  ! .cursor/commands: 3 command copy(ies) not recorded as ai-specs-managed (kept; delete them if ai-specs did not write them)"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr missing %q:\n%s", want, stderr)
	}
	if !strings.Contains(stderr, "  ! .pi/extensions/orphan.ts: generated hook shims not recorded as ai-specs-managed") {
		t.Fatalf("stderr does not report the kept legacy shim:\n%s", stderr)
	}
	if body, err := os.ReadFile(legacyShim); err != nil || !strings.HasPrefix(string(body), hooksTSHeader) {
		t.Fatalf("generated-looking unrecorded shim changed: %q (err=%v)", body, err)
	}
}

// stripLinkRecords removes the [links] section, leaving a lock written before
// ai-specs recorded link ownership.
func stripLinkRecords(t *testing.T, project string) {
	t.Helper()
	path := filepath.Join(project, "ai-specs/.ai-specs.lock")
	var kept []string
	skipping := false
	for _, line := range strings.Split(readLockText(t, project), "\n") {
		if line == "[links]" {
			skipping = true
			continue
		}
		if skipping && line == "" {
			skipping = false
			continue
		}
		if !skipping {
			kept = append(kept, line)
		}
	}
	spineWrite(t, path, strings.Join(kept, "\n"), 0o644)
}

// T3 (S4, B-001): a link only sync itself created carries a record, so a link
// from before 0.26 is kept and named instead of deleted, even when the rest of
// the retired agent's recorded artifacts still go.
func TestSyncRetiredAgentKeepsUnrecordedLinks(t *testing.T) {
	home, project := agentLeftoverFixture(t, "['claude', 'pi']")
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("seed sync stderr = %q", stderr)
	}
	stripLinkRecords(t, project)
	retireAgents(t, project, "['pi']")
	out, stderr := runSyncCaseOutput(t, home, project)

	for _, rel := range []string{"CLAUDE.md", ".claude/skills"} {
		path := filepath.Join(project, rel)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("unrecorded link %s was not kept (err=%v)", rel, err)
		}
	}
	if strings.Contains(out, "removed retired agent link") {
		t.Fatalf("an unrecorded link was deleted:\n%s", out)
	}
	// The agent still has other records, so it is retired and loses them.
	if !strings.Contains(out, "  ✓ removed retired agent command .claude/commands/go.md") {
		t.Fatalf("stdout = %s", out)
	}
	want := "  ! CLAUDE.md, .claude/skills: retired agent link(s) not recorded as ai-specs-managed (kept; delete them if ai-specs did not write them)"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr missing %q:\n%s", want, stderr)
	}
}

func TestSyncRetiredAgentRefusalPrecedesEveryDelete(t *testing.T) {
	home, project := agentLeftoverFixture(t, "['claude', 'cursor', 'pi']")
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("seed sync stderr = %q", stderr)
	}
	// The retired agent's directory now points outside the project — keeping the
	// recorded artifacts, so the plan reaches them — and its recorded files may
	// not be deleted there. Nothing else of the run may be deleted either.
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Rename(filepath.Join(project, ".pi"), outside); err != nil {
		t.Fatal(err)
	}
	spineWrite(t, filepath.Join(outside, "mine.txt"), "outside\n", 0o644)
	pi := filepath.Join(project, ".pi")
	if err := os.Symlink(outside, pi); err != nil {
		t.Fatal(err)
	}
	claudeCommand := filepath.Join(project, ".claude", "commands", "go.md")
	before, err := os.ReadFile(claudeCommand)
	if err != nil {
		t.Fatal(err)
	}

	retireAgents(t, project, "['cursor']")
	var out, stderr bytes.Buffer
	rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &stderr)
	if rc != 1 {
		t.Fatalf("sync rc=%d, want 1 (stdout=%s stderr=%s)", rc, out.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "ERROR: cannot prune agent leftovers: refuse to write ") ||
		!strings.Contains(stderr.String(), ".pi/skills: it resolves outside the project root") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if body, err := os.ReadFile(claudeCommand); err != nil || !bytes.Equal(body, before) {
		t.Fatalf("another agent's recorded command was deleted before the refusal (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "mine.txt")); err != nil {
		t.Fatalf("outside tree changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "extensions", "agentdemo-main.ts")); err != nil {
		t.Fatalf("the outside shim was deleted: %v", err)
	}
}
