package sync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mcpRefResult is the canonical JSON emitted by testdata/mcprender_ref.py.
type mcpRefResult struct {
	Stdout    string  `json:"stdout"`
	Stderr    string  `json:"stderr"`
	RC        int     `json:"rc"`
	TargetB64 *string `json:"target_b64"`
}

// mcpRefCase is one differential scenario for the mcp-render port.
//
// manifest nil models a missing manifest file. recipeRel non-empty models the
// Bash spine passing --recipe-mcp (recipe nil leaves the file absent, proving
// load_mcp's is_file guard). target nil leaves the target file absent.
type mcpRefCase struct {
	name      string
	manifest  *string
	recipeRel string
	recipe    *string
	targetRel string
	target    *string
	agent     string
	mcpKey    string
	dryRun    bool
}

const mcpManifestRel = "ai-specs/ai-specs.toml"

// mcpBaseManifest exercises the generic (local + HTTP with headers), list-command
// and env-ref paths in one manifest, used across the agent matrix.
const mcpBaseManifest = `[project]
name = 'mcp'

[mcp.alpha]
command = 'npx'
args = ['-y', '@scope/server']
env = { TOKEN = '$ALPHA_TOKEN', PLAIN = 'value', HOME = '${HOME}' }

[mcp.beta]
type = 'http'
url = 'https://example.test/mcp'
headers = { Authorization = '${env:BETA_TOKEN}', XBare = '$BARE' }
timeout = 30

[mcp.gamma]
command = ['node', 'server.js']
`

func mcpStr(s string) *string { return &s }

func mcpB64(s string) *string {
	b := base64.StdEncoding.EncodeToString([]byte(s))
	return &b
}

func runMCPRef(t *testing.T, root, script string, tc mcpRefCase) mcpRefResult {
	t.Helper()
	caseJSON := map[string]any{
		"manifest_rel": mcpManifestRel,
		"target_rel":   tc.targetRel,
		"agent":        tc.agent,
		"mcp_key":      tc.mcpKey,
		"dry_run":      tc.dryRun,
		"recipe_rel":   tc.recipeRel,
	}
	if tc.manifest != nil {
		caseJSON["manifest_b64"] = mcpB64(*tc.manifest)
	} else {
		caseJSON["manifest_b64"] = nil
	}
	if tc.recipe != nil {
		caseJSON["recipe_b64"] = mcpB64(*tc.recipe)
	} else {
		caseJSON["recipe_b64"] = nil
	}
	if tc.target != nil {
		caseJSON["target_b64"] = mcpB64(*tc.target)
	} else {
		caseJSON["target_b64"] = nil
	}
	payload, err := json.Marshal(caseJSON)
	if err != nil {
		t.Fatalf("marshal case: %v", err)
	}
	cmd := exec.Command("python3", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
	cmd.Stdin = bytes.NewReader(payload)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("ref driver failed: %v\nstderr: %s", err, errBuf.String())
	}
	var ref mcpRefResult
	if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
		t.Fatalf("parse ref JSON: %v\nraw: %s", err, out.String())
	}
	return ref
}

// runMCPGo runs the Go port against a fresh sandbox with the same layout the
// ref driver uses, returning stdout/stderr (temp root collapsed to <TEMP>),
// rc and the target bytes.
func runMCPGo(t *testing.T, tc mcpRefCase) (string, string, int, *string) {
	t.Helper()
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, filepath.FromSlash(mcpManifestRel))
	if tc.manifest != nil {
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatalf("mkdir manifest dir: %v", err)
		}
		if err := os.WriteFile(manifestPath, []byte(*tc.manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}
	recipePath := ""
	if tc.recipeRel != "" {
		recipePath = filepath.Join(dir, filepath.FromSlash(tc.recipeRel))
		if tc.recipe != nil {
			if err := os.WriteFile(recipePath, []byte(*tc.recipe), 0o644); err != nil {
				t.Fatalf("write recipe: %v", err)
			}
		}
	}
	targetPath := filepath.Join(dir, filepath.FromSlash(tc.targetRel))
	if tc.target != nil {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatalf("mkdir target dir: %v", err)
		}
		if err := os.WriteFile(targetPath, []byte(*tc.target), 0o644); err != nil {
			t.Fatalf("write target: %v", err)
		}
	}
	var out, errBuf bytes.Buffer
	rc := RenderMCPFile(manifestPath, tc.agent, targetPath, tc.mcpKey, RenderMCPOptions{
		RecipeMCPPath: recipePath,
		DryRun:        tc.dryRun,
	}, &out, &errBuf)

	norm := func(s string) string { return strings.ReplaceAll(s, dir, "<TEMP>") }
	var targetB64 *string
	if data, err := os.ReadFile(targetPath); err == nil {
		b := base64.StdEncoding.EncodeToString(data)
		targetB64 = &b
	}
	return norm(out.String()), norm(errBuf.String()), rc, targetB64
}

// TestMCPRenderDifferential requires byte equality of the written file bytes,
// stdout, stderr and exit code across the Go port and the REAL mcp-render.py.
func TestMCPRenderDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	refScript := filepath.Join(root, "internal", "sync", "testdata", "mcprender_ref.py")

	for _, tc := range mcpRenderCases() {
		t.Run(tc.name, func(t *testing.T) {
			ref := runMCPRef(t, root, refScript, tc)
			goOut, goErr, goRC, goTarget := runMCPGo(t, tc)

			if goOut != ref.Stdout {
				t.Errorf("stdout differs\n--- go ---\n%q\n--- ref ---\n%q", goOut, ref.Stdout)
			}
			if goErr != ref.Stderr {
				t.Errorf("stderr differs\n--- go ---\n%q\n--- ref ---\n%q", goErr, ref.Stderr)
			}
			if goRC != ref.RC {
				t.Errorf("rc: go=%d ref=%d", goRC, ref.RC)
			}
			if !govPtrEqual(goTarget, ref.TargetB64) {
				t.Errorf("target bytes differ\n  go:  %s\n  ref: %s",
					derefString(goTarget), derefString(ref.TargetB64))
			}
		})
	}
}

func mcpRenderCases() []mcpRefCase {
	return []mcpRefCase{
		// --- every MCP agent in the platform.sh matrix, real path/key ---
		{name: "claude .mcp.json mcpServers", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers"},
		{name: "cursor .cursor/mcp.json mcpServers", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".cursor/mcp.json", agent: "cursor", mcpKey: "mcpServers"},
		{name: "opencode opencode.json mcp", manifest: mcpStr(mcpBaseManifest),
			targetRel: "opencode.json", agent: "opencode", mcpKey: "mcp"},
		{name: "codex .codex/config.toml mcp_servers", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers"},
		{name: "gemini .gemini/settings.json mcpServers", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".gemini/settings.json", agent: "gemini", mcpKey: "mcpServers"},
		{name: "pi .mcp.json mcpServers", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "pi", mcpKey: "mcpServers"},
		{name: "omp .omp/mcp.json mcpServers", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".omp/mcp.json", agent: "omp", mcpKey: "mcpServers"},

		// --- existing JSON: foreign keys preserved in order, duplicate keys,
		// non-ASCII + astral, floats, ints vs floats ---
		{
			name:      "existing json preserves keys order duplicates and unicode",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`{"emoji": "🚀", "café": "naïve", "dup": 1, "z": 2, "dup": 3, "pi": 3.14, "n": 7, "whole": 2.0, "exp": 1e3, "neg": -0, "mcpServers": {"keep": {"command": "x"}}}`),
		},
		{
			name:      "existing json numeric extremes round-trip",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`{"big": 123456789012345678901234567890, "ovf": 1e400, "novf": -1e400, "negzero": -0, "tiny": 1e-400, "mcpServers": {}}`),
		},
		{name: "invalid json becomes empty object", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`{not valid json`)},
		{name: "json array becomes empty object", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`[1, 2, 3]`)},
		{name: "json scalar becomes empty object", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`"just a string"`)},

		// --- opencode $schema handling ---
		{name: "opencode schema not first is promoted", manifest: mcpStr(mcpBaseManifest),
			targetRel: "opencode.json", agent: "opencode", mcpKey: "mcp",
			target: mcpStr(`{"theme": "dark", "$schema": "https://custom/schema.json", "other": 1}`)},
		{name: "opencode schema default inserted", manifest: mcpStr(mcpBaseManifest),
			targetRel: "opencode.json", agent: "opencode", mcpKey: "mcp",
			target: mcpStr(`{"theme": "dark"}`)},
		{name: "opencode schema null preserved", manifest: mcpStr(mcpBaseManifest),
			targetRel: "opencode.json", agent: "opencode", mcpKey: "mcp",
			target: mcpStr(`{"$schema": null}`)},

		// --- recipe-mcp merge (recipe overrides, new keys append) + absent file ---
		{
			name:      "recipe mcp merges and overrides",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"alpha": {"command": "override", "args": ["z"]}, "delta": {"command": "d", "env": {"K": "$D_VAR"}}}`),
		},
		{
			name:      "recipe mcp big integer json agent",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"big": {"command": "d", "timeout": 123456789012345678901234567890}}`),
		},
		{
			name:      "recipe mcp big integer codex toml",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"big": {"command": "d", "timeout": 123456789012345678901234567890}}`),
		},
		{
			name:      "recipe mcp non-object json ignored",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`["not", "a", "dict"]`),
		},
		{
			name:      "recipe mcp missing file is ignored",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json", // flag given, file never created
		},

		// --- TOML merge: prior blocks, trailing user tables, blank lines, CRLF ---
		{
			name:      "codex strips prior blocks and keeps user tables",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			target: mcpStr("# user comment\n\n[user_table]\nx = 1\n\n[mcp_servers.old]\ncommand = \"old\"\n\n[mcp_servers.old.env]\nA = \"1\"\n\n[after]\ny = 2\n\n\n"),
		},
		{
			name:      "codex CRLF lines",
			manifest:  mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			target: mcpStr("[user_table]\r\nx = 1\r\n\r\n[mcp_servers.old]\r\ncommand = \"old\"\r\n"),
		},

		// --- env-ref regex quirks (Python `$` before trailing newline; optional
		// independent braces; lowercase/bare forms) ---
		{
			name:      "env ref quirks generic",
			manifest:  mcpStr("[mcp.q]\ncommand = 'x'\nenv = { A = '$ALPHA', B = '${BETA}', C = '${GAMMA', D = 'DELTA}', E = '$lower', F = '$1BAD', G = '$', H = \"$OK\\n\" }\n"),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
		},
		{
			name:      "env ref quirks opencode local",
			manifest:  mcpStr("[mcp.q]\ncommand = 'x'\nargs = ['$A', '${B}', '$lower', 'plain']\nenv = { A = '$ALPHA', B = '${BETA}', H = \"$OK\\n\" }\n"),
			targetRel: "opencode.json", agent: "opencode", mcpKey: "mcp",
		},
		{
			name:      "opencode remote url stays literal",
			manifest:  mcpStr("[mcp.r]\ntype = 'http'\nurl = 'https://x/$TOKEN/${env:OTHER}'\nheaders = { H = '${env:X}' }\n"),
			targetRel: "opencode.json", agent: "opencode", mcpKey: "mcp",
		},

		// --- empty / missing manifest ---
		{name: "missing manifest rc 1", manifest: nil,
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers"},
		{name: "manifest without mcp entries skips", manifest: mcpStr("[project]\nname = 'x'\n"),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers"},

		// --- dry-run for both formats ---
		{name: "dry-run json", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers", dryRun: true},
		{name: "dry-run toml", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers", dryRun: true},
	}
}
