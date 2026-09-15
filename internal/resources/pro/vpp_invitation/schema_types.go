// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// vppInvitationTimeoutAttributeTypes defines the timeout attribute types for the
// resource operations.
var vppInvitationTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// usageAttrTypes is the attribute-type map for one read-only invitation_usages
// element. Mirrors VPPInvitationUsageModel.
var usageAttrTypes = map[string]attr.Type{
	"id":                     types.StringType,
	"name":                   types.StringType,
	"email_address":          types.StringType,
	"status":                 types.StringType,
	"last_action_date_utc":   types.StringType,
	"last_action_date_epoch": types.StringType,
	"vpp_account":            types.StringType,
}

// usageObjectType is the element type for the Computed invitation_usages list.
var usageObjectType = types.ObjectType{AttrTypes: usageAttrTypes}

// distributionMethods is the exact set of accepted general.distribution_method
// wire strings (enumerated by probe — the classic endpoint rejects anything else
// with 409 "Invalid distribution method"). "Send emails" additionally requires
// the sender_name / sender_email_address / subject / message fields.
var distributionMethods = []string{
	proclassic.VppInvitationGeneralDistributionMethodPromptUsersToAcceptMakeAvailableInSelfService,
	proclassic.VppInvitationGeneralDistributionMethodMakeAvailableInSelfServiceOnly,
	proclassic.VppInvitationGeneralDistributionMethodSendEmails,
}

// distributionMethodSendEmails is the email-dispatch distribution method that
// gates the sender_* / subject / message / require_login fields.
const distributionMethodSendEmails = proclassic.VppInvitationGeneralDistributionMethodSendEmails
