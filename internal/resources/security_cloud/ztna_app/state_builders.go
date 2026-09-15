// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignAppResourceModel populates a resource model from an App response.
//
// Three response shapes need reconciling against the configuration rather than
// being copied across, and each is a permanent-diff bug if it is not:
//
//   - Every collection comes back re-ordered and de-duplicated, so all four are
//     sets. Order is never echoed, which is also why nothing tries to preserve it.
//   - An empty collection comes back as `[]`, never absent. Written straight
//     through, an app with no host names would hold an empty set in state while the
//     configuration holds null. Each one collapses to null instead, which is the
//     other half of the schema refusing an explicit empty collection.
//   - `security` is always present on read with all three cards populated from
//     server defaults, whether or not the configuration mentions them. Populating
//     what the configuration never asked for would put values in state the operator
//     did not write, so each card is filled only when the target model already
//     carries it — per STYLE_GUIDE §`SingleNestedAttribute` blocks and the
//     Optional-only block population gate.
//
// `hostnames` needs no case reconciliation despite the server lower-casing them and
// stripping a trailing root dot, because `normalisedHostname()` refuses any spelling
// the server would rewrite, so the configuration can only ever hold the stored form.
// State and configuration therefore already agree by the time this runs. That
// validator is the entire mechanism, and nothing here can stand in for it: because
// the attribute is Optional rather than Optional+Computed, Terraform will not let the
// provider rewrite the planned value, so a non-normalised spelling accepted at plan
// time would diff forever.
func assignAppResourceModel(ctx context.Context, state *ZtnaAppResourceModel, app *securitycloud.App) diag.Diagnostics {
	var diags diag.Diagnostics

	if app.ID != "" {
		state.ID = types.StringValue(app.ID)
	}
	state.Name = types.StringPointerValue(app.Name)
	state.PredefinedAppID = types.StringPointerValue(app.PredefinedAppID)
	state.AppType = types.StringValue(appTypeFor(app.PredefinedAppID))
	state.Category = types.StringValue(app.CategoryName)

	hostnames, hostnameDiags := stringSetOrNull(ctx, app.Hostnames)
	diags.Append(hostnameDiags...)
	state.Hostnames = hostnames

	bareIPs, bareIPDiags := stringSetOrNull(ctx, app.BareIps)
	diags.Append(bareIPDiags...)
	state.DirectIPsAndSubnets = bareIPs

	state.AllDeviceGroups = types.BoolValue(false)
	state.DeviceGroupIDs = types.SetNull(types.StringType)
	if app.Assignments != nil {
		state.AllDeviceGroups = types.BoolValue(app.Assignments.Inclusions.AllUsers)
		groups, groupDiags := stringSetOrNull(ctx, derefStrings(app.Assignments.Inclusions.Groups))
		diags.Append(groupDiags...)
		state.DeviceGroupIDs = groups
	}

	state.Routing = routingFromWire(app.Routing)

	overrides, overrideDiags := routingOverrideListValue(ctx, app.GroupOverrides)
	diags.Append(overrideDiags...)
	state.RoutingOverrides = overrides

	state.Security = securityFromWire(state.Security, app.Security)

	return diags
}

// assignAppDataSourceModel populates the singular data source model from an App
// response. Data sources report what the server holds with no configuration to
// reconcile against, so every collection is a list, an empty collection stays
// empty rather than collapsing to null, and all three security cards are always
// populated.
func assignAppDataSourceModel(ctx context.Context, state *ZtnaAppDataSourceModel, app *securitycloud.App) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(app.ID)
	state.Name = types.StringPointerValue(app.Name)
	state.PredefinedAppID = types.StringPointerValue(app.PredefinedAppID)
	state.AppType = types.StringValue(appTypeFor(app.PredefinedAppID))
	state.Category = types.StringValue(app.CategoryName)

	hostnames, hostnameDiags := types.ListValueFrom(ctx, types.StringType, app.Hostnames)
	diags.Append(hostnameDiags...)
	state.Hostnames = hostnames

	bareIPs, bareIPDiags := types.ListValueFrom(ctx, types.StringType, app.BareIps)
	diags.Append(bareIPDiags...)
	state.DirectIPsAndSubnets = bareIPs

	allUsers := false
	var groupIDs []string
	if app.Assignments != nil {
		allUsers = app.Assignments.Inclusions.AllUsers
		groupIDs = derefStrings(app.Assignments.Inclusions.Groups)
	}
	state.AllDeviceGroups = types.BoolValue(allUsers)
	groups, groupDiags := types.ListValueFrom(ctx, types.StringType, groupIDs)
	diags.Append(groupDiags...)
	state.DeviceGroupIDs = groups

	routing, routingDiags := routingObjectValue(app.Routing)
	diags.Append(routingDiags...)
	state.Routing = routing

	overrides, overrideDiags := dsRoutingOverrideListValue(ctx, app.GroupOverrides)
	diags.Append(overrideDiags...)
	state.RoutingOverrides = overrides

	security, securityDiags := securityObjectValue(app.Security)
	diags.Append(securityDiags...)
	state.Security = security

	return diags
}

// buildAppsResultModel maps one App response into a plural data source result
// element.
func buildAppsResultModel(ctx context.Context, app securitycloud.App) (ZtnaAppsDataSourceResultModel, diag.Diagnostics) {
	var ds ZtnaAppDataSourceModel
	diags := assignAppDataSourceModel(ctx, &ds, &app)
	return ZtnaAppsDataSourceResultModel{
		ID:                  ds.ID,
		Name:                ds.Name,
		PredefinedAppID:     ds.PredefinedAppID,
		AppType:             ds.AppType,
		Category:            ds.Category,
		Hostnames:           ds.Hostnames,
		DirectIPsAndSubnets: ds.DirectIPsAndSubnets,
		AllDeviceGroups:     ds.AllDeviceGroups,
		DeviceGroupIDs:      ds.DeviceGroupIDs,
		Routing:             ds.Routing,
		RoutingOverrides:    ds.RoutingOverrides,
		Security:            ds.Security,
	}, diags
}

// routingFromWire converts a wire routing object into the resource model, mapping
// both enumerated members back to their admin-UI labels.
func routingFromWire(routing *securitycloud.Routing) *RoutingModel {
	if routing == nil {
		return nil
	}
	out := &RoutingModel{
		TrafficRouting: types.StringValue(labelFor(routing.Type, routingModeLabels)),
		GatewayID:      types.StringPointerValue(routing.GatewayID),
		RoutingMode:    types.StringNull(),
	}
	if routing.DnsIpResolutionType != nil {
		out.RoutingMode = types.StringValue(labelFor(*routing.DnsIpResolutionType, dnsResolutionLabels))
	}
	return out
}

// securityFromWire fills the security block from the response, but only the cards
// the target model already declares.
//
// The gate is on the target rather than on the response because the response always
// carries all three: the server defaults every card whether or not the write
// mentioned it. Gating on the response would therefore populate state for cards the
// configuration never wrote, and the next plan would show them as removals.
func securityFromWire(target *SecurityModel, security *securitycloud.AppSecurity) *SecurityModel {
	if target == nil || security == nil {
		return target
	}
	out := &SecurityModel{}
	if target.ManagedDevice != nil && security.DeviceManagementBasedAccess != nil {
		out.ManagedDevice = &SecurityControlModel{
			Enabled:                 types.BoolValue(security.DeviceManagementBasedAccess.Enabled),
			DevicePushNotifications: types.BoolValue(security.DeviceManagementBasedAccess.NotificationsEnabled),
		}
	}
	if target.DeviceRisk != nil && security.RiskControls != nil {
		out.DeviceRisk = &DeviceRiskModel{
			Enabled:                 types.BoolValue(security.RiskControls.Enabled),
			DenyAtRiskLevel:         types.StringValue(labelFor(security.RiskControls.LevelThreshold, riskLevelLabels)),
			DevicePushNotifications: types.BoolValue(security.RiskControls.NotificationsEnabled),
		}
	}
	if target.JamfTrust != nil && security.DohIntegration != nil {
		out.JamfTrust = &SecurityControlModel{
			Enabled:                 types.BoolValue(security.DohIntegration.Blocking),
			DevicePushNotifications: types.BoolValue(security.DohIntegration.NotificationsEnabled),
		}
	}
	return out
}

// routingObjectValue renders a wire routing object as a framework object value, for
// the data sources.
func routingObjectValue(routing *securitycloud.Routing) (types.Object, diag.Diagnostics) {
	model := routingFromWire(routing)
	if model == nil {
		return types.ObjectNull(routingAttributeTypes), nil
	}
	return types.ObjectValue(routingAttributeTypes, map[string]attr.Value{
		"traffic_routing": model.TrafficRouting,
		"gateway_id":      model.GatewayID,
		"routing_mode":    model.RoutingMode,
	})
}

// routingOverrideListValue builds the resource-side routing_overrides list.
func routingOverrideListValue(ctx context.Context, overrides *securitycloud.GroupOverrides) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	entries := routingOverrideEntries(overrides)
	if len(entries) == 0 {
		return types.ListNull(routingOverrideObjectType), diags
	}

	values := make([]attr.Value, 0, len(entries))
	for _, entry := range entries {
		groups, groupDiags := types.SetValueFrom(ctx, types.StringType, entry.GroupIds)
		diags.Append(groupDiags...)

		routing, routingDiags := routingObjectValue(&entry.Routing)
		diags.Append(routingDiags...)

		obj, objDiags := types.ObjectValue(routingOverrideAttributeTypes, map[string]attr.Value{
			"device_group_ids": groups,
			"routing":          routing,
		})
		diags.Append(objDiags...)
		values = append(values, obj)
	}
	if diags.HasError() {
		return types.ListNull(routingOverrideObjectType), diags
	}
	list, listDiags := types.ListValue(routingOverrideObjectType, values)
	diags.Append(listDiags...)
	return list, diags
}

// dsRoutingOverrideListValue builds the data-source-side routing_overrides list,
// where the nested group IDs are a list rather than a set and an empty collection
// stays empty.
func dsRoutingOverrideListValue(ctx context.Context, overrides *securitycloud.GroupOverrides) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	entries := routingOverrideEntries(overrides)

	values := make([]attr.Value, 0, len(entries))
	for _, entry := range entries {
		groups, groupDiags := types.ListValueFrom(ctx, types.StringType, entry.GroupIds)
		diags.Append(groupDiags...)

		routing, routingDiags := routingObjectValue(&entry.Routing)
		diags.Append(routingDiags...)

		obj, objDiags := types.ObjectValue(dsRoutingOverrideAttributeTypes, map[string]attr.Value{
			"device_group_ids": groups,
			"routing":          routing,
		})
		diags.Append(objDiags...)
		values = append(values, obj)
	}
	if diags.HasError() {
		return types.ListNull(dsRoutingOverrideObjectType), diags
	}
	list, listDiags := types.ListValue(dsRoutingOverrideObjectType, values)
	diags.Append(listDiags...)
	return list, diags
}

// securityObjectValue renders the whole security block as a framework object value,
// for the data sources. Unlike the resource path it populates every card, because a
// data source reports what the server holds and has no configuration to gate on.
func securityObjectValue(security *securitycloud.AppSecurity) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	if security == nil {
		return types.ObjectNull(securityAttributeTypes), diags
	}

	managed := types.ObjectNull(securityControlAttributeTypes)
	if c := security.DeviceManagementBasedAccess; c != nil {
		obj, objDiags := securityControlObjectValue(c.Enabled, c.NotificationsEnabled)
		diags.Append(objDiags...)
		managed = obj
	}

	trust := types.ObjectNull(securityControlAttributeTypes)
	if c := security.DohIntegration; c != nil {
		obj, objDiags := securityControlObjectValue(c.Blocking, c.NotificationsEnabled)
		diags.Append(objDiags...)
		trust = obj
	}

	risk := types.ObjectNull(deviceRiskAttributeTypes)
	if c := security.RiskControls; c != nil {
		obj, objDiags := types.ObjectValue(deviceRiskAttributeTypes, map[string]attr.Value{
			"enabled":                   types.BoolValue(c.Enabled),
			"deny_at_risk_level":        types.StringValue(labelFor(c.LevelThreshold, riskLevelLabels)),
			"device_push_notifications": types.BoolValue(c.NotificationsEnabled),
		})
		diags.Append(objDiags...)
		risk = obj
	}

	obj, objDiags := types.ObjectValue(securityAttributeTypes, map[string]attr.Value{
		"managed_device": managed,
		"device_risk":    risk,
		"jamf_trust":     trust,
	})
	diags.Append(objDiags...)
	return obj, diags
}

// securityControlObjectValue renders one plain security card as an object value.
func securityControlObjectValue(enabled, notifications bool) (types.Object, diag.Diagnostics) {
	return types.ObjectValue(securityControlAttributeTypes, map[string]attr.Value{
		"enabled":                   types.BoolValue(enabled),
		"device_push_notifications": types.BoolValue(notifications),
	})
}

// routingOverrideEntries unwraps the two levels of optionality the wire puts around
// the override list, so callers see one slice either way.
func routingOverrideEntries(overrides *securitycloud.GroupOverrides) []securitycloud.RoutingOverride {
	if overrides == nil || overrides.RoutingOverrides == nil {
		return nil
	}
	return *overrides.RoutingOverrides
}

// stringSetOrNull builds a set from a response collection, collapsing an empty one
// to null. The endpoint returns `[]` rather than omitting an empty collection, and
// the schema refuses an explicit empty collection, so `[]` can only ever have come
// from an absent attribute.
func stringSetOrNull(ctx context.Context, values []string) (types.Set, diag.Diagnostics) {
	if len(values) == 0 {
		return types.SetNull(types.StringType), nil
	}
	return types.SetValueFrom(ctx, types.StringType, values)
}

// derefStrings unwraps an optional wire string slice.
func derefStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
