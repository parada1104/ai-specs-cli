package sync

// CLI-served vault filesystem MCP at sync time (cli-served-recipes T4, S5).
//
// The recipe no longer materializes ai-specs/recipes/vault-canonical-store/bin/
// vault-fs-mcp.sh: `ai-specs mcp vault-fs` serves it, so the generated MCP entry
// names the CLI with the verb as its args and keeps the env the script read.
// An older sync's script copy is retired, including a user-edited one (notice).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

const vaultMCPManifest = cliServedManifest + `
[recipes.vault-canonical-store]
enabled = true
`

const vaultMCPScript = "ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh"

func vaultMCPNotice() string {
	return "  ✓ removed user-modified vault-fs-mcp.sh " + vaultMCPScript +
		" (ai-specs now serves the vault MCP from the CLI)\n"
}

// vaultMCPKeptNotice is what a manifest entry that still launches the wrapper
// gets instead of the deletion: one line naming the fix.
func vaultMCPKeptNotice() string {
	return "  ! kept " + vaultMCPScript + ": your ai-specs.toml [mcp.vault-canonical] still launches it; " +
		"switch that entry to `command = \"ai-specs\"` with `args = [\"mcp\", \"vault-fs\"]`, then run ai-specs sync to retire the script.\n"
}

// vaultMCPRefManifest is a hand-written entry, which wins over the recipe preset
// render and keeps pointing at the wrapper an older sync materialized.
const vaultMCPRefManifest = vaultMCPManifest + `
[mcp.vault-canonical]
command = 'bash'
args = ['ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh']
`

// readClaudeMCPServer returns the "vault-canonical" entry of the project's
// .mcp.json, parsed with the renderer's own ordered JSON reader.
func readClaudeMCPServer(t *testing.T, project, name string) *mcpObj {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	doc, err := mcpParseJSONOrdered(data)
	if err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	root, ok := doc.(*mcpObj)
	if !ok {
		t.Fatalf(".mcp.json is not an object")
	}
	servers, ok := root.vals["mcpServers"].(*mcpObj)
	if !ok {
		t.Fatalf(".mcp.json has no mcpServers object")
	}
	entry, ok := servers.vals[name].(*mcpObj)
	if !ok {
		t.Fatalf(".mcp.json has no %q server", name)
	}
	return entry
}

// S5: the vault MCP entry names the CLI verb, keeps the env the script read, and
// the script itself is not materialized.
func TestSyncServesVaultMCPFromTheCLI(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, vaultMCPManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(vaultMCPScript))); !os.IsNotExist(err) {
		t.Fatalf("vault-fs-mcp.sh was materialized into the project: %v", err)
	}
	entry := readClaudeMCPServer(t, project, "vault-canonical")
	command, _ := entry.get("command")
	if command != "ai-specs" {
		t.Errorf("command = %v, want ai-specs", command)
	}
	args, _ := entry.get("args")
	wantArgs := []any{"mcp", "vault-fs"}
	if got, ok := args.([]any); !ok || len(got) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", args, wantArgs)
	} else {
		for i := range wantArgs {
			if got[i] != wantArgs[i] {
				t.Fatalf("args = %v, want %v", got, wantArgs)
			}
		}
	}
	env, ok := entry.vals["env"].(*mcpObj)
	if !ok {
		t.Fatalf("env = %v, want the canonical vault env", entry.vals["env"])
	}
	if got := env.vals["CANONICAL_VAULT_PATH"]; got != "${CANONICAL_VAULT_PATH}" {
		t.Errorf("env CANONICAL_VAULT_PATH = %v, want ${CANONICAL_VAULT_PATH}", got)
	}
	// The lock records the rendered entry, never the retired script.
	lk, err := lock.LoadLock(filepath.Join(project, "ai-specs", ".ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := lk.Managed[vaultMCPScript]; recorded {
		t.Errorf("lock recorded the retired script")
	}
	// A resync must not resurrect the script either.
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("resync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(vaultMCPScript))); !os.IsNotExist(err) {
		t.Fatalf("the script came back on resync: %v", err)
	}
}

// S5/S1: the migration retires an older sync's script copy — silently when the
// bytes are still the managed ones, with one notice when the user edited them.
// A copy no lock record proves ai-specs wrote is left alone.
func TestSyncPrunesVaultMCPScriptCopies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		recorded   bool
		edited     bool
		wantKept   bool
		wantNotice bool
	}{
		{name: "managed", recorded: true},
		{name: "user-modified", recorded: true, edited: true, wantNotice: true},
		{name: "untracked", wantKept: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := spineEnv(t)
			project := spineProject(t, vaultMCPManifest)
			if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			path := filepath.Join(project, filepath.FromSlash(vaultMCPScript))
			const body = "#!/usr/bin/env bash\nexec npx -y mcp-server-filesystem \"$CANONICAL_VAULT_PATH\"\n"
			spineWrite(t, path, body, 0o755)
			lockPath := filepath.Join(project, "ai-specs", ".ai-specs.lock")
			if tc.recorded {
				lk, err := lock.LoadLock(lockPath)
				if err != nil {
					t.Fatal(err)
				}
				recipe, source, kind, policy := "vault-canonical-store", "templates/vault-fs-mcp.sh", "template", "auto"
				lock.SetManagedOverride(lk, vaultMCPScript, lock.Sha256Bytes([]byte(body)),
					&recipe, &source, &kind, &policy)
				if err := lock.WriteLock(lockPath, lk); err != nil {
					t.Fatal(err)
				}
			}
			if tc.edited {
				spineWrite(t, path, "#!/usr/bin/env bash\nexec my-own-vault-server\n", 0o755)
			}
			rc, stdout, stderr := runCLIServedSync(t, home, project)
			if rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			if _, err := os.Lstat(path); os.IsNotExist(err) == tc.wantKept {
				t.Fatalf("script kept = %v, want %v (stat err %v)", !tc.wantKept, tc.wantKept, err)
			}
			if got, want := strings.Contains(stderr, vaultMCPNotice()), tc.wantNotice; got != want {
				t.Fatalf("notice present = %v, want %v\nstderr: %q", got, want, stderr)
			}
			if strings.Contains(stderr, vaultMCPKeptNotice()) {
				t.Fatalf("kept notice printed with no manifest reference\nstderr: %q", stderr)
			}
			lk, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, recorded := lk.Managed[vaultMCPScript]; recorded {
				t.Errorf("lock still records the retired script")
			}
		})
	}
}

// R1/R3: a hand-written [mcp.vault-canonical] wins over the recipe preset
// render, so the script it launches is kept, stays recorded, and the user gets
// the one migration notice.
func TestSyncKeepsVaultMCPScriptWhileManifestLaunchesIt(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, vaultMCPRefManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	path := filepath.Join(project, filepath.FromSlash(vaultMCPScript))
	const body = "#!/usr/bin/env bash\nexec npx -y mcp-server-filesystem \"$CANONICAL_VAULT_PATH\"\n"
	spineWrite(t, path, body, 0o755)
	lockPath := filepath.Join(project, "ai-specs", ".ai-specs.lock")
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	recipe, source, kind, policy := "vault-canonical-store", "templates/vault-fs-mcp.sh", "template", "auto"
	lock.SetManagedOverride(lk, vaultMCPScript, lock.Sha256Bytes([]byte(body)), &recipe, &source, &kind, &policy)
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	rc, stdout, stderr := runCLIServedSync(t, home, project)
	if rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("script removed while the manifest launches it: %v", err)
	}
	if !strings.Contains(stderr, vaultMCPKeptNotice()) {
		t.Fatalf("kept notice missing\nstderr: %q", stderr)
	}
	if _, recorded := lk.Managed[vaultMCPScript]; !recorded {
		t.Errorf("lock dropped the record of a kept script")
	}
}
