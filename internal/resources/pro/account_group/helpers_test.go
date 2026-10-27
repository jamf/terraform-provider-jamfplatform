// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// TestLdapServerIDForWrite pins the clear token: a null ldap_server_id must
// reach the wire as -1 (the only value the endpoint accepts as "none"), a
// configured id is sent as-is, and unknown stays off the wire.
func TestLdapServerIDForWrite(t *testing.T) {
	if got := ldapServerIDForWrite(types.Int64Null()); got == nil || *got != ldapServerNone {
		t.Errorf("null must emit %d, got %v", ldapServerNone, got)
	}
	if got := ldapServerIDForWrite(types.Int64Value(7)); got == nil || *got != 7 {
		t.Errorf("configured id must be sent as-is, got %v", got)
	}
	if got := ldapServerIDForWrite(types.Int64Unknown()); got != nil {
		t.Errorf("unknown must be omitted, got %d", *got)
	}
}

// TestIsPrivilegeSetRefusal pins the match on the Jamf Pro refusal text, on both
// status codes it uses, and that other client errors and plain errors fall through.
func TestIsPrivilegeSetRefusal(t *testing.T) {
	apiErr := func(status int, msg string) error {
		return &jamfplatform.APIResponseError{StatusCode: status, Body: msg}
	}
	const msg = "Only an administrator can assign the AUDITOR privilege level"
	for _, status := range []int{http.StatusConflict, http.StatusForbidden} {
		if !isPrivilegeSetRefusal(apiErr(status, msg)) {
			t.Errorf("status %d with the refusal text must match", status)
		}
	}
	if isPrivilegeSetRefusal(apiErr(http.StatusConflict, "Duplicate name")) {
		t.Error("an unrelated 409 must not match")
	}
	if isPrivilegeSetRefusal(apiErr(http.StatusInternalServerError, msg)) {
		t.Error("a 5xx must not match")
	}
	if isPrivilegeSetRefusal(errors.New(msg)) || isPrivilegeSetRefusal(nil) {
		t.Error("a non-API error or nil must not match")
	}
}
