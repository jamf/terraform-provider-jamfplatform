// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildConnectorCreateInput builds the create request from the plan, selecting the
// authentication strategy from whichever block is present.
//
// The create body is a vendor-discriminated union, so the return is the envelope
// rather than the variant. `vendor` is set in two places and both are load-bearing:
// the envelope's copy is what the SDK's MarshalJSON dispatches on — set it wrong
// and the whole variant is dropped in favour of a bare `{"vendor":...}` — and the
// variant's own copy is what reaches the server. Both are written from the same
// plan attribute so they cannot disagree.
//
// Only the Jamf Pro variant is ever built, because vendorJamfPro is the only vendor
// this resource accepts. That variant is fully typed upstream, unlike the generic
// one the other nine vendors share, so `authStrategy`, `deviceSyncAuth` and
// `tenantId` are declared fields rather than the wire-restored additions they used
// to be — and `authStrategy` is required, which is why it is no longer a pointer.
//
// writeOnlyClientSecret comes from the config rather than the plan: a WriteOnly
// attribute is null in the plan by design, so the value has to be read from
// req.Config and passed in.
func buildConnectorCreateInput(plan UEMConnectResourceModel, writeOnlyClientSecret string) (*securitycloud.ConnectorCreateRequestBody, diag.Diagnostics) {
	var diags diag.Diagnostics

	vendor := plan.UEMVendor.ValueString()
	jamfPro := &securitycloud.JamfProConnectorCreateRequest{
		Vendor: vendor,
	}
	if !plan.UEMServerURL.IsNull() && !plan.UEMServerURL.IsUnknown() {
		jamfPro.URL = plan.UEMServerURL.ValueString()
	}

	switch {
	case plan.PlatformTenant != nil:
		tenant := plan.PlatformTenant.TenantID.ValueString()
		jamfPro.AuthStrategy = authStrategyPlatformTenant
		jamfPro.TenantID = &tenant
	case plan.OAuth != nil:
		clientID := plan.OAuth.ClientID.ValueString()
		jamfPro.AuthStrategy = authStrategyOAuth
		jamfPro.DeviceSyncAuth = &securitycloud.JamfProCredentials{
			ClientID: &clientID,
		}
		if writeOnlyClientSecret != "" {
			jamfPro.DeviceSyncAuth.ClientSecret = &writeOnlyClientSecret
		}
	default:
		diags.AddError(
			"No authentication method configured",
			"Exactly one of `platform_tenant` or `oauth` must be set. This should have been caught before the "+
				"apply began; please report it.",
		)
	}

	return &securitycloud.ConnectorCreateRequestBody{
		Vendor:  vendor,
		JAMFPRO: jamfPro,
	}, diags
}

// buildSyncSettingsInput builds the sync settings request from the plan.
//
// Every field is sent on every write, always. The settings endpoint is a full
// replacement, so an omitted field is not left alone — it is reset to Jamf's
// default. A sparse body built from "what the user changed" would therefore
// silently revert the rest of the integration's configuration, which is why the
// resource declares each of these Optional+Computed with a default and reads them
// back out of the plan rather than deciding what to include.
//
// `groupSettings` is the one field the server does leave alone when omitted. It is
// still sent unconditionally: relying on the exemption would make the write's
// effect depend on a quirk, and sending it means an emptied `mappings` list
// actually clears the mappings.
//
// The top-level scalars are therefore built explicitly rather than through
// helpers.OptionalStringPointer and friends. Those helpers exist to drop a field
// that is Null or Unknown, which is the right behaviour almost everywhere and the
// wrong behaviour here — a dropped field is a reset. Every one of these attributes
// carries a schema default, so none can be Unknown, and the nested members that
// genuinely are optional do use the helpers.
func buildSyncSettingsInput(plan UEMConnectResourceModel) (*securitycloud.SyncSettings, diag.Diagnostics) {
	var diags diag.Diagnostics

	behaviour, ok := uemAutoDeleteBehaviourToWire[plan.UEMAutoDeleteBehaviour.ValueString()]
	if !ok {
		diags.AddAttributeError(
			path.Root("uem_auto_delete_behavior"),
			"Unrecognised auto-delete behavior",
			"The value "+plan.UEMAutoDeleteBehaviour.ValueString()+" has no Jamf Security Cloud equivalent. This "+
				"should have been caught before the apply began; please report it.",
		)
		return nil, diags
	}

	scheduled := plan.ScheduledSyncEnabled.ValueBool()
	interval := plan.SyncRefreshIntervalMinutes.ValueInt64()
	riskTagging := plan.DeviceRiskUEMSignalingEnabled.ValueBool()
	disableOnAuthError := plan.DisableSyncOnAuthError.ValueBool()
	concurrent := plan.ConcurrentDeviceSyncEnabled.ValueBool()

	input := &securitycloud.SyncSettings{
		Vendor:                 plan.UEMVendor.ValueString(),
		AutoDeviceDeletion:     behaviour,
		Scheduled:              &scheduled,
		RefreshRateMinutes:     &interval,
		DeviceRiskTagging:      &riskTagging,
		DisableSyncOnAuthError: &disableOnAuthError,
		ConcurrentSyncEnabled:  &concurrent,
		DeviceFieldMappings:    buildDeviceFieldMappings(plan.UserDataFieldMapping),
		GroupSettings:          buildGroupSettings(plan.GroupMembershipMapping),
	}

	return input, diags
}

// buildDeviceFieldMappings builds the device field mapping payload.
//
// An omitted block sends an empty object, which the server reads as "all five at
// their defaults" — the same thing the admin UI's "Use default data field
// mapping" checkbox does. An omitted field *within* a configured block behaves the
// same way, so there is nothing to fill in here.
func buildDeviceFieldMappings(model *DataFieldMappingModel) securitycloud.DeviceFieldMappings {
	out := securitycloud.DeviceFieldMappings{}
	if model == nil {
		return out
	}

	if v := helpers.OptionalStringPointer(model.DeviceName); v != nil {
		out.DeviceNameMapping = v
	}
	if v := helpers.OptionalStringPointer(model.UserName); v != nil {
		out.UserNameMapping = v
	}
	if v := helpers.OptionalStringPointer(model.UserID); v != nil {
		out.UserIDMapping = v
	}
	if v := helpers.OptionalStringPointer(model.PhoneNumber); v != nil {
		out.PhoneNumberMapping = v
	}
	if model.Email != nil {
		email := &securitycloud.EmailMapping{
			Type:                  defaultUserEmailMappingType,
			FieldPrefix:           helpers.OptionalStringPointer(model.Email.Prefix),
			FieldSuffix:           helpers.OptionalStringPointer(model.Email.Suffix),
			UseOnlyIfEmailMissing: helpers.OptionalBoolPointer(model.Email.OnlyIfEmailMissing),
		}
		if v := helpers.OptionalStringPointer(model.Email.Source); v != nil {
			email.Type = *v
		}
		out.UserEmailMapping = email
	}

	return out
}

// buildGroupSettings builds the group mapping payload.
//
// An omitted block sends an object with no members, which is not the same as
// omitting the block: within groupSettings the server replaces what it is given,
// so this resets the group configuration to its defaults rather than preserving
// whatever was there. That is the right reading of an absent Terraform block —
// unmanaged means default, not "keep whatever happens to be set".
func buildGroupSettings(model *GroupMappingModel) *securitycloud.GroupSettings {
	out := &securitycloud.GroupSettings{}
	if model == nil {
		return out
	}

	out.GroupMappingEnabled = helpers.OptionalBoolPointer(model.Enabled)
	out.DefaultGroupID = helpers.OptionalStringPointer(model.DefaultSecurityCloudGroupID)

	if model.Mappings != nil {
		entries := make([]securitycloud.GroupMapping, 0, len(model.Mappings))
		for _, m := range model.Mappings {
			entries = append(entries, securitycloud.GroupMapping{
				EmmGroupID:     m.UEMGroupID.ValueString(),
				WanderaGroupID: m.SecurityCloudGroupID.ValueString(),
			})
		}
		out.GroupMappings = &entries
	}

	return out
}
