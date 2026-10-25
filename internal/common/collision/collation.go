// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Collation describes how one Jamf object type compares keys for uniqueness.
// The axes vary independently across types, and across the API that performs the
// write, so a single normaliser would either miss collisions Jamf refuses or
// report ones it accepts. IgnoreCase folds upper and lower case together.
// IgnoreTrailingSpace and IgnoreLeadingSpace drop spaces at that end before
// comparing. IgnoreAccents folds accented letters onto their base letter.
type Collation struct {
	IgnoreCase          bool
	IgnoreTrailingSpace bool
	IgnoreLeadingSpace  bool
	IgnoreAccents       bool
}

// CollationFolded ignores case, trailing spaces and accents. Pro v1 categories,
// buildings, departments and scripts use it, as do the Pro v2 and Platform
// mobile device groups.
var CollationFolded = Collation{IgnoreCase: true, IgnoreTrailingSpace: true, IgnoreAccents: true}

// CollationCaseOnly ignores case and nothing else. The Pro v3 and Platform
// computer groups use it: a trailing space, a leading space and an accent each
// make a distinct name.
var CollationCaseOnly = Collation{IgnoreCase: true}

// CollationAccentSensitive ignores case and surrounding spaces but not accents.
// It is the rule of the Classic API, which trims the XML text of every field
// before comparing, so a leading space collides there too. Classic policies use it.
var CollationAccentSensitive = Collation{IgnoreCase: true, IgnoreTrailingSpace: true, IgnoreLeadingSpace: true}

// CollationCaseSensitive ignores surrounding spaces only, the Classic API's rule
// for user groups.
var CollationCaseSensitive = Collation{IgnoreTrailingSpace: true, IgnoreLeadingSpace: true}

// CollationExact compares byte for byte.
var CollationExact = Collation{}

// Fold returns the comparison form of a key under this collation.
func (c Collation) Fold(key string) string {
	if c.IgnoreTrailingSpace {
		key = strings.TrimRight(key, " ")
	}
	if c.IgnoreLeadingSpace {
		key = strings.TrimLeft(key, " ")
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
	case c.IgnoreTrailingSpace && c.IgnoreLeadingSpace:
		ignored = append(ignored, "surrounding spaces")
	case c.IgnoreTrailingSpace:
		ignored = append(ignored, "trailing spaces")
	case c.IgnoreLeadingSpace:
		ignored = append(ignored, "leading spaces")
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
