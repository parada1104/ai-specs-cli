package sync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// ---------------------------------------------------------------------------
// S3b: write-governance differential against the real Python authority
// ---------------------------------------------------------------------------

// briefGovRefInput mirrors testdata/briefgov_ref.py's stdin JSON.
type briefGovRefInput struct {
	ManifestTOMLB64   string  `json:"manifest_toml_b64"`
	ExistingAgentsB64 *string `json:"existing_agents_b64"`
	LockTOMLB64       *string `json:"lock_toml_b64"`
	ResolvedJSONB64   *string `json:"resolved_json_b64"`
	AdoptBrief        bool    `json:"adopt_brief"`
	PreserveFlag      bool    `json:"preserve_flag"`
	PolicyValidate    bool    `json:"policy_validate"`
}

// briefGovRefPolicy mirrors the policy leg of the ref driver's output.
type briefGovRefPolicy struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	RC     int    `json:"rc"`
}

// briefGovRefResult mirrors testdata/briefgov_ref.py's stdout JSON.
type briefGovRefResult struct {
	Policy         briefGovRefPolicy `json:"policy"`
	RenderStdout   string            `json:"render_stdout"`
	RenderStderr   string            `json:"render_stderr"`
	RenderRC       int               `json:"render_rc"`
	Action         string            `json:"action"`
	ClassifyState  string            `json:"classify_state"`
	EffectiveState string            `json:"effective_state"`
	Wrote          bool              `json:"wrote"`
	AgentsSHA256   *string           `json:"agents_sha256"`
	LockSHA256     *string           `json:"lock_sha256"`
}

// briefGovCase is one governance scenario. Pointers keep "absent" distinct
// from an empty file; resolved nil means render() runs without a
// --resolved-config file.
type briefGovCase struct {
	name           string
	manifest       string
	existingAgents *string
	lockTOML       *string
	resolved       map[string]any
	adoptBrief     bool
	preserveFlag   bool
	policyValidate bool
}

func govPtr(s string) *string { return &s }

// briefLockTOML builds the [managed."AGENTS.md"] baseline entry lock.py/Go's
// lock package both read.
func briefLockTOML(sha string) string {
	return "[managed.\"AGENTS.md\"]\nsha256 = \"" + sha + "\"\nkind = \"runtime-brief\"\npolicy = \"never-force\"\n"
}

func briefGovCases() []briefGovCase {
	const manifest = "[project]\nname = 'gov'\n\n[agents]\nenabled = ['claude']\n"
	const stale = "# stale brief\n"
	const mine = "# mine\n"
	const withMarker = "# mine\n<!-- ai-specs:runtime-brief -->\n"
	otherBaseline := sha256Hex([]byte("# original\n"))
	return []briefGovCase{
		{name: "missing writes", manifest: manifest},
		{
			name:           "managed_stale writes",
			manifest:       manifest,
			existingAgents: govPtr(stale),
			lockTOML:       govPtr(briefLockTOML(sha256Hex([]byte(stale)))),
		},
		{name: "marker preserves", manifest: manifest, existingAgents: govPtr(withMarker)},
		{
			name:           "marker preserves with inert preserve flag",
			manifest:       manifest,
			existingAgents: govPtr(withMarker),
			preserveFlag:   true,
		},
		{name: "untracked preserves", manifest: manifest, existingAgents: govPtr(mine)},
		{
			name:           "untracked adopts with adopt_brief",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			adoptBrief:     true,
		},
		{
			name:           "user_modified preserves",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			lockTOML:       govPtr(briefLockTOML(otherBaseline)),
		},
		{
			name:           "user_modified adopts with adopt_brief",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			lockTOML:       govPtr(briefLockTOML(otherBaseline)),
			adoptBrief:     true,
		},
		{
			name:           "corrupt lock is undetermined and preserves",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			lockTOML:       govPtr("managed = = broken\n"),
		},
		{
			name:     "brief render false disables the policy gate",
			manifest: "[project]\nname = 'x'\n\n[brief]\nrender = false\n",
		},
		{
			name:           "brief render non-boolean string fails safe under validate",
			manifest:       "[project]\nname = 'x'\n\n[brief]\nrender = 'yes'\n",
			policyValidate: true,
		},
		{
			name:           "brief render non-boolean int fails safe under validate",
			manifest:       "[project]\nname = 'x'\n\n[brief]\nrender = 1\n",
			policyValidate: true,
		},
		{
			name:     "unknown vcs recipe warns on stderr",
			manifest: manifest,
			resolved: map[string]any{
				"bindings": map[string]any{"vcs-pr-flow": "custom-vcs"},
				"recipes":  map[string]any{"custom-vcs": map[string]any{"base_branch": "trunk"}},
				"enabled":  []any{},
			},
		},
	}
}

// setupBriefGovDir materializes the real project layout for one leg.
func setupBriefGovDir(t *testing.T, dir string, tc briefGovCase) (tomlPath, outPath, lockPath, resolvedPath string) {
	t.Helper()
	tomlPath = filepath.Join(dir, "ai-specs", "ai-specs.toml")
	outPath = filepath.Join(dir, "AGENTS.md")
	lockPath = filepath.Join(dir, "ai-specs", ".ai-specs.lock")
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(tomlPath, []byte(tc.manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if tc.existingAgents != nil {
		if err := os.WriteFile(outPath, []byte(*tc.existingAgents), 0o644); err != nil {
			t.Fatalf("write AGENTS.md: %v", err)
		}
	}
	if tc.lockTOML != nil {
		if err := os.WriteFile(lockPath, []byte(*tc.lockTOML), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
	}
	if tc.resolved != nil {
		data, err := json.Marshal(tc.resolved)
		if err != nil {
			t.Fatalf("marshal resolved: %v", err)
		}
		resolvedPath = filepath.Join(dir, "resolved.json")
		if err := os.WriteFile(resolvedPath, data, 0o644); err != nil {
			t.Fatalf("write resolved: %v", err)
		}
	}
	return tomlPath, outPath, lockPath, resolvedPath
}

// runBriefGovRef drives testdata/briefgov_ref.py for one case.
func runBriefGovRef(t *testing.T, root, script string, tc briefGovCase) briefGovRefResult {
	t.Helper()
	in := briefGovRefInput{
		ManifestTOMLB64: base64.StdEncoding.EncodeToString([]byte(tc.manifest)),
		AdoptBrief:      tc.adoptBrief,
		PreserveFlag:    tc.preserveFlag,
		PolicyValidate:  tc.policyValidate,
	}
	if tc.existingAgents != nil {
		b := base64.StdEncoding.EncodeToString([]byte(*tc.existingAgents))
		in.ExistingAgentsB64 = &b
	}
	if tc.lockTOML != nil {
		b := base64.StdEncoding.EncodeToString([]byte(*tc.lockTOML))
		in.LockTOMLB64 = &b
	}
	if tc.resolved != nil {
		data, err := json.Marshal(tc.resolved)
		if err != nil {
			t.Fatalf("marshal resolved: %v", err)
		}
		b := base64.StdEncoding.EncodeToString(data)
		in.ResolvedJSONB64 = &b
	}
	caseJSON, err := json.Marshal(in)
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
	var ref briefGovRefResult
	if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
		t.Fatalf("parse ref JSON: %v\nraw: %s", err, out.String())
	}
	return ref
}

// goBriefPolicy reproduces brief-render-policy.py main()'s observable surface.
func goBriefPolicy(t *testing.T, tomlPath string, validate bool) (string, string, int) {
	t.Helper()
	enabled, evalErr := EvaluateBriefRender(tomlPath)
	var validateErr error
	if validate {
		validateErr = ValidateBriefRender(tomlPath)
	}
	if validateErr != nil {
		return "", "error: " + validateErr.Error() + "\n", 1
	}
	if evalErr != nil {
		return "", "error: " + evalErr.Error() + "\n", 1
	}
	return fmt.Sprintf("%t\n", enabled), "", 0
}

func fileSHA256OrNull(path string) *string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := sha256Hex(data)
	return &s
}

func govPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// TestAgentsRenderGovernanceDifferential runs every governance case through
// both the Go port and the REAL agents-render.py + brief-render-policy.py and
// requires byte equality of the policy surface, the render surface, the raw
// state decision, and the written AGENTS.md / lock bytes.
func TestAgentsRenderGovernanceDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	refScript := filepath.Join(root, "internal", "sync", "testdata", "briefgov_ref.py")

	for _, tc := range briefGovCases() {
		t.Run(tc.name, func(t *testing.T) {
			ref := runBriefGovRef(t, root, refScript, tc)

			goDir := t.TempDir()
			tomlPath, outPath, lockPath, resolvedPath := setupBriefGovDir(t, goDir, tc)

			pOut, pErr, pRC := goBriefPolicy(t, tomlPath, tc.policyValidate)
			if pOut != ref.Policy.Stdout {
				t.Errorf("policy stdout:\n  go: %q\n ref: %q", pOut, ref.Policy.Stdout)
			}
			if pErr != ref.Policy.Stderr {
				t.Errorf("policy stderr:\n  go: %q\n ref: %q", pErr, ref.Policy.Stderr)
			}
			if pRC != ref.Policy.RC {
				t.Errorf("policy rc: go=%d ref=%d", pRC, ref.Policy.RC)
			}

			manifest, resolved, err := loadAgentsRenderInputs(tomlPath, resolvedPath)
			if err != nil {
				t.Fatalf("load inputs: %v", err)
			}
			content := RenderAgentsMarkdown(manifest, resolved)
			if got := classifyBriefState(tomlPath, outPath, content, lockPath, nil); got != ref.ClassifyState {
				t.Errorf("classify state: go=%q ref=%q", got, ref.ClassifyState)
			}
			if got := briefEffectiveState(tomlPath, outPath, content, lockPath, nil); got != ref.EffectiveState {
				t.Errorf("effective state: go=%q ref=%q", got, ref.EffectiveState)
			}

			var rOut, rErr bytes.Buffer
			action, wrote, rc := RenderAgentsFile(tomlPath, outPath, RenderAgentsOptions{
				PreserveIfRuntimeBrief: tc.preserveFlag,
				AdoptBrief:             tc.adoptBrief,
				ResolvedConfigPath:     resolvedPath,
			}, &rOut, &rErr)
			if action != ref.Action {
				t.Errorf("action: go=%q ref=%q", action, ref.Action)
			}
			if wrote != ref.Wrote {
				t.Errorf("wrote: go=%v ref=%v", wrote, ref.Wrote)
			}
			if rc != ref.RenderRC {
				t.Errorf("render rc: go=%d ref=%d", rc, ref.RenderRC)
			}
			if rOut.String() != ref.RenderStdout {
				t.Errorf("render stdout:\n  go: %q\n ref: %q", rOut.String(), ref.RenderStdout)
			}
			if rErr.String() != ref.RenderStderr {
				t.Errorf("render stderr:\n  go: %q\n ref: %q", rErr.String(), ref.RenderStderr)
			}
			if got := fileSHA256OrNull(outPath); !govPtrEqual(got, ref.AgentsSHA256) {
				t.Errorf("AGENTS.md sha: go=%v ref=%v", derefString(got), derefString(ref.AgentsSHA256))
			}
			if got := fileSHA256OrNull(lockPath); !govPtrEqual(got, ref.LockSHA256) {
				t.Errorf("lock sha: go=%v ref=%v", derefString(got), derefString(ref.LockSHA256))
			}
		})
	}
}

// TestRenderAgentsStepPolicyGate pins the sync wiring's two-step contract:
// the policy gate runs first, and a false gate skips the renderer entirely.
func TestRenderAgentsStepPolicyGate(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ai-specs", "ai-specs.toml")
	outPath := filepath.Join(dir, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(tomlPath, []byte("[project]\nname = 'gate'\n\n[brief]\nrender = false\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var out, errW bytes.Buffer
	if rc := renderAgentsStep(tomlPath, outPath, "", false, &out, &errW); rc != 0 {
		t.Fatalf("renderAgentsStep rc = %d, want 0", rc)
	}
	if want := "  ℹ skipped AGENTS.md (brief.render = false)\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md exists despite brief.render = false (err=%v)", err)
	}

	if err := os.WriteFile(tomlPath, []byte("[project]\nname = 'gate'\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	out.Reset()
	errW.Reset()
	if rc := renderAgentsStep(tomlPath, outPath, "", false, &out, &errW); rc != 0 {
		t.Fatalf("renderAgentsStep rc = %d, want 0; stderr=%s", rc, errW.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("AGENTS.md not written: %v", err)
	}
	if !strings.Contains(string(data), "# gate Runtime Brief") {
		t.Errorf("AGENTS.md missing the rendered heading:\n%s", data)
	}
}
