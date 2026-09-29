package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

func TestLoadExpectedGateDigests(t *testing.T) {
	home := t.TempDir()
	sums := filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS")
	good := strings.Repeat("A", 64)
	writeFile(t, sums, strings.Join([]string{
		"# a comment",
		"",
		good + "  worktree-gate-darwin-arm64",
		strings.Repeat("b", 64) + "  other-asset",
		"short  worktree-gate-linux-amd64",
	}, "\n")+"\n")

	digests, ok := loadExpectedGateDigests(home)
	if !ok {
		t.Fatal("a readable trust root must not fail closed")
	}
	want := map[string]string{"worktree-gate-darwin-arm64": strings.ToLower(good)}
	if len(digests) != len(want) || digests["worktree-gate-darwin-arm64"] != want["worktree-gate-darwin-arm64"] {
		t.Fatalf("digests = %v, want %v", digests, want)
	}

	// A missing trust root is an empty map, not a failure.
	if got, ok := loadExpectedGateDigests(t.TempDir()); !ok || len(got) != 0 {
		t.Fatalf("missing trust root = %v, %v; want empty, true", got, ok)
	}

	// An undecodable trust root fails closed.
	if err := os.WriteFile(sums, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := loadExpectedGateDigests(home); ok {
		t.Fatal("an undecodable trust root must fail closed")
	}
}

func TestResolveVerifiedGateBinaryCache(t *testing.T) {
	t.Setenv("WORKTREE_GATE_BIN", "")
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "VERSION"), "0.24.0\n")
	goos, goarch := detectGatePlatform()
	binary := gateCacheBinPath(home, goos, goarch)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(binary), err)
	}
	writeExecutableFile(t, binary, "#!/bin/sh\nexit 0\n")

	// No receipt: never selected.
	if got := resolveVerifiedGateBinary(home); got != "" {
		t.Fatalf("candidate without receipt = %q, want empty", got)
	}

	// Receipt and no committed SHA256SUMS: the historical receipt acceptance.
	writeFile(t, binary+".verified", "status=verified\n")
	if got := resolveVerifiedGateBinary(home); got != binary {
		t.Fatalf("receipted candidate without trust root = %q, want %q", got, binary)
	}

	// Receipt plus a matching committed digest: selected.
	digest, err := fileSHA256(binary)
	if err != nil {
		t.Fatalf("sha256: %v", err)
	}
	sums := filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS")
	asset := "worktree-gate-" + goos + "-" + goarch
	writeFile(t, sums, digest+"  "+asset+"\n")
	if got := resolveVerifiedGateBinary(home); got != binary {
		t.Fatalf("verified candidate = %q, want %q", got, binary)
	}

	// Receipt plus a stale digest: the bytes are rejected.
	writeFile(t, sums, strings.Repeat("b", 64)+"  "+asset+"\n")
	if got := resolveVerifiedGateBinary(home); got != "" {
		t.Fatalf("stale digest = %q, want empty", got)
	}

	// An undecodable trust root fails closed even with a receipt.
	if err := os.WriteFile(sums, []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := resolveVerifiedGateBinary(home); got != "" {
		t.Fatalf("undecodable trust root = %q, want empty", got)
	}
}

func TestGateBinaryVersionAndSelftest(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gate.sh")
	writeExecutableFile(t, bin, "#!/bin/sh\ncase \"$1\" in\n"+
		"  --version) printf '1.2.3\\n' ;;\n"+
		"  --selftest) printf 'ok\\n' ;;\n"+
		"esac\nexit 0\n")
	if got := gateBinaryVersion(bin); got != "1.2.3" {
		t.Fatalf("gateBinaryVersion = %q, want 1.2.3", got)
	}
	if got := gateSelftest(bin); got != "" {
		t.Fatalf("gateSelftest = %q, want empty", got)
	}

	failing := filepath.Join(t.TempDir(), "gate.sh")
	writeExecutableFile(t, failing, "#!/bin/sh\necho 'boom' 1>&2\nexit 3\n")
	if got := gateSelftest(failing); got != "boom" {
		t.Fatalf("failing gateSelftest = %q, want boom", got)
	}
	if got := gateBinaryVersion(failing); got != "" {
		t.Fatalf("non-zero --version = %q, want empty", got)
	}

	// Non-zero selftest with no output falls back to the literal.
	silent := filepath.Join(t.TempDir(), "gate.sh")
	writeExecutableFile(t, silent, "#!/bin/sh\nexit 2\n")
	if got := gateSelftest(silent); got != "selftest failed" {
		t.Fatalf("silent failure = %q, want %q", got, "selftest failed")
	}
}

func TestGateCacheSize(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "cache", "bin", "worktree-gate")
	writeFile(t, filepath.Join(root, "0.24.0", "darwin-arm64", "a"), "12345")
	writeFile(t, filepath.Join(root, "0.24.0", "darwin-arm64", "b"), "123")
	writeFile(t, filepath.Join(root, "0.1.0", "last-digest-mismatch.txt"), "0123456789")
	if got := gateCacheSize(home); got != 18 {
		t.Fatalf("gateCacheSize = %d, want 18", got)
	}
	if got := gateCacheSize(t.TempDir()); got != 0 {
		t.Fatalf("missing cache = %d, want 0", got)
	}
}

func TestClassifyManagedOverride(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	writeFile(t, dest, "disk\n")
	diskSHA := lock.Sha256Bytes([]byte("disk\n"))

	if got := classifyManagedOverride(filepath.Join(dir, "absent"), nil, nil, false); got != "missing" {
		t.Errorf("absent = %q, want missing", got)
	}
	if got := classifyManagedOverride(dest, nil, nil, false); got != "untracked" {
		t.Errorf("no entry = %q, want untracked", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": ""}, nil, false); got != "untracked" {
		t.Errorf("empty sha = %q, want untracked", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": lock.Sha256Bytes([]byte("other"))}, nil, false); got != "user_modified" {
		t.Errorf("changed disk = %q, want user_modified", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, nil, false); got != "managed_current" {
		t.Errorf("no candidate = %q, want managed_current", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, []byte("disk\n"), true); got != "managed_current" {
		t.Errorf("matching candidate = %q, want managed_current", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, []byte("elsewhere\n"), true); got != "managed_stale" {
		t.Errorf("stale candidate = %q, want managed_stale", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, []byte{}, true); got != "managed_stale" {
		t.Errorf("empty candidate = %q, want managed_stale", got)
	}
}

func TestPyStrOr(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{nil, "auto"},
		{"go", "go"},
		{"", "auto"},
		{true, "True"},
		{false, "auto"},
		{int64(0), "auto"},
		{int64(7), "7"},
	}
	for _, tc := range cases {
		if got := pyStrOr(tc.value, "auto"); got != tc.want {
			t.Errorf("pyStrOr(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
