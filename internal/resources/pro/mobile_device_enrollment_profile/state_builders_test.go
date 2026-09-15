// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func bigInt(t *testing.T, s string) *proclassic.BigInt {
	t.Helper()
	var b proclassic.BigInt
	if !b.SetString(s) {
		t.Fatalf("bad BigInt %q", s)
	}
	return &b
}

func TestAssignResourceModel_FullResponse(t *testing.T) {
	state := EnrollmentProfileResourceModel{
		ID:       types.StringValue("61"),
		Location: &LocationModel{}, // authored
	}
	api := &proclassic.MobileDeviceEnrollmentProfile{
		General: &proclassic.MobileDeviceEnrollmentProfileGeneral{
			ID:          new(61),
			Name:        new("Test"),
			Description: new("descr"),
			Invitation:  bigInt(t, "138277457037961032316766183186860280252"),
			UUID:        new("E7159A5B-5EA4-42A7-A409-F9A4F28DABBC"),
			Site:        &proclassic.SiteObject{ID: new(70), Name: new("platform-test-site")},
		},
		Location: &proclassic.Location{Username: new("alice"), RealName: new("Alice A"), Room: new("R1")},
		Attachments: &proclassic.MobileDeviceEnrollmentProfileAttachments{
			Attachment: &[]proclassic.Attachment{{ID: new(31), Filename: new("Signature.jpg"), URI: new("https://x")}},
		},
	}
	assignEnrollmentProfileResourceModel(&state, api, false)

	if state.Invitation.ValueString() != "138277457037961032316766183186860280252" {
		t.Errorf("invitation = %q", state.Invitation.ValueString())
	}
	if state.UUID.ValueString() != "E7159A5B-5EA4-42A7-A409-F9A4F28DABBC" {
		t.Errorf("uuid = %q", state.UUID.ValueString())
	}
	if state.SiteID.ValueString() != "70" || state.SiteName.ValueString() != "platform-test-site" {
		t.Errorf("site = %q/%q", state.SiteID.ValueString(), state.SiteName.ValueString())
	}
	if state.Location == nil || state.Location.Username.ValueString() != "alice" || state.Location.RealName.ValueString() != "Alice A" {
		t.Errorf("location not refreshed: %+v", state.Location)
	}
	if state.Attachments.IsNull() || len(state.Attachments.Elements()) != 1 {
		t.Errorf("attachments = %v", state.Attachments)
	}
}

func TestAssignResourceModel_DoesNotFabricateOmittedBlocks(t *testing.T) {
	// User omitted location + purchasing (nil models). The server always returns
	// both — assign must NOT fabricate them.
	state := EnrollmentProfileResourceModel{ID: types.StringValue("1")}
	api := &proclassic.MobileDeviceEnrollmentProfile{
		General:    &proclassic.MobileDeviceEnrollmentProfileGeneral{ID: new(1), Name: new("x"), Invitation: bigInt(t, "5")},
		Location:   &proclassic.Location{},
		Purchasing: &proclassic.Purchasing{IsPurchased: new(true), IsLeased: new(false)},
	}
	assignEnrollmentProfileResourceModel(&state, api, false)
	if state.Location != nil {
		t.Errorf("omitted location must stay nil, got %+v", state.Location)
	}
	if state.Purchasing != nil {
		t.Errorf("omitted purchasing must stay nil, got %+v", state.Purchasing)
	}
}

// TestAssignResourceModel_HydratesOnImport covers first-time import, where
// every block arrives nil and name is unset: both blocks must be adopted from
// the wire so a declared block does not plan as an addition on the first plan
// after import.
func TestAssignResourceModel_HydratesOnImport(t *testing.T) {
	state := EnrollmentProfileResourceModel{ID: types.StringValue("1")}
	api := &proclassic.MobileDeviceEnrollmentProfile{
		General:    &proclassic.MobileDeviceEnrollmentProfileGeneral{ID: new(1), Name: new("x"), Invitation: bigInt(t, "5")},
		Location:   &proclassic.Location{Username: new("alice")},
		Purchasing: &proclassic.Purchasing{IsPurchased: new(true), PoNumber: new("PO-1")},
	}
	assignEnrollmentProfileResourceModel(&state, api, importHydration(false, state.Name))
	if state.Location == nil || state.Location.Username.ValueString() != "alice" {
		t.Errorf("location not hydrated on import: %+v", state.Location)
	}
	if state.Purchasing == nil || state.Purchasing.PONumber.ValueString() != "PO-1" {
		t.Errorf("purchasing not hydrated on import: %+v", state.Purchasing)
	}
}

func TestImportHydration(t *testing.T) {
	if !importHydration(true, types.StringValue("managed")) {
		t.Error("identity-only import (no prior state) must hydrate")
	}
	if !importHydration(false, types.StringNull()) {
		t.Error("passthrough import (sparse id-only state) must hydrate")
	}
	if importHydration(false, types.StringValue("managed")) {
		t.Error("a genuine refresh must not hydrate")
	}
}

func TestAssignResourceModel_RealnameFallback(t *testing.T) {
	state := EnrollmentProfileResourceModel{ID: types.StringValue("1"), Location: &LocationModel{}}
	api := &proclassic.MobileDeviceEnrollmentProfile{
		General:  &proclassic.MobileDeviceEnrollmentProfileGeneral{ID: new(1), Name: new("x")},
		Location: &proclassic.Location{Realname: new("Legacy Name"), Phone: new("999")}, // only legacy fields set
	}
	assignEnrollmentProfileResourceModel(&state, api, false)
	if state.Location.RealName.ValueString() != "Legacy Name" {
		t.Errorf("real_name should fall back to legacy realname, got %q", state.Location.RealName.ValueString())
	}
	if state.Location.PhoneNumber.ValueString() != "999" {
		t.Errorf("phone_number should fall back to legacy phone, got %q", state.Location.PhoneNumber.ValueString())
	}
}

func TestFlattenAttachments_Empty(t *testing.T) {
	l := flattenAttachments(nil)
	if l.IsNull() || len(l.Elements()) != 0 {
		t.Errorf("nil attachments must be empty list, got %v", l)
	}
}

func TestBigIntStringOrNull(t *testing.T) {
	if !bigIntStringOrNull(nil).IsNull() {
		t.Error("nil BigInt must be null")
	}
	if !bigIntStringOrNull(bigInt(t, "0")).IsNull() {
		t.Error("zero BigInt must be null")
	}
	if got := bigIntStringOrNull(bigInt(t, "138277457037961032316766183186860280252")).ValueString(); got != "138277457037961032316766183186860280252" {
		t.Errorf("big invitation = %q", got)
	}
}
