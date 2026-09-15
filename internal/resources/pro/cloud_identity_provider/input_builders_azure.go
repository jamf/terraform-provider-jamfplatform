// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// azureConsentCodePlaceholder is sent as the OAuth consent `code` on Azure
// create. The Entra `code` is a single-use artifact obtained interactively in
// the Jamf admin UI; it is not exposed as a Terraform attribute. The server
// REQUIRES `code` to be non-blank (an empty value is rejected with
// 400 INVALID_FIELD "must not be blank"), but it does NOT validate the consent
// at create time — create returns 201 with an inactive connection. The admin
// then completes the manual "refresh consent" step in the Jamf UI to activate
// it. This placeholder satisfies the non-blank requirement.
const azureConsentCodePlaceholder = "terraform-managed"

// buildAzureCreateRequest assembles the Azure Cloud Identity Provider create
// body from the plan. `Code` is the non-blank placeholder (see
// azureConsentCodePlaceholder); `ID` and `Type` are omitempty pointers left
// nil on create.
func buildAzureCreateRequest(plan CloudIdentityProviderResourceModel) *pro.AzureConfigurationRequest {
	az := plan.Azure
	return &pro.AzureConfigurationRequest{
		CloudIDPCommon: pro.CloudIDPCommonRequest{
			DisplayName:  plan.DisplayName.ValueString(),
			ProviderName: wireProviderAzure,
		},
		Server: pro.AzureServerConfigurationRequest{
			Code:                                     azureConsentCodePlaceholder,
			TenantID:                                 az.TenantID.ValueString(),
			SearchTimeout:                            int(az.SearchTimeout.ValueInt64()),
			Enabled:                                  az.Enabled.ValueBool(),
			MembershipCalculationOptimizationEnabled: helpers.OptionalBoolPointer(az.MembershipCalculationOptimizationEnabled),
			TransitiveMembershipEnabled:              az.TransitiveMembershipEnabled.ValueBool(),
			TransitiveMembershipUserField:            az.TransitiveMembershipUserField.ValueString(),
			TransitiveDirectoryMembershipEnabled:     az.TransitiveDirectoryMembershipEnabled.ValueBool(),
			Mappings:                                 buildAzureMappings(az.Mappings, nil),
			// ID and Type are omitempty *string — left nil on create.
		},
	}
}

// buildAzureUpdateRequest assembles the Azure Cloud Identity Provider update
// body from the plan. There is no Code, TenantID, or Type on the update shape.
// Server.ID is the same as the CloudIdP id (the TF state id).
//
// live carries the connection's stored attribute mappings, for the case where
// the configuration declares no mappings block — see buildAzureMappings. The
// caller reads them only in that case, so it is nil whenever the block is
// authored.
func buildAzureUpdateRequest(plan CloudIdentityProviderResourceModel, live *pro.AzureMappings) *pro.AzureConfigurationUpdate {
	az := plan.Azure
	return &pro.AzureConfigurationUpdate{
		CloudIDPCommon: pro.CloudIDPCommon{
			ID:           plan.ID.ValueString(),
			DisplayName:  plan.DisplayName.ValueString(),
			ProviderName: wireProviderAzure,
		},
		Server: pro.AzureServerConfigurationUpdate{
			ID:                                       plan.ID.ValueString(),
			Enabled:                                  az.Enabled.ValueBool(),
			SearchTimeout:                            int(az.SearchTimeout.ValueInt64()),
			MembershipCalculationOptimizationEnabled: helpers.OptionalBoolPointer(az.MembershipCalculationOptimizationEnabled),
			TransitiveMembershipEnabled:              az.TransitiveMembershipEnabled.ValueBool(),
			TransitiveMembershipUserField:            az.TransitiveMembershipUserField.ValueString(),
			TransitiveDirectoryMembershipEnabled:     az.TransitiveDirectoryMembershipEnabled.ValueBool(),
			Mappings:                                 buildAzureMappings(az.Mappings, live),
		},
	}
}

// buildAzureMappings converts the TF mappings model to the SDK struct.
//
// Mappings is a non-pointer field on both request shapes with no omitempty, so
// all eleven keys are always on the wire and there is no way to omit them. Jamf
// Pro stores exactly what it is sent and generates nothing — wire-probed on the
// EU tenant 2026-09-07 and recorded above azureMappingsAreWireEmpty. So an
// undeclared block would send eleven empty strings and wipe whatever mappings
// the connection holds (#404).
//
// live is the answer: when the configuration declares no block, the connection's
// stored mappings go back out unchanged and the omission preserves them, the
// same contract every other undeclared block in the provider keeps. A create
// passes nil because a connection being minted has nothing to preserve, and the
// zero value it sends is what the read path recognises as never-written.
//
// A declared block stays authoritative, down to the field: a leaf the
// configuration omits inside one is written empty and clears that mapping, which
// is the block-versus-field split mobile_device_enrollment_profile documents.
//
// The erasure itself could not be observed on the wire: every PUT to an
// unconsented Entra connection is refused 400 INVALID_CONNECTION before it
// reaches persistence (probed 2026-09-08 — the stored mappings were byte
// identical afterwards), and a consented connection needs a real Entra tenant
// that has completed the interactive consent step in the admin UI. Carrying the
// stored mappings across is correct under both readings: a no-op if the server
// would have preserved them, a rescue if it would not.
func buildAzureMappings(m *cloudAzureMappingsModel, live *pro.AzureMappings) pro.AzureMappings {
	if m == nil {
		if live != nil {
			return *live
		}
		return pro.AzureMappings{}
	}
	return pro.AzureMappings{
		UserID:     m.UserID.ValueString(),
		UserName:   m.UserName.ValueString(),
		RealName:   m.RealName.ValueString(),
		Email:      m.Email.ValueString(),
		Department: m.Department.ValueString(),
		Building:   m.Building.ValueString(),
		Room:       m.Room.ValueString(),
		Phone:      m.Phone.ValueString(),
		Position:   m.Position.ValueString(),
		GroupID:    m.GroupID.ValueString(),
		GroupName:  m.GroupName.ValueString(),
	}
}
