package tracker

import "testing"

// Expected values were produced by running _preprocess_command from the python
// reference in catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh.
func TestPyPreprocessCommand(t *testing.T) {
	tests := []struct {
		cmd, want string
	}{
		{"echo hi # comment", "echo hi "},
		{"cat <<EOF\nbody\nEOF\necho x", "cat <<EOF\necho x"},
		{"cat <<-'EOF'\n\tbody\n\tEOF\necho y", "cat <<-'EOF'\necho y"},
		{"echo a \\\n b", "echo a  b"},
		{"echo \"a # b\"", "echo \"a # b\""},
		{"echo $((1+2)) # c", "echo $((1+2)) "},
		{"cat <<EOF\nx", "cat <<EOF"},
		{"echo x#y", "echo x#y"},
	}
	for _, tt := range tests {
		if got := pyPreprocessCommand(tt.cmd); got != tt.want {
			t.Errorf("pyPreprocessCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}
