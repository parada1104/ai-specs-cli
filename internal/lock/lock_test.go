package lock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- shared assertions (mirroring the gate module's test helpers) ---

func assertFileBytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file bytes mismatch for %s\n--- got ---\n%q\n--- want ---\n%q", path, got, want)
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".ai-specs.lock.") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// --- header byte identity ---

// TestLockHeaderByteIdentity pins LOCK_HEADER character-for-character against
// the Python authority lib/_internal/lock.py (the same pin the gate module
// carries). A divergence must fail the test.
func TestLockHeaderByteIdentity(t *testing.T) {
	// Copied verbatim from LOCK_HEADER in lib/_internal/lock.py.
	want := "# Managed by ai-specs. Do not edit by hand.\n" +
		"# Provenance stamp: [meta] records the CLI version and timestamp of the last\n" +
		"# sync. [managed.*] records integrity only for CLI-owned override targets;\n" +
		"# it is not a general content-integrity manifest. git covers the committed\n" +
		"# project surface; dep content hashes ([deps.*]) are tracked for drift\n" +
		"# detection; recipe/skill content hashes are not tracked.\n"
	if LockHeader != want {
		t.Fatalf("LockHeader diverges from lib/_internal/lock.py\n--- got ---\n%q\n--- want ---\n%q", LockHeader, want)
	}
}

// --- _toml_string / hasControlChar / sha256 units ---

func TestTOMLString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`plain`, `"plain"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{`a\"b`, `"a\\\"b"`},
		{"", `""`},
		// Control characters stay raw: never \n, \t or any escape sequence.
		{"a\nb", "\"a\nb\""},
		{"a\tb", "\"a\tb\""},
		{"café", `"café"`},
	}
	for _, tc := range cases {
		if got := TOMLString(tc.in); got != tc.want {
			t.Errorf("TOMLString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHasControlChar(t *testing.T) {
	for _, s := range []string{"tab\tinside", "line\nbreak", "\x00nul", "del\x7f"} {
		if !HasControlChar(s) {
			t.Errorf("HasControlChar(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"clean", "café", "spaces   ", "\u00a0nbsp", "~"} {
		if HasControlChar(s) {
			t.Errorf("HasControlChar(%q) = true, want false", s)
		}
	}
}

func TestSha256Bytes(t *testing.T) {
	// CRLF is normalized to LF before hashing.
	sum := sha256.Sum256([]byte("a\nb\nc"))
	want := hex.EncodeToString(sum[:])
	if got := Sha256Bytes([]byte("a\r\nb\r\nc")); got != want {
		t.Errorf("Sha256Bytes CRLF = %q, want %q", got, want)
	}
	if got := Sha256Bytes([]byte("a\nb\nc")); got != want {
		t.Errorf("Sha256Bytes LF = %q, want %q", got, want)
	}
}

func TestSha256OfFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("hello\r\nworld\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("hello\nworld\n"))
	want := hex.EncodeToString(sum[:])
	got, err := Sha256OfFile(path)
	if err != nil {
		t.Fatalf("Sha256OfFile: %v", err)
	}
	if got != want {
		t.Errorf("Sha256OfFile = %q, want %q", got, want)
	}
	if _, err := Sha256OfFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Errorf("Sha256OfFile on a missing file must error")
	}
}

// --- differential write test (Go WriteLock vs Python _write_lock_python) ---

const refDriver = "testdata/write_lock_ref.py"

// runRefDriver runs the Python reference driver. It returns trimmed stdout;
// the driver prints "OK" on success or "ERROR: <message>" on failure and
// exits 1 for the failure case.
func runRefDriver(t *testing.T, args ...string) string {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	cmd := exec.Command(py, append([]string{refDriver}, args...)...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if runErr != nil && !strings.HasPrefix(out, "ERROR:") {
		t.Fatalf("reference driver failed: %v\nstdout: %s\nstderr: %s", runErr, out, stderr.String())
	}
	if out == "" {
		t.Fatalf("reference driver produced no output (err %v, stderr %s)", runErr, stderr.String())
	}
	return out
}

func writeSpecFile(t *testing.T, dir, name, rawJSON string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(rawJSON), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// lockFromSpec builds the Go *Lock from the same JSON spec the Python driver
// consumes, so both sides start from identical inputs.
func lockFromSpec(t *testing.T, raw string) *Lock {
	t.Helper()
	var spec map[string]any
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("spec json: %v", err)
	}
	lk := NewLock()
	if meta, ok := spec["meta"].(map[string]any); ok {
		for k, v := range meta {
			if s, ok := v.(string); ok {
				lk.Meta[k] = s
			}
		}
	}
	if managed, ok := spec["managed"].(map[string]any); ok {
		for path, v := range managed {
			entry, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("managed entry for %q is not an object", path)
			}
			lk.Managed[path] = entry
		}
	}
	if deps, ok := spec["deps"].(map[string]any); ok {
		for depID, v := range deps {
			skills, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("deps entry for %q is not an object", depID)
			}
			for skill, fv := range skills {
				files, ok := fv.(map[string]any)
				if !ok {
					t.Fatalf("deps files for %q/%q is not an object", depID, skill)
				}
				if lk.Deps[depID] == nil {
					lk.Deps[depID] = map[string]map[string]any{}
				}
				lk.Deps[depID][skill] = files
			}
		}
	}
	if agents, ok := spec["agents"].(map[string]any); ok {
		for harness, v := range agents {
			files, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("agents entry for %q is not an object", harness)
			}
			lk.Agents[harness] = files
		}
	}
	return lk
}

// TestWriteLockDifferential runs the Python emission authority against copy A
// and the Go WriteLock against copy B and byte-compares the results. Refusal
// cases require BOTH sides to fail with the identical message.
func TestWriteLockDifferential(t *testing.T) {
	cases := []struct {
		name string
		spec string
	}{
		{"empty lock", `{"meta": {}, "managed": {}, "deps": {}, "agents": {}}`},
		{"meta only", `{"meta": {"cli_version": "0.24.0", "synced_at": "2026-07-14T00:00:00Z"}}`},
		{"meta blank values keep header", `{"meta": {"cli_version": "", "synced_at": ""}}`},
		{"meta synced_at only", `{"meta": {"synced_at": "2026-07-14T00:00:00Z"}}`},
		{"all sections", `{"meta": {"cli_version": "0.24.0"}, "managed": {"AGENTS.md": {"sha256": "abc", "recipe": "wf", "source": "tpl.md", "kind": "gate", "policy": "auto"}}, "deps": {"dep1": {"skill1": {"SKILL.md": "h1", "scripts/run.sh": "h2"}}}, "agents": {"claude": {"AGENTS.md": "ah"}}}`},
		{"managed sha256 only", `{"managed": {"a.md": {"sha256": "abc"}, "b.md": {"sha256": "def", "recipe": "", "source": "", "kind": "", "policy": ""}}}`},
		{"managed entry without sha256 skipped", `{"managed": {"nosha.md": {"recipe": "x", "kind": "template"}, "yes.md": {"sha256": "s"}}, "meta": {"cli_version": "1.0"}}`},
		{"escaping quotes and backslashes", `{"managed": {"we\"ird\\path": {"sha256": "ha\\\"sh"}}, "agents": {"my \"agent\"": {"file\\name.md": "h\"ash"}}, "deps": {"d\\1": {"s\"2": {"re\\l.md": "h"}}}}`},
		{"unicode", `{"agents": {"clé": {"café.md": "héllo"}}, "managed": {"ünïcode/文件.md": {"sha256": "süm"}}}`},
		{"empty maps skipped", `{"deps": {"d1": {"s1": {}, "s2": {"a.md": "h"}}}, "agents": {"empty": {}, "claude": {"AGENTS.md": "h"}}, "meta": {"cli_version": "1.0"}}`},
		{"unsorted insertion order sorts", `{"managed": {"zz.md": {"sha256": "z"}, "aa.md": {"sha256": "a"}, "mm.md": {"sha256": "m"}}, "agents": {"zsh": {"b.md": "zb", "a.md": "za"}, "claude": {"z.md": "cz", "a.md": "ca"}}, "deps": {"z-dep": {"s": {"z.md": "1", "a.md": "2"}}, "a-dep": {"s": {"m.md": "3"}}}, "meta": {"synced_at": "T", "cli_version": "9.9"}}`},
		{"refusal meta newline", `{"meta": {"cli_version": "0.1\n0"}}`},
		{"refusal managed value control char", `{"managed": {"a.md": {"sha256": "abc", "recipe": "wf\u0007x"}}, "agents": {"claude": {"A.md": "h"}}}`},
		{"refusal agents hash DEL", `{"agents": {"claude": {"AGENTS.md": "h\u007fsh"}}}`},
		{"refusal deps skill name newline", `{"deps": {"d": {"s\nk": {"a.md": "h"}}}}`},
		{"refusal deps hash newline", `{"deps": {"d": {"s": {"a.md": "h\nash"}}}}`},
		{"refusal order agents before deps", `{"agents": {"cl\naude": {"A.md": "h"}}, "deps": {"d": {"s": {"a\n.md": "h"}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			pathA := filepath.Join(dir, "a", ".ai-specs.lock")
			pathB := filepath.Join(dir, "b", ".ai-specs.lock")
			specPath := writeSpecFile(t, dir, "spec.json", tc.spec)
			pyOut := runRefDriver(t, "write", specPath, pathA)

			if strings.HasPrefix(pyOut, "ERROR: ") {
				pyMsg := strings.TrimPrefix(pyOut, "ERROR: ")
				goErr := WriteLock(pathB, lockFromSpec(t, tc.spec))
				if goErr == nil {
					t.Fatalf("python refused (%q) but Go wrote the lock", pyMsg)
				}
				if goErr.Error() != pyMsg {
					t.Fatalf("refusal message mismatch\npython: %q\ngo:     %q", pyMsg, goErr.Error())
				}
				if _, err := os.Stat(pathA); err == nil {
					t.Errorf("python refusal wrote a file at %s", pathA)
				}
				if _, err := os.Stat(pathB); err == nil {
					t.Errorf("go refusal wrote a file at %s", pathB)
				}
				return
			}

			if err := WriteLock(pathB, lockFromSpec(t, tc.spec)); err != nil {
				t.Fatalf("go write failed: %v (python succeeded)", err)
			}
			gotA, errA := os.ReadFile(pathA)
			gotB, errB := os.ReadFile(pathB)
			if errA != nil || errB != nil {
				t.Fatalf("read back: errA=%v errB=%v", errA, errB)
			}
			if !bytes.Equal(gotA, gotB) {
				t.Fatalf("byte mismatch\npython:\n%q\ngo:\n%q", gotA, gotB)
			}
		})
	}
}

// --- LoadLock ---

func TestLoadLockMissingFile(t *testing.T) {
	lk, err := LoadLock(filepath.Join(t.TempDir(), "missing.lock"))
	if err != nil {
		t.Fatalf("LoadLock missing: %v", err)
	}
	if lk == nil {
		t.Fatal("LoadLock returned nil")
	}
	for name, m := range map[string]int{
		"skills": len(lk.Skills), "meta": len(lk.Meta), "recipes": len(lk.Recipes),
		"deps": len(lk.Deps), "agents": len(lk.Agents), "managed": len(lk.Managed),
	} {
		if m != 0 {
			t.Errorf("%s = %d entries on a missing file, want 0", name, m)
		}
	}
	// A directory is not a file: is_file() parity returns the empty lock.
	lk, err = LoadLock(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLock on a directory: %v", err)
	}
	if len(lk.Meta) != 0 || len(lk.Managed) != 0 {
		t.Errorf("LoadLock on a directory must return the empty lock")
	}
}

func TestLoadLockParse(t *testing.T) {
	src, err := filepath.Abs(filepath.Join("testdata", "sample.ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	lk, err := LoadLock(src)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	// meta keeps only cli_version/synced_at, stripped, non-blank.
	if len(lk.Meta) != 2 {
		t.Fatalf("meta = %v, want exactly cli_version+synced_at", lk.Meta)
	}
	if lk.Meta["cli_version"] != "0.24.0" || lk.Meta["synced_at"] != "2026-07-14T00:00:00Z" {
		t.Errorf("meta = %v, want stripped values", lk.Meta)
	}
	// managed: sha256 stripped, whole entry kept.
	entry := lk.Managed["AGENTS.md"]
	if entry == nil {
		t.Fatalf("managed AGENTS.md missing: %v", lk.Managed)
	}
	if entry["sha256"] != "abchash" {
		t.Errorf("managed sha256 = %v, want stripped abchash", entry["sha256"])
	}
	if entry["recipe"] != "worktree-flow" || entry["source"] != "AGENTS.tpl.md" || entry["kind"] != "template" || entry["policy"] != "auto" {
		t.Errorf("managed entry = %v, want verbatim fields", entry)
	}
	if lk.Managed["partial.md"]["sha256"] != "partialhash" {
		t.Errorf("managed partial.md = %v", lk.Managed["partial.md"])
	}
	// deps normalized to owner -> skill -> files.
	wantDeps := map[string]string{"SKILL.md": "skillhash", "scripts/run.sh": "scripthash"}
	files := lk.Deps["vendored-demo"]["vendored-demo"]
	if len(files) != len(wantDeps) {
		t.Fatalf("deps files = %v, want %v", files, wantDeps)
	}
	for k, v := range wantDeps {
		if files[k] != v {
			t.Errorf("deps[%q] = %v, want %q", k, files[k], v)
		}
	}
	// recipes normalized the same way.
	if lk.Recipes["worktree-flow"]["hook-sync"]["hooks/gate.sh"] != "hookhash" {
		t.Errorf("recipes worktree-flow = %v", lk.Recipes["worktree-flow"])
	}
	if lk.Recipes["other-recipe"]["top"]["TOP.md"] != "tophash" {
		t.Errorf("recipes other-recipe = %v", lk.Recipes["other-recipe"])
	}
	// agents verbatim dicts, including escaped keys.
	if lk.Agents["claude"]["AGENTS.md"] != "agenthash" {
		t.Errorf("agents claude = %v", lk.Agents["claude"])
	}
	if lk.Agents[`pi "agent"`][`file\name.md`] != "pihash" {
		t.Errorf("agents escaped harness = %v", lk.Agents[`pi "agent"`])
	}
	// legacy skills verbatim.
	if lk.Skills["legacy-skill"]["SKILL.md"] != "legacyhash" {
		t.Errorf("skills = %v", lk.Skills)
	}
}

func TestLoadLockInvalidTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.lock")
	if err := os.WriteFile(path, []byte("[meta\nbroken ===="), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLock(path); err == nil {
		t.Errorf("LoadLock on invalid TOML must error")
	}
}

// TestLoadLockWriteRoundTripDifferential: WriteLock(LoadLock(f)) on the Go
// side vs load_lock+_write_lock_python on the Python side. The [recipes] and
// [skills] sections are DROPPED by the writer on BOTH sides — that IS the
// parity (recorded-defect behavior, do not fix).
func TestLoadLockWriteRoundTripDifferential(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.ai-specs.lock")
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	goOut := filepath.Join(dir, "go.lock")
	lk, err := LoadLock(src)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if err := WriteLock(goOut, lk); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	pyOut := filepath.Join(dir, "py.lock")
	runRefDriver(t, "roundtrip", src, pyOut)
	gotGo, err := os.ReadFile(goOut)
	if err != nil {
		t.Fatal(err)
	}
	gotPy, err := os.ReadFile(pyOut)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotGo, gotPy) {
		t.Fatalf("round-trip byte mismatch\ngo:\n%q\npython:\n%q", gotGo, gotPy)
	}
	if strings.Contains(string(gotGo), "[recipes.") || strings.Contains(string(gotGo), "[skills.") {
		t.Errorf("writer must drop recipes/skills sections, got:\n%s", gotGo)
	}
}

// --- control-char refusal walk order ---

// TestControlCharRefusalOrder pins the first-locator determinism against the
// Python walk order: lock_path, meta, managed (sorted), agents (sorted),
// deps (sorted). Note agents is walked BEFORE deps even though the emitter
// writes deps between managed and agents.
func TestControlCharRefusalOrder(t *testing.T) {
	ctrl := func(s string) string { return s + "\n" }
	mkMeta := func() map[string]string { return map[string]string{"cli_version": "1.0", "synced_at": "T"} }
	cases := []struct {
		name string
		lk   *Lock
		want string
	}{
		{"meta before managed", &Lock{
			Meta:    map[string]string{"synced_at": ctrl("meta")},
			Managed: map[string]map[string]any{ctrl("path"): {"sha256": "s"}},
		}, "meta.synced_at"},
		{"managed before agents", &Lock{
			Meta:    mkMeta(),
			Managed: map[string]map[string]any{"a.md": {"sha256": "s", "kind": ctrl("managed")}},
			Agents:  map[string]map[string]any{ctrl("harness"): {"A.md": "h"}},
		}, "managed.kind"},
		{"managed path even without sha256", &Lock{
			Managed: map[string]map[string]any{ctrl("path"): {"recipe": "r"}},
		}, "managed path"},
		{"managed values skipped without sha256", &Lock{
			Managed: map[string]map[string]any{"ok.md": {"recipe": ctrl("managed")}},
			Agents:  map[string]map[string]any{ctrl("harness"): {"A.md": "h"}},
		}, "agents harness"},
		{"agents before deps", &Lock{
			Agents: map[string]map[string]any{ctrl("harness"): {"A.md": "h"}},
			Deps:   map[string]map[string]map[string]any{"d": {"s": {ctrl("rel"): "h"}}},
		}, "agents harness"},
		{"deps dep id", &Lock{
			Deps: map[string]map[string]map[string]any{ctrl("dep"): {"s": {"rel.md": "h"}}},
		}, "deps dep id"},
		{"deps rel", &Lock{
			Deps: map[string]map[string]map[string]any{"d": {"s": {ctrl("rel"): "h"}}},
		}, "deps rel"},
		{"deps hash", &Lock{
			Deps: map[string]map[string]map[string]any{"d": {"s": {"rel.md": ctrl("hash")}}},
		}, "deps hash"},
		{"agents filename before hash", &Lock{
			Agents: map[string]map[string]any{"h": {ctrl("name"): ctrl("hash")}},
		}, "agents filename"},
		{"agents harness before filename", &Lock{
			Agents: map[string]map[string]any{ctrl("harness"): {ctrl("name"): "hash"}},
		}, "agents harness"},
		{"sorted first wins", &Lock{
			Agents: map[string]map[string]any{
				"a-clean": {"A.md": "h"},
				"z" + ctrl(""): {"A.md": "h"},
			},
		}, "agents harness"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := WriteLock(filepath.Join(t.TempDir(), ".ai-specs.lock"), tc.lk)
			if err == nil {
				t.Fatalf("expected a control-character refusal")
			}
			want := "value for " + tc.want + " contains a control character"
			if err.Error() != want {
				t.Fatalf("refusal = %q, want %q", err.Error(), want)
			}
		})
	}
}

// --- atomicity, modes, temp cleanup ---

func TestAtomicityAndInterruptedWrite(t *testing.T) {
	t.Run("fresh write mode 0600 and no temp files", func(t *testing.T) {
		dir := t.TempDir()
		lockPath := filepath.Join(dir, ".ai-specs.lock")
		if err := WriteLock(lockPath, &Lock{Meta: map[string]string{"cli_version": "1.0"}}); err != nil {
			t.Fatalf("WriteLock: %v", err)
		}
		info, err := os.Stat(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("fresh lock mode = %o, want 600 (mkstemp parity)", got)
		}
		assertNoTempFiles(t, dir)
	})

	t.Run("existing file mode replaced by the temp file's 0600", func(t *testing.T) {
		dir := t.TempDir()
		lockPath := filepath.Join(dir, ".ai-specs.lock")
		if err := os.WriteFile(lockPath, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteLock(lockPath, &Lock{Meta: map[string]string{"cli_version": "1.0"}}); err != nil {
			t.Fatalf("WriteLock: %v", err)
		}
		info, err := os.Stat(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("replaced lock mode = %o, want 600 (os.replace swaps in the temp file's mode)", got)
		}
	})

	t.Run("parent dirs auto-created", func(t *testing.T) {
		dir := t.TempDir()
		lockPath := filepath.Join(dir, "deep", "nested", ".ai-specs.lock")
		if err := WriteLock(lockPath, NewLock()); err != nil {
			t.Fatalf("WriteLock: %v", err)
		}
		assertFileBytes(t, lockPath, LockHeader)
	})

	t.Run("CreateTemp failure leaves the original untouched", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		dir := t.TempDir()
		lockPath := filepath.Join(dir, ".ai-specs.lock")
		original := LockHeader + "\n[meta]\ncli_version = \"old\"\n"
		if err := os.WriteFile(lockPath, []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if err := WriteLock(lockPath, &Lock{Meta: map[string]string{"cli_version": "new"}}); err == nil {
			t.Fatal("write into a read-only directory must fail")
		}
		assertFileBytes(t, lockPath, original)
		assertNoTempFiles(t, dir)
	})

	t.Run("rename failure after temp creation cleans up", func(t *testing.T) {
		dir := t.TempDir()
		// The lock path itself is a non-empty directory: MkdirAll and
		// CreateTemp succeed, os.Rename(file, dir) fails, the temp must be
		// removed and no partial bytes may survive anywhere.
		lockPath := filepath.Join(dir, ".ai-specs.lock")
		if err := os.Mkdir(lockPath, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(lockPath, "child.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := WriteLock(lockPath, &Lock{Meta: map[string]string{"cli_version": "1.0"}}); err == nil {
			t.Fatal("renaming a file onto a directory must fail")
		}
		assertNoTempFiles(t, dir)
		// The "original" (the directory and its child) is untouched.
		if _, err := os.Stat(filepath.Join(lockPath, "child.txt")); err != nil {
			t.Errorf("original content disturbed: %v", err)
		}
	})

	t.Run("success replaces bytes entirely", func(t *testing.T) {
		dir := t.TempDir()
		lockPath := filepath.Join(dir, ".ai-specs.lock")
		if err := os.WriteFile(lockPath, []byte("stale bytes that must not survive"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := WriteLock(lockPath, &Lock{Meta: map[string]string{"cli_version": "new"}}); err != nil {
			t.Fatalf("WriteLock: %v", err)
		}
		assertFileBytes(t, lockPath, LockHeader+"\n[meta]\ncli_version = \"new\"\n")
		assertNoTempFiles(t, dir)
	})
}

// --- mutation helpers ---

func strp(s string) *string { return &s }

func TestMutationHelpers(t *testing.T) {
	t.Run("SetManagedOverride upsert preserves existing keys", func(t *testing.T) {
		lk := NewLock()
		SetManagedOverride(lk, "a.md", "h1", strp("recipe1"), strp("src1"), nil, nil)
		SetManagedOverride(lk, "a.md", "h2", nil, nil, strp("gate"), strp("auto"))
		entry := lk.Managed["a.md"]
		if entry["sha256"] != "h2" {
			t.Errorf("sha256 = %v, want h2", entry["sha256"])
		}
		if entry["recipe"] != "recipe1" || entry["source"] != "src1" {
			t.Errorf("existing keys lost: %v", entry)
		}
		if entry["kind"] != "gate" || entry["policy"] != "auto" {
			t.Errorf("new keys missing: %v", entry)
		}
		// Empty-string values ARE set (Python: value is not None).
		SetManagedOverride(lk, "a.md", "h3", strp(""), nil, nil, nil)
		entry = lk.Managed["a.md"]
		if v, ok := entry["recipe"].(string); !ok || v != "" {
			t.Errorf("recipe = %v, want explicitly empty string", entry["recipe"])
		}
	})

	t.Run("SetManagedOverride on a fresh lock", func(t *testing.T) {
		lk := NewLock()
		SetManagedOverride(lk, "a.md", "h", nil, nil, nil, nil)
		entry := lk.Managed["a.md"]
		if entry["sha256"] != "h" || len(entry) != 1 {
			t.Errorf("entry = %v", entry)
		}
	})

	t.Run("SetGateBaseline pins kind and policy", func(t *testing.T) {
		lk := NewLock()
		SetGateBaseline(lk, "hooks/gate", "gh", strp("wf"), strp("gate.sh"))
		entry := lk.Managed["hooks/gate"]
		if entry["sha256"] != "gh" || entry["recipe"] != "wf" || entry["source"] != "gate.sh" {
			t.Errorf("entry = %v", entry)
		}
		if entry["kind"] != "gate" || entry["policy"] != "auto" {
			t.Errorf("kind/policy = %v/%v, want gate/auto", entry["kind"], entry["policy"])
		}
	})

	t.Run("SetBriefBaseline pins kind and policy", func(t *testing.T) {
		lk := NewLock()
		SetBriefBaseline(lk, "AGENTS.md", "bh")
		entry := lk.Managed["AGENTS.md"]
		if entry["sha256"] != "bh" {
			t.Errorf("entry = %v", entry)
		}
		if entry["kind"] != "runtime-brief" || entry["policy"] != "never-force" {
			t.Errorf("kind/policy = %v/%v, want runtime-brief/never-force", entry["kind"], entry["policy"])
		}
	})

	t.Run("SetRecipeSkillHashes copies and nests", func(t *testing.T) {
		lk := NewLock()
		hashes := map[string]string{"SKILL.md": "h1"}
		SetRecipeSkillHashes(lk, "wf", "hook", hashes)
		hashes["SKILL.md"] = "mutated"
		if lk.Recipes["wf"]["hook"]["SKILL.md"] != "h1" {
			t.Errorf("stored hashes were aliased, not copied: %v", lk.Recipes["wf"]["hook"])
		}
		SetRecipeSkillHashes(lk, "wf", "hook", map[string]string{"SKILL.md": "h2"})
		if lk.Recipes["wf"]["hook"]["SKILL.md"] != "h2" {
			t.Errorf("overwrite failed: %v", lk.Recipes["wf"]["hook"])
		}
	})

	t.Run("SetDepSkillHashes mirrors recipes", func(t *testing.T) {
		lk := NewLock()
		SetDepSkillHashes(lk, "dep1", "skill1", map[string]string{"a.md": "h"})
		if lk.Deps["dep1"]["skill1"]["a.md"] != "h" {
			t.Errorf("Deps = %v", lk.Deps)
		}
	})

	t.Run("Remove helpers return presence", func(t *testing.T) {
		lk := NewLock()
		if RemoveRecipeLockEntries(lk, "wf") {
			t.Errorf("remove on empty lock returned true")
		}
		if RemoveDepLockEntries(lk, "dep1") {
			t.Errorf("remove on empty lock returned true")
		}
		SetRecipeSkillHashes(lk, "wf", "hook", map[string]string{"a.md": "h"})
		SetDepSkillHashes(lk, "dep1", "s", map[string]string{"a.md": "h"})
		if !RemoveRecipeLockEntries(lk, "wf") {
			t.Errorf("remove existing recipe returned false")
		}
		if RemoveRecipeLockEntries(lk, "wf") {
			t.Errorf("second remove returned true")
		}
		if !RemoveDepLockEntries(lk, "dep1") {
			t.Errorf("remove existing dep returned false")
		}
		if len(lk.Recipes) != 0 || len(lk.Deps) != 0 {
			t.Errorf("maps not emptied: %v %v", lk.Recipes, lk.Deps)
		}
		// Nil-map safety matches the Python setdefault chain.
		var nilLK Lock
		if RemoveRecipeLockEntries(&nilLK, "wf") || RemoveDepLockEntries(&nilLK, "d") {
			t.Errorf("remove on nil maps returned true")
		}
		SetRecipeSkillHashes(&nilLK, "wf", "hook", map[string]string{"a.md": "h"})
		if nilLK.Recipes["wf"]["hook"]["a.md"] != "h" {
			t.Errorf("set on nil lock maps failed: %v", nilLK.Recipes)
		}
	})
}
