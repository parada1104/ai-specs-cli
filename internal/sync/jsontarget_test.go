package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHooksJSONTargetRecovery(t *testing.T) {
	for _, agent := range []struct{ name, path, event string }{
		{"claude", ".claude/settings.json", "PreToolUse"},
		{"cursor", ".cursor/hooks.json", "beforeShellExecution"},
	} {
		t.Run(agent.name, func(t *testing.T) {
			user := fmt.Sprintf(`{"user":42,"hooks":{%q:[{"command":"user hook"}]}}`, agent.event)
			testJSONTargetRecovery(t, agent.path, user, "", func(t *testing.T, root, _ string) (string, int, string) {
				input := filepath.Join(root, "resolved.json")
				spineWrite(t, input, `{"hooks":[{"recipe":"demo","id":"check","event":"pre-tool-use","matcher":"Bash","script_path":"hooks/check.sh"}]}`, 0o644)
				var out, stderr bytes.Buffer
				rc := RenderHooks(input, agent.name, root, &out, &stderr)
				return out.String(), rc, stderr.String()
			}, func(t *testing.T, doc map[string]json.RawMessage, valid bool) {
				var hooks map[string][]map[string]any
				if err := json.Unmarshal(doc["hooks"], &hooks); err != nil {
					t.Fatal(err)
				}
				bucket := hooks[agent.event]
				if len(bucket) == 0 || bucket[len(bucket)-1]["_ai_specs_managed"] != "ai-specs:hooks:demo:check" {
					t.Errorf("managed hook not regenerated: %v", bucket)
				}
				if valid && (len(bucket) != 2 || bucket[0]["command"] != "user hook") {
					t.Errorf("user hook not merged: %v", bucket)
				}
			})
		})
	}
}

func TestMCPJSONTargetRecovery(t *testing.T) {
	for _, agent := range []struct{ name, path, key, command string }{
		{"claude", ".mcp.json", "mcpServers", `"npx"`},
		{"cursor", ".cursor/mcp.json", "mcpServers", `"npx"`},
		{"opencode", "opencode.json", "mcp", `["npx","-y","@scope/server"]`},
		{"gemini", ".gemini/settings.json", "mcpServers", `"npx"`},
		{"pi", ".pi/mcp.json", "mcpServers", `"npx"`},
		{"omp", ".omp/mcp.json", "mcpServers", `"npx"`},
	} {
		t.Run(agent.name, func(t *testing.T) {
			user := fmt.Sprintf(`{"user":42,%q:{"old":{"command":"old"}}}`, agent.key)
			testJSONTargetRecovery(t, agent.path, user, "  ✓ "+agent.name+" → %s (3 server(s))\n", func(t *testing.T, root, path string) (string, int, string) {
				return mcpRunRender(t, root, agent.name, path, agent.key, "")
			}, func(t *testing.T, doc map[string]json.RawMessage, preserved bool) {
				var servers map[string]map[string]json.RawMessage
				if err := json.Unmarshal(doc[agent.key], &servers); err != nil {
					t.Fatal(err)
				}
				// S5: a readable target keeps the unrecorded user server; a
				// regenerated one has nothing left to preserve.
				want := 3
				if preserved {
					want = 4
					if got := string(servers["old"]["command"]); got != `"old"` {
						t.Errorf("user server not preserved: %q", got)
					}
				}
				var command bytes.Buffer
				if err := json.Compact(&command, servers["alpha"]["command"]); len(servers) != want || err != nil || command.String() != agent.command {
					t.Errorf("servers not regenerated as expected: %s", doc[agent.key])
				}
				if agent.name == "opencode" && string(doc["$schema"]) != `"https://opencode.ai/config.json"` {
					t.Errorf("OpenCode schema: %s", doc["$schema"])
				}
			})
		})
	}
}

func testJSONTargetRecovery(t *testing.T, rel, valid, stdout string, run func(*testing.T, string, string) (string, int, string), check func(*testing.T, map[string]json.RawMessage, bool)) {
	t.Helper()
	for _, tc := range []struct{ name, original, backup string }{
		{"missing", "", ""},
		{"valid", valid, ""},
		{"trailing comma", "{\r\n  \"user\": 42,\r\n}\r\n", ".bak"},
		{"backups occupied", "{\n // keep this comment\n}\n", ".bak.2"},
		{"invalid UTF-8", "{\"user\":\"\xff\"}", ".bak"},
		{"empty", "", ".bak"},
		{"non-JSON numeric constants", "{\"nan\":NaN,\"positive\":Infinity,\"negative\":-Infinity}\n", ".bak"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, rel)
			wantBackups := map[string]string{}
			if tc.name != "missing" {
				spineWrite(t, target, tc.original, 0o644)
			}
			if tc.backup == ".bak.2" {
				for suffix, body := range map[string]string{".bak": "first backup\n", ".bak.1": "second backup\n"} {
					spineWrite(t, target+suffix, body, 0o644)
					wantBackups[target+suffix] = body
				}
			}
			wantErr, wantOut := "", stdout
			if tc.backup != "" {
				wantErr = fmt.Sprintf("warning: invalid JSON in %s; backed up to %s\n", target, target+tc.backup)
				wantBackups[target+tc.backup] = tc.original
			}
			if stdout != "" {
				wantOut = fmt.Sprintf(stdout, target)
			}
			if out, rc, stderr := run(t, root, target); rc != 0 || out != wantOut || stderr != wantErr {
				t.Errorf("rc=%d stdout=%q stderr=%q; want rc=0 stdout=%q stderr=%q", rc, out, stderr, wantOut, wantErr)
			}
			backups, err := filepath.Glob(target + ".bak*")
			if err != nil || len(backups) != len(wantBackups) {
				t.Errorf("backups=%v err=%v, want %v", backups, err, wantBackups)
			}
			for path, want := range wantBackups {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Errorf("backup %s = %q (%v), want %q", path, got, err, want)
				}
			}
			raw, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]json.RawMessage
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			wantUser := ""
			if tc.name == "valid" {
				wantUser = "42"
			}
			if got := string(doc["user"]); got != wantUser {
				t.Errorf("user key = %q; want %q", got, wantUser)
			}
			check(t, doc, tc.name == "valid")
		})
	}
}

func TestMCPJSONTargetRecoveryDryRun(t *testing.T) {
	dir := t.TempDir()
	manifest, target := filepath.Join(dir, "manifest.toml"), filepath.Join(dir, ".mcp.json")
	spineWrite(t, manifest, "[mcp.demo]\ncommand = 'echo'\n", 0o644)
	const original = `{"user":42,}`
	spineWrite(t, target, original, 0o644)
	var out, stderr bytes.Buffer
	rc := RenderMCPFile(manifest, "claude", target, "mcpServers", RenderMCPOptions{DryRun: true}, &out, &stderr)
	wantOut := fmt.Sprintf("--- %s (dry-run) ---\n{\n  \"mcpServers\": {\n    \"demo\": {\n      \"command\": \"echo\",\n      \"args\": [],\n      \"env\": {}\n    }\n  }\n}\n\n", target)
	if rc != 0 || out.String() != wantOut || stderr.Len() != 0 {
		t.Errorf("rc=%d stdout=%q stderr=%q", rc, out.String(), stderr.String())
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != original {
		t.Errorf("dry-run changed target: %q (%v)", raw, err)
	}
	if _, err := os.Lstat(target + ".bak"); !os.IsNotExist(err) {
		t.Errorf("dry-run backup: %v", err)
	}
}

func TestMCPJSONTargetRecoveryBackupRefusal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions do not deny root")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, ".cursor", "mcp.json")
	const original = `{"user":42,}`
	spineWrite(t, target, original, 0o644)
	parent := filepath.Dir(target)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	out, rc, stderr := mcpRunRender(t, dir, "cursor", target, "mcpServers", "")
	if rc != 1 || out != "" || !strings.HasPrefix(stderr, "error: ") || !strings.Contains(stderr, target+".bak") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("rc=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != original {
		t.Errorf("backup failure changed target: %q (%v)", raw, err)
	}
	if _, err := os.Lstat(target + ".bak"); !os.IsNotExist(err) {
		t.Errorf("backup unexpectedly created: %v", err)
	}
}
