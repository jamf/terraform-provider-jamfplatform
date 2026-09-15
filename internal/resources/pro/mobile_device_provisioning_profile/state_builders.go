// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignProvisioningProfileResourceModel populates a resource model from a GET
// response. state.ID is only overwritten when the API ID is non-nil so a
// transient GET that drops the ID does not clobber the ID persisted from Create.
//
// profile_data is mirrored from the server echo (Jamf returns it byte-identical),
// which keeps drift detection honest without diff suppression.
func assignProvisioningProfileResourceModel(state *ProvisioningProfileResourceModel, p *proclassic.MobileDeviceProvisioningProfile) {
	if p == nil || p.General == nil {
		return
	}
	g := p.General

	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.DisplayName = helpers.StringPointerValueOrNull(g.DisplayName)
	state.UUID = helpers.StringPointerValueOrNull(g.UUID)
	state.CreationDateUTC = helpers.StringPointerValueOrNull(g.CreationDateUtc)
	state.CreationDateEpoch = intMillisStringOrNull(g.CreationDateEpoch)
	state.ExpirationDateUTC = helpers.StringPointerValueOrNull(g.ExpirationDateUtc)
	state.ExpirationDateEpoch = bigIntStringOrNull(g.ExpirationDateEpoch)

	if g.Profile != nil {
		state.ProfileData = helpers.StringPointerValueOrNull(g.Profile.Data)
	}
}

// assignProvisioningProfileDataSourceModel populates a data source model from a
// GET response. Symmetric with the resource assigner; nil API fields do not
// overwrite the caller-supplied selector.
func assignProvisioningProfileDataSourceModel(state *ProvisioningProfileDataSourceModel, p *proclassic.MobileDeviceProvisioningProfile) {
	if p == nil || p.General == nil {
		return
	}
	g := p.General

	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.DisplayName = helpers.StringPointerValueOrNull(g.DisplayName)
	state.UUID = helpers.StringPointerValueOrNull(g.UUID)
	state.CreationDateUTC = helpers.StringPointerValueOrNull(g.CreationDateUtc)
	state.CreationDateEpoch = intMillisStringOrNull(g.CreationDateEpoch)
	state.ExpirationDateUTC = helpers.StringPointerValueOrNull(g.ExpirationDateUtc)
	state.ExpirationDateEpoch = bigIntStringOrNull(g.ExpirationDateEpoch)

	if g.Profile != nil {
		state.ProfileData = helpers.StringPointerValueOrNull(g.Profile.Data)
	}
}
