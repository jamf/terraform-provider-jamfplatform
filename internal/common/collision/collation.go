// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Collation describes how one Jamf object type compares keys for uniqueness.
// The three axes vary independently across types, so a single normaliser would
// either miss collisions Jamf refuses or report ones it accepts. IgnoreCase folds
// upper and lower case together. IgnoreTrailingSpace drops trailing spaces before
// comparing. IgnoreLeadingSpace drops leading spaces, which Jamf does only for
// the types that trim a name on save (extension attributes, configuration
// profiles, Jamf Security Cloud device groups); elsewhere a leading space keeps
// two names apart. IgnoreAccents folds accented letters onto their base letter.
type Collation struct {
	IgnoreCase          bool
	IgnoreLeadingSpace  bool
	IgnoreTrailingSpace bool
	IgnoreAccents       bool
}

// CollationFolded ignores case, trailing spaces and accents. Pro v1 categories,
// buildings, departments and scripts use it, as do several Classic types.
var CollationFolded = Collation{IgnoreCase: true, IgnoreTrailingSpace: true, IgnoreAccents: true}

// CollationFoldedTrimmed is CollationFolded for the types that also trim leading
// spaces on save. Pro v1 extension attributes and the Classic configuration
// profiles use it, wire-probed 2026-10-25.
var CollationFoldedTrimmed = Collation{IgnoreCase: true, IgnoreLeadingSpace: true, IgnoreTrailingSpace: true, IgnoreAccents: true}

// CollationAccentSensitive ignores case and trailing spaces but not accents.
// Classic policies, computer groups and mobile device groups use it.
var CollationAccentSensitive = Collation{IgnoreCase: true, IgnoreTrailingSpace: true}

// CollationCaseSensitive ignores trailing spaces only. Classic user groups use it.
var CollationCaseSensitive = Collation{IgnoreTrailingSpace: true}

// CollationExact compares byte for byte.
var CollationExact = Collation{}

// Fold returns the comparison form of a key under this collation.
func (c Collation) Fold(key string) string {
	if c.IgnoreLeadingSpace {
		key = strings.TrimLeft(key, " ")
	}
	if c.IgnoreTrailingSpace {
		key = strings.TrimRight(key, " ")
	}
	if c.IgnoreAccents {
		key = stripAccents(key)
	}
	if c.IgnoreCase {
		key = strings.ToLower(key)
	}
	return key
}

// Equal reports whether two keys collide under this collation.
func (c Collation) Equal(a, b string) bool {
	return c.Fold(a) == c.Fold(b)
}

// Describe renders the comparison rule for a diagnostic, for example "Names are
// compared without regard to case or accents."
func (c Collation) Describe(keyLabel string) string {
	var ignored []string
	if c.IgnoreCase {
		ignored = append(ignored, "case")
	}
	switch {
	case c.IgnoreLeadingSpace && c.IgnoreTrailingSpace:
		ignored = append(ignored, "leading and trailing spaces")
	case c.IgnoreLeadingSpace:
		ignored = append(ignored, "leading spaces")
	case c.IgnoreTrailingSpace:
		ignored = append(ignored, "trailing spaces")
	}
	if c.IgnoreAccents {
		ignored = append(ignored, "accents")
	}
	label := strings.ToUpper(keyLabel[:1]) + keyLabel[1:] + "s"
	if len(ignored) == 0 {
		return label + " are compared exactly."
	}
	return label + " are compared without regard to " + joinAnd(ignored) + "."
}

func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " or " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1]
	}
}

func stripAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}
