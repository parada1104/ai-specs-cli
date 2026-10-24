package cli

// `ai-specs hook field [--string] <path>` and `ai-specs hook normalize-path`
// (GO-20 U2e, unit U-F2a): the JSON readers that replace the python3 helpers of
// plan-build-gate.sh. Behavior is pinned through the public Run.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runHook(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"hook"}, args...), t.TempDir(), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestHookFieldMatchesTrackerFieldScalars(t *testing.T) {
	cases := []struct {
		json, path, want string
	}{
		{`{"decision":"block"}`, "decision", "block\n"},
		{`{"active":true}`, "active", "1\n"},
		{`{"active":false}`, "active", "0\n"},
		{`{"reason":null}`, "reason", "\n"},
		{`{"other":1}`, "reason", "\n"},
		{`{"evidence":{"git":"x"}}`, "evidence.git", "x\n"},
	}
	for _, tc := range cases {
		code, stdout, stderr := runHook(t, tc.json, "field", tc.path)
		if code != 0 || stdout != tc.want || stderr != "" {
			t.Errorf("hook field %s on %s = rc %d stdout %q stderr %q; want %q", tc.path, tc.json, code, stdout, stderr, tc.want)
		}
	}
}

func TestHookFieldStringFlagAcceptsOnlyStrings(t *testing.T) {
	code, stdout, _ := runHook(t, `{"central_root":"/repo"}`, "field", "--string", "central_root")
	if code != 0 || stdout != "/repo\n" {
		t.Fatalf("string value = rc %d stdout %q; want 0 and /repo", code, stdout)
	}
	for _, body := range []string{`{"v":5}`, `{"v":true}`, `{"v":null}`, `{"v":["/a"]}`, `{"v":{"a":"b"}}`, `{}`} {
		code, stdout, stderr := runHook(t, body, "field", "--string", "v")
		if code != 1 || stdout != "" || stderr == "" {
			t.Errorf("--string on %s = rc %d stdout %q stderr %q; want rc 1, nothing on stdout", body, code, stdout, stderr)
		}
	}
}

func TestHookFieldInvalidJSONFailsWithNothingPrinted(t *testing.T) {
	for _, body := range []string{``, `{"decision":`, `[1]`, `"x"`} {
		code, stdout, _ := runHook(t, body, "field", "decision")
		if code != 1 || stdout != "" {
			t.Errorf("hook field on %q = rc %d stdout %q; want rc 1, nothing printed", body, code, stdout)
		}
	}
}

func TestHookFieldUsageErrors(t *testing.T) {
	code, _, stderr := runHook(t, `{}`, "field")
	if code != 2 || !strings.Contains(stderr, "<path>") {
		t.Errorf("missing path = rc %d stderr %q; want rc 2", code, stderr)
	}
	code, _, _ = runHook(t, `{}`, "field", "a", "b")
	if code != 2 {
		t.Errorf("two paths rc = %d; want 2", code)
	}
}

// normalizeEvent runs `hook normalize-path` on a hook event and returns the
// verb's exit code and its stdout.
func normalizeEvent(t *testing.T, event string) (int, string) {
	t.Helper()
	code, stdout, stderr := runHook(t, event, "normalize-path")
	if stderr != "" {
		t.Fatalf("normalize-path wrote to stderr: %q", stderr)
	}
	return code, stdout
}

func canonicalDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestHookNormalizePathAbsoluteAndRelativeTargets(t *testing.T) {
	root := canonicalDir(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out := normalizeEvent(t, `{"tool_name":"Write","cwd":"`+root+`","tool_input":{"file_path":"src/new.go"}}`)
	want := "Write\t" + filepath.Join(root, "src", "new.go") + "\t" + filepath.Join(root, "src") + "\n"
	if code != 0 || out != want {
		t.Fatalf("relative target = rc %d out %q; want %q", code, out, want)
	}
	code, out = normalizeEvent(t, `{"tool_name":"Edit","cwd":"/","tool_input":{"file_path":"`+filepath.Join(root, "src", "a.go")+`"}}`)
	if code != 0 || out != "Edit\t"+filepath.Join(root, "src", "a.go")+"\t"+filepath.Join(root, "src")+"\n" {
		t.Fatalf("absolute target = rc %d out %q", code, out)
	}
}

func TestHookNormalizePathNotebookPathFallback(t *testing.T) {
	root := canonicalDir(t, t.TempDir())
	code, out := normalizeEvent(t, `{"tool_name":"NotebookEdit","cwd":"`+root+`","tool_input":{"notebook_path":"n.ipynb"}}`)
	if code != 0 || out != "NotebookEdit\t"+filepath.Join(root, "n.ipynb")+"\t"+root+"\n" {
		t.Fatalf("notebook fallback = rc %d out %q", code, out)
	}
}

func TestHookNormalizePathFailsOpenSilently(t *testing.T) {
	root := t.TempDir()
	for name, event := range map[string]string{
		"invalid json":      `{"tool_input":`,
		"no file path":      `{"tool_name":"Write","cwd":"` + root + `","tool_input":{}}`,
		"empty file path":   `{"cwd":"` + root + `","tool_input":{"file_path":""}}`,
		"non-string path":   `{"cwd":"` + root + `","tool_input":{"file_path":5}}`,
		"non-string cwd":    `{"cwd":5,"tool_input":{"file_path":"a"}}`,
		"non-object event":  `[]`,
		"no tool_input obj": `{"cwd":"` + root + `","tool_input":"x"}`,
	} {
		code, out := normalizeEvent(t, event)
		if code != 0 || out != "" {
			t.Errorf("%s = rc %d out %q; want rc 0 and no output", name, code, out)
		}
	}
}

func TestHookNormalizePathResolvesSymlinkedAncestors(t *testing.T) {
	base := canonicalDir(t, t.TempDir())
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	code, out := normalizeEvent(t, `{"tool_name":"Write","cwd":"`+base+`","tool_input":{"file_path":"link/new.go"}}`)
	if code != 0 || out != "Write\t"+filepath.Join(real, "new.go")+"\t"+real+"\n" {
		t.Fatalf("symlinked ancestor = rc %d out %q", code, out)
	}
}

func TestHookNormalizePathMissingTailClimbsToExistingDir(t *testing.T) {
	root := canonicalDir(t, t.TempDir())
	code, out := normalizeEvent(t, `{"tool_name":"Write","cwd":"`+root+`","tool_input":{"file_path":"a/b/c/new.go"}}`)
	if code != 0 || out != "Write\t"+filepath.Join(root, "a", "b", "c", "new.go")+"\t"+root+"\n" {
		t.Fatalf("missing tail = rc %d out %q", code, out)
	}
}

func TestHookNormalizePathCwdDefaultsToProcessDir(t *testing.T) {
	root := canonicalDir(t, t.TempDir())
	t.Chdir(root)
	code, out := normalizeEvent(t, `{"tool_name":"Write","tool_input":{"file_path":"x.go"}}`)
	if code != 0 || out != "Write\t"+filepath.Join(root, "x.go")+"\t"+root+"\n" {
		t.Fatalf("default cwd = rc %d out %q", code, out)
	}
}

func TestHookUsageErrors(t *testing.T) {
	code, _, stderr := runHook(t, "", "normalize-path", "extra")
	if code != 2 || stderr == "" {
		t.Errorf("extra argument = rc %d stderr %q; want rc 2", code, stderr)
	}
	code, stdout, _ := runHook(t, "", "--help")
	if code != 0 || !strings.Contains(stdout, "field [--string]") || !strings.Contains(stdout, "normalize-path") {
		t.Errorf("hook --help = rc %d stdout %q", code, stdout)
	}
}
