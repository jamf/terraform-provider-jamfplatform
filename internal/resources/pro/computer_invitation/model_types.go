// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// ComputerInvitationResourceModel is the Terraform resource model for a Jamf Pro
// computer enrollment invitation backed by the ProClassic /computerinvitations
// endpoint.
//
// The endpoint is create + delete only: there is no Update route on the wire
// (the SDK exposes no update function), so every user-settable attribute is
// RequiresReplace. The framework still requires an Update method, which is
// effectively dead code — see crud.go.
//
// `SSHPassword` is `WriteOnly`: the user-supplied plaintext is sent on Create
// but never persisted in Terraform state. `SSHPasswordWoVersion` is the
// rotation trigger — because there is no Update path, any change to it forces
// a replace (like every other attribute), which re-sends the secret.
//
// Server-derived fields (`invitation`, `invitation_status`, `times_used`,
// `invited_user_uuid`, `expiration_date_epoch`, `expiration_date_utc`) are
// Computed-only: never Optional+Computed siblings of an input.
type ComputerInvitationResourceModel struct {
	ID                          types.String           `tfsdk:"id"`
	Invitation                  types.String           `tfsdk:"invitation"`
	InvitationType              types.String           `tfsdk:"invitation_type"`
	ExpirationDate              types.String           `tfsdk:"expiration_date"`
	ExpirationDateEpoch         types.String           `tfsdk:"expiration_date_epoch"`
	ExpirationDateUtc           types.String           `tfsdk:"expiration_date_utc"`
	EnrollIntoSiteID            types.String           `tfsdk:"enroll_into_site_id"`
	EnrollIntoSiteName          types.String           `tfsdk:"enroll_into_site_name"`
	KeepExistingSiteMembership  types.Bool             `tfsdk:"keep_existing_site_membership"`
	MultipleUsesAllowed         types.Bool             `tfsdk:"multiple_uses_allowed"`
	CreateAccountIfDoesNotExist types.Bool             `tfsdk:"create_account_if_does_not_exist"`
	HideAccount                 types.Bool             `tfsdk:"hide_account"`
	LockDownSSH                 types.Bool             `tfsdk:"lock_down_ssh"`
	SSHUsername                 types.String           `tfsdk:"ssh_username"`
	SSHPassword                 types.String           `tfsdk:"ssh_password"`
	SSHPasswordWoVersion        types.Int64            `tfsdk:"ssh_password_wo_version"`
	InvitationStatus            types.String           `tfsdk:"invitation_status"`
	TimesUsed                   types.Int64            `tfsdk:"times_used"`
	InvitedUserUUID             types.String           `tfsdk:"invited_user_uuid"`
	Timeouts                    resourceTimeouts.Value `tfsdk:"timeouts"`
}

// ComputerInvitationDataSourceModel is the Terraform data source model. Mirrors
// the resource shape minus the WriteOnly secret + its rotation companion. The
// /computerinvitations endpoint has no name lookup that is meaningful for an
// invitation (there is no name field on the wire), so the data source selects
// by `id` OR by `invitation` (the server-minted code shown in the admin UI as
// the "Invitation ID") — exactly one of the two.
type ComputerInvitationDataSourceModel struct {
	ID                          types.String             `tfsdk:"id"`
	Invitation                  types.String             `tfsdk:"invitation"`
	InvitationType              types.String             `tfsdk:"invitation_type"`
	ExpirationDate              types.String             `tfsdk:"expiration_date"`
	ExpirationDateEpoch         types.String             `tfsdk:"expiration_date_epoch"`
	ExpirationDateUtc           types.String             `tfsdk:"expiration_date_utc"`
	EnrollIntoSiteID            types.String             `tfsdk:"enroll_into_site_id"`
	EnrollIntoSiteName          types.String             `tfsdk:"enroll_into_site_name"`
	KeepExistingSiteMembership  types.Bool               `tfsdk:"keep_existing_site_membership"`
	MultipleUsesAllowed         types.Bool               `tfsdk:"multiple_uses_allowed"`
	CreateAccountIfDoesNotExist types.Bool               `tfsdk:"create_account_if_does_not_exist"`
	HideAccount                 types.Bool               `tfsdk:"hide_account"`
	LockDownSSH                 types.Bool               `tfsdk:"lock_down_ssh"`
	SSHUsername                 types.String             `tfsdk:"ssh_username"`
	InvitationStatus            types.String             `tfsdk:"invitation_status"`
	TimesUsed                   types.Int64              `tfsdk:"times_used"`
	InvitedUserUUID             types.String             `tfsdk:"invited_user_uuid"`
	Timeouts                    datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// computerInvitationIdentityModel is the identity object for resource imports
// and list-resource identities. Import / identity key is the numeric `id`.
type computerInvitationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// ComputerInvitationListResourceModel is the config model for the list
// resource. Unlike the inventory list resources, computer invitations carry no
// `name` on the wire (the list item exposes only id / invitation /
// invitation_type / expiration), so there is no meaningful client-side
// substring filter — the list resource takes no configuration and returns all
// invitations.
type ComputerInvitationListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
