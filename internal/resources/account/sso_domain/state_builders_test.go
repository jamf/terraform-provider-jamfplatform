// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// numberPtr builds a *json.Number the way the SDK decodes one, from either a
// quoted string or a bare number in the body.
func numberPtr(s string) *json.Number {
	n := json.Number(s)
	return &n
}

// timePtr parses an RFC 3339 timestamp for a fixture.
func timePtr(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}
	return &parsed
}

// verifiedDomain is a claim that has verified and inherits nothing, with every
// optional field populated.
func verifiedDomain(t *testing.T) *account.Domain {
	t.Helper()
	status := account.DomainStatusVerified
	return &account.Domain{
		AccountID:                  "001ABCDEFGHIJKLMNO",
		CreatedByName:              new("Ada Lovelace"),
		CreatedDate:                timePtr(t, "2026-08-01T09:00:00Z"),
		Domain:                     "corp.example",
		DomainStatus:               &status,
		ID:                         numberPtr("26917"),
		LastModifiedDate:           timePtr(t, "2026-08-02T09:00:00Z"),
		LastVerificationDate:       timePtr(t, "2026-08-02T09:00:00Z"),
		SharedDomain:               true,
		VerificationExpirationDate: timePtr(t, "2026-08-16T09:00:00Z"),
		VerificationKey:            "verification-key-verified",
		VerifiedTldID:              numberPtr("26900"),
	}
}

// freshClaim is the shape the claim response takes for a domain that has never
// verified: no user behind it, no verification date, and no parent to inherit
// from.
func freshClaim(t *testing.T) *account.Domain {
	t.Helper()
	status := account.DomainStatusPending
	return &account.Domain{
		AccountID:                  "001ABCDEFGHIJKLMNO",
		CreatedDate:                timePtr(t, "2026-09-02T12:33:32Z"),
		Domain:                     "tf-probe.example",
		DomainStatus:               &status,
		ID:                         numberPtr("26918"),
		LastModifiedDate:           timePtr(t, "2026-09-02T12:33:32Z"),
		SharedDomain:               false,
		VerificationExpirationDate: timePtr(t, "2026-09-16T12:33:32Z"),
		VerificationKey:            "verification-key-fresh-claim",
	}
}

func TestAssignDomainResourceModel_PopulatesEveryField(t *testing.T) {
	var state DomainResourceModel
	assignDomainResourceModel(&state, verifiedDomain(t))

	cases := map[string]struct {
		got  types.String
		want string
	}{
		"id":                      {state.ID, "26917"},
		"domain":                  {state.Domain, "corp.example"},
		"verification_status":     {state.VerificationStatus, account.DomainStatusVerified},
		"verification_key":        {state.VerificationKey, "verification-key-verified"},
		"verification_txt_record": {state.VerificationTXTRecord, "jamf-site-verification=verification-key-verified"},
		"parent_domain_id":        {state.ParentDomainID, "26900"},
		"account_id":              {state.AccountID, "001ABCDEFGHIJKLMNO"},
		"created_by":              {state.CreatedBy, "Ada Lovelace"},
		"created_at":              {state.CreatedAt, "2026-08-01T09:00:00Z"},
		"last_modified_at":        {state.LastModifiedAt, "2026-08-02T09:00:00Z"},
		"last_verified_at":        {state.LastVerifiedAt, "2026-08-02T09:00:00Z"},
		"verification_expires_at": {state.VerificationExpiresAt, "2026-08-16T09:00:00Z"},
	}
	for name, tc := range cases {
		if tc.got.ValueString() != tc.want {
			t.Errorf("%s = %q, want %q", name, tc.got.ValueString(), tc.want)
		}
	}
	if !state.Shared.ValueBool() {
		t.Error("shared = false, want true")
	}
}

// TestAssignDomainResourceModel_NullsTheAbsentOptionals pins the fresh-claim
// shape, which is what Create writes. Three fields are legitimately absent —
// there is no Jamf Account user behind a claim made over this integration, the
// domain has never verified, and it inherits from no parent — and each has to
// land as null rather than as an empty string.
func TestAssignDomainResourceModel_NullsTheAbsentOptionals(t *testing.T) {
	var state DomainResourceModel
	assignDomainResourceModel(&state, freshClaim(t))

	nulls := map[string]types.String{
		"created_by":       state.CreatedBy,
		"last_verified_at": state.LastVerifiedAt,
		"parent_domain_id": state.ParentDomainID,
	}
	for name, value := range nulls {
		if !value.IsNull() {
			t.Errorf("%s = %q, want null", name, value.ValueString())
		}
	}
	if state.VerificationStatus.ValueString() != account.DomainStatusPending {
		t.Errorf("verification_status = %q, want %q", state.VerificationStatus.ValueString(), account.DomainStatusPending)
	}
	if state.Shared.ValueBool() {
		t.Error("shared = true, want false")
	}
}

// TestAssignDomainResourceModel_NormalisesTheTimestampZone pins the timestamp
// form. Jamf sends fractional seconds and a Z offset; state carries RFC 3339 in
// UTC, so a claim read in a different zone cannot produce a diff against one
// created in another.
func TestAssignDomainResourceModel_NormalisesTheTimestampZone(t *testing.T) {
	offset := time.FixedZone("UTC+2", 2*60*60)
	created := time.Date(2026, 9, 2, 14, 33, 32, 658000000, offset)

	domain := freshClaim(t)
	domain.CreatedDate = &created

	var state DomainResourceModel
	assignDomainResourceModel(&state, domain)

	if got := state.CreatedAt.ValueString(); got != "2026-09-02T12:33:32Z" {
		t.Errorf("created_at = %q, want the same instant in UTC", got)
	}
}

func TestAssignDomainDataSourceModel_PopulatesAssignments(t *testing.T) {
	var state DomainDataSourceModel
	diags := assignDomainDataSourceModel(&state, verifiedDomain(t), &account.DomainAllocation{
		Domain: "corp.example",
		Connections: []account.DomainAllocationConnection{
			{AssignedConnection: "con_abc", AssignedConnectionOrgID: "org_abc", Region: account.RegionUs},
			{AssignedConnection: "con_def", AssignedConnectionOrgID: "org_def", Region: account.RegionEu},
		},
		JamfIDEnabled: true,
	})
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.AssignedConnections.IsNull() || len(state.AssignedConnections.Elements()) != 2 {
		t.Fatalf("assigned_connections = %v, want a 2-element list", state.AssignedConnections)
	}
	if !state.JamfIDEnabled.ValueBool() {
		t.Error("jamf_id_enabled = false, want true")
	}
}

// TestAssignDomainDataSourceModel_UnassignedDomainYieldsEmptyList pins the
// verified-but-unused case: an empty assignment set has to be an empty list, not
// a null one, so a configuration counting the connections reads zero rather than
// erroring.
func TestAssignDomainDataSourceModel_UnassignedDomainYieldsEmptyList(t *testing.T) {
	var state DomainDataSourceModel
	diags := assignDomainDataSourceModel(&state, verifiedDomain(t), &account.DomainAllocation{
		Domain:        "corp.example",
		Connections:   nil,
		JamfIDEnabled: true,
	})
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.AssignedConnections.IsNull() {
		t.Error("an unassigned domain must produce an empty assignment list, not a null one")
	}
	if len(state.AssignedConnections.Elements()) != 0 {
		t.Errorf("assigned_connections = %v, want no elements", state.AssignedConnections.Elements())
	}
}

// TestAssignDomainDataSourceModel_NilAllocationNullsTheAssignments covers the
// path where the assignment record could not be established at all.
func TestAssignDomainDataSourceModel_NilAllocationNullsTheAssignments(t *testing.T) {
	var state DomainDataSourceModel
	diags := assignDomainDataSourceModel(&state, verifiedDomain(t), nil)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if !state.AssignedConnections.IsNull() {
		t.Errorf("assigned_connections = %v, want null", state.AssignedConnections)
	}
	if !state.JamfIDEnabled.IsNull() {
		t.Errorf("jamf_id_enabled = %v, want null", state.JamfIDEnabled)
	}
}

func TestBuildDomainsResultModel_PopulatesEveryField(t *testing.T) {
	got := buildDomainsResultModel(*verifiedDomain(t))

	if got.ID.ValueString() != "26917" || got.Domain.ValueString() != "corp.example" {
		t.Errorf("result model lost an identity field: %+v", got)
	}
	if got.VerificationTXTRecord.ValueString() != "jamf-site-verification=verification-key-verified" {
		t.Errorf("verification_txt_record = %q", got.VerificationTXTRecord.ValueString())
	}
	if !got.Shared.ValueBool() {
		t.Error("shared = false, want true")
	}
}
