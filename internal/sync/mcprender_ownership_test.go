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

	"ai-specs.dev/ai-specs/internal/toml"
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

const mcpOwnershipManifestSame = `[project]
name = 'own'

[mcp.same]
command = 'manifest-cmd'
`

const mcpOwnershipManifestNone = `[project]
name = 'own'
`

const mcpOwnershipManifestAlphaChanged = `[project]
name = 'own'

[mcp.alpha]
command = 'alpha-new'

[mcp.beta]
command = 'beta-cmd'
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
func mcpRenderOwned(t *testing.T, dir, manifest, agent, target, key string, owned []string, blockSHA ...string) (string, string, int, MCPResult) {
	t.Helper()
	manifestPath := filepath.Join(dir, "ai-specs.toml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	opts := RenderMCPOptions{Owned: owned, Recorded: owned != nil}
	if len(blockSHA) > 0 {
		opts.BlockSHA = blockSHA[0]
	}
	res := MCPResult{}
	opts.Result = &res
	var out, errBuf bytes.Buffer
	rc := RenderMCPFile(manifestPath, agent, target, key, opts, &out, &errBuf)
	return out.String(), errBuf.String(), rc, res
}

// mcpNotice is the one upgrade-notice line for target listing servers.
func mcpNotice(target, servers string) string {
	return fmt.Sprintf("    ! %s: MCP servers not recorded as ai-specs-managed: %s (remove them if ai-specs generated them)\n",
		target, servers)
}

// mcpPreservedLine is the collision notice: the user's entry was kept because it
// differs from what sync would render today, plus how to regenerate it.
func mcpPreservedLine(target, name string) string {
	return fmt.Sprintf("    ! preserved user MCP server %q in %s (not recorded as ai-specs-managed; delete that entry and run sync to regenerate it)\n",
		name, target)
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
			_, _, rc, first := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 {
				t.Fatalf("first render rc=%d", rc)
			}
			firstBytes := mcpReadTarget(t, target)
			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, mcpManagedNames(first), first.BlockSHA)
			if rc != 0 || stderr != "" {
				t.Fatalf("second render rc=%d stderr=%q", rc, stderr)
			}
			if got := mcpReadTarget(t, target); got != firstBytes {
				t.Errorf("re-sync is not idempotent\n--- first ---\n%s\n--- second ---\n%s", firstBytes, got)
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
			_, _, rc, first := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 {
				t.Fatalf("first render rc=%d", rc)
			}
			// alpha stays in the manifest: it must be updated in place, not
			// duplicated, while beta disappears and the user server survives.
			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestAlphaOnly, a.agent, target, a.key, mcpManagedNames(first), first.BlockSHA)
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
			_, _, rc, first := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 {
				t.Fatalf("first render rc=%d", rc)
			}
			out, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, a.agent, target, a.key, mcpManagedNames(first), first.BlockSHA)
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
	if want := mcpPreservedLine(target, "alpha"); stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
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

// TestMCPOwnershipUndecodableCodexKeyRefuses pins the refusal for a Codex server
// key whose escapes cannot be decoded safely: guessing would merge two different
// keys, so the render fails atomically and the target keeps its exact bytes.
func TestMCPOwnershipUndecodableCodexKeyRefuses(t *testing.T) {
	for _, header := range []string{`"bad\qname"`, `"\UFFFFFFFF"`} {
		t.Run(header, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			original := "[mcp_servers." + header + "]\ncommand = \"user-cmd\"\n"
			if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}

			manifestPath := filepath.Join(dir, "ai-specs.toml")
			if err := os.WriteFile(manifestPath, []byte(mcpOwnershipManifest), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, errBuf bytes.Buffer
			rc := RenderMCPFile(manifestPath, "codex", target, "mcp_servers", RenderMCPOptions{}, &out, &errBuf)
			if rc != 1 {
				t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out.String(), errBuf.String())
			}
			if lines := strings.Split(strings.TrimSuffix(errBuf.String(), "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
				t.Errorf("stderr is not exactly one error line: %q", errBuf.String())
			}
			if got := mcpReadTarget(t, target); got != original {
				t.Errorf("undecodable key modified the target: got %q want %q", got, original)
			}
		})
	}
}

// TestMCPOwnershipCodexHeaderLookalikesAreContent pins that a table-header line
// inside a multi-line string (basic or literal), a single-line string or a
// comment is content, not a table: pruning a managed server must leave every
// byte of it alone and keep the document parseable.
func TestMCPOwnershipCodexHeaderLookalikesAreContent(t *testing.T) {
	for _, tc := range []struct{ name, user string }{
		{"basic multiline", "user_note = \"\"\"\n[mcp_servers.same]\nthis is user text, not a table\n\"\"\"\n"},
		{"escaped basic multiline", "user_note = \"\"\"\n[mcp_servers.\"\\u0073ame\"]\ntext\n\"\"\"\n"},
		{"literal multiline", "user_note = '''\n[mcp_servers.same]\ntext\n'''\n"},
		{"comment", "# [mcp_servers.same]\n"},
		{"single-line string", "note = \"[mcp_servers.same]\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(tc.user), 0o644); err != nil {
				t.Fatal(err)
			}
			// The lock records `same` as managed but the manifest no longer
			// declares it: the prune must find no table to remove.
			out, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, "codex", target, "mcp_servers", []string{"same"})
			if rc != 0 {
				t.Fatalf("rc=%d stdout=%q stderr=%q", rc, out, stderr)
			}
			if len(res.Removed) != 0 {
				t.Errorf("removed = %v, want none", res.Removed)
			}
			if got := mcpReadTarget(t, target); got != tc.user {
				t.Errorf("header lookalike rewritten:\n--- got ---\n%s\n--- want ---\n%s", got, tc.user)
			}
			if _, err := toml.Parse([]byte(tc.user)); err != nil {
				t.Fatalf("the pinned fixture must stay valid TOML: %v", err)
			}
		})
	}
}

// TestMCPOwnershipCodexQuotedKeysAreOneServer pins that every TOML spelling of a
// name denotes the same server: a quoted key is matched (and preserved), never
// duplicated into a second table.
func TestMCPOwnershipCodexQuotedKeysAreOneServer(t *testing.T) {
	for _, header := range []string{`"same"`, `"\u0073ame"`, `'same'`, `same`} {
		t.Run(header, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			original := "[mcp_servers." + header + "]\ncommand = \"user-cmd\"\n"
			if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
			if rc != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, stderr)
			}
			if len(res.Preserved) != 1 || res.Preserved[0] != "same" {
				t.Errorf("preserved = %v, want [same]", res.Preserved)
			}
			if got := mcpReadTarget(t, target); got != original {
				t.Errorf("preserved user table changed:\n--- got ---\n%s\n--- want ---\n%s", got, original)
			}
		})
	}
}

// TestMCPOwnershipCodexHeaderlessServerIsPreserved pins the "if in doubt, don't
// touch" rule for a user server declared without its own table header — an
// inline table or a dotted key under [<mcp_key>], or a root-level dotted key:
// it counts as a present user server, so a manifest server with the same name is
// preserved and no table is emitted that would redeclare it.
func TestMCPOwnershipCodexHeaderlessServerIsPreserved(t *testing.T) {
	for _, tc := range []struct{ name, path, user, want string }{
		{
			name: "inline table under the parent table",
			path: ".codex/config.toml",
			user: "[mcp_servers]\nsame = { command = \"user\" }\n",
			want: "[mcp_servers]\nsame = { command = \"user\" }\n",
		},
		{
			name: "dotted key under the parent table",
			path: ".codex/config.toml",
			user: "[mcp_servers]\nsame.command = \"user\"\n",
			want: "[mcp_servers]\nsame.command = \"user\"\n",
		},
		{
			name: "root-level dotted key",
			path: ".codex/config.toml",
			user: "mcp_servers.same.command = \"user\"\n",
			want: "mcp_servers.same.command = \"user\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(tc.user), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := toml.Parse([]byte(tc.user)); err != nil {
				t.Fatalf("the pinned fixture must be valid TOML: %v", err)
			}

			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
			if rc != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, stderr)
			}
			if len(res.Preserved) != 1 || res.Preserved[0] != "same" {
				t.Errorf("preserved = %v, want [same]", res.Preserved)
			}
			if got := mcpReadTarget(t, target); got != tc.want {
				t.Errorf("headerless user server rewritten:\n--- got ---\n%s\n--- want ---\n%s", got, tc.want)
			}
			if _, err := toml.Parse([]byte(mcpReadTarget(t, target))); err != nil {
				t.Errorf("the merged document is not valid TOML: %v", err)
			}
		})
	}
}

// TestMCPOwnershipCodexUnparseableKeyRefuses pins the refusal for a key line the
// scanner cannot read: the render fails atomically and the target keeps its
// exact bytes.
func TestMCPOwnershipCodexUnparseableKeyRefuses(t *testing.T) {
	for _, tc := range []struct{ name, user string }{
		{"undecodable key under the parent table", "[mcp_servers]\n\"bad\\qname\" = { command = \"user\" }\n"},
		{"undecodable root key", "mcp_servers.\"bad\\qname\".command = \"user\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(tc.user), 0o644); err != nil {
				t.Fatal(err)
			}
			out, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
			if rc != 1 {
				t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out, stderr)
			}
			if lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
				t.Errorf("stderr is not exactly one error line: %q", stderr)
			}
			if len(res.Managed) != 0 {
				t.Errorf("managed = %v, want none", res.Managed)
			}
			if got := mcpReadTarget(t, target); got != tc.user {
				t.Errorf("failed render modified the target: got %q want %q", got, tc.user)
			}
		})
	}
}

// TestMCPOwnershipCodexArrayRowDoesNotHideServer pins the managed-block contract
// for the case the old scanner misread: an array row that looks like a header
// cannot change what ai-specs writes, because only the marker block is ours.
// The root-level dotted server is user content outside the block, so it is
// preserved (with the collision warning) and the file stays byte-identical.
func TestMCPOwnershipCodexArrayRowDoesNotHideServer(t *testing.T) {
	user := "user_choices = [\n  [\"same\"]\n]\nmcp_servers.same.command = \"user\"\n"
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := toml.Parse([]byte(user)); err != nil {
		t.Fatalf("the pinned fixture must be valid TOML: %v", err)
	}

	_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, stderr)
	}
	if len(res.Preserved) != 1 || res.Preserved[0] != "same" {
		t.Errorf("preserved = %v, want [same]", res.Preserved)
	}
	if !strings.Contains(stderr, `preserved user MCP server "same"`) {
		t.Errorf("no collision warning: %q", stderr)
	}
	if got := mcpReadTarget(t, target); got != user {
		t.Errorf("user bytes changed:\n--- got ---\n%s\n--- want ---\n%s", got, user)
	}
}

// TestMCPOwnershipCodexUnparsableTargetRefuses pins the pre-merge gate: a Codex
// target the real parser cannot read is never touched (it is user config, not an
// S4 JSON target, so there is no backup/regenerate).
func TestMCPOwnershipCodexUnparsableTargetRefuses(t *testing.T) {
	unparsable := "[mcp_servers.other]\ncommand = \"x\n"
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(unparsable), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := toml.Parse([]byte(unparsable)); err == nil {
		t.Fatal("the pinned fixture must not parse")
	}

	out, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
	if rc != 1 {
		t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out, stderr)
	}
	if lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
		t.Errorf("stderr is not exactly one error line: %q", stderr)
	}
	if len(res.Managed) != 0 {
		t.Errorf("managed = %v, want none", res.Managed)
	}
	if got := mcpReadTarget(t, target); got != unparsable {
		t.Errorf("unparsable target modified: got %q want %q", got, unparsable)
	}
	if _, err := os.Stat(target + ".bak"); !os.IsNotExist(err) {
		t.Errorf("a TOML target must not be backed up: stat err=%v", err)
	}
}

// TestMCPOwnershipCodexUntrustedMarkersRefuse pins the split validation: marker
// text that does not delimit a real block — an example inside a user multi-line
// string, or a begin marker moved above a user table — is refused with the file
// byte-identical instead of being treated as ours.
func TestMCPOwnershipCodexUntrustedMarkersRefuse(t *testing.T) {
	for _, tc := range []struct{ name, user string }{
		{
			name: "marker example inside a user string",
			user: "developer_instructions = \"\"\"\nExample:\n" +
				mcpTOMLBlockBegin + "\n[mcp_servers.same]\ncommand=\"example\"\n" +
				mcpTOMLBlockEnd + "\nEnd example.\n\"\"\"\n",
		},
		{
			name: "begin marker above a user table",
			user: mcpTOMLBlockBegin + "\n[features]\nshell_tool = true\n\n" +
				"[mcp_servers.same]\ncommand = \"managed\"\n\n" + mcpTOMLBlockEnd + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(tc.user), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := toml.Parse([]byte(tc.user)); err != nil {
				t.Fatalf("the pinned fixture must be valid TOML: %v", err)
			}
			out, stderr, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
			if rc != 1 {
				t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out, stderr)
			}
			if lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
				t.Errorf("stderr is not exactly one error line: %q", stderr)
			}
			if got := mcpReadTarget(t, target); got != tc.user {
				t.Errorf("refused rewrite modified the target: got %q want %q", got, tc.user)
			}
		})
	}
}

// TestMCPOwnershipCodexDottedNameIsOneQuotedKey pins that a manifest server name
// which is not a bare TOML key is written as one quoted key: it must not nest
// into a user server of the same first segment.
func TestMCPOwnershipCodexDottedNameIsOneQuotedKey(t *testing.T) {
	user := "[mcp_servers.dev]\ncommand = \"user\"\n"
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "[project]\nname = 'own'\n\n[mcp.\"dev.server\"]\ncommand = 'owned'\n"
	_, stderr, rc, res := mcpRenderOwned(t, dir, manifest, "codex", target, "mcp_servers", nil)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, stderr)
	}
	if got := mcpManagedNames(res); len(got) != 1 || got[0] != "dev.server" {
		t.Errorf("managed = %v, want [dev.server]", got)
	}
	parsed, err := toml.Parse([]byte(mcpReadTarget(t, target)))
	if err != nil {
		t.Fatalf("merged document does not parse: %v", err)
	}
	servers, _ := parsed.Table("mcp_servers")
	dev, _ := servers.Table("dev")
	if cmd, _ := dev.String("command"); cmd != "user" {
		t.Errorf("user server changed: %q", cmd)
	}
	owned, _ := servers.Table("dev.server")
	if cmd, _ := owned.String("command"); cmd != "owned" {
		t.Errorf("dotted managed server is not one key at mcp_servers.dev.server: %v", servers.Keys())
	}
}

// TestMCPOwnershipCodexBlockRequiresOwnershipRecord pins the ownership guard: a
// block that wraps a server the lock does not record (a hand-moved end marker)
// is refused with the file byte-identical, so a prune can never delete a server
// that was never ours.
func TestMCPOwnershipCodexBlockRequiresOwnershipRecord(t *testing.T) {
	block := mcpTOMLBlockBegin + "\n[mcp_servers.same]\ncommand = \"managed\"\n\n" +
		"[mcp_servers.user_owned]\ncommand = \"keep-user\"\n\n" + mcpTOMLBlockEnd + "\n"
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(block), 0o644); err != nil {
		t.Fatal(err)
	}
	// `same` is recorded, `user_owned` is not. The fixture's own hash is passed so
	// the hash guard accepts the block and the ownership guard is what refuses.
	blockSHA := mcpTOMLSHA256(block)
	out, stderr, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, "codex", target, "mcp_servers", []string{"same"}, blockSHA)
	if rc != 1 {
		t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out, stderr)
	}
	if !strings.Contains(stderr, `"user_owned"`) {
		t.Errorf("refusal does not name the unowned server: %q", stderr)
	}
	if lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
		t.Errorf("stderr is not exactly one error line: %q", stderr)
	}
	if got := mcpReadTarget(t, target); got != block {
		t.Errorf("refused rewrite modified the target: got %q want %q", got, block)
	}

	// A block with no record at all is refused the same way.
	if _, stderr, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, "codex", target, "mcp_servers", nil, blockSHA); rc != 1 || !strings.Contains(stderr, "not recorded") {
		t.Errorf("no-record block: rc=%d stderr=%q", rc, stderr)
	}
}

// TestMCPOwnershipCodexNonBMPNameRoundTrips pins the TOML quoting of a non-BMP
// managed name: JSON surrogate-pair escapes are not valid TOML, so the key must
// use a \\U code point escape (or raw UTF-8) and survive a re-parse.
func TestMCPOwnershipCodexNonBMPNameRoundTrips(t *testing.T) {
	name := "dev🙂"
	manifest := "[project]\nname = 'own'\n\n[mcp.\"" + name + "\"]\ncommand = 'owned'\n"
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, rc, res := mcpRenderOwned(t, dir, manifest, "codex", target, "mcp_servers", nil)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, stderr)
	}
	if got := mcpManagedNames(res); len(got) != 1 || got[0] != name {
		t.Errorf("managed = %v, want [%s]", got, name)
	}
	text := mcpReadTarget(t, target)
	parsed, err := toml.Parse([]byte(text))
	if err != nil {
		t.Fatalf("merged document does not parse: %v\n%s", err, text)
	}
	servers, _ := parsed.Table("mcp_servers")
	if _, ok := servers.Table(name); !ok {
		t.Errorf("non-BMP server not at mcp_servers.%s: %v\n%s", name, servers.Keys(), text)
	}
}

// TestMCPOwnershipCodexBlockCommentRefuses pins that ai-specs writes no comments
// inside its block: a comment line between the markers is user text the marker
// cut would delete, so the render refuses with the file byte-identical.
func TestMCPOwnershipCodexBlockCommentRefuses(t *testing.T) {
	// The end marker sits between the user's comment and the user's table, so the
	// comment is inside the markers while the table stays outside them.
	block := mcpTOMLBlockBegin + "\n[mcp_servers.same]\ncommand = \"managed\"\n\n" +
		"# User note for features: keep enabled on this machine.\n" +
		mcpTOMLBlockEnd + "\n[features]\nshell_tool = true\n"
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(block), 0o644); err != nil {
		t.Fatal(err)
	}
	out, stderr, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, "codex", target, "mcp_servers", []string{"same"})
	if rc != 1 {
		t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out, stderr)
	}
	if !strings.Contains(stderr, "comment") {
		t.Errorf("refusal does not name the comment: %q", stderr)
	}
	if lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
		t.Errorf("stderr is not exactly one error line: %q", stderr)
	}
	if got := mcpReadTarget(t, target); got != block {
		t.Errorf("refused rewrite modified the target: got %q want %q", got, block)
	}
}

// TestMCPOwnershipCodexEditedBlockRefuses pins the block-hash guard: once the
// lock records the exact block bytes ai-specs wrote, any hand edit inside them —
// an edited value, an inline comment on a generated line — refuses the render
// with the target byte-identical instead of overwriting the user's change.
func TestMCPOwnershipCodexEditedBlockRefuses(t *testing.T) {
	for _, tc := range []struct{ name, edit string }{
		{"edited value", "managed-edit"},
		{"inline comment on a generated line", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			_, _, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
			if rc != 0 || res.BlockSHA == "" {
				t.Fatalf("first render rc=%d blockSHA=%q", rc, res.BlockSHA)
			}
			written := mcpReadTarget(t, target)
			edited := strings.Replace(written, `command = "manifest-cmd"`, `command = "`+tc.edit+`"`, 1)
			if tc.edit == "" {
				edited = strings.Replace(written, "[mcp_servers.same]\n", "[mcp_servers.same] # mine\n", 1)
			}
			if edited == written {
				t.Fatalf("fixture did not change: %q", written)
			}
			if err := os.WriteFile(target, []byte(edited), 0o644); err != nil {
				t.Fatal(err)
			}
			out, stderr, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", []string{"same"}, res.BlockSHA)
			if rc != 1 {
				t.Fatalf("rc=%d want 1 (stdout %q stderr %q)", rc, out, stderr)
			}
			if !strings.Contains(stderr, "edited by hand") {
				t.Errorf("refusal does not explain the edit: %q", stderr)
			}
			if lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
				t.Errorf("stderr is not exactly one error line: %q", stderr)
			}
			if got := mcpReadTarget(t, target); got != edited {
				t.Errorf("refused rewrite modified the target: got %q want %q", got, edited)
			}
		})
	}
}

// TestMCPOwnershipCodexUnchangedBlockIsStable pins the other half of the guard:
// an untouched block keeps its recorded hash across syncs (so an update and a
// prune both proceed) and the bytes are identical every time.
func TestMCPOwnershipCodexUnchangedBlockIsStable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", nil)
	if rc != 0 {
		t.Fatalf("first render rc=%d stderr=%q", rc, stderr)
	}
	first := mcpReadTarget(t, target)
	// A second sync with the recorded hash and the same manifest changes nothing.
	_, stderr, rc, again := mcpRenderOwned(t, dir, mcpOwnershipManifestSame, "codex", target, "mcp_servers", []string{"same"}, res.BlockSHA)
	if rc != 0 || stderr != "" {
		t.Fatalf("second render rc=%d stderr=%q", rc, stderr)
	}
	if again.BlockSHA != res.BlockSHA {
		t.Errorf("block hash changed across syncs: %q → %q", res.BlockSHA, again.BlockSHA)
	}
	if got := mcpReadTarget(t, target); got != first {
		t.Errorf("re-sync changed the block:\n--- first ---\n%s\n--- second ---\n%s", first, got)
	}
	// A prune (manifest server gone) still proceeds on the untouched block.
	_, stderr, rc, pruned := mcpRenderOwned(t, dir, mcpOwnershipManifestNone, "codex", target, "mcp_servers", []string{"same"}, again.BlockSHA)
	if rc != 0 || stderr != "" {
		t.Fatalf("prune rc=%d stderr=%q", rc, stderr)
	}
	if pruned.BlockSHA != "" {
		t.Errorf("pruned block hash = %q, want none", pruned.BlockSHA)
	}
	if got := mcpReadTarget(t, target); got != "" {
		t.Errorf("pruned target = %q, want empty", got)
	}
}

// TestMCPOwnershipAdoptsIdenticalLegacyServers pins L4 (B-002): an existing
// server the lock does not record is adopted — recorded as ai-specs-managed and
// from then on updated and removed like any managed one — only when its entry is
// exactly what sync would render today. This is the v0.25.2 → v0.26 upgrade path,
// where the generated entries carry no record at all.
func TestMCPOwnershipAdoptsIdenticalLegacyServers(t *testing.T) {
	for _, a := range mcpOwnershipAgents {
		if a.agent == "codex" {
			// Codex TOML keeps its managed-block contract (rule: unchanged).
			continue
		}
		t.Run(a.agent, func(t *testing.T) {
			dir := t.TempDir()
			target := mcpSeedTarget(t, dir, a)
			// A pre-0.26 sync generated the entries; the record never existed.
			if _, _, rc, _ := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil); rc != 0 {
				t.Fatalf("seed render rc=%d", rc)
			}
			legacy := mcpReadTarget(t, target)

			// Identical entries are adopted: no collision line, still the only
			// remaining notice is the undeclared user server.
			_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, a.agent, target, a.key, nil)
			if rc != 0 || stderr != mcpNotice(target, "mine") {
				t.Fatalf("adoption render rc=%d stderr=%q want %q", rc, stderr, mcpNotice(target, "mine"))
			}
			if got, want := mcpManagedNames(res), []string{"alpha", "beta"}; strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("adopted managed = %v, want %v", got, want)
			}
			if got := mcpReadTarget(t, target); got != legacy {
				t.Errorf("adoption rewrote identical entries:\n--- got ---\n%s\n--- want ---\n%s", got, legacy)
			}

			// Adopted means managed: a changed manifest value is applied.
			_, stderr, rc, updated := mcpRenderOwned(t, dir, mcpOwnershipManifestAlphaChanged, a.agent, target, a.key, mcpManagedNames(res))
			if rc != 0 || stderr != "" {
				t.Fatalf("update render rc=%d stderr=%q", rc, stderr)
			}
			if got, want := mcpManagedNames(updated), []string{"alpha", "beta"}; strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("updated managed = %v, want %v", got, want)
			}
			text := mcpReadTarget(t, target)
			if !strings.Contains(text, "alpha-new") || strings.Contains(text, "alpha-cmd") {
				t.Errorf("adopted server was not updated from the manifest:\n%s", text)
			}
			if !strings.Contains(text, a.marker) {
				t.Errorf("update lost the user server:\n%s", text)
			}

			// Adopted also means removable when the manifest drops it.
			_, stderr, rc, _ = mcpRenderOwned(t, dir, mcpOwnershipManifestNone, a.agent, target, a.key, mcpManagedNames(updated))
			if rc != 0 || stderr != "" {
				t.Fatalf("prune render rc=%d stderr=%q", rc, stderr)
			}
			if text := mcpReadTarget(t, target); strings.Contains(text, "alpha") || strings.Contains(text, "beta") {
				t.Errorf("adopted servers survived the manifest removal:\n%s", text)
			}
		})
	}
}

// TestMCPOwnershipAdoptionIgnoresTheRecord pins the two limits of L4 (B-002):
// adoption does not depend on whether the target has a record (so pruning the
// last managed server, which drops the record, cannot re-arm anything), and a
// differing entry is never adopted.
func TestMCPOwnershipAdoptionIgnoresTheRecord(t *testing.T) {
	t.Run("identical entry with a record naming another server", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, ".mcp.json")
		original := `{"mcpServers": {"alpha": {"command": "alpha-cmd", "args": [], "env": {}}}}` + "\n"
		if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifest, "claude", target, "mcpServers", []string{"other"})
		if rc != 0 || stderr != "" {
			t.Fatalf("rc=%d stderr=%q", rc, stderr)
		}
		if got, want := mcpManagedNames(res), []string{"alpha", "beta"}; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("managed = %v, want %v", got, want)
		}
	})

	for _, tc := range []struct {
		name  string
		owned []string
	}{
		{"record without the name", []string{}},
		{"no record at all", nil},
	} {
		t.Run("differing entry/"+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, ".mcp.json")
			original := `{"mcpServers": {"alpha": {"command": "user-authored"}}}` + "\n"
			if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			// The record is gone: the last managed server was just pruned.
			for i := 0; i < 2; i++ {
				_, stderr, rc, res := mcpRenderOwned(t, dir, mcpOwnershipManifestAlphaOnly, "claude", target, "mcpServers", tc.owned)
				if rc != 0 {
					t.Fatalf("rc=%d stderr=%q", rc, stderr)
				}
				if want := mcpPreservedLine(target, "alpha"); stderr != want {
					t.Fatalf("run %d stderr = %q, want %q", i, stderr, want)
				}
				if got := mcpManagedNames(res); len(got) != 0 {
					t.Errorf("run %d managed = %v, want none", i, got)
				}
				if got := mcpReadTarget(t, target); got != original {
					t.Errorf("run %d user server was overwritten: got %q want %q", i, got, original)
				}
			}
		})
	}
}
