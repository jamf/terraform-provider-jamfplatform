// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignDNSZoneResourceModel populates a resource model from a Zone response.
//
// Both collections are written straight from the response with no reconciliation
// against the prior state. That is safe here because neither is order-sensitive
// in Terraform: `domains` and `authoritative_name_servers` are sets, so the
// comparison ignores order. It also has to be that way for `domains` — Jamf
// Security Cloud sorts the stored domain list byte-wise ascending, so the read
// never echoes the authored order back (wire-probed 2026-08-27).
func assignDNSZoneResourceModel(ctx context.Context, state *DNSZoneResourceModel, z *securitycloud.Zone) diag.Diagnostics {
	var diags diag.Diagnostics

	if z.ID != "" {
		state.ID = types.StringValue(z.ID)
	}
	state.Name = types.StringValue(z.Name)

	domains, domainDiags := types.SetValueFrom(ctx, types.StringType, z.Domains)
	diags.Append(domainDiags...)
	state.Domains = domains

	nameServers, nsDiags := nameServerSetValue(z.NameServers)
	diags.Append(nsDiags...)
	state.NameServers = nameServers

	return diags
}

// assignDNSZoneDataSourceModel populates the singular data source model from a
// Zone response.
func assignDNSZoneDataSourceModel(ctx context.Context, state *DNSZoneDataSourceModel, z *securitycloud.Zone) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(z.ID)
	state.Name = types.StringValue(z.Name)

	domains, domainDiags := types.ListValueFrom(ctx, types.StringType, z.Domains)
	diags.Append(domainDiags...)
	state.Domains = domains

	nameServers, nsDiags := nameServerListValue(z.NameServers)
	diags.Append(nsDiags...)
	state.NameServers = nameServers

	return diags
}

// buildDNSZonesResultModel maps one Zone response into a plural data source
// result element.
func buildDNSZonesResultModel(ctx context.Context, z securitycloud.Zone) (DNSZonesDataSourceResultModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	domains, domainDiags := types.ListValueFrom(ctx, types.StringType, z.Domains)
	diags.Append(domainDiags...)

	nameServers, nsDiags := nameServerListValue(z.NameServers)
	diags.Append(nsDiags...)

	return DNSZonesDataSourceResultModel{
		ID:          types.StringValue(z.ID),
		Name:        types.StringValue(z.Name),
		Domains:     domains,
		NameServers: nameServers,
	}, diags
}

// nameServerObjectValues converts SDK name server entries into object values.
func nameServerObjectValues(servers []securitycloud.NameServer) ([]attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics
	values := make([]attr.Value, 0, len(servers))
	for _, s := range servers {
		obj, objDiags := types.ObjectValue(nameServerAttributeTypes, map[string]attr.Value{
			"ip_address": types.StringValue(s.IP),
			"gateway_id": types.StringValue(s.GatewayID),
		})
		diags.Append(objDiags...)
		values = append(values, obj)
	}
	return values, diags
}

// nameServerSetValue builds the resource-side authoritative_name_servers set.
func nameServerSetValue(servers []securitycloud.NameServer) (types.Set, diag.Diagnostics) {
	values, diags := nameServerObjectValues(servers)
	if diags.HasError() {
		return types.SetNull(nameServerObjectType), diags
	}
	set, setDiags := types.SetValue(nameServerObjectType, values)
	diags.Append(setDiags...)
	return set, diags
}

// nameServerListValue builds the data-source-side authoritative_name_servers list.
func nameServerListValue(servers []securitycloud.NameServer) (types.List, diag.Diagnostics) {
	values, diags := nameServerObjectValues(servers)
	if diags.HasError() {
		return types.ListNull(nameServerObjectType), diags
	}
	list, listDiags := types.ListValue(nameServerObjectType, values)
	diags.Append(listDiags...)
	return list, diags
}
