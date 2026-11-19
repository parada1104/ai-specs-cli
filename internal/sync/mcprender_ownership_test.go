package sync

// S5 (MCP ownership): sync records the server names it manages per agent
// target and touches only those, so user-authored servers survive, a server
// removed from the manifest is deleted even when none remain, and a name
// collision is resolved conservatively in favour of the user's entry.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const mcpOwnershipManifest = `[project]
name = 'own'

[mcp.alpha]
command = 'alpha-cmd'

[mcp.beta]
command = 'beta-cmd'
`

const mcpOwnershipManifestAlphaOnly = `[project]
name = 'own'

[mcp.alpha]
command = 'alpha-cmd'
`

const mcpOwnershipManifestNone = `[project]
name = 'own'
`

type mcpOwnershipAgent struct {
	agent  string
	rel    string
	key    string
	user   string // the target text seeded with one user-authored server
	marker string // the user entry's marker that must survive
}

var mcpOwnershipAgents = []mcpOwnershipAgent{
	{"claude", ".mcp.json", "mcpServers",
		"{\"top\": 1, \"mcpServers\": {\"mine\": {\"command\": \"user-cmd\"}}}\n", `"mine"`},
	{"cursor", ".cursor/mcp.json", "mcpServers",
		"{\"mcpServers\": {\"mine\": {\"command\": \"user-cmd\"}}}\n", `"mine"`},
	{"opencode", "opencode.json", "mcp",
		"{\"theme\": \"dark\", \"mcp\": {\"mine\": {\"type\": \"local\"}}}\n", `"mine"`},
	{"gemini", ".gemini/settings.json", "mcpServers",
		"{\"mcpServers\": {\"mine\": {\"command\": \"user-cmd\"}}}\n", `"mine"`},
	{"pi", ".pi/mcp.json", "mcpServers",
		"{\"mcpServers\": {\"mine\": {\"command\": \"user-cmd\"}}}\n", `"mine"`},
	{"omp", ".omp/mcp.json", "mcpServers",
		"{\"mcpServers\": {\"mine\": {\"command\": \"user-cmd\"}}}\n", `"mine"`},
	{"codex", ".codex/config.toml", "mcp_servers",
		"# user comment\n\n[user_table]\nx = 1\n\n[mcp_servers.mine]\ncommand = \"user-cmd\"\n",
		"[mcp_servers.mine]"},
}

// mcpRenderOwned runs one render with the recorded ownership and returns the
// stdout, stderr, rc and ownership outcome. owned nil models "no record entry
// yet" (so the upgrade notice applies); a non-nil slice models a record.
func mcpRenderOwned(t *testing.T, dir, manifest, agent, target, key string, owned []string) (string, string, int, MCPResult) {
	t.Helper()
	manifestPath := filepath.Join(dir, "ai-specs.toml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var out, errBuf bytes.Buffer
	res := MCPResult{}
	rc := RenderMCPFile(manifestPath, agent, target, key, RenderMCPOptions{
		Owned:    owned,
		Recorded: owned != nil,
		Result:   &res,
	}, &out, &errBuf)
	return out.String(), errBuf.String(), rc, res
}

// mcpNotice is the one upgrade-notice line for target listing servers.
func mcpNotice(target, servers string) string {
	return fmt.Sprintf("    ! %s: MCP servers not recorded as ai-specs-managed: %s (remove them if ai-specs generated them)\n",
		target, servers)
}

// mcpSeedTarget writes the target file, creating its parent directory.
func mcpSeedTarget(t *testing.T, dir string, a mcpOwnershipAgent) string {
	t.Helper()
	target := filepath.Join(dir, filepath.FromSlash(a.rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(a.user), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	return target
}

func mcpManagedNames(res MCPResult) []string {
	names := make([]string, 0, len(res.Managed))
	for name := range res.Managed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mcpReadTarget(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	return string(data)
}

func TestMCPOwnershipKeepsUserServersAcrossAgents(t *testing.T) {
	for _, a := range mcpOwnershipAgents {
		t.Run(a.agent, func(t *testing.T) {
			dir := t.TempDir()
			target := mcpSeedTarget(t, dir, a)
			if _, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil); rc != 0 || stderr != mcpNotice(target, "mine") {
				t.Fatalf("rc=%d stderr=%q", rc, stderr)
			} else if got, want := mcpManagedNames(res), []string{"alpha", "beta"}; strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("managed = %v, want %v", got, want)
			}
			text := mcpReadTarget(t, target)
			for _, want := range []string{a.marker, "alpha", "beta"} {
				if !strings.Contains(text, want) {
					t.Errorf("target lost %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestMCPOwnershipResyncIsIdempotent(t *testing.T) {
	for _, a := range mcpOwnershipAgents {
		t.Run(a.agent, func(t *testing.T) {
			dir := t.TempDir()
			target := mcpSeedTarget(t, dir, a)
			_, _, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 {
				t.Fatalf("first render rc=%d", rc)
			}
			first := mcpReadTarget(t, target)
			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, mcpManagedNames(res))
			if rc != 0 || stderr != "" {
				t.Fatalf("second render rc=%d stderr=%q", rc, stderr)
			}
			if got := mcpReadTarget(t, target); got != first {
				t.Errorf("re-sync is not idempotent\n--- first ---\n%s\n--- second ---\n%s", first, got)
			} else if !strings.Contains(got, a.marker) {
				t.Errorf("re-sync dropped the user server %q:\n%s", a.marker, got)
			}
			if got, want := mcpManagedNames(res), []string{"alpha", "beta"}; strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("managed = %v, want %v", got, want)
			}
		})
	}
}

func TestMCPOwnershipRemovesOnlyManagedServers(t *testing.T) {
	for _, a := range mcpOwnershipAgents {
		t.Run(a.agent+"/manifest removal", func(t *testing.T) {
			dir := t.TempDir()
			target := mcpSeedTarget(t, dir, a)
			_, _, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 {
				t.Fatalf("first render rc=%d", rc)
			}
			// alpha stays in the manifest: it must be updated in place, not
			// duplicated, while beta disappears and the user server survives.
			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestAlphaOnly, a.agent, target, a.key, mcpManagedNames(res))
			if rc != 0 || stderr != "" {
				t.Fatalf("second render rc=%d stderr=%q", rc, stderr)
			}
			text := mcpReadTarget(t, target)
			if strings.Contains(text, "beta") {
				t.Errorf("removed manifest server survived:\n%s", text)
			}
			if !strings.Contains(text, a.marker) || !strings.Contains(text, "alpha") {
				t.Errorf("kept entries lost:\n%s", text)
			}
			if got := mcpManagedNames(res); len(got) != 1 || got[0] != "alpha" {
				t.Errorf("managed = %v, want [alpha]", got)
			}
			if len(res.Removed) != 1 || res.Removed[0] != "beta" {
				t.Errorf("removed = %v, want [beta]", res.Removed)
			}
		})

		t.Run(a.agent+"/zero servers", func(t *testing.T) {
			dir := t.TempDir()
			target := mcpSeedTarget(t, dir, a)
			_, _, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 {
				t.Fatalf("first render rc=%d", rc)
			}
			out, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, a.agent, target, a.key, mcpManagedNames(res))
			if rc != 0 || stderr != "" {
				t.Fatalf("prune render rc=%d stderr=%q out=%q", rc, stderr, out)
			}
			text := mcpReadTarget(t, target)
			if strings.Contains(text, "alpha") || strings.Contains(text, "beta") {
				t.Errorf("stale managed servers survived with zero servers left:\n%s", text)
			}
			if !strings.Contains(text, a.marker) {
				t.Errorf("user server lost while pruning:\n%s", text)
			}
			if got := mcpManagedNames(res); len(got) != 0 {
				t.Errorf("managed = %v, want none", got)
			}
		})
	}
}

func TestMCPOwnershipCollisionKeepsUserServer(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".mcp.json")
	original := `{"mcpServers": {"alpha": {"command": "user-authored"}}}` + "\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, "claude", target, "mcpServers", nil)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, stderr)
	}
	if !strings.HasPrefix(stderr, "    ! preserved user MCP server \"alpha\"") {
		t.Fatalf("stderr = %q", stderr)
	}
	text := mcpReadTarget(t, target)
	if !strings.Contains(text, "user-authored") {
		t.Errorf("collision overwrote the user-authored server:\n%s", text)
	}
	if !strings.Contains(stderr, `"alpha"`) {
		t.Errorf("no warning naming the preserved server: %q", stderr)
	}
	if got := mcpManagedNames(res); len(got) != 1 || got[0] != "beta" {
		t.Errorf("managed = %v, want [beta]", got)
	}
	if len(res.Preserved) != 1 || res.Preserved[0] != "alpha" {
		t.Errorf("preserved = %v, want [alpha]", res.Preserved)
	}
	// The decision is stable: a second sync keeps the same user server.
	if _, _, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifest, "claude", target, "mcpServers", mcpManagedNames(res)); rc != 0 {
		t.Fatalf("second render rc=%d", rc)
	}
	if got := mcpReadTarget(t, target); !strings.Contains(got, "user-authored") {
		t.Errorf("re-sync overwrote the user-authored server:\n%s", got)
	}
}

func TestMCPOwnershipCodexEncodingErrorLeavesTargetIntact(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe-mcp.json")
	if err := os.WriteFile(recipe, []byte(`{"\ud800": {"command": "d"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "ai-specs.toml")
	if err := os.WriteFile(manifestPath, []byte(mcpOwnershipManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "[user_table]\nx = 1\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	rc := RenderMCPFile(manifestPath, "codex", target, "mcp_servers", RenderMCPOptions{
		RecipeMCPPath: recipe,
	}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("rc=%d want 1 (stdout %q)", rc, out.String())
	}
	if got := mcpReadTarget(t, target); got != original {
		t.Errorf("failed render modified the target: got %q want %q", got, original)
	}
	if _, err := os.Stat(target + ".bak"); !os.IsNotExist(err) {
		t.Errorf("failed render created a backup: stat err=%v", err)
	}
}
