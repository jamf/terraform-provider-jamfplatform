// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignAzureState folds an Azure Cloud Identity Provider GET response into the
// resource model. There are no WriteOnly fields on Azure, so no prior-value
// threading is required.
//
// `mappings` is Optional and not Computed, so a block the user did not author
// must stay null in state or the apply trips a "planned null, got object"
// consistency error — hence the prior shape is captured before `entra_id` is
// rebuilt. hydrating releases that gate on a first hydration, where the prior is
// always nil; see assignAzureMappingsState.
func assignAzureState(state *CloudIdentityProviderResourceModel, resp *pro.AzureConfiguration, hydrating bool) {
	if resp == nil {
		return
	}

	var priorMappings *cloudAzureMappingsModel
	if state.Azure != nil {
		priorMappings = state.Azure.Mappings
	}

	if resp.CloudIDPCommon != nil {
		if resp.CloudIDPCommon.ID != "" {
			state.ID = types.StringValue(resp.CloudIDPCommon.ID)
		}
		state.DisplayName = types.StringValue(resp.CloudIDPCommon.DisplayName)
		state.ProviderName = types.StringValue(providerNameFromWire(resp.CloudIDPCommon.ProviderName))
	}

	if resp.Server != nil {
		s := resp.Server
		state.Azure = &cloudIdentityProviderAzureModel{
			TenantID:                                 types.StringValue(s.TenantID),
			SearchTimeout:                            types.Int64Value(int64(s.SearchTimeout)),
			Enabled:                                  types.BoolValue(s.Enabled),
			MembershipCalculationOptimizationEnabled: types.BoolValue(s.MembershipCalculationOptimizationEnabled),
			TransitiveMembershipEnabled:              types.BoolValue(s.TransitiveMembershipEnabled),
			TransitiveMembershipUserField:            types.StringValue(s.TransitiveMembershipUserField),
			TransitiveDirectoryMembershipEnabled:     types.BoolValue(s.TransitiveDirectoryMembershipEnabled),
			// Server-derived echoes (Computed-only).
			Type:              types.StringValue(s.Type),
			Migrated:          types.BoolValue(s.Migrated),
			DeprecatedConsent: types.BoolValue(s.DeprecatedConsent),
			Mappings:          assignAzureMappingsState(s.Mappings, priorMappings, hydrating),
		}
	}
}

// assignAzureMappingsState builds the TF mappings model from the SDK response,
// scoped to whether the user authored the block (`prior`). `mappings` is
// Optional (not Computed), so surfacing server-generated mappings the user did
// not configure would trip a "planned null, got object" consistency error.
// hydrating releases that gate on first hydration — import and config
// generation — where prior is always nil and there is no plan to stay
// consistent with (issue #391).
func assignAzureMappingsState(m *pro.AzureMappings, prior *cloudAzureMappingsModel, hydrating bool) *cloudAzureMappingsModel {
	if m == nil || (prior == nil && !hydrating) {
		return nil
	}
	if prior == nil && azureMappingsAreWireEmpty(m) {
		return nil
	}
	return &cloudAzureMappingsModel{
		UserID:     types.StringValue(m.UserID),
		UserName:   types.StringValue(m.UserName),
		RealName:   types.StringValue(m.RealName),
		Email:      types.StringValue(m.Email),
		Department: types.StringValue(m.Department),
		Building:   types.StringValue(m.Building),
		Room:       types.StringValue(m.Room),
		Phone:      types.StringValue(m.Phone),
		Position:   types.StringValue(m.Position),
		GroupID:    types.StringValue(m.GroupID),
		GroupName:  types.StringValue(m.GroupName),
	}
}

// azureMappingsAreWireEmpty reports whether Jamf Pro returned a mappings object
// with nothing in it, which means nobody ever wrote one. `mappings` is a
// non-pointer field on the create request, so a connection made without the
// block still sends eleven empty strings and the server stores and echoes
// exactly that. Wire-probed on the EU test tenant 2026-09-07: a create sending
// all-empty mappings read back all-empty, a create sending populated ones read
// them back verbatim, both on an equally unconsented connection — so the echo
// tracks what was written and Jamf Pro generates no defaults.
//
// Hydration therefore skips an empty object: storing a block of empty strings
// the practitioner never wrote is the same fabrication the collection rule
// forbids. The field-by-field emptiness test is the shape
// disk_encryption_configuration already uses for its recovery key.
func azureMappingsAreWireEmpty(m *pro.AzureMappings) bool {
	return m.UserID == "" &&
		m.UserName == "" &&
		m.RealName == "" &&
		m.Email == "" &&
		m.Department == "" &&
		m.Building == "" &&
		m.Room == "" &&
		m.Phone == "" &&
		m.Position == "" &&
		m.GroupID == "" &&
		m.GroupName == ""
}
