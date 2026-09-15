// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignGatewayResourceModel populates a resource model from a Gateway response.
//
// Two fields deliberately do not come from the wire. The pre-shared key is never
// returned, so `shared_secret` stays whatever the framework left in the model
// (which is null — it is WriteOnly), and `shared_secret_wo_version` is carried
// over from the prior model rather than read, because the wire has no counterpart
// and losing it would make the next update look like a rotation.
//
// `ipsec_source_ip_addresses` is written back only when the server actually reports
// zones. It answers null for a gateway created without them, and writing an empty
// set over a null config value would turn a no-op into a permanent diff.
func assignGatewayResourceModel(ctx context.Context, state *GatewayResourceModel, g *securitycloud.Gateway) diag.Diagnostics {
	var diags diag.Diagnostics

	if g.ID != "" {
		state.ID = types.StringValue(g.ID)
	}
	state.Name = types.StringValue(g.Name)
	state.EgressRegion = types.StringValue(labelFor(g.Datacenter, datacenterLabels))
	state.Enabled = types.BoolValue(g.Enabled)

	if g.Contact != nil {
		state.Contact = &ContactModel{
			Name:  types.StringValue(g.Contact.Name),
			Email: types.StringValue(g.Contact.Email),
		}
	}

	tenantIDs, tenantDiags := types.SetValueFrom(ctx, types.StringType, g.TenantIds)
	diags.Append(tenantDiags...)
	state.TenantIDs = tenantIDs

	if len(g.AvailabilityZones) > 0 {
		zones, zoneDiags := types.SetValueFrom(ctx, types.StringType, g.AvailabilityZones)
		diags.Append(zoneDiags...)
		state.IPSecSourceIPAddresses = zones
	} else {
		state.IPSecSourceIPAddresses = types.SetNull(types.StringType)
	}

	dedicatedIPs, dedicatedDiags := dedicatedEgressIPList(ctx, g.DedicatedIps)
	diags.Append(dedicatedDiags...)
	state.DedicatedEgressIPAddresses = dedicatedIPs

	status, statusDiags := statusObjectValue(g.Status)
	diags.Append(statusDiags...)
	state.Status = status

	diags.Append(assignIPSecResourceModel(ctx, state, g.Ipsec)...)

	return diags
}

// assignIPSecResourceModel refreshes the IPsec block in place, preserving the two
// fields the wire cannot supply.
func assignIPSecResourceModel(ctx context.Context, state *GatewayResourceModel, wire *securitycloud.GatewayIpSec) diag.Diagnostics {
	var diags diag.Diagnostics

	if wire == nil {
		state.IPSec = nil
		return diags
	}

	priorWoVersion := types.Int64Null()
	priorSecret := types.StringNull()
	if state.IPSec != nil && state.IPSec.JamfSide != nil {
		priorWoVersion = state.IPSec.JamfSide.AuthenticationSecretWoVersion
		priorSecret = state.IPSec.JamfSide.AuthenticationSecret
	}

	ipsec := &IPSecModel{
		KeyExchangeProtocol: types.StringValue(labelFor(wire.KeyExchange, keyExchangeLabels)),
		Phase1:              cipherSuiteModel(wire.Ike),
		Phase2:              cipherSuiteModel(wire.Esp),
	}

	if wire.Left != nil {
		subnet := types.StringNull()
		if len(wire.Left.Subnets) > 0 {
			subnet = types.StringValue(wire.Left.Subnets[0])
		}
		ipsec.JamfSide = &JamfSideModel{
			Host:                          types.StringValue(wire.Left.Host),
			IKEDomainID:                   types.StringValue(wire.Left.ID),
			Subnet:                        subnet,
			AuthenticationSecret:          priorSecret,
			AuthenticationSecretWoVersion: priorWoVersion,
			AuthMethod:                    types.StringValue(wire.Left.Auth),
		}
	}

	if wire.Right != nil {
		subnets, subnetDiags := types.SetValueFrom(ctx, types.StringType, wire.Right.Subnets)
		diags.Append(subnetDiags...)
		ipsec.CustomerSide = &CustomerSideModel{
			Host:        types.StringValue(wire.Right.Host),
			IKEDomainID: types.StringValue(wire.Right.ID),
			Subnets:     subnets,
			Vendor:      types.StringValue(wire.Right.Vendor),
			AuthMethod:  types.StringValue(wire.Right.Auth),
		}
	}

	state.IPSec = ipsec
	return diags
}

// cipherSuiteModel collapses one wire cipher-suite phase into the single
// admin-UI-labelled values the schema exposes. The wire arrays hold exactly one
// element each; an empty array yields null rather than an index panic.
func cipherSuiteModel(wire *securitycloud.CipherSuiteConfig) *CipherSuiteModel {
	if wire == nil {
		return nil
	}
	return &CipherSuiteModel{
		Encryption:         firstLabelOrNull(wire.Encryption, encryptionLabels),
		Integrity:          firstLabelOrNull(wire.Integrity, integrityLabels),
		DiffieHellmanGroup: firstLabelOrNull(wire.DhGroups, dhGroupLabels),
		SALifetimeSeconds:  types.Int64Value(wire.LifetimeInSec),
	}
}

// firstLabelOrNull returns the first element of a wire array as its admin-UI
// label, or null when the array is empty.
func firstLabelOrNull(values []string, labels map[string]string) types.String {
	if len(values) == 0 {
		return types.StringNull()
	}
	return types.StringValue(labelFor(values[0], labels))
}

// firstOrNull returns the first element of a wire array as a string value, or
// null when the array is empty.
func firstOrNull(values []string) types.String {
	if len(values) == 0 {
		return types.StringNull()
	}
	return types.StringValue(values[0])
}

// dedicatedEgressIPList renders the server-assigned egress addresses. A gateway
// still provisioning reports none, which is an empty list rather than null so the
// attribute is always readable.
func dedicatedEgressIPList(ctx context.Context, wire *securitycloud.DedicatedIps) (types.List, diag.Diagnostics) {
	if wire == nil || wire.Ips == nil {
		return types.ListValueMust(types.StringType, []attr.Value{}), nil
	}
	return types.ListValueFrom(ctx, types.StringType, *wire.Ips)
}

// statusObjectValue renders the read-only status block.
func statusObjectValue(wire *securitycloud.GatewayStatus) (types.Object, diag.Diagnostics) {
	if wire == nil {
		return types.ObjectNull(statusAttributeTypes), nil
	}
	tunnelState := types.StringNull()
	if wire.TunnelState != nil {
		tunnelState = types.StringValue(*wire.TunnelState)
	}
	return types.ObjectValue(statusAttributeTypes, map[string]attr.Value{
		"state":        types.StringValue(wire.State),
		"tunnel_state": tunnelState,
	})
}

// assignGatewayDataSourceModel populates the singular data source model from a
// Gateway response.
func assignGatewayDataSourceModel(ctx context.Context, state *GatewayDataSourceModel, g *securitycloud.Gateway) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(g.ID)
	state.Name = types.StringValue(g.Name)
	state.EgressRegion = types.StringValue(labelFor(g.Datacenter, datacenterLabels))
	state.Enabled = types.BoolValue(g.Enabled)

	contact, contactDiags := contactObjectValue(g.Contact)
	diags.Append(contactDiags...)
	state.Contact = contact

	tenantIDs, tenantDiags := types.ListValueFrom(ctx, types.StringType, g.TenantIds)
	diags.Append(tenantDiags...)
	state.TenantIDs = tenantIDs

	zones, zoneDiags := types.ListValueFrom(ctx, types.StringType, g.AvailabilityZones)
	diags.Append(zoneDiags...)
	state.IPSecSourceIPAddresses = zones

	state.DedicatedEgressIPsEnabled = types.BoolValue(g.DedicatedIps != nil && g.DedicatedIps.Enabled)

	dedicatedIPs, dedicatedDiags := dedicatedEgressIPList(ctx, g.DedicatedIps)
	diags.Append(dedicatedDiags...)
	state.DedicatedEgressIPAddresses = dedicatedIPs

	ipsec, ipsecDiags := dsIPSecObjectValue(ctx, g.Ipsec)
	diags.Append(ipsecDiags...)
	state.IPSec = ipsec

	status, statusDiags := statusObjectValue(g.Status)
	diags.Append(statusDiags...)
	state.Status = status

	return diags
}

// buildGatewaysResultModel maps one Gateway response into a plural data source
// result element.
func buildGatewaysResultModel(ctx context.Context, g securitycloud.Gateway) (GatewaysDataSourceResultModel, diag.Diagnostics) {
	var ds GatewayDataSourceModel
	diags := assignGatewayDataSourceModel(ctx, &ds, &g)
	return GatewaysDataSourceResultModel{
		ID:                         ds.ID,
		Name:                       ds.Name,
		EgressRegion:               ds.EgressRegion,
		Contact:                    ds.Contact,
		Enabled:                    ds.Enabled,
		TenantIDs:                  ds.TenantIDs,
		IPSecSourceIPAddresses:     ds.IPSecSourceIPAddresses,
		DedicatedEgressIPsEnabled:  ds.DedicatedEgressIPsEnabled,
		DedicatedEgressIPAddresses: ds.DedicatedEgressIPAddresses,
		IPSec:                      ds.IPSec,
		Status:                     ds.Status,
	}, diags
}

// contactObjectValue renders the contact block for the data sources.
func contactObjectValue(wire *securitycloud.GatewayContact) (types.Object, diag.Diagnostics) {
	if wire == nil {
		return types.ObjectNull(contactAttributeTypes), nil
	}
	return types.ObjectValue(contactAttributeTypes, map[string]attr.Value{
		"name":  types.StringValue(wire.Name),
		"email": types.StringValue(wire.Email),
	})
}

// dsIPSecObjectValue renders the IPsec block for the data sources, which carry no
// pre-shared key because the wire never returns one.
func dsIPSecObjectValue(ctx context.Context, wire *securitycloud.GatewayIpSec) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	if wire == nil {
		return types.ObjectNull(dsIPSecAttributeTypes), diags
	}

	ike, ikeDiags := dsCipherSuiteObjectValue(wire.Ike)
	diags.Append(ikeDiags...)
	esp, espDiags := dsCipherSuiteObjectValue(wire.Esp)
	diags.Append(espDiags...)

	jamfSide := types.ObjectNull(dsJamfSideAttributeTypes)
	if wire.Left != nil {
		subnet := types.StringNull()
		if len(wire.Left.Subnets) > 0 {
			subnet = types.StringValue(wire.Left.Subnets[0])
		}
		obj, objDiags := types.ObjectValue(dsJamfSideAttributeTypes, map[string]attr.Value{
			"host":          types.StringValue(wire.Left.Host),
			"ike_domain_id": types.StringValue(wire.Left.ID),
			"subnet":        subnet,
			"auth_method":   types.StringValue(wire.Left.Auth),
		})
		diags.Append(objDiags...)
		jamfSide = obj
	}

	customerSide := types.ObjectNull(dsCustomerSideAttributeTypes)
	if wire.Right != nil {
		subnets, subnetDiags := types.ListValueFrom(ctx, types.StringType, wire.Right.Subnets)
		diags.Append(subnetDiags...)
		obj, objDiags := types.ObjectValue(dsCustomerSideAttributeTypes, map[string]attr.Value{
			"host":          types.StringValue(wire.Right.Host),
			"ike_domain_id": types.StringValue(wire.Right.ID),
			"subnets":       subnets,
			"vendor":        types.StringValue(wire.Right.Vendor),
			"auth_method":   types.StringValue(wire.Right.Auth),
		})
		diags.Append(objDiags...)
		customerSide = obj
	}

	obj, objDiags := types.ObjectValue(dsIPSecAttributeTypes, map[string]attr.Value{
		"key_exchange_protocol": types.StringValue(labelFor(wire.KeyExchange, keyExchangeLabels)),
		"phase_1":               ike,
		"phase_2":               esp,
		"jamf_side":             jamfSide,
		"customer_side":         customerSide,
	})
	diags.Append(objDiags...)
	return obj, diags
}

// dsCipherSuiteObjectValue renders one cipher-suite phase for the data sources.
func dsCipherSuiteObjectValue(wire *securitycloud.CipherSuiteConfig) (types.Object, diag.Diagnostics) {
	if wire == nil {
		return types.ObjectNull(cipherSuiteAttributeTypes), nil
	}
	return types.ObjectValue(cipherSuiteAttributeTypes, map[string]attr.Value{
		"encryption":           firstLabelOrNull(wire.Encryption, encryptionLabels),
		"integrity":            firstLabelOrNull(wire.Integrity, integrityLabels),
		"diffie_hellman_group": firstLabelOrNull(wire.DhGroups, dhGroupLabels),
		"sa_lifetime_seconds":  types.Int64Value(wire.LifetimeInSec),
	})
}
