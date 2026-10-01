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

	// targetDir creates the target as a directory (is_file() is false but
	// write_text() then raises IsADirectoryError).
	targetDir bool
	// unreadableRel chmods that sandbox path 0200 (read denied, write allowed)
	// before running (skipped as root, where the mode is not enforced).
	unreadableRel string
	// tracebackStderr marks the F1-F3 cases where Python fails with an
	// uncaught traceback: only rc / stdout / target bytes are byte-compared,
	// Python stderr must contain pythonExc and Go stderr must be one
	// "error: " line.
	tracebackStderr bool
	pythonExc       string
	// goErrNotContains guards a misattributed message (F3: a read error must
	// not be reported as "not found").
	goErrNotContains string
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
		"manifest_rel":   mcpManifestRel,
		"target_rel":     tc.targetRel,
		"agent":          tc.agent,
		"mcp_key":        tc.mcpKey,
		"dry_run":        tc.dryRun,
		"recipe_rel":     tc.recipeRel,
		"target_dir":     tc.targetDir,
		"unreadable_rel": tc.unreadableRel,
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
	if tc.targetDir {
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			t.Fatalf("mkdir target dir: %v", err)
		}
	} else if tc.target != nil {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatalf("mkdir target dir: %v", err)
		}
		if err := os.WriteFile(targetPath, []byte(*tc.target), 0o644); err != nil {
			t.Fatalf("write target: %v", err)
		}
	}
	unreadablePath := ""
	if tc.unreadableRel != "" {
		unreadablePath = filepath.Join(dir, filepath.FromSlash(tc.unreadableRel))
		if err := os.Chmod(unreadablePath, 0o200); err != nil {
			t.Fatalf("chmod unreadable: %v", err)
		}
	}
	var out, errBuf bytes.Buffer
	rc := RenderMCPFile(manifestPath, tc.agent, targetPath, tc.mcpKey, RenderMCPOptions{
		RecipeMCPPath: recipePath,
		DryRun:        tc.dryRun,
	}, &out, &errBuf)
	if unreadablePath != "" {
		if err := os.Chmod(unreadablePath, 0o644); err != nil {
			t.Fatalf("restore readable: %v", err)
		}
	}

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
			if tc.unreadableRel != "" && os.Geteuid() == 0 {
				t.Skip("chmod 000 does not deny reads as root")
			}
			ref := runMCPRef(t, root, refScript, tc)
			goOut, goErr, goRC, goTarget := runMCPGo(t, tc)

			if goOut != ref.Stdout {
				t.Errorf("stdout differs\n--- go ---\n%q\n--- ref ---\n%q", goOut, ref.Stdout)
			}
			if goRC != ref.RC {
				t.Errorf("rc: go=%d ref=%d", goRC, ref.RC)
			}
			if !govPtrEqual(goTarget, ref.TargetB64) {
				t.Errorf("target bytes differ\n  go:  %s\n  ref: %s",
					derefString(goTarget), derefString(ref.TargetB64))
			}
			if tc.tracebackStderr {
				// Python's stderr is an unreproducible traceback here.
				if !strings.Contains(ref.Stderr, tc.pythonExc) {
					t.Errorf("ref stderr missing %q:\n%s", tc.pythonExc, ref.Stderr)
				}
				lines := strings.Split(strings.TrimSuffix(goErr, "\n"), "\n")
				if len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
					t.Errorf("go stderr is not exactly one `error: ` line: %q", goErr)
				}
				if tc.goErrNotContains != "" && strings.Contains(goErr, tc.goErrNotContains) {
					t.Errorf("go stderr must not contain %q: %q", tc.goErrNotContains, goErr)
				}
				return
			}
			if goErr != ref.Stderr {
				t.Errorf("stderr differs\n--- go ---\n%q\n--- ref ---\n%q", goErr, ref.Stderr)
			}
		})
	}
}

// TestMCPRenderDryRunSurrogateFails pins the --dry-run leg of the F5 check that
// the differential cannot cover: the Python reference prints through
// contextlib.redirect_stdout(io.StringIO()), which does not encode and so does
// not raise, and a lone surrogate cannot survive the ref driver's JSON
// round-trip back into Go. On a real UTF-8 stdout Python's print() raises
// UnicodeEncodeError (rc 1); the port fails before printing anything.
func TestMCPRenderDryRunSurrogateFails(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "ai-specs.toml")
	if err := os.WriteFile(manifestPath, []byte(mcpBaseManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	recipePath := filepath.Join(dir, "recipe-mcp.json")
	if err := os.WriteFile(recipePath, []byte(`{"\ud800": {"command": "d"}}`), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	targetPath := filepath.Join(dir, ".codex", "config.toml")

	var out, errBuf bytes.Buffer
	rc := RenderMCPFile(manifestPath, "codex", targetPath, "mcp_servers", RenderMCPOptions{
		RecipeMCPPath: recipePath,
		DryRun:        true,
	}, &out, &errBuf)
	if rc != 1 {
		t.Errorf("rc: got %d want 1", rc)
	}
	if out.Len() != 0 {
		t.Errorf("stdout: got %q want empty", out.String())
	}
	lines := strings.Split(strings.TrimSuffix(errBuf.String(), "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
		t.Errorf("stderr is not exactly one `error: ` line: %q", errBuf.String())
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Errorf("dry-run must not create the target: stat err=%v", err)
	}
}

// mcpDeepJSON returns a JSON document nested `depth` arrays deep, generated
// programmatically (100k levels is ~200 KB, too heavy to ship as a Python
// differential fixture).
func mcpDeepJSON(depth int) string {
	return strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
}

// mcpRunRender writes the base manifest into dir and runs RenderMCPFile with
// recipe ("" = flag absent), returning stdout, rc and stderr.
func mcpRunRender(t *testing.T, dir, agent, target, mcpKey, recipe string) (string, int, string) {
	t.Helper()
	manifestPath := filepath.Join(dir, "ai-specs.toml")
	if err := os.WriteFile(manifestPath, []byte(mcpBaseManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var out, errBuf bytes.Buffer
	rc := RenderMCPFile(manifestPath, agent, target, mcpKey, RenderMCPOptions{RecipeMCPPath: recipe}, &out, &errBuf)
	return out.String(), rc, errBuf.String()
}

// TestMCPRenderJSONDepthLimit pins the decoder's nesting bound. Python's
// json.loads raises RecursionError at a stack/version-dependent depth (100000
// accepted on 3.14); unbounded Go recursion dies with a fatal stack overflow
// (rc 2) instead of the port's rc 1, and a depth overflow must NOT degrade to
// {} (nor be silently ignored for --recipe-mcp).
func TestMCPRenderJSONDepthLimit(t *testing.T) {
	dir := t.TempDir()
	over := mcpDeepJSON(mcpJSONMaxDepth + 1)

	atLimit := filepath.Join(dir, "at-limit.json")
	if err := os.WriteFile(atLimit, []byte(mcpDeepJSON(mcpJSONMaxDepth)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, rc, stderr := mcpRunRender(t, dir, "claude", atLimit, "mcpServers", ""); rc != 0 {
		t.Fatalf("at limit: rc=%d want 0 (stderr: %s)", rc, stderr)
	}

	target := filepath.Join(dir, "over-limit.json")
	if err := os.WriteFile(target, []byte(over), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, rc, stderr := mcpRunRender(t, dir, "claude", target, "mcpServers", ""); rc != 1 || out != "" || !strings.HasPrefix(stderr, "error: ") {
		t.Fatalf("target over limit: rc=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if got, _ := os.ReadFile(target); string(got) != over {
		t.Errorf("target changed: got %d bytes want %d", len(got), len(over))
	}

	recipe := filepath.Join(dir, "deep-recipe.json")
	if err := os.WriteFile(recipe, []byte(over), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "fresh.json")
	if out, rc, stderr := mcpRunRender(t, dir, "claude", fresh, "mcpServers", recipe); rc != 1 || out != "" || !strings.HasPrefix(stderr, "error: ") {
		t.Fatalf("recipe over limit: rc=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Errorf("recipe failure must write nothing: stat err=%v", err)
	}
}

// TestMCPRenderSurrogateWriteReportsOpenError pins the F5 write path: Python's
// write_text() opens (and truncates) the target BEFORE encoding, so a failing
// open is reported first (PermissionError), not the surrogate encode error.
// Non-root only: mode bits are not enforced for root.
func TestMCPRenderSurrogateWriteReportsOpenError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only file is not enforced as root")
	}
	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe-mcp.json")
	if err := os.WriteFile(recipe, []byte(`{"\ud800": {"command": "d"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "config.toml")
	original := "[user]\nx = 1\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o644)

	out, rc, stderr := mcpRunRender(t, dir, "codex", target, "mcp_servers", recipe)
	if rc != 1 || out != "" || !strings.Contains(stderr, "permission denied") || strings.Contains(stderr, "lone surrogate") {
		t.Fatalf("rc=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if got, _ := os.ReadFile(target); string(got) != original {
		t.Errorf("target changed: got %q want %q", got, original)
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

		// --- F1: existing target cannot be read (Python read_text raises) ---
		{name: "target unreadable json", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`{"keep": 1}`), unreadableRel: ".mcp.json",
			tracebackStderr: true, pythonExc: "PermissionError"},
		{name: "target unreadable toml", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			target: mcpStr("[user]\nx = 1\n"), unreadableRel: ".codex/config.toml",
			tracebackStderr: true, pythonExc: "PermissionError"},
		{name: "target invalid utf8 json", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target:          mcpStr("\xff\xfe{}"),
			tracebackStderr: true, pythonExc: "UnicodeDecodeError"},
		{name: "target invalid utf8 toml", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			target:          mcpStr("\xff\xfe[user]\n"),
			tracebackStderr: true, pythonExc: "UnicodeDecodeError"},
		{name: "target is a directory", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			targetDir: true, tracebackStderr: true, pythonExc: "IsADirectoryError"},

		// --- F2: recipe-mcp file cannot be read ---
		{name: "recipe mcp unreadable", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json", recipe: mcpStr(`{"delta": {"command": "d"}}`),
			unreadableRel:   "recipe-mcp.json",
			tracebackStderr: true, pythonExc: "PermissionError"},
		{name: "recipe mcp invalid utf8", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json", recipe: mcpStr("\xff\xfe{}"),
			tracebackStderr: true, pythonExc: "UnicodeDecodeError"},
		{name: "recipe mcp invalid json ignored", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json", recipe: mcpStr(`{"delta": `)},

		// --- F3: manifest exists but cannot be read ---
		{name: "manifest unreadable", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			unreadableRel:   mcpManifestRel,
			tracebackStderr: true, pythonExc: "PermissionError",
			goErrNotContains: "not found"},
		{name: "manifest invalid utf8", manifest: mcpStr("\xff\xfe[mcp.a]\n"),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			tracebackStderr: true, pythonExc: "UnicodeDecodeError"},

		// --- F4: json.loads literals NaN/Infinity/-Infinity + lone surrogates ---
		{name: "json nan infinity literals", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`{"n": NaN, "p": Infinity, "m": -Infinity, "mcpServers": {}}`)},
		{name: "json lone surrogate escape", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr(`{"s": "\ud800", "pair": "\ud83d\ude00", "mcpServers": {}}`)},
		{name: "json raw control char rejected", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			target: mcpStr("{\"a\": \"x\x01y\", \"mcpServers\": {}}")},
		{name: "recipe json nan json agent", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"delta": {"command": "d", "env": {"K": NaN, "I": Infinity, "J": -Infinity}}}`)},
		{name: "recipe json nan codex toml", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"delta": {"command": "d", "env": {"K": NaN, "I": Infinity, "J": -Infinity}}}`)},
		{name: "recipe lone surrogate codex toml", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"delta": {"command": "d", "env": {"K": "\ud800"}}}`)},

		// --- F5: a lone surrogate that reaches the rendered content raw. JSON
		// and TOML *values* escape it via json.dumps, but a raw TOML table name
		// ([mcp_servers.<name>]) or inline-table key does not, so Python's
		// write_text re-encode raises UnicodeEncodeError (rc 1; write_text opens
		// the target before encoding, so the file is left created/truncated
		// empty). JSON targets are unaffected (ensure_ascii).
		{name: "surrogate server name codex toml truncates existing target", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			target:          mcpStr("[user]\nx = 1\n"),
			recipeRel:       "recipe-mcp.json",
			recipe:          mcpStr(`{"\ud800": {"command": "d"}}`),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "surrogate server name codex toml creates empty target", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			recipeRel:       "recipe-mcp.json",
			recipe:          mcpStr(`{"\ud800": {"command": "d"}}`),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "surrogate env key codex toml truncates existing target", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".codex/config.toml", agent: "codex", mcpKey: "mcp_servers",
			target:          mcpStr("[user]\nx = 1\n"),
			recipeRel:       "recipe-mcp.json",
			recipe:          mcpStr(`{"delta": {"command": "d", "env": {"\ud800": "v"}}}`),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "surrogate server name json escaped is unaffected", manifest: mcpStr(mcpBaseManifest),
			targetRel: ".mcp.json", agent: "claude", mcpKey: "mcpServers",
			recipeRel: "recipe-mcp.json",
			recipe:    mcpStr(`{"\ud800": {"command": "d"}}`)},
	}
}
