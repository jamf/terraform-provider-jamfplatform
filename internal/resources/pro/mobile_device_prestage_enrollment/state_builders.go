// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

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
// prior state, used for the PreserveStringWhenWireEmpty defence.
// versionLocks and nested-block ids are NOT stored in state.
func assignGetToResource(_ context.Context, plan *MobileDevicePrestageEnrollmentResourceModel, state MobileDevicePrestageEnrollmentResourceModel, got *pro.GetMobileDevicePrestageV3) diag.Diagnostics {
	var diags diag.Diagnostics

	plan.ID = types.StringValue(got.ID)
	plan.DisplayName = types.StringValue(got.DisplayName)
	plan.DeviceEnrollmentProgramInstanceID = types.StringValue(got.DeviceEnrollmentProgramInstanceID)

	plan.Mandatory = types.BoolValue(got.Mandatory)
	plan.MdmRemovable = types.BoolValue(got.MDMRemovable)
	plan.RequireAuthentication = types.BoolValue(got.RequireAuthentication)
	plan.Supervised = types.BoolValue(got.Supervised)
	plan.AllowPairing = types.BoolValue(got.AllowPairing)
	plan.AutoAdvanceSetup = types.BoolValue(got.AutoAdvanceSetup)
	plan.ConfigureDeviceBeforeSetupAssistant = types.BoolValue(got.ConfigureDeviceBeforeSetupAssistant)
	plan.DefaultPrestage = types.BoolValue(got.DefaultPrestage)
	plan.SendTimezone = types.BoolValue(got.SendTimezone)
	plan.PreventActivationLock = types.BoolValue(got.PreventActivationLock)
	plan.EnableDeviceBasedActivationLock = types.BoolValue(got.EnableDeviceBasedActivationLock)
	plan.KeepExistingSiteMembership = types.BoolValue(got.KeepExistingSiteMembership)
	plan.KeepExistingLocationInformation = types.BoolValue(got.KeepExistingLocationInformation)
	plan.MultiUser = types.BoolValue(got.MultiUser)
	plan.UseStorageQuotaSize = types.BoolValue(got.UseStorageQuotaSize)
	plan.TemporarySessionOnly = types.BoolValue(got.TemporarySessionOnly)
	plan.EnforceTemporarySessionTimeout = types.BoolValue(got.EnforceTemporarySessionTimeout)
	plan.EnforceUserSessionTimeout = types.BoolValue(got.EnforceUserSessionTimeout)
	plan.PreserveManagedApps = types.BoolValue(got.PreserveManagedApps)
	plan.DoNotUseProfileFromBackup = types.BoolValue(got.DoNotUseProfileFromBackup)
	plan.InstallAppsDuringEnrollment = types.BoolValue(got.InstallAppsDuringEnrollment)
	plan.RtsEnabled = types.BoolValue(got.RtsEnabled)

	// SDK types are plain `string`, so any wire-level null is "" by the time
	// we see it — no null/"" flicker at the Go boundary; use the value
	// directly. (Do NOT use PreserveStringWhenWireEmpty here: on a minimal
	// Create the prior "state" is the plan, whose omitted Optional+Computed
	// fields are Unknown, and the helper would leak that Unknown into the
	// State.Set. The computer sibling documents the same lesson.)
	plan.AuthenticationPrompt = types.StringValue(got.AuthenticationPrompt)
	plan.SupportPhoneNumber = types.StringValue(got.SupportPhoneNumber)
	plan.SupportEmailAddress = types.StringValue(got.SupportEmailAddress)
	plan.Department = types.StringValue(got.Department)
	plan.Timezone = types.StringValue(got.Timezone)
	plan.Language = types.StringValue(got.Language)
	plan.Region = types.StringValue(got.Region)
	plan.EnrollmentSiteID = types.StringValue(got.EnrollmentSiteID)
	plan.EnrollmentCustomizationID = types.StringValue(got.EnrollmentCustomizationID)
	plan.RtsConfigProfileID = types.StringValue(got.RtsConfigProfileID)
	plan.SiteID = types.StringValue(got.SiteID)

	plan.MaximumSharedAccounts = types.Int64Value(int64(got.MaximumSharedAccounts))
	plan.TemporarySessionTimeout = types.Int64Value(int64(got.TemporarySessionTimeout))
	plan.UserSessionTimeout = types.Int64Value(int64(got.UserSessionTimeout))
	plan.StorageQuotaSizeMegabytes = types.Int64Value(int64(got.StorageQuotaSizeMegabytes))

	plan.PrestageMinimumOsTargetVersionTypeIos = types.StringValue(got.PrestageMinimumOsTargetVersionTypeIos)
	plan.PrestageMinimumOsTargetVersionTypeIpad = types.StringValue(got.PrestageMinimumOsTargetVersionTypeIpad)
	plan.MinimumOsSpecificVersionIos = types.StringValue(got.MinimumOsSpecificVersionIos)
	plan.MinimumOsSpecificVersionIpad = types.StringValue(got.MinimumOsSpecificVersionIpad)

	plan.AnchorCertificates = stringSliceToList(got.AnchorCertificates)
	plan.ProfileUUID = types.StringValue(got.ProfileUUID)

	// Optional-only typed-pointer nested blocks (STYLE_GUIDE §303: block
	// omission == "do not manage this section"). The gate MUST key off the
	// model being WRITTEN to state — `plan`, the mutated target — never the
	// prior `state` param. On Update the target is the NEW plan: when the
	// user removes a previously-managed block, the plan pointer is nil while
	// the prior state still holds the populated block. Gating on the prior
	// state would repopulate from the wire and trip Terraform Core's
	// "was null, but now ObjectVal(...)" consistency error at apply. On
	// Create / Read the target is the only model, so the gate is unchanged.
	// Capture presence first — the assignments below reassign each field.
	manageSkipSetupItems := plan.SkipSetupItems != nil
	manageNames := plan.Names != nil
	manageLocationInformation := plan.LocationInformation != nil
	managePurchasingInformation := plan.PurchasingInformation != nil

	if manageSkipSetupItems {
		plan.SkipSetupItems = flattenSkipSetupItems(got.SkipSetupItems)
	} else {
		plan.SkipSetupItems = nil
	}
	if manageNames {
		plan.Names = flattenNames(got.Names)
	} else {
		plan.Names = nil
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

	return diags
}

func flattenSkipSetupItems(m map[string]bool) *SkipSetupItemsModel {
	if m == nil {
		return nil
	}
	return &SkipSetupItemsModel{
		ActionButton:          types.BoolValue(m["ActionButton"]),
		Android:               types.BoolValue(m["Android"]),
		Appearance:            types.BoolValue(m["Appearance"]),
		AppleID:               types.BoolValue(m["AppleID"]),
		Biometric:             types.BoolValue(m["Biometric"]),
		CameraButton:          types.BoolValue(m["CameraButton"]),
		CloudStorage:          types.BoolValue(m["CloudStorage"]),
		Diagnostics:           types.BoolValue(m["Diagnostics"]),
		DisplayTone:           types.BoolValue(m["DisplayTone"]),
		EnableLockdownMode:    types.BoolValue(m["EnableLockdownMode"]),
		ExpressLanguage:       types.BoolValue(m["ExpressLanguage"]),
		HomeButtonSensitivity: types.BoolValue(m["HomeButtonSensitivity"]),
		Intelligence:          types.BoolValue(m["Intelligence"]),
		Keyboard:              types.BoolValue(m["Keyboard"]),
		Location:              types.BoolValue(m["Location"]),
		Multitasking:          types.BoolValue(m["Multitasking"]),
		OSShowcase:            types.BoolValue(m["OSShowcase"]),
		OnBoarding:            types.BoolValue(m["OnBoarding"]),
		Passcode:              types.BoolValue(m["Passcode"]),
		Payment:               types.BoolValue(m["Payment"]),
		PreferredLanguage:     types.BoolValue(m["PreferredLanguage"]),
		Privacy:               types.BoolValue(m["Privacy"]),
		Restore:               types.BoolValue(m["Restore"]),
		RestoreCompleted:      types.BoolValue(m["RestoreCompleted"]),
		SIMSetup:              types.BoolValue(m["SIMSetup"]),
		Safety:                types.BoolValue(m["Safety"]),
		SafetyAndHandling:     types.BoolValue(m["SafetyAndHandling"]),
		ScreenSaver:           types.BoolValue(m["ScreenSaver"]),
		ScreenTime:            types.BoolValue(m["ScreenTime"]),
		Siri:                  types.BoolValue(m["Siri"]),
		SoftwareUpdate:        types.BoolValue(m["SoftwareUpdate"]),
		SpokenLanguage:        types.BoolValue(m["SpokenLanguage"]),
		TOS:                   types.BoolValue(m["TOS"]),
		TVHomeScreenSync:      types.BoolValue(m["TVHomeScreenSync"]),
		TVProviderSignIn:      types.BoolValue(m["TVProviderSignIn"]),
		TVRoom:                types.BoolValue(m["TVRoom"]),
		TapToSetup:            types.BoolValue(m["TapToSetup"]),
		TermsOfAddress:        types.BoolValue(m["TermsOfAddress"]),
		TransferData:          types.BoolValue(m["TransferData"]),
		UpdateCompleted:       types.BoolValue(m["UpdateCompleted"]),
		VoiceSelection:        types.BoolValue(m["VoiceSelection"]),
		WatchMigration:        types.BoolValue(m["WatchMigration"]),
		Welcome:               types.BoolValue(m["Welcome"]),
		Zoom:                  types.BoolValue(m["Zoom"]),
		IMessageAndFaceTime:   types.BoolValue(m["iMessageAndFaceTime"]),
	}
}

func flattenNames(n *pro.MobileDevicePrestageNamesV3) *NamesModel {
	if n == nil {
		return nil
	}
	out := &NamesModel{
		AssignNamesUsing: helpers.StringPointerValueOrNull(n.AssignNamesUsing),
		ManageNames:      helpers.BoolPointerValueOrNull(n.ManageNames),
		DeviceNamePrefix: helpers.StringPointerValueOrNull(n.DeviceNamePrefix),
		DeviceNameSuffix: helpers.StringPointerValueOrNull(n.DeviceNameSuffix),
		SingleDeviceName: helpers.StringPointerValueOrNull(n.SingleDeviceName),
	}
	// prestage_device_names is Optional-only (no Computed) — mirror the
	// enrollment_customization text_panes pattern: leave the slice nil when
	// the server returns no entries so a config that omits the list (the
	// non-List naming modes) does not drift against a non-null empty list.
	if n.PrestageDeviceNames != nil && len(*n.PrestageDeviceNames) > 0 {
		elems := make([]PrestageDeviceNameModel, 0, len(*n.PrestageDeviceNames))
		for _, el := range *n.PrestageDeviceNames {
			elems = append(elems, PrestageDeviceNameModel{
				DeviceName: helpers.StringPointerValueOrNull(el.DeviceName),
				ID:         helpers.StringPointerValueOrNull(el.ID),
				Used:       helpers.BoolPointerValueOrNull(el.Used),
			})
		}
		out.PrestageDeviceNames = elems
	}
	return out
}

func flattenLocationInformation(loc *pro.LocationInformationV3) *LocationInformationModel {
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

func flattenPurchasingInformation(pur *pro.PrestagePurchasingInformationV3) *PurchasingInformationModel {
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

// scopeSerialsToSet builds the scope_serial_numbers Set<String> from a fresh
// GET /scope response. assignmentDate / userAssigned are server echoes and are
// dropped (§F7).
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
//
// §9.1 — server-authoritative exclusion set. The following fields are
// deliberately NOT round-tripped by the server; their post-PUT drift is
// expected, NOT a silent rollback, so they are omitted from this check:
//
//	storage_quota_size_megabytes  (recalculated to a device floor, §F8)
//	use_storage_quota_size        (forced false when temp-session wins, §F9)
//	temporary_session_only        (symmetric to above, §F9)
//	temporary_session_timeout     (nulled below min / not enforced, §F11)
//
// anchor_certificates and names/prestage_device_names stay IN the check —
// their mismatch IS a real failure (§F4b). So does default_prestage: it was
// previously excluded as a "conditional singleton" on the assumption Jamf Pro
// silently keeps it false, but a refused claim is a hard 400 ALREADY_DEFAULT
// (wire-probed 2026-08-07), so a mismatch here really is a rollback.
func diffPlanAgainstGet(ctx context.Context, plan MobileDevicePrestageEnrollmentResourceModel, got *pro.GetMobileDevicePrestageV3) []string {
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

	checkStr("display_name", plan.DisplayName, got.DisplayName)
	checkStr("device_enrollment_program_instance_id", plan.DeviceEnrollmentProgramInstanceID, got.DeviceEnrollmentProgramInstanceID)
	checkBool("mandatory", plan.Mandatory, got.Mandatory)
	checkBool("mdm_removable", plan.MdmRemovable, got.MDMRemovable)
	checkBool("require_authentication", plan.RequireAuthentication, got.RequireAuthentication)
	checkBool("supervised", plan.Supervised, got.Supervised)
	checkBool("allow_pairing", plan.AllowPairing, got.AllowPairing)
	checkBool("auto_advance_setup", plan.AutoAdvanceSetup, got.AutoAdvanceSetup)
	checkBool("configure_device_before_setup_assistant", plan.ConfigureDeviceBeforeSetupAssistant, got.ConfigureDeviceBeforeSetupAssistant)
	checkBool("default_prestage", plan.DefaultPrestage, got.DefaultPrestage)
	checkBool("send_timezone", plan.SendTimezone, got.SendTimezone)
	checkBool("prevent_activation_lock", plan.PreventActivationLock, got.PreventActivationLock)
	checkBool("enable_device_based_activation_lock", plan.EnableDeviceBasedActivationLock, got.EnableDeviceBasedActivationLock)
	checkBool("keep_existing_site_membership", plan.KeepExistingSiteMembership, got.KeepExistingSiteMembership)
	checkBool("keep_existing_location_information", plan.KeepExistingLocationInformation, got.KeepExistingLocationInformation)
	checkBool("multi_user", plan.MultiUser, got.MultiUser)
	// use_storage_quota_size — EXCLUDED (§9.1).
	// temporary_session_only — EXCLUDED (§9.1).
	checkBool("enforce_temporary_session_timeout", plan.EnforceTemporarySessionTimeout, got.EnforceTemporarySessionTimeout)
	checkBool("enforce_user_session_timeout", plan.EnforceUserSessionTimeout, got.EnforceUserSessionTimeout)
	checkBool("preserve_managed_apps", plan.PreserveManagedApps, got.PreserveManagedApps)
	checkBool("do_not_use_profile_from_backup", plan.DoNotUseProfileFromBackup, got.DoNotUseProfileFromBackup)
	checkBool("install_apps_during_enrollment", plan.InstallAppsDuringEnrollment, got.InstallAppsDuringEnrollment)
	checkBool("rts_enabled", plan.RtsEnabled, got.RtsEnabled)

	checkStr("authentication_prompt", plan.AuthenticationPrompt, got.AuthenticationPrompt)
	checkStr("support_phone_number", plan.SupportPhoneNumber, got.SupportPhoneNumber)
	checkStr("support_email_address", plan.SupportEmailAddress, got.SupportEmailAddress)
	checkStr("department", plan.Department, got.Department)
	checkStr("timezone", plan.Timezone, got.Timezone)
	checkStr("language", plan.Language, got.Language)
	checkStr("region", plan.Region, got.Region)
	checkStr("enrollment_site_id", plan.EnrollmentSiteID, got.EnrollmentSiteID)
	checkStr("enrollment_customization_id", plan.EnrollmentCustomizationID, got.EnrollmentCustomizationID)
	checkStr("rts_config_profile_id", plan.RtsConfigProfileID, got.RtsConfigProfileID)
	checkStr("prestage_minimum_os_target_version_type_ios", plan.PrestageMinimumOsTargetVersionTypeIos, got.PrestageMinimumOsTargetVersionTypeIos)
	checkStr("prestage_minimum_os_target_version_type_ipad", plan.PrestageMinimumOsTargetVersionTypeIpad, got.PrestageMinimumOsTargetVersionTypeIpad)
	checkStr("minimum_os_specific_version_ios", plan.MinimumOsSpecificVersionIos, got.MinimumOsSpecificVersionIos)
	checkStr("minimum_os_specific_version_ipad", plan.MinimumOsSpecificVersionIpad, got.MinimumOsSpecificVersionIpad)

	// storage_quota_size_megabytes / temporary_session_timeout — EXCLUDED (§9.1).
	if !plan.MaximumSharedAccounts.IsNull() && !plan.MaximumSharedAccounts.IsUnknown() {
		if plan.MaximumSharedAccounts.ValueInt64() != int64(got.MaximumSharedAccounts) {
			mismatched = append(mismatched, "maximum_shared_accounts")
		}
	}
	if !plan.UserSessionTimeout.IsNull() && !plan.UserSessionTimeout.IsUnknown() {
		if plan.UserSessionTimeout.ValueInt64() != int64(got.UserSessionTimeout) {
			mismatched = append(mismatched, "user_session_timeout")
		}
	}

	if planAnchors, d := stringListToSlice(ctx, plan.AnchorCertificates); !d.HasError() && planAnchors != nil {
		if !equalStringSlices(planAnchors, got.AnchorCertificates) {
			mismatched = append(mismatched, "anchor_certificates")
		}
	}

	// Nested blocks. names/prestage_device_names stay in the rollback check
	// (§F4b — a missing-id rollback drops the whole names mutation).
	diffSkipSetupItems(plan.SkipSetupItems, got.SkipSetupItems, &mismatched)
	diffNames(plan.Names, got.Names, &mismatched)
	diffLocationInformation(plan.LocationInformation, got.LocationInformation, &mismatched)
	diffPurchasingInformation(plan.PurchasingInformation, got.PurchasingInformation, &mismatched)

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
	check("action_button", "ActionButton", plan.ActionButton)
	check("android", "Android", plan.Android)
	check("appearance", "Appearance", plan.Appearance)
	check("apple_id", "AppleID", plan.AppleID)
	check("biometric", "Biometric", plan.Biometric)
	check("camera_button", "CameraButton", plan.CameraButton)
	check("cloud_storage", "CloudStorage", plan.CloudStorage)
	check("diagnostics", "Diagnostics", plan.Diagnostics)
	check("display_tone", "DisplayTone", plan.DisplayTone)
	check("enable_lockdown_mode", "EnableLockdownMode", plan.EnableLockdownMode)
	check("express_language", "ExpressLanguage", plan.ExpressLanguage)
	check("home_button_sensitivity", "HomeButtonSensitivity", plan.HomeButtonSensitivity)
	check("intelligence", "Intelligence", plan.Intelligence)
	check("keyboard", "Keyboard", plan.Keyboard)
	check("location", "Location", plan.Location)
	check("multitasking", "Multitasking", plan.Multitasking)
	check("os_showcase", "OSShowcase", plan.OSShowcase)
	check("onboarding", "OnBoarding", plan.OnBoarding)
	check("passcode", "Passcode", plan.Passcode)
	check("payment", "Payment", plan.Payment)
	check("preferred_language", "PreferredLanguage", plan.PreferredLanguage)
	check("privacy", "Privacy", plan.Privacy)
	check("restore", "Restore", plan.Restore)
	check("restore_completed", "RestoreCompleted", plan.RestoreCompleted)
	check("sim_setup", "SIMSetup", plan.SIMSetup)
	check("safety", "Safety", plan.Safety)
	check("safety_and_handling", "SafetyAndHandling", plan.SafetyAndHandling)
	check("screen_saver", "ScreenSaver", plan.ScreenSaver)
	check("screen_time", "ScreenTime", plan.ScreenTime)
	check("siri", "Siri", plan.Siri)
	check("software_update", "SoftwareUpdate", plan.SoftwareUpdate)
	check("spoken_language", "SpokenLanguage", plan.SpokenLanguage)
	check("tos", "TOS", plan.TOS)
	check("tv_home_screen_sync", "TVHomeScreenSync", plan.TVHomeScreenSync)
	check("tv_provider_sign_in", "TVProviderSignIn", plan.TVProviderSignIn)
	check("tv_room", "TVRoom", plan.TVRoom)
	check("tap_to_setup", "TapToSetup", plan.TapToSetup)
	check("terms_of_address", "TermsOfAddress", plan.TermsOfAddress)
	check("transfer_data", "TransferData", plan.TransferData)
	check("update_completed", "UpdateCompleted", plan.UpdateCompleted)
	check("voice_selection", "VoiceSelection", plan.VoiceSelection)
	check("watch_migration", "WatchMigration", plan.WatchMigration)
	check("welcome", "Welcome", plan.Welcome)
	check("zoom", "Zoom", plan.Zoom)
	check("imessage_and_facetime", "iMessageAndFaceTime", plan.IMessageAndFaceTime)
}

func diffNames(plan *NamesModel, got *pro.MobileDevicePrestageNamesV3, out *[]string) {
	if plan == nil || got == nil {
		return
	}
	checkStr := func(field string, v types.String, gotVal *string) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		g := ""
		if gotVal != nil {
			g = *gotVal
		}
		if v.ValueString() != g {
			*out = append(*out, "names."+field)
		}
	}
	checkBool := func(field string, v types.Bool, gotVal *bool) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		g := false
		if gotVal != nil {
			g = *gotVal
		}
		if v.ValueBool() != g {
			*out = append(*out, "names."+field)
		}
	}
	checkStr("assign_names_using", plan.AssignNamesUsing, got.AssignNamesUsing)
	checkBool("manage_names", plan.ManageNames, got.ManageNames)
	checkStr("device_name_prefix", plan.DeviceNamePrefix, got.DeviceNamePrefix)
	checkStr("device_name_suffix", plan.DeviceNameSuffix, got.DeviceNameSuffix)
	checkStr("single_device_name", plan.SingleDeviceName, got.SingleDeviceName)

	// prestage_device_names: compare the set of device names the user
	// authored. A missing-id silent rollback (§F4b) drops elements, so a
	// size or content mismatch flags the rollback.
	if len(plan.PrestageDeviceNames) > 0 {
		gotNames := map[string]struct{}{}
		if got.PrestageDeviceNames != nil {
			for _, el := range *got.PrestageDeviceNames {
				if el.DeviceName != nil {
					gotNames[*el.DeviceName] = struct{}{}
				}
			}
		}
		for _, el := range plan.PrestageDeviceNames {
			if el.DeviceName.IsNull() || el.DeviceName.IsUnknown() {
				continue
			}
			if _, ok := gotNames[el.DeviceName.ValueString()]; !ok {
				*out = append(*out, "names.prestage_device_names")
				break
			}
		}
	}
}

func diffLocationInformation(plan *LocationInformationModel, got *pro.LocationInformationV3, out *[]string) {
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

func diffPurchasingInformation(plan *PurchasingInformationModel, got *pro.PrestagePurchasingInformationV3, out *[]string) {
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
