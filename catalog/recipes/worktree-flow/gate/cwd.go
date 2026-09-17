package main

import "path/filepath"

// cwdSource names how a write candidate's resolution base was obtained.
type cwdSource string

const (
	cwdSourceCommand cwdSource = "command"
	cwdSourceEvent   cwdSource = "event"
	cwdSourceNone    cwdSource = "none"
)

// segment is one pipeline/list piece plus the operator that preceded it.
type segment struct {
	tokens []string
	sep    string // "", "&&", "||", "|", ";"
}

type segmentCwd struct {
	Base   string
	Source cwdSource
}

func isShellSep(t string) bool {
	return t == "|" || t == "||" || t == "&&" || t == ";"
}

// splitSegmentsWithSep keeps the operator that preceded each segment so
// sequential cd (&& / ;) can be distinguished from a pipeline-local cd.
func splitSegmentsWithSep(tokens []string) []segment {
	if len(tokens) == 0 {
		return nil
	}
	var out []segment
	pred := ""
	start := 0
	for i, t := range tokens {
		if !isShellSep(t) {
			continue
		}
		var piece []string
		if i > start {
			piece = append([]string(nil), tokens[start:i]...)
		}
		out = append(out, segment{tokens: piece, sep: pred})
		pred = t
		start = i + 1
	}
	var tail []string
	if start < len(tokens) {
		tail = append([]string(nil), tokens[start:]...)
	}
	out = append(out, segment{tokens: tail, sep: pred})
	return out
}

func staticDirOperand(op string) bool {
	if op == "" || op == "-" {
		return false
	}
	if op[0] == '-' && isAllDigits(op[1:]) {
		return false
	}
	if op[0] == '~' {
		return false
	}
	if containsAnyByte(op, '$', '`') {
		return false
	}
	return true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func containsAnyByte(s string, chars ...byte) bool {
	for i := 0; i < len(s); i++ {
		for _, c := range chars {
			if s[i] == c {
				return true
			}
		}
	}
	return false
}

func resolveDir(operand, base string) (string, bool) {
	if !staticDirOperand(operand) {
		return "", false
	}
	abs := operand
	if !filepath.IsAbs(operand) {
		if base == "" {
			return "", false
		}
		abs = filepath.Join(base, operand)
	}
	if !IsExistingDirectory(abs) {
		return "", false
	}
	return abs, true
}

func effectiveBase(c WriteCandidate) (base string, degrade bool) {
	if filepath.IsAbs(c.Path) {
		return "", false
	}
	switch c.Source {
	case cwdSourceCommand, cwdSourceEvent:
		return c.Base, false
	default:
		return "", true
	}
}

func followingSep(segs []segment, i int) string {
	if i+1 < len(segs) {
		return segs[i+1].sep
	}
	return ""
}

func isPipelineSep(sep string) bool {
	return sep == "|" || sep == "||"
}

func recoverCwdWalk(segs []segment, eventCwd string, trusted bool) (overlays []segmentCwd, seqBase string, seqSrc cwdSource) {
	overlays = make([]segmentCwd, len(segs))
	if trusted {
		seqBase = eventCwd
		seqSrc = cwdSourceEvent
	} else {
		seqBase = ""
		seqSrc = cwdSourceNone
	}
	for i, seg := range segs {
		base := seqBase
		src := seqSrc
		if gitBase, gitSrc, gitHit := gitCOverlay(seg.tokens, seqBase, seqSrc); gitHit {
			base = gitBase
			src = gitSrc
		}
		overlays[i] = segmentCwd{Base: base, Source: src}

		cdHit, cdOK, cdDir := parseCd(seg.tokens)
		if !cdHit {
			continue
		}
		follow := followingSep(segs, i)
		inPipeline := isPipelineSep(seg.sep) || isPipelineSep(follow)
		if inPipeline {
			// Pipeline-local cd does not update sequential S.
			if !cdOK {
				overlays[i] = segmentCwd{Base: "", Source: cwdSourceNone}
			}
			continue
		}
		if !cdOK {
			seqBase = ""
			seqSrc = cwdSourceNone
			continue
		}
		resolved, ok := resolveDir(cdDir, seqBase)
		if !ok {
			seqBase = ""
			seqSrc = cwdSourceNone
			continue
		}
		seqBase = resolved
		seqSrc = cwdSourceCommand
	}
	return overlays, seqBase, seqSrc
}

func gitCOverlay(tokens []string, seqBase string, seqSrc cwdSource) (string, cwdSource, bool) {
	if len(tokens) == 0 || !isGitWord(tokens[0]) {
		return "", cwdSourceNone, false
	}
	overlay := seqBase
	src := seqSrc
	sawC := false
	failed := false
	for i := 1; i < len(tokens); i++ {
		t := tokens[i]
		if t == "--" {
			break
		}
		dir := ""
		switch {
		case t == "-C":
			if i+1 >= len(tokens) {
				failed = true
				break
			}
			dir = tokens[i+1]
			i++
		case len(t) > 2 && t[:2] == "-C":
			dir = t[2:]
		default:
			if len(t) > 0 && t[0] == '-' {
				continue
			}
			// First non-flag git subcommand; stop scanning.
			i = len(tokens)
			continue
		}
		if failed {
			break
		}
		resolved, ok := resolveDir(dir, overlay)
		if !ok {
			failed = true
			break
		}
		overlay = resolved
		src = cwdSourceCommand
		sawC = true
	}
	if failed {
		return "", cwdSourceNone, true
	}
	if !sawC {
		return "", cwdSourceNone, false
	}
	return overlay, src, true
}

func isGitWord(t string) bool {
	if t == "git" {
		return true
	}
	return len(t) >= 4 && (t[len(t)-4:] == "/git" || filepath.Base(t) == "git")
}

func parseCd(tokens []string) (hit bool, ok bool, dir string) {
	if len(tokens) == 0 {
		return false, false, ""
	}
	if looksLikeSubshell(tokens) {
		if hasCdWord(tokens) {
			return true, false, ""
		}
		return false, false, ""
	}
	if tokens[0] != "cd" {
		return false, false, ""
	}
	i := 1
	for i < len(tokens) {
		t := tokens[i]
		if t == "-L" || t == "-P" || t == "--" {
			i++
			continue
		}
		break
	}
	rest := tokens[i:]
	if len(rest) != 1 {
		return true, false, ""
	}
	op := rest[0]
	if !staticDirOperand(op) {
		return true, false, ""
	}
	return true, true, op
}

func looksLikeSubshell(tokens []string) bool {
	for _, t := range tokens {
		if t == "(" || t == ")" || hasByte(t, '(') {
			return true
		}
	}
	return false
}

func hasCdWord(tokens []string) bool {
	for _, t := range tokens {
		if t == "cd" || len(t) >= 3 && (t[:3] == "cd" || t[len(t)-2:] == "cd") {
			return true
		}
		if t == "(cd" || len(t) >= 3 && t[:3] == "(cd" {
			return true
		}
	}
	return false
}

func hasByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}
