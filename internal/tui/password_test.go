package tui

// The Password prompt's contract (GO-20 U4d-2, S30): questionary.password over
// the same TextSource line editor Text uses. What is pinned here is what the
// env offer's secret step branches on — the question drawn before the first
// key (with questionary's instruction token), the typed runes NEVER echoed
// (prompt_toolkit's PasswordProcessor renders one `*` per code point,
// lib/_vendor/prompt_toolkit/layout/processors.py:315), the raw buffer as the
// answer (questionary does not trim), and abort/EOF answering ("", false).

import (
	"bytes"
	"strings"
	"testing"
)

// passwordTyped builds a text script that types s and submits it.
func passwordTyped(s string) *scriptedText {
	script := textRunes(s)
	script = append(script, TextKey{Kind: TextEnter})
	return &scriptedText{text: script}
}

// TestPasswordDrawsTheQuestionBeforeItReads pins questionary's prompt line: the
// qmark, the message and — when the offer passes it — the "(hidden input)"
// instruction token, each with questionary's surrounding spaces, on screen
// before the first keystroke is read (a PTY stage waits for the message).
func TestPasswordDrawsTheQuestionBeforeItReads(t *testing.T) {
	t.Run("with the instruction the offer passes", func(t *testing.T) {
		var out bytes.Buffer
		var onScreen string
		in := &scriptedText{
			text:    append(textRunes("x"), TextKey{Kind: TextEnter}),
			onFirst: func() { onScreen = out.String() },
		}
		got, ok := Password(&out, in, "TRELLO_API_KEY", "(hidden input)")
		if !ok || got != "x" {
			t.Fatalf("Password = (%q, %v), want (\"x\", true)", got, ok)
		}
		if !strings.Contains(onScreen, "? TRELLO_API_KEY  (hidden input) ") {
			t.Errorf("the question was not drawn before the first key: %q", onScreen)
		}
	})

	t.Run("without an instruction the line is Text's shape", func(t *testing.T) {
		var out bytes.Buffer
		var onScreen string
		in := &scriptedText{
			text:    append(textRunes("x"), TextKey{Kind: TextEnter}),
			onFirst: func() { onScreen = out.String() },
		}
		if _, ok := Password(&out, in, "SECRET", ""); !ok {
			t.Fatal("Password aborted, want confirmed")
		}
		if !strings.Contains(onScreen, "? SECRET ") {
			t.Errorf("the question was not drawn before the first key: %q", onScreen)
		}
	})
}

// TestPasswordNeverEchoesTheTypedRunes pins the hidden-input contract the env
// offer needs for secret-looking variables: the answer comes back whole, while
// every byte the prompt wrote is one `*` per code point — the secret itself
// never reaches the screen (or the PTY transcript a test captures).
func TestPasswordNeverEchoesTheTypedRunes(t *testing.T) {
	var out bytes.Buffer
	got, ok := Password(&out, passwordTyped("hunter2"), "SECRET", "")
	if !ok || got != "hunter2" {
		t.Fatalf("Password = (%q, %v), want (\"hunter2\", true)", got, ok)
	}
	if strings.Contains(out.String(), "hunter2") {
		t.Errorf("the typed runes were echoed: %q", out.String())
	}
	if !strings.Contains(out.String(), strings.Repeat("*", len("hunter2"))) {
		t.Errorf("the masked buffer is missing from %q", out.String())
	}
	// The answered line stays masked: prompt_toolkit re-renders the buffer
	// through the same processor when Enter accepts it.
	if !strings.HasSuffix(out.String(), "? SECRET *******\n") {
		t.Errorf("the answered line must be the masked one: %q", out.String())
	}
}

// TestPasswordEditsTheHiddenLine pins that the mask follows the same line
// editor Text uses: backspace and the cursor keys edit the hidden buffer, and
// the displayed mask grows and shrinks with it.
func TestPasswordEditsTheHiddenLine(t *testing.T) {
	var out bytes.Buffer
	script := textRunes("abc")
	script = append(script, TextKey{Kind: TextBackspace})
	script = append(script, textRunes("d")...)
	script = append(script, TextKey{Kind: TextEnter})
	got, ok := Password(&out, &scriptedText{text: script}, "SECRET", "")
	if !ok || got != "abd" {
		t.Errorf("Password after backspace = (%q, %v), want (\"abd\", true)", got, ok)
	}
	if strings.Contains(out.String(), "abd") {
		t.Errorf("the edited answer leaked through the mask: %q", out.String())
	}
	if !strings.HasSuffix(out.String(), "? SECRET ***\n") {
		t.Errorf("the mask must track the edited length: %q", out.String())
	}

	// Cursor-left insert: the answer is the edited line, still masked.
	out.Reset()
	script = textRunes("ab")
	script = append(script, TextKey{Kind: TextCursorLeft})
	script = append(script, textRunes("X")...)
	script = append(script, TextKey{Kind: TextEnter})
	got, ok = Password(&out, &scriptedText{text: script}, "SECRET", "")
	if !ok || got != "aXb" {
		t.Errorf("Password after cursor insert = (%q, %v), want (\"aXb\", true)", got, ok)
	}
	if strings.Contains(out.String(), "aXb") {
		t.Errorf("the edited runes were echoed: %q", out.String())
	}
}

// TestPasswordReturnsTheRawBuffer pins S30's correction: questionary.password
// is questionary.text with a mask, and questionary.text does not trim — the
// env prompt hands the buffer verbatim to write_env, whose own blank rule owns
// the "never wipe a secret" invariant.
func TestPasswordReturnsTheRawBuffer(t *testing.T) {
	var out bytes.Buffer
	got, ok := Password(&out, passwordTyped("  padded  "), "SECRET", "")
	if !ok {
		t.Fatal("Password aborted, want confirmed")
	}
	if got != "  padded  " {
		t.Errorf("Password = %q, want the raw buffer %q", got, "  padded  ")
	}
}

// TestPasswordAbortsReturnNothing pins the cancel semantics the offer's
// "cancelling cancels the offer" rule branches on: ESC, Ctrl-C and a closed
// stream all end as ("", false) with the drawn line terminated.
func TestPasswordAbortsReturnNothing(t *testing.T) {
	cases := map[string][]TextKey{
		"escape": {{Kind: TextAbort}},
		"ctrl-c": {{Kind: TextAbort}}, // the decoder maps 0x03 to TextAbort
		"eof":    {},                  // no events: the source answers io.EOF
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok := Password(&out, &scriptedText{text: script}, "SECRET", "(hidden input)")
			if ok || got != "" {
				t.Errorf("Password(%s) = (%q, %v), want (\"\", false)", name, got, ok)
			}
			if !strings.HasSuffix(out.String(), "\n") {
				t.Errorf("the aborted line was not terminated: %q", out.String())
			}
		})
	}
}

// TestPasswordNeedsARuneCapableSource documents the shared seam: Password
// reads characters through TextSource like Text, so a KeySource without the
// rune capability refuses rather than accepting an invisible answer.
func TestPasswordNeedsARuneCapableSource(t *testing.T) {
	var out bytes.Buffer
	got, ok := Password(&out, keys(KeyEnter), "SECRET", "")
	if ok || got != "" {
		t.Errorf("Password over a key-only source = (%q, %v), want (\"\", false)", got, ok)
	}
}
