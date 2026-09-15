// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildAppCreateInput converts the Terraform plan model into the create payload.
//
// `name` and `predefinedAppId` are mutually exclusive and the schema's config
// validators have already enforced that exactly one is set, so each is sent only
// when non-null rather than being reconciled here.
func buildAppCreateInput(ctx context.Context, plan ZtnaAppResourceModel) (*securitycloud.AppCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	hostnames, hostnameDiags := stringSlice(ctx, plan.Hostnames)
	diags.Append(hostnameDiags...)

	bareIPs, bareIPDiags := stringSlice(ctx, plan.DirectIPsAndSubnets)
	diags.Append(bareIPDiags...)

	assignments, assignmentDiags := assignmentsFromPlan(ctx, plan)
	diags.Append(assignmentDiags...)

	overrides, overrideDiags := groupOverridesFromPlan(ctx, plan)
	diags.Append(overrideDiags...)

	if diags.HasError() {
		return nil, diags
	}

	input := &securitycloud.AppCreateRequest{
		Assignments:  assignments,
		CategoryName: plan.Category.ValueString(),
		Routing:      routingToWire(plan.Routing),
		Hostnames:    &hostnames,
		BareIps:      &bareIPs,
		GroupOverrides: &securitycloud.GroupOverrides{
			RoutingOverrides: &overrides,
		},
		Security: securityToWire(plan.Security),
	}
	if !plan.Name.IsNull() {
		name := plan.Name.ValueString()
		input.Name = &name
	}
	if !plan.PredefinedAppID.IsNull() {
		predefined := plan.PredefinedAppID.ValueString()
		input.PredefinedAppID = &predefined
	}
	return input, diags
}

// buildAppPatchInput converts the Terraform plan model into the update payload.
//
// Every writable field is sent on every update, even when unchanged. The endpoint
// is a JSON merge patch where an omitted field is preserved, so a subset write
// would work — but each of these attributes is Optional rather than
// Optional+Computed, which makes the configuration authoritative: an attribute
// removed from the configuration has to clear on the server, not linger. Sending
// the whole object is what makes that true, and it means the omit semantics never
// have to be reasoned about at a call site. An absent collection therefore goes out
// as `[]` rather than being omitted, which is how the endpoint clears one
// (wire-verified 2026-08-30).
//
// Two fields are deliberately not sent. `predefinedAppId` has no place in the patch
// body at all — the form is immutable, which is why the attribute is
// RequiresReplace. `name` is skipped for a predefined app because the server
// silently discards it there; the schema refuses the combination anyway, so this is
// belt and braces rather than the enforcement point.
//
// `security` is sent only for the cards the configuration declares. Each is
// Optional-only, so a card left out of the configuration is one the operator has
// not taken a position on, and omitting it from the patch leaves the server's value
// alone.
func buildAppPatchInput(ctx context.Context, plan ZtnaAppResourceModel) (*securitycloud.AppPatchRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	hostnames, hostnameDiags := stringSlice(ctx, plan.Hostnames)
	diags.Append(hostnameDiags...)

	bareIPs, bareIPDiags := stringSlice(ctx, plan.DirectIPsAndSubnets)
	diags.Append(bareIPDiags...)

	assignments, assignmentDiags := assignmentsFromPlan(ctx, plan)
	diags.Append(assignmentDiags...)

	overrides, overrideDiags := groupOverridesFromPlan(ctx, plan)
	diags.Append(overrideDiags...)

	if diags.HasError() {
		return nil, diags
	}

	category := plan.Category.ValueString()
	routing := routingToWire(plan.Routing)

	input := &securitycloud.AppPatchRequest{
		Assignments:  &assignments,
		CategoryName: &category,
		Routing:      &routing,
		Hostnames:    &hostnames,
		BareIps:      &bareIPs,
		GroupOverrides: &securitycloud.GroupOverrides{
			RoutingOverrides: &overrides,
		},
		Security: securityToWire(plan.Security),
	}
	if plan.PredefinedAppID.IsNull() && !plan.Name.IsNull() {
		name := plan.Name.ValueString()
		input.Name = &name
	}
	return input, diags
}

// routingToWire converts a routing block into the wire object.
//
// Both optional members are sent as explicit nulls when absent rather than being
// omitted, which the SDK's `emitNullForOptional` entry for this schema makes
// possible. It is the only way a routing type transition can be expressed: merge
// patch merges this object field by field, so switching to `DIRECT` has to null the
// gateway and the resolution mode the previous `CUSTOM` left behind, and switching
// to `CUSTOM` has to carry a resolution mode the server treats as mandatory.
func routingToWire(routing *RoutingModel) securitycloud.Routing {
	if routing == nil {
		return securitycloud.Routing{}
	}
	out := securitycloud.Routing{
		Type: wireValueFor(routing.TrafficRouting.ValueString(), routingModeLabels),
	}
	if !routing.GatewayID.IsNull() && !routing.GatewayID.IsUnknown() {
		gateway := routing.GatewayID.ValueString()
		out.GatewayID = &gateway
	}
	if !routing.RoutingMode.IsNull() && !routing.RoutingMode.IsUnknown() {
		resolution := wireValueFor(routing.RoutingMode.ValueString(), dnsResolutionLabels)
		out.DnsIpResolutionType = &resolution
	}
	return out
}

// assignmentsFromPlan converts the flattened device group assignment attributes
// into the wire object.
//
// `groups` is sent as an empty array rather than omitted when no groups are
// configured, for the same reason the other collections are: the configuration is
// authoritative. That is a legal state on this endpoint — `allUsers: false` with an
// empty `groups` array is accepted, contrary to what the spec claims — and it is
// also what the server stores whenever `allUsers` is true.
func assignmentsFromPlan(ctx context.Context, plan ZtnaAppResourceModel) (securitycloud.Assignments, diag.Diagnostics) {
	groups, diags := stringSlice(ctx, plan.DeviceGroupIDs)
	return securitycloud.Assignments{
		Inclusions: securitycloud.AssignmentsInclusions{
			AllUsers: plan.AllDeviceGroups.ValueBool(),
			Groups:   &groups,
		},
	}, diags
}

// groupOverridesFromPlan converts the routing_overrides list into wire objects.
func groupOverridesFromPlan(ctx context.Context, plan ZtnaAppResourceModel) ([]securitycloud.RoutingOverride, diag.Diagnostics) {
	overrides := make([]securitycloud.RoutingOverride, 0, len(plan.RoutingOverrides.Elements()))
	if plan.RoutingOverrides.IsNull() || plan.RoutingOverrides.IsUnknown() {
		return overrides, nil
	}

	var models []RoutingOverrideModel
	diags := plan.RoutingOverrides.ElementsAs(ctx, &models, false)
	if diags.HasError() {
		return nil, diags
	}
	for _, m := range models {
		groups, groupDiags := stringSlice(ctx, m.DeviceGroupIDs)
		diags.Append(groupDiags...)
		overrides = append(overrides, securitycloud.RoutingOverride{
			GroupIds: groups,
			Routing:  routingToWire(m.Routing),
		})
	}
	if diags.HasError() {
		return nil, diags
	}
	return overrides, diags
}

// securityToWire converts the security block into the wire object, carrying only
// the cards the configuration declares.
//
// Each card maps to a whole wire sub-object whose members are non-pointer, so a
// declared card must supply all of its values. That is why the leaf attributes are
// Optional+Computed with defaults matching the server's own rather than
// Optional-only: it keeps every leaf known at plan time, so a card can never be
// sent half-filled with an empty risk level the enum would refuse.
func securityToWire(security *SecurityModel) *securitycloud.AppSecurity {
	if security == nil {
		return nil
	}
	out := &securitycloud.AppSecurity{}
	if c := security.ManagedDevice; c != nil {
		out.DeviceManagementBasedAccess = &securitycloud.DeviceManagementBasedAccess{
			Enabled:              c.Enabled.ValueBool(),
			NotificationsEnabled: c.DevicePushNotifications.ValueBool(),
		}
	}
	if c := security.DeviceRisk; c != nil {
		out.RiskControls = &securitycloud.RiskControls{
			Enabled:              c.Enabled.ValueBool(),
			LevelThreshold:       wireValueFor(c.DenyAtRiskLevel.ValueString(), riskLevelLabels),
			NotificationsEnabled: c.DevicePushNotifications.ValueBool(),
		}
	}
	if c := security.JamfTrust; c != nil {
		out.DohIntegration = &securitycloud.DohIntegration{
			Blocking:             c.Enabled.ValueBool(),
			NotificationsEnabled: c.DevicePushNotifications.ValueBool(),
		}
	}
	return out
}

// stringSlice extracts a set of strings from the plan model, returning an empty
// slice rather than nil for an absent collection so the caller can send `[]`.
func stringSlice(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	out := make([]string, 0, len(set.Elements()))
	if set.IsNull() || set.IsUnknown() {
		return out, nil
	}
	diags := set.ElementsAs(ctx, &out, false)
	return out, diags
}
