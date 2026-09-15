// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignSettingsResourceModel folds a /v4 GET response into resource state. The
// six Re-enrollment fields are intentionally not modelled and are ignored here.
//
// The cert identity object is null on GET, so keystore_file_name is preserved
// from prior state rather than echoed. subject / serial_number come from the
// matching *Details object and only populate while the toggle is enabled.
func assignSettingsResourceModel(state *UserInitiatedEnrollmentSettingsResourceModel, s *pro.EnrollmentSettingsV4) {
	if s == nil {
		return
	}

	// General tab.
	state.SkipCertificateInstallation = helpers.BoolPointerValueOrNull(s.InstallSingleProfile)
	state.RestrictReenrollment = helpers.BoolPointerValueOrNull(s.RestrictReenrollment)
	state.SigningMdmProfileEnabled = helpers.BoolPointerValueOrNull(s.SigningMDMProfileEnabled)

	// Computers tab.
	state.EnableComputerEnrollment = helpers.BoolPointerValueOrNull(s.MacOsEnterpriseEnrollmentEnabled)
	state.CreateManagementAccount = helpers.BoolPointerValueOrNull(s.CreateManagementAccount)
	state.ManagementUsername = helpers.StringValueOrNull(s.ManagementUsername)
	state.HideManagementAccount = helpers.BoolPointerValueOrNull(s.HideManagementAccount)
	state.AllowSshOnlyManagementAccount = helpers.BoolPointerValueOrNull(s.AllowSshOnlyManagementAccount)
	state.EnsureSshRunning = helpers.BoolPointerValueOrNull(s.EnsureSshRunning)
	state.LaunchSelfService = helpers.BoolPointerValueOrNull(s.LaunchSelfService)
	state.SignQuickaddPackage = helpers.BoolPointerValueOrNull(s.SignQuickAdd)
	state.AccountDrivenDeviceEnrollmentMac = helpers.BoolPointerValueOrNull(s.AccountDrivenDeviceMacosEnrollmentEnabled)

	// Devices tab.
	state.ProfileDrivenEnrollmentViaURLInstitutional = helpers.BoolPointerValueOrNull(s.IosEnterpriseEnrollmentEnabled)
	state.ProfileDrivenEnrollmentViaURLPersonal = helpers.BoolPointerValueOrNull(s.IosPersonalEnrollmentEnabled)
	state.AccountDrivenUserEnrollment = helpers.BoolPointerValueOrNull(s.AccountDrivenUserEnrollmentEnabled)
	state.AccountDrivenUserEnrollmentVisionos = helpers.BoolPointerValueOrNull(s.AccountDrivenUserVisionosEnrollmentEnabled)
	state.MergeManagedAppleAccountUsernames = helpers.BoolPointerValueOrNull(s.MaidUsernameMergeEnabled)
	state.AccountDrivenDeviceEnrollmentIos = helpers.BoolPointerValueOrNull(s.AccountDrivenDeviceIosEnrollmentEnabled)
	state.AccountDrivenDeviceEnrollmentVisionos = helpers.BoolPointerValueOrNull(s.AccountDrivenDeviceVisionosEnrollmentEnabled)

	// Deprecated, server-controlled.
	state.PersonalDeviceEnrollmentType = helpers.StringPointerValueOrNull(s.PersonalDeviceEnrollmentType)

	// Cert details. Only populate the block details when the user authored a
	// block (state non-nil); never inject a tenant cert Terraform was not asked
	// to manage. keystore_file_name is preserved from prior state because the
	// GET cert identity object is null.
	if state.MdmSigningCertificate != nil {
		applyCertDetails(state.MdmSigningCertificate, s.MDMSigningCertificateDetails)
	}
	if state.DeveloperCertificate != nil {
		applyCertDetails(state.DeveloperCertificate, s.DeveloperCertificateIdentityDetails)
	}
}

// applyCertDetails copies subject / serial_number from a *Details echo into the
// cert sub-model. keystore_file_name and the WriteOnly inputs are left
// untouched (preserved from prior state).
func applyCertDetails(cert *certificateModel, details *pro.CertificateDetails) {
	if details == nil {
		cert.Subject = types.StringNull()
		cert.SerialNumber = types.StringNull()
		return
	}
	cert.Subject = helpers.StringPointerValueOrNull(details.Subject)
	cert.SerialNumber = helpers.StringPointerValueOrNull(details.SerialNumber)
}

// assignAccessGroupsState builds the access_group set from a /v3 list response.
func assignAccessGroupsState(ctx context.Context, groups []pro.EnrollmentAccessGroupPreview) (types.Set, diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: accessGroupAttrTypes}
	values := make([]attr.Value, 0, len(groups))
	for i := range groups {
		obj, d := accessGroupObject(groups[i])
		if d.HasError() {
			return types.SetNull(elemType), d
		}
		values = append(values, obj)
	}
	return types.SetValueFrom(ctx, elemType, values)
}

// accessGroupObject converts an SDK preview into a Terraform object value.
func accessGroupObject(g pro.EnrollmentAccessGroupPreview) (attr.Value, diag.Diagnostics) {
	return types.ObjectValue(accessGroupAttrTypes, map[string]attr.Value{
		"id":                                     helpers.StringPointerValueOrNull(g.ID),
		"directory_service_group_id":             helpers.StringValueOrNull(g.GroupID),
		"ldap_server_id":                         helpers.StringValueOrNull(g.LdapServerID),
		"name":                                   helpers.StringValueOrNull(g.Name),
		"site_id":                                helpers.StringPointerValueOrNull(g.SiteID),
		"enterprise_enrollment_enabled":          helpers.BoolPointerValueOrNull(g.EnterpriseEnrollmentEnabled),
		"personal_enrollment_enabled":            helpers.BoolPointerValueOrNull(g.PersonalEnrollmentEnabled),
		"account_driven_user_enrollment_enabled": helpers.BoolPointerValueOrNull(g.AccountDrivenUserEnrollmentEnabled),
		"require_eula":                           helpers.BoolPointerValueOrNull(g.RequireEula),
	})
}

// ===== Data-source-side assigners ============================================

// assignSettingsDataSourceModel folds a /v4 GET response into data-source state.
func assignSettingsDataSourceModel(state *UserInitiatedEnrollmentSettingsDataSourceModel, s *pro.EnrollmentSettingsV4) {
	if s == nil {
		return
	}
	state.SkipCertificateInstallation = helpers.BoolPointerValueOrNull(s.InstallSingleProfile)
	state.RestrictReenrollment = helpers.BoolPointerValueOrNull(s.RestrictReenrollment)
	state.SigningMdmProfileEnabled = helpers.BoolPointerValueOrNull(s.SigningMDMProfileEnabled)

	state.EnableComputerEnrollment = helpers.BoolPointerValueOrNull(s.MacOsEnterpriseEnrollmentEnabled)
	state.CreateManagementAccount = helpers.BoolPointerValueOrNull(s.CreateManagementAccount)
	state.ManagementUsername = helpers.StringValueOrNull(s.ManagementUsername)
	state.HideManagementAccount = helpers.BoolPointerValueOrNull(s.HideManagementAccount)
	state.AllowSshOnlyManagementAccount = helpers.BoolPointerValueOrNull(s.AllowSshOnlyManagementAccount)
	state.EnsureSshRunning = helpers.BoolPointerValueOrNull(s.EnsureSshRunning)
	state.LaunchSelfService = helpers.BoolPointerValueOrNull(s.LaunchSelfService)
	state.SignQuickaddPackage = helpers.BoolPointerValueOrNull(s.SignQuickAdd)
	state.AccountDrivenDeviceEnrollmentMac = helpers.BoolPointerValueOrNull(s.AccountDrivenDeviceMacosEnrollmentEnabled)

	state.ProfileDrivenEnrollmentViaURLInstitutional = helpers.BoolPointerValueOrNull(s.IosEnterpriseEnrollmentEnabled)
	state.ProfileDrivenEnrollmentViaURLPersonal = helpers.BoolPointerValueOrNull(s.IosPersonalEnrollmentEnabled)
	state.AccountDrivenUserEnrollment = helpers.BoolPointerValueOrNull(s.AccountDrivenUserEnrollmentEnabled)
	state.AccountDrivenUserEnrollmentVisionos = helpers.BoolPointerValueOrNull(s.AccountDrivenUserVisionosEnrollmentEnabled)
	state.MergeManagedAppleAccountUsernames = helpers.BoolPointerValueOrNull(s.MaidUsernameMergeEnabled)
	state.AccountDrivenDeviceEnrollmentIos = helpers.BoolPointerValueOrNull(s.AccountDrivenDeviceIosEnrollmentEnabled)
	state.AccountDrivenDeviceEnrollmentVisionos = helpers.BoolPointerValueOrNull(s.AccountDrivenDeviceVisionosEnrollmentEnabled)

	state.PersonalDeviceEnrollmentType = helpers.StringPointerValueOrNull(s.PersonalDeviceEnrollmentType)

	state.MdmSigningCertificate = certReadOnly(s.MDMSigningCertificateDetails)
	state.DeveloperCertificate = certReadOnly(s.DeveloperCertificateIdentityDetails)
}

// certReadOnly builds a data-source cert projection from a *Details echo.
func certReadOnly(details *pro.CertificateDetails) *certificateReadOnlyModel {
	if details == nil {
		return nil
	}
	return &certificateReadOnlyModel{
		KeystoreFileName: types.StringNull(),
		Subject:          helpers.StringPointerValueOrNull(details.Subject),
		SerialNumber:     helpers.StringPointerValueOrNull(details.SerialNumber),
	}
}
