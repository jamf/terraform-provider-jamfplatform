// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildPostInput translates the plan into the SDK create body used by Create.
//
// POST requirements (spike §F1/§F2/§F3): locationInformation &
// purchasingInformation must be FULLY populated (id="-1", versionLock=0, every
// scalar); `names` must be a populated object (empty names:{} → server 500);
// storageQuotaSizeMegabytes >= 1024 even when useStorageQuotaSize is false.
func buildPostInput(ctx context.Context, plan, cfg MobileDevicePrestageEnrollmentResourceModel) (*pro.MobileDevicePrestageV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	post := &pro.MobileDevicePrestageV3{
		DisplayName:                       plan.DisplayName.ValueString(),
		DeviceEnrollmentProgramInstanceID: plan.DeviceEnrollmentProgramInstanceID.ValueString(),

		Mandatory:                           plan.Mandatory.ValueBool(),
		MDMRemovable:                        plan.MdmRemovable.ValueBool(),
		RequireAuthentication:               plan.RequireAuthentication.ValueBool(),
		Supervised:                          plan.Supervised.ValueBool(),
		AllowPairing:                        plan.AllowPairing.ValueBool(),
		AutoAdvanceSetup:                    plan.AutoAdvanceSetup.ValueBool(),
		ConfigureDeviceBeforeSetupAssistant: plan.ConfigureDeviceBeforeSetupAssistant.ValueBool(),
		DefaultPrestage:                     plan.DefaultPrestage.ValueBool(),
		SendTimezone:                        plan.SendTimezone.ValueBool(),
		PreventActivationLock:               plan.PreventActivationLock.ValueBool(),
		EnableDeviceBasedActivationLock:     plan.EnableDeviceBasedActivationLock.ValueBool(),
		KeepExistingSiteMembership:          plan.KeepExistingSiteMembership.ValueBool(),
		KeepExistingLocationInformation:     plan.KeepExistingLocationInformation.ValueBool(),
		MultiUser:                           plan.MultiUser.ValueBool(),
		UseStorageQuotaSize:                 plan.UseStorageQuotaSize.ValueBool(),

		AuthenticationPrompt: plan.AuthenticationPrompt.ValueString(),
		SupportPhoneNumber:   plan.SupportPhoneNumber.ValueString(),
		SupportEmailAddress:  plan.SupportEmailAddress.ValueString(),
		Department:           plan.Department.ValueString(),
		Timezone:             plan.Timezone.ValueString(),

		EnrollmentSiteID: stringOrSentinel(plan.EnrollmentSiteID, sentinelNoneIDDash1),

		MaximumSharedAccounts:     int(plan.MaximumSharedAccounts.ValueInt64()),
		StorageQuotaSizeMegabytes: createStorageQuota(plan.StorageQuotaSizeMegabytes),

		Language:                  helpers.OptionalStringPointer(plan.Language),
		Region:                    helpers.OptionalStringPointer(plan.Region),
		EnrollmentCustomizationID: helpers.OptionalStringPointer(plan.EnrollmentCustomizationID),
		// rtsConfigProfileId is an id-sentinel field: the PUT rejects "" with
		// 400 INVALID_ID ("must be string of positive numeric value or -1").
		// On update, UseStateForUnknown feeds back the server-echoed "" when no
		// RTS profile is set, so normalise null/empty to the "-1" none sentinel
		// (never send a bare pointer-to-empty-string). Mirrors enrollment_site_id.
		RtsConfigProfileID:                     stringPtrOrSentinel(plan.RtsConfigProfileID, sentinelNoneIDDash1),
		MinimumOsSpecificVersionIos:            helpers.OptionalStringPointer(plan.MinimumOsSpecificVersionIos),
		MinimumOsSpecificVersionIpad:           helpers.OptionalStringPointer(plan.MinimumOsSpecificVersionIpad),
		PrestageMinimumOsTargetVersionTypeIos:  helpers.OptionalStringPointer(plan.PrestageMinimumOsTargetVersionTypeIos),
		PrestageMinimumOsTargetVersionTypeIpad: helpers.OptionalStringPointer(plan.PrestageMinimumOsTargetVersionTypeIpad),

		RtsEnabled:                     helpers.OptionalBoolPointer(plan.RtsEnabled),
		TemporarySessionOnly:           helpers.OptionalBoolPointer(plan.TemporarySessionOnly),
		EnforceTemporarySessionTimeout: helpers.OptionalBoolPointer(plan.EnforceTemporarySessionTimeout),
		EnforceUserSessionTimeout:      helpers.OptionalBoolPointer(plan.EnforceUserSessionTimeout),
		PreserveManagedApps:            helpers.OptionalBoolPointer(plan.PreserveManagedApps),
		DoNotUseProfileFromBackup:      helpers.OptionalBoolPointer(plan.DoNotUseProfileFromBackup),
		InstallAppsDuringEnrollment:    helpers.OptionalBoolPointer(plan.InstallAppsDuringEnrollment),

		TemporarySessionTimeout: helpers.OptionalInt64Pointer(plan.TemporarySessionTimeout),
		UserSessionTimeout:      helpers.OptionalInt64Pointer(plan.UserSessionTimeout),
	}

	anchors, d := stringListToSlice(ctx, plan.AnchorCertificates)
	diags.Append(d...)
	safeAnchors := nilSafeStringSlice(anchors)
	post.AnchorCertificates = &safeAnchors

	post.LocationInformation = buildLocationInformation(plan.LocationInformation, sentinelNestedIDForCreate, 0)
	post.PurchasingInformation = buildPurchasingInformation(plan.PurchasingInformation, sentinelNestedIDForCreate, 0)
	post.SkipSetupItems = buildSkipSetupItemsMap(plan.SkipSetupItems)
	// `names` must ALWAYS be a populated object on POST — an empty names:{}
	// triggers a server 500 (§F2). buildNames synthesizes a default block
	// when the user omitted it. id="-1" on every element (§F4b).
	post.Names = buildNames(plan.Names, namingIntended(cfg.Names), true)

	return post, diags
}

// buildPutInput translates the plan + GET-derived state into the PUT body.
// versionLocks are NOT set here — caller invokes injectVersionLocks. PUT is
// full-replace, so every managed field is emitted (§F6).
func buildPutInput(ctx context.Context, plan, cfg MobileDevicePrestageEnrollmentResourceModel) (*pro.PutMobileDevicePrestageV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	put := &pro.PutMobileDevicePrestageV3{
		DisplayName:                       plan.DisplayName.ValueString(),
		DeviceEnrollmentProgramInstanceID: plan.DeviceEnrollmentProgramInstanceID.ValueString(),

		Mandatory:                           plan.Mandatory.ValueBool(),
		MDMRemovable:                        plan.MdmRemovable.ValueBool(),
		RequireAuthentication:               plan.RequireAuthentication.ValueBool(),
		Supervised:                          plan.Supervised.ValueBool(),
		AllowPairing:                        plan.AllowPairing.ValueBool(),
		AutoAdvanceSetup:                    plan.AutoAdvanceSetup.ValueBool(),
		ConfigureDeviceBeforeSetupAssistant: plan.ConfigureDeviceBeforeSetupAssistant.ValueBool(),
		DefaultPrestage:                     plan.DefaultPrestage.ValueBool(),
		SendTimezone:                        plan.SendTimezone.ValueBool(),
		PreventActivationLock:               plan.PreventActivationLock.ValueBool(),
		EnableDeviceBasedActivationLock:     plan.EnableDeviceBasedActivationLock.ValueBool(),
		KeepExistingSiteMembership:          plan.KeepExistingSiteMembership.ValueBool(),
		KeepExistingLocationInformation:     plan.KeepExistingLocationInformation.ValueBool(),
		MultiUser:                           plan.MultiUser.ValueBool(),
		UseStorageQuotaSize:                 plan.UseStorageQuotaSize.ValueBool(),

		AuthenticationPrompt: plan.AuthenticationPrompt.ValueString(),
		SupportPhoneNumber:   plan.SupportPhoneNumber.ValueString(),
		SupportEmailAddress:  plan.SupportEmailAddress.ValueString(),
		Department:           plan.Department.ValueString(),
		Timezone:             plan.Timezone.ValueString(),

		EnrollmentSiteID: stringOrSentinel(plan.EnrollmentSiteID, sentinelNoneIDDash1),

		MaximumSharedAccounts: int(plan.MaximumSharedAccounts.ValueInt64()),
		// storage_quota_size_megabytes is create-only; the plan modifier
		// renders the value Unknown on a change. Flooring here is harmless —
		// the server recalculates regardless (§F8).
		StorageQuotaSizeMegabytes: createStorageQuota(plan.StorageQuotaSizeMegabytes),

		Language:                  helpers.OptionalStringPointer(plan.Language),
		Region:                    helpers.OptionalStringPointer(plan.Region),
		EnrollmentCustomizationID: helpers.OptionalStringPointer(plan.EnrollmentCustomizationID),
		// rtsConfigProfileId is an id-sentinel field: the PUT rejects "" with
		// 400 INVALID_ID ("must be string of positive numeric value or -1").
		// On update, UseStateForUnknown feeds back the server-echoed "" when no
		// RTS profile is set, so normalise null/empty to the "-1" none sentinel
		// (never send a bare pointer-to-empty-string). Mirrors enrollment_site_id.
		RtsConfigProfileID:                     stringPtrOrSentinel(plan.RtsConfigProfileID, sentinelNoneIDDash1),
		MinimumOsSpecificVersionIos:            helpers.OptionalStringPointer(plan.MinimumOsSpecificVersionIos),
		MinimumOsSpecificVersionIpad:           helpers.OptionalStringPointer(plan.MinimumOsSpecificVersionIpad),
		PrestageMinimumOsTargetVersionTypeIos:  helpers.OptionalStringPointer(plan.PrestageMinimumOsTargetVersionTypeIos),
		PrestageMinimumOsTargetVersionTypeIpad: helpers.OptionalStringPointer(plan.PrestageMinimumOsTargetVersionTypeIpad),

		RtsEnabled:                     helpers.OptionalBoolPointer(plan.RtsEnabled),
		TemporarySessionOnly:           helpers.OptionalBoolPointer(plan.TemporarySessionOnly),
		EnforceTemporarySessionTimeout: helpers.OptionalBoolPointer(plan.EnforceTemporarySessionTimeout),
		EnforceUserSessionTimeout:      helpers.OptionalBoolPointer(plan.EnforceUserSessionTimeout),
		PreserveManagedApps:            helpers.OptionalBoolPointer(plan.PreserveManagedApps),
		DoNotUseProfileFromBackup:      helpers.OptionalBoolPointer(plan.DoNotUseProfileFromBackup),
		InstallAppsDuringEnrollment:    helpers.OptionalBoolPointer(plan.InstallAppsDuringEnrollment),

		TemporarySessionTimeout: helpers.OptionalInt64Pointer(plan.TemporarySessionTimeout),
		UserSessionTimeout:      helpers.OptionalInt64Pointer(plan.UserSessionTimeout),
	}

	anchors, d := stringListToSlice(ctx, plan.AnchorCertificates)
	diags.Append(d...)
	safeAnchors := nilSafeStringSlice(anchors)
	put.AnchorCertificates = &safeAnchors

	put.LocationInformation = buildLocationInformation(plan.LocationInformation, "", 0)
	put.PurchasingInformation = buildPurchasingInformation(plan.PurchasingInformation, "", 0)
	put.SkipSetupItems = buildSkipSetupItemsMap(plan.SkipSetupItems)
	// `names` must ALWAYS be a populated object on PUT too (full-replace).
	// On update, prestage_device_names elements echo their state-derived id
	// (carried in via UseNonNullStateForUnknown) — omitting id silently rolls
	// the whole names mutation back (§F4b).
	put.Names = buildNames(plan.Names, namingIntended(cfg.Names), false)

	return put, diags
}

// createStorageQuota returns a wire value for storageQuotaSizeMegabytes,
// flooring to minStorageQuotaMegabytes (1024). Jamf Pro rejects a POST with a
// smaller value even when useStorageQuotaSize is false (§F3); unset/unknown
// defaults to the floor.
func createStorageQuota(v types.Int64) int {
	if v.IsNull() || v.IsUnknown() {
		return minStorageQuotaMegabytes
	}
	q := int(v.ValueInt64())
	if q < minStorageQuotaMegabytes {
		return minStorageQuotaMegabytes
	}
	return q
}

// buildNames assembles the SDK MobileDevicePrestageNamesV3. The block must
// always be populated on the wire (§F2). When the user omitted `names`
// (m == nil) a safe default object is synthesized. Every prestage_device_names
// element serialises with id (state value, else "-1") AND used (§F4b).
//
// isCreate forces id="-1" for every element (no server ids exist yet).
//
// namingConfigured is the caller-derived deviceNamingConfigured value. It MUST
// come from the CONFIG (namingIntended(cfg.Names)), never from the plan: the
// Optional+Computed siblings carry UseStateForUnknown, so on update the plan's
// assign_names_using holds the server's echoed default even for a bare
// `names = {}`, which would flip an unconfigured PreStage to configured on any
// unrelated edit.
func buildNames(m *NamesModel, namingConfigured, isCreate bool) *pro.MobileDevicePrestageNamesV3 {
	out := &pro.MobileDevicePrestageNamesV3{}
	out.DeviceNamingConfigured = new(namingConfigured)

	if m == nil {
		// Synthesized default — empty names:{} would 500 the server.
		def := defaultAssignNamesUsing
		manage := false
		empty := []pro.MobileDevicePrestageNameV3{}
		out.AssignNamesUsing = &def
		out.ManageNames = &manage
		out.PrestageDeviceNames = &empty
		return out
	}

	out.AssignNamesUsing = stringPtrOrSentinel(m.AssignNamesUsing, defaultAssignNamesUsing)
	out.ManageNames = boolPtrOrFalse(m.ManageNames)
	out.DeviceNamePrefix = optionalStringPtr(m.DeviceNamePrefix)
	out.DeviceNameSuffix = optionalStringPtr(m.DeviceNameSuffix)
	out.SingleDeviceName = optionalStringPtr(m.SingleDeviceName)

	names := make([]pro.MobileDevicePrestageNameV3, 0, len(m.PrestageDeviceNames))
	for i := range m.PrestageDeviceNames {
		el := m.PrestageDeviceNames[i]
		deviceName := el.DeviceName.ValueString()
		id := sentinelNameIDForCreate
		if !isCreate && !el.ID.IsNull() && !el.ID.IsUnknown() && el.ID.ValueString() != "" {
			id = el.ID.ValueString()
		}
		used := false
		if !el.Used.IsNull() && !el.Used.IsUnknown() {
			used = el.Used.ValueBool()
		}
		dn := deviceName
		eid := id
		eused := used
		names = append(names, pro.MobileDevicePrestageNameV3{
			DeviceName: &dn,
			ID:         &eid,
			Used:       &eused,
		})
	}
	out.PrestageDeviceNames = &names
	return out
}

// buildLocationInformation populates the SDK LocationInformationV3 from the
// nested block. nestedID / nestedLock are echoed verbatim — Create passes "-1"
// + 0 to request a fresh server-side record; Update is overwritten by
// injectVersionLocks after this returns.
func buildLocationInformation(m *LocationInformationModel, nestedID string, nestedLock int) pro.LocationInformationV3 {
	out := pro.LocationInformationV3{
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

func buildPurchasingInformation(m *PurchasingInformationModel, nestedID string, nestedLock int) pro.PrestagePurchasingInformationV3 {
	out := pro.PrestagePurchasingInformationV3{
		ID:           nestedID,
		VersionLock:  nestedLock,
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

// buildSkipSetupItemsMap builds the wire map from the model. Sends all 45 keys
// (the server echoes the full set, §F12); a null/unknown model bool serialises
// as false. Returns nil when the user omitted the block.
func buildSkipSetupItemsMap(m *SkipSetupItemsModel) *map[string]bool {
	if m == nil {
		return nil
	}
	out := map[string]bool{
		"ActionButton":          m.ActionButton.ValueBool(),
		"Android":               m.Android.ValueBool(),
		"Appearance":            m.Appearance.ValueBool(),
		"AppleID":               m.AppleID.ValueBool(),
		"Biometric":             m.Biometric.ValueBool(),
		"CameraButton":          m.CameraButton.ValueBool(),
		"CloudStorage":          m.CloudStorage.ValueBool(),
		"Diagnostics":           m.Diagnostics.ValueBool(),
		"DisplayTone":           m.DisplayTone.ValueBool(),
		"EnableLockdownMode":    m.EnableLockdownMode.ValueBool(),
		"ExpressLanguage":       m.ExpressLanguage.ValueBool(),
		"HomeButtonSensitivity": m.HomeButtonSensitivity.ValueBool(),
		"Intelligence":          m.Intelligence.ValueBool(),
		"Keyboard":              m.Keyboard.ValueBool(),
		"Location":              m.Location.ValueBool(),
		"Multitasking":          m.Multitasking.ValueBool(),
		"OSShowcase":            m.OSShowcase.ValueBool(),
		"OnBoarding":            m.OnBoarding.ValueBool(),
		"Passcode":              m.Passcode.ValueBool(),
		"Payment":               m.Payment.ValueBool(),
		"PreferredLanguage":     m.PreferredLanguage.ValueBool(),
		"Privacy":               m.Privacy.ValueBool(),
		"Restore":               m.Restore.ValueBool(),
		"RestoreCompleted":      m.RestoreCompleted.ValueBool(),
		"SIMSetup":              m.SIMSetup.ValueBool(),
		"Safety":                m.Safety.ValueBool(),
		"SafetyAndHandling":     m.SafetyAndHandling.ValueBool(),
		"ScreenSaver":           m.ScreenSaver.ValueBool(),
		"ScreenTime":            m.ScreenTime.ValueBool(),
		"Siri":                  m.Siri.ValueBool(),
		"SoftwareUpdate":        m.SoftwareUpdate.ValueBool(),
		"SpokenLanguage":        m.SpokenLanguage.ValueBool(),
		"TOS":                   m.TOS.ValueBool(),
		"TVHomeScreenSync":      m.TVHomeScreenSync.ValueBool(),
		"TVProviderSignIn":      m.TVProviderSignIn.ValueBool(),
		"TVRoom":                m.TVRoom.ValueBool(),
		"TapToSetup":            m.TapToSetup.ValueBool(),
		"TermsOfAddress":        m.TermsOfAddress.ValueBool(),
		"TransferData":          m.TransferData.ValueBool(),
		"UpdateCompleted":       m.UpdateCompleted.ValueBool(),
		"VoiceSelection":        m.VoiceSelection.ValueBool(),
		"WatchMigration":        m.WatchMigration.ValueBool(),
		"Welcome":               m.Welcome.ValueBool(),
		"Zoom":                  m.Zoom.ValueBool(),
		"iMessageAndFaceTime":   m.IMessageAndFaceTime.ValueBool(),
	}
	return &out
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
// Jamf Pro V3 PreStage POST/PUT rejects null array fields.
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

// optionalStringPtr returns a *string for a value field, nil when null/unknown.
func optionalStringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// stringPtrOrSentinel returns the value as a *string, defaulting to sentinel
// when null/unknown/empty.
func stringPtrOrSentinel(v types.String, sentinel string) *string {
	s := sentinel
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		s = v.ValueString()
	}
	return &s
}

// namingIntended reports whether a names block expresses any device-naming
// intent, and so what deviceNamingConfigured is written as. A bare `names = {}`
// expresses none. Call it on the CONFIG — see buildNames.
func namingIntended(m *NamesModel) bool {
	if m == nil {
		return false
	}
	return knownNonEmpty(m.AssignNamesUsing) || namingIntentBesidesMode(m)
}

// namingIntentBesidesMode is namingIntended minus assign_names_using, for the
// one caller that has only post-GET state to work from (warnNamingNotConfigured).
// Jamf Pro always echoes an assign_names_using — it is populated even on a
// PreStage where nothing was ever configured — so state cannot distinguish a
// user-set mode from that echo. Every other field here is only non-zero because
// somebody set it, so it is safe evidence of real naming intent.
func namingIntentBesidesMode(m *NamesModel) bool {
	if m == nil {
		return false
	}
	switch {
	case knownNonEmpty(m.DeviceNamePrefix),
		knownNonEmpty(m.DeviceNameSuffix),
		knownNonEmpty(m.SingleDeviceName):
		return true
	case !m.ManageNames.IsNull() && !m.ManageNames.IsUnknown() && m.ManageNames.ValueBool():
		return true
	default:
		return len(m.PrestageDeviceNames) > 0
	}
}

func knownNonEmpty(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

// boolPtrOrFalse returns the value as a *bool, defaulting to false when
// null/unknown.
func boolPtrOrFalse(v types.Bool) *bool {
	b := false
	if !v.IsNull() && !v.IsUnknown() {
		b = v.ValueBool()
	}
	return &b
}
