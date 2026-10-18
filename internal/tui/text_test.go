package tui

// The Text prompt's contract (odd/tasks/go-20-u4-init.md S13): the wizard's one
// new primitive, a questionary.text port. What is pinned here is the behaviour
// the wizard's name step branches on — draw before read, trim + default on an
// empty answer, abort to ("", false) — and the TextSource capability the
// KeyKind-only KeySource seam cannot carry (a rune needs its own stream).

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// textTerminal builds a Terminal over a pipe whose script is already fully
// written AND closed, so the decoder sees a real end-of-stream after the
// script — the closed-pty shape a wizard prompt has to abort on.
func textTerminal(t *testing.T, script string) *Terminal {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(script); err != nil {
		r.Close()
		w.Close()
		t.Fatal(err)
	}
	w.Close() // EOF after the script
	term, err := newTerminal(r, func(*os.File) (func(), error) {
		return func() {}, nil
	})
	if err != nil {
		r.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); term.Close() })
	return term
}

// scriptedText is a KeySource AND a TextSource: the list prompts consume the
// key script, the text prompt consumes the text script, and an exhausted
// script answers io.EOF like a closed terminal.
type scriptedText struct {
	keys    []Key
	keyAt   int
	text    []TextKey
	textAt  int
	onFirst func() // observed before the first text event is handed out
}

func (s *scriptedText) NextKey() (Key, error) {
	if s.keyAt >= len(s.keys) {
		return 0, io.EOF
	}
	k := s.keys[s.keyAt]
	s.keyAt++
	return k, nil
}

func (s *scriptedText) NextTextKey() (TextKey, error) {
	if s.textAt == 0 && s.onFirst != nil {
		s.onFirst()
	}
	if s.textAt >= len(s.text) {
		return TextKey{}, io.EOF
	}
	e := s.text[s.textAt]
	s.textAt++
	return e, nil
}

func textRunes(s string) []TextKey {
	var out []TextKey
	for _, r := range s {
		out = append(out, TextKey{Kind: TextRune, Rune: r})
	}
	return out
}

// TestTextDrawsTheQuestionBeforeItReads pins the order the Confirm prompt
// already pins: the question has to be on screen while the prompt waits, or a
// PTY stage that waits for the question text before typing never fires.
func TestTextDrawsTheQuestionBeforeItReads(t *testing.T) {
	var out bytes.Buffer
	var onScreen string
	in := &scriptedText{
		text:    append(clearLine(), append(textRunes("x"), TextKey{Kind: TextEnter})...),
		onFirst: func() { onScreen = out.String() },
	}
	got, ok := Text(&out, in, "Project name:", "demo")
	if !ok || got != "x" {
		t.Fatalf("Text = (%q, %v), want (\"x\", true)", got, ok)
	}
	if !strings.Contains(onScreen, "? Project name: demo") {
		t.Errorf("the question (with its default) was not drawn before the first key: %q", onScreen)
	}
}

// clearLine is the prompt_toolkit clearing gesture tests/test_init_tui.py
// feeds over a pty (Ctrl-A Ctrl-K): the default text is PRE-FILLED, so a
// custom value starts by emptying the line.
func clearLine() []TextKey {
	return []TextKey{{Kind: TextLineStart}, {Kind: TextKillToEnd}}
}

// TestTextEmptyAnswerKeepsTheDefault pins S14's name rule end to end: the
// answer is strip()ed and an empty result keeps the default — exactly what
// init_tui.py's `project_name.strip() or default_name` does.
func TestTextEmptyAnswerKeepsTheDefault(t *testing.T) {
	typed := func(s string) []TextKey {
		return append(append(clearLine(), textRunes(s)...), TextKey{Kind: TextEnter})
	}
	cases := map[string]struct {
		input []TextKey
		want  string
	}{
		"immediate enter keeps the default": {[]TextKey{{Kind: TextEnter}}, "demo"},
		"whitespace-only keeps the default": {typed("   \t "), "demo"},
		"typed value is stripped":           {typed("  my-app  "), "my-app"},
		"typing appends to the prefill": {append(textRunes("-x"), TextKey{Kind: TextEnter}),
			"demo-x"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok := Text(&out, &scriptedText{text: tc.input}, "Project name:", "demo")
			if !ok || got != tc.want {
				t.Errorf("Text = (%q, %v), want (%q, true)", got, ok, tc.want)
			}
		})
	}
}

// TestTextBackspaceAndCursorEditTheValue pins the editing keys the raw-mode
// decoder delivers, so a typo is fixable before Enter and Ctrl-A/Ctrl-K can
// clear the prefill (which is how tests/test_init_tui.py types a custom name).
func TestTextBackspaceAndCursorEditTheValue(t *testing.T) {
	var out bytes.Buffer
	in := clearLine()
	in = append(in, textRunes("abc")...)
	in = append(in, TextKey{Kind: TextBackspace})
	in = append(in, textRunes("d")...)
	in = append(in, TextKey{Kind: TextEnter})
	if got, ok := Text(&out, in2(in), "Project name:", "demo"); !ok || got != "abd" {
		t.Errorf("Text after backspace = (%q, %v), want (\"abd\", true)", got, ok)
	}

	// Ctrl-A then Ctrl-K clears the default; the typed value survives whole.
	out.Reset()
	in = append(clearLine(), textRunes("typed")...)
	in = append(in, TextKey{Kind: TextLineStart}, TextKey{Kind: TextKillToEnd})
	in = append(in, textRunes("brand")...)
	in = append(in, TextKey{Kind: TextEnter})
	if got, ok := Text(&out, in2(in), "Project name:", "demo"); !ok || got != "brand" {
		t.Errorf("Text after clear-then-type = (%q, %v), want (\"brand\", true)", got, ok)
	}

	// Ctrl-U kills back to the line start: what was typed before the cursor is
	// gone, and the emptied line falls back to the default.
	out.Reset()
	in = append(clearLine(), textRunes("keepme-")...)
	in = append(in, TextKey{Kind: TextKillToStart})
	in = append(in, TextKey{Kind: TextEnter})
	if got, ok := Text(&out, in2(in), "Project name:", "demo"); !ok || got != "demo" {
		t.Errorf("Text after Ctrl-U = (%q, %v), want the default on the emptied line", got, ok)
	}
}

// in2 wraps a text script in the shared seam.
func in2(text []TextKey) *scriptedText { return &scriptedText{text: text} }

// TestTextAbortsReturnNothing pins the cancel semantics the wizard's S15
// branches on: ESC, Ctrl-C and a closed stream all end as ("", false) with the
// drawn line terminated, never a hang and never a silent default.
func TestTextAbortsReturnNothing(t *testing.T) {
	cases := map[string][]TextKey{
		"escape": {{Kind: TextAbort}},
		"ctrl-c": {{Kind: TextAbort}}, // the decoder maps 0x03 to TextAbort
		"eof":    {},                  // no events: the source answers io.EOF
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok := Text(&out, in2(script), "Project name:", "demo")
			if ok || got != "" {
				t.Errorf("Text(%s) = (%q, %v), want (\"\", false)", name, got, ok)
			}
			if !strings.HasSuffix(out.String(), "\n") {
				t.Errorf("the aborted line was not terminated: %q", out.String())
			}
		})
	}
}

// TestTextCtrlDOnAnEmptyLineAborts pins the prompt_toolkit meaning the PTY
// suite drives: Ctrl-D on an empty buffer is EOF (cancel); on a line with
// content it deletes forward instead of throwing the answer away.
func TestTextCtrlDOnAnEmptyLineAborts(t *testing.T) {
	var out bytes.Buffer
	in := clearLine() // empties the pre-filled default
	in = append(in, TextKey{Kind: TextCtrlD})
	got, ok := Text(&out, in2(in), "Project name:", "demo")
	if ok || got != "" {
		t.Errorf("Text(ctrl-D on empty) = (%q, %v), want (\"\", false)", got, ok)
	}

	out.Reset()
	in = append(clearLine(), textRunes("abc")...)
	in = append(in, TextKey{Kind: TextLineStart}, TextKey{Kind: TextCursorRight}) // cursor after "a"
	in = append(in, TextKey{Kind: TextCtrlD})                                     // deletes 'b'
	in = append(in, TextKey{Kind: TextEnter})
	if got, ok := Text(&out, in2(in), "Project name:", "demo"); !ok || got != "ac" {
		t.Errorf("Text(ctrl-D mid-line) = (%q, %v), want (\"ac\", true)", got, ok)
	}
}

// TestTextNeedsARuneCapableSource documents the seam: KeySource carries key
// KINDS, a text prompt needs the character itself, so a source without the
// TextSource capability aborts instead of accepting an invisible answer.
func TestTextNeedsARuneCapableSource(t *testing.T) {
	var out bytes.Buffer
	got, ok := Text(&out, keys(KeyEnter), "Project name:", "demo")
	if ok || got != "" {
		t.Errorf("Text over a key-only source = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestTerminalTextKeyDecoding pins the production decoder: printable bytes
// (UTF-8 included) arrive as runes, the control bytes map to their editing
// keys, and an arrow sequence moves the cursor instead of aborting.
func TestTerminalTextKeyDecoding(t *testing.T) {
	term := textTerminal(t, "a\xc3\xa9 \x7f\x01\x0b\x15\r\x1b[D\x1b[C\x03\x04")
	want := []TextKey{
		{Kind: TextRune, Rune: 'a'},
		{Kind: TextRune, Rune: 'é'},
		{Kind: TextRune, Rune: ' '},
		{Kind: TextBackspace},
		{Kind: TextLineStart},
		{Kind: TextKillToEnd},
		{Kind: TextKillToStart},
		{Kind: TextEnter},
		{Kind: TextCursorLeft},
		{Kind: TextCursorRight},
		{Kind: TextAbort},
	}
	for i, w := range want {
		got, err := term.NextTextKey()
		if err != nil {
			t.Fatalf("event %d: NextTextKey returned error %v", i, err)
		}
		if got != w {
			t.Errorf("event %d: NextTextKey = %+v, want %+v", i, got, w)
		}
	}
	// Ctrl-D and a closed stream stay distinguishable for the prompt.
	if k, err := term.NextTextKey(); err != nil || k.Kind != TextCtrlD {
		t.Errorf("NextTextKey(ctrl-D) = (%+v, %v), want (TextCtrlD, nil)", k, err)
	}
	if _, err := term.NextTextKey(); err != io.EOF {
		t.Errorf("NextTextKey at end of stream error = %v, want io.EOF", err)
	}
}

// TestTerminalTextKeyEOFAndEscapeSequence pins EOF propagation and the ESC
// family: a bare ESC (or one not starting a CSI) is the package's abort, while
// an unknown CSI sequence is ignored rather than eating the answer.
func TestTerminalTextKeyEOFAndEscapeSequence(t *testing.T) {
	term := textTerminal(t, "\x1b[An")
	if k, err := term.NextTextKey(); err != nil || k.Kind != TextIgnore {
		t.Errorf("ESC[A in a text prompt = (%+v, %v), want (TextIgnore, nil)", k, err)
	}
	if k, err := term.NextTextKey(); err != nil || k.Kind != TextRune || k.Rune != 'n' {
		t.Errorf("the key after an ignored sequence = (%+v, %v), want the rune 'n'", k, err)
	}
	if _, err := term.NextTextKey(); err != io.EOF {
		t.Errorf("NextTextKey at end of stream error = %v, want io.EOF", err)
	}
}
