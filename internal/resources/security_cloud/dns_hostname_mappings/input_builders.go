// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildMappingsWriteInput turns the planned mappings into the full-replace payload.
//
// Absent and empty are the same thing for an address list on this endpoint, so a
// null set is sent as an empty array rather than omitted. Sending null outright is
// what the SDK's types invite — both lists are *[]string with omitempty — but a
// mapping whose aRecords is absent is refused exactly as one whose aRecords is empty
// is, so there is no behaviour to gain and a nil pointer to get wrong.
func buildMappingsWriteInput(ctx context.Context, mappings types.Set) ([]securitycloud.Mapping, diag.Diagnostics) {
	var diags diag.Diagnostics

	var models []MappingModel
	diags.Append(mappings.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil, diags
	}

	out := make([]securitycloud.Mapping, 0, len(models))
	for _, model := range models {
		ipv4, ipv4Diags := addressSlice(ctx, model.IPv4Addresses)
		diags.Append(ipv4Diags...)
		ipv6, ipv6Diags := addressSlice(ctx, model.IPv6Addresses)
		diags.Append(ipv6Diags...)
		if diags.HasError() {
			return nil, diags
		}

		out = append(out, securitycloud.Mapping{
			Hostname:    model.Hostname.ValueString(),
			ARecords:    &ipv4,
			AaaaRecords: &ipv6,
			SecureDns:   model.ConnectToSecureDNS.ValueBoolPointer(),
			Ztna:        model.ConnectToZTNA.ValueBoolPointer(),
		})
	}
	return out, diags
}

// addressSlice converts an address set into a slice, treating null and unknown as
// empty. It always returns a non-nil slice so the payload carries `[]` rather than
// `null`.
func addressSlice(ctx context.Context, addresses types.Set) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := []string{}
	if addresses.IsNull() || addresses.IsUnknown() {
		return out, diags
	}
	diags.Append(addresses.ElementsAs(ctx, &out, false)...)
	return out, diags
}
