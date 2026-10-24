package tracker

// Event half of the python classifier embedded in
// catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh: it turns one
// hook event (the JSON the hook gets on stdin) into the path/shell protocol the
// bash hook reads. Port of the reference's module-level main plus
// command_word/nonflag_args/detect_shell_actions; classifySegments supplies the
// token segments.

import (
	"regexp"
	"strings"
)

// pyIsSpace is python str.isspace(). \s in the reference regexes and the no-arg
// str.strip() share exactly this set.
func pyIsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\x1c', '\x1d', '\x1e', '\x1f',
		'\u0085', '\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000':
		return true
	}
	return r >= '\u2000' && r <= '\u200a'
}

// pyStrip is python str.strip() with no arguments.
func pyStrip(s string) string {
	return strings.TrimFunc(s, pyIsSpace)
}

var pyVarAssignRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

var shellWrappers = map[string]bool{
	"sudo": true, "env": true, "nice": true, "time": true,
	"nohup": true, "xargs": true, "command": true, "coproc": true,
}

// wrapperArgFlags are the wrapper flags that consume the word after them.
var wrapperArgFlags = map[string]bool{
	"-n": true, "--max-args": true, "-I": true, "--replace": true,
	"-P": true, "--max-procs": true, "-d": true, "--delimiter": true,
}

// shellAction is one (action, detail) pair; only pr_create/"" is reachable.
type shellAction struct{ action, detail string }

// ClassifyEvent is the CLI seam for classifyEvent.
func ClassifyEvent(event []byte) ([]byte, bool) { return classifyEvent(event) }

// classifyEvent returns the exact python stdout bytes. ok=false means the
// python exits non-zero (an exception while printing); the verb then prints
// nothing and exits 1.
func classifyEvent(event []byte) ([]byte, bool) {
	value, err := decodePyJSON(event)
	if err != nil {
		return nil, true
	}
	obj, isObject := value.(map[string]any)
	if !isObject {
		return nil, true
	}

	toolInput := map[string]any{}
	if raw := obj["tool_input"]; pyTruthy(raw) {
		if typed, typedOK := raw.(map[string]any); typedOK {
			toolInput = typed
		}
	}
	toolName, _ := obj["tool_name"].(string)
	cwd, _ := obj["cwd"].(string)

	// Python `or` chain: a truthy non-string file_path blocks notebook_path.
	filePath := toolInput["file_path"]
	if !pyTruthy(filePath) {
		filePath = toolInput["notebook_path"]
	}
	if !pyTruthy(filePath) {
		filePath = ""
	}
	if path, isString := filePath.(string); isString && pyStrip(path) != "" {
		if !pyPrintable(toolName) || !pyPrintable(cwd) || !pyPrintable(path) {
			return nil, false
		}
		var b strings.Builder
		b.WriteString("path\t" + toolName + "\t" + cwd + "\n")
		b.WriteString(pyStrip(path) + "\n")
		return []byte(b.String()), true
	}

	command := ""
	found := false
	for _, lookup := range []struct {
		fromToolInput bool
		key           string
	}{
		{true, "command"}, {true, "script"}, {true, "cmd"},
		{false, "command"}, {false, "script"},
	} {
		var value any
		if lookup.fromToolInput {
			value = toolInput[lookup.key]
		} else {
			value = obj[lookup.key]
		}
		if text, isString := value.(string); isString && pyStrip(text) != "" {
			command, found = text, true
			break
		}
	}
	if !found {
		return nil, true
	}

	actions := detectShellActions(command)
	if len(actions) == 0 {
		return nil, true
	}
	// The command itself is never printed, so it needs no printability check.
	if !pyPrintable(toolName) || !pyPrintable(cwd) {
		return nil, false
	}
	name := toolName
	if name == "" {
		name = "Bash"
	}
	var b strings.Builder
	b.WriteString("shell\t" + name + "\t" + cwd + "\n")
	for _, action := range actions {
		b.WriteString(action.action + "\t" + action.detail + "\n")
	}
	return []byte(b.String()), true
}

// detectShellActions mirrors detect_shell_actions.
func detectShellActions(cmd string) []shellAction {
	actions := make([]shellAction, 0, 1)
	caseContext := shellCaseContext(cmd)
	for _, seg := range classifySegments(cmd) {
		if len(seg) == 0 {
			continue
		}
		idx, command, ok := commandWord(seg, caseContext)
		if !ok {
			continue
		}
		body := seg[idx:]
		rest := nonflagArgs(body)
		if command == "gh" && len(rest) >= 2 && rest[0] == "pr" && rest[1] == "create" {
			actions = append(actions, shellAction{action: "pr_create"})
		}
	}
	return actions
}

// shellCaseContext ports re.search(r"(?:^|[;|&()\n])\s*case(?:\s|$)", cmd).
// The `$` alternative is subsumed by \s because \s already contains a newline.
func shellCaseContext(cmd string) bool {
	chars := []rune(cmd)
	starts := make([]int, 0, 4)
	starts = append(starts, 0) // the ^ alternative consumes nothing
	for i, char := range chars {
		if strings.ContainsRune(";|&()\n", char) {
			starts = append(starts, i+1)
		}
	}
	for _, start := range starts {
		i := start
		for i < len(chars) && pyIsSpace(chars[i]) {
			i++
		}
		if i+len("case") > len(chars) || string(chars[i:i+4]) != "case" {
			continue
		}
		j := i + len("case")
		if j == len(chars) || pyIsSpace(chars[j]) {
			return true
		}
	}
	return false
}

// nonflagArgs mirrors nonflag_args.
func nonflagArgs(body []string) []string {
	args := make([]string, 0, len(body))
	for _, token := range body[1:] {
		if !strings.HasPrefix(token, "-") {
			args = append(args, token)
		}
	}
	return args
}

// commandWord mirrors command_word. It returns the index of the command word in
// seg and the word itself (which may be a suffix left after a case pattern).
func commandWord(seg []string, caseContext bool) (int, string, bool) {
	i := 0
	for i < len(seg) {
		t := seg[i]
		if pyVarAssignRE.MatchString(t) {
			i++
			continue
		}
		if shellWrappers[t] {
			i++
			for i < len(seg) && strings.HasPrefix(seg[i], "-") {
				flag := seg[i]
				i++
				if wrapperArgFlags[flag] && i < len(seg) {
					i++
				}
			}
			if t == "coproc" && i+1 < len(seg) &&
				!strings.HasPrefix(seg[i], "-") && (seg[i+1] == "{" || seg[i+1] == "(") {
				i++
			}
			continue
		}
		if t == "case" {
			i++
			for i < len(seg) && seg[i] != "in" {
				i++
			}
			if i < len(seg) {
				i++
				if i < len(seg) {
					pattern := seg[i]
					if at := strings.Index(pattern, ")"); at >= 0 && pattern[at+1:] != "" {
						return i, pattern[at+1:], true
					}
					if pattern != ")" {
						i++
					}
					if i < len(seg) && seg[i] == ")" {
						i++
					}
				}
			}
			continue
		}
		if t == "in" || t == "then" || t == "do" || t == "else" || t == "{" || t == "(" {
			i++
			continue
		}
		if caseContext {
			if _, tail, found := strings.Cut(t, ")"); found {
				if tail == "" {
					i++
					continue
				}
				t = tail
			}
		}
		if strings.HasPrefix(t, "(") && t != "((" {
			t = t[1:]
			if t == "" {
				i++
				continue
			}
		}
		if t == ")" {
			i++
			continue
		}
		if t == "*" && i+1 < len(seg) && seg[i+1] == ")" {
			i += 2
			continue
		}
		if strings.HasPrefix(t, "*") && strings.HasSuffix(t, ")") {
			i++
			continue
		}
		return i, t, true
	}
	return 0, "", false
}
