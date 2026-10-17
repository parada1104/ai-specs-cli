package config

// GO-20 U3b-2 (T4c) data-safety fixes for SetEnabledAgents: invalid UTF-8,
// symlinked and read-only manifests, and the valid→invalid TOML guard the
// writer shares with the other surgical manifest ops. Each test pins one
// defect an independent verifier found in the manifest writer.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestSetEnabledAgentsKeepsInvalidUTF8Bytes pins FIX 1: a manifest whose bytes
// are not valid UTF-8 must be refused, byte-identically. The line editor splits
// through []rune, which would silently replace every invalid byte with U+FFFD
// and persist the corruption.
func TestSetEnabledAgentsKeepsInvalidUTF8Bytes(t *testing.T) {
	body := "[project]\nname = \"t\"\n# caf\xe9\n"
	path := manifestAt(t, body)

	if err := SetEnabledAgents(path, []string{"pi"}); err == nil {
		t.Fatal("SetEnabledAgents accepted an invalid-UTF-8 manifest, want an error")
	}
	if got := readBack(t, path); got != body {
		t.Errorf("refused write changed the manifest:\n got %q\nwant %q", got, body)
	}
}

// TestSetEnabledAgentsWritesThroughSymlink pins FIX 2a: a symlinked manifest is
// written at its target so the link survives, instead of being replaced by a
// regular file (atomicWrite renames a temp file over the path).
func TestSetEnabledAgentsWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.toml")
	link := filepath.Join(dir, "ai-specs.toml")
	if err := os.WriteFile(target, []byte("[project]\nname = \"t\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}

	if err := SetEnabledAgents(link, []string{"pi"}); err != nil {
		t.Fatalf("SetEnabledAgents through symlink: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: mode %v, want a symlink", fi.Mode())
	}
	want := "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"pi\"]\n"
	if got := readBack(t, target); got != want {
		t.Errorf("target = %q\nwant   = %q", got, want)
	}
}

// TestSetEnabledAgentsRefusesReadOnlyManifest pins FIX 2b: a manifest without
// its owner-write bit is refused with a permission error and left byte-identical.
// Skipped as root, where the mode bit does not stop a write.
func TestSetEnabledAgentsRefusesReadOnlyManifest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the file mode does not prevent writes")
	}
	body := "[project]\nname = \"t\"\n"
	path := manifestAt(t, body)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}

	err := SetEnabledAgents(path, []string{"pi"})
	if err == nil {
		t.Fatal("SetEnabledAgents overwrote a read-only manifest, want a permission error")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("error = %v, want a permission error", err)
	}
	if got := readBack(t, path); got != body {
		t.Errorf("read-only manifest changed:\n got %q\nwant %q", got, body)
	}
}

// TestSetEnabledAgentsNeverWritesInvalidTOML pins FIX 3 and FIX 3c: the writer
// runs the shared valid→invalid guard before it touches the file, and it
// matches the `enabled` KEY exactly. The invariant, for every body, is
// "never regress a valid manifest into an invalid one": a refused write leaves
// the file byte-identical, an accepted write leaves a valid manifest (an
// already-invalid manifest may still be edited — frozen parity contract), and
// no body ever gains a second [agents] block.
func TestSetEnabledAgentsNeverWritesInvalidTOML(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		// substrings that must survive in a successful write; the header-name
		// and exact-key fixes are proven by these, not by the parse alone.
		wantInFile []string
	}{
		{
			name:    "multi-line array would orphan the tail",
			body:    "[agents]\nenabled = [\n  \"claude\",\n]\n",
			wantErr: true,
		},
		{
			name:       "header with inline comment is still the agents table",
			body:       "[agents] # note\nfoo = 1\n",
			wantInFile: []string{"[agents] # note", "foo = 1", "enabled = [\"pi\"]"},
		},
		{
			name:       "enabled_extra is a different key",
			body:       "[agents]\nenabled_extra = [\"x\"]\n",
			wantInFile: []string{"enabled_extra = [\"x\"]", "enabled = [\"pi\"]"},
		},
		{
			name:       "already-invalid manifest may still be edited",
			body:       "\x1c[agents]\n",
			wantInFile: []string{"enabled = [\"pi\"]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, origErr := toml.Parse([]byte(tc.body))
			path := manifestAt(t, tc.body)

			err := SetEnabledAgents(path, []string{"pi"})
			got := readBack(t, path)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("write accepted, want a refusal; file = %q", got)
				}
				if got != tc.body {
					t.Errorf("refused write changed the file:\n got %q\nwant %q", got, tc.body)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetEnabledAgents = %v, want success", err)
			}
			if origErr == nil {
				if _, perr := toml.Parse([]byte(got)); perr != nil {
					t.Errorf("valid input became invalid TOML: %v\n%s", perr, got)
				}
			}
			for _, needle := range tc.wantInFile {
				if !strings.Contains(got, needle) {
					t.Errorf("result missing %q:\n%s", needle, got)
				}
			}
			if n := strings.Count(got, "[agents]"); n != 1 {
				t.Errorf("[agents] appears %d times, want 1:\n%s", n, got)
			}
		})
	}
}
