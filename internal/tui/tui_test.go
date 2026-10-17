package tui

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// scriptedKeys is the test seam for the whole layer: the real key source puts a
// terminal into raw mode and reads bytes from it, while a test feeds the exact key
// stream it wants the user to have pressed. The raw-mode plumbing itself is covered
// by the PTY acceptance suites, which drive the built binary through a real pty.
type scriptedKeys struct {
	keys []Key
	at   int
}

func (s *scriptedKeys) NextKey() (Key, error) {
	if s.at >= len(s.keys) {
		return 0, io.EOF
	}
	key := s.keys[s.at]
	s.at++
	return key, nil
}

func keys(ks ...Key) *scriptedKeys { return &scriptedKeys{keys: ks} }

// keySourceFunc is a KeySource whose NextKey runs a callback, which is how a test
// observes what the caller had already drawn when it asked for the first key.
type keySourceFunc func() (Key, error)

func (f keySourceFunc) NextKey() (Key, error) { return f() }

// threeChoices is the fixture list the select tests navigate: two real entries and a
// Quit, which is the shape the hub's menu has.
func threeChoices() []Choice {
	return []Choice{{Title: "Sync", Value: "sync"}, {Title: "Doctor", Value: "doctor"}, {Title: "Quit", Value: "quit"}}
}

// TestSelectMovesAndConfirms pins navigation and the value a caller receives.
func TestSelectMovesAndConfirms(t *testing.T) {
	var out bytes.Buffer
	got, ok := Select(&out, keys(KeyDown, KeyDown, KeyEnter), "What do you want to do?", threeChoices())
	if !ok || got != "quit" {
		t.Errorf("Select = (%q, %v), want (\"quit\", true)", got, ok)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "What do you want to do?") {
		t.Errorf("prompt message missing from output: %q", rendered)
	}
	for _, want := range []string{"Sync", "Doctor", "Quit"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("choice %q missing from output: %q", want, rendered)
		}
	}
}

// TestFrameKeepsQuestionaryMessageLine pins the exact bytes of the message line,
// trailing space included. questionary renders `? <message> ` before its choices,
// and two PTY acceptance stages wait for `b"Recipes: "` and `b"Skills: "`; without
// that space those stages never fire and the suite TIMES OUT instead of failing.
func TestFrameKeepsQuestionaryMessageLine(t *testing.T) {
	var out bytes.Buffer
	if _, ok := Select(&out, keys(KeyEnter), "Recipes:", threeChoices()); !ok {
		t.Fatal("Select aborted, want confirmed")
	}
	if !strings.Contains(out.String(), "? Recipes: \n") {
		t.Errorf("message line = %q, want it to carry questionary's `? Recipes: ` shape", out.String())
	}
}

// TestSelectAbortsReturnNothing pins the abort semantics hub.py's callers branch on:
// an aborted prompt is the empty choice with ok=false, never a crash.
func TestSelectAbortsReturnNothing(t *testing.T) {
	cases := map[string]Key{"escape": KeyEscape, "ctrl-c": KeyCtrlC}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok := Select(&out, keys(key), "Pick:", threeChoices())
			if ok || got != "" {
				t.Errorf("Select after %s = (%q, %v), want (\"\", false)", name, got, ok)
			}
		})
	}
}

// TestSelectReportsEOF pins the closed-pty case: EOF is an abort, not an error the
// hub has to translate.
func TestSelectReportsEOF(t *testing.T) {
	var out bytes.Buffer
	got, ok := Select(&out, keys(), "Pick:", threeChoices())
	if ok || got != "" {
		t.Errorf("Select on EOF = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestMultiSelectTogglesAndConfirms pins the checkbox: space toggles relative to the
// INITIAL state, enter confirms, and the result follows the choice order rather than
// the order in which the user toggled things.
func TestMultiSelectTogglesAndConfirms(t *testing.T) {
	var out bytes.Buffer
	choices := threeChoices()
	// Start checked on the middle entry, toggle the first and the last on, and never
	// touch the middle one again: it must come back checked, in its own position.
	got, ok := MultiSelect(&out, keys(KeySpace, KeyDown, KeyDown, KeySpace, KeyEnter),
		"Select agents to enable:", choices, []bool{false, true, false})
	if !ok {
		t.Fatal("MultiSelect aborted, want confirmed")
	}
	want := []string{"sync", "doctor", "quit"}
	if len(got) != len(want) {
		t.Fatalf("MultiSelect = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MultiSelect = %v, want %v (order must follow the choices)", got, want)
		}
	}
}

// TestConfirmDrawsTheQuestionBeforeItReads pins the ORDER, which is not cosmetic:
// questionary renders the question first, and a confirm that only prints once it has
// been answered is invisible on a real terminal — and deadlocks any PTY stage that
// waits for the question text before it types the answer. That is exactly how
// tests/test_hub.py::test_confirm_action_returns_bool and
// tests/test_hub.py::TestGatingDecision's offer-init stage caught it.
func TestConfirmDrawsTheQuestionBeforeItReads(t *testing.T) {
	var out bytes.Buffer
	var onScreenWhenFirstKeyWasRead string
	in := keySourceFunc(func() (Key, error) {
		onScreenWhenFirstKeyWasRead = out.String()
		return KeyEnter, nil
	})
	if _, ok := Confirm(&out, in, "Run the init wizard now?", true); !ok {
		t.Fatal("Confirm aborted, want confirmed")
	}
	if !strings.Contains(onScreenWhenFirstKeyWasRead, "Run the init wizard now?") {
		t.Errorf("the question was not on screen when the first key was read: %q", onScreenWhenFirstKeyWasRead)
	}
	if !strings.Contains(out.String(), "yes") {
		t.Errorf("the submitted answer is missing from the drawn line: %q", out.String())
	}
}

// TestConfirmUsesItsDefault pins confirm's default and its abort.
func TestConfirmUsesItsDefault(t *testing.T) {
	var out bytes.Buffer
	yes, ok := Confirm(&out, keys(KeyEnter), "Run the init wizard now?", true)
	if !ok || !yes {
		t.Errorf("Confirm(enter, default=true) = (%v, %v), want (true, true)", yes, ok)
	}

	// y/n set the answer and ENTER submits it. That ordering is deliberate: if a single
	// y/n closed the prompt, the newline the user pressed after it would stay in the
	// stream and the hub's loop would read it as an Enter on the NEXT prompt, silently
	// confirming the default selection there.
	out.Reset()
	no, ok := Confirm(&out, keys(KeyNo, KeyEnter), "Run the init wizard now?", true)
	if !ok || no {
		t.Errorf("Confirm(no then enter, default=true) = (%v, %v), want (false, true)", no, ok)
	}

	// An unknown key is ignored, not a decision: the prompt keeps waiting, and a
	// stream that then ends is an abort rather than a silent default.
	out.Reset()
	if _, ok := Confirm(&out, keys(KeyRune), "Run the init wizard now?", true); ok {
		t.Error("Confirm(unknown key then EOF) = ok=true, want an abort")
	}

	out.Reset()
	_, ok = Confirm(&out, keys(KeyEscape), "Run the init wizard now?", true)
	if ok {
		t.Error("Confirm(escape) = ok=true, want an abort")
	}
}

// TestPauseWaitsForEnter pins pause: any other key keeps waiting, enter returns, and
// a closed stdin stops waiting instead of hanging.
func TestPauseWaitsForEnter(t *testing.T) {
	var out bytes.Buffer
	if !Pause(&out, keys(KeyDown, KeySpace, KeyEnter), "Press Enter to return…") {
		t.Error("Pause = false after enter, want true")
	}
	out.Reset()
	if Pause(&out, keys(), "Press Enter to return…") {
		t.Error("Pause = true on EOF, want false so the caller stops instead of hanging")
	}
}

// TestPanelCarriesTheFrozenVocabulary pins the panel's rows and its border colour:
// the vocabulary is what the pty suite asserts in plain text, and the border is what
// hub.py chose from the severities.
func TestPanelCarriesTheFrozenVocabulary(t *testing.T) {
	data := PanelData{
		Version: "0.24.0",
		Target:  "/tmp/project",
		Counts:  Counts{OK: 12, Info: 2, Warn: 1, Error: 0},
		Checks:  []PanelCheck{{Label: "WARN", Text: "agents: no manifest"}},
	}
	green := RenderPanel(data, false)
	for _, want := range []string{"ai-specs", "version", "0.24.0", "target", "/tmp/project",
		"12 OK, 2 INFO, 1 WARN, 0 ERROR (1 warning)", "WARN", "agents: no manifest"} {
		if !strings.Contains(green, want) {
			t.Errorf("panel is missing %q:\n%s", want, green)
		}
	}

	data.Counts.Error = 1
	data.Counts.Warn = 0
	red := RenderPanel(data, false)
	if !strings.Contains(red, "1 error(s)") {
		t.Errorf("panel headline must keep the literal error(s):\n%s", red)
	}
	if green == red {
		t.Error("a panel with an error must render differently from a healthy one")
	}

	colourless := RenderPanel(data, true)
	for _, ansi := range []string{"\x1b[31m", "\x1b[33m", "\x1b[32m"} {
		if strings.Contains(colourless, ansi) {
			t.Errorf("no-colour rendering must not emit %q:\n%s", ansi, colourless)
		}
	}
	if !strings.Contains(colourless, "ai-specs") {
		t.Error("no-colour rendering must still print the panel")
	}
}

// TestKeySourceErrorsAreNotSwallowed keeps the contract explicit: a real read error
// has to surface as an abort rather than as a silent "user chose nothing".
func TestKeySourceErrorsAreNotSwallowed(t *testing.T) {
	failing := failingKeys{errors.New("terminal went away")}
	var out bytes.Buffer
	if _, ok := Select(&out, failing, "Pick:", threeChoices()); ok {
		t.Error("Select with a failing key source = ok=true, want an abort")
	}
}

type failingKeys struct{ err error }

func (f failingKeys) NextKey() (Key, error) { return 0, f.err }
