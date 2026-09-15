// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildGatewayCreateInput converts the Terraform plan into the create payload.
//
// `config` carries the WriteOnly pre-shared key, which the framework nullifies in
// the plan, so both are needed.
//
// The dedicated-egress-IP flag is derived rather than configured: the API demands
// exactly one of it or `ipsec`, so an absent `ipsec` block means an internet
// gateway and the flag goes out as true. Sending it as false alongside `ipsec`
// would be equivalent, but the server materialises that itself and echoing it
// back would be one more field to keep in step.
func buildGatewayCreateInput(ctx context.Context, plan, config GatewayResourceModel) (*securitycloud.GatewayCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	tenantIDs := make([]string, 0, len(plan.TenantIDs.Elements()))
	diags.Append(plan.TenantIDs.ElementsAs(ctx, &tenantIDs, false)...)

	enabled := plan.Enabled.ValueBool()
	req := &securitycloud.GatewayCreateRequest{
		Name:       plan.Name.ValueString(),
		Datacenter: wireValueFor(plan.EgressRegion.ValueString(), datacenterLabels),
		Contact:    buildContactInput(plan.Contact),
		Enabled:    &enabled,
		TenantIds:  tenantIDs,
	}

	if plan.IPSec == nil {
		req.DedicatedIps = &securitycloud.DedicatedIps{Enabled: true}
		if diags.HasError() {
			return nil, diags
		}
		return req, diags
	}

	if !plan.IPSecSourceIPAddresses.IsNull() && !plan.IPSecSourceIPAddresses.IsUnknown() {
		zones := make([]string, 0, len(plan.IPSecSourceIPAddresses.Elements()))
		diags.Append(plan.IPSecSourceIPAddresses.ElementsAs(ctx, &zones, false)...)
		req.AvailabilityZones = &zones
	}

	ipsec, ipsecDiags := buildIPSecCreateInput(ctx, plan.IPSec, configIPSec(config))
	diags.Append(ipsecDiags...)
	if diags.HasError() {
		return nil, diags
	}
	req.Ipsec = ipsec

	return req, diags
}

// buildGatewayPatchInput converts the Terraform plan into the update payload.
//
// Every mutable field is sent on every update. The endpoint is a merge patch
// where omission preserves, so a subset write would also work — but the schema
// makes each of these either Required or defaulted, so the plan always describes
// a complete desired state and there is no partial case to model. The one
// exception is the pre-shared key, which is genuinely conditional: it goes on the
// wire only when its rotation trigger changed, because it cannot be read back and
// re-sending it on every update would mean the config, not Jamf, silently
// deciding when the key rotates.
func buildGatewayPatchInput(ctx context.Context, plan, state, config GatewayResourceModel) (*securitycloud.GatewayPatchRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	tenantIDs := make([]string, 0, len(plan.TenantIDs.Elements()))
	diags.Append(plan.TenantIDs.ElementsAs(ctx, &tenantIDs, false)...)

	name := plan.Name.ValueString()
	datacenter := wireValueFor(plan.EgressRegion.ValueString(), datacenterLabels)
	enabled := plan.Enabled.ValueBool()

	req := &securitycloud.GatewayPatchRequest{
		Name:       &name,
		Datacenter: &datacenter,
		Contact:    new(buildContactInput(plan.Contact)),
		Enabled:    &enabled,
		TenantIds:  &tenantIDs,
	}

	if plan.IPSec == nil {
		if diags.HasError() {
			return nil, diags
		}
		return req, diags
	}

	if plan.IPSecSourceIPAddresses.IsNull() || plan.IPSecSourceIPAddresses.IsUnknown() {
		empty := []string{}
		req.AvailabilityZones = &empty
	} else {
		zones := make([]string, 0, len(plan.IPSecSourceIPAddresses.Elements()))
		diags.Append(plan.IPSecSourceIPAddresses.ElementsAs(ctx, &zones, false)...)
		req.AvailabilityZones = &zones
	}

	ipsec, ipsecDiags := buildIPSecPatchInput(ctx, plan.IPSec, statePriorIPSec(state), configIPSec(config))
	diags.Append(ipsecDiags...)
	if diags.HasError() {
		return nil, diags
	}
	req.Ipsec = ipsec

	return req, diags
}

// buildContactInput converts the contact model into the wire shape. A nil model
// yields the zero contact, which the server refuses — the schema makes the block
// Required so that cannot arrive from a valid plan.
func buildContactInput(contact *ContactModel) securitycloud.GatewayContact {
	if contact == nil {
		return securitycloud.GatewayContact{}
	}
	return securitycloud.GatewayContact{
		Name:  contact.Name.ValueString(),
		Email: contact.Email.ValueString(),
	}
}

// buildIPSecCreateInput converts the IPsec model into the create payload.
func buildIPSecCreateInput(ctx context.Context, plan *IPSecModel, config *IPSecModel) (*securitycloud.GatewayIpSecRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	subnets, subnetDiags := customerSubnets(ctx, plan.CustomerSide)
	diags.Append(subnetDiags...)
	if diags.HasError() {
		return nil, diags
	}

	req := &securitycloud.GatewayIpSecRequest{
		KeyExchange: wireValueFor(plan.KeyExchangeProtocol.ValueString(), keyExchangeLabels),
		Ike:         buildCipherSuiteInput(plan.Phase1),
		Esp:         buildCipherSuiteInput(plan.Phase2),
		Left: securitycloud.ConnectionConfigLeftRequest{
			Host:    plan.JamfSide.Host.ValueString(),
			ID:      plan.JamfSide.IKEDomainID.ValueString(),
			Subnets: []string{plan.JamfSide.Subnet.ValueString()},
		},
		Right: securitycloud.ConnectionConfigRightRequest{
			Host:    plan.CustomerSide.Host.ValueString(),
			ID:      plan.CustomerSide.IKEDomainID.ValueString(),
			Subnets: subnets,
			Vendor:  plan.CustomerSide.Vendor.ValueString(),
		},
	}

	if secret := configSharedSecret(config); secret != "" {
		req.Left.Secret = &secret
	}

	return req, diags
}

// buildIPSecPatchInput converts the IPsec model into the update payload, carrying
// the pre-shared key only when its rotation trigger moved.
func buildIPSecPatchInput(ctx context.Context, plan, prior, config *IPSecModel) (*securitycloud.GatewayIpSecPatchRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	subnets, subnetDiags := customerSubnets(ctx, plan.CustomerSide)
	diags.Append(subnetDiags...)
	if diags.HasError() {
		return nil, diags
	}

	keyExchange := wireValueFor(plan.KeyExchangeProtocol.ValueString(), keyExchangeLabels)
	jamfHost := plan.JamfSide.Host.ValueString()
	jamfID := plan.JamfSide.IKEDomainID.ValueString()
	jamfSubnets := []string{plan.JamfSide.Subnet.ValueString()}
	customerHost := plan.CustomerSide.Host.ValueString()
	customerID := plan.CustomerSide.IKEDomainID.ValueString()
	vendor := plan.CustomerSide.Vendor.ValueString()

	ike := buildCipherSuiteInput(plan.Phase1)
	esp := buildCipherSuiteInput(plan.Phase2)

	req := &securitycloud.GatewayIpSecPatchRequest{
		KeyExchange: &keyExchange,
		Ike:         &ike,
		Esp:         &esp,
		Left: &securitycloud.ConnectionConfigPatchLeftRequest{
			Host:    &jamfHost,
			ID:      &jamfID,
			Subnets: &jamfSubnets,
		},
		Right: &securitycloud.ConnectionConfigPatchRightRequest{
			Host:    &customerHost,
			ID:      &customerID,
			Subnets: &subnets,
			Vendor:  &vendor,
		},
	}

	if sharedSecretRotated(plan, prior) {
		if secret := configSharedSecret(config); secret != "" {
			req.Left.Secret = &secret
		}
	}

	return req, diags
}

// buildCipherSuiteInput converts one cipher-suite phase into the wire shape. Each
// algorithm is translated from its admin-UI label to the stored value and becomes
// a single-element array, which is the only size the server accepts.
func buildCipherSuiteInput(suite *CipherSuiteModel) securitycloud.CipherSuiteConfig {
	if suite == nil {
		return securitycloud.CipherSuiteConfig{}
	}
	return securitycloud.CipherSuiteConfig{
		Encryption:    []string{wireValueFor(suite.Encryption.ValueString(), encryptionLabels)},
		Integrity:     []string{wireValueFor(suite.Integrity.ValueString(), integrityLabels)},
		DhGroups:      []string{wireValueFor(suite.DiffieHellmanGroup.ValueString(), dhGroupLabels)},
		LifetimeInSec: suite.SALifetimeSeconds.ValueInt64(),
	}
}

// customerSubnets extracts the remote-peer subnet list.
func customerSubnets(ctx context.Context, side *CustomerSideModel) ([]string, diag.Diagnostics) {
	if side == nil {
		return nil, nil
	}
	subnets := make([]string, 0, len(side.Subnets.Elements()))
	diags := side.Subnets.ElementsAs(ctx, &subnets, false)
	return subnets, diags
}

// sharedSecretRotated reports whether the rotation trigger moved between the
// prior state and the plan. A null trigger on both sides means "leave the stored
// key alone"; the very first update after an import has no prior IPsec block at
// all, which counts as a rotation so an imported gateway can be given its key.
func sharedSecretRotated(plan, prior *IPSecModel) bool {
	if plan == nil || plan.JamfSide == nil {
		return false
	}
	if prior == nil || prior.JamfSide == nil {
		return true
	}
	return !plan.JamfSide.AuthenticationSecretWoVersion.Equal(prior.JamfSide.AuthenticationSecretWoVersion)
}

// configSharedSecret reads the WriteOnly pre-shared key out of the config model.
func configSharedSecret(config *IPSecModel) string {
	if config == nil || config.JamfSide == nil {
		return ""
	}
	return config.JamfSide.AuthenticationSecret.ValueString()
}

// configIPSec returns the config model's IPsec block, if any.
func configIPSec(config GatewayResourceModel) *IPSecModel {
	return config.IPSec
}

// statePriorIPSec returns the prior state's IPsec block, if any.
func statePriorIPSec(state GatewayResourceModel) *IPSecModel {
	return state.IPSec
}
