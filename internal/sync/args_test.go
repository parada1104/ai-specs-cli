package sync

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFlagsHelpPrintsUsageAndStops(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var out, errOut bytes.Buffer
		_, code, done := parseFlags([]string{flag}, &out, &errOut)
		if !done || code != 0 {
			t.Errorf("%s: done=%v code=%d, want done=true code=0", flag, done, code)
		}
		if !strings.HasPrefix(out.String(), "Usage: ai-specs sync [path]") {
			t.Errorf("%s: stdout = %q, want usage prefix", flag, out.String())
		}
		if errOut.Len() != 0 {
			t.Errorf("%s: stderr = %q, want empty", flag, errOut.String())
		}
	}
}

func TestParseFlagsUnknownFlagIsExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	_, code, done := parseFlags([]string{"--verbos"}, &out, &errOut)
	if !done || code != 2 {
		t.Errorf("done=%v code=%d, want done=true code=2", done, code)
	}
	want := "ERROR: unknown flag: --verbos\nRun 'ai-specs sync --help' for usage.\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestParseFlagsSecondPositionalIsExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, code, done := parseFlags([]string{"a", "b"}, &out, &errOut)
	if !done || code != 2 {
		t.Errorf("done=%v code=%d, want done=true code=2", done, code)
	}
	if opts.target != "a" {
		t.Errorf("target = %q, want %q", opts.target, "a")
	}
	want := "ERROR: unexpected positional argument: b\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestParseFlagsDoubleDashStopsParsing(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, _, done := parseFlags([]string{"proj", "--", "--verbose", "junk"}, &out, &errOut)
	if done {
		t.Errorf("done = true, want false")
	}
	if opts.target != "proj" {
		t.Errorf("target = %q, want %q", opts.target, "proj")
	}
	if opts.verbose {
		t.Errorf("verbose = true, want false")
	}
}

func TestParseFlagsAcceptsEveryExistingFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, _, done := parseFlags([]string{"--ignore-cli-version", "--refresh-gates", "--adopt-brief", "--verbose", "proj"}, &out, &errOut)
	if done {
		t.Errorf("done = true, want false")
	}
	if !opts.ignoreCliVersion || !opts.refreshGates || !opts.adoptBrief || !opts.verbose {
		t.Errorf("flags not all set: %+v", opts)
	}
	if opts.target != "proj" {
		t.Errorf("target = %q, want %q", opts.target, "proj")
	}
}

func TestStepModeDefaultsToPython(t *testing.T) {
	if got := stepMode("gitignore"); got != "python" {
		t.Errorf("unset: stepMode = %q, want %q", got, "python")
	}
	t.Setenv("GO_SYNC_STEP_GITIGNORE", "python")
	if got := stepMode("gitignore"); got != "python" {
		t.Errorf("=python: stepMode = %q, want %q", got, "python")
	}
	t.Setenv("GO_SYNC_STEP_GITIGNORE", "go")
	if got := stepMode("gitignore"); got != "go" {
		t.Errorf("=go: stepMode = %q, want %q", got, "go")
	}
}
