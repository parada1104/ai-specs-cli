package cli

// `ai-specs mcp vault-fs` — the CLI serves the vault filesystem MCP server
// (cli-served-recipes T4, S5).
//
// The verb replaces ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh:
// the same CANONICAL_VAULT_PATH validation with the same messages and exit
// codes, and the same pinned npx invocation. Nothing is printed on stdout before
// the server starts, so the MCP stdio handshake is unaffected.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeNpxDir installs a PATH stub named npx that records its argv and the vault
// env it saw, then exits 0. Nothing on PATH else, so the verb's own resolution
// is what the case observes.
func fakeNpxDir(t *testing.T) (path, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "record")
	script := "#!/bin/sh\n" +
		"{\n" +
		"  printf 'argv0=%s\\n' \"$0\"\n" +
		"  for a in \"$@\"; do printf 'arg=%s\\n' \"$a\"; done\n" +
		"  printf 'env=%s\\n' \"${CANONICAL_VAULT_PATH-<unset>}\"\n" +
		"} > \"$FAKE_NPX_RECORD\"\n"
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("FAKE_NPX_RECORD", record)
	return bin, record
}

func runVaultFS(t *testing.T) (code int, stdout, stderr string) {
	t.Helper()
	var out, errW bytes.Buffer
	code = Run([]string{"mcp", "vault-fs"}, t.TempDir(), strings.NewReader(""), &out, &errW)
	return code, out.String(), errW.String()
}

// S5: every refusal the retired script made keeps its exact message and exit
// code, and no server is started.
func TestMCPVaultFSRefusesLikeTheRetiredScript(t *testing.T) {
	for _, tc := range []struct {
		name    string
		vault   string
		unset   bool
		wantOut string
		wantRC  int
	}{
		{
			name:    "unset",
			unset:   true,
			wantOut: "vault-fs-mcp: CANONICAL_VAULT_PATH is unset or empty.\nSet it in .envrc to an absolute vault scope path, then restart the agent.\n",
			wantRC:  1,
		},
		{
			name:    "empty",
			wantOut: "vault-fs-mcp: CANONICAL_VAULT_PATH is unset or empty.\nSet it in .envrc to an absolute vault scope path, then restart the agent.\n",
			wantRC:  1,
		},
		{
			name:    "unexpanded-shell-ref",
			vault:   "$VAULT_ROOT/proyectos/app",
			wantOut: "vault-fs-mcp: CANONICAL_VAULT_PATH still contains '$' ($VAULT_ROOT/proyectos/app).\nUse an absolute path, or set it in .envrc so the shell expands nested vars.\n",
			wantRC:  1,
		},
		{
			name:    "not-a-directory",
			vault:   "/nonexistent-ai-specs-vault-scope",
			wantOut: "vault-fs-mcp: not a directory: /nonexistent-ai-specs-vault-scope\n",
			wantRC:  1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, record := fakeNpxDir(t)
			if tc.unset {
				os.Unsetenv("CANONICAL_VAULT_PATH")
			} else {
				t.Setenv("CANONICAL_VAULT_PATH", tc.vault)
			}
			code, stdout, stderr := runVaultFS(t)
			if code != tc.wantRC {
				t.Errorf("exit = %d, want %d", code, tc.wantRC)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty (the MCP handshake must not be polluted)", stdout)
			}
			if stderr != tc.wantOut {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantOut)
			}
			if _, err := os.Stat(record); !os.IsNotExist(err) {
				t.Errorf("npx ran for a refused configuration")
			}
		})
	}
}

// S5: a valid vault scope execs the same pinned npx with the same argv, and the
// vault path reaches the child environment.
func TestMCPVaultFSExecsThePinnedNpx(t *testing.T) {
	for _, tc := range []struct {
		name  string
		homed bool
	}{
		{name: "absolute"},
		{name: "home-relative", homed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, record := fakeNpxDir(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			vault := filepath.Join(home, "vault")
			if err := os.MkdirAll(vault, 0o755); err != nil {
				t.Fatal(err)
			}
			// The retired script expanded a leading ~/ against HOME.
			declared := vault
			if tc.homed {
				declared = "~/vault"
			}
			t.Setenv("CANONICAL_VAULT_PATH", declared)

			code, stdout, stderr := runVaultFS(t)
			if code != 0 || stdout != "" || stderr != "" {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q; want 0 with no output", code, stdout, stderr)
			}
			data, err := os.ReadFile(record)
			if err != nil {
				t.Fatalf("npx did not run: %v", err)
			}
			want := strings.Join([]string{
				"argv0=" + filepath.Join(os.Getenv("PATH"), "npx"),
				"arg=-y",
				"arg=-p",
				"arg=@modelcontextprotocol/server-filesystem@2025.7.1",
				"arg=-p",
				"arg=zod@3",
				"arg=mcp-server-filesystem",
				"arg=" + vault,
				"env=" + declared,
				"",
			}, "\n")
			if got := string(data); got != want {
				t.Errorf("npx invocation = %q, want %q", got, want)
			}
		})
	}
}

// S5: the child's exit code is the verb's exit code, so a host that reads it
// sees the server's own status.
func TestMCPVaultFSPropagatesTheChildExitCode(t *testing.T) {
	bin, _ := fakeNpxDir(t)
	script := filepath.Join(bin, "npx")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CANONICAL_VAULT_PATH", t.TempDir())
	if code, _, _ := runVaultFS(t); code != 7 {
		t.Errorf("exit = %d, want the child's 7", code)
	}
}
