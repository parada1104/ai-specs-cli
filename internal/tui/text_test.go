package tui

// The Text prompt's contract (odd/tasks/go-20-u4-init.md S13): the wizard's one
// new primitive, a questionary.text port. What is pinned here is the behaviour
// the wizard's name step branches on — draw before read, trim + default on an
// empty answer, abort to ("", false) — and the TextSource capability the
// KeyKind-only KeySource seam cannot carry (a rune needs its own stream).

import (
	"bytes"
	"errors"
	"io"
	"os"
	"reflect"
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

// TestTextRawReturnsTheBufferVerbatim pins S30's third correction: the env
// offer's value prompt is questionary.text, which does NOT trim — S14's
// strip() belongs to the project-name step alone. TextRaw shares the same
// draw-before-read editor over the same TextSource and answers with the
// buffer as typed; questionary resets the buffer to the default instead of
// substituting it afterwards, so a cleared prefill stays cleared.
func TestTextRawReturnsTheBufferVerbatim(t *testing.T) {
	t.Run("typed value is not stripped", func(t *testing.T) {
		script := append(textRunes("  padded  "), TextKey{Kind: TextEnter})
		var raw bytes.Buffer
		got, ok := TextRaw(&raw, in2(script), "VAR:", "")
		if !ok || got != "  padded  " {
			t.Errorf("TextRaw = (%q, %v), want the raw buffer %q", got, ok, "  padded  ")
		}
		// The contrast that makes this a split and not a change: Text still
		// strips, because the wizard's name step depends on it (S14).
		var stripped bytes.Buffer
		if got, ok := Text(&stripped, in2(append(textRunes("  padded  "), TextKey{Kind: TextEnter})), "VAR:", ""); !ok || got != "padded" {
			t.Errorf("Text = (%q, %v), want %q (S14's strip must hold)", got, ok, "padded")
		}
	})

	t.Run("immediate enter returns the prefill", func(t *testing.T) {
		var out bytes.Buffer
		got, ok := TextRaw(&out, in2([]TextKey{{Kind: TextEnter}}), "VAR:", "demo")
		if !ok || got != "demo" {
			t.Errorf("TextRaw = (%q, %v), want %q (the buffer carries the default)", got, ok, "demo")
		}
	})

	t.Run("a cleared prefill stays cleared", func(t *testing.T) {
		script := append(clearLine(), TextKey{Kind: TextEnter})
		var raw bytes.Buffer
		got, ok := TextRaw(&raw, in2(script), "VAR:", "demo")
		if !ok || got != "" {
			t.Errorf("TextRaw = (%q, %v), want %q (no default substitution)", got, ok, "")
		}
		// Text substitutes the default back — the name step's rule (S14).
		var stripped bytes.Buffer
		if got, ok := Text(&stripped, in2(append(clearLine(), TextKey{Kind: TextEnter})), "VAR:", "demo"); !ok || got != "demo" {
			t.Errorf("Text = (%q, %v), want %q", got, ok, "demo")
		}
	})

	t.Run("whitespace-only stays whitespace", func(t *testing.T) {
		// write_env's own blank rule owns "never wipe a secret"; the prompt
		// hands back exactly what was typed.
		script := append(textRunes("   "), TextKey{Kind: TextEnter})
		var out bytes.Buffer
		got, ok := TextRaw(&out, in2(script), "VAR:", "")
		if !ok || got != "   " {
			t.Errorf("TextRaw = (%q, %v), want %q", got, ok, "   ")
		}
	})

	t.Run("aborts answer nothing", func(t *testing.T) {
		for name, script := range map[string][]TextKey{
			"escape": {{Kind: TextAbort}},
			"eof":    {},
		} {
			var out bytes.Buffer
			if got, ok := TextRaw(&out, in2(script), "VAR:", ""); ok || got != "" {
				t.Errorf("TextRaw(%s) = (%q, %v), want (\"\", false)", name, got, ok)
			}
			if !strings.HasSuffix(out.String(), "\n") {
				t.Errorf("the aborted line was not terminated: %q", out.String())
			}
		}
	})
}

// TestTextBytesAreUnchangedForExistingCallers pins the EXACT byte contract
// Text and TextRaw answer with today (GO-20 U6d-2): the validated prompt
// TextValidated adds beside them must not move a single byte of either — the
// draw before the first key, the per-keystroke redraw with its cursor park,
// and the Enter settle are asserted here as literals, so any change to the
// shared engine fails this test first. Preservation test: it is expected to
// pass before and after the slice (PRESERVE baseline, captured pre-change).
func TestTextBytesAreUnchangedForExistingCallers(t *testing.T) {
	cases := map[string]struct {
		prompt func(out *bytes.Buffer, in *scriptedText) (string, bool)
		script []TextKey
		want   string // the exact bytes the prompt must write
		answer string // expected answer
		ok     bool   // expected ok
	}{
		"Text immediate enter": {
			prompt: func(out *bytes.Buffer, in *scriptedText) (string, bool) {
				return Text(out, in, "Project name:", "demo")
			},
			script: []TextKey{{Kind: TextEnter}},
			want: "\r\x1b[2K? Project name: demo" +
				"\r\x1b[2K? Project name: demo\n",
			answer: "demo", ok: true,
		},
		"Text typed append": {
			prompt: func(out *bytes.Buffer, in *scriptedText) (string, bool) {
				return Text(out, in, "Project name:", "demo")
			},
			script: append(textRunes("ab"), TextKey{Kind: TextEnter}),
			want: "\r\x1b[2K? Project name: demo" +
				"\r\x1b[2K? Project name: demoa" +
				"\r\x1b[2K? Project name: demoab" +
				"\r\x1b[2K? Project name: demoab\n",
			answer: "demoab", ok: true,
		},
		"Text cursor park after Ctrl-A": {
			prompt: func(out *bytes.Buffer, in *scriptedText) (string, bool) {
				return Text(out, in, "Project name:", "demo")
			},
			script: append(append([]TextKey{{Kind: TextLineStart}}, textRunes("x")...), TextKey{Kind: TextEnter}),
			want: "\r\x1b[2K? Project name: demo" +
				"\r\x1b[2K? Project name: demo\x1b[4D" +
				"\r\x1b[2K? Project name: xdemo\x1b[4D" +
				"\r\x1b[2K? Project name: xdemo\n",
			answer: "xdemo", ok: true,
		},
		"Text abort terminates the drawn line": {
			prompt: func(out *bytes.Buffer, in *scriptedText) (string, bool) {
				return Text(out, in, "Project name:", "demo")
			},
			script: []TextKey{{Kind: TextAbort}},
			want:   "\r\x1b[2K? Project name: demo" + "\n",
			answer: "", ok: false,
		},
		"TextRaw immediate enter": {
			prompt: func(out *bytes.Buffer, in *scriptedText) (string, bool) {
				return TextRaw(out, in, "VAR:", "demo")
			},
			script: []TextKey{{Kind: TextEnter}},
			want: "\r\x1b[2K? VAR: demo" +
				"\r\x1b[2K? VAR: demo\n",
			answer: "demo", ok: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok := tc.prompt(&out, in2(tc.script))
			if ok != tc.ok || got != tc.answer {
				t.Errorf("answer = (%q, %v), want (%q, %v)", got, ok, tc.answer, tc.ok)
			}
			if out.String() != tc.want {
				t.Errorf("bytes changed:\n got = %q\nwant = %q", out.String(), tc.want)
			}
		})
	}
}

// TestTextValidatedRepromptsUntilTheValidatorAccepts is U6d-2's new primitive:
// questionary's validate= behaviour. A failing Enter prints the validator's
// message under the settled line, keeps the buffer, and draws the prompt
// again; the next answer is accepted. The validator sees the buffer AS TYPED
// (questionary validates the raw line, not a substituted answer).
func TestTextValidatedRepromptsUntilTheValidatorAccepts(t *testing.T) {
	var gotInputs []string
	validate := func(answer string) error {
		gotInputs = append(gotInputs, answer)
		if len(answer) < 3 {
			return errors.New("must be at least 3 characters")
		}
		return nil
	}
	script := append(textRunes("ab"), TextKey{Kind: TextEnter})
	script = append(script, clearLine()...)
	script = append(script, textRunes("abc")...)
	script = append(script, TextKey{Kind: TextEnter})

	var out bytes.Buffer
	got, ok := TextValidated(&out, in2(script), "code:", "", validate)
	if !ok || got != "abc" {
		t.Fatalf("TextValidated = (%q, %v), want (\"abc\", true)", got, ok)
	}
	if !reflect.DeepEqual(gotInputs, []string{"ab", "abc"}) {
		t.Errorf("validator inputs = %q, want [\"ab\" \"abc\"] (raw buffer each round)", gotInputs)
	}
	if !strings.Contains(out.String(), "must be at least 3 characters") {
		t.Errorf("the failing message was never shown: %q", out.String())
	}
	// The prompt is drawn again AFTER the message: message line between two
	// `? code: ` draws.
	if strings.Count(out.String(), "? code: ") < 3 { // initial + per-key redraws + re-draw
		t.Errorf("prompt not redrawn after the failure: %q", out.String())
	}
	if idxMsg := strings.Index(out.String(), "must be at least 3 characters"); idxMsg >= 0 {
		if rest := out.String()[idxMsg:]; !strings.Contains(rest, "? code: ") {
			t.Errorf("no prompt follows the message: %q", out.String())
		}
	}
}

// TestTextValidatedValidatesTheRawBuffer pins questionary's contract on the
// corners: the pre-filled default is validated as the line stands, a cleared
// line reaches the validator as "", and a passing answer comes back VERBATIM
// (no strip, no substitution — config_wizard strips the blank itself).
func TestTextValidatedValidatesTheRawBuffer(t *testing.T) {
	t.Run("untouched prefill is the answer", func(t *testing.T) {
		var inputs []string
		validate := func(answer string) error {
			inputs = append(inputs, answer)
			return nil
		}
		var out bytes.Buffer
		got, ok := TextValidated(&out, in2([]TextKey{{Kind: TextEnter}}), "code:", "demo", validate)
		if !ok || got != "demo" {
			t.Errorf("TextValidated = (%q, %v), want (\"demo\", true)", got, ok)
		}
		if !reflect.DeepEqual(inputs, []string{"demo"}) {
			t.Errorf("validator inputs = %q, want [\"demo\"]", inputs)
		}
	})

	t.Run("cleared line reaches the validator as empty", func(t *testing.T) {
		var inputs []string
		validate := func(answer string) error {
			inputs = append(inputs, answer)
			if answer == "" {
				return errors.New("This field is required.")
			}
			return nil
		}
		script := append(clearLine(), TextKey{Kind: TextEnter})
		script = append(script, textRunes("x")...)
		script = append(script, TextKey{Kind: TextEnter})
		var out bytes.Buffer
		got, ok := TextValidated(&out, in2(script), "code:", "demo", validate)
		if !ok || got != "x" {
			t.Fatalf("TextValidated = (%q, %v), want (\"x\", true)", got, ok)
		}
		if !reflect.DeepEqual(inputs, []string{"", "x"}) {
			t.Errorf("validator inputs = %q, want [\"\" \"x\"]", inputs)
		}
		if !strings.Contains(out.String(), "This field is required.") {
			t.Errorf("the required message was never shown: %q", out.String())
		}
	})

	t.Run("a passing answer is verbatim like TextRaw", func(t *testing.T) {
		validate := func(string) error { return nil }
		var out bytes.Buffer
		got, ok := TextValidated(&out, in2(append(textRunes("  ok  "), TextKey{Kind: TextEnter})), "code:", "", validate)
		if !ok || got != "  ok  " {
			t.Errorf("TextValidated = (%q, %v), want the raw buffer %q", got, ok, "  ok  ")
		}
	})
}

// TestTextValidatedAbortsKeepTheSharedCancelSemantics: ESC and EOF answer
// ("", false) with the drawn line terminated, exactly like Text/TextRaw — a
// cancelled prompt never re-prompts and never invents an answer.
func TestTextValidatedAbortsKeepTheSharedCancelSemantics(t *testing.T) {
	for name, script := range map[string][]TextKey{
		"escape": {{Kind: TextAbort}},
		"eof":    {},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok := TextValidated(&out, in2(script), "code:", "",
				func(string) error { return errors.New("never reached") })
			if ok || got != "" {
				t.Errorf("TextValidated(%s) = (%q, %v), want (\"\", false)", name, got, ok)
			}
			if !strings.HasSuffix(out.String(), "\n") {
				t.Errorf("the aborted line was not terminated: %q", out.String())
			}
		})
	}
}
