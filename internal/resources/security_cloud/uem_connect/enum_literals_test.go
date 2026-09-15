// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// No code changed to add this test, and that is the point. Every literal below
// collides with securitycloud.EmailMappingType, so a scan filtered only by
// package imports flags all thirteen — but the four device-field mapping sets
// are separate server-side enums the spec types as plain strings, and only the
// email mapping has a generated vocabulary. mappings.go already keys the email
// set on the SDK and says why the other four do not; the exemptions here state
// it per value, which is what makes the claim checkable rather than a comment
// asserting something true of some values and false of others.
//
// These are Ignore rather than Absent deliberately: the values are members of a
// different set, not values the SDK has yet to generate, so a change to the
// email vocabulary says nothing about them.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.EmailMappingTypeValues(),
		),
		Ignore: map[string]string{
			"DEVICE_NAME":      "member of the device-name mapping set, a separate server-side enum the spec types as a plain string. securitycloud.EmailMappingType carries this spelling but is the *email* mapping vocabulary",
			"IMEI":             "member of the device-name mapping set; not the email vocabulary",
			"MDM_ID":           "member of the device-name, user-name and user-ID mapping sets; not the email vocabulary",
			"PHONE_NUMBER":     "member of the device-name and phone-number mapping sets; not the email vocabulary",
			"SERIAL_NUMBER":    "member of the device-name and user-name mapping sets; not the email vocabulary",
			"USER_NAME":        "member of the device-name, user-name and user-ID mapping sets; not the email vocabulary",
			"EMAIL_ADDRESS":    "member of the user-name and user-ID mapping sets; the email vocabulary carries the spelling too, but these are different sets",
			"FIRST_LAST_NAME":  "member of the user-name and user-ID mapping sets; not the email vocabulary",
			"NO_CHANGE":        "member of the user-name and user-ID mapping sets; not the email vocabulary",
			"EXTERNAL_USER_ID": "member of the user-ID mapping set; not the email vocabulary",
			"NO_PHONE_NUMBER":  "member of the phone-number mapping set; not the email vocabulary",
		},
	})
	if err != nil {
		t.Fatalf("enumguard.Check: %v", err)
	}
	for _, problem := range got.Problems() {
		t.Error(problem)
	}
	if got.Examined == 0 {
		t.Fatal("no string literals parsed — the guard found nothing to check")
	}
}
