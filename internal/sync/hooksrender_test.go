package sync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hooksRefResult is the canonical JSON emitted by testdata/hooksrender_ref.py.
type hooksRefResult struct {
	Stdout string            `json:"stdout"`
	Stderr string            `json:"stderr"`
	RC     int               `json:"rc"`
	Tree   map[string]string `json:"tree"`
	Modes  map[string]int    `json:"modes"`
}

// hooksRefCase is one differential scenario for the hooks-render port.
//
// resolved nil models a missing resolved-hooks file; resolvedDir models the
// path being a directory (is_file() is false so _load_json_file returns {}).
// files are pre-seeded project files (the agent's settings/hooks JSON, stale TS
// adapters, foreign config). unreadableRel chmods that sandbox path 0200 (read
// denied, write allowed) before running.
type hooksRefCase struct {
	name          string
	agent         string
	resolvedRel   string
	resolved      *string
	resolvedDir   bool
	unreadableRel string
	files         map[string]string

	// tracebackStderr marks the cases where Python fails with an uncaught
	// traceback: only rc / stdout / written tree+modes are byte-compared,
	// Python stderr must contain pythonExc and Go stderr must be one
	// "error: " line.
	tracebackStderr bool
	pythonExc       string
}

const hooksScriptPath = "ai-specs/recipes/hookdemo/hooks/gate.sh"

func hooksStr(s string) *string { return &s }

// hook builds one resolved-hook object. matcher nil omits the key entirely;
// env nil omits it as well.
func hook(recipe, id, event string, matcher any, script string, blocking bool, env map[string]any) map[string]any {
	m := map[string]any{
		"recipe":      recipe,
		"id":          id,
		"event":       event,
		"script_path": script,
		"blocking":    blocking,
	}
	if matcher != nil {
		m["matcher"] = matcher
	}
	if env != nil {
		m["env"] = env
	}
	return m
}

// hooksBlob serializes a resolved-hooks document (enabled_agents + hooks). The
// key order is irrelevant: json.dumps in the port sorts keys, and the reference
// re-encodes it with sort_keys=True too.
func hooksBlob(hooks ...any) string {
	b, err := json.Marshal(map[string]any{
		"enabled_agents": []string{"claude", "cursor", "opencode", "pi", "omp"},
		"hooks":          hooks,
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// hooksRawBlob builds a resolved-hooks document from a raw JSON hook fragment,
// so a \uXXXX lone-surrogate escape survives into the port exactly as
// json.loads keeps it (encoding/json would replace invalid UTF-8 with U+FFFD).
func hooksRawBlob(rawHook string) string {
	return `{"enabled_agents":["claude","cursor","opencode","pi","omp"],"hooks":[` + rawHook + `]}`
}

// hooksAllEvents is one hook per abstract event plus an unknown event, using a
// shell matcher so every harness has a target for pre-tool-use.
func hooksAllEvents() []any {
	return []any{
		hook("hookdemo", "shell-gate", "pre-tool-use", "Bash|Shell|Execute|Terminal", hooksScriptPath, true, nil),
		hook("hookdemo", "post-report", "post-tool-use", "Bash", hooksScriptPath, false, nil),
		hook("hookdemo", "boot", "session-start", "", hooksScriptPath, false, nil),
		hook("hookdemo", "bye", "stop", "", hooksScriptPath, false, nil),
		hook("hookdemo", "custom", "on-custom", "", hooksScriptPath, false, nil),
	}
}

func runHooksRef(t *testing.T, root, script string, tc hooksRefCase) hooksRefResult {
	t.Helper()
	caseJSON := map[string]any{
		"resolved_rel":   tc.resolvedRel,
		"resolved_dir":   tc.resolvedDir,
		"unreadable_rel": tc.unreadableRel,
		"agent":          tc.agent,
	}
	if tc.resolved != nil {
		caseJSON["resolved_b64"] = base64.StdEncoding.EncodeToString([]byte(*tc.resolved))
	} else {
		caseJSON["resolved_b64"] = nil
	}
	files := map[string]string{}
	for rel, content := range tc.files {
		files[rel] = base64.StdEncoding.EncodeToString([]byte(content))
	}
	caseJSON["files"] = files
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
	var ref hooksRefResult
	if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
		t.Fatalf("parse ref JSON: %v\nraw: %s", err, out.String())
	}
	return ref
}

// hooksModeBits mirrors Python's st_mode & 0o7777 (permission bits plus
// setuid/setgid/sticky), which is what the reference driver reports.
func hooksModeBits(m os.FileMode) int {
	b := int(m.Perm())
	if m&os.ModeSetuid != 0 {
		b |= 0o4000
	}
	if m&os.ModeSetgid != 0 {
		b |= 0o2000
	}
	if m&os.ModeSticky != 0 {
		b |= 0o1000
	}
	return b
}

func hooksSnapshot(t *testing.T, root string) (map[string]string, map[string]int) {
	t.Helper()
	tree := map[string]string{}
	modes := map[string]int{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		tree[key] = base64.StdEncoding.EncodeToString(data)
		modes[key] = hooksModeBits(info.Mode())
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return tree, modes
}

// runHooksGo runs the Go port against a fresh sandbox with the same layout the
// ref driver uses, returning stdout/stderr (temp root collapsed to <TEMP>),
// rc, and the written tree + modes.
func runHooksGo(t *testing.T, tc hooksRefCase) (string, string, int, map[string]string, map[string]int) {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range tc.files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	resolvedPath := filepath.Join(dir, filepath.FromSlash(tc.resolvedRel))
	if tc.resolvedDir {
		if err := os.MkdirAll(resolvedPath, 0o755); err != nil {
			t.Fatalf("mkdir resolved: %v", err)
		}
	} else if tc.resolved != nil {
		if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
			t.Fatalf("mkdir resolved dir: %v", err)
		}
		if err := os.WriteFile(resolvedPath, []byte(*tc.resolved), 0o644); err != nil {
			t.Fatalf("write resolved: %v", err)
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
	rc := RenderHooks(resolvedPath, tc.agent, dir, &out, &errBuf)
	if unreadablePath != "" {
		if err := os.Chmod(unreadablePath, 0o644); err != nil {
			t.Fatalf("restore readable: %v", err)
		}
	}
	norm := func(s string) string { return strings.ReplaceAll(s, dir, "<TEMP>") }
	tree, modes := hooksSnapshot(t, dir)
	return norm(out.String()), norm(errBuf.String()), rc, tree, modes
}

// TestHooksRenderDifferential requires byte equality of every written file,
// stdout, stderr and exit code across the Go port and the REAL hooks-render.py.
func TestHooksRenderDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	refScript := filepath.Join(root, "internal", "sync", "testdata", "hooksrender_ref.py")

	for _, tc := range hooksRenderCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unreadableRel != "" && os.Geteuid() == 0 {
				t.Skip("chmod 0200 does not deny reads as root")
			}
			ref := runHooksRef(t, root, refScript, tc)
			goOut, goErr, goRC, goTree, goModes := runHooksGo(t, tc)

			if goOut != ref.Stdout {
				t.Errorf("stdout differs\n--- go ---\n%q\n--- ref ---\n%q", goOut, ref.Stdout)
			}
			if goRC != ref.RC {
				t.Errorf("rc: go=%d ref=%d", goRC, ref.RC)
			}
			if !hooksTreeEqual(goTree, ref.Tree) {
				t.Errorf("tree differs:\n%s", hooksTreeDiff(goTree, ref.Tree))
			}
			if !hooksModesEqual(goModes, ref.Modes) {
				t.Errorf("modes differ:\n  go:  %v\n  ref: %v", goModes, ref.Modes)
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
				return
			}
			if goErr != ref.Stderr {
				t.Errorf("stderr differs\n--- go ---\n%q\n--- ref ---\n%q", goErr, ref.Stderr)
			}
		})
	}
}

func hooksTreeEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func hooksModesEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func hooksTreeDiff(a, b map[string]string) string {
	var sb strings.Builder
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		if a[k] != b[k] {
			sb.WriteString("  " + k + "\n    go:  " + a[k] + "\n    ref: " + b[k] + "\n")
		}
	}
	return sb.String()
}

func hooksRenderCases() []hooksRefCase {
	const rel = "resolved-hooks.json"

	existingSettings := `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [
      {"_ai_specs_managed": "ai-specs:hooks:hookdemo:shell-gate", "matcher": "Old", "hooks": []},
      {"matcher": "UserOwned", "hooks": [{"type": "command", "command": "/usr/bin/true"}]}
    ]
  },
  "foreign": {"nested": [1, 2.5, true, null, "x"]}
}
`
	existingCursorHooks := `{
  "version": 1,
  "hooks": {
    "beforeShellExecution": [
      {"_ai_specs_managed": "ai-specs:hooks:hookdemo:shell-gate", "command": "./.cursor/hooks/stale.sh"},
      {"command": "./.cursor/hooks/user.sh"}
    ]
  }
}
`
	envRich := map[string]any{
		"TOKEN":  "s3cr3t",
		"QUOTE":  `a"b\c`,
		"NUM":    float64(7),
		"FLAG":   true,
		"NONE":   nil,
		"UNI":    "caf\u00e9 \U0001F680",
		"EMPTY":  "",
		"ESCAPE": "line\nbreak\ttab",
	}

	return []hooksRefCase{
		// --- all four abstract events + unknown, every harness ---
		{name: "claude all events", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(hooksAllEvents()...))},
		{name: "cursor all events", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(hooksAllEvents()...))},
		{name: "opencode all events", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(hooksAllEvents()...))},
		{name: "pi all events", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(hooksAllEvents()...))},
		{name: "omp all events", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(hooksAllEvents()...))},

		// --- cursor: file-write matcher has no target (warn+skip) vs shell ---
		{name: "cursor file-write matcher skipped", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "write-gate", "pre-tool-use",
					"Edit|Write|MultiEdit|NotebookEdit", hooksScriptPath, true, nil),
				hook("hookdemo", "shell-gate", "pre-tool-use",
					"Bash|Shell", hooksScriptPath, true, nil),
			))},
		{name: "claude file-write matcher kept", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "write-gate", "pre-tool-use",
					"Edit|Write|MultiEdit|NotebookEdit", hooksScriptPath, true, nil),
			))},
		// Whitespace/pipes in the matcher: only complete tokens count.
		{name: "cursor matcher tokens with padding", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "padded", "pre-tool-use",
					" Edit | Write | Bash ", hooksScriptPath, false, nil),
				hook("hookdemo", "substring", "pre-tool-use",
					"Editor|Rewriter", hooksScriptPath, false, nil),
			))},

		// --- env: non-string values, quotes, unicode, empty map/absent ---
		{name: "claude env rich", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash",
					hooksScriptPath, true, envRich),
				hook("hookdemo", "no-env", "pre-tool-use", "Bash",
					hooksScriptPath, true, nil),
				hook("hookdemo", "empty-env", "pre-tool-use", "Bash",
					hooksScriptPath, true, map[string]any{}),
			))},
		{name: "cursor env rich", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash",
					hooksScriptPath, true, envRich),
			))},
		{name: "opencode env rich", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash",
					hooksScriptPath, true, envRich),
			))},
		{name: "pi env rich", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash",
					hooksScriptPath, true, envRich),
			))},
		{name: "omp env rich", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash",
					hooksScriptPath, true, envRich),
			))},

		// --- claude existing settings: foreign keys, duplicate managed entry
		// replaced, user entry preserved, insertion ordering ---
		{name: "claude existing settings merged", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".claude/settings.json": existingSettings}},
		{name: "claude multiple hooks ordering", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "custom", "on-unknown", "", hooksScriptPath, false, nil),
				hook("hooka", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
				hook("hookb", "write-gate", "pre-tool-use", "Edit|Write", hooksScriptPath, true, nil),
				hook("hookc", "bye", "stop", "", hooksScriptPath, false, nil),
			)),
			files: map[string]string{".claude/settings.json": existingSettings}},
		{name: "claude existing settings invalid json", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".claude/settings.json": `{not valid json`}},
		{name: "claude existing settings non-object", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".claude/settings.json": `[1, 2, 3]`}},
		{name: "claude existing settings duplicate keys", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".claude/settings.json": `{"z": 1, "hooks": null, "a": 2, "NaNlike": NaN}`}},
		{name: "claude existing settings surrogate and bigint", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".claude/settings.json": `{"lone": "\ud800", "pair": "\ud83d\ude00", "big": 123456789012345678901234567890, "inf": Infinity, "neg": -Infinity, "nan": NaN}`}},

		// --- claude settings failure paths ---
		{name: "claude settings invalid utf8", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files:           map[string]string{".claude/settings.json": "\xff\xfe{}"},
			tracebackStderr: true, pythonExc: "UnicodeDecodeError"},
		{name: "claude settings unreadable swallowed", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files:         map[string]string{".claude/settings.json": existingSettings},
			unreadableRel: ".claude/settings.json"},

		// --- cursor existing hooks.json + stale wrapper ---
		{name: "cursor existing hooks merged", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{
				".cursor/hooks.json":      existingCursorHooks,
				".cursor/hooks/old.sh":    "#!/usr/bin/env bash\necho stale\n",
				".cursor/hooks/stale.sh":  "#!/usr/bin/env bash\necho other\n",
				".cursor/other-config.md": "keep me\n",
			}},
		{name: "cursor hooks.json invalid json", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".cursor/hooks.json": `{"version": 1, OOPS`}},
		{name: "cursor wrapper written then hooks json invalid utf8", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files:           map[string]string{".cursor/hooks.json": "\xff\xfe[]"},
			tracebackStderr: true, pythonExc: "UnicodeDecodeError"},

		// --- stale TS adapters preserved for the three TS harnesses ---
		{name: "opencode stale adapter preserved", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{
				".opencode/plugin/old-hook.ts": "// stale adapter\n",
				".opencode/other.txt":          "keep\n",
			}},
		{name: "pi stale adapter preserved", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".pi/extensions/old-hook.ts": "// stale adapter\n"}},
		{name: "omp stale adapter preserved", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			files: map[string]string{".omp/extensions/old-hook.ts": "// stale adapter\n"}},

		// --- missing / invalid resolved hooks ---
		{name: "missing resolved file", agent: "claude", resolvedRel: rel},
		{name: "invalid resolved json", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(`{not valid json`)},
		{name: "non-object resolved json", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(`["not", "a", "dict"]`)},
		{name: "resolved hooks not a list", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(`{"enabled_agents": ["claude"], "hooks": "nope"}`)},
		{name: "agent with no hooks", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob())},
		{name: "resolved invalid utf8", agent: "claude", resolvedRel: rel,
			resolved: hooksStr("\xff\xfe{}"), tracebackStderr: true,
			pythonExc: "UnicodeDecodeError"},
		{name: "resolved unreadable swallowed", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
			)),
			unreadableRel: rel},
		{name: "resolved is a directory", agent: "claude", resolvedRel: rel, resolvedDir: true},

		// --- harness with no renderer ---
		{name: "agent without renderer", agent: "gemini", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(hooksAllEvents()...))},

		// --- non-dict hook entries are skipped ---
		{name: "non-dict hooks skipped", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob("a string", 42, nil, hooksAllEvents()[0]))},

		// --- missing keys Python indexes with hook['key'] → KeyError (rc 1) ---
		{name: "claude missing recipe keyerror", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "KeyError"},
		{name: "opencode missing id keyerror", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "KeyError"},
		{name: "pi missing script_path keyerror", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash",
			})), tracebackStderr: true, pythonExc: "KeyError"},
		{name: "omp missing recipe keyerror", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "KeyError"},
		// The cursor pre-file-write warning f-string indexes recipe/id too.
		{name: "cursor file-write skip missing recipe keyerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"id": "i", "event": "pre-tool-use", "matcher": "Edit|Write", "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "KeyError"},

		// --- script_path non-string: claude/cursor str + x TypeError, TS str() ---
		{name: "claude script_path number typeerror", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": 123,
			})), tracebackStderr: true, pythonExc: "TypeError"},
		{name: "cursor script_path number typeerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": 123,
			})), tracebackStderr: true, pythonExc: "TypeError"},
		{name: "opencode script_path number tolerated", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": 123,
			}))},
		{name: "pi script_path bool tolerated", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": true,
			}))},
		{name: "omp script_path list tolerated", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": []any{"x"},
			}))},

		// --- recipe/id non-string: str() coercion tolerated everywhere ---
		{name: "claude recipe number tolerated", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": 123, "id": 456, "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath,
			}))},
		{name: "opencode recipe bool tolerated", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": true, "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath,
			}))},

		// --- matcher non-string: claude/TS json.dumps it, cursor .split raises ---
		{name: "claude matcher number tolerated", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": 1.5, "script_path": hooksScriptPath,
			}))},
		{name: "cursor matcher number attributeerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": 1.5, "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "AttributeError"},
		{name: "opencode matcher list tolerated", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": []any{"Write"}, "script_path": hooksScriptPath,
			}))},
		{name: "pi matcher object tolerated", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": map[string]any{"a": 1}, "script_path": hooksScriptPath,
			}))},
		{name: "omp matcher bool tolerated", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": true, "script_path": hooksScriptPath,
			}))},
		// A non-str matcher with a non-pre-tool event short-circuits in cursor.
		{name: "cursor non-pre-tool matcher tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "stop", "matcher": 1.5, "script_path": hooksScriptPath,
			}))},

		// --- env non-dict: dict-shape users AttributeError; cursor emulates
		// sorted(env)/env[k], tolerating a list of valid int indices ---
		{name: "claude env number attributeerror", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": 123,
			})), tracebackStderr: true, pythonExc: "AttributeError"},
		{name: "opencode env list attributeerror", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{"A"},
			})), tracebackStderr: true, pythonExc: "AttributeError"},
		{name: "pi env string attributeerror", agent: "pi", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": "x",
			})), tracebackStderr: true, pythonExc: "AttributeError"},
		{name: "omp env number attributeerror", agent: "omp", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": 123,
			})), tracebackStderr: true, pythonExc: "AttributeError"},
		{name: "cursor env number typeerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": 123,
			})), tracebackStderr: true, pythonExc: "TypeError"},
		{name: "cursor env list of strings typeerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{"A"},
			})), tracebackStderr: true, pythonExc: "TypeError"},
		{name: "cursor env list out of range indexerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{2, 0},
			})), tracebackStderr: true, pythonExc: "IndexError"},
		{name: "cursor env list of valid indices tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{0, 1},
			}))},
		{name: "cursor env empty list tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{},
			}))},

		// --- event unhashable (list/dict) → TypeError in EVENT_MAP.get ---
		{name: "claude event list typeerror", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": []any{1}, "matcher": "Bash", "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "TypeError"},
		{name: "opencode event object typeerror", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": map[string]any{"a": 1}, "matcher": "Bash", "script_path": hooksScriptPath,
			})), tracebackStderr: true, pythonExc: "TypeError"},
		// A written hook before the bad event must survive the rc-1 abort.
		{name: "unhashable event after a written hook", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(
				hook("r", "a", "pre-tool-use", "Bash", hooksScriptPath, true, nil),
				map[string]any{"recipe": "r", "id": "b", "event": []any{1}, "script_path": hooksScriptPath},
			)), tracebackStderr: true, pythonExc: "TypeError"},
		// Hashable non-string events are merely unknown → warn-and-skip.
		{name: "claude event number tolerated", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": 123, "matcher": "Bash", "script_path": hooksScriptPath,
			}))},
		{name: "claude event true tolerated", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": true, "matcher": "Bash", "script_path": hooksScriptPath,
			}))},

		// --- lone surrogates reaching raw interpolation: json.dumps escapes
		// them (claude settings, TS env lines); raw module paths, cursor
		// wrappers and TS headers raise UnicodeEncodeError on write_text ---
		{name: "claude script_path surrogate tolerated", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"\ud800"}`))},
		{name: "claude env value surrogate tolerated", agent: "claude", resolvedRel: rel,
			resolved: hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":{"K":"\ud800"}}`))},
		{name: "opencode env value surrogate tolerated", agent: "opencode", resolvedRel: rel,
			resolved: hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":{"K":"\ud800"}}`))},
		{name: "cursor matcher surrogate tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"\ud800","script_path":"x"}`))},
		{name: "cursor recipe surrogate path encode error", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"\ud800","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x"}`)),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "opencode id surrogate path encode error", agent: "opencode", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"\ud800","event":"pre-tool-use","matcher":"Bash","script_path":"x"}`)),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "pi script_path surrogate content encode error", agent: "pi", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"\ud800"}`)),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "omp script_path surrogate content encode error", agent: "omp", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"\ud800"}`)),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "cursor script_path surrogate content encode error", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"\ud800"}`)),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
		{name: "cursor env value surrogate content encode error", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":{"K":"\ud800"}}`)),
			tracebackStderr: true, pythonExc: "UnicodeEncodeError"},
	}
}

// TestHooksJSONDumpsSortedDifferential pins the sort_keys=True writer against
// Python's json.dumps(indent=2, sort_keys=True) for the exact shapes the
// settings/hooks merge produces, including empty containers and nested keys.
func TestHooksJSONDumpsSortedDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	refScript := filepath.Join(root, "internal", "sync", "testdata", "hooksrender_ref.py")

	// Drive the reference through a claude render of a resolved blob whose
	// settings surface is known, then compare the written settings.json bytes.
	blob := hooksBlob(
		hook("hookdemo", "shell-gate", "pre-tool-use", "Bash", hooksScriptPath, true, map[string]any{
			"ZED": "last", "ALPHA": "first",
		}),
	)
	settingsSeed := `{"zz": {}, "aa": [], "mm": {"b": 1, "a": [ {"y": 1, "x": 2} ]}}`
	tc := hooksRefCase{
		name: "sorted dumps", agent: "claude", resolvedRel: "resolved-hooks.json",
		resolved: hooksStr(blob),
		files:    map[string]string{".claude/settings.json": settingsSeed},
	}
	ref := runHooksRef(t, root, refScript, tc)
	_, _, _, goTree, _ := runHooksGo(t, tc)
	if !hooksTreeEqual(goTree, ref.Tree) {
		t.Fatalf("tree differs:\n%s", hooksTreeDiff(goTree, ref.Tree))
	}
}
