package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lockWriteEnvelope decodes the CLI stdout contract: {"written": true} on
// success or {"error": "<string>"} on a structured refusal, both exit 2-style
// JSON envelopes per the --write-lock contract.
type lockWriteEnvelope struct {
	Written bool    `json:"written"`
	Error   *string `json:"error"`
}

func decodeLockWriteEnvelope(t *testing.T, out string) lockWriteEnvelope {
	t.Helper()
	var envelope lockWriteEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %q: %v", out, err)
	}
	return envelope
}

// runWriteLockCLI drives the command the way the Python bridge will: one JSON
// envelope on stdin, one JSON envelope on stdout, exit 0/2.
func runWriteLockCLI(t *testing.T, envelopeJSON string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runWriteLock(strings.NewReader(envelopeJSON), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// --- header byte identity ---

// TestLockHeaderByteIdentity pins LOCK_HEADER character-for-character against
// the Python authority: lib/_internal/lock.py:9-15. A divergence here fails
// the test — the lock header is part of the emitted format and slices 6/7
// read these bytes.
func TestLockHeaderByteIdentity(t *testing.T) {
	// Copied verbatim from LOCK_HEADER in lib/_internal/lock.py:9-15.
	want := "# Managed by ai-specs. Do not edit by hand.\n" +
		"# Provenance stamp: [meta] records the CLI version and timestamp of the last\n" +
		"# sync. [managed.*] records integrity only for CLI-owned override targets;\n" +
		"# it is not a general content-integrity manifest. git covers the committed\n" +
		"# project surface; skill/recipe/dep content hashes are not tracked.\n"
	if lockHeader != want {
		t.Fatalf("lockHeader diverges from lib/_internal/lock.py:9-15\n--- got ---\n%q\n--- want ---\n%q", lockHeader, want)
	}
}

// --- _toml_string parity ---

// TestLockTOMLString pins the Python _toml_string contract (lock.py:81-83):
// escape backslash then double-quote ONLY. Control characters are emitted
// raw — never \n, \t or any other escape sequence.
func TestLockTOMLString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`plain`, `"plain"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{`a\"b`, `"a\\\"b"`},
		{"", `""`},
		// Control characters stay raw: a newline inside the value is a real
		// newline byte inside the quotes, never the two-byte escape \n.
		{"a\nb", "\"a\nb\""},
		{"a\tb", "\"a\tb\""},
		{"caf\xc3\xa9", `"café"`},
	}
	for _, tc := range cases {
		if got := lockTOMLString(tc.in); got != tc.want {
			t.Errorf("lockTOMLString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if escaped := lockTOMLString("x\ny"); strings.Contains(escaped, `\n`) || strings.Contains(escaped, `\t`) {
		t.Errorf("lockTOMLString emitted a control-character escape: %q", escaped)
	}
}

// --- exact-byte emission pins (mirroring tests/test_lock.py fixtures) ---

func TestRenderLockMetaOnly(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Meta: map[string]string{"cli_version": "0.14.0", "synced_at": "2026-07-01T00:00:00Z"},
	})
	want := lockHeader +
		"\n" +
		"[meta]\n" +
		"cli_version = \"0.14.0\"\n" +
		"synced_at = \"2026-07-01T00:00:00Z\"\n"
	if out != want {
		t.Fatalf("renderLock\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
}

// TestRenderLockEmpty is the empty lock: only the header, single trailing
// newline (Python: "\n".join([LOCK_HEADER]).rstrip("\n") + "\n").
func TestRenderLockEmpty(t *testing.T) {
	out := renderLock(&lockWriteRequest{})
	if out != lockHeader {
		t.Fatalf("renderLock(empty) = %q, want the header only", out)
	}
	if !strings.HasSuffix(out, ".\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("empty lock must end with exactly one newline, got %q", out)
	}
}

// TestRenderLockManagedSortedSkipsEmpty mirrors the Python loop: entries
// sorted by path, entries without sha256 skipped, empty/missing optional
// values skipped, keys in the fixed order sha256, recipe, source, kind, policy.
func TestRenderLockManagedSortedSkipsEmpty(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Managed: map[string]lockManagedEntry{
			"zz/late.md":    {SHA256: "zzz"},
			"aa/first.md":   {SHA256: "aaa", Recipe: "worktree-flow", Source: "tpl.md", Kind: "template", Policy: "auto"},
			"mm/nosha.md":   {Recipe: "no-sha-here"},
			"bb/partial.md": {SHA256: "bbb"},
		},
	})
	want := lockHeader +
		"\n" +
		"[managed.\"aa/first.md\"]\n" +
		"sha256 = \"aaa\"\n" +
		"recipe = \"worktree-flow\"\n" +
		"source = \"tpl.md\"\n" +
		"kind = \"template\"\n" +
		"policy = \"auto\"\n" +
		"\n" +
		"[managed.\"bb/partial.md\"]\n" +
		"sha256 = \"bbb\"\n" +
		"\n" +
		"[managed.\"zz/late.md\"]\n" +
		"sha256 = \"zzz\"\n"
	if out != want {
		t.Fatalf("renderLock\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
	if strings.Contains(out, "mm/nosha.md") {
		t.Errorf("entry without sha256 must be skipped, got %q", out)
	}
}

// TestRenderLockAgentsSortedNested mirrors tests/test_lock.py:132: agents is
// nested {harness: {filename: hash}}, harnesses sorted, filenames sorted, all
// quoted per _toml_string.
func TestRenderLockAgentsSortedNested(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Agents: map[string]map[string]string{
			"zsh-agent": {"README.md": "readmehash"},
			"claude":    {"AGENTS.md": "agenthash", "CLAUDE.md": "claudehash"},
		},
	})
	want := lockHeader +
		"\n" +
		"[agents.\"claude\"]\n" +
		"\"AGENTS.md\" = \"agenthash\"\n" +
		"\"CLAUDE.md\" = \"claudehash\"\n" +
		"\n" +
		"[agents.\"zsh-agent\"]\n" +
		"\"README.md\" = \"readmehash\"\n"
	if out != want {
		t.Fatalf("renderLock\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
}

// TestRenderLockFullLock is the full-lock pin: meta → managed → agents in the
// fixed section order with blank lines between sections.
func TestRenderLockFullLock(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Meta: map[string]string{"cli_version": "0.24.0", "synced_at": "2026-07-14T00:00:00Z"},
		Managed: map[string]lockManagedEntry{
			"ai-specs/recipes/worktree-flow/hooks/gate": {SHA256: "gatehash", Recipe: "worktree-flow", Source: "hooks/gate", Kind: "gate", Policy: "auto"},
		},
		Agents: map[string]map[string]string{
			"claude": {"AGENTS.md": "agenthash"},
		},
	})
	want := lockHeader +
		"\n" +
		"[meta]\n" +
		"cli_version = \"0.24.0\"\n" +
		"synced_at = \"2026-07-14T00:00:00Z\"\n" +
		"\n" +
		"[managed.\"ai-specs/recipes/worktree-flow/hooks/gate\"]\n" +
		"sha256 = \"gatehash\"\n" +
		"recipe = \"worktree-flow\"\n" +
		"source = \"hooks/gate\"\n" +
		"kind = \"gate\"\n" +
		"policy = \"auto\"\n" +
		"\n" +
		"[agents.\"claude\"]\n" +
		"\"AGENTS.md\" = \"agenthash\"\n"
	if out != want {
		t.Fatalf("renderLock\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
}

// TestRenderLockMetaHeaderOnly pins the Python dict-truthiness edge: a meta
// object that is present but whose known keys are empty still emits the
// [meta] header (lock.py `if meta:`), with no key lines.
func TestRenderLockMetaHeaderOnly(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Meta: map[string]string{"cli_version": "", "synced_at": ""},
	})
	want := lockHeader + "\n[meta]\n"
	if out != want {
		t.Fatalf("renderLock = %q, want %q", out, want)
	}
}

// TestRenderLockEscapedSectionKeys pins quoting of section keys: managed
// paths and agent harness names go through the same _toml_string quoting as
// Python's f'[managed."{path}"]' emission.
func TestRenderLockEscapedSectionKeys(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Managed: map[string]lockManagedEntry{
			`we"ird\path`: {SHA256: "aaa"},
		},
		Agents: map[string]map[string]string{
			`my "agent"`: {`file\name.md`: `ha"sh`},
		},
	})
	want := lockHeader +
		"\n" +
		"[managed.\"we\\\"ird\\\\path\"]\n" +
		"sha256 = \"aaa\"\n" +
		"\n" +
		"[agents.\"my \\\"agent\\\"\"]\n" +
		"\"file\\\\name.md\" = \"ha\\\"sh\"\n"
	if out != want {
		t.Fatalf("renderLock\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
}

// TestRenderLockRawControlCharsInValues pins the NO-control-escape contract
// end to end: a value containing a newline is emitted with the raw newline
// byte (Python has no tomllib validation on write, parity over validity).
func TestRenderLockRawControlCharsInValues(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Meta: map[string]string{"cli_version": "0.1\n0\t0"},
	})
	if !strings.Contains(out, "cli_version = \"0.1\n0\t0\"\n") {
		t.Fatalf("raw control characters must survive byte for byte, got %q", out)
	}
	if strings.Contains(out, `\n`) || strings.Contains(out, `\t`) {
		t.Fatalf("control-character escape emitted: %q", out)
	}
}

// TestRenderLockNoLegacySections pins the legacy read-but-dropped contract
// (tests/test_lock.py:31-122): the writer never emits [skills], [recipes.*],
// [deps.*], [commands] or [opted-out].
func TestRenderLockNoLegacySections(t *testing.T) {
	out := renderLock(&lockWriteRequest{
		Meta:    map[string]string{"cli_version": "0.14.0"},
		Managed: map[string]lockManagedEntry{"a.md": {SHA256: "aaa"}},
		Agents:  map[string]map[string]string{"claude": {"AGENTS.md": "agenthash"}},
	})
	for _, legacy := range []string{"[skills.", "[recipes.", "[deps.", "[commands]", "[opted-out]"} {
		if strings.Contains(out, legacy) {
			t.Errorf("legacy section %q emitted: %q", legacy, out)
		}
	}
}

// --- CLI contract ---

// TestWriteLockCLISuccess drives the full contract: JSON envelope in,
// {"written": true} out, exit 0, exact bytes on disk, mode 0600 (mkstemp
// parity), and no temp files left in the lock directory.
func TestWriteLockCLISuccess(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".ai-specs.lock")
	envelope := `{"lock_path": "` + lockPath + `", "meta": {"cli_version": "0.24.0", "synced_at": "2026-07-14T00:00:00Z"}, "managed": {"AGENTS.md": {"sha256": "abc", "recipe": "worktree-flow", "source": "tpl.md", "kind": "template", "policy": "auto"}}, "agents": {"claude": {"AGENTS.md": "agenthash"}}}`
	code, out, stderr := runWriteLockCLI(t, envelope)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	if envelopeOut := decodeLockWriteEnvelope(t, out); !envelopeOut.Written {
		t.Fatalf("envelope = %#v, want written true", envelopeOut)
	}
	want := lockHeader +
		"\n" +
		"[meta]\n" +
		"cli_version = \"0.24.0\"\n" +
		"synced_at = \"2026-07-14T00:00:00Z\"\n" +
		"\n" +
		"[managed.\"AGENTS.md\"]\n" +
		"sha256 = \"abc\"\n" +
		"recipe = \"worktree-flow\"\n" +
		"source = \"tpl.md\"\n" +
		"kind = \"template\"\n" +
		"policy = \"auto\"\n" +
		"\n" +
		"[agents.\"claude\"]\n" +
		"\"AGENTS.md\" = \"agenthash\"\n"
	assertFileBytes(t, lockPath, want)
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("lock mode = %o, want 600 (tempfile.mkstemp parity)", got)
	}
	assertNoTempFiles(t, dir)
}

// TestWriteLockCLIStructuredErrors pins the structured-refusal contract:
// invalid envelope JSON, non-string values, and empty lock_path all exit 2
// with {"error": "<string>"} on stdout.
func TestWriteLockCLIStructuredErrors(t *testing.T) {
	cases := []struct {
		name    string
		envelop string
	}{
		{"invalid json", `not json`},
		{"missing lock_path", `{"meta": {}}`},
		{"empty lock_path", `{"lock_path": ""}`},
		{"meta number value", `{"lock_path": "/tmp/x", "meta": {"cli_version": 1}}`},
		{"managed field number value", `{"lock_path": "/tmp/x", "managed": {"a.md": {"sha256": 5}}}`},
		{"agents hash number value", `{"lock_path": "/tmp/x", "agents": {"claude": {"AGENTS.md": 7}}}`},
		{"managed not object", `{"lock_path": "/tmp/x", "managed": ["a.md"]}`},
		{"agents harness not object", `{"lock_path": "/tmp/x", "agents": {"claude": "agenthash"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, stderr := runWriteLockCLI(t, tc.envelop)
			if code != 2 {
				t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
			}
			envelope := decodeLockWriteEnvelope(t, out)
			if envelope.Error == nil || strings.TrimSpace(*envelope.Error) == "" {
				t.Fatalf("error envelope = %#v, want a non-empty error string", envelope)
			}
			if envelope.Written {
				t.Errorf("written = true on a refusal")
			}
		})
	}
}

// TestWriteLockCLIInfraFailure pins the infra-failure contract: an I/O error
// (unwritable parent directory) exits 2 with a diagnostic on stderr and NO
// {"error": ...} envelope on stdout.
func TestWriteLockCLIInfraFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	readOnly := filepath.Join(dir, "ro")
	if err := os.Mkdir(readOnly, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(readOnly, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })

	code, out, stderr := runWriteLockCLI(t, `{"lock_path": "`+filepath.Join(readOnly, ".ai-specs.lock")+`"}`)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stdout %q)", code, out)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatalf("expected a stderr diagnostic, got %q", stderr)
	}
	if out != "" {
		t.Errorf("infra failure must not emit an error envelope, got %q", out)
	}
}

// TestWriteLockAtomicReplace pins the atomicity contract: a failed write
// leaves the original bytes untouched and no temp file behind, and a
// successful write replaces the previous content entirely (always-write, no
// byte-equality no-op).
func TestWriteLockAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".ai-specs.lock")
	original := lockHeader + "\n[meta]\ncli_version = \"old\"\n"
	if err := os.WriteFile(lockPath, []byte(original), 0o600); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	t.Run("failed write preserves original", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("chmod dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		req := &lockWriteRequest{LockPath: lockPath, Meta: map[string]string{"cli_version": "new"}}
		if err := writeLockFile(req); err == nil {
			t.Fatalf("write into a read-only directory must fail")
		}
		assertFileBytes(t, lockPath, original)
	})

	t.Run("success replaces bytes entirely", func(t *testing.T) {
		req := &lockWriteRequest{
			LockPath: lockPath,
			Meta:     map[string]string{"cli_version": "new"},
		}
		if err := writeLockFile(req); err != nil {
			t.Fatalf("writeLockFile: %v", err)
		}
		want := lockHeader + "\n[meta]\ncli_version = \"new\"\n"
		assertFileBytes(t, lockPath, want)
		assertNoTempFiles(t, dir)
	})
}
