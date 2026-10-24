package tracker

import "strings"

// classifySegments mirrors segments() from the python classifier embedded in
// catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh.
//
// Input is decoded to runes, so invalid UTF-8 bytes arrive as U+FFFD and are
// treated as ordinary characters; that matches every decision python makes here.
func classifySegments(cmd string) [][]string {
	prepared := pyPreprocessCommand(cmd)
	if pyTrailingBackslashes([]rune(prepared))%2 == 1 {
		prepared = prepared[:len(prepared)-1]
	}

	tokens, failed := pyShlexTokens(strings.ReplaceAll(prepared, "\n", " ;\n"))
	if failed {
		if errorLine, ok := pyUnterminatedQuoteLine(prepared); ok {
			kept := tokens[:0]
			for _, tok := range tokens {
				if tok.line < errorLine {
					kept = append(kept, tok)
				}
			}
			tokens = kept
		}
	}

	caseContext := false
	for _, tok := range tokens {
		if tok.text == "case" || tok.text == "esac" {
			caseContext = true
			break
		}
	}

	segs := make([][]string, 0)
	cur := make([]string, 0)
	for pos, tok := range tokens {
		previous, following := "", ""
		if pos > 0 {
			previous = tokens[pos-1].text
		}
		if pos+1 < len(tokens) {
			following = tokens[pos+1].text
		}
		if tok.text == "&" && (strings.HasSuffix(previous, "<") || strings.HasSuffix(previous, ">") ||
			strings.HasPrefix(following, ">")) {
			cur = append(cur, tok.text)
			continue
		}
		if pyIsSeparator(tok.text) && (tok.text != ";;" && tok.text != ";&" && tok.text != ";;&" || caseContext) {
			if len(cur) > 0 {
				segs = append(segs, cur)
			}
			cur = make([]string, 0)
			continue
		}
		cur = append(cur, tok.text)
	}
	if len(cur) > 0 {
		segs = append(segs, cur)
	}
	return segs
}

func pyIsSeparator(t string) bool {
	switch t {
	case "|", "||", "&&", ";", "&", "|&", ";;", ";&", ";;&":
		return true
	}
	return false
}

type pyToken struct {
	line int
	text string
}

// pyShlexTokens reproduces the token stream segments() gets from
// shlex(text, posix=True, punctuation_chars=";|&") with commenters="" and
// whitespace_split=True. A token's line is where it starts, which matches
// shlex's lineno-before-next() here because every non-quoted newline is
// preceded by the inserted " ;", so a separator is always emitted before the
// counter moves. The bool reports the ValueError shlex raises at EOF inside a
// quote or after a backslash.
func pyShlexTokens(text string) ([]pyToken, bool) {
	chars := []rune(text)
	tokens := make([]pyToken, 0)
	line := 1
	for i := 0; i < len(chars); {
		char := chars[i]
		if pyIsWhitespace(char) {
			if char == '\n' {
				line++
			}
			i++
			continue
		}
		startLine := line
		var token []rune
		if pyIsPunctuation(char) {
			for i < len(chars) && pyIsPunctuation(chars[i]) {
				token = append(token, chars[i])
				i++
			}
		} else {
			for i < len(chars) {
				char := chars[i]
				if pyIsWhitespace(char) || pyIsPunctuation(char) {
					break
				}
				if char == '\'' || char == '"' {
					quote := char
					i++
					for {
						if i >= len(chars) {
							return tokens, true
						}
						c := chars[i]
						if c == '\n' {
							line++
						}
						if c == quote {
							i++
							break
						}
						if quote == '"' && c == '\\' && i+1 < len(chars) {
							i++
							next := chars[i]
							if next == '\n' {
								line++
							}
							if next != '"' && next != '\\' {
								token = append(token, c)
							}
							token = append(token, next)
							i++
							continue
						}
						token = append(token, c)
						i++
					}
					continue
				}
				if char == '\\' {
					if i+1 >= len(chars) {
						return tokens, true
					}
					i++
					next := chars[i]
					if next == '\n' {
						line++
					}
					token = append(token, next)
					i++
					continue
				}
				token = append(token, char)
				i++
			}
		}
		tokens = append(tokens, pyToken{line: startLine, text: string(token)})
	}
	return tokens, false
}

func pyIsWhitespace(char rune) bool {
	return char == ' ' || char == '\t' || char == '\r' || char == '\n'
}

func pyIsPunctuation(char rune) bool {
	return char == ';' || char == '|' || char == '&'
}
