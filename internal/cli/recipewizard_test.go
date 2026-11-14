package cli

// GO-20 U6d-2 (T4b): the wizard core, config_wizard.run_config_wizard ported
// onto internal/tui behind an injected prompt seam. Every case below drives
// runConfigWizard with SCRIPTED answers and asserts through the transcript of
// prompt calls — sorted iteration, the exact Python message shape, each step
// of default resolution, the enum/bool/text branches, the three validator
// shapes with their re-prompt (the failing message appears, the next answer
// is accepted), blank→omit and cancel→nothing. The production wiring onto
// internal/tui is pinned separately, over the same scripted key source the
// init-wizard tests use.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/tui"
)

// ── the scripted prompt seam ──

type scriptedConfirm struct {
	answer bool
	ok     bool // false = the prompt was cancelled
}

type scriptedSelect struct {
	answer string
	ok     bool
}

// scriptedTextAnswer is one Text call: every answer is tried in order through
// the validator (questionary re-prompts inside the one prompt), an empty queue
// cancels like questionary's None.
type scriptedTextAnswer struct {
	answers []string
}

// scriptedWizardPrompts implements wizardPrompts with scripted answers and
// records every call in one ordered transcript, so tests can assert the
// message shape, the resolved default, the validator messages and the call
// order together.
type scriptedWizardPrompts struct {
	confirmQ   []scriptedConfirm
	selectQ    []scriptedSelect
	textQ      []scriptedTextAnswer
	t          *testing.T
	transcript []string
}

func (s *scriptedWizardPrompts) Help(text string) {
	s.transcript = append(s.transcript, "help: "+text)
}

func (s *scriptedWizardPrompts) Select(message string, choices []string, initial string) (string, bool) {
	if len(s.selectQ) == 0 {
		s.t.Fatalf("unexpected Select %q: no scripted answer", message)
	}
	a := s.selectQ[0]
	s.selectQ = s.selectQ[1:]
	s.transcript = append(s.transcript,
		fmt.Sprintf("select: %s | choices=%v | initial=%q | answer=%q ok=%v",
			message, choices, initial, a.answer, a.ok))
	return a.answer, a.ok
}

func (s *scriptedWizardPrompts) Confirm(message string, def bool) (bool, bool) {
	if len(s.confirmQ) == 0 {
		s.t.Fatalf("unexpected Confirm %q: no scripted answer", message)
	}
	a := s.confirmQ[0]
	s.confirmQ = s.confirmQ[1:]
	s.transcript = append(s.transcript,
		fmt.Sprintf("confirm: %s | def=%v | answer=%v ok=%v", message, def, a.answer, a.ok))
	return a.answer, a.ok
}

func (s *scriptedWizardPrompts) Text(message, defaultValue string, validate func(string) error) (string, bool) {
	if len(s.textQ) == 0 {
		s.t.Fatalf("unexpected Text %q: no scripted answer", message)
	}
	qa := s.textQ[0]
	s.textQ = s.textQ[1:]
	s.transcript = append(s.transcript, fmt.Sprintf("text: %s | def=%q", message, defaultValue))
	for {
		if len(qa.answers) == 0 {
			s.transcript = append(s.transcript, `answer: "" ok=false`)
			return "", false
		}
		answer := qa.answers[0]
		qa.answers = qa.answers[1:]
		if validate != nil {
			if err := validate(answer); err != nil {
				// questionary's re-prompt: message shown, same prompt again.
				s.transcript = append(s.transcript, "invalid: "+err.Error())
				continue
			}
		}
		s.transcript = append(s.transcript, fmt.Sprintf("answer: %q ok=true", answer))
		return answer, true
	}
}

// ── field fixtures ──

func wizField(required bool, typ string, def any) *schema.ConfigField {
	return &schema.ConfigField{Required: required, Type: typ, Default: def}
}

// TestRunConfigWizardPromptsSortedFieldsWithThePythonMessage pins the loop
// and the message: sorted(fields) — never .extra/tables — one prompt per
// field, help_text printed immediately before its prompt, and the message
// `<recipe>.<key> (<required|optional>, <kind>)` with kind = the declared
// type, "enum" only when no type is declared, "string" when neither is.
func TestRunConfigWizardPromptsSortedFieldsWithThePythonMessage(t *testing.T) {
	fields := map[string]*schema.ConfigField{
		"zeta":  {Required: true, Type: "string", HelpText: "zeta help"},
		"alpha": {Required: false, Type: ""},
		"mid":   {Required: false, Type: "", Enum: []string{"a", "b"}},
		"flip":  {Required: false, Type: "bool"},
	}
	p := &scriptedWizardPrompts{t: t}
	p.textQ = []scriptedTextAnswer{{answers: []string{"av"}}, {answers: []string{"zv"}}}
	p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
	p.selectQ = []scriptedSelect{{answer: "a", ok: true}}

	values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
	if err != nil || !ok {
		t.Fatalf("runConfigWizard = (_, %v, %v), want ok with no error", ok, err)
	}
	want := []string{
		`text: rec.alpha (optional, string) | def=""`,
		`answer: "av" ok=true`,
		`confirm: rec.flip (optional, bool) | def=false | answer=true ok=true`,
		`select: rec.mid (optional, enum) | choices=[a b] | initial="a" | answer="a" ok=true`,
		`help: zeta help`,
		`text: rec.zeta (required, string) | def=""`,
		`answer: "zv" ok=true`,
	}
	if !reflect.DeepEqual(p.transcript, want) {
		t.Errorf("transcript mismatch:\n got = %#v\nwant = %#v", p.transcript, want)
	}
	if values["alpha"] != "av" || values["zeta"] != "zv" || values["mid"] != "a" || values["flip"] != true {
		t.Errorf("values = %#v", values)
	}
	// .extra and structured tables are never prompted: only schema.Fields
	// entered the transcript (the want slice above already pins the count).
}

// TestRunConfigWizardFieldDefaultResolution pins _field_default's four steps
// exactly: existing_config wins, else field.default when set, else False for
// a bool, else "" — with Python's str() rendering for the text prefill and
// Python's bool() truthiness for the confirm default.
func TestRunConfigWizardFieldDefaultResolution(t *testing.T) {
	fields := map[string]*schema.ConfigField{
		"old":      wizField(false, "string", "from-field"),
		"fresh":    wizField(false, "string", "from-field"),
		"bare":     wizField(false, "string", nil),
		"num":      wizField(false, "integer", int64(3)),
		"listy":    wizField(false, "list", []any{"alpha", "beta"}),
		"flag":     wizField(false, "bool", nil),
		"flagdef":  wizField(false, "bool", true),
		"flagfrom": wizField(false, "bool", nil),
		"flagzero": wizField(false, "bool", nil),
	}
	existing := map[string]any{
		"old":      "from-manifest",
		"flagfrom": "no", // Python bool("no") is True — a non-empty string is truthy
		"flagzero": int64(0),
	}
	p := &scriptedWizardPrompts{t: t}
	for i := 0; i < 5; i++ {
		// Optional fields accept (and omit) the blank answer.
		p.textQ = append(p.textQ, scriptedTextAnswer{answers: []string{""}})
	}
	p.confirmQ = make([]scriptedConfirm, 4)
	for i := range p.confirmQ {
		p.confirmQ[i] = scriptedConfirm{answer: true, ok: true}
	}
	_, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, existing, p)
	if err != nil || !ok {
		t.Fatalf("runConfigWizard = (_, %v, %v)", ok, err)
	}
	wantDefs := map[string]string{
		"old":   `def="from-manifest"`,
		"fresh": `def="from-field"`,
		"bare":  `def=""`,
		"num":   `def="3"`,
		"listy": `def="['alpha', 'beta']"`,
	}
	for key, want := range wantDefs {
		assertTranscriptContains(t, p.transcript, fmt.Sprintf("rec.%s ", key), want)
	}
	wantConfirm := map[string]string{
		"flag":     "def=false", // neither existing nor default → False
		"flagdef":  "def=true",  // field.default
		"flagfrom": "def=true",  // bool("no")
		"flagzero": "def=false", // bool(0)
	}
	for key, want := range wantConfirm {
		assertTranscriptContains(t, p.transcript, fmt.Sprintf("rec.%s ", key), want)
	}
}

// TestRunConfigWizardEnumSelectBranch pins the enum step: the select gets the
// declared choices, the default when it IS one of them and the FIRST choice
// when it is not (including a non-string existing value, which Python's
// `default in choices` never matches). A declared TYPE only names the message
// (kind = field.type); the enum still drives the branch, exactly like
// Python's `if field.enum:` before the type checks.
func TestRunConfigWizardEnumSelectBranch(t *testing.T) {
	fields := map[string]*schema.ConfigField{
		"match":     {Required: false, Type: "", Enum: []string{"a", "b", "c"}},
		"nomatch":   {Required: false, Type: "", Enum: []string{"a", "b"}},
		"defmatch":  {Required: false, Type: "", Enum: []string{"a", "b"}},
		"defmiss":   {Required: false, Type: "", Enum: []string{"a", "b"}},
		"nonstring": {Required: false, Type: "", Enum: []string{"a", "b"}},
		"typed":     {Required: false, Type: "string", Enum: []string{"a", "b"}},
	}
	existing := map[string]any{
		"match":     "b",
		"nomatch":   "zzz",
		"nonstring": int64(7),
		"typed":     "b",
	}
	fields["defmatch"].Default = "b"
	fields["defmiss"].Default = "zzz"

	p := &scriptedWizardPrompts{t: t}
	p.selectQ = []scriptedSelect{
		{answer: "b", ok: true}, {answer: "a", ok: true}, {answer: "b", ok: true},
		{answer: "a", ok: true}, {answer: "a", ok: true}, {answer: "b", ok: true},
	}
	values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, existing, p)
	if err != nil || !ok {
		t.Fatalf("runConfigWizard = (_, %v, %v)", ok, err)
	}
	for _, want := range []string{
		`select: rec.match (optional, enum) | choices=[a b c] | initial="b"`,
		`select: rec.nomatch (optional, enum) | choices=[a b] | initial="a"`,
		`select: rec.defmatch (optional, enum) | choices=[a b] | initial="b"`,
		`select: rec.defmiss (optional, enum) | choices=[a b] | initial="a"`,
		`select: rec.nonstring (optional, enum) | choices=[a b] | initial="a"`,
		`select: rec.typed (optional, string) | choices=[a b] | initial="b"`,
	} {
		assertTranscriptContains(t, p.transcript, want)
	}
	if values["typed"] != "b" {
		t.Errorf("typed enum answer = %#v, want %q (the enum drives the branch)", values["typed"], "b")
	}
}

// TestRunConfigWizardTextValidatorsReprompt pins the three validator shapes
// with their re-prompt: each failing answer records the EXACT Python message
// in the transcript and the NEXT scripted answer is accepted into the result.
func TestRunConfigWizardTextValidatorsReprompt(t *testing.T) {
	t.Run("required only", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{
			"board": {Required: true, Type: "string"},
		}
		p := &scriptedWizardPrompts{t: t}
		p.textQ = []scriptedTextAnswer{{answers: []string{"", "   ", "ok"}}}
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if err != nil || !ok || values["board"] != "ok" {
			t.Fatalf("values=%#v ok=%v err=%v", values, ok, err)
		}
		want := []string{
			`text: rec.board (required, string) | def=""`,
			"invalid: This field is required.",
			"invalid: This field is required.",
			`answer: "ok" ok=true`,
		}
		assertTranscriptOrder(t, p.transcript, want)
	})

	t.Run("regex only: mismatch reprompts, empty always passes", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{
			"retries": {Required: false, Type: "string", Validation: map[string]any{"regex": "^[0-9]+$"}},
		}
		p := &scriptedWizardPrompts{t: t}
		p.textQ = []scriptedTextAnswer{{answers: []string{"abc", "42"}}}
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if err != nil || !ok || values["retries"] != "42" {
			t.Fatalf("values=%#v ok=%v err=%v", values, ok, err)
		}
		assertTranscriptOrder(t, p.transcript, []string{
			"invalid: Must match ^[0-9]+$",
			`answer: "42" ok=true`,
		})

		// Empty passes the regex check (Python: `if not value: return True`)
		// and is then omitted by the wizard's blank rule.
		p2 := &scriptedWizardPrompts{t: t}
		p2.textQ = []scriptedTextAnswer{{answers: []string{""}}}
		values2, ok2, err2 := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p2)
		if err2 != nil || !ok2 {
			t.Fatalf("empty answer ok=%v err=%v", ok2, err2)
		}
		if _, present := values2["retries"]; present {
			t.Errorf("empty answer must be omitted, got %#v", values2)
		}
		if !reflect.DeepEqual(p2.transcript[len(p2.transcript)-1], `answer: "" ok=true`) {
			t.Errorf("transcript tail = %q, want the accepted empty answer", p2.transcript[len(p2.transcript)-1])
		}
	})

	t.Run("composed: required is checked before the regex", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{
			"code": {Required: true, Type: "string", Validation: map[string]any{"regex": "^[0-9]+$"}},
		}
		// Whitespace-only: required fails — the message must be the required
		// one, never "Must match".
		p := &scriptedWizardPrompts{t: t}
		p.textQ = []scriptedTextAnswer{{answers: []string{"  ", "7"}}}
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if err != nil || !ok || values["code"] != "7" {
			t.Fatalf("values=%#v ok=%v err=%v", values, ok, err)
		}
		assertTranscriptOrder(t, p.transcript, []string{
			"invalid: This field is required.",
			`answer: "7" ok=true`,
		})
		if strings.Contains(strings.Join(p.transcript, "\n"), "Must match") {
			t.Errorf("the regex must not be consulted when required already failed: %#v", p.transcript)
		}

		// Empty: required fails first as well.
		p2 := &scriptedWizardPrompts{t: t}
		p2.textQ = []scriptedTextAnswer{{answers: []string{"", "7"}}}
		_, ok2, err2 := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p2)
		if err2 != nil || !ok2 {
			t.Fatalf("ok=%v err=%v", ok2, err2)
		}
		assertTranscriptOrder(t, p2.transcript, []string{
			"invalid: This field is required.",
			`answer: "7" ok=true`,
		})

		// Non-empty and non-matching: the regex message, then acceptance.
		p3 := &scriptedWizardPrompts{t: t}
		p3.textQ = []scriptedTextAnswer{{answers: []string{"12x", "8"}}}
		values3, ok3, err3 := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p3)
		if err3 != nil || !ok3 || values3["code"] != "8" {
			t.Fatalf("values=%#v ok=%v err=%v", values3, ok3, err3)
		}
		assertTranscriptOrder(t, p3.transcript, []string{
			"invalid: Must match ^[0-9]+$",
			`answer: "8" ok=true`,
		})
	})
}

// TestRunConfigWizardBlankAnswerOmitsTheKey pins the blank rule: a whitespace
// answer to an optional text prompt is accepted by the validator-less prompt
// and OMITTED from the result, so write-back keeps the existing/default value.
func TestRunConfigWizardBlankAnswerOmitsTheKey(t *testing.T) {
	fields := map[string]*schema.ConfigField{
		"note": {Required: false, Type: "string"},
		"keep": {Required: false, Type: "string"},
	}
	p := &scriptedWizardPrompts{t: t}
	p.textQ = []scriptedTextAnswer{
		{answers: []string{"kept"}},  // keep (sorted first)
		{answers: []string{"  \t "}}, // note comes second alphabetically
	}
	values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, present := values["note"]; present {
		t.Errorf("blank answer was not omitted: %#v", values)
	}
	if values["keep"] != "kept" {
		t.Errorf("values = %#v", values)
	}
}

// TestRunConfigWizardCancelReturnsNothing pins cancel semantics on every
// branch: a cancelled prompt discards the WHOLE run — no partial map, ok=false,
// no error — exactly like Python's `return {}` (whose callers skip the write).
func TestRunConfigWizardCancelReturnsNothing(t *testing.T) {
	t.Run("select cancel", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{
			"a": {Enum: []string{"x", "y"}},
		}
		p := &scriptedWizardPrompts{t: t}
		p.selectQ = []scriptedSelect{{ok: false}}
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if ok || values != nil || err != nil {
			t.Errorf("values=%#v ok=%v err=%v, want nothing", values, ok, err)
		}
	})
	t.Run("confirm cancel", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{"a": {Type: "bool"}}
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{ok: false}}
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if ok || values != nil || err != nil {
			t.Errorf("values=%#v ok=%v err=%v, want nothing", values, ok, err)
		}
	})
	t.Run("text cancel", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{"a": {Type: "string", Required: true}}
		p := &scriptedWizardPrompts{t: t}
		p.textQ = []scriptedTextAnswer{{}} // exhausted → cancel
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if ok || values != nil || err != nil {
			t.Errorf("values=%#v ok=%v err=%v, want nothing", values, ok, err)
		}
	})
	t.Run("cancel at a later field discards earlier answers", func(t *testing.T) {
		fields := map[string]*schema.ConfigField{
			"a": {Type: "string"},
			"b": {Type: "string"},
		}
		p := &scriptedWizardPrompts{t: t}
		p.textQ = []scriptedTextAnswer{
			{answers: []string{"first"}},
			{}, // second prompt cancelled
		}
		values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
		if ok || values != nil || err != nil {
			t.Errorf("values=%#v ok=%v err=%v, want nothing (Python returns {})", values, ok, err)
		}
	})
}

// TestRunConfigWizardInvalidRegexSurfacesAsAnError: Python compiles the
// pattern inside run_config_wizard and lets re.error escape to the caller's
// `except Exception` — the add verb words it as the wizard-failure line. The
// compile happens when the field is reached, so earlier prompts keep their
// side effects, like the Python loop.
func TestRunConfigWizardInvalidRegexSurfacesAsAnError(t *testing.T) {
	fields := map[string]*schema.ConfigField{
		"a":   {Required: false, Type: "string"},
		"bad": {Required: false, Type: "string", Validation: map[string]any{"regex": "["}},
	}
	p := &scriptedWizardPrompts{t: t}
	p.textQ = []scriptedTextAnswer{{answers: []string{"av"}}}
	values, ok, err := runConfigWizard("rec", &schema.ConfigSchema{Fields: fields}, nil, p)
	if err == nil {
		t.Fatalf("an unparseable validation.regex must error (values=%#v ok=%v)", values, ok)
	}
	if ok || values != nil {
		t.Errorf("values=%#v ok=%v, want nothing alongside the error", values, ok)
	}
	assertTranscriptContains(t, p.transcript, `text: rec.a (optional, string)`, `answer: "av" ok=true`)
}

// TestSortedWizardConfigValuesPinsPythonDictOrder: the wizard inserts keys in
// sorted(fields) order, and UpdateRecipeConfig appends NEW keys in the
// values-slice order — so the caller's slice must be the sorted key order of
// the result map (Python dict insertion order), never Go's map order.
func TestSortedWizardConfigValuesPinsPythonDictOrder(t *testing.T) {
	values := map[string]any{"zeta": "z", "alpha": "a", "flip": true}
	got := sortedWizardConfigValues(values)
	var keys []string
	for _, cv := range got {
		keys = append(keys, cv.Key)
	}
	if want := []string{"alpha", "flip", "zeta"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("key order = %v, want %v", keys, want)
	}
	if got[2].Value != "z" || got[1].Value != true {
		t.Errorf("values not carried: %#v", got)
	}
	if empty := sortedWizardConfigValues(nil); len(empty) != 0 {
		t.Errorf("empty map → %#v, want no assignments", empty)
	}
}

// ── the production wiring onto internal/tui ──

// TestTUIWizardPromptsWireInternalTUI pins the CLI's side of the seam: Help
// prints the dim help line, Select/Confirm/Text route onto SelectDefault,
// Confirm and TextValidated, and a validator failure shows its message and
// lets the next answer through — over the same scripted key source the init
// wizard tests use (no PTY needed).
func TestTUIWizardPromptsWireInternalTUI(t *testing.T) {
	t.Run("Help prints before prompting", func(t *testing.T) {
		var out strings.Builder
		p := tuiWizardPrompts{out: &out, keys: &scriptedWizardKeys{}}
		p.Help("pick a board")
		if want := "ℹ️  pick a board\n"; out.String() != want {
			t.Errorf("Help bytes = %q, want %q", out.String(), want)
		}
	})

	t.Run("Confirm routes to tui.Confirm", func(t *testing.T) {
		var out strings.Builder
		p := tuiWizardPrompts{out: &out, keys: &scriptedWizardKeys{keys: []tui.Key{tui.KeyEnter}}}
		answer, ok := p.Confirm("Configure now?", true)
		if !ok || !answer {
			t.Errorf("Confirm = (%v, %v), want (true, true)", answer, ok)
		}
		if !strings.Contains(out.String(), "? Configure now? (Y/n) yes") {
			t.Errorf("stdout = %q, want the tui.Confirm frame", out.String())
		}
	})

	t.Run("Select routes to tui.SelectDefault with the initial", func(t *testing.T) {
		var out strings.Builder
		p := tuiWizardPrompts{out: &out, keys: &scriptedWizardKeys{keys: []tui.Key{tui.KeyEnter}}}
		answer, ok := p.Select("rec.k (optional, enum)", []string{"a", "b"}, "b")
		if !ok || answer != "b" {
			t.Errorf("Select = (%q, %v), want the initial %q (bare Enter)", answer, ok, "b")
		}
		if !strings.Contains(out.String(), "» b") {
			t.Errorf("stdout = %q, want the cursor parked on the initial", out.String())
		}
	})

	t.Run("Text routes validate through tui.TextValidated", func(t *testing.T) {
		var out strings.Builder
		script := append(append(textRunesTUI("ab"), tui.TextKey{Kind: tui.TextEnter}),
			clearLineTUI()...)
		script = append(script, textRunesTUI("abc")...)
		script = append(script, tui.TextKey{Kind: tui.TextEnter})
		p := tuiWizardPrompts{out: &out, keys: &scriptedWizardKeys{text: script}}
		answer, ok := p.Text("rec.k (required, string)", "", func(v string) error {
			if v == "" || len(v) < 3 {
				return errors.New("must be at least 3 characters")
			}
			return nil
		})
		if !ok || answer != "abc" {
			t.Errorf("Text = (%q, %v), want (\"abc\", true)", answer, ok)
		}
		if !strings.Contains(out.String(), "must be at least 3 characters") {
			t.Errorf("validator message never shown: %q", out.String())
		}
	})

	t.Run("the opener refuses off a terminal", func(t *testing.T) {
		prompts, closeFn, rc := openRecipeWizardPrompts(strings.NewReader(""), &strings.Builder{})
		if rc == 0 || prompts != nil || closeFn != nil {
			t.Errorf("opener = (prompts=%v, nil closer=%v, rc=%d), want a refusal before any prompt",
				prompts, closeFn == nil, rc)
		}
	})
}

// textRunesTUI / clearLineTUI mirror the tui-test helpers for the scripted
// key source (the tui package's own scriptedText is not exported).
func textRunesTUI(s string) []tui.TextKey {
	var out []tui.TextKey
	for _, r := range s {
		out = append(out, tui.TextKey{Kind: tui.TextRune, Rune: r})
	}
	return out
}

func clearLineTUI() []tui.TextKey {
	return []tui.TextKey{{Kind: tui.TextLineStart}, {Kind: tui.TextKillToEnd}}
}

// ── transcript assertions ──

func assertTranscriptContains(t *testing.T, transcript []string, wants ...string) {
	t.Helper()
	joined := strings.Join(transcript, "\n")
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Errorf("transcript missing %q:\n%s", want, joined)
		}
	}
}

// assertTranscriptOrder checks the wants appear in the given order.
func assertTranscriptOrder(t *testing.T, transcript, wants []string) {
	t.Helper()
	at := 0
	for _, want := range wants {
		found := false
		for ; at < len(transcript); at++ {
			if strings.Contains(transcript[at], want) {
				found = true
				at++
				break
			}
		}
		if !found {
			t.Errorf("transcript out of order: missing %q after position %d:\n%s",
				want, at, strings.Join(transcript, "\n"))
			return
		}
	}
}
