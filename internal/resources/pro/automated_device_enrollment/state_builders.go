// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package automated_device_enrollment

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignAutomatedDeviceEnrollmentResourceModel populates a resource model from
// a `pro.DeviceEnrollmentInstance` GET response. Only overwrites
// `state.ID` when the response carries a non-nil ID — Create's post-upload
// refresh must not clobber the ID already captured from the upload response.
//
// Apple-returned strings (`OrgPhone`, `OrgAddress`, etc.) may contain trailing
// whitespace; the assignment intentionally does not TrimSpace them so plan
// output mirrors the byte-exact server representation.
//
// `TokenFileName` is not echoed by the GET endpoint, so this function does not
// touch `state.TokenFileName` — the plan value (or null) carries through
// unchanged. `ServerToken` is `WriteOnly` and likewise never overwritten from
// the wire response.
func assignAutomatedDeviceEnrollmentResourceModel(state *AutomatedDeviceEnrollmentResourceModel, d *pro.DeviceEnrollmentInstance) {
	if d == nil {
		return
	}
	if d.ID != nil {
		state.ID = types.StringValue(*d.ID)
	}
	state.Name = types.StringValue(d.Name)
	state.SiteID = optionalStringFromPtr(d.SiteID)
	state.SupervisionIdentityID = optionalStringFromPtr(d.SupervisionIdentityID)
	state.AdminID = optionalStringFromPtr(d.AdminID)
	state.OrgName = optionalStringFromPtr(d.OrgName)
	state.OrgEmail = optionalStringFromPtr(d.OrgEmail)
	state.OrgPhone = optionalStringFromPtr(d.OrgPhone)
	state.OrgAddress = optionalStringFromPtr(d.OrgAddress)
	state.ServerName = optionalStringFromPtr(d.ServerName)
	state.ServerUUID = optionalStringFromPtr(d.ServerUUID)
	state.TokenExpirationDate = optionalStringFromPtr(d.TokenExpirationDate)
}

// assignAutomatedDeviceEnrollmentDataSourceModel populates the data source
// model from a `pro.DeviceEnrollmentInstance` GET response. Mirrors the
// resource state builder byte-for-byte: nil `*string` → `types.StringNull()`,
// non-nil → `types.StringValue(*p)`, no TrimSpace so Apple-returned
// whitespace round-trips unchanged. Only sets `ID` when the response carries
// a non-nil ID so a name-based lookup populated `state.ID` ahead of this
// call is preserved if the wire happens to omit it.
func assignAutomatedDeviceEnrollmentDataSourceModel(state *AutomatedDeviceEnrollmentDataSourceModel, d *pro.DeviceEnrollmentInstance) {
	if d == nil {
		return
	}
	if d.ID != nil {
		state.ID = types.StringValue(*d.ID)
	}
	state.Name = types.StringValue(d.Name)
	state.SiteID = optionalStringFromPtr(d.SiteID)
	state.SupervisionIdentityID = optionalStringFromPtr(d.SupervisionIdentityID)
	state.AdminID = optionalStringFromPtr(d.AdminID)
	state.OrgName = optionalStringFromPtr(d.OrgName)
	state.OrgEmail = optionalStringFromPtr(d.OrgEmail)
	state.OrgPhone = optionalStringFromPtr(d.OrgPhone)
	state.OrgAddress = optionalStringFromPtr(d.OrgAddress)
	state.ServerName = optionalStringFromPtr(d.ServerName)
	state.ServerUUID = optionalStringFromPtr(d.ServerUUID)
	state.TokenExpirationDate = optionalStringFromPtr(d.TokenExpirationDate)
}

// optionalStringFromPtr maps a nil `*string` to `types.StringNull()` and a
// non-nil pointer to `types.StringValue(*p)` byte-for-byte. Used for every
// `*string` field on the SDK response so absent wire fields land as null in
// state rather than empty strings.
func optionalStringFromPtr(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}
