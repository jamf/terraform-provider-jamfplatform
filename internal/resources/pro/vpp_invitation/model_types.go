// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// VPPInvitationResourceModel is the Terraform resource model for a Jamf Pro VPP
// invitation (the classic /vppinvitations endpoint — user-based Volume
// Purchasing). General-tab scalars are flattened to the top level (webhook
// precedent); scope is the user-based subset; invitation_usages is a read-only
// server-tracked list.
//
// Wire mapping notes (UI label ← wire name):
//   - name                 → general.name                       (UI "Display Name")
//   - vpp_account_id       → general.vpp_account.id             (UI "Location")
//   - distribution_method  → general.distribution_method
//   - auto_register_managed_users → general.auto_register_managed_users (server default true)
//   - sender_name / sender_email_address / subject / message / require_login →
//     general.* — email-mode only ("Send emails"); absent on the wire in
//     Self-Service modes.
type VPPInvitationResourceModel struct {
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	VPPAccountID             types.String `tfsdk:"vpp_account_id"`
	DistributionMethod       types.String `tfsdk:"distribution_method"`
	AutoRegisterManagedUsers types.Bool   `tfsdk:"auto_register_managed_users"`

	SenderName         types.String `tfsdk:"sender_name"`
	SenderEmailAddress types.String `tfsdk:"sender_email_address"`
	Subject            types.String `tfsdk:"subject"`
	Message            types.String `tfsdk:"message"`
	RequireLogin       types.Bool   `tfsdk:"require_login"`

	Scope            *scope.UserScopeModel  `tfsdk:"scope"`
	InvitationUsages types.List             `tfsdk:"invitation_usages"` // Computed list of VPPInvitationUsageModel
	Timeouts         resourceTimeouts.Value `tfsdk:"timeouts"`
}

// VPPInvitationUsageModel is a read-only invitation_usages element — per-user
// registration status the server tracks. Never written.
type VPPInvitationUsageModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	EmailAddress        types.String `tfsdk:"email_address"`
	Status              types.String `tfsdk:"status"`
	LastActionDateUTC   types.String `tfsdk:"last_action_date_utc"`
	LastActionDateEpoch types.String `tfsdk:"last_action_date_epoch"`
	VPPAccount          types.String `tfsdk:"vpp_account"`
}

// VPPInvitationDataSourceModel is the data source model. Lookup by exactly one
// of id / name.
type VPPInvitationDataSourceModel struct {
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	VPPAccountID             types.String `tfsdk:"vpp_account_id"`
	DistributionMethod       types.String `tfsdk:"distribution_method"`
	AutoRegisterManagedUsers types.Bool   `tfsdk:"auto_register_managed_users"`
	SenderName               types.String `tfsdk:"sender_name"`
	SenderEmailAddress       types.String `tfsdk:"sender_email_address"`
	Subject                  types.String `tfsdk:"subject"`
	Message                  types.String `tfsdk:"message"`
	RequireLogin             types.Bool   `tfsdk:"require_login"`

	Scope            *scope.UserScopeModel    `tfsdk:"scope"`
	InvitationUsages types.List               `tfsdk:"invitation_usages"`
	Timeouts         datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// vppInvitationIdentityModel is the identity object for the resource + list.
type vppInvitationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// VPPInvitationListResourceModel is the list config model. Classic has no RSQL —
// the filter shape is the shared client-side substring block.
type VPPInvitationListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
