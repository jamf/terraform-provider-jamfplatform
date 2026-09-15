// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignHostnameMappingsResourceModel copies the stored mapping set into the
// resource model.
//
// It deliberately does not touch the model's ID: the CRUD handler stamps
// helpers.SingletonID, and an assigner that also wrote it would give two places for
// the value to come from. state_builders_test.go pins that.
func assignHostnameMappingsResourceModel(ctx context.Context, model *HostnameMappingsResourceModel, got *securitycloud.MappingList) diag.Diagnostics {
	var diags diag.Diagnostics

	values := make([]types.Object, 0, len(got.Results))
	for _, mapping := range got.Results {
		ipv4, ipv4Diags := addressSetValue(ctx, mapping.ARecords)
		diags.Append(ipv4Diags...)
		ipv6, ipv6Diags := addressSetValue(ctx, mapping.AaaaRecords)
		diags.Append(ipv6Diags...)
		if diags.HasError() {
			return diags
		}

		object, objectDiags := types.ObjectValue(mappingAttributeTypes, map[string]attr.Value{
			"hostname":              types.StringValue(mapping.Hostname),
			"ipv4_addresses":        ipv4,
			"ipv6_addresses":        ipv6,
			"connect_to_ztna":       types.BoolValue(boolOrFalse(mapping.Ztna)),
			"connect_to_secure_dns": types.BoolValue(boolOrFalse(mapping.SecureDns)),
		})
		diags.Append(objectDiags...)
		if diags.HasError() {
			return diags
		}
		values = append(values, object)
	}

	set, setDiags := types.SetValueFrom(ctx, mappingObjectType, values)
	diags.Append(setDiags...)
	if diags.HasError() {
		return diags
	}
	model.Mappings = set
	return diags
}

// assignHostnameMappingsDataSourceModel copies the stored mapping set into the data
// source model. Address collections are lists here because data source attributes
// returning API data are read-only.
func assignHostnameMappingsDataSourceModel(ctx context.Context, model *HostnameMappingsDataSourceModel, got *securitycloud.MappingList) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Mappings = make([]HostnameMappingsDataSourceItem, 0, len(got.Results))
	for _, mapping := range got.Results {
		ipv4, ipv4Diags := addressListValue(ctx, mapping.ARecords)
		diags.Append(ipv4Diags...)
		ipv6, ipv6Diags := addressListValue(ctx, mapping.AaaaRecords)
		diags.Append(ipv6Diags...)
		if diags.HasError() {
			return diags
		}

		model.Mappings = append(model.Mappings, HostnameMappingsDataSourceItem{
			Hostname:           types.StringValue(mapping.Hostname),
			IPv4Addresses:      ipv4,
			IPv6Addresses:      ipv6,
			ConnectToZTNA:      types.BoolValue(boolOrFalse(mapping.Ztna)),
			ConnectToSecureDNS: types.BoolValue(boolOrFalse(mapping.SecureDns)),
		})
	}
	return diags
}

// addressSetValue converts a stored address list into a set, collapsing empty to
// null.
//
// Empty becomes null because the wire cannot tell the two apart: an omitted address
// list reads back as `[]`, so a configuration that omits ipv4_addresses would
// otherwise refresh into an empty set and diff against its own config forever. The
// schema refuses an explicitly empty collection for the same reason, which keeps the
// collapse lossless.
func addressSetValue(ctx context.Context, addresses *[]string) (types.Set, diag.Diagnostics) {
	if addresses == nil || len(*addresses) == 0 {
		return types.SetNull(types.StringType), nil
	}
	return types.SetValueFrom(ctx, types.StringType, *addresses)
}

// addressListValue converts a stored address list into a list for the data source.
//
// Unlike the resource, empty stays an empty list rather than null: a data source has
// no configuration to diff against, and an empty collection is easier to consume in
// a for expression than a null one.
func addressListValue(ctx context.Context, addresses *[]string) (types.List, diag.Diagnostics) {
	if addresses == nil {
		return types.ListValueFrom(ctx, types.StringType, []string{})
	}
	return types.ListValueFrom(ctx, types.StringType, *addresses)
}

// boolOrFalse dereferences an optional wire boolean. Both flags are omitted from the
// response only when unset, and the endpoint's own default for each is false —
// wire-probed, and deliberately not the admin UI's default, which pre-selects
// "Connect to ZTNA" in its add dialog.
func boolOrFalse(value *bool) bool {
	return value != nil && *value
}
