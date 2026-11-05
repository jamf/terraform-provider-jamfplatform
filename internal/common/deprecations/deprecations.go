// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package deprecations finds the removal markers that schema deprecations leave in Go source.
//
// A deprecated attribute carries one greppable comment naming the earliest date it may be removed
// and what replaces it. This package is the single reader of that comment, shared by
// scripts/deprecationsweep, which reports the markers whose date has passed, and by
// internal/conformance, which asserts every marker in the tree is well formed. The format rules
// live in STYLE_GUIDE.md under "Jamf Pro Terraform schema deprecation".
//
// A marker is a line comment that begins with PLATFORM-DEPRECATED or PRO-DEPRECATED, followed by
// remove-after=YYYY-MM-DD, replaced-by=<attribute or none>, and optionally a dash and a reason. Only
// production sources are read: test files are skipped, because a test may quote a marker while
// describing it. A line that begins like a marker but does not match the whole format is reported
// as a Problem rather than ignored, since a marker nobody can parse is a removal nobody is told
// about.
package deprecations

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DateLayout is the only date form a marker may carry.
const DateLayout = "2006-01-02"

var (
	prefixPattern = regexp.MustCompile(`^//\s*(PLATFORM|PRO)-DEPRECATED\b`)
	markerPattern = regexp.MustCompile(`^//\s*(PLATFORM|PRO)-DEPRECATED remove-after=(\d{4}-\d{2}-\d{2}) replaced-by=(\S+)(?: [—-] (.+))?$`)
)

// Marker is one parsed deprecation marker.
type Marker struct {
	// File is slash-separated and relative to the scanned root's parent, so it reads as a path from
	// the repository root when the root is "internal".
	File        string `json:"file"`
	Line        int    `json:"line"`
	Kind        string `json:"kind"`
	RemoveAfter string `json:"remove_after"`
	ReplacedBy  string `json:"replaced_by"`
	Reason      string `json:"reason,omitempty"`
}

// Problem is a line that begins like a marker but does not parse.
type Problem struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Detail string `json:"detail"`
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Detail)
}

// Parse reads one source line. ok is false when the line is not a marker candidate at all. When it
// is a candidate, either the Marker is returned or detail explains why it is malformed.
func Parse(line string) (m Marker, detail string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if !prefixPattern.MatchString(trimmed) {
		return Marker{}, "", false
	}
	match := markerPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return Marker{}, "marker must read `// <PLATFORM|PRO>-DEPRECATED remove-after=YYYY-MM-DD replaced-by=<attribute|none> [— reason]`", true
	}
	if _, err := time.Parse(DateLayout, match[2]); err != nil {
		return Marker{}, fmt.Sprintf("remove-after %q is not a calendar date", match[2]), true
	}
	return Marker{Kind: match[1], RemoveAfter: match[2], ReplacedBy: match[3], Reason: match[4]}, "", true
}

// Scan walks root for production Go files and returns every marker and every malformed candidate,
// each ordered by file then line. Directories named testdata and this package's own directory are
// skipped.
func Scan(root string) ([]Marker, []Problem, error) {
	var (
		markers  []Marker
		problems []Problem
	)
	base := filepath.Dir(filepath.Clean(root))
	own := filepath.Join(filepath.Clean(root), "common", "deprecations")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || filepath.Clean(path) == own {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, text := range strings.Split(string(body), "\n") {
			m, detail, ok := Parse(text)
			switch {
			case !ok:
			case detail != "":
				problems = append(problems, Problem{File: rel, Line: i + 1, Detail: detail})
			default:
				m.File, m.Line = rel, i+1
				markers = append(markers, m)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(markers, func(i, j int) bool {
		if markers[i].File != markers[j].File {
			return markers[i].File < markers[j].File
		}
		return markers[i].Line < markers[j].Line
	})
	return markers, problems, nil
}

// Expired returns the markers whose remove-after date is today or earlier. The style guide makes a
// field eligible for removal on or after that date, and the deprecation message tells operators the
// same, so the day itself counts.
func Expired(markers []Marker, today time.Time) []Marker {
	cutoff := today.UTC().Format(DateLayout)
	var out []Marker
	for _, m := range markers {
		if m.RemoveAfter <= cutoff {
			out = append(out, m)
		}
	}
	return out
}
