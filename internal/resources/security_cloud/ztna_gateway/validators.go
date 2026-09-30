// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// privateRange is one of the three RFC 1918 blocks Jamf Security Cloud accepts
// for the Jamf-side encryption domain, with the prefix lengths it allows inside
// it.
type privateRange struct {
	cidr      string
	minPrefix int
	maxPrefix int
}

// jamfSidePrivateRanges is the accepted set for the Jamf-side subnet, quoted from
// the server's own rejection message (wire-probed 2026-08-27): "IPSec left subnet
// must be a private range: 10.0.0.0/8 (/8–/30), 172.16.0.0/12 (/12–/30) or
// 192.168.0.0/16 (/16–/30)."
var jamfSidePrivateRanges = []privateRange{
	{cidr: "10.0.0.0/8", minPrefix: 8, maxPrefix: 30},
	{cidr: "172.16.0.0/12", minPrefix: 12, maxPrefix: 30},
	{cidr: "192.168.0.0/16", minPrefix: 16, maxPrefix: 30},
}

// cidrBlock returns a validator.String enforcing IPv4 CIDR notation on a customer
// subnet.
//
// Why plan time: an invalid subnet reaches the user as
// `400 [INVALID_FIELD] ipsec: IPSec configuration is not valid.` — the whole
// block named, nothing about which address or why (wire-probed 2026-08-27).
//
// It is the shared commonvalidators grammar rather than a local restatement, so the
// one CIDR parse in the provider governs every Security Cloud attribute that takes a
// range. The local version this replaced had drifted: it trusted net.ParseCIDR's
// `To4`, which is non-nil for an IPv4-mapped IPv6 block, so `::ffff:10.0.0.0/104`
// passed plan time and reached the opaque `ipsec` rejection above.
//
// It is the AllowingHostBits variant deliberately. The stricter IPv4CIDR also refuses
// a range carrying host bits, but that rule is a wire fact probed on the ZTNA app's
// `bareIps` endpoint and on no other. This construct is already shipped, and its IPsec
// endpoint has never been probed for canonicalisation, so applying the rule here would
// refuse at plan time a configuration that may apply cleanly today. Probe it — write
// `10.10.0.1/16` and read the subnet back — and if it canonicalises, switch to
// IPv4CIDR and drop the variant.
func cidrBlock() validator.String {
	return commonvalidators.IPv4CIDRAllowingHostBits()
}

// privateCIDRValidator checks that a string attribute holds an IPv4 CIDR block
// inside one of the private ranges Jamf Security Cloud accepts for the Jamf-side
// encryption domain, with a prefix length that range allows.
type privateCIDRValidator struct{}

// privateCIDR returns a validator.String enforcing the Jamf-side subnet rules.
//
// The prefix bounds matter as much as the range: the server accepts
// `172.16.0.0/12` but not `172.16.0.0/31`, and rejects both an out-of-range
// address and an out-of-bounds prefix with the same message.
//
// The grammar comes from commonvalidators.ParseIPv4CIDR, so the shape rules are the
// same ones cidrBlock applies; only the range and prefix bounds are this gateway's
// own.
func privateCIDR() validator.String {
	return privateCIDRValidator{}
}

// Description returns a plain-text description of the validator.
func (privateCIDRValidator) Description(_ context.Context) string {
	return "must be a private IPv4 CIDR block: 10.0.0.0/8 (/8-/30), 172.16.0.0/12 (/12-/30) or 192.168.0.0/16 (/16-/30)"
}

// MarkdownDescription returns the markdown description of the validator.
func (v privateCIDRValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (v privateCIDRValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()

	network, prefix, err := commonvalidators.ParseIPv4CIDR(value)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Jamf Security Cloud subnet",
			helpers.APIErrorDetail(err)+" Got: "+value,
		)
		return
	}

	for _, r := range jamfSidePrivateRanges {
		rangeNetwork, _, rangeErr := commonvalidators.ParseIPv4CIDR(r.cidr)
		if rangeErr != nil || !rangeNetwork.Contains(network.IP) {
			continue
		}
		if prefix < r.minPrefix || prefix > r.maxPrefix {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Jamf Security Cloud subnet prefix out of range",
				fmt.Sprintf("%s allows a prefix between /%d and /%d. Got: /%d.", r.cidr, r.minPrefix, r.maxPrefix, prefix),
			)
		}
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Jamf Security Cloud subnet is not a private range",
		"Jamf Security Cloud requires this subnet to fall inside 10.0.0.0/8 (/8-/30), 172.16.0.0/12 (/12-/30) or "+
			"192.168.0.0/16 (/16-/30). Got: "+value,
	)
}

// ipsecSourceAddressesValidator enforces that ipsec_source_ip_addresses is set on
// an IPsec gateway and only there.
//
// The server refuses addresses on a dedicated internet gateway with `400
// [INVALID_FIELD] availabilityZones: availabilityZones must be empty when
// dedicatedIps.enabled is true.` Because the provider derives that flag from the
// absence of the `ipsec` block, the equivalent config-level rule is: source
// addresses require `ipsec`.
//
// The converse holds too. Wire-probed on the EU gateway, 2026-09-30: an IPsec
// create omitting availabilityZones, or sending `[]`, is refused `400
// [INVALID_FIELD] availabilityZones must contain at least one address.`, and a
// PATCH sending `[]` or `null` to an existing IPsec gateway is refused `400
// EMPTY_AVAILABILITY_ZONES_NOT_SUPPORTED`. So an IPsec gateway can never be
// without addresses, and an unknown value is let through for the apply to settle.
// Earlier probing on 2026-08-27 had created IPsec gateways with none; that no
// longer works, and nothing in the specification marked the change.
type ipsecSourceAddressesValidator struct{}

var _ resource.ConfigValidator = ipsecSourceAddressesValidator{}

// Description returns a plain-text description of the validator.
func (ipsecSourceAddressesValidator) Description(_ context.Context) string {
	return "ipsec_source_ip_addresses is required with the ipsec block and not allowed without it"
}

// MarkdownDescription returns the markdown description of the validator.
func (v ipsecSourceAddressesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements resource.ConfigValidator.
func (v ipsecSourceAddressesValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config GatewayResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.IPSecSourceIPAddresses.IsUnknown() {
		return
	}
	if config.IPSec != nil {
		if config.IPSecSourceIPAddresses.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("ipsec_source_ip_addresses"),
				"An IPsec gateway needs source addresses",
				"Jamf Security Cloud refuses an IPsec gateway without source addresses. Set "+
					"`ipsec_source_ip_addresses` to one or both addresses the admin UI lists for this gateway's "+
					"egress region.",
			)
		}
		return
	}
	if config.IPSecSourceIPAddresses.IsNull() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("ipsec_source_ip_addresses"),
		"IPsec source addresses require an IPsec gateway",
		"`ipsec_source_ip_addresses` names the addresses IPsec traffic originates from, so it only applies to a "+
			"dedicated IPsec gateway. This gateway has no `ipsec` block, which makes it a dedicated internet "+
			"gateway — remove `ipsec_source_ip_addresses`, or add the `ipsec` block.",
	)
}
