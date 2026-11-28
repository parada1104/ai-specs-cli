package receipts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBinary writes a gentle-ai stand-in the test drives through GENTLE_AI_BIN, and
// returns its path plus the directory it records its own argv in — so the argv is
// asserted instead of assumed.
func fakeBinary(t *testing.T, script string) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "gentle-ai")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$(dirname \"$0\")/argv\"\n" + script
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

// S12: the receipt is consumed from gentle-ai and relayed verbatim, for the candidate
// base SHA of the documented argv — nothing is inferred from a commit message or a file.
func TestAssessRelaysTheReceiptVerbatim(t *testing.T) {
	bin, dir := fakeBinary(t, `printf '%s\n' '{"schema":"gentle-ai.review-assessment/v1",`+
		`"risk":"high","reasons":[{"code":"size"}],"changed_paths":3,"changed_lines":40,`+
		`"candidate":{"kind":"base-diff","base_ref":"deadbee","consumed":false},`+
		`"review_due":true,"review_due_reason":"high_risk"}'
`)
	t.Setenv(BinaryEnv, bin)

	got := Assess("/repo", "deadbeef")

	if !got.Known || got.Reason != "" {
		t.Fatalf("assessment = %+v, want a consumed receipt", got)
	}
	if got.Consumed == nil || *got.Consumed || got.ReviewDue == nil || !*got.ReviewDue ||
		got.ReviewDueReason != "high_risk" || got.Risk != "high" {
		t.Fatalf("assessment = %+v, want consumed=false, review_due=true/high_risk, risk high", got)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"review", "assess", "--cwd", "/repo", "--base-ref", "deadbeef",
		"--committed-only", "--json"}, "\n") + "\n"
	if string(raw) != want {
		t.Fatalf("argv = %q, want %q", raw, want)
	}
}

// An older binary omits candidate.consumed: the receipt stays absent, never false.
func TestAssessLeavesAnAbsentConsumedUnknown(t *testing.T) {
	bin, _ := fakeBinary(t, `printf '%s\n' '{"schema":"gentle-ai.review-assessment/v1","risk":"passive",`+
		`"reasons":[],"changed_paths":0,"changed_lines":0,"candidate":{"kind":"base-diff"}}'
`)
	t.Setenv(BinaryEnv, bin)

	got := Assess("/repo", "deadbeef")

	if got.Known || got.Consumed != nil {
		t.Fatalf("assessment = %+v, want unknown with no consumed value", got)
	}
	if !strings.Contains(got.Reason, "candidate.consumed") {
		t.Fatalf("reason = %q, want it naming the absent field", got.Reason)
	}
}

// Every unusable answer is unknown with its reason, and never a fabricated receipt.
func TestAssessReportsUnknownWithItsReason(t *testing.T) {
	cases := []struct {
		name   string
		script string
		env    string
		reason string
	}{
		{
			name:   "non-zero exit",
			script: "printf '%s\\n' 'no such candidate' >&2\nexit 4\n",
			reason: "refused the candidate",
		},
		{
			name:   "undecodable envelope",
			script: "printf '%s\\n' 'not json'\n",
			reason: "cannot decode",
		},
		{
			name:   "another schema",
			script: "printf '%s\\n' '{\"schema\":\"other/v2\",\"risk\":\"low\",\"candidate\":{\"consumed\":true}}'\n",
			reason: "other/v2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, _ := fakeBinary(t, tc.script)
			t.Setenv(BinaryEnv, bin)

			got := Assess("/repo", "deadbeef")

			if got.Known {
				t.Fatalf("assessment = %+v, want unknown", got)
			}
			if !strings.Contains(got.Reason, tc.reason) {
				t.Fatalf("reason = %q, want it containing %q", got.Reason, tc.reason)
			}
		})
	}

	t.Run("missing binary", func(t *testing.T) {
		t.Setenv(BinaryEnv, filepath.Join(t.TempDir(), "absent"))
		got := Assess("/repo", "deadbeef")
		if got.Known || !strings.Contains(got.Reason, "not installed") {
			t.Fatalf("assessment = %+v, want unknown naming the missing binary", got)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		bin, _ := fakeBinary(t, "exec sleep 5\n")
		t.Setenv(BinaryEnv, bin)
		restore := Timeout
		Timeout = 100 * time.Millisecond
		t.Cleanup(func() { Timeout = restore })

		got := Assess("/repo", "deadbeef")

		if got.Known || !strings.Contains(got.Reason, "did not answer") {
			t.Fatalf("assessment = %+v, want unknown naming the timeout", got)
		}
	})

	t.Run("no candidate SHA", func(t *testing.T) {
		bin, dir := fakeBinary(t, "exit 0\n")
		t.Setenv(BinaryEnv, bin)

		got := Assess("/repo", "")

		if got.Known || !strings.Contains(got.Reason, "no base ref") {
			t.Fatalf("assessment = %+v, want unknown naming the missing base", got)
		}
		if _, err := os.Stat(filepath.Join(dir, "argv")); !os.IsNotExist(err) {
			t.Fatalf("the binary ran with no candidate SHA (stat err = %v)", err)
		}
	})
}
