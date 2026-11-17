package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var contractBinary string

func contractBin(t *testing.T) string {
	t.Helper()
	if contractBinary == "" {
		contractBinary = filepath.Join(t.TempDir(), "worktree-gate")
		t.Cleanup(func() { contractBinary = "" })
		cmd := exec.Command("go", "build", "-o", contractBinary, ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build gate: %v\n%s", err, out)
		}
	}
	return contractBinary
}

func runContract(t *testing.T, dir, input string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(contractBin(t), args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "WORKTREE_GATE_MODE=", "WORKTREE_GATE_SCOPE=")
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if e, ok := err.(*exec.ExitError); ok {
		code = e.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return code, out.String(), errOut.String()
}

func exactContract(t *testing.T, gotCode int, gotOut, gotErr string, code int, out, errOut string) {
	t.Helper()
	if gotCode != code || gotOut != out || gotErr != errOut {
		t.Fatalf("got exit=%d stdout=%q stderr=%q; want exit=%d stdout=%q stderr=%q", gotCode, gotOut, gotErr, code, out, errOut)
	}
}

func TestSubprocessEventContract(t *testing.T) {
	root := gitFixture(t, "main")
	allow := `{"tool_name":"Write","cwd":"` + root + `","tool_input":{"file_path":"` + filepath.Join(t.TempDir(), "outside", "x") + `"}}`
	code, out, errOut := runContract(t, root, allow, "--gate-mode", "always", "--gate-scope", "auto", "--repo-topology", "standalone", "--protected", "main development")
	exactContract(t, code, out, errOut, 0, "", "")
	block := `{"tool_name":"Write","cwd":"` + root + `","tool_input":{"file_path":"README.md"}}`
	code, out, errOut = runContract(t, root, block, "--gate-mode", "always", "--gate-scope", "auto", "--repo-topology", "auto", "--protected", "main development")
	want := fmt.Sprintf("worktree-gate: refusing to Write 'README.md' on protected branch 'main' in the main worktree (%s). Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write.\n", root)
	exactContract(t, code, out, errOut, 2, "", want)
	code, out, errOut = runContract(t, root, "", "--bad-gate-flag")
	exactContract(t, code, out, errOut, 0, "", "flag provided but not defined: -bad-gate-flag\nusage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain] [--resolve-central-root]\nworktree-gate: warning: flag provided but not defined: -bad-gate-flag (failing open)\n")
}

func TestSubprocessCleanupContract(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-unmerged")
	if err := os.WriteFile(filepath.Join(wt, "unmerged.txt"), []byte("unmerged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "unmerged")
	args := []string{"--cleanup", "--dir", ".worktrees", "--base", "main", "--integration-branch", "integration", "--topology", "standalone", "--dry-run"}
	code, out, errOut := runContract(t, root, "", args...)
	exactContract(t, code, out, errOut, 0, "skipped feat-unmerged (unmerged)\n", "")
	code, out, errOut = runContract(t, root, "", "--cleanup", "--dir", ".worktrees", "--base", "main", "--topology", "standalone", "--dry-run", "--bad-cleanup-flag")
	exactContract(t, code, out, errOut, 2, "", "flag provided but not defined: -bad-cleanup-flag\nusage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain] [--resolve-central-root]\nworktree-cleanup: flag provided but not defined: -bad-cleanup-flag (refusing destructive run)\n")
	code, out, errOut = runContract(t, root, "", "--cleanup", "--dir", ".worktrees", "--base", "main", "--topology", "not-a-topology", "--dry-run")
	exactContract(t, code, out, errOut, 0, "skipped feat-unmerged (unmerged)\n", "")

	dirtyRoot := makeCleanupRepo(t)
	dirty := addCleanupWorktree(t, dirtyRoot, "feat-dirty")
	if err := os.WriteFile(filepath.Join(dirty, "landed.txt"), []byte("landed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, dirty, "add", ".")
	cleanupGitTest(t, dirty, "commit", "-qm", "landed")
	cleanupGitTest(t, dirtyRoot, "merge", "-q", "--no-ff", "-m", "merge dirty", "feat-dirty")
	if err := os.WriteFile(filepath.Join(dirty, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runContract(t, dirtyRoot, "", "--cleanup", "--dir", ".worktrees", "--base", "main", "--topology", "standalone", "--dry-run")
	exactContract(t, code, out, errOut, 0, "skipped feat-dirty (dirty)\n", "")

	detachedRoot := makeCleanupRepo(t)
	detached := filepath.Join(detachedRoot, ".worktrees", "detached")
	if err := os.MkdirAll(filepath.Dir(detached), 0o700); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, detachedRoot, "worktree", "add", "-q", "--detach", detached, "main")
	code, out, errOut = runContract(t, detachedRoot, "", "--cleanup", "--dir", ".worktrees", "--base", "main", "--topology", "standalone", "--dry-run")
	exactContract(t, code, out, errOut, 0, "skipped detached (detached)\n", "")
}

func TestSubprocessLedgerContract(t *testing.T) {
	root, common, branch := ledgerRepo(t)
	witness := writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	evidence := writeLedgerEvidence(t, map[string]string{"remote": "card-remote"})
	args := []string{"--ledger", "--checkpoint", "work-start", "--project-root", root, "--ledger-mode", "warn", "--evidence", evidence, "--witness", witness}
	code, out, errOut := runContract(t, root, "", args...)
	if code != 0 || errOut != "" {
		t.Fatalf("ledger evidence exit=%d stderr=%q", code, errOut)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["checkpoint"] != "work-start" || got["decision"] != "allow" {
		t.Fatalf("ledger evidence stdout=%q decode error=%v", out, err)
	}
	wantEvidence := fmt.Sprintf(`{"capability":"tracker","active":true,"checkpoint":"work-start","mode":"warn","decision":"allow","reason":"needs-item","identity":{"common_dir":%q,"branch":"main","change":null,"key":"%s\u001fmain"},"item":null,"conflict":null,"prompt":null,"doctor":{"severity":"WARN","name":"tracker-ledger","message":"no provider item linked; create or bind one to satisfy tracker tracking"}}`+"\n", common, common)
	exactContract(t, code, out, errOut, 0, wantEvidence, "")
	saveLedgerStore(t, common, branch, "card-local", nil)
	var decidedOut string
	// A decision is exercised through the public binary and its persisted effect is
	// observed by a second subprocess invocation.
	args = []string{"--ledger", "--checkpoint", "pre-merge", "--project-root", root, "--ledger-mode", "ask", "--witness", witness, "--decide", `{"choice":"local"}`}
	code, out, errOut = runContract(t, root, "", args...)
	if code != 0 || errOut != "" {
		t.Fatalf("ledger decide exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
	decidedOut = out
	args = []string{"--ledger", "--checkpoint", "pre-merge", "--project-root", root, "--ledger-mode", "ask", "--witness", witness}
	code, out, errOut = runContract(t, root, "", args...)
	exactContract(t, code, out, errOut, 0, decidedOut, "")
	got = nil
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["decision"] != "allow" || got["reason"] != "adjudicated" {
		t.Fatalf("ledger regrade stdout=%q decode error=%v", out, err)
	}
}

func TestSubprocessSimpleAndTopologyContracts(t *testing.T) {
	root := gitFixture(t, "main")
	code, out, errOut := runContract(t, root, "", "--version")
	exactContract(t, code, out, errOut, 0, "dev\n", "")
	code, out, errOut = runContract(t, root, "", "--selftest")
	exactContract(t, code, out, errOut, 0, "ok\n", "")
	code, out, errOut = runContract(t, root, "", "--explain", "--gate-mode", "always", "--gate-scope", "auto")
	exactContract(t, code, out, errOut, 0, "{\"mode\":\"\",\"tool\":\"\",\"cwd\":\"\",\"gate_mode\":\"always\",\"gate_scope\":\"auto\",\"repo_topology\":\"\",\"candidates\":null,\"cwd_source\":\"\",\"decision\":\"allow\",\"branch\":\"\",\"reason\":\"no-blocking-candidate\"}\n", "")
	code, out, errOut = runContract(t, root, "", "--resolve-central-root")
	exactContract(t, code, out, errOut, 1, "", "worktree-gate: resolve-central-root: unproven submodule topology\n")
}
