// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildZoneWriteInput converts the Terraform plan model into the create payload.
func buildZoneWriteInput(ctx context.Context, plan DNSZoneResourceModel) (*securitycloud.ZoneWrite, diag.Diagnostics) {
	var diags diag.Diagnostics

	domains, domainDiags := domainsFromPlan(ctx, plan)
	diags.Append(domainDiags...)

	nameServers, nsDiags := nameServersFromPlan(ctx, plan)
	diags.Append(nsDiags...)

	if diags.HasError() {
		return nil, diags
	}

	return &securitycloud.ZoneWrite{
		Name:        plan.Name.ValueString(),
		Domains:     domains,
		NameServers: nameServers,
	}, diags
}

// buildZonePatchInput converts the Terraform plan model into the update payload.
//
// Every writable field is sent on every update, even when unchanged. The endpoint
// is a JSON merge patch where an omitted field is preserved, so a subset write
// would work — but all three fields are Required in the schema, so the plan
// always carries a complete desired state and there is no partial case to model.
// Sending the whole object keeps the config authoritative and means the omit
// semantics never have to be reasoned about at a call site. Clearing is not
// expressible either way: `domains: []`, `nameServers: []` and an explicit null
// are all refused (wire-probed 2026-08-27), which is why neither collection is
// optional.
func buildZonePatchInput(ctx context.Context, plan DNSZoneResourceModel) (*securitycloud.ZonePatch, diag.Diagnostics) {
	var diags diag.Diagnostics

	domains, domainDiags := domainsFromPlan(ctx, plan)
	diags.Append(domainDiags...)

	nameServers, nsDiags := nameServersFromPlan(ctx, plan)
	diags.Append(nsDiags...)

	if diags.HasError() {
		return nil, diags
	}

	name := plan.Name.ValueString()
	return &securitycloud.ZonePatch{
		Name:        &name,
		Domains:     &domains,
		NameServers: &nameServers,
	}, diags
}

// domainsFromPlan extracts the domain list from the plan model.
func domainsFromPlan(ctx context.Context, plan DNSZoneResourceModel) ([]string, diag.Diagnostics) {
	domains := make([]string, 0, len(plan.Domains.Elements()))
	diags := plan.Domains.ElementsAs(ctx, &domains, false)
	return domains, diags
}

// nameServersFromPlan extracts the name server entries from the plan model.
func nameServersFromPlan(ctx context.Context, plan DNSZoneResourceModel) ([]securitycloud.NameServer, diag.Diagnostics) {
	var models []NameServerModel
	diags := plan.NameServers.ElementsAs(ctx, &models, false)
	if diags.HasError() {
		return nil, diags
	}
	nameServers := make([]securitycloud.NameServer, 0, len(models))
	for _, m := range models {
		nameServers = append(nameServers, securitycloud.NameServer{
			IP:        m.IP.ValueString(),
			GatewayID: m.GatewayID.ValueString(),
		})
	}
	return nameServers, diags
}
