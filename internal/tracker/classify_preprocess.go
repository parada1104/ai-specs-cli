package tracker

import "strings"

// pyAt returns -1 past either end; no rune outside the string equals that.
func pyAt(line []rune, i int) rune {
	if i < 0 || i >= len(line) {
		return -1
	}
	return line[i]
}

// pyLineContinuationState mirrors _line_continuation_state.
func pyLineContinuationState(line []rune, single, double, atWordStart bool) (bool, bool, bool, bool) {
	for i := 0; i < len(line); i++ {
		char := line[i]
		if single {
			single = char != '\''
			continue
		}
		if double {
			if char == '\\' && i+1 < len(line) {
				i++
			} else if char == '"' {
				double = false
			}
			continue
		}
		switch {
		case char == '\'':
			single, atWordStart = true, false
		case char == '"':
			double, atWordStart = true, false
		case char == '\\':
			if i+1 < len(line) {
				i++
				atWordStart = false
			}
		case char == '#' && atWordStart:
			return single, double, false, atWordStart
		default:
			atWordStart = strings.ContainsRune(" \t;|&()<>", char)
		}
	}
	return single, double, !single && pyTrailingBackslashes(line)%2 == 1, atWordStart
}

// pyFoldUnquotedHeredocLine mirrors _fold_unquoted_heredoc_line.
func pyFoldUnquotedHeredocLine(line []rune, physicalLines [][]rune, index int) ([]rune, int) {
	for pyTrailingBackslashes(line)%2 == 1 && index+1 < len(physicalLines) {
		line = append(append([]rune{}, line[:len(line)-1]...), physicalLines[index+1]...)
		index++
	}
	return line, index
}

type pyPendingHeredoc struct {
	delimiter string
	stripTabs bool
	quoted    bool
}

// pyHeredocDelimiter mirrors the delimiter scan inside _preprocess_command.
// start points just past the "<<".
func pyHeredocDelimiter(line []rune, start int) (pyPendingHeredoc, bool) {
	entry := pyPendingHeredoc{stripTabs: pyAt(line, start) == '-'}
	if entry.stripTabs {
		start++
	}
	if pyAt(line, start) == '<' {
		return entry, false
	}
	for pyAt(line, start) == ' ' || pyAt(line, start) == '\t' {
		start++
	}
	var delimiter []rune
	for end := start; end < len(line) && !strings.ContainsRune(" \t;|&()<>", line[end]); {
		if line[end] == '\\' && end+1 < len(line) {
			delimiter = append(delimiter, line[end+1])
			entry.quoted = true
			end += 2
		} else if line[end] == '\'' || line[end] == '"' {
			quote := line[end]
			entry.quoted = true
			for end++; end < len(line) && line[end] != quote; end++ {
				if quote == '"' && line[end] == '\\' && end+1 < len(line) &&
					strings.ContainsRune("$`\"\\\n", line[end+1]) {
					delimiter = append(delimiter, line[end+1])
					end++
					continue
				}
				delimiter = append(delimiter, line[end])
			}
			if end < len(line) {
				end++
			}
		} else {
			delimiter = append(delimiter, line[end])
			end++
		}
	}
	entry.delimiter = string(delimiter)
	return entry, len(delimiter) > 0
}

// pyPreprocessCommand mirrors _preprocess_command: it strips shell comments and
// heredoc bodies without interpreting quoted text.
func pyPreprocessCommand(cmd string) string {
	var output []string
	var pending []pyPendingHeredoc
	single, double := false, false
	physicalLines := pySplitLines(cmd)
	index := 0
	for index < len(physicalLines) {
		line := physicalLines[index]
		if len(pending) > 0 {
			entry := pending[0]
			if !entry.quoted {
				line, index = pyFoldUnquotedHeredocLine(line, physicalLines, index)
			}
			candidate := line
			if entry.stripTabs {
				candidate = pyLStripTabs(line)
			}
			if string(candidate) == entry.delimiter {
				pending = pending[1:]
			}
			index++
			continue
		}

		entrySingle, entryDouble := single, double
		foldAtWordStart := !(single || double)
		logicalLine := line
		for {
			nextSingle, nextDouble, joinsNext, nextFoldAtWordStart := pyLineContinuationState(
				physicalLines[index], single, double, foldAtWordStart,
			)
			single, double, foldAtWordStart = nextSingle, nextDouble, nextFoldAtWordStart
			if !joinsNext || index+1 >= len(physicalLines) {
				break
			}
			folded := append([]rune{}, logicalLine[:len(logicalLine)-1]...)
			logicalLine = append(folded, physicalLines[index+1]...)
			index++
		}
		line = logicalLine
		var visible []rune
		single, double = entrySingle, entryDouble
		atWordStart := !(single || double)
		arithmeticDepth, arithmeticBrackets := 0, 0
		for i := 0; i < len(line); i++ {
			char := line[i]
			if single {
				visible = append(visible, char)
				if char == '\'' {
					single = false
				}
				continue
			}
			if double {
				visible = append(visible, char)
				if char == '\\' && i+1 < len(line) {
					visible = append(visible, line[i+1])
					i++
				} else if char == '"' {
					double = false
				}
				continue
			}
			switch {
			case char == '\'':
				single, atWordStart = true, false
				visible = append(visible, char)
			case char == '"':
				double, atWordStart = true, false
				visible = append(visible, char)
			case char == '\\':
				visible = append(visible, char)
				if i+1 < len(line) {
					visible = append(visible, line[i+1])
					i++
				}
				atWordStart = false
			case char == '$' && pyAt(line, i+1) == '(' && pyAt(line, i+2) == '(':
				visible = append(visible, line[i:i+3]...)
				arithmeticDepth++
				i += 2
				atWordStart = false
			case char == '(' && pyAt(line, i+1) == '(' && atWordStart:
				visible = append(visible, line[i:i+2]...)
				arithmeticDepth++
				i++
				atWordStart = false
			case char == '$' && pyAt(line, i+1) == '[':
				visible = append(visible, line[i:i+2]...)
				arithmeticDepth++
				arithmeticBrackets++
				i++
				atWordStart = false
			case char == ']' && arithmeticBrackets > 0:
				visible = append(visible, char)
				arithmeticBrackets--
				arithmeticDepth--
				atWordStart = false
			case char == ')' && pyAt(line, i+1) == ')' && arithmeticDepth > arithmeticBrackets:
				visible = append(visible, line[i:i+2]...)
				arithmeticDepth--
				i++
				atWordStart = false
			case char == '#' && atWordStart:
				i = len(line)
			case char == '<' && pyAt(line, i+1) == '<' && arithmeticDepth == 0:
				if entry, ok := pyHeredocDelimiter(line, i+2); ok {
					pending = append(pending, entry)
				}
				visible = append(visible, '<', '<')
				i++
				atWordStart = true
			default:
				visible = append(visible, char)
				atWordStart = strings.ContainsRune(" \t;|&()<>", char)
			}
		}
		output = append(output, string(visible))
		index++
	}
	return strings.Join(output, "\n")
}

// pyUnterminatedQuoteLine mirrors _unterminated_quote_line.
func pyUnterminatedQuoteLine(prepared string) (int, bool) {
	single, double, escaped := false, false, false
	openedLine, lineNo := 0, 1
	for _, char := range prepared {
		if char == '\n' {
			lineNo++
			escaped = false
			continue
		}
		if single {
			single = char != '\''
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if !double && char == '\'' {
			single = !single
			if single {
				openedLine = lineNo
			}
		} else if !single && char == '"' {
			double = !double
			if double {
				openedLine = lineNo
			}
		}
	}
	return openedLine, single || double
}

func pySplitLines(s string) [][]rune {
	parts := strings.Split(s, "\n")
	lines := make([][]rune, len(parts))
	for i, part := range parts {
		lines[i] = []rune(part)
	}
	return lines
}

func pyLStripTabs(line []rune) []rune {
	for i := 0; i < len(line); i++ {
		if line[i] != '\t' {
			return line[i:]
		}
	}
	return line[:0]
}

func pyTrailingBackslashes(line []rune) int {
	count := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		count++
	}
	return count
}
