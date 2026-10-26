package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// configWritesPath is the frozen oracle file for the writes differential. Its
// writes_corpus section holds the frozen input manifests and writes_golden one
// record per write case, so this file never reads another differential's data.
const configWritesPath = "testdata/config_writes.json"

var (
	configWritesOnce sync.Once
	configWrites     map[string]json.RawMessage
	configWritesErr  error
)

func writesSection(t *testing.T, dir string) json.RawMessage {
	t.Helper()
	configWritesOnce.Do(func() {
		raw, err := os.ReadFile(configWritesPath)
		if err != nil {
			configWritesErr = err
			return
		}
		configWritesErr = json.Unmarshal(raw, &configWrites)
	})
	if configWritesErr != nil {
		t.Fatalf("read frozen oracle file %s: %v", configWritesPath, configWritesErr)
	}
	section, ok := configWrites[dir]
	if !ok {
		t.Fatalf("frozen oracle file %s has no %q section", configWritesPath, dir)
	}
	return section
}

// writesCorpusEntry is one frozen corpus input, stored as text unless base64
// marks bytes that are not valid UTF-8.
type writesCorpusEntry struct {
	Base64  bool   `json:"base64"`
	Content string `json:"content"`
}

// mustReadWriteCorpus returns one frozen corpus manifest from this file's
// writes_corpus section.
func mustReadWriteCorpus(t *testing.T, name string) string {
	t.Helper()
	var section map[string]map[string]writesCorpusEntry
	if err := json.Unmarshal(writesSection(t, "writes_corpus"), &section); err != nil {
		t.Fatalf("decode writes_corpus section of %s: %v", configWritesPath, err)
	}
	entries, ok := section["corpus"]
	if !ok {
		t.Fatalf("frozen oracle file %s has no writes_corpus/corpus entries", configWritesPath)
	}
	e, ok := entries[name]
	if !ok {
		t.Fatalf("frozen oracle file %s has no writes_corpus/%s entry", configWritesPath, name)
	}
	if !e.Base64 {
		return e.Content
	}
	data, err := base64.StdEncoding.DecodeString(e.Content)
	if err != nil {
		t.Fatalf("decode base64 corpus entry writes_corpus/%s: %v", name, err)
	}
	return string(data)
}

// writeFrozenRecord is one captured oracle invocation (the retired reference
// script): exit code, path-normalized stdout/stderr, and the manifest bytes
// after the run. Records live in this file's writes_golden section so the
// comparison needs no interpreter.
type writeFrozenRecord struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ResultFile string `json:"result_file"`
}

func mustLoadWriteRecord(t *testing.T, slug string) writeFrozenRecord {
	t.Helper()
	var records map[string]json.RawMessage
	if err := json.Unmarshal(writesSection(t, "writes_golden"), &records); err != nil {
		t.Fatalf("decode writes_golden section of %s: %v", configWritesPath, err)
	}
	raw, ok := records[slug]
	if !ok {
		t.Fatalf("frozen oracle file %s has no writes_golden/%s record", configWritesPath, slug)
	}
	var rec writeFrozenRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("decode frozen write record writes_golden/%s: %v", slug, err)
	}
	return rec
}

type writeCase struct {
	name string
	// manifest content; when empty, read from this file's writes_corpus/<manifestFile>
	manifest     string
	manifestFile string
	// golden is the frozen oracle record key in this file's writes_golden section.
	golden string
	goOp   func(manifestPath string) (string, error)
	// errPrefix: for guard errors the tomllib parse-error detail is
	// diagnostic only (ADR 0002), so compare the message prefix up to and
	// including "invalid TOML:" instead of the full string.
	errPrefix bool
}

func runWriteDiff(t *testing.T, tc writeCase) {
	t.Helper()
	manifest := tc.manifest
	if manifest == "" {
		manifest = mustReadWriteCorpus(t, tc.manifestFile)
	}
	rec := mustLoadWriteRecord(t, tc.golden)

	// dirA is the frozen oracle side, dirB the Go side.
	dirA, dirB := t.TempDir(), t.TempDir()
	pathA := filepath.Join(dirA, "manifest.toml")
	pathB := filepath.Join(dirB, "manifest.toml")
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	goMsg, goErr := tc.goOp(pathB)

	norm := func(s, dir string) string {
		return strings.ReplaceAll(strings.TrimSuffix(s, "\n"), dir, "<DIR>")
	}

	if rec.ExitCode == 0 {
		if goErr != nil {
			t.Fatalf("frozen oracle succeeded but Go op failed: %v", goErr)
		}
		if want, got := norm(rec.Stdout, dirA), norm(goMsg, dirB); want != got {
			t.Errorf("message mismatch:\n  go: %q\n  py: %q", got, want)
		}
		// The frozen post-run manifest is the expected A side.
		if err := os.WriteFile(pathA, []byte(rec.ResultFile), 0o644); err != nil {
			t.Fatalf("write %s: %v", pathA, err)
		}
	} else {
		if goErr == nil {
			t.Fatalf("frozen oracle failed (exit %d) but Go op succeeded with message %q", rec.ExitCode, goMsg)
		}
		want, got := norm(rec.Stderr, dirA), norm(goErr.Error(), dirB)
		if tc.errPrefix {
			const cut = "invalid TOML:"
			want = prefixThrough(want, cut)
			got = prefixThrough(got, cut)
		}
		if want != got {
			t.Errorf("error mismatch:\n  go: %q\n  py: %q", got, want)
		}
		// Error path: the oracle left the file untouched, Go must too.
		if rec.ResultFile != manifest {
			t.Fatalf("frozen oracle record %q changed the manifest on the error path", tc.golden)
		}
	}
	compareFileBytes(t, pathA, pathB)
	checkNoTempFiles(t, dirA)
	checkNoTempFiles(t, dirB)
}

// prefixThrough returns s truncated after the first occurrence of cut
// (inclusive); if cut is absent, s is returned unchanged.
func prefixThrough(s, cut string) string {
	if i := strings.Index(s, cut); i >= 0 {
		return s[:i+len(cut)]
	}
	return s
}

func compareFileBytes(t *testing.T, pathA, pathB string) {
	t.Helper()
	a, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("read %s: %v", pathA, err)
	}
	b, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("read %s: %v", pathB, err)
	}
	if string(a) != string(b) {
		t.Errorf("file bytes diverge:\n  A (%s): %q\n  B (%s): %q", pathA, a, pathB, b)
	}
}

func checkNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".ai-specs.toml.") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s/%s", dir, e.Name())
		}
	}
}

func TestDifferentialWrites(t *testing.T) {
	appendOp := func(depID, url, subdir, scope, trigger, license, attribution, ref string) func(string) (string, error) {
		return func(p string) (string, error) {
			return AppendDepsBlock(p, depID, url, subdir, scope, trigger, license, attribution, ref)
		}
	}

	cases := []writeCase{
		{
			name:         "remove recipe with sub-tables",
			manifestFile: "write_recipe_basic.toml",
			golden:       "remove_recipe_with_subtables",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "a")
				return msg, err
			},
		},
		{
			name:         "remove nonexistent recipe",
			manifestFile: "write_recipe_basic.toml",
			golden:       "remove_nonexistent_recipe",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "missing")
				return msg, err
			},
		},
		{
			name:         "remove recipe from already-invalid TOML (guard does not fire)",
			manifestFile: "write_recipe_invalid_base.toml",
			golden:       "remove_recipe_invalid_base",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "x")
				return msg, err
			},
		},
		{
			name:         "remove recipe valid→invalid guard fires",
			manifestFile: "write_recipe_guard.toml",
			golden:       "remove_recipe_guard",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "doom")
				return msg, err
			},
			errPrefix: true,
		},
		{
			name:         "remove recipe from CRLF manifest (output LF-normalized)",
			manifestFile: "write_recipe_crlf.toml",
			golden:       "remove_recipe_crlf",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "a")
				return msg, err
			},
		},
		{
			name:         "remove recipe CRLF valid→invalid guard fires (file untouched)",
			manifestFile: "write_recipe_crlf_guard.toml",
			golden:       "remove_recipe_crlf_guard",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "doom")
				return msg, err
			},
			errPrefix: true,
		},
		{
			name:         "remove dep first-of-two from CRLF manifest",
			manifestFile: "write_dep_crlf.toml",
			golden:       "remove_dep_crlf",
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "first")
			},
		},
		{
			name:     "append to CRLF manifest",
			manifest: "a = 1\r\nb = 2\r\n",
			golden:   "append_crlf",
			goOp:     appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:     "append to CRLF manifest without trailing newline",
			manifest: "a = 1\r\nb = 2\r",
			golden:   "append_crlf_no_trailing_newline",
			goOp:     appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:         "remove recipe from real repo manifest (sub-table configs)",
			manifestFile: "repo_ai-specs.toml",
			golden:       "remove_recipe_repo_manifest",
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "worktree-flow")
				return msg, err
			},
		},
		{
			name:         "remove dep first-of-two",
			manifestFile: "write_deps_two.toml",
			golden:       "remove_dep_first_of_two",
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "first")
			},
		},
		{
			name:         "remove nonexistent dep",
			manifestFile: "write_deps_two.toml",
			golden:       "remove_nonexistent_dep",
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "missing")
			},
		},
		{
			name:         "remove dep valid→invalid guard fires",
			manifestFile: "write_dep_guard.toml",
			golden:       "remove_dep_guard",
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "doom")
			},
			errPrefix: true,
		},
		{
			name:         "remove dep from already-invalid TOML (guard does not fire)",
			manifestFile: "write_dep_invalid_base.toml",
			golden:       "remove_dep_invalid_base",
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "x")
			},
		},
		{
			name:     "append with all fields",
			manifest: "a = 1\n",
			golden:   "append_all_fields",
			goOp:     appendOp("my-dep", "git::https://x/y", "sub/dir", "root,dev", "on-sync", "MIT", "vendor notes", "main"),
		},
		{
			name:     "append minimal",
			manifest: "a = 1\n",
			golden:   "append_minimal",
			goOp:     appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:     "append to file not ending in newline",
			manifest: "a = 1",
			golden:   "append_no_trailing_newline",
			goOp:     appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:     "append scope CSV with spaces and empties",
			manifest: "a = 1\n",
			golden:   "append_scope_csv",
			goOp:     appendOp("dep", "url", "", " root , , dev ,", "", "", "", ""),
		},
		{
			name:     "append escaping quotes and backslashes",
			manifest: "a = 1\n",
			golden:   "append_escaping",
			goOp:     appendOp(`a"b\c`, `he said "hi" \ ok`, "", "", "", "", "", ""),
		},
		{
			name:         "append onto repo manifest",
			manifestFile: "repo_ai-specs.toml",
			golden:       "append_repo_manifest",
			goOp:         appendOp("new-dep", "git::https://example.com/n", "", "root", "", "", "", ""),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runWriteDiff(t, tc)
		})
	}
}

// TestAtomicReplaceAndMode verifies mode preservation and temp-file hygiene
// for the Go write ops (pure Go; no Python needed).
func TestAtomicReplaceAndMode(t *testing.T) {
	manifest := "[[deps]]\nid = \"x\"\nsource = \"u\"\n\n[after]\ny = 1\n"

	for _, mode := range []os.FileMode{0o755, 0o600} {
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.toml")
		if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}

		if _, err := RemoveDepSegment(path, "x"); err != nil {
			t.Fatalf("mode %o: remove failed: %v", mode, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != mode {
			t.Errorf("mode %o: file mode after write is %o", mode, got)
		}
		checkNoTempFiles(t, dir)
	}

	// Failure path: file and mode untouched, no temp files.
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveDepSegment(path, "missing"); err == nil {
		t.Fatal("expected not-found error")
	}
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	compareFileBytes(t, path, filepath.Join(dir, "probe"))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode changed on failure path: %o", got)
	}
	checkNoTempFiles(t, dir)
}
