// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildCreateRequest turns the resource model into an activation profile create
// request.
//
// `origin` is always the SDK's PUBLIC_API constant: it identifies the caller
// rather than configuring the profile, so it is not a schema attribute. Sending
// it is not optional — the server refuses a create without it, and reports an
// out-of-enum value as "Origin not provided.", which misattributes the cause.
//
// `network_security` fans out to both `networkSecurity` and
// `vulnerabilityManagement`. The server refuses any request where the two
// disagree, and setting both is what lights the console's single "Network
// security" checkbox.
//
// The at-least-one-capability rule is asserted here as well as in the config
// validator, and both are needed. The validator cannot judge a value it has not
// seen — a capability taken from another resource's computed attribute is
// unknown at plan time, so the validator defers — while this function runs
// during apply, where every value is resolved. Without the second check that
// deferral is permanent and the operator gets the server's unattributed 400
// instead of the named diagnostic. The wording is shared with the validator via
// appendNoCapabilityEnabledError so the two cannot drift.
//
// A nil `capabilities` block is the same failure rather than a silent omission:
// the block is Required in the schema, so nil is unreachable from a real plan,
// and sending no capabilities at all is exactly the request the server refuses.
func buildCreateRequest(ctx context.Context, model *ActivationProfileResourceModel) (*securitycloud.PublicApiCreateActivationProfileRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	var labels []string
	diags.Append(model.Platforms.ElementsAs(ctx, &labels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	platforms := make([]string, 0, len(labels))
	for _, label := range sortedPlatformLabels(labels) {
		wire, ok := platformWire(label)
		if !ok {
			diags.AddError(
				"Unsupported platform",
				"The platform "+label+" is not one Jamf Security Cloud accepts for an activation profile.",
			)
			continue
		}
		platforms = append(platforms, wire)
	}
	if diags.HasError() {
		return nil, diags
	}

	request := &securitycloud.PublicApiCreateActivationProfileRequest{
		Name:      model.Name.ValueString(),
		Origin:    securitycloud.PublicApiCreateActivationProfileRequestOriginPublicApi,
		Platforms: platforms,
	}

	if model.Capabilities == nil {
		appendNoCapabilityEnabledError(&diags)
		return nil, diags
	}

	contentControls := model.Capabilities.ContentControls.ValueBool()
	networkSecurity := model.Capabilities.NetworkSecurity.ValueBool()
	if !contentControls && !networkSecurity {
		appendNoCapabilityEnabledError(&diags)
		return nil, diags
	}

	request.Capabilities = securitycloud.PublicApiCapabilities{
		DataPolicy:              &contentControls,
		NetworkSecurity:         &networkSecurity,
		VulnerabilityManagement: &networkSecurity,
	}
	if !model.Capabilities.Note.IsNull() && !model.Capabilities.Note.IsUnknown() {
		note := model.Capabilities.Note.ValueString()
		request.Capabilities.Note = &note
	}

	if !model.DeviceGroup.IsNull() && !model.DeviceGroup.IsUnknown() {
		group := model.DeviceGroup.ValueString()
		request.GroupID = &group
	}

	return request, diags
}
