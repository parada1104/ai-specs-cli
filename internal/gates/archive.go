package gates

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var datedArchiveRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)$`)

// resolveArchive resolves one exact dated archive, or the exact undated legacy
// fallback. Symlinked roots and candidates are rejected outright: is_dir()
// follows symlinks, so a dated archive symlink to content outside the planning
// tree could otherwise have its external files satisfy artifact/verification
// gates.
func resolveArchive(archiveRoot, slug string) (string, []string) {
	missing := fmt.Sprintf("missing archive at openspec/changes/archive/%s/ or a valid dated "+
		"archive named YYYY-MM-DD-%s/", slug, slug)
	var symlinkBlockers []string
	if isSymlink(archiveRoot) {
		symlinkBlockers = append(symlinkBlockers, fmt.Sprintf(
			"archive root %s is a symlink and is rejected — the archive tree must be a real "+
				"directory inside the planning tree", archiveRoot))
	}
	if !isDir(archiveRoot) {
		if len(symlinkBlockers) > 0 {
			return "", symlinkBlockers
		}
		return "", []string{missing}
	}

	var dated, invalidDates, nearMatches []string
	legacy := filepath.Join(archiveRoot, slug)
	if isSymlink(legacy) {
		symlinkBlockers = append(symlinkBlockers, fmt.Sprintf(
			"archive candidate %s is a symlink and is rejected — an archive must be a real "+
				"directory inside the planning tree", legacy))
	}

	entries, err := os.ReadDir(archiveRoot) // ReadDir sorts by name, like sorted(iterdir())
	if err != nil {
		return "", symlinkBlockers
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == slug {
			continue
		}
		candidate := filepath.Join(archiveRoot, name)
		if isSymlink(candidate) {
			if match := datedArchiveRe.FindStringSubmatch(name); match != nil {
				suffix := match[2]
				// Scope dated symlink blockers to the requested slug, mirroring the
				// real-directory candidates: an unrelated dated symlink must not
				// poison resolution for this slug.
				if suffix == slug || strings.HasPrefix(suffix, slug+"-") {
					symlinkBlockers = append(symlinkBlockers, fmt.Sprintf(
						"archive candidate %s is a symlink and is rejected — an archive must "+
							"be a real directory inside the planning tree", candidate))
				}
			}
			continue
		}
		if !isDir(candidate) {
			continue
		}
		match := datedArchiveRe.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		suffix := match[2]
		switch {
		case suffix == slug:
			if validArchiveDate(match[1]) {
				dated = append(dated, candidate)
			} else {
				invalidDates = append(invalidDates, candidate)
			}
		case strings.HasPrefix(suffix, slug+"-"):
			nearMatches = append(nearMatches, candidate)
		}
	}

	sort.Strings(dated)
	var blockers []string
	for _, candidate := range invalidDates {
		blockers = append(blockers, fmt.Sprintf(
			"invalid dated archive candidate %s — date prefix must be a valid ISO calendar date",
			candidate))
	}
	for _, candidate := range nearMatches {
		blockers = append(blockers, fmt.Sprintf(
			"near-match archive candidate %s does not exactly match YYYY-MM-DD-%s",
			candidate, slug))
	}

	legacyIsDir := isDir(legacy)
	if len(dated) > 1 || (len(dated) > 0 && legacyIsDir) {
		candidates := dated
		if legacyIsDir {
			candidates = append(candidates, legacy)
		}
		blockers = append(blockers, fmt.Sprintf("ambiguous archives for slug '%s': %s",
			slug, strings.Join(candidates, ", ")))
	}

	if len(symlinkBlockers) > 0 {
		return "", append(symlinkBlockers, blockers...)
	}
	if len(blockers) > 0 {
		return "", blockers
	}
	if len(dated) == 1 {
		return dated[0], nil
	}
	if legacyIsDir {
		return legacy, nil
	}
	return "", []string{missing}
}

// validArchiveDate is the exact YYYY-MM-DD form plus the calendar date.
func validArchiveDate(value string) bool {
	if !dateRe.MatchString(value) {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
