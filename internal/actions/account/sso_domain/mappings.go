// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"slices"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// Machine-readable error codes this action translates. Wire-probed against the
// production US gateway on 2026-09-02.
//
// Both are literals, and the SDK generates no constant for either: the Jamf Account
// SSO spec declares its error codes as prose inside ApiErrorItem.Code's
// documentation rather than as an enum schema, so the generator emits no type and
// no constants — unlike every namespace whose spec names the enum. Checked value by
// value rather than as a set, and pinned by enum_literals_test.go so an SDK release
// that starts generating them fails a test instead of passing review.
const (
	codeBadRequest = "BAD_REQUEST"
	codeNotFound   = "NOT_FOUND"
	codeConflict   = "CONFLICT"
)

// alreadyVerifiedMarker is the fragment of the refusal description that identifies
// a domain Jamf Account already holds as verified, matched lower-cased.
//
// The refusal is 409 CONFLICT, and CONFLICT is not specific enough to translate on
// alone — a duplicate domain claim uses the same code — so the description
// separates them, as with rateLimitMarker.
//
// Wire-probed 2026-09-02: verifying jamf-test.soundmacguy.org.uk once it had
// verified answered 409 immediately rather than 200 with domainStatus VERIFIED, and
// it answered before the five-minute limit was applied, so the check precedes the
// rate limit.
const alreadyVerifiedMarker = "already verified"

// rateLimitMarker is the fragment of the refusal description that identifies the
// five-minute verification limit, matched lower-cased.
//
// The code alone is not enough to translate on: BAD_REQUEST is also what an
// identifier that is not a decimal number and an invalid domain name come back as,
// and Field is null on all three, so the description is the only thing that
// separates them.
const rateLimitMarker = "once every five minutes"

// outcome is how a verification response is read.
//
// The whole reason it exists: a verification that proved nothing is reported the
// same way as one that succeeded — 200, full Domain body, domainStatus unchanged —
// so the outcome lives in the body and has to be classified rather than inferred
// from the call returning without error.
type outcome int

const (
	// outcomeVerified means Jamf Account now considers ownership proven.
	outcomeVerified outcome = iota
	// outcomeNotVerified means the check ran and ownership is still not proven.
	outcomeNotVerified
	// outcomeUnrecognised means the status was absent, or a value this provider
	// does not know. It is deliberately not folded into either of the others:
	// treating an unknown status as verified would report success on a domain
	// nothing has proven, and treating it as unverified would fail an apply on a
	// status that might well mean success.
	outcomeUnrecognised
)

// verifiedStatuses are the verification statuses that mean ownership is proven.
//
// MANUALLY_VERIFIED and MS_VERIFIED belong here even though this action can never
// produce them: they are set by Jamf staff and by Microsoft's own domain proof
// respectively, and a domain already carrying one is verified whatever a fresh DNS
// check would say. Classifying them as failures would fail an apply on a domain
// that is working.
var verifiedStatuses = []account.DomainStatus{
	account.DomainStatusVerified,
	account.DomainStatusManuallyVerified,
	account.DomainStatusMsVerified,
}

// unverifiedStatuses are the verification statuses that mean ownership is not
// proven. PENDING is what an unresolvable domain keeps reporting, wire-probed.
var unverifiedStatuses = []account.DomainStatus{
	account.DomainStatusPending,
	account.DomainStatusUnverified,
}

// classify reads the verification outcome off a verified domain's status.
//
// A nil status is unrecognised rather than a failure: the field is optional on the
// wire, and a body that omits it says nothing about ownership either way.
//
// mappings_test.go asserts every value account.DomainStatusValues() carries is
// classified, so an SDK release adding one fails a test rather than falling
// silently into the unrecognised branch.
func classify(status *account.DomainStatus) outcome {
	if status == nil {
		return outcomeUnrecognised
	}
	switch {
	case slices.Contains(verifiedStatuses, *status):
		return outcomeVerified
	case slices.Contains(unverifiedStatuses, *status):
		return outcomeNotVerified
	default:
		return outcomeUnrecognised
	}
}

// statusText renders a domain's verification status for a diagnostic, naming an
// absent one rather than rendering an empty string into the message.
func statusText(status *account.DomainStatus) string {
	if status == nil {
		return "not reported"
	}
	if *status == "" {
		return "not reported"
	}
	return *status
}
