package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Contract tests for the --apply-copy copy actuator (GO-08 WU2, strangler
// slice 5). Go owns the COPY DECISION + EXECUTION for the three blind copiers
// (bundled skills, recipe commands, docs); Python keeps hashing, lock writes,
// warnings and prints behind a fail-open bridge.
//
// stdin envelope: {"items": [{"kind": "bundled-skill"|"command"|"doc",
// "id": "...", "src": "...", "dest": "...", "commands_dir": "..."}]}
// exit 0: {"results": [{"id": ..., "status": "ok"|"source-missing", ...}]}
// exit 2: {"error": "<string>"} on stdout for an invalid envelope or an item
// execution failure (the Python bridge fails open on both).

// runApplyCopyCLI drives the command the way the Python bridge will: one JSON
// envelope on stdin, one JSON envelope on stdout, exit 0/2.
func runApplyCopyCLI(t *testing.T, envelopeJSON string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runApplyCopy(strings.NewReader(envelopeJSON), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

type applyCopyEnvelope struct {
	Results []copyResult `json:"results"`
	Error   *string      `json:"error"`
}

func decodeApplyCopyEnvelope(t *testing.T, out string) applyCopyEnvelope {
	t.Helper()
	var envelope applyCopyEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %q: %v", out, err)
	}
	return envelope
}

// makeTree builds a small skill-like source tree: a nested dir, a plain file,
// an executable script, and distinct mtimes. Returns the tree root.
func makeTree(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]os.FileMode{
		"SKILL.md":       0o644,
		"sub/helper.py":  0o600,
		"scripts/run.sh": 0o755,
	}
	past := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	for rel, mode := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content of "+rel), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
}

// assertTreeIdentity fails when dest does not mirror src exactly: same
// structure, same content, same permission bits, same mtimes (the copytree /
// copy2 parity contract).
func assertTreeIdentity(t *testing.T, src, dest string) {
	t.Helper()
	srcWalk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dest, rel)
		srcInfo, err := d.Info()
		if err != nil {
			return err
		}
		destInfo, err := os.Stat(destPath)
		if err != nil {
			t.Errorf("dest missing entry %s: %v", rel, err)
			return filepath.SkipDir
		}
		if srcInfo.IsDir() != destInfo.IsDir() {
			t.Errorf("entry %s: dir mismatch (src dir=%v dest dir=%v)", rel, srcInfo.IsDir(), destInfo.IsDir())
			return nil
		}
		if srcInfo.IsDir() {
			if srcInfo.Mode().Perm() != destInfo.Mode().Perm() {
				t.Errorf("dir %s: mode %v != src %v", rel, destInfo.Mode().Perm(), srcInfo.Mode().Perm())
			}
			return nil
		}
		srcBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		destBytes, err := os.ReadFile(destPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(srcBytes, destBytes) {
			t.Errorf("file %s: content differs", rel)
		}
		if srcInfo.Mode().Perm() != destInfo.Mode().Perm() {
			t.Errorf("file %s: mode %v != src %v", rel, destInfo.Mode().Perm(), srcInfo.Mode().Perm())
		}
		if !srcInfo.ModTime().Equal(destInfo.ModTime()) {
			t.Errorf("file %s: mtime %v != src %v", rel, destInfo.ModTime(), srcInfo.ModTime())
		}
		return nil
	}
	if err := filepath.WalkDir(src, srcWalk); err != nil {
		t.Fatalf("walking src tree: %v", err)
	}
	// No extra entries on the dest side: the replacement is wholesale.
	destEntries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("reading dest tree: %v", err)
	}
	for _, entry := range destEntries {
		if _, err := os.Lstat(filepath.Join(src, entry.Name())); err != nil {
			t.Errorf("dest has stale extra entry %q", entry.Name())
		}
	}
}

// --- bundled-skill item semantics ---

func TestApplyCopyBundledSkillReplacesDestWholesale(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "recipe", "skills", "my-skill")
	dest := filepath.Join(tmp, "cache", "recipe", "skills", "my-skill")
	makeTree(t, src)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// Stale dest content must vanish under the wholesale replacement.
	if err := os.WriteFile(filepath.Join(dest, "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	envelope := `{"items": [{"kind": "bundled-skill", "id": "my-skill", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
	code, out, stderr := runApplyCopyCLI(t, envelope)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	got := decodeApplyCopyEnvelope(t, out)
	want := []copyResult{{ID: "my-skill", Status: "ok"}}
	if len(got.Results) != 1 || got.Results[0] != want[0] {
		t.Fatalf("results = %+v, want %+v", got.Results, want)
	}
	assertTreeIdentity(t, src, dest)
}

func TestApplyCopyBundledSkillSourceMissing(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "missing-skill")
	dest := filepath.Join(tmp, "cache", "skills", "my-skill")

	envelope := `{"items": [{"kind": "bundled-skill", "id": "my-skill", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
	code, out, _ := runApplyCopyCLI(t, envelope)
	if code != 0 {
		t.Fatalf("exit = %d; source-missing is a per-item result, not a transport failure", code)
	}
	got := decodeApplyCopyEnvelope(t, out)
	if len(got.Results) != 1 || got.Results[0].ID != "my-skill" || got.Results[0].Status != "source-missing" {
		t.Fatalf("results = %+v, want one source-missing entry for my-skill", got.Results)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest must not be created for a missing source, got %v", err)
	}
}

// --- command item semantics ---

func TestApplyCopyCommandIdenticalDestIsNotOverwrite(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "recipe", "commands", "cmd.md")
	dest := filepath.Join(tmp, "cache", "commands", "cmd.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("command body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("command body"), 0o644); err != nil {
		t.Fatal(err)
	}

	envelope := `{"items": [{"kind": "command", "id": "cmd", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
	code, out, _ := runApplyCopyCLI(t, envelope)
	if code != 0 {
		t.Fatal("exit != 0")
	}
	got := decodeApplyCopyEnvelope(t, out)
	if len(got.Results) != 1 || got.Results[0].Status != "ok" || got.Results[0].Overwrite {
		t.Fatalf("results = %+v, want ok without overwrite for identical content", got.Results)
	}
}

func TestApplyCopyCommandDifferingDestSignalsOverwriteAndRewrites(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "recipe", "commands", "cmd.md")
	dest := filepath.Join(tmp, "cache", "commands", "cmd.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old body"), 0o644); err != nil {
		t.Fatal(err)
	}

	envelope := `{"items": [{"kind": "command", "id": "cmd", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
	code, out, _ := runApplyCopyCLI(t, envelope)
	if code != 0 {
		t.Fatal("exit != 0")
	}
	got := decodeApplyCopyEnvelope(t, out)
	if len(got.Results) != 1 || got.Results[0].Status != "ok" || !got.Results[0].Overwrite {
		t.Fatalf("results = %+v, want ok with overwrite for differing content", got.Results)
	}
	body, err := os.ReadFile(dest)
	if err != nil || string(body) != "new body" {
		t.Fatalf("dest not rewritten to src bytes: %q %v", body, err)
	}
}

// A dest that is a directory exercises the Python warn condition's
// `not dest.is_file()` arm: overwrite is signalled so the bridge warns.
func TestApplyCopyCommandDestDirectorySignalsOverwrite(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "recipe", "commands", "cmd.md")
	dest := filepath.Join(tmp, "cache", "commands", "cmd.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	// Go's copy2-parity file write cannot replace a directory with a file:
	// the item fails, which the Python bridge converts into its fail-open
	// fallback (where shutil.copy2 copies INTO the directory, exactly like
	// the reference). The decision data — overwrite=true — is still the
	// contract under test, so drive it through the decision seam directly.
	item := copyItem{Kind: "command", ID: "cmd", Src: src, Dest: dest}
	result, err := applyCopyDecision(&item)
	if err != nil {
		t.Fatalf("applyCopyDecision: %v", err)
	}
	if result.Overwrite != true || result.Status != "ok" {
		t.Fatalf("result = %+v, want overwrite=true before execution fails", result)
	}
}

func TestApplyCopyCommandAndDocSourceMissing(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		kind string
		id   string
	}{
		{"command", "cmd"},
		{"doc", "README.md"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			src := filepath.Join(tmp, "missing-"+tc.id)
			dest := filepath.Join(tmp, "cache", tc.id)
			envelope := `{"items": [{"kind": "` + tc.kind + `", "id": "` + tc.id + `", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
			code, out, _ := runApplyCopyCLI(t, envelope)
			if code != 0 {
				t.Fatal("exit != 0; source-missing is a per-item result")
			}
			got := decodeApplyCopyEnvelope(t, out)
			if len(got.Results) != 1 || got.Results[0].Status != "source-missing" || got.Results[0].ID != tc.id {
				t.Fatalf("results = %+v, want source-missing for %s", got.Results, tc.id)
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatalf("dest must not be created for a missing source")
			}
		})
	}
}

// --- doc item semantics ---

func TestApplyCopyDocIsABlindCopy(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "recipe", "docs", "guide.md")
	dest := filepath.Join(tmp, "project", "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("guide v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("guide v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	envelope := `{"items": [{"kind": "doc", "id": "guide.md", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
	code, out, _ := runApplyCopyCLI(t, envelope)
	if code != 0 {
		t.Fatal("exit != 0")
	}
	got := decodeApplyCopyEnvelope(t, out)
	if len(got.Results) != 1 || got.Results[0].Status != "ok" {
		t.Fatalf("results = %+v", got.Results)
	}
	body, err := os.ReadFile(dest)
	if err != nil || string(body) != "guide v2" {
		t.Fatalf("dest not overwritten: %q %v", body, err)
	}
}

// --- order preservation ---

func TestApplyCopyPreservesEnvelopeOrder(t *testing.T) {
	tmp := t.TempDir()
	srcSkill := filepath.Join(tmp, "skills", "s")
	srcCmd := filepath.Join(tmp, "cmd.md")
	srcDoc := filepath.Join(tmp, "doc.md")
	makeTree(t, srcSkill)
	if err := os.WriteFile(srcCmd, []byte("cmd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcDoc, []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	destSkill := filepath.Join(tmp, "out", "s")
	destCmd := filepath.Join(tmp, "out", "cmd.md")
	destDoc := filepath.Join(tmp, "out", "doc.md")

	// Deliberately not alphabetical: the results order must mirror the
	// envelope order, not any sorted canonical form.
	envelope := `{"items": [` +
		`{"kind": "doc", "id": "z-doc", "src": "` + jsonEscape(srcDoc) + `", "dest": "` + jsonEscape(destDoc) + `"},` +
		`{"kind": "bundled-skill", "id": "a-skill", "src": "` + jsonEscape(srcSkill) + `", "dest": "` + jsonEscape(destSkill) + `"},` +
		`{"kind": "command", "id": "m-cmd", "src": "` + jsonEscape(srcCmd) + `", "dest": "` + jsonEscape(destCmd) + `"}]}`
	code, out, stderr := runApplyCopyCLI(t, envelope)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	got := decodeApplyCopyEnvelope(t, out)
	if len(got.Results) != 3 {
		t.Fatalf("results = %+v, want 3 entries", got.Results)
	}
	wantOrder := []string{"z-doc", "a-skill", "m-cmd"}
	for i, id := range wantOrder {
		if got.Results[i].ID != id {
			t.Fatalf("results[%d].id = %q, want %q (envelope order must be preserved)", i, got.Results[i].ID, id)
		}
		if got.Results[i].Status != "ok" {
			t.Fatalf("results[%d].status = %q, want ok", i, got.Results[i].Status)
		}
	}
	// Every item actually executed.
	assertTreeIdentity(t, srcSkill, destSkill)
	for path, want := range map[string]string{destCmd: "cmd", destDoc: "doc"} {
		body, err := os.ReadFile(path)
		if err != nil || string(body) != want {
			t.Fatalf("%s = %q, %v", path, body, err)
		}
	}
}

// --- CLI contract ---

func TestApplyCopyInvalidEnvelopeExits2(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"not-json", "not json"},
		{"unknown-kind", `{"items": [{"kind": "template", "id": "x", "src": "/a", "dest": "/b"}]}`},
		{"missing-src", `{"items": [{"kind": "doc", "id": "x", "dest": "/b"}]}`},
		{"missing-dest", `{"items": [{"kind": "doc", "id": "x", "src": "/a"}]}`},
		{"missing-id", `{"items": [{"kind": "doc", "src": "/a", "dest": "/b"}]}`},
		{"items-not-list", `{"items": "nope"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, stderr := runApplyCopyCLI(t, tc.payload)
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			got := decodeApplyCopyEnvelope(t, out)
			if got.Error == nil || *got.Error == "" {
				t.Fatalf("stdout = %q, want an {\"error\": ...} envelope", out)
			}
			if got.Results != nil {
				t.Fatalf("refusal must not carry results: %+v", got.Results)
			}
			_ = stderr
		})
	}
}

func TestApplyCopyEmptyItemListSucceeds(t *testing.T) {
	code, out, _ := runApplyCopyCLI(t, `{"items": []}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := decodeApplyCopyEnvelope(t, out)
	if got.Results == nil || len(got.Results) != 0 {
		t.Fatalf("results = %+v, want an empty list", got.Results)
	}
}

// An item execution failure (the dest parent is a regular file, so no parent
// directory can be created) is a transport failure: exit 2 with a structured
// error envelope so the Python bridge fails open and re-runs the item with
// its idempotent reference semantics.
func TestApplyCopyItemExecutionFailureExits2(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "cmd.md")
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("a file, not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(blocker, "cmd.md")

	envelope := `{"items": [{"kind": "command", "id": "cmd", "src": "` + jsonEscape(src) + `", "dest": "` + jsonEscape(dest) + `"}]}`
	code, out, _ := runApplyCopyCLI(t, envelope)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	got := decodeApplyCopyEnvelope(t, out)
	if got.Error == nil || !strings.Contains(*got.Error, "cmd") {
		t.Fatalf("error envelope %v must name the failing item", got.Error)
	}
}

// --- copytree/copy2 primitives ---

func TestCopyFileStatPreservesModeAndMtime(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.sh")
	dest := filepath.Join(tmp, "dest", "dest.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(src, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFileStat(src, dest); err != nil {
		t.Fatalf("copyFileStat: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
	if !info.ModTime().Equal(past) {
		t.Fatalf("mtime = %v, want %v", info.ModTime(), past)
	}
	body, _ := os.ReadFile(dest)
	if string(body) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("content = %q", body)
	}
}

// jsonEscape embeds a path into a JSON string literal.
func jsonEscape(path string) string {
	encoded, err := json.Marshal(path)
	if err != nil {
		panic(err)
	}
	return strings.Trim(string(encoded), `"`)
}

// --- dir-mode parity (GO-08 findings fix) ---

// TestCopyTreeDirModeParity pins the copytree dir-mode contract: the
// directory is created with the umask-filtered default mode and the source's
// S_IMODE bits are applied by the copystat step after the children. A 0o777
// source directory (which a 022 umask would strip from any creation mode)
// must therefore still land at 0o777 on the dest side.
func TestCopyTreeDirModeParity(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dest := filepath.Join(tmp, "dest")
	if err := os.Mkdir(src, 0o777); err != nil {
		t.Fatal(err)
	}
	// Mkdir is umask-filtered; chmod is not, so this pins a genuine 0o777
	// source mode regardless of the test process's umask.
	if err := os.Chmod(src, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, dest); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o777 {
		t.Fatalf("dest dir mode = %o, want 777 (copystat parity, umask-independent)", info.Mode().Perm())
	}
}

// TestCopyTreePreservesSetgidDirBit pins statMode's S_IMODE parity: the
// setgid bit on a source directory (which copystat preserves and plain
// Perm() drops) survives the copy via the post-children chmod.
func TestCopyTreePreservesSetgidDirBit(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dest := filepath.Join(tmp, "dest")
	if err := os.Mkdir(src, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o775|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if srcInfo.Mode()&os.ModeSetgid == 0 {
		t.Skip("setgid bits not preserved in this environment")
	}
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, dest); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if destInfo.Mode()&os.ModeSetgid == 0 || destInfo.Mode().Perm() != 0o775 {
		t.Fatalf("dest dir mode = %v, want setgid + 0755 (copystat S_IMODE parity)", destInfo.Mode())
	}
}
