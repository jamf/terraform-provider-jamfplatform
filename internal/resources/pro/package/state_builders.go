// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignPackageResourceModel populates state from a Package response. The
// upload-source attrs (`package_file_source`, `package_file_source_checksum`,
// `manifest_file_source`) are provider-internal inputs with no wire echo —
// they're left untouched so the caller's pre-existing plan value is
// preserved.
//
// Every server-defaulted *string field uses ReconcileOptionalStringPointer
// per §12.2: the server returns `""` when the field is unset, and we want
// state to land on null in that case rather than carrying a tombstone empty
// string the next plan would treat as drift.
func assignPackageResourceModel(state *PackageResourceModel, p *pro.Package) diag.Diagnostics {
	if p == nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Nil package response",
				"The Jamf Pro client returned a nil Package without an error; the provider cannot map this to Terraform state.",
			),
		}
	}

	if p.ID != nil {
		state.ID = types.StringValue(*p.ID)
	}
	state.DisplayName = types.StringValue(p.PackageName)
	state.FileName = types.StringValue(p.FileName)

	// CategoryID is a value-type string in the SDK (NOT a pointer). Server
	// always returns either the user value or the `"-1"` sentinel.
	state.CategoryID = types.StringValue(p.CategoryID)

	state.Info = helpers.ReconcileOptionalStringPointer(p.Info, state.Info)
	state.Notes = helpers.ReconcileOptionalStringPointer(p.Notes, state.Notes)

	// Priority is value-type int — always reflected.
	state.Priority = types.Int64Value(int64(p.Priority))

	// Value-type bools: server always returns the boolean, reflect directly.
	state.FillUserTemplate = types.BoolValue(p.FillUserTemplate)
	state.RebootRequired = types.BoolValue(p.RebootRequired)

	// Pointer bools: reconcile so server `nil` collapses to null.
	state.FillExistingUsers = helpers.ReconcileOptionalBoolPointer(p.FillExistingUsers, state.FillExistingUsers)
	state.AvailableInSoftwareUpdate = helpers.ReconcileOptionalBoolPointer(p.Swu, state.AvailableInSoftwareUpdate)

	state.OSRequirements = helpers.ReconcileOptionalStringPointer(p.OsRequirements, state.OSRequirements)

	state.Manifest = helpers.ReconcileOptionalStringPointer(p.Manifest, state.Manifest)
	state.ManifestFileName = helpers.ReconcileOptionalStringPointer(p.ManifestFileName, state.ManifestFileName)

	state.Sha3512 = helpers.ReconcileOptionalStringPointer(p.Sha3512, state.Sha3512)
	state.Sha256 = helpers.ReconcileOptionalStringPointer(p.Sha256, state.Sha256)
	state.Md5 = helpers.ReconcileOptionalStringPointer(p.Md5, state.Md5)
	state.HashType = helpers.ReconcileOptionalStringPointer(p.HashType, state.HashType)
	state.HashValue = helpers.ReconcileOptionalStringPointer(p.HashValue, state.HashValue)

	state.Size = helpers.ReconcileOptionalStringPointer(p.Size, state.Size)
	state.InstallLanguage = helpers.ReconcileOptionalStringPointer(p.InstallLanguage, state.InstallLanguage)
	state.ParentPackageID = helpers.ReconcileOptionalStringPointer(p.ParentPackageID, state.ParentPackageID)
	state.SelfHealingAction = helpers.ReconcileOptionalStringPointer(p.SelfHealingAction, state.SelfHealingAction)
	state.SelfHealNotify = helpers.ReconcileOptionalBoolPointer(p.SelfHealNotify, state.SelfHealNotify)
	state.CloudTransferStatus = helpers.ReconcileOptionalStringPointer(p.CloudTransferStatus, state.CloudTransferStatus)
	state.Indexed = helpers.ReconcileOptionalBoolPointer(p.Indexed, state.Indexed)
	state.Format = helpers.ReconcileOptionalStringPointer(p.Format, state.Format)

	return nil
}

// assignPackageDataSourceModel populates the data source model. The data
// source has no Computed-with-user-supplied tension — every field is
// Computed — so reconcile-on-empty isn't needed; just collapse empties to
// null directly.
func assignPackageDataSourceModel(state *PackageDataSourceModel, p *pro.Package) diag.Diagnostics {
	if p == nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Nil package response",
				"The Jamf Pro client returned a nil Package without an error; the provider cannot map this to Terraform state.",
			),
		}
	}

	if p.ID != nil {
		state.ID = types.StringValue(*p.ID)
	}
	state.DisplayName = types.StringValue(p.PackageName)
	state.FileName = types.StringValue(p.FileName)
	state.CategoryID = types.StringValue(p.CategoryID)
	state.Info = helpers.StringPointerValueOrNull(p.Info)
	state.Notes = helpers.StringPointerValueOrNull(p.Notes)
	state.Priority = types.Int64Value(int64(p.Priority))
	state.FillUserTemplate = types.BoolValue(p.FillUserTemplate)
	state.RebootRequired = types.BoolValue(p.RebootRequired)
	state.FillExistingUsers = helpers.BoolPointerValueOrNull(p.FillExistingUsers)
	state.AvailableInSoftwareUpdate = helpers.BoolPointerValueOrNull(p.Swu)
	state.OSRequirements = helpers.StringPointerValueOrNull(p.OsRequirements)
	state.Manifest = helpers.StringPointerValueOrNull(p.Manifest)
	state.ManifestFileName = helpers.StringPointerValueOrNull(p.ManifestFileName)
	state.Sha3512 = helpers.StringPointerValueOrNull(p.Sha3512)
	state.Sha256 = helpers.StringPointerValueOrNull(p.Sha256)
	state.Md5 = helpers.StringPointerValueOrNull(p.Md5)
	state.HashType = helpers.StringPointerValueOrNull(p.HashType)
	state.HashValue = helpers.StringPointerValueOrNull(p.HashValue)
	state.Size = helpers.StringPointerValueOrNull(p.Size)
	state.InstallLanguage = helpers.StringPointerValueOrNull(p.InstallLanguage)
	state.ParentPackageID = helpers.StringPointerValueOrNull(p.ParentPackageID)
	state.SelfHealingAction = helpers.StringPointerValueOrNull(p.SelfHealingAction)
	state.SelfHealNotify = helpers.BoolPointerValueOrNull(p.SelfHealNotify)
	state.CloudTransferStatus = helpers.StringPointerValueOrNull(p.CloudTransferStatus)
	state.Indexed = helpers.BoolPointerValueOrNull(p.Indexed)
	state.Format = helpers.StringPointerValueOrNull(p.Format)

	return nil
}
