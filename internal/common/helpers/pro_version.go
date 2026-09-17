// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// RequireMinJamfProVersion compares the tenant's reported Jamf Pro version against the
// minimum required by a Pro resource. Empty required → no-op. Build suffixes like
// "11.5.0-t1700000000" are stripped before parsing. Returns an error diagnostic on
// mismatch or parse failure.
func RequireMinJamfProVersion(actual, required, resourceType string) diag.Diagnostics {
	var diags diag.Diagnostics
	if required == "" {
		return diags
	}

	actualParsed, err := parseSemverPrefix(actual)
	if err != nil {
		diags.AddError(
			"Unparseable Jamf Pro tenant version",
			fmt.Sprintf("%s requires Jamf Pro >= %s but the tenant reported %q which could not be parsed: %s", resourceType, required, actual, APIErrorDetail(err)),
		)
		return diags
	}

	requiredParsed, err := parseSemverPrefix(required)
	if err != nil {
		diags.AddError(
			"Invalid resource minimum Jamf Pro version",
			fmt.Sprintf("%s declared minJamfProVersion %q which could not be parsed: %s", resourceType, required, APIErrorDetail(err)),
		)
		return diags
	}

	if compareSemver(actualParsed, requiredParsed) < 0 {
		diags.AddError(
			"Jamf Pro tenant version below resource minimum",
			fmt.Sprintf("%s requires Jamf Pro >= %s; tenant reports %s.", resourceType, required, actual),
		)
	}
	return diags
}

// WarnIfBelowProviderFloor returns a warning diagnostic when the tenant version is below
// the provider-wide recommended floor. Returns nil-equivalent (no severity) when at/above.
func WarnIfBelowProviderFloor(actual, floor string) diag.Diagnostic {
	if floor == "" {
		return nil
	}

	actualParsed, err := parseSemverPrefix(actual)
	if err != nil {
		return diag.NewWarningDiagnostic(
			"Unparseable Jamf Pro tenant version",
			fmt.Sprintf("Provider recommends Jamf Pro >= %s but the tenant reported %q which could not be parsed: %s", floor, actual, err),
		)
	}

	floorParsed, err := parseSemverPrefix(floor)
	if err != nil {
		return nil
	}

	if compareSemver(actualParsed, floorParsed) < 0 {
		return diag.NewWarningDiagnostic(
			"Jamf Pro tenant older than provider build target",
			fmt.Sprintf(
				"This provider release was built against the Jamf Pro API as of version %s. The tenant reports %s. Some Pro resources may rely on endpoints or fields that did not exist in the tenant's version and could fail at apply time. Upgrade Jamf Pro or pin an older provider release.",
				floor, actual,
			),
		)
	}
	return nil
}

// AtLeastJamfProVersion reports whether actual is >= min (semver-prefix compare,
// build suffixes stripped). FAIL-OPEN: on either parse failure it returns true.
// Callers use this to gate version-specific WORKAROUNDS for behaviour that exists
// only at/after some Jamf Pro version; the provider's support floor is the modern
// version (ProviderMinJamfProVersion), so an unknown/unparseable tenant version is
// treated as modern and the workaround stays engaged.
func AtLeastJamfProVersion(actual, min string) bool {
	a, err := parseSemverPrefix(actual)
	if err != nil {
		return true
	}
	b, err := parseSemverPrefix(min)
	if err != nil {
		return true
	}
	return compareSemver(a, b) >= 0
}

// JamfProVersionInRange reports whether actual falls in the half-open version
// window [minInclusive, maxExclusive) (semver-prefix compare, build suffixes
// stripped). FAIL-OPEN: on any parse failure it returns true. Callers use this to
// gate a workaround that applies only to a specific version WINDOW — a regression
// introduced at one Jamf Pro version and fixed at a later one — so an
// unknown/unparseable tenant version keeps the workaround engaged, matching
// AtLeastJamfProVersion's fail-open convention. Choosing the fail-open default is
// the caller's responsibility: it is safe only when the workaround's behaviour is
// tolerated across the whole supported range (e.g. sending the numeric id is
// accepted both in the regressed window and after the fix).
func JamfProVersionInRange(actual, minInclusive, maxExclusive string) bool {
	a, err := parseSemverPrefix(actual)
	if err != nil {
		return true
	}
	lo, err := parseSemverPrefix(minInclusive)
	if err != nil {
		return true
	}
	hi, err := parseSemverPrefix(maxExclusive)
	if err != nil {
		return true
	}
	return compareSemver(a, lo) >= 0 && compareSemver(a, hi) < 0
}

// JamfProVersionInRangeStrict reports whether actual falls in the half-open
// version window [minInclusive, maxExclusive), and is the FAIL-CLOSED twin of
// JamfProVersionInRange: any parse failure returns false.
//
// The two exist because the answer is used for opposite purposes and a single
// default cannot serve both. JamfProVersionInRange gates a version-specific
// WORKAROUND, where an unknown tenant version should keep the workaround engaged
// — the workaround's behaviour is tolerated across the whole supported range, so
// staying engaged costs nothing and disengaging breaks a tenant inside the
// window. This function instead gates a REFUSAL: a construct that declines to
// operate on a known-bad version window. A refusal must be certain, because
// blocking on "cannot tell" turns an unparseable version string into an outage
// for every construct that consults it, which is strictly worse than the drift
// the refusal was protecting against.
//
// Pick deliberately. Using this one to gate a workaround silently disables the
// workaround on an unknown version; using the fail-open one to gate a refusal
// blocks every tenant whose version will not parse.
func JamfProVersionInRangeStrict(actual, minInclusive, maxExclusive string) bool {
	a, err := parseSemverPrefix(actual)
	if err != nil {
		return false
	}
	lo, err := parseSemverPrefix(minInclusive)
	if err != nil {
		return false
	}
	hi, err := parseSemverPrefix(maxExclusive)
	if err != nil {
		return false
	}
	return compareSemver(a, lo) >= 0 && compareSemver(a, hi) < 0
}

type semver struct {
	major, minor, patch int
}

func parseSemverPrefix(v string) (semver, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return semver{}, fmt.Errorf("empty version")
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("expected MAJOR.MINOR.PATCH, got %q", v)
	}
	out := semver{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, fmt.Errorf("segment %d (%q) is not numeric", i, p)
		}
		switch i {
		case 0:
			out.major = n
		case 1:
			out.minor = n
		case 2:
			out.patch = n
		}
	}
	return out, nil
}

func compareSemver(a, b semver) int {
	switch {
	case a.major != b.major:
		return a.major - b.major
	case a.minor != b.minor:
		return a.minor - b.minor
	case a.patch != b.patch:
		return a.patch - b.patch
	}
	return 0
}
