// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildFileShareDistributionPointInput converts the Terraform plan model into
// an SDK DistributionPoint payload used for both Create and Update.
//
// The endpoint merges: a field omitted from the request keeps its stored
// value. Every Optional+Computed field carries UseStateForUnknown, so on
// update the plan already holds the prior value for any field the user
// omitted — helpers.OptionalStringPointer / OptionalBoolPointer / OptionalInt64Pointer
// re-emit it, preserving it. To *clear* an optional string the user sets it to
// an empty string explicitly.
//
// The three plaintext passwords are WriteOnly (req.Plan exposes them as null),
// so the caller sources them from req.Config and decides — per the matching
// `*_wo_version` rotation trigger — whether to thread a non-nil value through.
// When the caller passes nil the password is omitted and the merge keeps the
// stored value.
func buildFileShareDistributionPointInput(plan FileShareDistributionPointResourceModel, readWritePassword, readOnlyPassword, httpsPassword *string) *pro.DistributionPoint {
	in := &pro.DistributionPoint{
		Name:                      plan.Name.ValueString(),
		ServerName:                plan.ServerName.ValueString(),
		FileSharingConnectionType: plan.FileSharingConnectionType.ValueString(),

		Principal:                 helpers.OptionalBoolPointer(plan.Principal),
		BackupDistributionPointID: helpers.OptionalStringPointer(plan.BackupDistributionPointID),
		EnableLoadBalancing:       helpers.OptionalBoolPointer(plan.EnableLoadBalancing),

		HttpsEnabled: helpers.OptionalBoolPointer(plan.HTTPSEnabled),
	}

	// The file-sharing fields are gated by file_sharing_connection_type. Jamf
	// Pro rejects a write that carries them when the type is NONE ("port should
	// be blank when fileSharingConnectionType is NONE") and blanks them on its
	// side when the type becomes NONE. Emit them only for AFP/SMB; when NONE,
	// omit them — the ModifyPlan nulls the corresponding state-backed attributes
	// so the plan matches the server's blanking. (STYLE_GUIDE §discriminator-
	// gated field clearing.)
	switch plan.FileSharingConnectionType.ValueString() {
	case connectionTypeAFP, connectionTypeSMB:
		in.ShareName = helpers.OptionalStringPointer(plan.ShareName)
		in.Port = helpers.OptionalInt64Pointer(plan.Port)
		in.Workgroup = helpers.OptionalStringPointer(plan.Workgroup)
		in.ReadWriteUsername = helpers.OptionalStringPointer(plan.ReadWriteUsername)
		in.ReadWritePassword = readWritePassword
		in.ReadOnlyUsername = helpers.OptionalStringPointer(plan.ReadOnlyUsername)
		in.ReadOnlyPassword = readOnlyPassword
	}

	// The HTTPS sub-fields are gated by https_enabled. Jamf Pro rejects a write
	// that carries httpsPort (and stores blank values for the rest) when HTTPS
	// is disabled — yet it retains those values internally, so on a merge update
	// the Optional+Computed plan would otherwise re-emit a stored httpsPort
	// alongside https_enabled=false and 400. Emit the sub-fields only when HTTPS
	// is on; when off, omit them so the merge leaves the stored values untouched.
	// (STYLE_GUIDE §discriminator-gated field clearing.)
	if plan.HTTPSEnabled.ValueBool() {
		in.HttpsPort = helpers.OptionalInt64Pointer(plan.HTTPSPort)
		in.HttpsContext = helpers.OptionalStringPointer(plan.HTTPSContext)
		in.HttpsSecurityType = helpers.OptionalStringPointer(plan.HTTPSSecurityType)
		in.HttpsUsername = helpers.OptionalStringPointer(plan.HTTPSUsername)
		in.HttpsPassword = httpsPassword
	}

	return in
}
