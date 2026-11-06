// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/prestagealias"
)

// buildPostInput translates the plan (and config-side WriteOnly secrets) into
// the SDK Post body used by Create.
func buildPostInput(ctx context.Context, plan, cfg ComputerPrestageEnrollmentResourceModel) (*pro.PostComputerPrestageV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	post := &pro.PostComputerPrestageV3{
		DisplayName:                        plan.DisplayName.ValueString(),
		Mandatory:                          prestagealias.Effective(plan.MakeMdmProfileMandatory, plan.Mandatory, false, false),
		MDMRemovable:                       prestagealias.Effective(plan.AllowMdmProfileRemoval, plan.MdmRemovable, false, false),
		DefaultPrestage:                    plan.DefaultPrestage.ValueBool(),
		SupportPhoneNumber:                 plan.SupportPhoneNumber.ValueString(),
		SupportEmailAddress:                plan.SupportEmailAddress.ValueString(),
		Department:                         plan.Department.ValueString(),
		RequireAuthentication:              plan.RequireAuthentication.ValueBool(),
		AuthenticationPrompt:               prestagealias.EffectiveString(plan.AuthenticationMessage, plan.AuthenticationPrompt, ""),
		DeviceEnrollmentProgramInstanceID:  plan.DeviceEnrollmentProgramInstanceID.ValueString(),
		EnrollmentSiteID:                   stringOrSentinel(plan.EnrollmentSiteID, sentinelNoneIDDash1),
		KeepExistingLocationInformation:    prestagealias.Effective(plan.UseExistingLocationInformation, plan.KeepExistingLocationInformation, false, false),
		KeepExistingSiteMembership:         prestagealias.Effective(plan.UseExistingSiteMembership, plan.KeepExistingSiteMembership, false, false),
		EnrollmentCustomizationID:          helpers.OptionalStringPointer(plan.EnrollmentCustomizationID),
		Language:                           helpers.OptionalStringPointer(plan.Language),
		Region:                             helpers.OptionalStringPointer(plan.Region),
		AutoAdvanceSetup:                   prestagealias.Effective(plan.AutomaticallyAdvanceThroughSetupAssistant, plan.AutoAdvanceSetup, false, false),
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

	prestageIDs, d := stringSetToSlice(ctx, effectiveSet(plan.ConfigurationProfileIds, plan.PrestageInstalledProfileIds))
	diags.Append(d...)
	post.PrestageInstalledProfileIds = nilSafeStringSlice(prestageIDs)
	post.InstallProfilesDuringSetup = installProfilesDuringSetup(plan.InstallProfilesDuringSetup, prestageIDs)

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
		Mandatory:                          prestagealias.Effective(plan.MakeMdmProfileMandatory, plan.Mandatory, false, false),
		MDMRemovable:                       prestagealias.Effective(plan.AllowMdmProfileRemoval, plan.MdmRemovable, false, false),
		DefaultPrestage:                    plan.DefaultPrestage.ValueBool(),
		SupportPhoneNumber:                 plan.SupportPhoneNumber.ValueString(),
		SupportEmailAddress:                plan.SupportEmailAddress.ValueString(),
		Department:                         plan.Department.ValueString(),
		RequireAuthentication:              plan.RequireAuthentication.ValueBool(),
		AuthenticationPrompt:               prestagealias.EffectiveString(plan.AuthenticationMessage, plan.AuthenticationPrompt, ""),
		DeviceEnrollmentProgramInstanceID:  plan.DeviceEnrollmentProgramInstanceID.ValueString(),
		EnrollmentSiteID:                   stringOrSentinel(plan.EnrollmentSiteID, sentinelNoneIDDash1),
		KeepExistingLocationInformation:    prestagealias.Effective(plan.UseExistingLocationInformation, plan.KeepExistingLocationInformation, false, false),
		KeepExistingSiteMembership:         prestagealias.Effective(plan.UseExistingSiteMembership, plan.KeepExistingSiteMembership, false, false),
		EnrollmentCustomizationID:          helpers.OptionalStringPointer(plan.EnrollmentCustomizationID),
		Language:                           helpers.OptionalStringPointer(plan.Language),
		Region:                             helpers.OptionalStringPointer(plan.Region),
		AutoAdvanceSetup:                   prestagealias.Effective(plan.AutomaticallyAdvanceThroughSetupAssistant, plan.AutoAdvanceSetup, false, false),
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

	prestageIDs, d := stringSetToSlice(ctx, effectiveSet(plan.ConfigurationProfileIds, plan.PrestageInstalledProfileIds))
	diags.Append(d...)
	put.PrestageInstalledProfileIds = nilSafeStringSlice(prestageIDs)
	put.InstallProfilesDuringSetup = installProfilesDuringSetup(plan.InstallProfilesDuringSetup, prestageIDs)

	customIDs, d := stringSetToSlice(ctx, plan.CustomPackageIds)
	diags.Append(d...)
	put.CustomPackageIds = nilSafeStringSlice(customIDs)

	anchors, d := stringListToSlice(ctx, plan.AnchorCertificates)
	diags.Append(d...)
	safeAnchors := nilSafeStringSlice(anchors)
	put.AnchorCertificates = &safeAnchors

	put.LocationInformation = buildLocationInformation(plan.LocationInformation, "", 0)
	put.PurchasingInformation = buildPurchasingInformation(plan.PurchasingInformation, "", 0)
	put.SkipSetupItems = buildSkipSetupItemsMapOver(plan.SkipSetupItems, got.SkipSetupItems)

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
		out.PayloadConfigured = payloadConfiguredFor(plan, cfg)
		out.LocalAdminAccountEnabled = effectiveBoolPtr(plan.CreateManagedLocalAdministratorAccount, plan.LocalAdminAccountEnabled)
		out.AdminUsername = stringPtrFromTF(plan.AdminUsername)
		out.HiddenAdminAccount = effectiveBoolPtr(plan.HideManagedLocalAdministratorAccount, plan.HiddenAdminAccount)
		out.LocalUserManaged = effectiveBoolPtr(plan.MakeManagedLocalAdministratorAccountMdmEnabled, plan.LocalUserManaged)
		out.UserAccountType = stringPtrFromTF(plan.UserAccountType)
		out.PrefillPrimaryAccountInfoFeatureEnabled = boolPtrFromTF(plan.PrefillPrimaryAccountInfoFeatureEnabled)
		out.PrefillType = stringPtrFromTF(plan.PrefillType)
		out.PrefillAccountFullName = stringPtrFromTF(plan.PrefillAccountFullName)
		out.PrefillAccountUserName = stringPtrFromTF(plan.PrefillAccountUserName)
		out.PreventPrefillInfoFromModification = effectiveBoolPtr(plan.LockPrimaryAccountInformation, plan.PreventPrefillInfoFromModification)
	}

	if rotate && cfg != nil && !cfg.AdminPassword.IsNull() && !cfg.AdminPassword.IsUnknown() && cfg.AdminPassword.ValueString() != "" {
		pw := cfg.AdminPassword.ValueString()
		out.AdminPassword = &pw
	}

	return out
}

// effectiveBoolPtr returns the replacement attribute's value, then the deprecated one's, or nil
// when neither holds a value.
func effectiveBoolPtr(replacement, deprecated types.Bool) *bool {
	if p := boolPtrFromTF(replacement); p != nil {
		return p
	}
	return boolPtrFromTF(deprecated)
}

// effectiveSet returns the replacement set when it holds a value, and the deprecated one
// otherwise.
func effectiveSet(replacement, deprecated types.Set) types.Set {
	if !replacement.IsNull() && !replacement.IsUnknown() {
		return replacement
	}
	return deprecated
}

// installProfilesDuringSetup is the flag Jamf Pro needs alongside the profile list. A configured
// value is sent as written, and otherwise the flag follows whether any profile is listed.
func installProfilesDuringSetup(configured types.Bool, ids []string) bool {
	if !configured.IsNull() && !configured.IsUnknown() {
		return configured.ValueBool()
	}
	return len(ids) > 0
}

// payloadConfiguredFor is the account-settings switch Jamf Pro requires whenever another account
// setting is non-default. A configured value is sent as written. Otherwise it is true when the
// configuration sets any account setting, and unset for an empty block. It reads the
// configuration, not the plan, because the plan fills unset attributes from state on Update.
func payloadConfiguredFor(plan, cfg *AccountSettingsModel) *bool {
	if p := boolPtrFromTF(plan.PayloadConfigured); p != nil {
		return p
	}
	if cfg == nil {
		cfg = plan
	}
	set := func(v attr.Value) bool { return !v.IsNull() }
	if set(cfg.CreateManagedLocalAdministratorAccount) || set(cfg.LocalAdminAccountEnabled) ||
		set(cfg.AdminUsername) || set(cfg.AdminPassword) ||
		set(cfg.HideManagedLocalAdministratorAccount) || set(cfg.HiddenAdminAccount) ||
		set(cfg.MakeManagedLocalAdministratorAccountMdmEnabled) || set(cfg.LocalUserManaged) ||
		set(cfg.UserAccountType) ||
		set(cfg.PrefillPrimaryAccountInfoFeatureEnabled) || set(cfg.PrefillType) ||
		set(cfg.PrefillAccountFullName) || set(cfg.PrefillAccountUserName) ||
		set(cfg.LockPrimaryAccountInformation) || set(cfg.PreventPrefillInfoFromModification) {
		t := true
		return &t
	}
	return nil
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
