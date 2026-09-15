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
)

// assignGetToResource maps a fresh GET response onto the resource model.
// plan is the target written to state and is mutated in-place — on Update it
// is the NEW plan, so it (not the prior state) governs which Optional-only
// nested blocks are populated (see the block-gating note below). state is the
// prior state, used only for *_wo_version reconciliation + the
// PssoConfigProfileID null/value defence.
func assignGetToResource(_ context.Context, plan *ComputerPrestageEnrollmentResourceModel, state ComputerPrestageEnrollmentResourceModel, got *pro.GetComputerPrestageV3) diag.Diagnostics {
	var diags diag.Diagnostics

	plan.ID = types.StringValue(got.ID)
	plan.DisplayName = types.StringValue(got.DisplayName)
	plan.Mandatory = types.BoolValue(got.Mandatory)
	plan.MdmRemovable = types.BoolValue(got.MDMRemovable)
	plan.DefaultPrestage = types.BoolValue(got.DefaultPrestage)
	plan.SupportPhoneNumber = types.StringValue(got.SupportPhoneNumber)
	plan.SupportEmailAddress = types.StringValue(got.SupportEmailAddress)
	plan.Department = types.StringValue(got.Department)
	plan.RequireAuthentication = types.BoolValue(got.RequireAuthentication)
	plan.AuthenticationPrompt = types.StringValue(got.AuthenticationPrompt)
	plan.DeviceEnrollmentProgramInstanceID = types.StringValue(got.DeviceEnrollmentProgramInstanceID)
	plan.SiteID = types.StringValue(got.SiteID)
	plan.EnrollmentSiteID = types.StringValue(got.EnrollmentSiteID)
	plan.KeepExistingLocationInformation = types.BoolValue(got.KeepExistingLocationInformation)
	plan.KeepExistingSiteMembership = types.BoolValue(got.KeepExistingSiteMembership)
	plan.EnrollmentCustomizationID = types.StringValue(got.EnrollmentCustomizationID)
	plan.Language = types.StringValue(got.Language)
	plan.Region = types.StringValue(got.Region)
	plan.AutoAdvanceSetup = types.BoolValue(got.AutoAdvanceSetup)
	plan.InstallProfilesDuringSetup = types.BoolValue(got.InstallProfilesDuringSetup)
	plan.CustomPackageDistributionPointID = types.StringValue(got.CustomPackageDistributionPointID)
	plan.PreventActivationLock = types.BoolValue(got.PreventActivationLock)
	plan.EnableDeviceBasedActivationLock = types.BoolValue(got.EnableDeviceBasedActivationLock)
	plan.EnableRecoveryLock = types.BoolValue(got.EnableRecoveryLock)
	plan.RecoveryLockPasswordType = types.StringValue(got.RecoveryLockPasswordType)
	plan.RotateRecoveryLockPassword = types.BoolValue(got.RotateRecoveryLockPassword)
	plan.PrestageMinimumOsTargetVersionType = types.StringValue(got.PrestageMinimumOsTargetVersionType)
	plan.MinimumOsSpecificVersion = types.StringValue(got.MinimumOsSpecificVersion)
	plan.PssoEnabled = types.BoolValue(got.PssoEnabled)
	// SDK type is plain `string`, so any wire-level null is decoded as ""
	// by Go before we see it. No null/"" flicker risk at the Go boundary;
	// use the value directly. (Spike OQ-8 had this on
	// PreserveStringWhenWireEmpty, but that helper returns the prior
	// state when wire is empty — and a fresh Create's plan is Unknown,
	// which leaks past apply.)
	plan.PlatformSsoAppBundleID = types.StringValue(got.PlatformSsoAppBundleID)
	plan.PssoConfigProfileID = helpers.ReconcileOptionalStringPointer(got.PssoConfigProfileID, state.PssoConfigProfileID)
	plan.ProfileURL = helpers.StringPointerValueOrNull(got.ProfileURL)
	plan.ManifestURL = helpers.StringPointerValueOrNull(got.ManifestURL)
	plan.ProfileUUID = types.StringValue(got.ProfileUUID)

	prestageIDs := stringSliceToSet(got.PrestageInstalledProfileIds)
	plan.PrestageInstalledProfileIds = prestageIDs
	plan.CustomPackageIds = stringSliceToSet(got.CustomPackageIds)
	plan.AnchorCertificates = stringSliceToList(got.AnchorCertificates)

	// Preserve the user-supplied wo_version values BEFORE we rebuild the
	// nested blocks from the GET. The wire never echoes wo_version; the
	// provider owns it client-side, and the user's plan value is what
	// should end up in state (not the prior state value — that would
	// trip the consistency check whenever the user bumps wo_version on
	// an Update).
	planAdminPwdWoVersion := types.Int64Null()
	if plan.AccountSettings != nil {
		planAdminPwdWoVersion = plan.AccountSettings.AdminPasswordWoVersion
	}
	planRecoveryLockPwdWoVersion := plan.RecoveryLockPasswordWoVersion

	// Optional-only typed-pointer nested blocks (STYLE_GUIDE §303: block
	// omission == "do not manage this section"). The gate MUST key off the
	// model being WRITTEN to state — `plan`, the mutated target — never the
	// prior `state` param. The target pointer is the planned value Terraform
	// Core compares the apply result against:
	//   - Create / Read: target is the only model; its pointer reflects the
	//     user's config / the stored (or import-seeded) state.
	//   - Update: target is the NEW plan. When the user removes a
	//     previously-managed block, the plan pointer is nil even though the
	//     prior state still holds the populated block. Gating on the prior
	//     state would repopulate from the wire and trip Terraform Core's
	//     "was null, but now ObjectVal(...)" consistency error at apply — the
	//     exact failure this guards against. (The policy resource gates the
	//     same way: it passes a single model and reads `state.X != nil` on
	//     that mutated target, which is the plan on Update.)
	// Capture presence first — the assignments below reassign each field.
	manageSkipSetupItems := plan.SkipSetupItems != nil
	manageLocationInformation := plan.LocationInformation != nil
	managePurchasingInformation := plan.PurchasingInformation != nil
	manageAccountSettings := plan.AccountSettings != nil

	if manageSkipSetupItems {
		plan.SkipSetupItems = flattenSkipSetupItems(got.SkipSetupItems)
	} else {
		plan.SkipSetupItems = nil
	}
	if manageLocationInformation {
		plan.LocationInformation = flattenLocationInformation(got.LocationInformation)
	} else {
		plan.LocationInformation = nil
	}
	if managePurchasingInformation {
		plan.PurchasingInformation = flattenPurchasingInformation(got.PurchasingInformation)
	} else {
		plan.PurchasingInformation = nil
	}
	if manageAccountSettings {
		plan.AccountSettings = flattenAccountSettings(got.AccountSettings, state.AccountSettings)
	} else {
		plan.AccountSettings = nil
	}

	// Restore the user's wo_version inputs onto the freshly-built models.
	if plan.AccountSettings != nil {
		plan.AccountSettings.AdminPasswordWoVersion = planAdminPwdWoVersion
	}
	plan.RecoveryLockPasswordWoVersion = planRecoveryLockPwdWoVersion

	return diags
}

func roundTripInt64(prior types.Int64) types.Int64 {
	if prior.IsNull() || prior.IsUnknown() {
		return types.Int64Null()
	}
	return prior
}

func flattenSkipSetupItems(m map[string]bool) *SkipSetupItemsModel {
	if m == nil {
		return nil
	}
	out := &SkipSetupItemsModel{}
	out.Biometric = types.BoolValue(m["Biometric"])
	out.FileVault = types.BoolValue(m["FileVault"])
	out.SoftwareUpdate = types.BoolValue(m["SoftwareUpdate"])
	out.Diagnostics = types.BoolValue(m["Diagnostics"])
	out.Accessibility = types.BoolValue(m["Accessibility"])
	out.Intelligence = types.BoolValue(m["Intelligence"])
	out.ScreenTime = types.BoolValue(m["ScreenTime"])
	out.Siri = types.BoolValue(m["Siri"])
	out.Restore = types.BoolValue(m["Restore"])
	out.Privacy = types.BoolValue(m["Privacy"])
	out.Registration = types.BoolValue(m["Registration"])
	out.EnableLockdownMode = types.BoolValue(m["EnableLockdownMode"])
	out.TermsOfAddress = types.BoolValue(m["TermsOfAddress"])
	out.ICloudDiagnostics = types.BoolValue(m["iCloudDiagnostics"])
	out.AppleID = types.BoolValue(m["AppleID"])
	out.DisplayTone = types.BoolValue(m["DisplayTone"])
	out.Appearance = types.BoolValue(m["Appearance"])
	out.Payment = types.BoolValue(m["Payment"])
	out.TOS = types.BoolValue(m["TOS"])
	out.OSShowcase = types.BoolValue(m["OSShowcase"])
	out.Welcome = types.BoolValue(m["Welcome"])
	out.Wallpaper = types.BoolValue(m["Wallpaper"])
	out.ICloudStorage = types.BoolValue(m["iCloudStorage"])
	out.AdditionalPrivacySettings = types.BoolValue(m["AdditionalPrivacySettings"])
	out.Location = types.BoolValue(m["Location"])
	return out
}

func flattenLocationInformation(loc *pro.LocationInformationV2) *LocationInformationModel {
	if loc == nil {
		return nil
	}
	return &LocationInformationModel{
		Username:     types.StringValue(loc.Username),
		Realname:     types.StringValue(loc.Realname),
		Phone:        types.StringValue(loc.Phone),
		Email:        types.StringValue(loc.Email),
		Room:         types.StringValue(loc.Room),
		Position:     types.StringValue(loc.Position),
		BuildingID:   types.StringValue(loc.BuildingID),
		DepartmentID: types.StringValue(loc.DepartmentID),
	}
}

func flattenPurchasingInformation(pur *pro.PrestagePurchasingInformationV2) *PurchasingInformationModel {
	if pur == nil {
		return nil
	}
	return &PurchasingInformationModel{
		Leased:            types.BoolValue(pur.Leased),
		Purchased:         types.BoolValue(pur.Purchased),
		AppleCareID:       types.StringValue(pur.AppleCareID),
		PoNumber:          types.StringValue(pur.PoNumber),
		Vendor:            types.StringValue(pur.Vendor),
		PurchasePrice:     types.StringValue(pur.PurchasePrice),
		LifeExpectancy:    types.Int64Value(int64(pur.LifeExpectancy)),
		PurchasingAccount: types.StringValue(pur.PurchasingAccount),
		PurchasingContact: types.StringValue(pur.PurchasingContact),
		LeaseDate:         types.StringValue(pur.LeaseDate),
		PoDate:            types.StringValue(pur.PoDate),
		WarrantyDate:      types.StringValue(pur.WarrantyDate),
	}
}

func flattenAccountSettings(got *pro.AccountSettingsResponse, prior *AccountSettingsModel) *AccountSettingsModel {
	if got == nil {
		return nil
	}
	out := &AccountSettingsModel{
		PayloadConfigured:                       types.BoolValue(got.PayloadConfigured),
		LocalAdminAccountEnabled:                types.BoolValue(got.LocalAdminAccountEnabled),
		AdminUsername:                           types.StringValue(got.AdminUsername),
		HiddenAdminAccount:                      types.BoolValue(got.HiddenAdminAccount),
		LocalUserManaged:                        types.BoolValue(got.LocalUserManaged),
		UserAccountType:                         types.StringValue(got.UserAccountType),
		PrefillPrimaryAccountInfoFeatureEnabled: types.BoolValue(got.PrefillPrimaryAccountInfoFeatureEnabled),
		PrefillType:                             types.StringValue(got.PrefillType),
		PrefillAccountFullName:                  types.StringValue(got.PrefillAccountFullName),
		PrefillAccountUserName:                  types.StringValue(got.PrefillAccountUserName),
		PreventPrefillInfoFromModification:      types.BoolValue(got.PreventPrefillInfoFromModification),
		AdminPassword:                           types.StringNull(),
		AdminPasswordWoVersion:                  types.Int64Null(),
	}
	if prior != nil {
		out.AdminPasswordWoVersion = roundTripInt64(prior.AdminPasswordWoVersion)
	}
	return out
}

// scopeSerialsToSet builds the scope_serial_numbers Set<String> from a fresh
// GET /scope response.
func scopeSerialsToSet(resp *pro.PrestageScopeResponseV2) types.Set {
	if resp == nil || len(resp.Assignments) == 0 {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, 0, len(resp.Assignments))
	for _, a := range resp.Assignments {
		elems = append(elems, types.StringValue(a.SerialNumber))
	}
	return types.SetValueMust(types.StringType, elems)
}

func stringSliceToSet(in []string) types.Set {
	if in == nil {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	return types.SetValueMust(types.StringType, elems)
}

func stringSliceToList(in []string) types.List {
	if in == nil {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	return types.ListValueMust(types.StringType, elems)
}

// diffPlanAgainstGet compares the user's desired plan-side fields against a
// fresh GET. Returns the snake_case paths of fields that did not round-trip.
// Used by the PUT-500 workaround (§11) to distinguish 500-with-commit from
// 500-with-silent-rollback.
func diffPlanAgainstGet(ctx context.Context, plan ComputerPrestageEnrollmentResourceModel, got *pro.GetComputerPrestageV3) []string {
	var mismatched []string

	checkStr := func(path string, planVal types.String, gotVal string) {
		if planVal.IsNull() || planVal.IsUnknown() {
			return
		}
		if planVal.ValueString() != gotVal {
			mismatched = append(mismatched, path)
		}
	}
	checkBool := func(path string, planVal types.Bool, gotVal bool) {
		if planVal.IsNull() || planVal.IsUnknown() {
			return
		}
		if planVal.ValueBool() != gotVal {
			mismatched = append(mismatched, path)
		}
	}
	checkStrPtr := func(path string, planVal types.String, gotVal *string) {
		if planVal.IsNull() || planVal.IsUnknown() {
			return
		}
		if gotVal == nil {
			mismatched = append(mismatched, path)
			return
		}
		if planVal.ValueString() != *gotVal {
			mismatched = append(mismatched, path)
		}
	}
	checkBoolPtrVal := func(path string, planVal types.Bool, gotVal bool) {
		if planVal.IsNull() || planVal.IsUnknown() {
			return
		}
		if planVal.ValueBool() != gotVal {
			mismatched = append(mismatched, path)
		}
	}

	checkStr("display_name", plan.DisplayName, got.DisplayName)
	checkBool("mandatory", plan.Mandatory, got.Mandatory)
	checkBool("mdm_removable", plan.MdmRemovable, got.MDMRemovable)
	checkBool("default_prestage", plan.DefaultPrestage, got.DefaultPrestage)
	checkStr("support_phone_number", plan.SupportPhoneNumber, got.SupportPhoneNumber)
	checkStr("support_email_address", plan.SupportEmailAddress, got.SupportEmailAddress)
	checkStr("department", plan.Department, got.Department)
	checkBool("require_authentication", plan.RequireAuthentication, got.RequireAuthentication)
	checkStr("authentication_prompt", plan.AuthenticationPrompt, got.AuthenticationPrompt)
	checkStr("device_enrollment_program_instance_id", plan.DeviceEnrollmentProgramInstanceID, got.DeviceEnrollmentProgramInstanceID)
	checkStr("enrollment_site_id", plan.EnrollmentSiteID, got.EnrollmentSiteID)
	checkBool("keep_existing_location_information", plan.KeepExistingLocationInformation, got.KeepExistingLocationInformation)
	checkBool("keep_existing_site_membership", plan.KeepExistingSiteMembership, got.KeepExistingSiteMembership)
	checkStr("enrollment_customization_id", plan.EnrollmentCustomizationID, got.EnrollmentCustomizationID)
	checkStr("language", plan.Language, got.Language)
	checkStr("region", plan.Region, got.Region)
	checkBool("auto_advance_setup", plan.AutoAdvanceSetup, got.AutoAdvanceSetup)
	checkBool("install_profiles_during_setup", plan.InstallProfilesDuringSetup, got.InstallProfilesDuringSetup)
	checkStr("custom_package_distribution_point_id", plan.CustomPackageDistributionPointID, got.CustomPackageDistributionPointID)
	checkBoolPtrVal("prevent_activation_lock", plan.PreventActivationLock, got.PreventActivationLock)
	checkBool("enable_device_based_activation_lock", plan.EnableDeviceBasedActivationLock, got.EnableDeviceBasedActivationLock)
	checkBoolPtrVal("enable_recovery_lock", plan.EnableRecoveryLock, got.EnableRecoveryLock)
	checkStr("recovery_lock_password_type", plan.RecoveryLockPasswordType, got.RecoveryLockPasswordType)
	checkBool("rotate_recovery_lock_password", plan.RotateRecoveryLockPassword, got.RotateRecoveryLockPassword)
	checkStr("prestage_minimum_os_target_version_type", plan.PrestageMinimumOsTargetVersionType, got.PrestageMinimumOsTargetVersionType)
	checkStr("minimum_os_specific_version", plan.MinimumOsSpecificVersion, got.MinimumOsSpecificVersion)
	checkBool("psso_enabled", plan.PssoEnabled, got.PssoEnabled)
	checkStr("platform_sso_app_bundle_id", plan.PlatformSsoAppBundleID, got.PlatformSsoAppBundleID)
	checkStrPtr("psso_config_profile_id", plan.PssoConfigProfileID, got.PssoConfigProfileID)

	if planAnchors, d := stringListToSlice(ctx, plan.AnchorCertificates); !d.HasError() && planAnchors != nil {
		if !equalStringSlices(planAnchors, got.AnchorCertificates) {
			mismatched = append(mismatched, "anchor_certificates")
		}
	}
	if planSet, d := stringSetToSlice(ctx, plan.CustomPackageIds); !d.HasError() && planSet != nil {
		if !equalStringSetsUnordered(planSet, got.CustomPackageIds) {
			mismatched = append(mismatched, "custom_package_ids")
		}
	}
	if planSet, d := stringSetToSlice(ctx, plan.PrestageInstalledProfileIds); !d.HasError() && planSet != nil {
		if !equalStringSetsUnordered(planSet, got.PrestageInstalledProfileIds) {
			mismatched = append(mismatched, "prestage_installed_profile_ids")
		}
	}

	// Nested blocks — walk each populated plan-side model against the
	// server's GET result. The PUT-500 silent-rollback case (F4b) often
	// affects nested fields without surfacing in errors[], so the diff
	// MUST cover the whole user-visible surface.
	diffSkipSetupItems(plan.SkipSetupItems, got.SkipSetupItems, &mismatched)
	diffLocationInformation(plan.LocationInformation, got.LocationInformation, &mismatched)
	diffPurchasingInformation(plan.PurchasingInformation, got.PurchasingInformation, &mismatched)
	diffAccountSettings(plan.AccountSettings, got.AccountSettings, &mismatched)

	return mismatched
}

func diffSkipSetupItems(plan *SkipSetupItemsModel, got map[string]bool, out *[]string) {
	if plan == nil || got == nil {
		return
	}
	check := func(field, wireKey string, v types.Bool) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueBool() != got[wireKey] {
			*out = append(*out, "skip_setup_items."+field)
		}
	}
	check("biometric", "Biometric", plan.Biometric)
	check("filevault", "FileVault", plan.FileVault)
	check("software_update", "SoftwareUpdate", plan.SoftwareUpdate)
	check("diagnostics", "Diagnostics", plan.Diagnostics)
	check("accessibility", "Accessibility", plan.Accessibility)
	check("intelligence", "Intelligence", plan.Intelligence)
	check("screen_time", "ScreenTime", plan.ScreenTime)
	check("siri", "Siri", plan.Siri)
	check("restore", "Restore", plan.Restore)
	check("privacy", "Privacy", plan.Privacy)
	check("registration", "Registration", plan.Registration)
	check("enable_lockdown_mode", "EnableLockdownMode", plan.EnableLockdownMode)
	check("terms_of_address", "TermsOfAddress", plan.TermsOfAddress)
	check("icloud_diagnostics", "iCloudDiagnostics", plan.ICloudDiagnostics)
	check("apple_id", "AppleID", plan.AppleID)
	check("display_tone", "DisplayTone", plan.DisplayTone)
	check("appearance", "Appearance", plan.Appearance)
	check("payment", "Payment", plan.Payment)
	check("tos", "TOS", plan.TOS)
	check("os_showcase", "OSShowcase", plan.OSShowcase)
	check("welcome", "Welcome", plan.Welcome)
	check("wallpaper", "Wallpaper", plan.Wallpaper)
	check("icloud_storage", "iCloudStorage", plan.ICloudStorage)
	check("additional_privacy_settings", "AdditionalPrivacySettings", plan.AdditionalPrivacySettings)
	check("location", "Location", plan.Location)
}

func diffLocationInformation(plan *LocationInformationModel, got *pro.LocationInformationV2, out *[]string) {
	if plan == nil || got == nil {
		return
	}
	checkStr := func(field string, v types.String, gotVal string) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueString() != gotVal {
			*out = append(*out, "location_information."+field)
		}
	}
	checkStr("username", plan.Username, got.Username)
	checkStr("realname", plan.Realname, got.Realname)
	checkStr("phone", plan.Phone, got.Phone)
	checkStr("email", plan.Email, got.Email)
	checkStr("room", plan.Room, got.Room)
	checkStr("position", plan.Position, got.Position)
	checkStr("building_id", plan.BuildingID, got.BuildingID)
	checkStr("department_id", plan.DepartmentID, got.DepartmentID)
}

func diffPurchasingInformation(plan *PurchasingInformationModel, got *pro.PrestagePurchasingInformationV2, out *[]string) {
	if plan == nil || got == nil {
		return
	}
	checkStr := func(field string, v types.String, gotVal string) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueString() != gotVal {
			*out = append(*out, "purchasing_information."+field)
		}
	}
	checkBool := func(field string, v types.Bool, gotVal bool) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueBool() != gotVal {
			*out = append(*out, "purchasing_information."+field)
		}
	}
	checkBool("leased", plan.Leased, got.Leased)
	checkBool("purchased", plan.Purchased, got.Purchased)
	checkStr("apple_care_id", plan.AppleCareID, got.AppleCareID)
	checkStr("po_number", plan.PoNumber, got.PoNumber)
	checkStr("vendor", plan.Vendor, got.Vendor)
	checkStr("purchase_price", plan.PurchasePrice, got.PurchasePrice)
	if !plan.LifeExpectancy.IsNull() && !plan.LifeExpectancy.IsUnknown() {
		if plan.LifeExpectancy.ValueInt64() != int64(got.LifeExpectancy) {
			*out = append(*out, "purchasing_information.life_expectancy")
		}
	}
	checkStr("purchasing_account", plan.PurchasingAccount, got.PurchasingAccount)
	checkStr("purchasing_contact", plan.PurchasingContact, got.PurchasingContact)
	checkStr("lease_date", plan.LeaseDate, got.LeaseDate)
	checkStr("po_date", plan.PoDate, got.PoDate)
	checkStr("warranty_date", plan.WarrantyDate, got.WarrantyDate)
}

func diffAccountSettings(plan *AccountSettingsModel, got *pro.AccountSettingsResponse, out *[]string) {
	if plan == nil || got == nil {
		return
	}
	checkStr := func(field string, v types.String, gotVal string) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueString() != gotVal {
			*out = append(*out, "account_settings."+field)
		}
	}
	checkBool := func(field string, v types.Bool, gotVal bool) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueBool() != gotVal {
			*out = append(*out, "account_settings."+field)
		}
	}
	checkBool("payload_configured", plan.PayloadConfigured, got.PayloadConfigured)
	checkBool("local_admin_account_enabled", plan.LocalAdminAccountEnabled, got.LocalAdminAccountEnabled)
	checkStr("admin_username", plan.AdminUsername, got.AdminUsername)
	// admin_password is WriteOnly — never echoed; skip.
	checkBool("hidden_admin_account", plan.HiddenAdminAccount, got.HiddenAdminAccount)
	checkBool("local_user_managed", plan.LocalUserManaged, got.LocalUserManaged)
	checkStr("user_account_type", plan.UserAccountType, got.UserAccountType)
	checkBool("prefill_primary_account_info_feature_enabled", plan.PrefillPrimaryAccountInfoFeatureEnabled, got.PrefillPrimaryAccountInfoFeatureEnabled)
	checkStr("prefill_type", plan.PrefillType, got.PrefillType)
	checkStr("prefill_account_full_name", plan.PrefillAccountFullName, got.PrefillAccountFullName)
	checkStr("prefill_account_user_name", plan.PrefillAccountUserName, got.PrefillAccountUserName)
	checkBool("prevent_prefill_info_from_modification", plan.PreventPrefillInfoFromModification, got.PreventPrefillInfoFromModification)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStringSetsUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]struct{}, len(a))
	for _, s := range a {
		am[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := am[s]; !ok {
			return false
		}
	}
	return true
}
