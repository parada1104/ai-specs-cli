package main

import (
	"encoding/json"
	"io"
)

func tokenizeRun(stdin io.Reader, stdout io.Writer) int {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	command := string(data)
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(data, &input) == nil && input.Command != "" {
		command = input.Command
	}
	tokens, ok := splitPOSIX(command)
	if !ok {
		tokens = []string{}
	}
	_ = json.NewEncoder(stdout).Encode(struct {
		Tokens []string `json:"tokens"`
		Error  bool     `json:"error"`
	}{Tokens: tokens, Error: !ok})
	return 0
}

// splitPOSIX replicates python3 shlex.split(cmd, posix=True) token-for-token,
// the exact tokenizer the frozen Bash reference's pass1 runs
// (worktree-gate-legacy.sh:129-133, task 1.18 / design D9). The reference is
// the specification: backslash outside quotes escapes ANY following character,
// backslash inside double quotes is kept literally except before the quote
// itself or another backslash (shlex escapedquotes = '"'), single quotes
// disable all escaping, and '#' is an ordinary character everywhere.
// Unterminated quotes or a trailing backslash yield (nil, false) exactly like
// ValueError, and a whitespace-only input yields ([], true) like shlex.
func splitPOSIX(input string) ([]string, bool) {
	tokens := []string{}
	var current []byte
	state := byte('b') // 'b' outside quotes, 's' single, 'd' double
	escaped := false   // inside double quotes: pending backslash
	flush := func() {
		tokens = append(tokens, string(current))
		current = current[:0]
	}
	started := false
	for i := 0; i < len(input); i++ {
		c := input[i]
		if state == 's' {
			if c == '\'' {
				state = 'b'
			} else {
				current = append(current, c)
			}
			started = true
			continue
		}
		if state == 'd' {
			if escaped {
				// shlex keeps the backslash literally except before the quote
				// itself or another backslash inside double quotes.
				if c == '"' || c == '\\' {
					current = append(current, c)
				} else {
					current = append(current, '\\', c)
				}
				escaped = false
			} else if c == '"' {
				state = 'b'
			} else if c == '\\' {
				escaped = true
			} else {
				current = append(current, c)
			}
			started = true
			continue
		}
		if escaped { // outside quotes: backslash escapes any char
			current = append(current, c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' {
			escaped = true
			started = true
			continue
		}
		if c == '\'' {
			state = 's'
			started = true
			continue
		}
		if c == '"' {
			state = 'd'
			started = true
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if started {
				flush()
				started = false
			}
			continue
		}
		current = append(current, c)
		started = true
	}
	if escaped || state != 'b' {
		return nil, false
	}
	if started {
		flush()
	}
	return tokens, true
}
