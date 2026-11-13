package tracker

// Behavior of the event decoder the command classifier needs. The python
// original (tracker-card-gate.sh) runs json.loads on the hook payload, so the
// decoder accepts what CPython accepts: NaN and Infinity literals, lone
// surrogates kept as text, integer literals up to 4300 digits.

import "testing"

func TestPyJSONAcceptsWhatCPythonAccepts(t *testing.T) {
	cases := map[string]string{
		"nan literal":        `{"x":NaN,"y":-Infinity,"z":Infinity}`,
		"escaped pair":       `{"s":"\ud83d\ude00"}`,
		"lone high escape":   `{"s":"\ud83dz"}`,
		"lone low escape":    `{"s":"\ude00"}`,
		"raw invalid byte":   "{\"s\":\"\xff\"}",
		"whitespace kinds":   " \t\r\n{ \"a\" :\t[ 1 , 2 ]\n}\n",
		"duplicate key":      `{"a":1,"a":2}`,
		"exponent underflow": `{"a":1e-400}`,
	}
	for name, input := range cases {
		if _, err := decodePyJSON([]byte(input)); err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
	}
}

func TestPyJSONRejectsWhatCPythonRejects(t *testing.T) {
	cases := map[string]string{
		"empty":             ``,
		"trailing comma":    `{"a":1,}`,
		"leading zero":      `{"a":01}`,
		"bare dot":          `{"a":1.}`,
		"control in string": "{\"a\":\"x\x01\"}",
		"bad escape":        `{"a":"\x"}`,
		"short unicode":     `{"a":"\u12"}`,
		"bad pair hex":      `{"a":"\ud83d\uzzzz"}`,
		"bom":               "\xef\xbb\xbf{}",
		"extra data":        `{} {}`,
		"bad literal":       `{"a":nul}`,
		"int too long":      `{"a":` + longDigits(4301) + `}`,
	}
	for name, input := range cases {
		if _, err := decodePyJSON([]byte(input)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestPyJSONIntegerDigitLimitIsExact(t *testing.T) {
	if _, err := decodePyJSON([]byte(`{"a":` + longDigits(4300) + `}`)); err != nil {
		t.Fatalf("4300 digits must decode: %v", err)
	}
	// A fraction is a float: CPython does not apply the integer digit limit.
	if _, err := decodePyJSON([]byte(`{"a":` + longDigits(4301) + `.5}`)); err != nil {
		t.Fatalf("a float with 4301 digits must decode: %v", err)
	}
}

func TestPyTruthinessFollowsPython(t *testing.T) {
	cases := map[string]bool{
		`0`: false, `-0`: false, `0.0`: false, `1e-400`: false,
		`1`: true, `NaN`: true, `-Infinity`: true,
		`""`: false, `"x"`: true, `[]`: false, `[0]`: true, `{}`: false, `{"a":0}`: true,
		`null`: false, `false`: false, `true`: true,
	}
	for literal, want := range cases {
		value, err := decodePyJSON([]byte(`[` + literal + `]`))
		if err != nil {
			t.Fatalf("%s: %v", literal, err)
		}
		item := value.([]any)[0]
		if got := pyTruthy(item); got != want {
			t.Errorf("truthy(%s) = %v, want %v", literal, got, want)
		}
	}
}

func TestPyJSONKeepsLoneSurrogatesVisibleToPrintCheck(t *testing.T) {
	value, err := decodePyJSON([]byte(`{"s":"a\ud800b","t":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	object := value.(map[string]any)
	if pyPrintable(object["s"].(string)) {
		t.Fatal("a lone surrogate must not be printable")
	}
	if !pyPrintable(object["t"].(string)) {
		t.Fatal("plain text must be printable")
	}
}

func longDigits(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = '7'
	}
	return string(out)
}
