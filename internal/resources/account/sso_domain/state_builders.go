// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// assignDomainResourceModel populates a resource model from a claim response.
//
// Every field is adopted from Jamf verbatim, with no reconciliation against prior
// state, because `domain` is the only attribute a practitioner sets and Jamf
// stores it unchanged apart from case — which the schema's validator already
// pins. Everything else is read-only lifecycle state, and two of those values
// move without Terraform doing anything: running the verification updates
// `last_modified_at` and pushes `verification_expires_at` out, whether it
// succeeded or not. Reconciling against prior state would hide exactly that.
func assignDomainResourceModel(state *DomainResourceModel, d *account.Domain) {
	state.ID = numberValueOrNull(d.ID)
	state.Domain = types.StringValue(d.Domain)
	state.VerificationStatus = types.StringPointerValue(d.DomainStatus)
	state.VerificationKey = types.StringValue(d.VerificationKey)
	state.VerificationTXTRecord = verificationTXTRecord(d.VerificationKey)
	state.ParentDomainID = numberValueOrNull(d.VerifiedTldID)
	state.Shared = types.BoolValue(d.SharedDomain)
	state.AccountID = types.StringValue(d.AccountID)
	state.CreatedBy = types.StringPointerValue(d.CreatedByName)
	state.CreatedAt = timeValueOrNull(d.CreatedDate)
	state.LastModifiedAt = timeValueOrNull(d.LastModifiedDate)
	state.LastVerifiedAt = timeValueOrNull(d.LastVerificationDate)
	state.VerificationExpiresAt = timeValueOrNull(d.VerificationExpirationDate)
}

// assignDomainDataSourceModel populates the singular data source model from a
// claim and its assignment record.
//
// alloc may be nil, which leaves the assignment attributes null. That is the
// shape a shared domain owned by another organization can produce: the claim is
// listed, and the assignment lookup is about connections in the organization
// that owns it.
func assignDomainDataSourceModel(state *DomainDataSourceModel, d *account.Domain, alloc *account.DomainAllocation) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = numberValueOrNull(d.ID)
	state.Domain = types.StringValue(d.Domain)
	state.VerificationStatus = types.StringPointerValue(d.DomainStatus)
	state.VerificationKey = types.StringValue(d.VerificationKey)
	state.VerificationTXTRecord = verificationTXTRecord(d.VerificationKey)
	state.ParentDomainID = numberValueOrNull(d.VerifiedTldID)
	state.Shared = types.BoolValue(d.SharedDomain)
	state.AccountID = types.StringValue(d.AccountID)
	state.CreatedBy = types.StringPointerValue(d.CreatedByName)
	state.CreatedAt = timeValueOrNull(d.CreatedDate)
	state.LastModifiedAt = timeValueOrNull(d.LastModifiedDate)
	state.LastVerifiedAt = timeValueOrNull(d.LastVerificationDate)
	state.VerificationExpiresAt = timeValueOrNull(d.VerificationExpirationDate)

	if alloc == nil {
		state.AssignedConnections = types.ListNull(assignedConnectionObjectType)
		state.JamfIDEnabled = types.BoolNull()
		return diags
	}

	connections, connDiags := assignedConnectionListValue(alloc.Connections)
	diags.Append(connDiags...)
	state.AssignedConnections = connections
	state.JamfIDEnabled = types.BoolValue(alloc.JamfIDEnabled)

	return diags
}

// buildDomainsResultModel maps one claim into a plural data source result element.
func buildDomainsResultModel(d account.Domain) DomainsDataSourceResultModel {
	return DomainsDataSourceResultModel{
		ID:                    numberValueOrNull(d.ID),
		Domain:                types.StringValue(d.Domain),
		VerificationStatus:    types.StringPointerValue(d.DomainStatus),
		VerificationKey:       types.StringValue(d.VerificationKey),
		VerificationTXTRecord: verificationTXTRecord(d.VerificationKey),
		ParentDomainID:        numberValueOrNull(d.VerifiedTldID),
		Shared:                types.BoolValue(d.SharedDomain),
		AccountID:             types.StringValue(d.AccountID),
		CreatedBy:             types.StringPointerValue(d.CreatedByName),
		CreatedAt:             timeValueOrNull(d.CreatedDate),
		LastModifiedAt:        timeValueOrNull(d.LastModifiedDate),
		LastVerifiedAt:        timeValueOrNull(d.LastVerificationDate),
		VerificationExpiresAt: timeValueOrNull(d.VerificationExpirationDate),
	}
}

// assignedConnectionListValue builds the assigned_connections list.
func assignedConnectionListValue(connections []account.DomainAllocationConnection) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	values := make([]attr.Value, 0, len(connections))
	for _, c := range connections {
		obj, objDiags := types.ObjectValue(assignedConnectionAttributeTypes, map[string]attr.Value{
			"connection_id":              types.StringValue(c.AssignedConnection),
			"connection_organization_id": types.StringValue(c.AssignedConnectionOrgID),
			"region":                     types.StringValue(c.Region),
		})
		diags.Append(objDiags...)
		values = append(values, obj)
	}
	if diags.HasError() {
		return types.ListNull(assignedConnectionObjectType), diags
	}
	list, listDiags := types.ListValue(assignedConnectionObjectType, values)
	diags.Append(listDiags...)
	return list, diags
}
