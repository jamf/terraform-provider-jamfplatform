// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildPostInput translates the plan (and config-side WriteOnly secrets) into
// the SDK Post body used by Create.
func buildPostInput(ctx context.Context, plan, cfg ComputerPrestageEnrollmentResourceModel) (*pro.PostComputerPrestageV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	post := &pro.PostComputerPrestageV3{
		DisplayName:                        plan.DisplayName.ValueString(),
		Mandatory:                          plan.Mandatory.ValueBool(),
		MDMRemovable:                       plan.MdmRemovable.ValueBool(),
		DefaultPrestage:                    plan.DefaultPrestage.ValueBool(),
		SupportPhoneNumber:                 plan.SupportPhoneNumber.ValueString(),
		SupportEmailAddress:                plan.SupportEmailAddress.ValueString(),
		Department:                         plan.Department.ValueString(),
		RequireAuthentication:              plan.RequireAuthentication.ValueBool(),
		AuthenticationPrompt:               plan.AuthenticationPrompt.ValueString(),
		DeviceEnrollmentProgramInstanceID:  plan.DeviceEnrollmentProgramInstanceID.ValueString(),
		EnrollmentSiteID:                   stringOrSentinel(plan.EnrollmentSiteID, sentinelNoneIDDash1),
		KeepExistingLocationInformation:    plan.KeepExistingLocationInformation.ValueBool(),
		KeepExistingSiteMembership:         plan.KeepExistingSiteMembership.ValueBool(),
		EnrollmentCustomizationID:          helpers.OptionalStringPointer(plan.EnrollmentCustomizationID),
		Language:                           helpers.OptionalStringPointer(plan.Language),
		Region:                             helpers.OptionalStringPointer(plan.Region),
		AutoAdvanceSetup:                   plan.AutoAdvanceSetup.ValueBool(),
		InstallProfilesDuringSetup:         plan.InstallProfilesDuringSetup.ValueBool(),
		CustomPackageDistributionPointID:   stringOrSentinel(plan.CustomPackageDistributionPointID, sentinelNoneIDDash1),
		PreventActivationLock:              plan.PreventActivationLock.ValueBool(),
		EnableDeviceBasedActivationLock:    plan.EnableDeviceBasedActivationLock.ValueBool(),
		EnableRecoveryLock:                 helpers.OptionalBoolPointer(plan.EnableRecoveryLock),
		RecoveryLockPasswordType:           helpers.OptionalStringPointer(plan.RecoveryLockPasswordType),
		RotateRecoveryLockPassword:         helpers.OptionalBoolPointer(plan.RotateRecoveryLockPassword),
		PrestageMinimumOsTargetVersionType: helpers.OptionalStringPointer(plan.PrestageMinimumOsTargetVersionType),
		MinimumOsSpecificVersion:           helpers.OptionalStringPointer(plan.MinimumOsSpecificVersion),
		PssoEnabled:                        helpers.OptionalBoolPointer(plan.PssoEnabled),
		PlatformSsoAppBundleID:             helpers.OptionalStringPointer(plan.PlatformSsoAppBundleID),
		PssoConfigProfileID:                helpers.OptionalStringPointer(plan.PssoConfigProfileID),
	}

	prestageIDs, d := stringSetToSlice(ctx, plan.PrestageInstalledProfileIds)
	diags.Append(d...)
	post.PrestageInstalledProfileIds = nilSafeStringSlice(prestageIDs)

	customIDs, d := stringSetToSlice(ctx, plan.CustomPackageIds)
	diags.Append(d...)
	post.CustomPackageIds = nilSafeStringSlice(customIDs)

	anchors, d := stringListToSlice(ctx, plan.AnchorCertificates)
	diags.Append(d...)
	safeAnchors := nilSafeStringSlice(anchors)
	post.AnchorCertificates = &safeAnchors

	post.LocationInformation = buildLocationInformation(plan.LocationInformation, sentinelNestedIDForCreate, 0)
	post.PurchasingInformation = buildPurchasingInformation(plan.PurchasingInformation, sentinelNestedIDForCreate, 0)
	post.SkipSetupItems = buildSkipSetupItemsMap(plan.SkipSetupItems)

	if as := buildAccountSettingsRequest(plan.AccountSettings, nil, cfg.AccountSettings, false); as != nil {
		post.AccountSettings = as
	}
	if recoveryPwd := plaintextRecoveryPassword(cfg); recoveryPwd != nil {
		post.RecoveryLockPassword = recoveryPwd
	}

	return post, diags
}

// buildPutInput translates the plan + GET-derived state into the PUT body.
// versionLocks are NOT set here — caller invokes injectVersionLocks.
//
// rotateAdminPwd / rotateRecoveryPwd indicate whether the corresponding
// _wo_version companion changed between state and plan; only when true is
// the WriteOnly plaintext re-sent on the wire.
func buildPutInput(ctx context.Context, plan, cfg ComputerPrestageEnrollmentResourceModel, got *pro.GetComputerPrestageV3, rotateAdminPwd, rotateRecoveryPwd bool) (*pro.PutComputerPrestageV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	put := &pro.PutComputerPrestageV3{
		DisplayName:                        plan.DisplayName.ValueString(),
		Mandatory:                          plan.Mandatory.ValueBool(),
		MDMRemovable:                       plan.MdmRemovable.ValueBool(),
		DefaultPrestage:                    plan.DefaultPrestage.ValueBool(),
		SupportPhoneNumber:                 plan.SupportPhoneNumber.ValueString(),
		SupportEmailAddress:                plan.SupportEmailAddress.ValueString(),
		Department:                         plan.Department.ValueString(),
		RequireAuthentication:              plan.RequireAuthentication.ValueBool(),
		AuthenticationPrompt:               plan.AuthenticationPrompt.ValueString(),
		DeviceEnrollmentProgramInstanceID:  plan.DeviceEnrollmentProgramInstanceID.ValueString(),
		EnrollmentSiteID:                   stringOrSentinel(plan.EnrollmentSiteID, sentinelNoneIDDash1),
		KeepExistingLocationInformation:    plan.KeepExistingLocationInformation.ValueBool(),
		KeepExistingSiteMembership:         plan.KeepExistingSiteMembership.ValueBool(),
		EnrollmentCustomizationID:          helpers.OptionalStringPointer(plan.EnrollmentCustomizationID),
		Language:                           helpers.OptionalStringPointer(plan.Language),
		Region:                             helpers.OptionalStringPointer(plan.Region),
		AutoAdvanceSetup:                   plan.AutoAdvanceSetup.ValueBool(),
		InstallProfilesDuringSetup:         plan.InstallProfilesDuringSetup.ValueBool(),
		CustomPackageDistributionPointID:   stringOrSentinel(plan.CustomPackageDistributionPointID, sentinelNoneIDDash1),
		PreventActivationLock:              plan.PreventActivationLock.ValueBool(),
		EnableDeviceBasedActivationLock:    plan.EnableDeviceBasedActivationLock.ValueBool(),
		EnableRecoveryLock:                 helpers.OptionalBoolPointer(plan.EnableRecoveryLock),
		RecoveryLockPasswordType:           helpers.OptionalStringPointer(plan.RecoveryLockPasswordType),
		RotateRecoveryLockPassword:         helpers.OptionalBoolPointer(plan.RotateRecoveryLockPassword),
		PrestageMinimumOsTargetVersionType: helpers.OptionalStringPointer(plan.PrestageMinimumOsTargetVersionType),
		MinimumOsSpecificVersion:           helpers.OptionalStringPointer(plan.MinimumOsSpecificVersion),
		PssoEnabled:                        helpers.OptionalBoolPointer(plan.PssoEnabled),
		PlatformSsoAppBundleID:             helpers.OptionalStringPointer(plan.PlatformSsoAppBundleID),
		PssoConfigProfileID:                helpers.OptionalStringPointer(plan.PssoConfigProfileID),
	}

	prestageIDs, d := stringSetToSlice(ctx, plan.PrestageInstalledProfileIds)
	diags.Append(d...)
	put.PrestageInstalledProfileIds = nilSafeStringSlice(prestageIDs)

	customIDs, d := stringSetToSlice(ctx, plan.CustomPackageIds)
	diags.Append(d...)
	put.CustomPackageIds = nilSafeStringSlice(customIDs)

	anchors, d := stringListToSlice(ctx, plan.AnchorCertificates)
	diags.Append(d...)
	safeAnchors := nilSafeStringSlice(anchors)
	put.AnchorCertificates = &safeAnchors

	put.LocationInformation = buildLocationInformation(plan.LocationInformation, "", 0)
	put.PurchasingInformation = buildPurchasingInformation(plan.PurchasingInformation, "", 0)
	put.SkipSetupItems = buildSkipSetupItemsMap(plan.SkipSetupItems)

	// accountSettings always present on PUT per F6.
	put.AccountSettings = buildAccountSettingsRequest(plan.AccountSettings, got.AccountSettings, cfg.AccountSettings, rotateAdminPwd)
	if put.AccountSettings == nil {
		// Server requires accountSettings; mirror server defaults so the
		// PUT body is well-formed even when the user omitted the block.
		put.AccountSettings = &pro.AccountSettingsRequest{}
	}

	if rotateRecoveryPwd {
		put.RecoveryLockPassword = plaintextRecoveryPassword(cfg)
	}

	return put, diags
}

// buildLocationInformation populates the SDK LocationInformationV2 from the
// nested block. nestedID / nestedLock are echoed verbatim — Create passes "-1"
// + 0 to request a fresh server-side record; Update is overwritten by
// injectVersionLocks after this returns.
func buildLocationInformation(m *LocationInformationModel, nestedID string, nestedLock int) pro.LocationInformationV2 {
	out := pro.LocationInformationV2{
		ID:           nestedID,
		VersionLock:  nestedLock,
		BuildingID:   sentinelNoneIDDash1,
		DepartmentID: sentinelNoneIDDash1,
	}
	if m == nil {
		return out
	}
	out.Username = m.Username.ValueString()
	out.Realname = m.Realname.ValueString()
	out.Phone = m.Phone.ValueString()
	out.Email = m.Email.ValueString()
	out.Room = m.Room.ValueString()
	out.Position = m.Position.ValueString()
	out.BuildingID = stringOrSentinel(m.BuildingID, sentinelNoneIDDash1)
	out.DepartmentID = stringOrSentinel(m.DepartmentID, sentinelNoneIDDash1)
	return out
}

func buildPurchasingInformation(m *PurchasingInformationModel, nestedID string, nestedLock int) pro.PrestagePurchasingInformationV2 {
	out := pro.PrestagePurchasingInformationV2{
		ID:           nestedID,
		VersionLock:  nestedLock,
		Purchased:    true,
		LeaseDate:    sentinelDateUnset,
		PoDate:       sentinelDateUnset,
		WarrantyDate: sentinelDateUnset,
	}
	if m == nil {
		return out
	}
	out.Leased = m.Leased.ValueBool()
	out.Purchased = m.Purchased.ValueBool()
	out.AppleCareID = m.AppleCareID.ValueString()
	out.PoNumber = m.PoNumber.ValueString()
	out.Vendor = m.Vendor.ValueString()
	out.PurchasePrice = m.PurchasePrice.ValueString()
	if !m.LifeExpectancy.IsNull() && !m.LifeExpectancy.IsUnknown() {
		out.LifeExpectancy = int(m.LifeExpectancy.ValueInt64())
	}
	out.PurchasingAccount = m.PurchasingAccount.ValueString()
	out.PurchasingContact = m.PurchasingContact.ValueString()
	out.LeaseDate = stringOrSentinel(m.LeaseDate, sentinelDateUnset)
	out.PoDate = stringOrSentinel(m.PoDate, sentinelDateUnset)
	out.WarrantyDate = stringOrSentinel(m.WarrantyDate, sentinelDateUnset)
	return out
}

func buildSkipSetupItemsMap(m *SkipSetupItemsModel) *map[string]bool {
	if m == nil {
		return nil
	}
	out := map[string]bool{
		"Biometric":                 m.Biometric.ValueBool(),
		"FileVault":                 m.FileVault.ValueBool(),
		"SoftwareUpdate":            m.SoftwareUpdate.ValueBool(),
		"Diagnostics":               m.Diagnostics.ValueBool(),
		"Accessibility":             m.Accessibility.ValueBool(),
		"Intelligence":              m.Intelligence.ValueBool(),
		"ScreenTime":                m.ScreenTime.ValueBool(),
		"Siri":                      m.Siri.ValueBool(),
		"Restore":                   m.Restore.ValueBool(),
		"Privacy":                   m.Privacy.ValueBool(),
		"Registration":              m.Registration.ValueBool(),
		"EnableLockdownMode":        m.EnableLockdownMode.ValueBool(),
		"TermsOfAddress":            m.TermsOfAddress.ValueBool(),
		"iCloudDiagnostics":         m.ICloudDiagnostics.ValueBool(),
		"AppleID":                   m.AppleID.ValueBool(),
		"DisplayTone":               m.DisplayTone.ValueBool(),
		"Appearance":                m.Appearance.ValueBool(),
		"Payment":                   m.Payment.ValueBool(),
		"TOS":                       m.TOS.ValueBool(),
		"OSShowcase":                m.OSShowcase.ValueBool(),
		"Welcome":                   m.Welcome.ValueBool(),
		"Wallpaper":                 m.Wallpaper.ValueBool(),
		"iCloudStorage":             m.ICloudStorage.ValueBool(),
		"AdditionalPrivacySettings": m.AdditionalPrivacySettings.ValueBool(),
		"Location":                  m.Location.ValueBool(),
	}
	return &out
}

// buildAccountSettingsRequest assembles the AccountSettingsRequest. plan
// carries non-secret fields; cfg supplies the WriteOnly admin_password
// plaintext (only when rotate=true). got is used as a source of the
// {id, versionLock} pair on Update — caller passes nil on Create.
func buildAccountSettingsRequest(plan *AccountSettingsModel, got *pro.AccountSettingsResponse, cfg *AccountSettingsModel, rotate bool) *pro.AccountSettingsRequest {
	if plan == nil && got == nil {
		return nil
	}
	out := &pro.AccountSettingsRequest{}

	if plan != nil {
		out.PayloadConfigured = boolPtrFromTF(plan.PayloadConfigured)
		out.LocalAdminAccountEnabled = boolPtrFromTF(plan.LocalAdminAccountEnabled)
		out.AdminUsername = stringPtrFromTF(plan.AdminUsername)
		out.HiddenAdminAccount = boolPtrFromTF(plan.HiddenAdminAccount)
		out.LocalUserManaged = boolPtrFromTF(plan.LocalUserManaged)
		out.UserAccountType = stringPtrFromTF(plan.UserAccountType)
		out.PrefillPrimaryAccountInfoFeatureEnabled = boolPtrFromTF(plan.PrefillPrimaryAccountInfoFeatureEnabled)
		out.PrefillType = stringPtrFromTF(plan.PrefillType)
		out.PrefillAccountFullName = stringPtrFromTF(plan.PrefillAccountFullName)
		out.PrefillAccountUserName = stringPtrFromTF(plan.PrefillAccountUserName)
		out.PreventPrefillInfoFromModification = boolPtrFromTF(plan.PreventPrefillInfoFromModification)
	}

	if rotate && cfg != nil && !cfg.AdminPassword.IsNull() && !cfg.AdminPassword.IsUnknown() && cfg.AdminPassword.ValueString() != "" {
		pw := cfg.AdminPassword.ValueString()
		out.AdminPassword = &pw
	}

	return out
}

// plaintextRecoveryPassword reads the WriteOnly recovery_lock_password from
// the config; returns nil when not set.
func plaintextRecoveryPassword(cfg ComputerPrestageEnrollmentResourceModel) *string {
	if cfg.RecoveryLockPassword.IsNull() || cfg.RecoveryLockPassword.IsUnknown() {
		return nil
	}
	s := cfg.RecoveryLockPassword.ValueString()
	if s == "" {
		return nil
	}
	return &s
}

// stringSetToSlice extracts a TF Set<String> into a Go []string.
// Returns nil when the set is null/unknown.
func stringSetToSlice(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	out := []string{}
	d := s.ElementsAs(ctx, &out, false)
	return out, d
}

// stringListToSlice extracts a TF List<String> into a Go []string.
// Returns nil when the list is null/unknown.
func stringListToSlice(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	out := []string{}
	d := l.ElementsAs(ctx, &out, false)
	return out, d
}

// nilSafeStringSlice returns an empty (non-nil) slice when the input is nil.
// Jamf Pro V3 PreStage POST/PUT rejects null array fields with
// `400 INVALID_FIELD: Cannot invoke "java.util.Collection.toArray()" because
// "c" is null`. Every list/set field in the body must be a JSON `[]`, never
// `null`.
func nilSafeStringSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func stringOrSentinel(v types.String, sentinel string) string {
	if v.IsNull() || v.IsUnknown() {
		return sentinel
	}
	if s := v.ValueString(); s != "" {
		return s
	}
	return sentinel
}

func boolPtrFromTF(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func stringPtrFromTF(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}
