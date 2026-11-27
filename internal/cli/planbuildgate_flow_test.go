package cli

// Real-flow scenario for the write gate (change-ledger-v2 T2, S1/S2/S9/S16): the
// built CLI runs against temporary repositories over the audit matrix. It drives
// the binary through exec, so the exit codes, the stderr bytes and the on-disk
// effects are the ones an agent hook sees.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// flowRun executes the built CLI in dir and returns stdout, stderr and the exit
// code, with the data home pinned away from the developer's real one.
func flowRun(t *testing.T, bin, dir string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AI_SPECS_HOME="+t.TempDir())
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

func flowGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// flowRepo is a committed repository on branch feat/thing with the execution mode
// declared in the manifest.
func flowRepo(t *testing.T, mode string, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	flowGit(t, repo, "init", "-b", "main")
	flowGit(t, repo, "config", "user.email", "flow@example.invalid")
	flowGit(t, repo, "config", "user.name", "flow")
	for path, content := range files {
		flowWrite(t, repo, path, content)
	}
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "--allow-empty", "-m", "base")
	flowWrite(t, repo, "ai-specs/ai-specs.toml",
		"[recipes.plan-build-flow]\nenabled = true\n\n[recipes.plan-build-flow.config]\nmode = \""+mode+"\"\n")
	flowGit(t, repo, "checkout", "-b", "feat/thing")
	return repo
}

func flowWrite(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func flowEvent(repo, target string) string {
	return `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"` + target + `"}}`
}

func TestPlanBuildGateWriteFlow(t *testing.T) {
	bin, _ := smokeSetup(t)

	t.Run("ODD production write with no plan is allowed", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"), "plan-build-gate")
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
		}
	})

	t.Run("OpenSpec production write with no plan is allowed", func(t *testing.T) {
		repo := flowRepo(t, "openspec", nil)
		stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"), "plan-build-gate")
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
		}
	})

	t.Run("worktree with no artifact and no divergence is tier none", func(t *testing.T) {
		repo := flowRepo(t, "openspec", nil)
		stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"), "plan-build-gate", "--explain")
		if code != 0 || stdout != "" || stderr != "plan-build-gate: tier none; refutations none\n" {
			t.Fatalf("explain = (%d, %q, %q), want the none tier", code, stdout, stderr)
		}
	})

	t.Run("R1 hand write to a canonical spec", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "odd/specs/cap/spec.md"), "plan-build-gate")
		if code != 2 || stdout != "" || !strings.Contains(stderr, "R1:") {
			t.Fatalf("gate = (%d, %q, %q), want exit 2 naming R1", code, stdout, stderr)
		}
		for _, name := range []string{"odd/changes/thing/specs/", "ai-specs specs promote"} {
			if !strings.Contains(stderr, name) {
				t.Errorf("refusal %q does not name %q", stderr, name)
			}
		}
		if _, err := os.Stat(filepath.Join(repo, "odd", "specs", "cap", "spec.md")); !os.IsNotExist(err) {
			t.Fatalf("refused target stat err = %v, want it absent", err)
		}
	})

	t.Run("R2 grades a canonical spec write that carries its delta", func(t *testing.T) {
		repo := t.TempDir()
		flowGit(t, repo, "init", "-b", "main")
		flowGit(t, repo, "config", "user.email", "flow@example.invalid")
		flowGit(t, repo, "config", "user.name", "flow")
		flowWrite(t, repo, "ai-specs/ai-specs.toml",
			"[recipes.plan-build-flow.config]\nmode = \"odd\"\nhigh_risk_paths = [\"odd/specs/**\"]\n")
		flowGit(t, repo, "add", "-A")
		flowGit(t, repo, "commit", "-m", "base")
		flowGit(t, repo, "checkout", "-b", "feat/thing")
		flowWrite(t, repo, "odd/changes/thing/specs/cap/spec.md", "# spec\n")
		stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "odd/specs/cap/spec.md"), "plan-build-gate")
		if code != 2 || stdout != "" || !strings.Contains(stderr, "R2:") || !strings.Contains(stderr, "odd/specs/cap/spec.md") {
			t.Fatalf("gate = (%d, %q, %q), want exit 2 naming R2 and the pending target", code, stdout, stderr)
		}
	})

	t.Run("a leftover declaration changes nothing", func(t *testing.T) {
		repo := flowRepo(t, "openspec", nil)
		flowGit(t, repo, "config", "--local", "branch.feat/thing.aiSpecsCeremony", "none")
		stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"), "plan-build-gate")
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
		}
	})

	t.Run("a fresh declaration warns and has no effect", func(t *testing.T) {
		repo := flowRepo(t, "openspec", nil)
		stdout, stderr, code := flowRun(t, bin, repo, "", "changes", "start", "--none", "--root", repo)
		if code != 0 || stdout != "" || !strings.Contains(stderr, "deprecated") || !strings.Contains(stderr, "changes workstream") {
			t.Fatalf("start = (%d, %q, %q), want the deprecation notice", code, stdout, stderr)
		}
		if out, _ := exec.Command("git", "-C", repo, "config", "--get", "branch.feat/thing.aiSpecsCeremony").Output(); string(out) != "" {
			t.Fatalf("declaration recorded %q, want nothing", out)
		}
		stdout, stderr, code = flowRun(t, bin, repo, flowEvent(repo, "src/app.go"), "plan-build-gate")
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("gate after start = (%d, %q, %q), want silence", code, stdout, stderr)
		}
	})
}

// S2: the refusal and `--explain` come from one verdict, so a refused write still
// reports the tier and the rule the refusal names. Today the canonical-spec path
// refuses before the explain line is reached.
func TestPlanBuildGateExplainsARefusedWrite(t *testing.T) {
	bin, _ := smokeSetup(t)
	repo := flowRepo(t, "odd", nil)
	stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "odd/specs/cap/spec.md"),
		"plan-build-gate", "--explain")
	if code != 2 || stdout != "" {
		t.Fatalf("gate = (%d, %q), want exit 2", code, stdout)
	}
	if !strings.Contains(stderr, "plan-build-gate: tier none; refutations R1\n") {
		t.Fatalf("stderr = %q, want the tier and the rule the refusal names", stderr)
	}
	if !strings.Contains(stderr, "R1:") {
		t.Fatalf("stderr = %q, want the refusal", stderr)
	}
}

// S16: an inherited canonical-spec edit in a worktree with no plan, no delta and
// no product divergence is not a change, so the next ordinary write is allowed.
func TestPlanBuildGateDoesNotGradeAWorkstreamWithACanonicalSpecEdit(t *testing.T) {
	bin, _ := smokeSetup(t)
	repo := flowRepo(t, "odd", map[string]string{"src/app.go": "package src\n"})
	flowWrite(t, repo, "odd/specs/widget/spec.md", "x\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-m", "spec: edit the canonical tree")

	stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"), "plan-build-gate")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want a silent allow", code, stdout, stderr)
	}
}

// R2 is independent of the production scope: a declared high-risk target is graded
// wherever it lives.
func TestPlanBuildGateGradesADeclaredHighRiskTargetOutsideProduction(t *testing.T) {
	bin, _ := smokeSetup(t)
	repo := t.TempDir()
	flowGit(t, repo, "init", "-b", "main")
	flowGit(t, repo, "config", "user.email", "flow@example.invalid")
	flowGit(t, repo, "config", "user.name", "flow")
	flowWrite(t, repo, "ai-specs/ai-specs.toml", "[recipes.plan-build-flow]\nenabled = true\n\n"+
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nhigh_risk_paths = [\"docs/**\"]\n")
	flowWrite(t, repo, "README.md", "x\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-m", "base")
	flowGit(t, repo, "checkout", "-b", "feat/thing")
	flowWrite(t, repo, "odd/tasks/thing.md", "## Specs\n\n- **S1** — x\n\n## Tasks\n\n- T1 — y\n\n"+
		"## Verification\n\n| Spec | Evidence |\n| --- | --- |\n| S1 | fixture |\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-m", "plan")

	_, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "docs/deploy.md"), "plan-build-gate")
	if code != 2 || !strings.Contains(stderr, "R2") || !strings.Contains(stderr, "docs/deploy.md") {
		t.Fatalf("gate = (%d, %q), want exit 2 naming R2 and the pending target", code, stderr)
	}
}

// R2 survives the S16 exemption: a workstream that writes a declared high-risk
// path still owes its evidence.
func TestPlanBuildGateGradesADeclaredHighRiskTargetInAWorkstream(t *testing.T) {
	bin, _ := smokeSetup(t)
	repo := t.TempDir()
	flowGit(t, repo, "init", "-b", "main")
	flowGit(t, repo, "config", "user.email", "flow@example.invalid")
	flowGit(t, repo, "config", "user.name", "flow")
	flowWrite(t, repo, "ai-specs/ai-specs.toml", "[recipes.plan-build-flow]\nenabled = true\n\n"+
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nhigh_risk_paths = [\"docs/**\"]\n")
	flowWrite(t, repo, "README.md", "x\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-m", "base")
	flowGit(t, repo, "checkout", "-b", "feat/thing")

	_, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "docs/deploy.md"), "plan-build-gate")
	if code != 2 || !strings.Contains(stderr, "R2") {
		t.Fatalf("gate = (%d, %q), want exit 2 naming R2 for the workstream target", code, stderr)
	}
}

// S16: a non-graded write that was asked to explain says so, so an operator can
// tell an exempt write from a hook that never evaluated the event.
func TestPlanBuildGateExplainsANonGradedWrite(t *testing.T) {
	bin, _ := smokeSetup(t)
	repo := flowRepo(t, "odd", nil)

	stdout, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "odd/tasks/thing.md"),
		"plan-build-gate", "--explain")
	if code != 0 || stdout != "" {
		t.Fatalf("gate = (%d, %q), want exit 0", code, stdout)
	}
	if !strings.Contains(stderr, "not graded") {
		t.Fatalf("stderr = %q, want the exemption named", stderr)
	}
}

// R2 grades a declared high-risk path anywhere it lives, including inside the odd
// tree, where only the planning folders are exempt.
func TestPlanBuildGateGradesADeclaredHighRiskPathInTheOddTree(t *testing.T) {
	bin, _ := smokeSetup(t)
	repo := t.TempDir()
	flowGit(t, repo, "init", "-b", "main")
	flowGit(t, repo, "config", "user.email", "flow@example.invalid")
	flowGit(t, repo, "config", "user.name", "flow")
	flowWrite(t, repo, "ai-specs/ai-specs.toml", "[recipes.plan-build-flow]\nenabled = true\n\n"+
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nhigh_risk_paths = [\"odd/tools/**\"]\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-m", "base")
	flowGit(t, repo, "checkout", "-b", "feat/thing")

	_, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "odd/tools/deploy.sh"), "plan-build-gate")
	if code != 2 || !strings.Contains(stderr, "R2") || !strings.Contains(stderr, "odd/tools/deploy.sh") {
		t.Fatalf("gate = (%d, %q), want exit 2 naming R2 and the pending target", code, stderr)
	}
}
