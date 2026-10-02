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
//
// Jamf Pro echoes an empty string field as an empty element whether it was
// written as "" or never sent, so description and the location and purchasing
// string leaves are reconciled against the incoming model (the plan on Create
// and Update, prior state on Read): an authored "" survives the read rather
// than collapsing to null, which the post-apply consistency check rejects.
func assignEnrollmentProfileResourceModel(state *EnrollmentProfileResourceModel, api *proclassic.MobileDeviceEnrollmentProfile, hydrating bool) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.Description = helpers.ReconcileOptionalStringPointer(g.Description, state.Description)
	state.Invitation = bigIntStringOrNull(g.Invitation)
	state.UUID = stringOrNull(g.UUID)
	if g.Site != nil {
		state.SiteID = helpers.StringValueFromIntPtr(g.Site.ID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	}

	if hydrating || state.Location != nil {
		state.Location = flattenLocationModel(api.Location, state.Location)
	}
	if hydrating || state.Purchasing != nil {
		state.Purchasing = flattenPurchasingModel(api.Purchasing, state.Purchasing)
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
	state.Location = flattenLocationModel(api.Location, nil)
	state.Purchasing = flattenPurchasingModel(api.Purchasing, nil)
	state.Attachments = flattenAttachments(api.Attachments)
}

// flattenLocationModel maps the wire <location> block onto the model. Each
// string leaf is reconciled against prior so an authored "" survives the read;
// a nil prior (data source, list resource, import) maps an empty value to null.
func flattenLocationModel(api *proclassic.Location, prior *LocationModel) *LocationModel {
	if api == nil {
		return nil
	}
	if prior == nil {
		prior = &LocationModel{}
	}
	return &LocationModel{
		Username:     helpers.ReconcileOptionalStringPointer(api.Username, prior.Username),
		RealName:     helpers.ReconcileOptionalStringPointer(firstNonNil(api.RealName, api.Realname), prior.RealName),
		EmailAddress: helpers.ReconcileOptionalStringPointer(api.EmailAddress, prior.EmailAddress),
		PhoneNumber:  helpers.ReconcileOptionalStringPointer(firstNonNil(api.PhoneNumber, api.Phone), prior.PhoneNumber),
		Department:   helpers.ReconcileOptionalStringPointer(api.Department, prior.Department),
		Building:     helpers.ReconcileOptionalStringPointer(api.Building, prior.Building),
		Room:         helpers.ReconcileOptionalStringPointer(api.Room, prior.Room),
		Position:     helpers.ReconcileOptionalStringPointer(api.Position, prior.Position),
	}
}

// flattenPurchasingModel maps the wire <purchasing> block onto the model. Each
// writable string leaf is reconciled against prior so an authored "" survives
// the read; a nil prior (data source, list resource, import) maps an empty
// value to null.
func flattenPurchasingModel(api *proclassic.Purchasing, prior *PurchasingModel) *PurchasingModel {
	if api == nil {
		return nil
	}
	if prior == nil {
		prior = &PurchasingModel{}
	}
	return &PurchasingModel{
		IsPurchased:       helpers.BoolPointerValueOrNull(api.IsPurchased),
		IsLeased:          helpers.BoolPointerValueOrNull(api.IsLeased),
		PONumber:          helpers.ReconcileOptionalStringPointer(api.PoNumber, prior.PONumber),
		PODate:            helpers.ReconcileOptionalStringPointer(api.PoDate, prior.PODate),
		PODateEpoch:       intStringOrNull(api.PoDateEpoch),
		PODateUTC:         stringOrNull(api.PoDateUtc),
		Vendor:            helpers.ReconcileOptionalStringPointer(api.Vendor, prior.Vendor),
		WarrantyExpires:   helpers.ReconcileOptionalStringPointer(api.WarrantyExpires, prior.WarrantyExpires),
		WarrantyEpoch:     intStringOrNull(api.WarrantyExpiresEpoch),
		WarrantyUTC:       stringOrNull(api.WarrantyExpiresUtc),
		AppleCareID:       helpers.ReconcileOptionalStringPointer(api.ApplecareID, prior.AppleCareID),
		LeaseExpires:      helpers.ReconcileOptionalStringPointer(api.LeaseExpires, prior.LeaseExpires),
		LeaseEpoch:        intStringOrNull(api.LeaseExpiresEpoch),
		LeaseUTC:          stringOrNull(api.LeaseExpiresUtc),
		PurchasePrice:     helpers.ReconcileOptionalStringPointer(api.PurchasePrice, prior.PurchasePrice),
		LifeExpectancy:    int64ValueOrNull(api.LifeExpectancy),
		PurchasingAccount: helpers.ReconcileOptionalStringPointer(api.PurchasingAccount, prior.PurchasingAccount),
		PurchasingContact: helpers.ReconcileOptionalStringPointer(api.PurchasingContact, prior.PurchasingContact),
	}
}

func firstNonNil(a, b *string) *string {
	if a != nil && *a != "" {
		return a
	}
	return b
}
