// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DomainResourceModel represents the Terraform resource model for a Jamf Account
// SSO domain claim.
type DomainResourceModel struct {
	ID                    types.String           `tfsdk:"id"`
	Domain                types.String           `tfsdk:"domain"`
	VerificationStatus    types.String           `tfsdk:"verification_status"`
	VerificationKey       types.String           `tfsdk:"verification_key"`
	VerificationTXTRecord types.String           `tfsdk:"verification_txt_record"`
	ParentDomainID        types.String           `tfsdk:"parent_domain_id"`
	Shared                types.Bool             `tfsdk:"shared"`
	AccountID             types.String           `tfsdk:"account_id"`
	CreatedBy             types.String           `tfsdk:"created_by"`
	CreatedAt             types.String           `tfsdk:"created_at"`
	LastModifiedAt        types.String           `tfsdk:"last_modified_at"`
	LastVerifiedAt        types.String           `tfsdk:"last_verified_at"`
	VerificationExpiresAt types.String           `tfsdk:"verification_expires_at"`
	Timeouts              resourceTimeouts.Value `tfsdk:"timeouts"`
}

// domainIdentityModel represents the identity object for SSO domain resources
// and list results.
//
// The identity is the domain name, not the Jamf-assigned ID. Two facts force
// that choice: a claim can be read back only by scanning the domain collection,
// which matches on the name, and withdrawing a claim then making it again mints
// a fresh ID — so the name is the stable handle and the ID is not.
type domainIdentityModel struct {
	Domain types.String `tfsdk:"domain"`
}

// DomainDataSourceModel represents the Terraform data source model for a single
// Jamf Account SSO domain. Collections are lists rather than sets because data
// source attributes returning Jamf's own data are always read-only.
type DomainDataSourceModel struct {
	ID                    types.String             `tfsdk:"id"`
	Domain                types.String             `tfsdk:"domain"`
	VerificationStatus    types.String             `tfsdk:"verification_status"`
	VerificationKey       types.String             `tfsdk:"verification_key"`
	VerificationTXTRecord types.String             `tfsdk:"verification_txt_record"`
	ParentDomainID        types.String             `tfsdk:"parent_domain_id"`
	Shared                types.Bool               `tfsdk:"shared"`
	AccountID             types.String             `tfsdk:"account_id"`
	CreatedBy             types.String             `tfsdk:"created_by"`
	CreatedAt             types.String             `tfsdk:"created_at"`
	LastModifiedAt        types.String             `tfsdk:"last_modified_at"`
	LastVerifiedAt        types.String             `tfsdk:"last_verified_at"`
	VerificationExpiresAt types.String             `tfsdk:"verification_expires_at"`
	AssignedConnections   types.List               `tfsdk:"assigned_connections"`
	JamfIDEnabled         types.Bool               `tfsdk:"jamf_id_enabled"`
	Timeouts              datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// DomainsDataSourceModel represents the Terraform data source model for the
// plural SSO domain lookup.
type DomainsDataSourceModel struct {
	ID         types.String                   `tfsdk:"id"`
	SSODomains []DomainsDataSourceResultModel `tfsdk:"sso_domains"`
	Timeouts   datasourceTimeouts.Value       `tfsdk:"timeouts"`
}

// DomainsDataSourceResultModel represents a single SSO domain in the plural data
// source results. It carries no assignment information: the assignment lookup is
// keyed on one domain name at a time, so including it here would mean one extra
// round trip per domain in the organization.
type DomainsDataSourceResultModel struct {
	ID                    types.String `tfsdk:"id"`
	Domain                types.String `tfsdk:"domain"`
	VerificationStatus    types.String `tfsdk:"verification_status"`
	VerificationKey       types.String `tfsdk:"verification_key"`
	VerificationTXTRecord types.String `tfsdk:"verification_txt_record"`
	ParentDomainID        types.String `tfsdk:"parent_domain_id"`
	Shared                types.Bool   `tfsdk:"shared"`
	AccountID             types.String `tfsdk:"account_id"`
	CreatedBy             types.String `tfsdk:"created_by"`
	CreatedAt             types.String `tfsdk:"created_at"`
	LastModifiedAt        types.String `tfsdk:"last_modified_at"`
	LastVerifiedAt        types.String `tfsdk:"last_verified_at"`
	VerificationExpiresAt types.String `tfsdk:"verification_expires_at"`
}

// AssignedConnectionModel represents one SSO connection an SSO domain is
// assigned to.
type AssignedConnectionModel struct {
	ConnectionID             types.String `tfsdk:"connection_id"`
	ConnectionOrganizationID types.String `tfsdk:"connection_organization_id"`
	Region                   types.String `tfsdk:"region"`
}

// DomainListResourceModel represents the config model for SSO domain list
// queries. Jamf Account exposes no filter parameters on the domain collection,
// so the model carries only the list-level timeouts.
type DomainListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
