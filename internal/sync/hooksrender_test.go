package sync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- frozen oracle ----------------------------------------------------------

// hooksGoldenCase is one frozen record of testdata/hooksrender_golden.json: the
// properties the live differential compared — rc, stdout, the written tree and
// modes, and stderr. For traceback records Python stderr is an uncaught
// traceback embedding host paths, so Stderr is nil and only the exception-type
// marker is frozen, exactly the containment check the live test made. Tokens is
// the per-field host-path token inventory; this family has none.
type hooksGoldenCase struct {
	Kind      string            `json:"kind"` // render | sorted-dumps
	RC        int               `json:"rc"`
	Stdout    string            `json:"stdout"`
	Stderr    *string           `json:"stderr"`
	ExcMarker string            `json:"exc_marker,omitempty"`
	Tree      map[string]string `json:"tree"`
	Modes     map[string]int    `json:"modes"`
	Tokens    map[string]int    `json:"tokens,omitempty"`
}

// hooksGoldenIdentity is one entry of the hard-coded identity inventory.
type hooksGoldenIdentity struct{ Kind, Name string }

// hooksGoldenInventory is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order and their kind. It is an
// independent oracle of coverage, kept in the test and never derived from the
// bundle, so a missing, renamed, duplicated or reordered record fails before
// any case loop runs.
var hooksGoldenInventory = []hooksGoldenIdentity{
	{"render", "claude all events"},
	{"render", "cursor all events"},
	{"render", "opencode all events"},
	{"render", "pi all events"},
	{"render", "omp all events"},
	{"render", "cursor file-write matcher skipped"},
	{"render", "claude file-write matcher kept"},
	{"render", "cursor matcher tokens with padding"},
	{"render", "claude env rich"},
	{"render", "cursor env rich"},
	{"render", "opencode env rich"},
	{"render", "pi env rich"},
	{"render", "omp env rich"},
	{"render", "claude existing settings merged"},
	{"render", "claude multiple hooks ordering"},
	{"render", "claude existing settings invalid json"},
	{"render", "claude existing settings non-object"},
	{"render", "claude existing settings duplicate keys"},
	{"render", "claude existing settings surrogate and bigint"},
	{"render", "claude settings invalid utf8"},
	{"render", "claude settings unreadable swallowed"},
	{"render", "cursor existing hooks merged"},
	{"render", "cursor hooks.json invalid json"},
	{"render", "cursor wrapper written then hooks json invalid utf8"},
	{"render", "opencode stale adapter preserved"},
	{"render", "pi stale adapter preserved"},
	{"render", "omp stale adapter preserved"},
	{"render", "missing resolved file"},
	{"render", "invalid resolved json"},
	{"render", "non-object resolved json"},
	{"render", "resolved hooks not a list"},
	{"render", "agent with no hooks"},
	{"render", "resolved invalid utf8"},
	{"render", "resolved unreadable swallowed"},
	{"render", "resolved is a directory"},
	{"render", "agent without renderer"},
	{"render", "non-dict hooks skipped"},
	{"render", "claude missing recipe keyerror"},
	{"render", "opencode missing id keyerror"},
	{"render", "pi missing script_path keyerror"},
	{"render", "omp missing recipe keyerror"},
	{"render", "cursor file-write skip missing recipe keyerror"},
	{"render", "claude script_path number typeerror"},
	{"render", "cursor script_path number typeerror"},
	{"render", "opencode script_path number tolerated"},
	{"render", "pi script_path bool tolerated"},
	{"render", "omp script_path list tolerated"},
	{"render", "claude recipe number tolerated"},
	{"render", "opencode recipe bool tolerated"},
	{"render", "claude matcher number tolerated"},
	{"render", "cursor matcher number attributeerror"},
	{"render", "opencode matcher list tolerated"},
	{"render", "pi matcher object tolerated"},
	{"render", "omp matcher bool tolerated"},
	{"render", "cursor non-pre-tool matcher tolerated"},
	{"render", "claude env number attributeerror"},
	{"render", "opencode env list attributeerror"},
	{"render", "pi env string attributeerror"},
	{"render", "omp env number attributeerror"},
	{"render", "cursor env number typeerror"},
	{"render", "cursor env list of strings typeerror"},
	{"render", "cursor env list out of range indexerror"},
	{"render", "cursor env list of valid indices tolerated"},
	{"render", "cursor env list negative index tolerated"},
	{"render", "cursor env list negative and positive index tolerated"},
	{"render", "cursor env list negative out of range indexerror"},
	{"render", "cursor env empty list tolerated"},
	{"render", "cursor raw env list negative index tolerated"},
	{"render", "cursor env list big index cannot fit"},
	{"render", "cursor env list negative big index cannot fit"},
	{"render", "cursor env list big vs int sorts then index out of range"},
	{"render", "cursor env list exact big float order"},
	{"render", "cursor env list big vs string typeerror"},
	{"render", "sort int vs str typeerror"},
	{"render", "sort str vs int typeerror"},
	{"render", "sort float vs str typeerror"},
	{"render", "sort str vs float typeerror"},
	{"render", "sort str vs big typeerror"},
	{"render", "sort big vs nan preserves order index cannot fit"},
	{"render", "sort nan vs big preserves order float index typeerror"},
	{"render", "sort inf vs big index cannot fit"},
	{"render", "sort neg inf vs big float index typeerror"},
	{"render", "claude event list typeerror"},
	{"render", "opencode event object typeerror"},
	{"render", "unhashable event after a written hook"},
	{"render", "claude event number tolerated"},
	{"render", "claude event true tolerated"},
	{"render", "claude script_path surrogate tolerated"},
	{"render", "claude env value surrogate tolerated"},
	{"render", "opencode env value surrogate tolerated"},
	{"render", "cursor matcher surrogate tolerated"},
	{"render", "cursor recipe surrogate path encode error"},
	{"render", "opencode id surrogate path encode error"},
	{"render", "pi script_path surrogate content encode error"},
	{"render", "omp script_path surrogate content encode error"},
	{"render", "cursor script_path surrogate content encode error"},
	{"render", "cursor env value surrogate content encode error"},
	{"sorted-dumps", "sorted dumps"},
}

// hooksGoldenKindCounts is the per-kind count of the inventory above.
var hooksGoldenKindCounts = map[string]int{"render": 97, "sorted-dumps": 1}

// hooksGoldenTokenInventory is the hard-coded host-path token inventory of the
// frozen bundle. This family captures no host path at all, so any record that
// carries a token is a bundle mutation, not a host-path occurrence.
var hooksGoldenTokenInventory = map[string]map[string]int{}

// hooksGoldenDroppedStderr is the dropped-stderr inventory: for each traceback
// record, the exception-type marker that was frozen instead of the Python
// stderr. The Python stderr of those records is an uncaught traceback that
// embeds host paths (verified at capture time), and the live test never
// compared it beyond the marker.
var hooksGoldenDroppedStderr = map[string]string{
	"claude settings invalid utf8":                             "UnicodeDecodeError",
	"cursor wrapper written then hooks json invalid utf8":      "UnicodeDecodeError",
	"resolved invalid utf8":                                    "UnicodeDecodeError",
	"claude missing recipe keyerror":                           "KeyError",
	"opencode missing id keyerror":                             "KeyError",
	"pi missing script_path keyerror":                          "KeyError",
	"omp missing recipe keyerror":                              "KeyError",
	"cursor file-write skip missing recipe keyerror":           "KeyError",
	"claude script_path number typeerror":                      "TypeError",
	"cursor script_path number typeerror":                      "TypeError",
	"cursor matcher number attributeerror":                     "AttributeError",
	"claude env number attributeerror":                         "AttributeError",
	"opencode env list attributeerror":                         "AttributeError",
	"pi env string attributeerror":                             "AttributeError",
	"omp env number attributeerror":                            "AttributeError",
	"cursor env number typeerror":                              "TypeError",
	"cursor env list of strings typeerror":                     "TypeError",
	"cursor env list out of range indexerror":                  "IndexError",
	"cursor env list negative out of range indexerror":         "IndexError",
	"cursor env list big index cannot fit":                     "IndexError",
	"cursor env list negative big index cannot fit":            "IndexError",
	"cursor env list big vs int sorts then index out of range": "IndexError",
	"cursor env list exact big float order":                    "TypeError",
	"cursor env list big vs string typeerror":                  "TypeError",
	"sort int vs str typeerror":                                "TypeError",
	"sort str vs int typeerror":                                "TypeError",
	"sort float vs str typeerror":                              "TypeError",
	"sort str vs float typeerror":                              "TypeError",
	"sort str vs big typeerror":                                "TypeError",
	"sort big vs nan preserves order index cannot fit":         "IndexError",
	"sort nan vs big preserves order float index typeerror":    "TypeError",
	"sort inf vs big index cannot fit":                         "IndexError",
	"sort neg inf vs big float index typeerror":                "TypeError",
	"claude event list typeerror":                              "TypeError",
	"opencode event object typeerror":                          "TypeError",
	"unhashable event after a written hook":                    "TypeError",
	"cursor recipe surrogate path encode error":                "UnicodeEncodeError",
	"opencode id surrogate path encode error":                  "UnicodeEncodeError",
	"pi script_path surrogate content encode error":            "UnicodeEncodeError",
	"omp script_path surrogate content encode error":           "UnicodeEncodeError",
	"cursor script_path surrogate content encode error":        "UnicodeEncodeError",
	"cursor env value surrogate content encode error":          "UnicodeEncodeError",
}

// checkHooksGoldenInventory validates one loaded bundle against the hard-coded
// identity, per-kind, dropped-stderr and token inventories. It is pure so the
// focused control can run it against a mutated in-memory copy.
func checkHooksGoldenInventory(names []string, cases []hooksGoldenCase) error {
	if len(names) != len(hooksGoldenInventory) {
		return fmt.Errorf("frozen oracle has %d names, want %d", len(names), len(hooksGoldenInventory))
	}
	if len(cases) != len(names) {
		return fmt.Errorf("frozen oracle has %d records for %d names", len(cases), len(names))
	}
	seen := make(map[string]bool, len(names))
	kinds := map[string]int{}
	dropped := 0
	for i, tc := range cases {
		want := hooksGoldenInventory[i]
		if names[i] != want.Name {
			return fmt.Errorf("frozen oracle record %d is %q, want %q (identity mismatch)", i, names[i], want.Name)
		}
		if seen[names[i]] {
			return fmt.Errorf("frozen oracle duplicates record %q", names[i])
		}
		seen[names[i]] = true
		if tc.Kind != want.Kind {
			return fmt.Errorf("frozen oracle record %q has kind %q, want %q", names[i], tc.Kind, want.Kind)
		}
		kinds[tc.Kind]++
		if marker, isDropped := hooksGoldenDroppedStderr[names[i]]; isDropped {
			dropped++
			if tc.Stderr != nil {
				return fmt.Errorf("frozen oracle record %q must drop its traceback stderr", names[i])
			}
			if tc.ExcMarker != marker {
				return fmt.Errorf("frozen oracle record %q has exception marker %q, want %q", names[i], tc.ExcMarker, marker)
			}
		} else {
			if tc.Stderr == nil {
				return fmt.Errorf("frozen oracle record %q must freeze its stderr", names[i])
			}
			if tc.ExcMarker != "" {
				return fmt.Errorf("frozen oracle record %q carries an unexpected exception marker %q", names[i], tc.ExcMarker)
			}
		}
		wantTokens, hasTokens := hooksGoldenTokenInventory[names[i]]
		if !hasTokens && len(tc.Tokens) != 0 {
			return fmt.Errorf("frozen oracle record %q carries a non-inventoried token %v", names[i], tc.Tokens)
		}
		for field, n := range tc.Tokens {
			switch field {
			case "stdout", "stderr":
			default:
				return fmt.Errorf("frozen oracle record %q tokenizes unknown field %q", names[i], field)
			}
			if wantTokens[field] != n {
				return fmt.Errorf("frozen oracle record %q has %d host tokens in %s, want %d", names[i], n, field, wantTokens[field])
			}
		}
		if hasTokens && len(tc.Tokens) != len(wantTokens) {
			return fmt.Errorf("frozen oracle record %q tokenizes %d fields, want %d", names[i], len(tc.Tokens), len(wantTokens))
		}
	}
	if dropped != len(hooksGoldenDroppedStderr) {
		return fmt.Errorf("frozen oracle drops %d stderr records, want %d", dropped, len(hooksGoldenDroppedStderr))
	}
	for kind, want := range hooksGoldenKindCounts {
		if kinds[kind] != want {
			return fmt.Errorf("frozen oracle has %d %s records, want %d", kinds[kind], kind, want)
		}
	}
	return nil
}

// loadHooksGolden validates and indexes testdata/hooksrender_golden.json. The
// inventories are asserted before any case loop runs.
func loadHooksGolden(t *testing.T) map[string]hooksGoldenCase {
	t.Helper()
	var bundle struct {
		Names []string          `json:"names"`
		Cases []hooksGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "hooksrender_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if err := checkHooksGoldenInventory(bundle.Names, bundle.Cases); err != nil {
		t.Fatalf("frozen hooks-render oracle: %v", err)
	}
	golden := make(map[string]hooksGoldenCase, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		golden[bundle.Names[i]] = rec
	}
	return golden
}

// frozenHooksRecord looks up one frozen record by its identity name.
func frozenHooksRecord(t *testing.T, golden map[string]hooksGoldenCase, name string) hooksGoldenCase {
	t.Helper()
	rec, ok := golden[name]
	if !ok {
		t.Fatalf("frozen record %q missing", name)
	}
	return rec
}

// TestHooksRenderFrozenInventoryControl proves the inventory validation bites:
// the intact bundle validates, while renaming, removing or duplicating a record
// (all in memory, never on disk) fails loudly.
func TestHooksRenderFrozenInventoryControl(t *testing.T) {
	var bundle struct {
		Names []string          `json:"names"`
		Cases []hooksGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "hooksrender_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if err := checkHooksGoldenInventory(bundle.Names, bundle.Cases); err != nil {
		t.Fatalf("intact inventory must validate: %v", err)
	}

	renamed := append([]string{}, bundle.Names...)
	renamed[0] = "renamed record"
	if err := checkHooksGoldenInventory(renamed, bundle.Cases); err == nil {
		t.Fatal("a renamed record must fail the inventory validation")
	}

	missing := append(append([]hooksGoldenCase{}, bundle.Cases[:1]...), bundle.Cases[2:]...)
	if err := checkHooksGoldenInventory(bundle.Names, missing); err == nil {
		t.Fatal("a removed record must fail the inventory validation")
	}

	duplicated := append(append([]hooksGoldenCase{}, bundle.Cases...), bundle.Cases[0])
	doubledNames := append(append([]string{}, bundle.Names...), bundle.Names[0])
	if err := checkHooksGoldenInventory(doubledNames, duplicated); err == nil {
		t.Fatal("a duplicated record must fail the inventory validation")
	}
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
	// "error: " line. goErrContains additionally pins that the port reproduces
	// the specific Python failure mode (the exception message is otherwise
	// free-form, so rc+tree alone cannot distinguish TypeError from IndexError).
	tracebackStderr bool
	pythonExc       string
	goErrContains   string
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
// stdout, stderr and exit code across the Go port and the frozen records
// captured from the REAL hooks-render.py (PR2-B S8).
func TestHooksRenderDifferential(t *testing.T) {
	golden := loadHooksGolden(t)

	for _, tc := range hooksRenderCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unreadableRel != "" && os.Geteuid() == 0 {
				t.Skip("chmod 0200 does not deny reads as root")
			}
			ref := frozenHooksRecord(t, golden, tc.name)
			if tc.tracebackStderr != (ref.Stderr == nil) {
				t.Fatalf("frozen record stderr/dropped mismatch (traceback=%v)", tc.tracebackStderr)
			}
			goOut, goErr, goRC, goTree, goModes := runHooksGo(t, tc)

			if goOut != ref.Stdout {
				t.Errorf("stdout differs\n--- go ---\n%q\n--- frozen ---\n%q", goOut, ref.Stdout)
			}
			if goRC != ref.RC {
				t.Errorf("rc: go=%d frozen=%d", goRC, ref.RC)
			}
			if !hooksTreeEqual(goTree, ref.Tree) {
				t.Errorf("tree differs:\n%s", hooksTreeDiff(goTree, ref.Tree))
			}
			if !hooksModesEqual(goModes, ref.Modes) {
				t.Errorf("modes differ:\n  go:     %v\n  frozen: %v", goModes, ref.Modes)
			}
			if tc.tracebackStderr {
				// Only the exception-type marker is frozen here; Python's stderr
				// is an unreproducible traceback embedding host paths.
				if ref.ExcMarker != tc.pythonExc {
					t.Errorf("frozen exception marker %q, want %q", ref.ExcMarker, tc.pythonExc)
				}
				lines := strings.Split(strings.TrimSuffix(goErr, "\n"), "\n")
				if len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
					t.Errorf("go stderr is not exactly one `error: ` line: %q", goErr)
				}
				if tc.goErrContains != "" && !strings.Contains(goErr, tc.goErrContains) {
					t.Errorf("go stderr missing %q: %q", tc.goErrContains, goErr)
				}
				return
			}
			if goErr != *ref.Stderr {
				t.Errorf("stderr differs\n--- go ---\n%q\n--- frozen ---\n%q", goErr, *ref.Stderr)
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
		{name: "cursor env list negative index tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{-1},
			}))},
		{name: "cursor env list negative and positive index tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{-2, 1},
			}))},
		{name: "cursor env list negative out of range indexerror", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{-3, 1},
			})), tracebackStderr: true, pythonExc: "IndexError"},
		{name: "cursor env empty list tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksBlob(map[string]any{
				"recipe": "r", "id": "i", "event": "pre-tool-use", "matcher": "Bash", "script_path": hooksScriptPath, "env": []any{},
			}))},

		// --- mcpBigInt (> int64) in the cursor env-list path ---
		{name: "cursor raw env list negative index tolerated", agent: "cursor", resolvedRel: rel,
			resolved: hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[-1]}`))},
		{name: "cursor env list big index cannot fit", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[100000000000000000000]}`)),
			tracebackStderr: true, pythonExc: "IndexError", goErrContains: "cannot fit 'int' into an index-sized integer"},
		{name: "cursor env list negative big index cannot fit", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[-100000000000000000000]}`)),
			tracebackStderr: true, pythonExc: "IndexError", goErrContains: "cannot fit 'int' into an index-sized integer"},
		// The sort must compare the big int numerically (not error), so the
		// first out-of-range int index decides the failure.
		{name: "cursor env list big vs int sorts then index out of range", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[100000000000000000000,5]}`)),
			tracebackStderr: true, pythonExc: "IndexError", goErrContains: "list index out of range"},
		// float64 rounding would put 10**20+1 before 1e20 (equal as float64) and
		// change the failure mode; the exact order puts 1e20 first (float index).
		{name: "cursor env list exact big float order", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[100000000000000000001,1e20]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "not float"},
		{name: "cursor env list big vs string typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[100000000000000000000,"a"]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "'str' and 'int'"},
		// Python's sort names the compared pair in REVERSE input order:
		// [1,'a'] -> "'str' and 'int'", ['a',1] -> "'int' and 'str'".
		{name: "sort int vs str typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[1,"a"]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "'str' and 'int'"},
		{name: "sort str vs int typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":["a",1]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "'int' and 'str'"},
		{name: "sort float vs str typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[1.5,"b"]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "'str' and 'float'"},
		{name: "sort str vs float typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":["b",1.5]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "'float' and 'str'"},
		{name: "sort str vs big typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":["a",100000000000000000000]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "'int' and 'str'"},
		// NaN compares equal, so the sort preserves input order; ±Inf compare
		// exactly. The env-list path then indexes the first element, which is the
		// observable consequence of that order.
		{name: "sort big vs nan preserves order index cannot fit", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[100000000000000000000,NaN]}`)),
			tracebackStderr: true, pythonExc: "IndexError", goErrContains: "cannot fit 'int' into an index-sized integer"},
		{name: "sort nan vs big preserves order float index typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[NaN,100000000000000000000]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "not float"},
		{name: "sort inf vs big index cannot fit", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[Infinity,100000000000000000000]}`)),
			tracebackStderr: true, pythonExc: "IndexError", goErrContains: "cannot fit 'int' into an index-sized integer"},
		{name: "sort neg inf vs big float index typeerror", agent: "cursor", resolvedRel: rel,
			resolved:        hooksStr(hooksRawBlob(`{"recipe":"r","id":"i","event":"pre-tool-use","matcher":"Bash","script_path":"x","env":[-Infinity,100000000000000000000]}`)),
			tracebackStderr: true, pythonExc: "TypeError", goErrContains: "not float"},

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
	golden := loadHooksGolden(t)

	// Drive the frozen capture through a claude render of a resolved blob whose
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
	ref := frozenHooksRecord(t, golden, tc.name)
	_, _, _, goTree, _ := runHooksGo(t, tc)
	if !hooksTreeEqual(goTree, ref.Tree) {
		t.Fatalf("tree differs:\n%s", hooksTreeDiff(goTree, ref.Tree))
	}
}

// hooksCursorSandbox renders one cursor hook into a fresh sandbox and returns
// the project root, which is also the wrapper's $CURSOR_PROJECT_DIR. It also
// installs a hook script that prints hookPayloadPath (when set) and exits 2, so
// the deny branch of the generated wrapper is exercised.
func hooksCursorSandbox(t *testing.T, event, payload string) (dir, wrapperPath string) {
	t.Helper()
	const recipe, id = "hookdemo", "shell-gate"
	dir = t.TempDir()
	resolvedPath := filepath.Join(dir, "resolved-hooks.json")
	blob := hooksBlob(hook(recipe, id, event, "", hooksScriptPath, false, nil))
	if err := os.WriteFile(resolvedPath, []byte(blob), 0o644); err != nil {
		t.Fatalf("write resolved: %v", err)
	}
	var out, errBuf bytes.Buffer
	if rc := RenderHooks(resolvedPath, "cursor", dir, &out, &errBuf); rc != 0 {
		t.Fatalf("RenderHooks rc=%d stderr=%s", rc, errBuf.String())
	}
	scriptPath := filepath.Join(dir, filepath.FromSlash(hooksScriptPath))
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir script: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(
		"#!/usr/bin/env bash\ncat \"$(dirname \"$0\")/payload.bin\"\nexit 2\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(scriptPath), "payload.bin"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return dir, filepath.Join(dir, ".cursor", "hooks", recipe+"-"+id+".sh")
}

// hooksRunCursorWrapper runs a generated wrapper with bash and returns stdout.
// PATH is a temp bin dir of the tools the wrapper legitimately needs, without
// python3, so the run proves the generated runtime does not depend on it.
func hooksRunCursorWrapper(t *testing.T, dir, wrapperPath string) string {
	t.Helper()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	for _, name := range []string{"bash", "env", "awk", "cat", "dirname"} {
		resolved, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("look up %s: %v", name, err)
		}
		if err := os.Symlink(resolved, filepath.Join(binDir, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}
	cmd := exec.Command("bash", wrapperPath)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CURSOR_PROJECT_DIR="+dir, "PATH="+binDir)
	cmd.Stdin = strings.NewReader(`{"query":"echo hi"}`)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("run wrapper: %v\nstderr: %s", err, errBuf.String())
	}
	return out.String()
}

// TestHooksCursorDenyEscapingNoPython pins the deny message escaping: the
// generated wrapper must JSON-encode its own message (quotes, backslash, CR,
// LF, TAB, backspace) without invoking python3.
func TestHooksCursorDenyEscapingNoPython(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	if _, err := exec.LookPath("awk"); err != nil {
		t.Skipf("awk not available: %v", err)
	}
	msg := "denied: quote\" backslash\\ line\nbreak\ttab\rreturn\bback\fform"
	dir, wrapperPath := hooksCursorSandbox(t, "post-tool-use", msg)

	got := hooksRunCursorWrapper(t, dir, wrapperPath)

	var decoded struct {
		Permission   string `json:"permission"`
		AgentMessage string `json:"agent_message"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\nraw: %q", err, got)
	}
	if decoded.Permission != "deny" {
		t.Errorf("permission: got %q, want %q", decoded.Permission, "deny")
	}
	if decoded.AgentMessage != msg {
		t.Errorf("agent_message round-trip failed:\n got %q\nwant %q", decoded.AgentMessage, msg)
	}
}

// TestHooksCursorDenySimpleMessageBytes pins the byte-identical output for a
// plain ASCII message: no trailing newline, message wrapped in quotes.
func TestHooksCursorDenySimpleMessageBytes(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	const msg = "denied: not allowed"
	dir, wrapperPath := hooksCursorSandbox(t, "post-tool-use", msg)

	got := hooksRunCursorWrapper(t, dir, wrapperPath)
	want := `{"permission":"deny","agent_message":"` + msg + `"}`
	if got != want {
		t.Fatalf("stdout bytes differ:\n got %q\nwant %q", got, want)
	}
}

// TestHooksCursorWrapperHasNoPython3 pins that the generated wrapper text does
// not depend on the user's python3 (GO-20 U2e T7).
func TestHooksCursorWrapperHasNoPython3(t *testing.T) {
	_, wrapperPath := hooksCursorSandbox(t, "post-tool-use", "x")
	data, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	if strings.Contains(string(data), "python3") {
		t.Fatalf("wrapper still invokes python3:\n%s", data)
	}
}
