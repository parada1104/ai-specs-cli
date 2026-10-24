// Package specs composes file-backed change delta specs into the canonical
// openspec/specs/ specs. It is the port of lib/_internal/spec_promotion.py
// (GO-20 U2e, unit U-A) and keeps its behaviour, messages and bytes.
//
// Composition follows the SDD archive contract, per domain:
//
//   - openspec/changes/<slug>/specs/<domain>/spec.md is the delta;
//   - `## ADDED Requirements` appends new requirements;
//   - `## MODIFIED Requirements` replaces the full canonical block with the
//     exact same requirement name;
//   - `## REMOVED Requirements` deletes the matching block, but only through the
//     explicit approval path (allowRemoved);
//   - `## RENAMED Requirements` is unsupported and blocks;
//   - unrelated canonical requirements and document sections are preserved.
//
// The operation is idempotent and resume-safe (an already-composed delta is a
// no-op), writes are atomic (temp file plus rename), nothing is written when any
// domain is blocked, and every destination is checked against the
// repository-bound openspec/specs/ tree, including symlink escapes.
package specs

import (
	"os"
	"regexp"
	"strings"
	"unicode"
)

var (
	requirementHeadingRe    = regexp.MustCompile(`(?m)^###[ \t]+Requirement:[ \t]*(.+?)[ \t]*$`)
	topLevelHeadingRe       = regexp.MustCompile(`(?m)^##[ \t]+`)
	deltaSectionRe          = regexp.MustCompile(`(?mi)^##[ \t]+([a-z][a-z _-]*)[ \t]+requirements[ \t]*$`)
	requirementsSectionRe   = regexp.MustCompile(`(?m)^##[ \t]+Requirements[ \t]*$`)
	trailingSeparatorRe     = regexp.MustCompile(`\n[ \t]*---[ \t]*$`)
	slugRe                  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	deltaOperations         = [...]string{"added", "modified", "removed"}
	requirementBlockPadding = "\n\n"
)

// RequirementBlock is one `### Requirement:` block with its byte offsets in the
// document it was parsed from.
type RequirementBlock struct {
	Name    string
	Content string
	Start   int
	End     int
}

// DeltaSpec is a parsed delta document.
type DeltaSpec struct {
	Added    []RequirementBlock
	Modified []RequirementBlock
	Removed  []RequirementBlock
}

func (d DeltaSpec) blocks(operation string) []RequirementBlock {
	switch operation {
	case "added":
		return d.Added
	case "modified":
		return d.Modified
	default:
		return d.Removed
	}
}

// Composition is the result of composing one delta against canonical text.
type Composition struct {
	Changed  bool
	Content  string
	Applied  []string
	Pending  []string
	Blockers []string
}

// DomainPlan is one delta→canonical pair.
type DomainPlan struct {
	Domain      string
	Source      string
	Canonical   string
	Composition Composition
}

// PromotionReport is the outcome of PromoteChange.
type PromotionReport struct {
	Root     string
	Slug     string
	Folder   string
	Plans    []DomainPlan
	Blockers []string
}

// Changed reports whether any domain would write.
func (r PromotionReport) Changed() bool {
	for _, plan := range r.Plans {
		if plan.Composition.Changed {
			return true
		}
	}
	return false
}

// Pending lists the composed but unapplied items, prefixed by domain.
func (r PromotionReport) Pending() []string {
	return r.domainItems(func(c Composition) []string { return c.Pending })
}

// Applied lists the already-canonical items, prefixed by domain.
func (r PromotionReport) Applied() []string {
	return r.domainItems(func(c Composition) []string { return c.Applied })
}

func (r PromotionReport) domainItems(pick func(Composition) []string) []string {
	items := []string{}
	for _, plan := range r.Plans {
		for _, item := range pick(plan.Composition) {
			items = append(items, plan.Domain+": "+item)
		}
	}
	return items
}

// OK reports a promotion with nothing blocked and nothing left to apply.
func (r PromotionReport) OK() bool { return len(r.Blockers) == 0 && len(r.Pending()) == 0 }

// normalize mirrors Python's universal-newline read plus the module's explicit
// CRLF replacement, and keeps invalid UTF-8 from breaking the byte offsets.
func normalize(markdown string) string {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\r", "\n")
	return strings.ToValidUTF8(markdown, "\uFFFD")
}

func readMarkdown(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return normalize(string(data)), nil
}

func rstrip(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) }
