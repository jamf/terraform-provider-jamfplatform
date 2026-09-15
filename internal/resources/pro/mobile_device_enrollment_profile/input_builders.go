// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildEnrollmentProfileInput converts the plan into an SDK Post payload.
//
// Writes are a MERGE (omit=retain, empty=clear). Plain-Optional string fields are
// therefore ALWAYS emitted (empty when null) so removing a value clears it.
// Optional+Computed fields (site_id, is_purchased, is_leased, life_expectancy)
// are omitted when null/unknown so the server keeps its default/prior value.
// invitation and uuid are server-minted and never sent.
func buildEnrollmentProfileInput(plan EnrollmentProfileResourceModel) *proclassic.MobileDeviceEnrollmentProfilePost {
	general := &proclassic.MobileDeviceEnrollmentProfilePostGeneral{
		Name:        helpers.OptionalStringPointer(plan.Name),
		Description: clearable(plan.Description),
	}
	if site := siteObject(plan.SiteID); site != nil {
		general.Site = site
	}

	post := &proclassic.MobileDeviceEnrollmentProfilePost{General: general}

	if plan.Location != nil {
		post.Location = &proclassic.Location{
			Username:     clearable(plan.Location.Username),
			RealName:     clearable(plan.Location.RealName),
			Realname:     clearable(plan.Location.RealName), // legacy duplicate kept in sync
			EmailAddress: clearable(plan.Location.EmailAddress),
			PhoneNumber:  clearable(plan.Location.PhoneNumber),
			Phone:        clearable(plan.Location.PhoneNumber), // legacy duplicate kept in sync
			Department:   clearable(plan.Location.Department),
			Building:     clearable(plan.Location.Building),
			Room:         clearable(plan.Location.Room),
			Position:     clearable(plan.Location.Position),
		}
	}

	if p := plan.Purchasing; p != nil {
		post.Purchasing = &proclassic.Purchasing{
			IsPurchased:       helpers.OptionalBoolPointer(p.IsPurchased),
			IsLeased:          helpers.OptionalBoolPointer(p.IsLeased),
			PoNumber:          clearable(p.PONumber),
			PoDate:            clearable(p.PODate),
			Vendor:            clearable(p.Vendor),
			WarrantyExpires:   clearable(p.WarrantyExpires),
			ApplecareID:       clearable(p.AppleCareID),
			LeaseExpires:      clearable(p.LeaseExpires),
			PurchasePrice:     clearable(p.PurchasePrice),
			LifeExpectancy:    helpers.OptionalInt64Pointer(p.LifeExpectancy),
			PurchasingAccount: clearable(p.PurchasingAccount),
			PurchasingContact: clearable(p.PurchasingContact),
		}
	}

	return post
}

// clearable returns a pointer suitable for merge writes: nil for Unknown (omit),
// otherwise a pointer to the value (empty string for Null, which clears the
// field under the server's merge semantics).
//
// Boundary: a user who sets a field to an explicit empty string ("") gets the
// same wire send as null, and the server drops the field so GET returns null —
// producing a plan(="")-vs-state(null) inconsistency. This is benign in practice
// (to clear a field, omit it rather than assigning ""), but worth knowing.
func clearable(v types.String) *string {
	if v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// siteObject builds <site><id>N</id></site> from site_id, or nil when the value
// is null/unknown/empty (let the server keep its default).
func siteObject(siteID types.String) *proclassic.SiteObject {
	if siteID.IsNull() || siteID.IsUnknown() || siteID.ValueString() == "" {
		return nil
	}
	n, err := strconv.Atoi(siteID.ValueString())
	if err != nil {
		return nil
	}
	return &proclassic.SiteObject{ID: &n}
}
