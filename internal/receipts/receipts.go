// Package receipts consumes the review receipt gentle-ai records for a candidate
// (change-ledger-v2 T7.3, S12).
//
// A receipt is CONSUMED, never fabricated: this package runs the documented,
// read-only `gentle-ai review assess` verb and relays what it printed. Nothing here
// reads a commit message, a file on disk or a private format, and nothing is
// defaulted — an absent field, a missing binary, a non-zero exit, a timeout or an
// undecodable envelope is `unknown` with its reason, never a guess dressed as a fact.
package receipts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// Schema is the only envelope this build decodes.
	Schema = "gentle-ai.review-assessment/v1"

	// BinaryEnv names the gentle-ai executable. Unset falls back to PATH.
	BinaryEnv = "GENTLE_AI_BIN"
)

// Timeout bounds the child invocation. It is a variable so a test can shorten it
// instead of waiting the default out.
var Timeout = 20 * time.Second

// Assessment is one candidate's receipt. Known reports whether gentle-ai answered the
// question at all; every other field is what it printed, verbatim, and Reason explains
// an unknown answer so the report can say why instead of showing a false.
type Assessment struct {
	Known bool
	// Consumed is candidate.consumed: present only when native reported it. An older
	// binary omits it, and an absent value stays absent rather than becoming false.
	Consumed *bool
	// ReviewDue and ReviewDueReason are review_due and review_due_reason, present
	// together or absent together on an older binary.
	ReviewDue       *bool
	ReviewDueReason string
	// Risk is the native risk tier, relayed as printed.
	Risk string
	// Reason is why this receipt is unknown; "" when Known.
	Reason string
}

// Argv is the documented read-only verb: one candidate, committed work only, JSON.
func Argv(root, baseSHA string) []string {
	return []string{"review", "assess", "--cwd", root, "--base-ref", baseSHA, "--committed-only", "--json"}
}

// binaryName resolves the executable to run: GENTLE_AI_BIN when set, else PATH.
func binaryName() string {
	if name := strings.TrimSpace(os.Getenv(BinaryEnv)); name != "" {
		return name
	}
	return "gentle-ai"
}

// Assess asks gentle-ai for the receipt of the candidate baseSHA resolves to. It
// never fails: every failure is an unknown receipt with its reason, because a missing
// receipt is a fact about the environment and not an error of the caller.
func Assess(root, baseSHA string) Assessment {
	if strings.TrimSpace(baseSHA) == "" {
		return Assessment{Reason: "no base ref resolves in this root, so there is no candidate SHA to ask about"}
	}
	name := binaryName()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, Argv(root, baseSHA)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Assessment{Reason: fmt.Sprintf("`%s review assess` did not answer within %s", name, Timeout)}
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
			return Assessment{Reason: fmt.Sprintf("the gentle-ai binary %q is not installed; set %s or put "+
				"gentle-ai on PATH", name, BinaryEnv)}
		}
		if line := firstLine(stderr.String()); line != "" {
			return Assessment{Reason: fmt.Sprintf("`%s review assess` refused the candidate (%v): %s", name, err, line)}
		}
		return Assessment{Reason: fmt.Sprintf("`%s review assess` refused the candidate (%v)", name, err)}
	}
	return decode(stdout.Bytes())
}

// decode reads the one envelope this build understands. A field native did not print
// stays absent: only an absent candidate.consumed is an unknown receipt, because
// without it nobody can say whether the review was consumed.
func decode(data []byte) Assessment {
	var envelope struct {
		Schema string `json:"schema"`
		Risk   string `json:"risk"`
		// Consumed is a pointer so an absent value is distinguishable from false.
		Candidate struct {
			Consumed *bool `json:"consumed"`
		} `json:"candidate"`
		ReviewDue       *bool   `json:"review_due"`
		ReviewDueReason *string `json:"review_due_reason"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Assessment{Reason: "`review assess` printed an envelope this build cannot decode: " + err.Error()}
	}
	if envelope.Schema != Schema {
		return Assessment{Reason: fmt.Sprintf("`review assess` printed schema %q, want %q", envelope.Schema, Schema)}
	}
	if envelope.Candidate.Consumed == nil {
		return Assessment{Reason: "the receipt carries no candidate.consumed (an older gentle-ai build), so " +
			"nothing says whether the review of this candidate was consumed"}
	}
	assessment := Assessment{
		Known:     true,
		Consumed:  envelope.Candidate.Consumed,
		ReviewDue: envelope.ReviewDue,
		Risk:      envelope.Risk,
	}
	if envelope.ReviewDueReason != nil {
		assessment.ReviewDueReason = *envelope.ReviewDueReason
	}
	return assessment
}

// firstLine is the child's first stderr line: enough to name the refusal, never a
// transcript.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
