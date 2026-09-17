package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func posixTokens(t *testing.T, cmd string) []string {
	t.Helper()
	tokens, ok := splitPOSIX(cmd)
	if !ok {
		t.Fatalf("splitPOSIX(%q) failed", cmd)
	}
	return tokens
}

func TestSplitSegmentsWithSepPreservesOperators(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want []segment
	}{
		{
			name: "cd then pipeline keeps && vs | distinct",
			cmd:  "cd A && foo | tee f",
			want: []segment{
				{tokens: []string{"cd", "A"}, sep: ""},
				{tokens: []string{"foo"}, sep: "&&"},
				{tokens: []string{"tee", "f"}, sep: "|"},
			},
		},
		{
			name: "pipeline-local cd is preceded by | not &&",
			cmd:  "foo | cd A | tee f",
			want: []segment{
				{tokens: []string{"foo"}, sep: ""},
				{tokens: []string{"cd", "A"}, sep: "|"},
				{tokens: []string{"tee", "f"}, sep: "|"},
			},
		},
		{
			name: "or-separator",
			cmd:  "false || echo x > f",
			want: []segment{
				{tokens: []string{"false"}, sep: ""},
				{tokens: []string{"echo", "x", ">", "f"}, sep: "||"},
			},
		},
		{
			name: "semicolon sequential",
			cmd:  "cd A ; echo x > f",
			want: []segment{
				{tokens: []string{"cd", "A"}, sep: ""},
				{tokens: []string{"echo", "x", ">", "f"}, sep: ";"},
			},
		},
		{
			name: "empty segment between duplicated &&",
			cmd:  "echo a && && echo b",
			want: []segment{
				{tokens: []string{"echo", "a"}, sep: ""},
				{tokens: nil, sep: "&&"},
				{tokens: []string{"echo", "b"}, sep: "&&"},
			},
		},
		{
			name: "single segment no separator",
			cmd:  "echo x > rel",
			want: []segment{
				{tokens: []string{"echo", "x", ">", "rel"}, sep: ""},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSegmentsWithSep(posixTokens(t, tc.cmd))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitSegmentsWithSep(%q)\n got %#v\nwant %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestSplitSegmentsWithSepEmptyInput(t *testing.T) {
	got := splitSegmentsWithSep(nil)
	if len(got) != 0 {
		t.Fatalf("empty tokens: got %#v, want empty", got)
	}
	got = splitSegmentsWithSep([]string{})
	if len(got) != 0 {
		t.Fatalf("zero-length tokens: got %#v, want empty", got)
	}
}

func TestStaticDirOperandClosedList(t *testing.T) {
	cases := []struct {
		op   string
		want bool
	}{
		{"/tmp", true},
		{"rel", true},
		{"/tmp/My WT", true},
		{"$WT", false},
		{"pre$fix", false},
		{"`pwd`", false},
		{"$(pwd)", false},
		{"~/src", false},
		{"-", false},
		{"-1", false},
		{"-2", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := staticDirOperand(tc.op); got != tc.want {
			t.Fatalf("staticDirOperand(%q) = %v, want %v", tc.op, got, tc.want)
		}
	}
}

func TestResolveDirExistingAndRelativeBase(t *testing.T) {
	abs := t.TempDir()
	relName := "child"
	child := filepath.Join(abs, relName)
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(abs, "no-such-dir")

	if got, ok := resolveDir(abs, ""); !ok || got != abs {
		t.Fatalf("absolute existing: got %q ok=%v", got, ok)
	}
	if _, ok := resolveDir(missing, ""); ok {
		t.Fatal("non-existing absolute must fail")
	}
	if _, ok := resolveDir(relName, ""); ok {
		t.Fatal("relative operand without base must fail")
	}
	if got, ok := resolveDir(relName, abs); !ok || got != child {
		t.Fatalf("relative with base: got %q ok=%v, want %q", got, ok, child)
	}
	quoted := filepath.Join(t.TempDir(), "My WT")
	if err := os.Mkdir(quoted, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolveDir(quoted, ""); !ok || got != quoted {
		t.Fatalf("quoted-space dir as one operand: got %q ok=%v", got, ok)
	}
}

func TestRecoverCwdWalkSemantics(t *testing.T) {
	event := t.TempDir()
	dirA := filepath.Join(t.TempDir(), "A")
	dirB := filepath.Join(t.TempDir(), "B")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	overlayOf := func(cmd string) []segmentCwd {
		t.Helper()
		segs := splitSegmentsWithSep(posixTokens(t, cmd))
		overlays, _, _ := recoverCwdWalk(segs, event, true)
		return overlays
	}
	last := func(overlays []segmentCwd) segmentCwd {
		t.Helper()
		if len(overlays) == 0 {
			t.Fatal("no overlays")
		}
		return overlays[len(overlays)-1]
	}

	t.Run("cd && then pipeline uses sequential S", func(t *testing.T) {
		cmd := "cd " + dirA + " && foo | tee f"
		ov := last(overlayOf(cmd))
		if ov.Base != dirA || ov.Source != cwdSourceCommand {
			t.Fatalf("tee overlay = %+v, want base=%s source=command", ov, dirA)
		}
	})

	t.Run("pipeline siblings inherit pre-pipeline S", func(t *testing.T) {
		cmd := "foo | cd " + dirA + " | tee f"
		ov := last(overlayOf(cmd))
		if ov.Base != event || ov.Source != cwdSourceEvent {
			t.Fatalf("pipeline tee overlay = %+v, want event cwd", ov)
		}
	})

	t.Run("git -C overlay does not change sequential S", func(t *testing.T) {
		cmd := "git -C " + dirA + " && echo x > rel"
		ov := overlayOf(cmd)
		if ov[0].Base != dirA || ov[0].Source != cwdSourceCommand {
			t.Fatalf("git segment = %+v, want A/command", ov[0])
		}
		if ov[1].Base != event || ov[1].Source != cwdSourceEvent {
			t.Fatalf("echo after git -C && = %+v, want event cwd not A", ov[1])
		}
	})

	t.Run("multiple -C chain", func(t *testing.T) {
		cmd := "git -C " + dirA + " -C " + dirB + " mv rel dest"
		ov := last(overlayOf(cmd))
		if ov.Base != dirB || ov.Source != cwdSourceCommand {
			t.Fatalf("chained -C overlay = %+v, want B", ov)
		}
	})

	t.Run("attached -Cpath", func(t *testing.T) {
		cmd := "git -C" + dirA + " mv a b"
		ov := last(overlayOf(cmd))
		if ov.Base != dirA || ov.Source != cwdSourceCommand {
			t.Fatalf("attached -C overlay = %+v, want A", ov)
		}
	})

	t.Run("nested cd last wins", func(t *testing.T) {
		cmd := "cd " + dirA + " && cd " + dirB + " && echo x > rel"
		ov := last(overlayOf(cmd))
		if ov.Base != dirB || ov.Source != cwdSourceCommand {
			t.Fatalf("nested cd overlay = %+v, want B", ov)
		}
	})

	t.Run("cd then git -C relative dest against git overlay", func(t *testing.T) {
		cmd := "cd " + dirA + " && git -C " + dirB + " mv a dest"
		ov := last(overlayOf(cmd))
		if ov.Base != dirB || ov.Source != cwdSourceCommand {
			t.Fatalf("cd+git overlay = %+v, want B", ov)
		}
	})

	t.Run("relative git -C resolves against sequential S", func(t *testing.T) {
		relB := "nested"
		if err := os.Mkdir(filepath.Join(dirA, relB), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := "cd " + dirA + " && git -C " + relB + " mv a dest"
		ov := last(overlayOf(cmd))
		want := filepath.Join(dirA, relB)
		if ov.Base != want || ov.Source != cwdSourceCommand {
			t.Fatalf("relative -C overlay = %+v, want %s", ov, want)
		}
	})

	t.Run("unrecoverable cd - poisons later S not prior", func(t *testing.T) {
		cmd := "cd " + dirA + " && echo x > a && cd - && echo y > b"
		ov := overlayOf(cmd)
		if ov[1].Base != dirA || ov[1].Source != cwdSourceCommand {
			t.Fatalf("prior echo = %+v, want A (history preserved)", ov[1])
		}
		if ov[3].Source != cwdSourceNone {
			t.Fatalf("later echo after cd - = %+v, want none", ov[3])
		}
	})

	t.Run("unrecoverable expansion", func(t *testing.T) {
		for _, cmd := range []string{
			`cd "$WT" && echo x > rel`,
			"cd $(pwd) && echo x > rel",
		} {
			ov := last(overlayOf(cmd))
			if ov.Source != cwdSourceNone {
				t.Fatalf("%s overlay = %+v, want none", cmd, ov)
			}
		}
	})

	t.Run("subshell cd does not update S", func(t *testing.T) {
		cmd := "( cd " + dirA + " ) && echo x > rel"
		ov := last(overlayOf(cmd))
		if ov.Source == cwdSourceCommand && ov.Base == dirA {
			t.Fatalf("subshell must not recover A: %+v", ov)
		}
	})

	t.Run("non-existing operand poisons later S", func(t *testing.T) {
		missing := filepath.Join(event, "no-such-cd-target")
		cmd := "cd " + missing + " && echo x > rel"
		ov := last(overlayOf(cmd))
		if ov.Source != cwdSourceNone {
			t.Fatalf("missing cd operand = %+v, want none", ov)
		}
	})
}

func TestRecoverCwdWalkQuotedSpaceGitC(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My WT")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := `git -C "` + dir + `" mv a b`
	tokens := posixTokens(t, cmd)
	if len(tokens) < 3 || tokens[2] != dir {
		t.Fatalf("quoted space must be one operand, tokens=%v", tokens)
	}
	overlays, _, _ := recoverCwdWalk(splitSegmentsWithSep(tokens), t.TempDir(), true)
	if overlays[0].Base != dir || overlays[0].Source != cwdSourceCommand {
		t.Fatalf("quoted-space git -C = %+v, want %s/command", overlays[0], dir)
	}
}

func TestParseCdNoOperandOrExtraIsUnrecoverable(t *testing.T) {
	event := t.TempDir()
	dirA := t.TempDir()
	cases := []string{
		"cd && echo x > rel",
		"cd " + dirA + " extra && echo x > rel",
	}
	for _, cmd := range cases {
		ov, _, src := recoverCwdWalk(splitSegmentsWithSep(posixTokens(t, cmd)), event, true)
		last := ov[len(ov)-1]
		if last.Source != cwdSourceNone || src != cwdSourceNone {
			t.Fatalf("%q last=%+v seq=%s, want none", cmd, last, src)
		}
	}
}
