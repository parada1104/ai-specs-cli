package tracker

import "testing"

// Python expected values were produced by running the reference segments()
// implementation in catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh.
func TestClassifySegments(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want [][]string
	}{
		{"plain words", "echo hello world", [][]string{{"echo", "hello", "world"}}},
		{"double quotes", "echo \"a b\" c", [][]string{{"echo", "a b", "c"}}},
		{"single quotes", "echo 'a b' c", [][]string{{"echo", "a b", "c"}}},
		{"empty double quoted", "echo \"\"", [][]string{{"echo", ""}}},
		{"empty single quoted", "echo ''", [][]string{{"echo", ""}}},
		{"escaped quote in double", "echo \"a\\\"b\"", [][]string{{"echo", "a\"b"}}},
		{"escaped backslash in double", "echo \"a\\\\b\"", [][]string{{"echo", "a\\b"}}},
		{"escaped dollar in double", "echo \"a\\$b\"", [][]string{{"echo", "a\\$b"}}},
		{"unknown escape in double", "echo \"a\\xb\"", [][]string{{"echo", "a\\xb"}}},
		{"backslash newline continuation", "echo a\\\nb", [][]string{{"echo", "ab"}}},
		{"heredoc plain", "cat <<EOF\nbody\nEOF\n", [][]string{{"cat", "<<EOF"}}},
		{"heredoc dash", "cat <<-EOF\n\tbody\n\tEOF\n", [][]string{{"cat", "<<-EOF"}}},
		{"heredoc single quoted", "cat <<'EOF'\nbody $x\nEOF\n", [][]string{{"cat", "<<EOF"}}},
		{"heredoc double quoted", "cat <<\"EOF\"\nbody\nEOF\n", [][]string{{"cat", "<<EOF"}}},
		{"heredoc escaped", "cat <<\\EOF\nbody\nEOF\n", [][]string{{"cat", "<<EOF"}}},
		{"heredoc delimiter trailing spaces", "cat <<EOF\nEOF  \nbody\nEOF\n", [][]string{{"cat", "<<EOF"}}},
		{"comment at word start", "echo hi # a comment\n", [][]string{{"echo", "hi"}}},
		{"comment mid word", "echo a#b", [][]string{{"echo", "a#b"}}},
		{"semicolon", "a ; b", [][]string{{"a"}, {"b"}}},
		{"and", "a && b", [][]string{{"a"}, {"b"}}},
		{"or", "a || b", [][]string{{"a"}, {"b"}}},
		{"pipe", "a | b", [][]string{{"a"}, {"b"}}},
		{"pipe amp", "a |& b", [][]string{{"a"}, {"b"}}},
		{"double semicolon", "a ;; b", [][]string{{"a", ";;", "b"}}},
		{"semicolon amp", "a ;& b", [][]string{{"a", ";&", "b"}}},
		{"triple semicolon amp", "a ;;& b", [][]string{{"a", ";;&", "b"}}},
		{"arithmetic dollar double paren", "echo $(( 1 + 2 ))", [][]string{{"echo", "$((", "1", "+", "2", "))"}}},
		{"arithmetic double paren", "(( x++ ))", [][]string{{"((", "x++", "))"}}},
		{"arithmetic dollar bracket", "echo $[ 1 + 2 ]", [][]string{{"echo", "$[", "1", "+", "2", "]"}}},
		{"case patterns", "case x in a) cmd;; esac", [][]string{{"case", "x", "in", "a)", "cmd"}, {"esac"}}},
		{"unterminated double quote", "echo \"a\nb\n", nil},
		{"unterminated single quote", "echo 'a\nb\n", nil},
		{"trailing odd backslash", "echo a\\", [][]string{{"echo", "a"}}},
		{"unicode letters", "echo café ñ", [][]string{{"echo", "café", "ñ"}}},
		{"tabs and CR", "echo\ta\r b", [][]string{{"echo", "a", "b"}}},
		{"multiline", "a\nb\n", [][]string{{"a"}, {"b"}}},
		{"empty", "", nil},
		{"only spaces", "   ", nil},
		{"nested quotes concat", "echo ab\"cd\"ef'gh'", [][]string{{"echo", "abcdefgh"}}},
		{"redirect amp", "cmd 2>&1", [][]string{{"cmd", "2>", "&", "1"}}},
		{"fd redirect", "cmd <in >out", [][]string{{"cmd", "<in", ">out"}}},
		{"comment after quoted empty", "echo \"\" # c\n", [][]string{{"echo", ""}}},
		{"heredoc inside case", "case x in a) cat <<EOF\nhi\nEOF\n;; esac", [][]string{{"case", "x", "in", "a)", "cat", "<<EOF"}, {"esac"}}},
		{"double quote with newline", "echo \"a\nb\" c", [][]string{{"echo", "a ;\nb", "c"}}},
		{"escape then quote", "echo \\\"a", [][]string{{"echo", "\"a"}}},
		{"backtick", "echo `date`", [][]string{{"echo", "`date`"}}},
		{"dollar paren subshell", "echo $(date)", [][]string{{"echo", "$(date)"}}},
		{"hash in quotes", "echo \"a # b\" # c\n", [][]string{{"echo", "a # b"}}},
		{"semicolon newline only", "a;\nb", [][]string{{"a"}, {"b"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySegments(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("classifySegments(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
			for i := range tt.want {
				if len(got[i]) != len(tt.want[i]) {
					t.Fatalf("classifySegments(%q) = %q, want %q", tt.cmd, got, tt.want)
				}
				for j := range tt.want[i] {
					if got[i][j] != tt.want[i][j] {
						t.Fatalf("classifySegments(%q) = %q, want %q", tt.cmd, got, tt.want)
					}
				}
			}
		})
	}
}
