package main

import (
	"regexp"
	"strings"
)

var redirectPattern = regexp.MustCompile(`^(?:[0-9]*)(>>?)(.*)$`)

// pass2 (interpreter writers). The reference Bash gate
// (worktree-gate-legacy.sh) uses regex backreferences (\1, \3) to require a
// closing quote to match its opener. Go's RE2 has no backreferences, so each
// family matches with independent delimiter groups and pairing is enforced in
// Go (design decision D10): a mismatched pair must not yield a candidate.
var (
	pyOpenPattern    = regexp.MustCompile(`open\(\s*(["'])(.+?)(["'])\s*,\s*(["'])([^"']*)(["'])`)
	pyPathPattern    = regexp.MustCompile(`Path\(\s*(["'])(.+?)(["'])\s*\)\s*\.write_(?:text|bytes)\(`)
	nodeWritePattern = regexp.MustCompile(`(?:fs\.)?(?:writeFileSync|appendFileSync|writeFile|appendFile|createWriteStream)\(\s*(["'])(.+?)(["'])`)
	rubyWritePattern = regexp.MustCompile(`File\.write\(\s*(["'])(.+?)(["'])`)
	rubyOpenPattern  = regexp.MustCompile(`File\.open\(\s*(["'])(.+?)(["'])\s*,\s*(["'])([^"']*)(["'])`)
)

// init registers the pass2 patterns with --selftest (gateRegexps in main.go)
// so a pattern that fails to compile under the release toolchain is caught
// before the binary is trusted.
func init() {
	for _, p := range []*regexp.Regexp{pyOpenPattern, pyPathPattern, nodeWritePattern, rubyWritePattern, rubyOpenPattern} {
		gateRegexps = append(gateRegexps, p.String())
	}
}

// extractPass2 finds candidate write targets inside interpreter snippets:
// Python open(path, mode) and Path(path).write_text/write_bytes, the Node fs
// writers (writeFileSync, appendFileSync, writeFile, appendFile,
// createWriteStream) and Ruby File.write / File.open(path, mode). It mirrors
// the reference pass2(): the mode families require the mode to contain 'w',
// 'a' or 'x', and every closing delimiter must pair with its opener.
func extractPass2(cmd string) []string {
	var out []string
	for _, m := range pyOpenPattern.FindAllStringSubmatch(cmd, -1) {
		if m[1] != m[3] || m[4] != m[6] {
			continue
		}
		if strings.ContainsAny(m[5], "wax") {
			out = append(out, m[2])
		}
	}
	for _, m := range pyPathPattern.FindAllStringSubmatch(cmd, -1) {
		if m[1] == m[3] {
			out = append(out, m[2])
		}
	}
	for _, m := range nodeWritePattern.FindAllStringSubmatch(cmd, -1) {
		if m[1] == m[3] {
			out = append(out, m[2])
		}
	}
	for _, m := range rubyWritePattern.FindAllStringSubmatch(cmd, -1) {
		if m[1] == m[3] {
			out = append(out, m[2])
		}
	}
	for _, m := range rubyOpenPattern.FindAllStringSubmatch(cmd, -1) {
		if m[1] != m[3] || m[4] != m[6] {
			continue
		}
		if strings.ContainsAny(m[5], "wax") {
			out = append(out, m[2])
		}
	}
	return out
}

func extractPass1(tokens []string) []string {
	out := extractRedirects(tokens)
	for i, t := range tokens {
		if t == "tee" {
			for _, arg := range tokens[i+1:] {
				if arg != "-a" && len(arg) > 0 && arg[0] != '-' {
					out = append(out, arg)
				}
			}
			break
		}
		if t == "sed" || t == "perl" {
			if hasI(tokens[i+1:]) {
				args := nonFlags(tokens[i+1:])
				if len(args) > 0 {
					out = append(out, args[len(args)-1])
				}
			}
			break
		}
		if t == "cp" || t == "mv" {
			args := nonFlags(tokens[i+1:])
			if len(args) > 0 {
				out = append(out, args[len(args)-1])
			}
			break
		}
	}
	return dedupeStrings(out)
}
func hasI(ts []string) bool {
	for _, t := range ts {
		if t == "-i" || len(t) > 2 && t[:2] == "-i" {
			return true
		}
	}
	return false
}
func nonFlags(ts []string) []string {
	var out []string
	for _, t := range ts {
		if len(t) > 0 && t[0] != '-' {
			out = append(out, t)
		}
	}
	return out
}
func extractRedirects(tokens []string) []string {
	var out []string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t == ">" || t == ">>" {
			if i+1 < len(tokens) {
				out = append(out, tokens[i+1])
				i++
			}
			continue
		}
		m := redirectPattern.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		if m[2] == "" && i+1 < len(tokens) {
			out = append(out, tokens[i+1])
			i++
		} else if m[2] != "" && m[2][0] != '&' {
			out = append(out, m[2])
		}
	}
	return out
}

// scrub mirrors the reference dedupe() scrub step (worktree-gate-legacy.sh:90-100):
// a candidate that is empty or whitespace-only, the literal ".", "-", any
// "&"-prefixed token (fd duplication), or the /dev/null | /dev/stdout |
// /dev/stderr | /dev/fd/* special files is not a real write target and is
// dropped. Returns "" for scrubbed values so the dedupe loop discards them.
func scrub(path string) string {
	p := strings.TrimSpace(path)
	if p == "" || p == "." || p == "-" {
		return ""
	}
	if strings.HasPrefix(p, "&") {
		return ""
	}
	if p == "/dev/null" || p == "/dev/stdout" || p == "/dev/stderr" ||
		strings.HasPrefix(p, "/dev/fd/") {
		return ""
	}
	return p
}

// dedupeStrings mirrors the reference dedupe() (worktree-gate-legacy.sh:102-110):
// scrub every candidate first (dropping scrubbed values), then keep the first
// occurrence order.
func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = scrub(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
