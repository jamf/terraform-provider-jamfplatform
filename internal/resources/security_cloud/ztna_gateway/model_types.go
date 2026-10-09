// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// GatewayResourceModel represents the Terraform resource model for a Jamf
// Security Cloud ZTNA gateway.
type GatewayResourceModel struct {
	ID                         types.String           `tfsdk:"id"`
	Name                       types.String           `tfsdk:"name"`
	EgressRegion               types.String           `tfsdk:"egress_region"`
	Contact                    *ContactModel          `tfsdk:"contact"`
	Enabled                    types.Bool             `tfsdk:"enabled"`
	TenantIDs                  types.Set              `tfsdk:"tenant_ids"`
	IPSecSourceIPAddresses     types.Set              `tfsdk:"ipsec_source_ip_addresses"`
	DedicatedEgressIPAddresses types.List             `tfsdk:"dedicated_egress_ip_addresses"`
	IPSec                      *IPSecModel            `tfsdk:"ipsec"`
	Status                     types.Object           `tfsdk:"status"`
	Timeouts                   resourceTimeouts.Value `tfsdk:"timeouts"`
}

// ContactModel represents the operational contact for a gateway.
type ContactModel struct {
	Name  types.String `tfsdk:"name"`
	Email types.String `tfsdk:"email"`
}

// IPSecModel represents the IPsec tunnel configuration of a gateway.
type IPSecModel struct {
	KeyExchangeProtocol types.String       `tfsdk:"key_exchange_protocol"`
	Phase1              *CipherSuiteModel  `tfsdk:"phase_1"`
	Phase2              *CipherSuiteModel  `tfsdk:"phase_2"`
	JamfSide            *JamfSideModel     `tfsdk:"jamf_side"`
	CustomerSide        *CustomerSideModel `tfsdk:"customer_side"`
}

// CipherSuiteModel represents one IPsec cipher-suite phase. The wire carries each
// algorithm as a single-element array; the model holds the one value it accepts.
type CipherSuiteModel struct {
	Encryption         types.String `tfsdk:"encryption"`
	Integrity          types.String `tfsdk:"integrity"`
	DiffieHellmanGroup types.String `tfsdk:"diffie_hellman_group"`
	SALifetimeSeconds  types.Int64  `tfsdk:"sa_lifetime_seconds"`
}

// JamfSideModel represents the Jamf-side tunnel endpoint. Wire object: `left`.
type JamfSideModel struct {
	Host                          types.String `tfsdk:"host"`
	IKEDomainID                   types.String `tfsdk:"ike_domain_id"`
	Subnet                        types.String `tfsdk:"subnet"`
	AuthenticationSecret          types.String `tfsdk:"authentication_secret"`
	AuthenticationSecretWoVersion types.Int64  `tfsdk:"authentication_secret_wo_version"`
	AuthMethod                    types.String `tfsdk:"auth_method"`
}

// CustomerSideModel represents the remote-peer tunnel endpoint. Wire object:
// `right`.
type CustomerSideModel struct {
	Host        types.String `tfsdk:"host"`
	IKEDomainID types.String `tfsdk:"ike_domain_id"`
	Subnets     types.Set    `tfsdk:"subnets"`
	Vendor      types.String `tfsdk:"vendor"`
	AuthMethod  types.String `tfsdk:"auth_method"`
}

// gatewayIdentityModel represents the identity object for gateway resources and
// list results.
type gatewayIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// GatewayDataSourceModel represents the Terraform data source model for a single
// gateway. Collections are lists rather than sets because data source attributes
// returning API data are always read-only.
type GatewayDataSourceModel struct {
	ID                         types.String             `tfsdk:"id"`
	Name                       types.String             `tfsdk:"name"`
	EgressRegion               types.String             `tfsdk:"egress_region"`
	Contact                    types.Object             `tfsdk:"contact"`
	Enabled                    types.Bool               `tfsdk:"enabled"`
	TenantIDs                  types.List               `tfsdk:"tenant_ids"`
	IPSecSourceIPAddresses     types.List               `tfsdk:"ipsec_source_ip_addresses"`
	DedicatedEgressIPsEnabled  types.Bool               `tfsdk:"dedicated_egress_ips_enabled"`
	DedicatedEgressIPAddresses types.List               `tfsdk:"dedicated_egress_ip_addresses"`
	IPSec                      types.Object             `tfsdk:"ipsec"`
	Status                     types.Object             `tfsdk:"status"`
	Timeouts                   datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// GatewaysDataSourceModel represents the Terraform data source model for the
// plural gateway lookup.
type GatewaysDataSourceModel struct {
	ID       types.String                    `tfsdk:"id"`
	Gateways []GatewaysDataSourceResultModel `tfsdk:"gateways"`
	Timeouts datasourceTimeouts.Value        `tfsdk:"timeouts"`
}

// GatewaysDataSourceResultModel represents a single gateway in the plural data
// source results.
type GatewaysDataSourceResultModel struct {
	ID                         types.String `tfsdk:"id"`
	Name                       types.String `tfsdk:"name"`
	EgressRegion               types.String `tfsdk:"egress_region"`
	Contact                    types.Object `tfsdk:"contact"`
	Enabled                    types.Bool   `tfsdk:"enabled"`
	TenantIDs                  types.List   `tfsdk:"tenant_ids"`
	IPSecSourceIPAddresses     types.List   `tfsdk:"ipsec_source_ip_addresses"`
	DedicatedEgressIPsEnabled  types.Bool   `tfsdk:"dedicated_egress_ips_enabled"`
	DedicatedEgressIPAddresses types.List   `tfsdk:"dedicated_egress_ip_addresses"`
	IPSec                      types.Object `tfsdk:"ipsec"`
	Status                     types.Object `tfsdk:"status"`
}

// GatewayListResourceModel represents the config model for gateway list queries.
// Jamf Security Cloud exposes no query parameters on the gateway list endpoint,
// so the model carries only the `timeouts` attribute.
type GatewayListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
