package sync

import (
	"bytes"
	"testing"
)

// TestParseFlagsDashDashParity pins the `--` separator branch to the frozen
// legacy oracle. lib/sync.sh:57 handles `--` with `shift; break`: parsing
// stops and every following argument is discarded, so `ai-specs sync -- <path>`
// leaves TARGET_PATH empty and lib/sync.sh:75 falls back to pwd. The Go port
// must reproduce that exact behavior (frozen parity contract), including the
// case where a positional target was already assigned before `--`.
func TestParseFlagsDashDashParity(t *testing.T) {
	t.Run("args after -- are discarded and target falls back to cwd", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		opts, code, done := parseFlags([]string{"--", "/tmp/definitely-not-a-real-path"}, &stdout, &stderr)
		if code != 0 || done {
			t.Fatalf("parseFlags(--, path) = code %d, done %v; want 0, false", code, done)
		}
		if opts.target != "" {
			t.Fatalf("opts.target = %q; want empty (lib/sync.sh:57-75 discards post-`--` args and falls back to pwd)", opts.target)
		}
	})
	t.Run("positional assigned before -- is kept", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		opts, code, done := parseFlags([]string{"/assigned/before", "--", "/discarded/after"}, &stdout, &stderr)
		if code != 0 || done {
			t.Fatalf("parseFlags(path, --, path) = code %d, done %v; want 0, false", code, done)
		}
		if opts.target != "/assigned/before" {
			t.Fatalf("opts.target = %q; want %q (positional before `--` wins, post-`--` args discarded)", opts.target, "/assigned/before")
		}
	})
}
