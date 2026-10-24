package tracker

// Native port of the two lib/_internal/ledger_bridge.py verbs the tracker
// checkpoint host uses (GO-20 U2e, U-C): `change_slug` and `evidence_payload`,
// plus the acquisition helpers they need (`change_dir`, `valid_slug`,
// `tracker_none_reason`). Acquisition only: it reads the planning tree and never
// grades. `recipe_id` and `_git_common_dir` stay in internal/doctor.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const trackerNone = "tracker.none"
const trackerNoneReasonFallback = "tracker.none"

var (
	isoDateRE     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	slugSegmentRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ChangeSlug mirrors ledger_bridge.change_slug: the single active change slug
// under openspec/changes (the archive/ directory excluded), else "".
func ChangeSlug(root string) string {
	dir := filepath.Join(root, "openspec", "changes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() == "archive" {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil && info.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 1 {
		return names[0]
	}
	return ""
}

// validSlug mirrors ledger_bridge.valid_slug: exactly one plain, relative path
// segment.
func validSlug(slug string) bool {
	if slug == "" || slug != strings.TrimSpace(slug) {
		return false
	}
	if slug == "." || slug == ".." {
		return false
	}
	return slugSegmentRE.MatchString(slug)
}

// insideTree mirrors _inside_tree: candidate resolves to a real path strictly
// inside base once symlinks are followed.
func insideTree(base, candidate string) bool {
	baseReal, err := filepath.EvalSymlinks(base)
	if err != nil {
		return false
	}
	candidateReal, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseReal, candidateReal)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// changeDir mirrors ledger_bridge.change_dir: active changes/<slug>, then the
// newest dated archive <ISO-date>-<slug>, then legacy archive/<slug>, then the
// active-shaped fallback. A candidate that exists only as a symlink out of the
// planning tree is refused rather than replaced by another branch.
func changeDir(root, slug string) (string, bool) {
	if !validSlug(slug) {
		return "", false
	}
	changes := filepath.Join(root, "openspec", "changes")
	refused := false
	active := filepath.Join(changes, slug)
	if isDir(active) {
		if insideTree(changes, active) {
			return active, true
		}
		refused = true
	}

	archive := filepath.Join(changes, "archive")
	dated := []string{}
	if entries, err := os.ReadDir(archive); err == nil {
		suffix := "-" + slug
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, suffix) {
				continue
			}
			datePrefix := strings.TrimSuffix(name, suffix)
			if !isoDateRE.MatchString(datePrefix) {
				continue
			}
			if _, err := time.Parse("2006-01-02", datePrefix); err != nil {
				continue
			}
			path := filepath.Join(archive, name)
			if !isDir(path) {
				continue
			}
			if !insideTree(changes, path) {
				refused = true
				continue
			}
			dated = append(dated, path)
		}
	}
	if len(dated) > 0 {
		sort.Strings(dated)
		return dated[len(dated)-1], true
	}

	legacy := filepath.Join(archive, slug)
	if isDir(legacy) {
		if insideTree(changes, legacy) {
			return legacy, true
		}
		refused = true
	}
	if refused {
		return "", false
	}
	return active, true
}

// trackerNoneReason mirrors ledger_bridge.tracker_none_reason: the persisted
// exemption reason, present=false when the file is absent, and an error for an
// undecodable file. It never creates or deletes tracker.none.
func trackerNoneReason(root, slug string) (string, bool, error) {
	folder, ok := changeDir(root, slug)
	if !ok || !isDir(folder) {
		return "", false, nil
	}
	raw, err := os.ReadFile(filepath.Join(folder, trackerNone))
	if err != nil {
		return "", false, nil
	}
	if !utf8.Valid(raw) {
		return "", false, fmt.Errorf("%s: invalid UTF-8", filepath.Join(folder, trackerNone))
	}
	for _, line := range splitLines(string(raw)) {
		if stripped := strings.TrimSpace(line); stripped != "" {
			return stripped, true, nil
		}
	}
	return trackerNoneReasonFallback, true, nil
}

// Evidence is the --evidence JSON object the ledger grades. The field order and
// JSON bytes match Python's json.dumps for the same values.
type Evidence struct {
	Local  string
	Remote string
	Code   string
	Git    string
}

// JSON renders the payload byte-for-byte like Python's json.dumps (default
// separators, ensure_ascii).
func (e Evidence) JSON() []byte {
	return []byte(`{"local": ` + jsonString(e.Local) +
		`, "remote": ` + jsonString(e.Remote) +
		`, "code": ` + jsonString(e.Code) +
		`, "git": ` + jsonString(e.Git) + `}`)
}

// jsonString mirrors Python json.dumps' string escaping: control characters,
// quote and backslash escaped, non-ASCII as \uXXXX (surrogate pairs above the
// BMP), and HTML characters left alone.
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			switch {
			case r < 0x20 || r == 0x7f:
				fmt.Fprintf(&b, `\u%04x`, r)
			case r < 0x7f:
				b.WriteRune(r)
			case r <= 0xffff:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// EvidencePayload mirrors ledger_bridge.evidence_payload: local and remote stay
// empty, code is the change's card_id (blank under tracker.none) and git is that
// same id only when a pr: is recorded. Parse and decode errors propagate.
func EvidencePayload(root, slug string) (Evidence, error) {
	empty := Evidence{}
	if slug == "" {
		return empty, nil
	}
	if _, present, err := trackerNoneReason(root, slug); err != nil {
		return empty, err
	} else if present {
		return empty, nil
	}
	folder, ok := changeDir(root, slug)
	if !ok {
		return empty, nil
	}
	fields, err := ParseSection([]string{
		filepath.Join(folder, "proposal.md"),
		filepath.Join(folder, "tasks.md"),
	})
	if err != nil {
		return empty, err
	}
	card := strings.TrimSpace(fields["card_id"])
	if card == "" {
		return empty, nil
	}
	git := ""
	if strings.TrimSpace(fields["pr"]) != "" {
		git = card
	}
	return Evidence{Code: card, Git: git}, nil
}
