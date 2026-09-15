// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignUEMConnectResourceModel maps the stored integration onto the resource
// model, mutating in place so attributes the read cannot speak to are preserved.
//
// Two are preserved that way. `oauth.client_secret` is write-only and never
// returned, so it stays null in state; `oauth.client_secret_wo_version` is the
// user's own rotation counter and the server has never seen it.
//
// # How the authentication form is recovered
//
// The stored authStrategy is no help — it reads JAMF_PRO_OAUTH whichever form
// created the integration. `tenantId` is the discriminator instead: it is
// populated for an integration created by naming a platform tenant and null for
// one created from supplied credentials (wire-verified 2026-08-28, both forms).
// This is what makes import work without the user telling us which form they used.
//
// A platform-tenant integration also reports a clientId — the one Jamf Security
// Cloud provisioned for itself — so the presence of credentials in the response
// cannot be used as the signal, and `oauth` is deliberately left null there. Those
// credentials are not the user's to manage.
//
// # Why the server address is dropped on the platform_tenant form
//
// Jamf Security Cloud reports the resolved address for both forms, and on this one
// `uem_server_url` cannot be written in configuration at all: the resource refuses
// the pair (resource.go's Conflicting validator), because the tenant resolves the
// address. Committing what the server resolved would make the resource contradict
// its own rule, and the way that surfaces is
// `terraform plan -generate-config-out`, which writes state back out as
// configuration — a generated file carrying both the tenant and the address cannot
// be planned. Create already nulls it for the same reason
// (nullUnknownReadBackValues), so this is what keeps a refresh from undoing that.
// The data source keeps reporting the real address: it owns no configuration to
// contradict and the resolved address is worth knowing.
//
// # Why the optional blocks are gated
//
// `user_data_field_mapping` and `group_membership_mapping` are Optional-only, and Jamf Security
// Cloud returns both populated on every read whether or not the user manages them.
// Populating them unconditionally therefore breaks the framework's consistency
// contract wherever the plan said null — including the case that is easy to miss,
// an Update that *removes* a previously-managed block. So population is gated on
// the pointer in the model being written, captured before anything reassigns it,
// per STYLE_GUIDE §SingleNestedAttribute blocks.
//
// isImport is the one case where the gate has to be lifted: an import starts from
// no state at all, so every block is nil and nothing else can populate them.
//
// A response carrying no syncConfig is refused rather than absorbed. The two
// attributes it feeds are Optional+Computed with schema defaults, so the plan always
// holds a known value for them; committing nulls would trip the framework's
// consistency check with a message naming no cause, where this names one. The data
// source has no such contract and tolerates the nulls.
func assignUEMConnectResourceModel(state *UEMConnectResourceModel, config *securitycloud.ConnectorConfig, isImport bool) diag.Diagnostics {
	var diags diag.Diagnostics

	manageDataFieldMapping := isImport || state.UserDataFieldMapping != nil
	manageEmailMapping := isImport || (state.UserDataFieldMapping != nil && state.UserDataFieldMapping.Email != nil)
	manageGroupMapping := isImport || state.GroupMembershipMapping != nil
	manageGroupMappings := isImport || (state.GroupMembershipMapping != nil && state.GroupMembershipMapping.Mappings != nil)

	state.ID = types.StringValue(config.ID)
	state.UEMVendor = types.StringValue(config.Vendor)

	if config.TenantID != nil && *config.TenantID != "" {
		state.UEMServerURL = types.StringNull()
		state.PlatformTenant = &PlatformTenantModel{TenantID: types.StringValue(*config.TenantID)}
		state.OAuth = nil
	} else {
		state.UEMServerURL = types.StringValue(config.URL)
		state.PlatformTenant = nil
		clientID := types.StringNull()
		if config.DeviceSyncAuth != nil {
			clientID = helpers.StringValueOrNull(config.DeviceSyncAuth.ClientID)
		}
		woVersion := types.Int64Null()
		if state.OAuth != nil {
			woVersion = state.OAuth.ClientSecretWOVersion
		}
		state.OAuth = &OAuthModel{
			ClientID:              clientID,
			ClientSecret:          types.StringNull(),
			ClientSecretWOVersion: woVersion,
		}
	}

	state.Enabled = types.BoolValue(config.Enabled)
	state.ScheduledSyncEnabled = types.BoolValue(config.Scheduled)
	state.SyncRefreshIntervalMinutes = types.Int64Value(config.RefreshRateMinutes)
	state.UnmanagedSyncThreshold = types.Int64Value(int64(config.DeviceUnmanagedThreshold))
	state.DeviceRiskUEMSignalingEnabled = types.BoolValue(config.DeviceRiskTagging)
	state.ConcurrentDeviceSyncEnabled = types.BoolValue(config.ConcurrentSyncEnabled)

	if config.SyncConfig == nil {
		diags.AddError(
			"Jamf Security Cloud reported no sync configuration",
			"The integration was read successfully but carried no syncConfig, which is where the auto-delete "+
				"behavior and the disable-on-auth-error setting live. Both attributes carry a schema default, so "+
				"the plan holds a value there is now nothing to satisfy. Re-run to refresh; if it persists, "+
				"please report it.",
		)
		return diags
	}

	behaviour, authError, behaviourDiags := syncConfigToState(config.SyncConfig)
	diags.Append(behaviourDiags...)
	if diags.HasError() {
		return diags
	}
	state.UEMAutoDeleteBehaviour = behaviour
	state.DisableSyncOnAuthError = authError

	if manageDataFieldMapping {
		state.UserDataFieldMapping = dataFieldMappingToModel(config.DeviceFieldMappings, manageEmailMapping)
	}
	if manageGroupMapping {
		state.GroupMembershipMapping = groupMappingToModel(config.GroupSettings, manageGroupMappings)
	}

	return diags
}

// syncConfigToState translates the two settings the response nests under
// syncConfig but the update request carries at the top level.
//
// A nil syncConfig yields nulls here and is left to the caller to judge: the data
// source's equivalents are plain Computed, so nulls are a truthful reading, while
// the resource's carry schema defaults and therefore have a planned value to
// satisfy — see assignUEMConnectResourceModel.
func syncConfigToState(cfg *securitycloud.SyncConfig) (behaviour types.String, disableOnAuthError types.Bool, diags diag.Diagnostics) {
	if cfg == nil {
		return types.StringNull(), types.BoolNull(), diags
	}

	value, ok := uemAutoDeleteBehaviourFromWire[cfg.AutoDeviceDeletion]
	if !ok {
		diags.AddError(
			"Unrecognised auto-delete behavior from Jamf Security Cloud",
			"Jamf Security Cloud reported the auto-delete behavior as "+cfg.AutoDeviceDeletion+", which this "+
				"provider version does not recognise. Upgrade the provider; if the value is new, please report it.",
		)
		return types.StringNull(), types.BoolNull(), diags
	}
	return types.StringValue(value), types.BoolValue(cfg.DisableSyncOnAuthError), diags
}

// dataFieldMappingToModel maps the stored device field mappings onto the resource
// model.
//
// The four scalars are Optional+Computed, so a value the user omitted arrives from
// the server and is accepted. `email` is a nested block and Optional-only, so it is
// populated only when the caller says it is managed — the same gate, one level down.
func dataFieldMappingToModel(mappings *securitycloud.DeviceFieldMappings, manageEmail bool) *DataFieldMappingModel {
	if mappings == nil {
		return nil
	}

	out := &DataFieldMappingModel{
		DeviceName:  helpers.StringPointerValueOrNull(mappings.DeviceNameMapping),
		UserName:    helpers.StringPointerValueOrNull(mappings.UserNameMapping),
		UserID:      helpers.StringPointerValueOrNull(mappings.UserIDMapping),
		PhoneNumber: helpers.StringPointerValueOrNull(mappings.PhoneNumberMapping),
	}
	if manageEmail && mappings.UserEmailMapping != nil {
		out.Email = &EmailMappingModel{
			Source:             types.StringValue(mappings.UserEmailMapping.Type),
			Prefix:             helpers.StringPointerValueOrNull(mappings.UserEmailMapping.FieldPrefix),
			Suffix:             helpers.StringPointerValueOrNull(mappings.UserEmailMapping.FieldSuffix),
			OnlyIfEmailMissing: helpers.BoolPointerValueOrNull(mappings.UserEmailMapping.UseOnlyIfEmailMissing),
		}
	}
	return out
}

// groupMappingToModel maps the stored group settings onto the resource model,
// preserving the order the mappings are stored in — membership is evaluated top to
// bottom, so the order is configuration rather than presentation.
//
// manageMappings gates the list for the same reason the block itself is gated: it is
// Optional-only, and the server reports an empty array rather than nothing when
// there are no mappings.
func groupMappingToModel(settings *securitycloud.GroupSettings, manageMappings bool) *GroupMappingModel {
	if settings == nil {
		return nil
	}

	out := &GroupMappingModel{
		Enabled:                     helpers.BoolPointerValueOrNull(settings.GroupMappingEnabled),
		DefaultSecurityCloudGroupID: helpers.StringPointerValueOrNull(settings.DefaultGroupID),
	}
	if manageMappings && settings.GroupMappings != nil {
		entries := make([]GroupMappingEntryModel, 0, len(*settings.GroupMappings))
		for _, m := range *settings.GroupMappings {
			entries = append(entries, GroupMappingEntryModel{
				UEMGroupID:           types.StringValue(m.EmmGroupID),
				SecurityCloudGroupID: types.StringValue(m.WanderaGroupID),
			})
		}
		out.Mappings = entries
	}
	return out
}

// assignUEMConnectDataSourceModel maps the stored integration onto the data source
// model.
//
// It carries the observed state the resource leaves out. `latest_sync` is the one
// worth explaining: the connector record keeps only the current transaction's
// state, so it has a status and timings but no device counts — those live in the
// sync run history, which this provider does not surface.
func assignUEMConnectDataSourceModel(ctx context.Context, state *UEMConnectDataSourceModel, config *securitycloud.ConnectorConfig) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(config.ID)
	state.UEMVendor = types.StringValue(config.Vendor)
	state.UEMServerURL = types.StringValue(config.URL)
	state.PlatformTenantID = helpers.StringPointerValueOrNull(config.TenantID)
	state.Enabled = types.BoolValue(config.Enabled)
	state.Connected = types.BoolValue(config.Connected)
	state.JamfProVersion = helpers.StringPointerValueOrNull(config.UemVersion)
	state.ScheduledSyncEnabled = types.BoolValue(config.Scheduled)
	state.SyncRefreshIntervalMinutes = types.Int64Value(config.RefreshRateMinutes)
	state.UnmanagedSyncThreshold = types.Int64Value(int64(config.DeviceUnmanagedThreshold))
	state.DeviceRiskUEMSignalingEnabled = types.BoolValue(config.DeviceRiskTagging)
	state.ConcurrentDeviceSyncEnabled = types.BoolValue(config.ConcurrentSyncEnabled)

	state.ClientID = types.StringNull()
	if config.DeviceSyncAuth != nil {
		state.ClientID = helpers.StringValueOrNull(config.DeviceSyncAuth.ClientID)
	}

	behaviour, authError, behaviourDiags := syncConfigToState(config.SyncConfig)
	diags.Append(behaviourDiags...)
	if diags.HasError() {
		return diags
	}
	state.UEMAutoDeleteBehaviour = behaviour
	state.DisableSyncOnAuthError = authError

	mappingObject, mappingDiags := dataFieldMappingObject(ctx, config.DeviceFieldMappings)
	diags.Append(mappingDiags...)
	state.UserDataFieldMapping = mappingObject

	groupObject, groupDiags := groupMappingObject(ctx, config.GroupSettings)
	diags.Append(groupDiags...)
	state.GroupMembershipMapping = groupObject

	syncObject, syncDiags := latestSyncObject(ctx, config.LatestSync)
	diags.Append(syncDiags...)
	state.LatestSync = syncObject

	return diags
}

// dataFieldMappingObject renders the device field mappings as a data source
// object.
func dataFieldMappingObject(ctx context.Context, mappings *securitycloud.DeviceFieldMappings) (types.Object, diag.Diagnostics) {
	if mappings == nil {
		return types.ObjectNull(dataFieldMappingAttributeTypes), nil
	}

	email := types.ObjectNull(emailMappingAttributeTypes)
	if mappings.UserEmailMapping != nil {
		obj, diags := types.ObjectValueFrom(ctx, emailMappingAttributeTypes, EmailMappingModel{
			Source:             types.StringValue(mappings.UserEmailMapping.Type),
			Prefix:             helpers.StringPointerValueOrNull(mappings.UserEmailMapping.FieldPrefix),
			Suffix:             helpers.StringPointerValueOrNull(mappings.UserEmailMapping.FieldSuffix),
			OnlyIfEmailMissing: helpers.BoolPointerValueOrNull(mappings.UserEmailMapping.UseOnlyIfEmailMissing),
		})
		if diags.HasError() {
			return types.ObjectNull(dataFieldMappingAttributeTypes), diags
		}
		email = obj
	}

	return types.ObjectValue(dataFieldMappingAttributeTypes, map[string]attr.Value{
		"device_name":  helpers.StringPointerValueOrNull(mappings.DeviceNameMapping),
		"user_name":    helpers.StringPointerValueOrNull(mappings.UserNameMapping),
		"user_id":      helpers.StringPointerValueOrNull(mappings.UserIDMapping),
		"phone_number": helpers.StringPointerValueOrNull(mappings.PhoneNumberMapping),
		"email":        email,
	})
}

// groupMappingObject renders the group settings as a data source object.
func groupMappingObject(ctx context.Context, settings *securitycloud.GroupSettings) (types.Object, diag.Diagnostics) {
	if settings == nil {
		return types.ObjectNull(groupMappingAttributeTypes), nil
	}

	entries := []GroupMappingEntryModel{}
	if settings.GroupMappings != nil {
		for _, m := range *settings.GroupMappings {
			entries = append(entries, GroupMappingEntryModel{
				UEMGroupID:           types.StringValue(m.EmmGroupID),
				SecurityCloudGroupID: types.StringValue(m.WanderaGroupID),
			})
		}
	}
	mappings, diags := types.ListValueFrom(ctx, groupMappingEntryObjectType, entries)
	if diags.HasError() {
		return types.ObjectNull(groupMappingAttributeTypes), diags
	}

	return types.ObjectValue(groupMappingAttributeTypes, map[string]attr.Value{
		"enabled":                         helpers.BoolPointerValueOrNull(settings.GroupMappingEnabled),
		"default_security_cloud_group_id": helpers.StringPointerValueOrNull(settings.DefaultGroupID),
		"mappings":                        mappings,
	})
}

// latestSyncObject renders the most recent sync summary as a data source object.
// The whole object is null until the integration has run its first sync.
func latestSyncObject(_ context.Context, sync *securitycloud.LatestSync) (types.Object, diag.Diagnostics) {
	if sync == nil {
		return types.ObjectNull(latestSyncAttributeTypes), nil
	}

	reason := types.StringNull()
	description := types.StringNull()
	if sync.ErrorDetails != nil {
		reason = helpers.StringPointerValueOrNull(sync.ErrorDetails.Reason)
		description = helpers.StringPointerValueOrNull(sync.ErrorDetails.Description)
	}

	return types.ObjectValue(latestSyncAttributeTypes, map[string]attr.Value{
		"transaction_id":    types.StringValue(sync.TransactionID),
		"status":            types.StringValue(sync.Status),
		"trigger":           helpers.StringPointerValueOrNull(sync.RefreshType),
		"started":           timePointerValue(sync.Started),
		"finished":          timePointerValue(sync.Finished),
		"error_reason":      reason,
		"error_description": description,
	})
}
