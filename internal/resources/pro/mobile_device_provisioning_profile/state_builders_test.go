// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func bigInt(t *testing.T, s string) *proclassic.BigInt {
	t.Helper()
	var b proclassic.BigInt
	if !b.SetString(s) {
		t.Fatalf("bad BigInt fixture %q", s)
	}
	return &b
}

func TestAssignResourceModel_FullResponse(t *testing.T) {
	state := ProvisioningProfileResourceModel{ID: types.StringValue("3")}
	api := &proclassic.MobileDeviceProvisioningProfile{
		General: &proclassic.MobileDeviceProvisioningProfileGeneral{
			ID:                  new(3),
			Name:                new("in-house"),
			DisplayName:         new("In-House Apps"),
			UUID:                new("beeb6fc5-416f-40ba-bb19-b7a1714f8d83"),
			CreationDateEpoch:   new(1749130800000),
			CreationDateUtc:     new("2026-06-05T13:00:00.000+0000"),
			ExpirationDateEpoch: bigInt(t, "1893456000000"),
			ExpirationDateUtc:   new("2030-01-01T00:00:00.000+0000"),
			Profile:             &proclassic.MobileDeviceProvisioningProfileGeneralProfile{Data: new("QkxPQg==")},
		},
	}

	assignProvisioningProfileResourceModel(&state, api)

	checks := map[string]string{
		"name":                  "in-house",
		"display_name":          "In-House Apps",
		"uuid":                  "beeb6fc5-416f-40ba-bb19-b7a1714f8d83",
		"creation_date_epoch":   "1749130800000",
		"expiration_date_epoch": "1893456000000",
		"profile_data":          "QkxPQg==",
	}
	got := map[string]string{
		"name":                  state.Name.ValueString(),
		"display_name":          state.DisplayName.ValueString(),
		"uuid":                  state.UUID.ValueString(),
		"creation_date_epoch":   state.CreationDateEpoch.ValueString(),
		"expiration_date_epoch": state.ExpirationDateEpoch.ValueString(),
		"profile_data":          state.ProfileData.ValueString(),
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("%s: expected %q, got %q", k, want, got[k])
		}
	}
}

func TestAssignResourceModel_EmptyShellNullsDerivedFields(t *testing.T) {
	// Empty-shell GET: uuid/dates empty, profile present but data empty.
	state := ProvisioningProfileResourceModel{ID: types.StringValue("1")}
	api := &proclassic.MobileDeviceProvisioningProfile{
		General: &proclassic.MobileDeviceProvisioningProfileGeneral{
			ID:      new(1),
			Name:    new("shell"),
			UUID:    new(""),
			Profile: &proclassic.MobileDeviceProvisioningProfileGeneralProfile{Data: new("")},
		},
	}
	assignProvisioningProfileResourceModel(&state, api)

	if !state.UUID.IsNull() {
		t.Errorf("empty uuid must map to null, got %q", state.UUID.ValueString())
	}
	if !state.ProfileData.IsNull() {
		t.Errorf("empty profile.data must map to null, got %q", state.ProfileData.ValueString())
	}
	if !state.ExpirationDateEpoch.IsNull() {
		t.Errorf("nil expiration epoch must map to null")
	}
	if !state.CreationDateEpoch.IsNull() {
		t.Errorf("nil creation epoch must map to null")
	}
}

func TestAssignResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := ProvisioningProfileResourceModel{ID: types.StringValue("42")}
	api := &proclassic.MobileDeviceProvisioningProfile{
		General: &proclassic.MobileDeviceProvisioningProfileGeneral{ID: nil, Name: new("x")},
	}
	assignProvisioningProfileResourceModel(&state, api)
	if state.ID.ValueString() != "42" {
		t.Errorf("expected ID preserved, got %q", state.ID.ValueString())
	}
}

func TestAssignResourceModel_NilAPIIsNoop(t *testing.T) {
	state := ProvisioningProfileResourceModel{ID: types.StringValue("7"), Name: types.StringValue("keep")}
	assignProvisioningProfileResourceModel(&state, nil)
	assignProvisioningProfileResourceModel(&state, &proclassic.MobileDeviceProvisioningProfile{General: nil})
	if state.ID.ValueString() != "7" || state.Name.ValueString() != "keep" {
		t.Errorf("expected state unchanged on nil API/General")
	}
}

func TestAssignDataSourceModel_FullResponse(t *testing.T) {
	state := ProvisioningProfileDataSourceModel{}
	api := &proclassic.MobileDeviceProvisioningProfile{
		General: &proclassic.MobileDeviceProvisioningProfileGeneral{
			ID:      new(11),
			Name:    new("looked-up"),
			UUID:    new("U-1"),
			Profile: &proclassic.MobileDeviceProvisioningProfileGeneralProfile{Data: new("REFUQQ==")},
		},
	}
	assignProvisioningProfileDataSourceModel(&state, api)
	if state.ID.ValueString() != "11" || state.Name.ValueString() != "looked-up" || state.UUID.ValueString() != "U-1" {
		t.Errorf("ds assign mismatch: id=%q name=%q uuid=%q", state.ID.ValueString(), state.Name.ValueString(), state.UUID.ValueString())
	}
	if state.ProfileData.ValueString() != "REFUQQ==" {
		t.Errorf("ds profile_data mismatch, got %q", state.ProfileData.ValueString())
	}
}

func TestBigIntStringOrNull(t *testing.T) {
	if !bigIntStringOrNull(nil).IsNull() {
		t.Errorf("nil BigInt must map to null")
	}
	if got := bigIntStringOrNull(bigInt(t, "1893456000000")).ValueString(); got != "1893456000000" {
		t.Errorf("expected decimal string, got %q", got)
	}
}

func TestIntMillisStringOrNull(t *testing.T) {
	if !intMillisStringOrNull(nil).IsNull() {
		t.Errorf("nil int must map to null")
	}
	if !intMillisStringOrNull(new(0)).IsNull() {
		t.Errorf("zero epoch must map to null")
	}
	if got := intMillisStringOrNull(new(1749130800000)).ValueString(); got != "1749130800000" {
		t.Errorf("expected decimal string, got %q", got)
	}
}
