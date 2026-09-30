package sync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// agentsRenderRefError is the {"error": {...}} payload testdata/agentsrender_ref.py
// emits when the Python authority raises (the brief-modes validation path).
type agentsRenderRefError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// agentsRenderRefResult is the canonical JSON emitted by the ref driver.
type agentsRenderRefResult struct {
	BytesB64 string                `json:"bytes_b64"`
	SHA256   string                `json:"sha256"`
	Error    *agentsRenderRefError `json:"error"`
}

// agentsRenderCase is one differential scenario. resolved builds a fresh
// resolved-config map for a leg's own project root (so the two legs never
// share mutable state and topology detection sees a real, non-git dir).
type agentsRenderCase struct {
	name     string
	manifest string
	resolved func(projectRoot string) map[string]any
	wantErr  bool
}

// emptyResolved builds a resolved-config with no bindings/recipes/enabled.
func emptyResolved(projectRoot string) map[string]any {
	return map[string]any{
		"bindings":     map[string]any{},
		"recipes":      map[string]any{},
		"enabled":      []any{},
		"project_root": projectRoot,
	}
}

func runAgentsRenderRef(t *testing.T, root, script, manifest string, resolved map[string]any) agentsRenderRefResult {
	t.Helper()
	resolvedJSON, err := json.Marshal(resolved)
	if err != nil {
		t.Fatalf("marshal resolved: %v", err)
	}
	caseJSON, err := json.Marshal(map[string]string{
		"manifest_toml_b64": base64.StdEncoding.EncodeToString([]byte(manifest)),
		"resolved_json_b64": base64.StdEncoding.EncodeToString(resolvedJSON),
	})
	if err != nil {
		t.Fatalf("marshal case: %v", err)
	}
	cmd := exec.Command("python3", script)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(caseJSON)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("ref driver failed: %v\nstderr: %s", err, errBuf.String())
	}
	var ref agentsRenderRefResult
	if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
		t.Fatalf("parse ref JSON: %v\nraw: %s", err, out.String())
	}
	return ref
}

// TestAgentsRenderDifferential runs every case through both the Go rendering
// core and the REAL agents-render.py (via the ref driver) and requires byte
// equality of the would-write bytes, plus SHA-256 agreement.
func TestAgentsRenderDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	refScript := filepath.Join(root, "internal", "sync", "testdata", "agentsrender_ref.py")

	for _, tc := range agentsRenderCases() {
		t.Run(tc.name, func(t *testing.T) {
			refDir := t.TempDir()
			ref := runAgentsRenderRef(t, root, refScript, tc.manifest, tc.resolved(refDir))

			manifestTable, err := toml.Parse([]byte(tc.manifest))
			if err != nil {
				t.Fatalf("parse manifest with internal/toml: %v", err)
			}

			goDir := t.TempDir()
			got := RenderAgentsMarkdown(manifestTable, tc.resolved(goDir))

			if tc.wantErr {
				if ref.Error == nil {
					t.Fatalf("ref did not raise; got %+v", ref)
				}
				if ref.Error.Type != "ValueError" {
					t.Errorf("ref error type = %q, want ValueError", ref.Error.Type)
				}
				if got != nil {
					t.Errorf("RenderAgentsMarkdown returned %d byte(s), want nil on invalid brief mode", len(got))
				}
				brief, _ := manifestTable.Table("brief")
				verr := ValidateBriefModes(brief)
				if verr == nil {
					t.Fatalf("ValidateBriefModes returned nil, want %q", ref.Error.Message)
				}
				if verr.Error() != ref.Error.Message {
					t.Errorf("ValidateBriefModes message:\n  go:  %q\n  ref: %q", verr.Error(), ref.Error.Message)
				}
				return
			}

			if ref.Error != nil {
				t.Fatalf("ref raised unexpectedly: %s: %s", ref.Error.Type, ref.Error.Message)
			}
			want, err := base64.StdEncoding.DecodeString(ref.BytesB64)
			if err != nil {
				t.Fatalf("decode ref bytes: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("would-write bytes differ\n--- go ---\n%s\n--- ref ---\n%s", got, want)
			}
			if gotSHA := sha256Hex(got); gotSHA != ref.SHA256 {
				t.Errorf("sha256: go=%s ref=%s", gotSHA, ref.SHA256)
			}
		})
	}
}

func agentsRenderCases() []agentsRenderCase {
	return []agentsRenderCase{
		{
			name: "minimal manifest project only",
			manifest: `[project]
name = 'minimal'

[agents]
enabled = ['claude']
`,
			resolved: emptyResolved,
		},
		{
			name: "full manifest with every brief section",
			manifest: `[project]
name = 'full'

[agents]
enabled = ['claude', 'pi']

[brief]
intro = """
First line.

Second line.
"""
purpose = 'Do great things.'
runtime_flow = ['Flow one.', 'Flow two.']
context_sources = ['Source one.']
conflict_policy = ['Policy one.']
workflow_rules = ['Rule one.']
useful_commands = ['Run the focused suite.']

[brief.mcp_descriptions]
trello = 'Project tracking.'
vault = 'Canonical notes.'

[mcp.trello]
command = 'npx'
args = ['-y', '@trello/mcp']
env = { TOKEN = '$TRELLO_TOKEN' }

[mcp.engram]
command = 'engram'
env = { API_KEY = 'sk-super-secret-value-1234567890', HOME = '${HOME}' }
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{
						"tracker":     "trello-mcp-workflow",
						"vcs-pr-flow": "git-pr-flow",
						"test-runner": "tdd-flow",
					},
					"recipes": map[string]any{
						"trello-mcp-workflow": map[string]any{"board_id": "69ec097f13e2d38ecd89a557"},
						"git-pr-flow":         map[string]any{"base_branch": "development"},
						"tdd-flow":            map[string]any{"test_command": "./tests/validate.sh"},
						"worktree-flow":       map[string]any{"integration_branch": "development"},
					},
					"enabled": []any{
						"git-pr-flow", "tdd-flow", "worktree-flow", "trello-mcp-workflow",
					},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "deps and recipe brief fragments",
			manifest: `[project]
name = 'fragments'

[agents]
enabled = ['claude']

[[deps]]
id = 'some-dep'
source = '../some-dep'
path = 'skills/some'

[recipes.wf]
enabled = true

[recipes.wf.config]
integration_branch = 'main'

[recipes.tdd]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{},
					"recipes": map[string]any{
						"wf": map[string]any{
							"integration_branch": "main",
							"brief_fragments": map[string]any{
								"workflow_rules": []any{
									map[string]any{"key": "wt", "text": "Create a worktree on `{config.integration_branch}`."},
								},
								"context_sources": []any{
									map[string]any{"text": "Trello is the source of truth."},
								},
							},
						},
						"tdd": map[string]any{
							"test_command": "./tests/run.sh",
							"brief_fragments": map[string]any{
								"workflow_rules": []any{
									map[string]any{"text": "Write failing tests first."},
								},
								"useful_commands": []any{
									map[string]any{"text": "Run tests: `{config.test_command}`"},
								},
							},
						},
					},
					"enabled":      []any{"wf", "tdd"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "config substitution edge cases",
			manifest: `[project]
name = 'sub'

[agents]
enabled = ['claude']

[recipes.sub]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{},
					"recipes": map[string]any{
						"sub": map[string]any{
							"known": "VALUE",
							"flag":  true,
							"count": 3,
							"scope": []any{"root", "skill"},
							"brief_fragments": map[string]any{
								"workflow_rules": []any{
									map[string]any{"text": "A {config.known} B {config.missing} C {bare} D {{esc}} E {config.flag} F {config.count} G {config.scope} H { lone"},
								},
							},
						},
					},
					"enabled":      []any{"sub"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "mcp env redaction",
			manifest: `[project]
name = 'envs'

[agents]
enabled = ['claude']

[mcp.alpha]
command = 'alpha'
env = ['PLAIN_VAR', 'OTHER_VAR']

[mcp.beta]
command = 'beta'
environment = { A = '$A_VAR', B = '${B_VAR}', C = 'plain-secret-value', D = '  $PADDED  ' }
`,
			resolved: emptyResolved,
		},
		{
			name: "missing optional sections",
			manifest: `[project]
name = 'empty'
`,
			resolved: emptyResolved,
		},
		{
			name: "blank optional sections",
			manifest: `[project]
name = 'blank'

[agents]
enabled = ['pi']

[brief]
intro = '   '
purpose = ''
`,
			resolved: emptyResolved,
		},
		{
			name: "repo topology from project field",
			manifest: `[project]
name = 'topo'
repo_topology = 'monorepo-apps'

[agents]
enabled = ['claude']
`,
			resolved: emptyResolved,
		},
		{
			name: "unknown vcs recipe uses generic label",
			manifest: `[project]
name = 'vcs'

[agents]
enabled = ['claude']

[recipes.custom-vcs]
enabled = true

[recipes.custom-vcs.config]
base_branch = 'trunk'
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{"vcs-pr-flow": "custom-vcs"},
					"recipes": map[string]any{
						"custom-vcs": map[string]any{"base_branch": "trunk"},
					},
					"enabled":      []any{"custom-vcs"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "vcs sibling filtering keeps only the bound provider",
			manifest: `[project]
name = 'vcsfilter'

[agents]
enabled = ['claude']

[recipes.git-pr-flow]
enabled = true

[recipes.gitlab-mr-flow]
enabled = true

[recipes.bitbucket-pr-flow]
enabled = true

[recipes.worktree-flow]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				frag := func(text string) map[string]any {
					return map[string]any{
						"brief_fragments": map[string]any{
							"workflow_rules": []any{map[string]any{"text": text}},
						},
					}
				}
				return map[string]any{
					"bindings": map[string]any{"vcs-pr-flow": "gitlab-mr-flow"},
					"recipes": map[string]any{
						"git-pr-flow":       frag("Use GitHub PRs to merge."),
						"gitlab-mr-flow":    frag("Use GitLab MRs to merge."),
						"bitbucket-pr-flow": frag("Use Bitbucket PRs to merge."),
						"worktree-flow":     frag("Create a worktree for every change."),
					},
					"enabled": []any{
						"git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow", "worktree-flow",
					},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "replace modes suppress recipe fragments per section",
			manifest: `[project]
name = 'replace'

[agents]
enabled = ['claude']

[brief]
workflow_rules_mode = 'replace'
workflow_rules = ['Only this rule.']

[recipes.wf]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{},
					"recipes": map[string]any{
						"wf": map[string]any{
							"brief_fragments": map[string]any{
								"workflow_rules":  []any{map[string]any{"text": "Recipe rule should not appear."}},
								"context_sources": []any{map[string]any{"text": "Recipe context."}},
							},
						},
					},
					"enabled":      []any{"wf"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "invalid brief mode raises",
			manifest: `[project]
name = 'bad'

[brief]
workflow_rules_mode = 'merge'
`,
			resolved: emptyResolved,
			wantErr:  true,
		},
	}
}
