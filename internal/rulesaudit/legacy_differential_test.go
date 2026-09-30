package rulesaudit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/skills"
)

// Full-payload differential: run the native scanner and the legacy
// rules-inventory.py over the same fabricated trees and compare stdout bytes
// and exit codes (plus stderr where warnings are expected). Skips when
// python3 or the legacy script is unavailable.

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

func runLegacy(t *testing.T, root, home string) (string, string, int) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	script := filepath.Join(repoRoot(t), "lib", "_internal", "rules-inventory.py")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy rules-inventory.py unavailable: %v", err)
	}
	cmd := exec.Command(python, script, root)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "AI_SPECS_HOME="+home)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err = cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("python3 %s: %v", script, err)
		}
	}
	return out.String(), errb.String(), code
}

type treeCase struct {
	name       string
	compareErr bool
	build      func(t *testing.T, root, home string)
}

func differentialTrees() []treeCase {
	writeSkill := func(t *testing.T, dir string) {
		writeFileT(t, filepath.Join(dir, "SKILL.md"), "---\nname: sample\nversion: 1.0.0\n---\nbody\n")
	}
	return []treeCase{
		{
			name: "empty",
			build: func(t *testing.T, root, home string) {
			},
		},
		{
			name: "cursor-agents-manifest",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "alpha.mdc"),
					"---\ndescription: Deploy workflow rules\nglobs: [\"**/*.ts\"]\nalwaysApply: true\n---\n# Heading\nBody about pull request and worktree.\n")
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "nested", "beta.mdc"),
					"---\ndescription: Legacy notes\n# not yaml comment\nalwaysApply: false\n---\nold project rules to deprecate\n")
				writeFileT(t, filepath.Join(root, ".cursorrules"),
					"Always use the trello board and the vault.\n")
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"# Runtime Brief\n\nIntro paragraph.\n\n## Build\n\nRun tests with worktree skill here.\n\n~~~sh\n# not a heading\necho hi\n~~~\n\n### Deploy\n\nShip the pull request.\n")
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[agents]\nenabled = [\"claude\", \"pi\"]\n\n[recipes.worktree-flow]\nenabled = true\n\n[recipes.tdd-flow]\nenabled = false\n")
			},
		},
		{
			name: "mode-b-stack-hints",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "package-lock.json"), "{}\n")
				writeFileT(t, filepath.Join(root, "pyproject.toml"), "[project]\nname = \"x\"\n")
				writeFileT(t, filepath.Join(root, "go.mod"), "module x\n")
				writeFileT(t, filepath.Join(root, ".atl", "skill-registry.md"),
					"See `atl-only` and `worktree-flow`.\n")
			},
		},
		{
			name: "manifest-parse-error",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), "key =\n")
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"## Project Rules\n\nkeep this project rule.\n")
			},
		},
		{
			name: "recipes-list-and-monolithic-agents",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[[recipes]]\nid = \"worktree-flow\"\nenabled = true\n\n[[recipes]]\nid = \"tdd-flow\"\n")
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"Just a simple runtime brief paragraph with no headings.\n")
			},
		},
		{
			name: "agents-heading-depths",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"# One\nbody one\n## Two\nbody two\n### Three\nbody three\n#### Four\nnot a heading\n")
			},
		},
		{
			name: "mdc-scalar-globs-and-snake-always",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "x.mdc"),
					"---\nglobs: **/*.py\nalways_apply: 1\n---\nbody x\n")
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "y.mdc"),
					"---\ndescription: Empty globs\nglobs:\n---\nbody y\n")
			},
		},
		{
			name:       "skills-cache-atl",
			compareErr: true,
			build: func(t *testing.T, root, home string) {
				cache := skills.CacheRoot(root, home)
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "local-skill"))
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "worktree-flow"))
				writeSkill(t, filepath.Join(cache, ".recipe", "rec-a", "skills", "recipe-skill"))
				writeSkill(t, filepath.Join(cache, ".recipe", "rec-b", "skills", "recipe-skill"))
				writeSkill(t, filepath.Join(cache, ".deps", "dep-a", "skills", "dep-skill"))
				writeSkill(t, filepath.Join(cache, ".bundled", "skills", "bundled-skill"))
				writeSkill(t, filepath.Join(cache, ".bundled", "skills", "local-skill"))
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "atl.mdc"),
					"---\ndescription: Trello sync rules\n---\nSync trello cards in a worktree.\n")
				writeFileT(t, filepath.Join(root, ".atl", "skill-registry.md"),
					"`trello-mcp-workflow`, `recipe-skill`\n")
			},
		},
	}
}

func TestDifferentialRulesInventory(t *testing.T) {
	for _, tc := range differentialTrees() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			tc.build(t, root, home)

			var gout, gerr bytes.Buffer
			gcode := Run(root, home, &gout, &gerr)
			lout, lerr, lcode := runLegacy(t, root, home)

			if gcode != lcode {
				t.Fatalf("exit = %d, legacy %d\nport stderr: %s\nlegacy stderr: %s",
					gcode, lcode, gerr.String(), lerr)
			}
			if gout.String() != lout {
				t.Errorf("stdout mismatch:\n--- port ---\n%s\n--- legacy ---\n%s", gout.String(), lout)
			}
			if tc.compareErr && gerr.String() != lerr {
				t.Errorf("stderr mismatch:\n--- port ---\n%q\n--- legacy ---\n%q", gerr.String(), lerr)
			}
		})
	}
}
