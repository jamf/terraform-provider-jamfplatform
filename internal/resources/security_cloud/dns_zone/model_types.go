// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DNSZoneResourceModel represents the Terraform resource model for a Jamf
// Security Cloud custom DNS zone.
type DNSZoneResourceModel struct {
	ID          types.String           `tfsdk:"id"`
	Name        types.String           `tfsdk:"name"`
	Domains     types.Set              `tfsdk:"domains"`
	NameServers types.Set              `tfsdk:"authoritative_name_servers"`
	Timeouts    resourceTimeouts.Value `tfsdk:"timeouts"`
}

// NameServerModel represents one authoritative name server entry.
type NameServerModel struct {
	IP        types.String `tfsdk:"ip_address"`
	GatewayID types.String `tfsdk:"gateway_id"`
}

// dnsZoneIdentityModel represents the identity object for DNS zone resources and
// list results.
type dnsZoneIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DNSZoneDataSourceModel represents the Terraform data source model for a single
// Jamf Security Cloud custom DNS zone. Collections are lists rather than sets
// because data source attributes returning API data are always read-only.
type DNSZoneDataSourceModel struct {
	ID          types.String             `tfsdk:"id"`
	Name        types.String             `tfsdk:"name"`
	Domains     types.List               `tfsdk:"domains"`
	NameServers types.List               `tfsdk:"authoritative_name_servers"`
	Timeouts    datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// DNSZonesDataSourceModel represents the Terraform data source model for the
// plural DNS zone lookup.
type DNSZonesDataSourceModel struct {
	ID       types.String                    `tfsdk:"id"`
	DNSZones []DNSZonesDataSourceResultModel `tfsdk:"dns_zones"`
	Timeouts datasourceTimeouts.Value        `tfsdk:"timeouts"`
}

// DNSZonesDataSourceResultModel represents a single DNS zone in the plural data
// source results.
type DNSZonesDataSourceResultModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Domains     types.List   `tfsdk:"domains"`
	NameServers types.List   `tfsdk:"authoritative_name_servers"`
}

// DNSZoneListResourceModel represents the config model for DNS zone list
// queries. Jamf Security Cloud exposes no filter parameters on the zone list
// endpoint, so the model carries only the `timeouts` attribute.
type DNSZoneListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
