// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignEnrollmentProfileResourceModel refreshes a resource model from a GET.
// Optional nested blocks (location / purchasing) are refreshed only when already
// authored (the model pointer is non-nil) so the always-returned server defaults
// don't fabricate blocks the user never declared.
//
// hydrating releases that ownership gate on first-time import, where the model
// arrives with every block nil and would otherwise keep them nil — leaving a
// declared block planning as an addition on the first plan after import. See
// importHydration for the signal.
func assignEnrollmentProfileResourceModel(state *EnrollmentProfileResourceModel, api *proclassic.MobileDeviceEnrollmentProfile, hydrating bool) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.Description = helpers.StringPointerValueOrNull(g.Description)
	state.Invitation = bigIntStringOrNull(g.Invitation)
	state.UUID = stringOrNull(g.UUID)
	if g.Site != nil {
		state.SiteID = helpers.StringValueFromIntPtr(g.Site.ID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	}

	if hydrating || state.Location != nil {
		state.Location = flattenLocationModel(api.Location)
	}
	if hydrating || state.Purchasing != nil {
		state.Purchasing = flattenPurchasingModel(api.Purchasing)
	}
	state.Attachments = flattenAttachments(api.Attachments)
}

// importHydration reports whether this Read is the first one to write state for
// the resource, which is the only time the optional-block ownership gate may be
// released.
//
// stateAbsent covers the identity-only import path (Terraform 1.12+), where the
// framework hands Read no prior state at all. The name check covers
// `terraform import <addr> <id>`, where ImportStatePassthroughID leaves a
// sparse-but-non-null state carrying only the id — so req.State.Raw.IsNull() is
// false and cannot be used on its own. name is schema-Required, so Create and
// Update always populate it before any Read; it can only be null here on a
// first-time hydration. Sample it before assignEnrollmentProfileResourceModel
// runs, which overwrites it from the wire.
func importHydration(stateAbsent bool, name types.String) bool {
	return stateAbsent || name.IsNull()
}

// assignEnrollmentProfileDataSourceModel populates a DS model from a GET. The DS
// always surfaces location / purchasing / attachments (read-only lookup).
func assignEnrollmentProfileDataSourceModel(state *EnrollmentProfileDataSourceModel, api *proclassic.MobileDeviceEnrollmentProfile) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.Description = helpers.StringPointerValueOrNull(g.Description)
	state.Invitation = bigIntStringOrNull(g.Invitation)
	state.UUID = stringOrNull(g.UUID)
	if g.Site != nil {
		state.SiteID = helpers.StringValueFromIntPtr(g.Site.ID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	}
	state.Location = flattenLocationModel(api.Location)
	state.Purchasing = flattenPurchasingModel(api.Purchasing)
	state.Attachments = flattenAttachments(api.Attachments)
}

func flattenLocationModel(api *proclassic.Location) *LocationModel {
	if api == nil {
		return nil
	}
	return &LocationModel{
		Username:     stringOrNull(api.Username),
		RealName:     stringOrNull(firstNonNil(api.RealName, api.Realname)),
		EmailAddress: stringOrNull(api.EmailAddress),
		PhoneNumber:  stringOrNull(firstNonNil(api.PhoneNumber, api.Phone)),
		Department:   stringOrNull(api.Department),
		Building:     stringOrNull(api.Building),
		Room:         stringOrNull(api.Room),
		Position:     stringOrNull(api.Position),
	}
}

func flattenPurchasingModel(api *proclassic.Purchasing) *PurchasingModel {
	if api == nil {
		return nil
	}
	return &PurchasingModel{
		IsPurchased:       helpers.BoolPointerValueOrNull(api.IsPurchased),
		IsLeased:          helpers.BoolPointerValueOrNull(api.IsLeased),
		PONumber:          stringOrNull(api.PoNumber),
		PODate:            stringOrNull(api.PoDate),
		PODateEpoch:       intStringOrNull(api.PoDateEpoch),
		PODateUTC:         stringOrNull(api.PoDateUtc),
		Vendor:            stringOrNull(api.Vendor),
		WarrantyExpires:   stringOrNull(api.WarrantyExpires),
		WarrantyEpoch:     intStringOrNull(api.WarrantyExpiresEpoch),
		WarrantyUTC:       stringOrNull(api.WarrantyExpiresUtc),
		AppleCareID:       stringOrNull(api.ApplecareID),
		LeaseExpires:      stringOrNull(api.LeaseExpires),
		LeaseEpoch:        intStringOrNull(api.LeaseExpiresEpoch),
		LeaseUTC:          stringOrNull(api.LeaseExpiresUtc),
		PurchasePrice:     stringOrNull(api.PurchasePrice),
		LifeExpectancy:    int64ValueOrNull(api.LifeExpectancy),
		PurchasingAccount: stringOrNull(api.PurchasingAccount),
		PurchasingContact: stringOrNull(api.PurchasingContact),
	}
}

func firstNonNil(a, b *string) *string {
	if a != nil && *a != "" {
		return a
	}
	return b
}
