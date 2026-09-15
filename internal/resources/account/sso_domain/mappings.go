// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"sort"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// Machine-readable error codes Jamf Account returns on the SSO domain
// operations, wire-probed on 2026-09-02. Each is translated into a diagnostic
// attached to the attribute that caused it, because the raw message names the
// code and not the fix.
//
// These four are restated rather than aliased because the Jamf Account SDK
// package generates no error-code vocabulary at all: its ApiErrorItem.Code is a
// plain string, and the six values its doc comment names are prose, with no
// constant and no *Values() helper behind them. That is the genuinely-absent
// exemption in STYLE_GUIDE §"Enum values and error codes come from the SDK, not
// from literals", and enum_literals_test.go records it per value so an SDK
// release that starts generating them fails the test rather than passing
// silently. Note also that two of the four are not in the SDK's documented list
// at all — CONFLICT and FIELD_VALIDATION were observed on the wire and are
// undocumented, so no future alias can be assumed for them either.
const (
	codeConflict        = "CONFLICT"
	codeBadRequest      = "BAD_REQUEST"
	codeFieldValidation = "FIELD_VALIDATION"
	codeNotFound        = "NOT_FOUND"
)

// verificationTXTRecordPrefix is the fixed prefix Jamf's own verification
// instructions pair the verification key with. The complete record value —
// prefix plus key — is what the Jamf Account console presents behind its Copy
// button, and what a DNS provider expects, so the provider exports the assembled
// value alongside the bare key.
const verificationTXTRecordPrefix = "jamf-site-verification="

// verificationStatusUILabels maps each verification status to the label the Jamf
// Account console shows for it, where it shows one.
//
// The status is read-only and its values are kept in Jamf's own spelling rather
// than translated to the console's labels, which diverges from the general rule
// that enum values follow the admin UI. Three reasons: the console's STATUS
// column is a composite that mixes verification state with connection usage
// ("Used in connection"), so it is not a one-to-one relabelling; two of the five
// values have no console label at all, so a translation would have to invent
// them; and the rule exists for values a practitioner writes, which this is not.
// The labels are documented in the attribute description instead.
var verificationStatusUILabels = map[string]string{
	account.DomainStatusVerified:   "Jamf Verified",
	account.DomainStatusMsVerified: "Microsoft Verified",
	account.DomainStatusPending:    "Pending Approval",
}

// verificationStatusDocs renders the verification status vocabulary for the
// attribute description, annotating each value with the Jamf Account console
// label where one exists.
//
// It is built from the SDK's own DomainStatusValues() helper so the documented
// list cannot drift from the vocabulary Jamf declares.
func verificationStatusDocs() string {
	values := account.DomainStatusValues()
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)

	parts := make([]string, 0, len(sorted))
	for _, v := range sorted {
		if label, ok := verificationStatusUILabels[v]; ok {
			parts = append(parts, "`"+v+"` (shown as \""+label+"\")")
			continue
		}
		parts = append(parts, "`"+v+"`")
	}
	return strings.Join(parts, ", ")
}
