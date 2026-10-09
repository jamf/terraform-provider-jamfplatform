// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// MobileDeviceInvitationResourceModel is the Terraform resource model for a Jamf
// Pro mobile device enrollment invitation backed by the ProClassic
// /mobiledeviceinvitations endpoint.
//
// The endpoint is create + delete only: there is no usable Update route on the
// wire. The SDK exposes UpdateMobileDeviceInvitationByInvitation, but the server
// rejects PUT (`409 "Put is not supported"`), so it is never called. Every
// user-settable attribute is RequiresReplace. The framework still requires an
// Update method, which is effectively dead code — see crud.go.
//
// Mobile invitations carry a Jamf classic write-name≠read-name asymmetry on two
// boolean fields: `multiple_uses_allowed` is written as `allow_multiple_uses`
// and read back as `multiple_uses_allowed`; `require_login` is written as
// `require_login` and read back as `login_required`. The schema attribute names
// follow the UI / read names.
//
// Server-derived fields (`invitation`, `last_action`, `date_sent`,
// `date_sent_utc`, `date_sent_epoch`, `expiration_date_epoch`,
// `expiration_date_utc`) are Computed-only: never Optional+Computed siblings of
// an input.
type MobileDeviceInvitationResourceModel struct {
	ID                         types.String           `tfsdk:"id"`
	Invitation                 types.String           `tfsdk:"invitation"`
	InvitationType             types.String           `tfsdk:"invitation_type"`
	ExpirationDate             types.String           `tfsdk:"expiration_date"`
	ExpirationDateEpoch        types.String           `tfsdk:"expiration_date_epoch"`
	ExpirationDateUtc          types.String           `tfsdk:"expiration_date_utc"`
	EnrollIntoSiteID           types.String           `tfsdk:"enroll_into_site_id"`
	EnrollIntoSiteName         types.String           `tfsdk:"enroll_into_site_name"`
	KeepExistingSiteMembership types.Bool             `tfsdk:"keep_existing_site_membership"`
	MultipleUsesAllowed        types.Bool             `tfsdk:"multiple_uses_allowed"`
	RequireLogin               types.Bool             `tfsdk:"require_login"`
	Subject                    types.String           `tfsdk:"subject"`
	Message                    types.String           `tfsdk:"message"`
	ReplyTo                    types.String           `tfsdk:"reply_to"`
	SentFrom                   types.String           `tfsdk:"sent_from"`
	SentTo                     types.String           `tfsdk:"sent_to"`
	Username                   types.String           `tfsdk:"username"`
	TargetIos                  types.String           `tfsdk:"target_ios"`
	LastAction                 types.String           `tfsdk:"last_action"`
	DateSent                   types.String           `tfsdk:"date_sent"`
	DateSentUtc                types.String           `tfsdk:"date_sent_utc"`
	DateSentEpoch              types.String           `tfsdk:"date_sent_epoch"`
	Timeouts                   resourceTimeouts.Value `tfsdk:"timeouts"`
}

// MobileDeviceInvitationDataSourceModel is the Terraform data source model.
// Mirrors the resource shape. The /mobiledeviceinvitations endpoint has no name
// lookup that is meaningful for an invitation (there is no name field on the
// wire), so the data source selects by `id` OR by `invitation` (the
// server-minted code shown in the admin UI as the "Invitation ID") — exactly
// one of the two.
type MobileDeviceInvitationDataSourceModel struct {
	ID                         types.String             `tfsdk:"id"`
	Invitation                 types.String             `tfsdk:"invitation"`
	InvitationType             types.String             `tfsdk:"invitation_type"`
	ExpirationDate             types.String             `tfsdk:"expiration_date"`
	ExpirationDateEpoch        types.String             `tfsdk:"expiration_date_epoch"`
	ExpirationDateUtc          types.String             `tfsdk:"expiration_date_utc"`
	EnrollIntoSiteID           types.String             `tfsdk:"enroll_into_site_id"`
	EnrollIntoSiteName         types.String             `tfsdk:"enroll_into_site_name"`
	KeepExistingSiteMembership types.Bool               `tfsdk:"keep_existing_site_membership"`
	MultipleUsesAllowed        types.Bool               `tfsdk:"multiple_uses_allowed"`
	RequireLogin               types.Bool               `tfsdk:"require_login"`
	Subject                    types.String             `tfsdk:"subject"`
	Message                    types.String             `tfsdk:"message"`
	ReplyTo                    types.String             `tfsdk:"reply_to"`
	SentFrom                   types.String             `tfsdk:"sent_from"`
	SentTo                     types.String             `tfsdk:"sent_to"`
	Username                   types.String             `tfsdk:"username"`
	TargetIos                  types.String             `tfsdk:"target_ios"`
	LastAction                 types.String             `tfsdk:"last_action"`
	DateSent                   types.String             `tfsdk:"date_sent"`
	DateSentUtc                types.String             `tfsdk:"date_sent_utc"`
	DateSentEpoch              types.String             `tfsdk:"date_sent_epoch"`
	Timeouts                   datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// mobileDeviceInvitationIdentityModel is the identity object for resource
// imports and list-resource identities. Import / identity key is the numeric
// `id`.
type mobileDeviceInvitationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// MobileDeviceInvitationListResourceModel is the config model for the list
// resource. Mobile device invitations carry no `name` on the wire (the list
// item exposes only id / invitation / invitation_type / expiration /
// last_action), so there is no meaningful client-side substring filter — the
// list resource takes no configuration and returns all invitations.
type MobileDeviceInvitationListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
