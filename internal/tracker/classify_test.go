package tracker

// `tracker classify` event half (GO-20 U2e, unit U-F2b): the classifier the
// tracker-card-gate.sh hook reads its kind/tool/hint protocol from. Every
// expected value below was produced by running the python reference on the same
// event text; the one deliberate difference is documented on its case.

import (
	"testing"
)

func TestClassifyEventMatchesPythonReference(t *testing.T) {
	cases := []struct {
		name   string
		event  string
		wantOK bool
		out    string
	}{
		// --- shell mode detection ---
		{"plain gh pr create", `{"tool_input":{"command":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"gh pr create with flags", `{"tool_input":{"command":"gh pr create --title x"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"gh pr merge is not an action", `{"tool_input":{"command":"gh pr merge"}}`, true, ""},
		{"gh pr create with sudo", `{"tool_input":{"command":"sudo gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"sudo -u root gh pr create", `{"tool_input":{"command":"sudo -u root gh pr create"}}`, true, ""},
		{"sudo -E gh pr create", `{"tool_input":{"command":"sudo -E gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"env FOO=1 gh pr create", `{"tool_input":{"command":"env FOO=1 gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"env -i gh pr create", `{"tool_input":{"command":"env -i gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"nice -n 5 gh pr create", `{"tool_input":{"command":"nice -n 5 gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"time gh pr create", `{"tool_input":{"command":"time gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"nohup gh pr create", `{"tool_input":{"command":"nohup gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"xargs gh pr create", `{"tool_input":{"command":"xargs gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"xargs -I{} gh pr create", `{"tool_input":{"command":"xargs -I{} gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"command gh pr create", `{"tool_input":{"command":"command gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"coproc gh pr create", `{"tool_input":{"command":"coproc gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"coproc NAME { gh pr create; }", `{"tool_input":{"command":"coproc NAME { gh pr create; }"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"coproc { gh pr create; }", `{"tool_input":{"command":"coproc { gh pr create; }"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"VAR=x prefix", `{"tool_input":{"command":"FOO=1 gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"two VAR prefixes", `{"tool_input":{"command":"A=1 B=2 gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"leading =x is not a VAR", `{"tool_input":{"command":"=x gh pr create"}}`, true, ""},
		{"case x in gh) gh pr create", `{"tool_input":{"command":"case x in gh) gh pr create;; esac"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"case with extra space before pattern", `{"tool_input":{"command":"case x in  gh) gh pr create ;; esac"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"case multi pattern", `{"tool_input":{"command":"case x in a|b) gh pr create ;; esac"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"case starred pattern", `{"tool_input":{"command":"case x in *) gh pr create ;; esac"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"case without body action", `{"tool_input":{"command":"case x in esac"}}`, true, ""},
		{"then keyword", `{"tool_input":{"command":"if true; then gh pr create; fi"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"do keyword", `{"tool_input":{"command":"for x in a; do gh pr create; done"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"else keyword", `{"tool_input":{"command":"if x; then a; else gh pr create; fi"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"brace keyword", `{"tool_input":{"command":"{ gh pr create; }"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"paren keyword", `{"tool_input":{"command":"( gh pr create )"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"lone paren then gh", `{"tool_input":{"command":") gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"arithmetic double paren", `{"tool_input":{"command":"(( x = 1 )) ; gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"arithmetic dollar double paren", `{"tool_input":{"command":"echo $((1+2)) ; gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"arithmetic dollar bracket", `{"tool_input":{"command":"echo $[1+2] ; gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"heredoc body must not match", `{"tool_input":{"command":"cat <<EOF\ngh pr create\nEOF"}}`, true, ""},
		{"heredoc dash body must not match", `{"tool_input":{"command":"cat <<-EOF\n\tgh pr create\n\tEOF"}}`, true, ""},
		{"comment must not match", `{"tool_input":{"command":"echo hi # gh pr create"}}`, true, ""},
		{"double quoted must not match", `{"tool_input":{"command":"echo \"gh pr create\""}}`, true, ""},
		{"single quoted must not match", `{"tool_input":{"command":"echo 'gh pr create'"}}`, true, ""},
		{"semicolon separator", `{"tool_input":{"command":"cd x; gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"and-and separator", `{"tool_input":{"command":"cd x && gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"or-or separator", `{"tool_input":{"command":"cd x || gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"pipe separator", `{"tool_input":{"command":"cd x | gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"pipe-amp separator", `{"tool_input":{"command":"cd x |& gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"amp separator", `{"tool_input":{"command":"cd x & gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"two actions", `{"tool_input":{"command":"gh pr create; gh pr create"}}`, true, "shell\tBash\t\npr_create\t\npr_create\t\n"},
		{"gh create order", `{"tool_input":{"command":"gh create pr"}}`, true, ""},
		{"gh flag before pr", `{"tool_input":{"command":"gh -R o/r pr create --draft"}}`, true, ""},
		{"newline separated", `{"tool_input":{"command":"echo x\ngh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"tab separated", `{"tool_input":{"command":"echo x\tgh\tpr\tcreate"}}`, true, ""},
		{"script key", `{"tool_input":{"script":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"cmd key", `{"tool_input":{"cmd":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"top-level command key", `{"command":"gh pr create"}`, true, "shell\tBash\t\npr_create\t\n"},
		{"top-level script key", `{"script":"gh pr create"}`, true, "shell\tBash\t\npr_create\t\n"},
		{"tool_input command wins over top", `{"tool_input":{"command":"gh pr create"},"command":"echo no"}`, true, "shell\tBash\t\npr_create\t\n"},
		{"empty tool_input command falls to script", `{"tool_input":{"command":"","script":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"whitespace tool_input command falls to cmd", `{"tool_input":{"command":" \t ","cmd":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"tool_name missing is Bash", `{"tool_input":{"command":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"tool_name set is used", `{"tool_name":"Bash","tool_input":{"command":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"cwd is used", `{"tool_name":"Bash","cwd":"/repo","tool_input":{"command":"gh pr create"}}`, true, "shell\tBash\t/repo\npr_create\t\n"},
		// --- path mode ---
		{"path relative", `{"tool_name":"Write","cwd":"/repo","tool_input":{"file_path":"a/b.md"}}`, true, "path\tWrite\t/repo\na/b.md\n"},
		{"path absolute", `{"tool_name":"Write","cwd":"/repo","tool_input":{"file_path":"/abs/b.md"}}`, true, "path\tWrite\t/repo\n/abs/b.md\n"},
		{"path whitespace trimmed", `{"tool_name":"Write","cwd":"/repo","tool_input":{"file_path":"  a.md \n"}}`, true, "path\tWrite\t/repo\na.md\n"},
		{"path with inner newline", `{"tool_name":"Write","cwd":"/repo","tool_input":{"file_path":"a\nb.md"}}`, true, "path\tWrite\t/repo\na\nb.md\n"},
		{"path truthy number blocks notebook", `{"tool_input":{"file_path":7,"notebook_path":"n.ipynb"}}`, true, ""},
		{"path truthy list blocks notebook", `{"tool_input":{"file_path":[1],"notebook_path":"n.ipynb"}}`, true, ""},
		{"empty file_path falls to notebook", `{"tool_input":{"file_path":"","notebook_path":"n.ipynb"}}`, true, "path\t\t\nn.ipynb\n"},
		{"null file_path falls to notebook", `{"tool_input":{"file_path":null,"notebook_path":"n.ipynb"}}`, true, "path\t\t\nn.ipynb\n"},
		{"zero file_path falls to notebook", `{"tool_input":{"file_path":0,"notebook_path":"n.ipynb"}}`, true, "path\t\t\nn.ipynb\n"},
		{"missing file_path falls to notebook", `{"tool_input":{"notebook_path":"n.ipynb"}}`, true, "path\t\t\nn.ipynb\n"},
		// --- structural ---
		{"no path no command", `{"tool_input":{}}`, true, ""},
		{"tool_input not object", `{"tool_input":[1]}`, true, ""},
		{"tool_input zero", `{"tool_input":0}`, true, ""},
		{"non-object JSON array", `[1,2]`, true, ""},
		{"non-object JSON string", `"x"`, true, ""},
		{"non-object JSON null", `null`, true, ""},
		{"invalid JSON", `{not json`, true, ""},
		{"empty event", ``, true, ""},
		{"NaN unrelated field", `{"x":NaN,"tool_input":{"file_path":"a.md"}}`, true, "path\t\t\na.md\n"},
		{"NaN unrelated shell", `{"x":NaN,"tool_input":{"command":"gh pr create"}}`, true, "shell\tBash\t\npr_create\t\n"},
		{"NaN command", `{"tool_input":{"command":NaN}}`, true, ""},
		{"emoji path is printable", `{"tool_input":{"file_path":"\ud83d\ude00.md"}}`, true, "path\t\t\n\U0001F600.md\n"},
		// The python prints the first protocol line, then raises on the path, so
		// it exits 1 with partial stdout; the verb prints nothing on a failure.
		{"unpaired surrogate path", `{"tool_input":{"file_path":"\ud800bad.md"}}`, false, ""},
		{"invalid UTF-8 byte in cwd", `{"tool_input":{"command":"gh pr create"},"cwd":"bad\udcff"}`, false, ""},
		{"invalid UTF-8 byte in tool_name", `{"tool_input":{"command":"gh pr create"},"tool_name":"Ba\ud800sh"}`, false, ""},
		{"invalid UTF-8 byte in cwd of path mode", `{"tool_input":{"file_path":"ok.md"},"cwd":"bad\udcff"}`, false, ""},
		{"empty command everywhere", `{"tool_input":{"command":"","script":"","cmd":""},"command":"","script":""}`, true, ""},
	}
	if len(cases) < 80 {
		t.Fatalf("table shrank to %d cases", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ok := classifyEvent([]byte(tc.event))
			if ok != tc.wantOK || string(out) != tc.out {
				t.Fatalf("classifyEvent(%q) = (%q, %v); want (%q, %v)", tc.event, out, ok, tc.out, tc.wantOK)
			}
		})
	}
}

func TestDetectShellActionsCounts(t *testing.T) {
	cases := []struct {
		cmd  string
		want int
	}{
		{"gh pr create", 1},
		{"gh pr create; gh pr create", 2},
		{"gh pr create && gh pr create || gh pr create", 3},
		{"gh pr merge", 0},
		{"case x in gh) gh pr create;; esac", 1},
		{"case x in *) gh pr create;; esac", 1},
		{"cat <<EOF\ngh pr create\nEOF", 0},
		{"", 0},
	}
	for _, tc := range cases {
		if got := detectShellActions(tc.cmd); len(got) != tc.want {
			t.Fatalf("detectShellActions(%q) = %v; want %d actions", tc.cmd, got, tc.want)
		}
	}
}

func TestPyStripUsesPythonWhitespace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  a  ", "a"},
		{"\t\n\v\f\r a \t\n\v\f\r", "a"},
		{"\x1c\x1d\x1e\x1f a \x1c\x1d\x1e\x1f", "a"},
		{"\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000a", "a"},
		{"", ""},
		{"a b", "a b"},
	}
	for _, tc := range cases {
		if got := pyStrip(tc.in); got != tc.want {
			t.Fatalf("pyStrip(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
	if !pyIsSpace('\u3000') || pyIsSpace('x') {
		t.Fatalf("pyIsSpace did not follow python str.isspace")
	}
}
