// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"encoding/base64"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// mergeEnrollmentSettingsInput builds the /v4 PUT body from the current GET
// body (got) overlaid with the resource's OWNED fields from plan.
//
// The /v4 PUT is full-replace: any omitted scalar resets to its default, so
// the body MUST carry every field. The merge starts from a shallow copy of the
// GET body, which:
//
//   - round-trips the six Re-enrollment fields this resource does NOT own
//     (FlushPolicyHistory, FlushLocationInformation,
//     FlushLocationHistoryInformation, FlushExtensionAttributes,
//     FlushSoftwareUpdatePlans, FlushMDMCommandsOnReenroll) unchanged, and
//   - supplies the server's current value as the fallback for any owned scalar
//     the user left unset on Create (Optional+Computed → server default).
//
// Owned scalars are overwritten only when the plan value is known (not
// null/unknown). On Update the UseStateForUnknown plan modifier makes omitted
// fields known (= prior state), so the overwrite fires; on Create an omitted
// field stays unknown and the GET value is preserved.
//
// Certificate identity objects are always cleared to nil here (omit on the
// wire) so the server preserves any existing certificate. The CRUD orchestrator
// re-populates the relevant cert object only when the user is uploading a new
// certificate this apply. The cert *Details echoes are likewise cleared so a
// GET-side detail object is never echoed back as a write.
func mergeEnrollmentSettingsInput(got pro.EnrollmentSettingsV4, plan UserInitiatedEnrollmentSettingsResourceModel) pro.EnrollmentSettingsV4 {
	body := got

	// Never resend the GET's cert objects or detail echoes — they are null on
	// GET and resending them risks the dangerous explicit form. The CRUD layer
	// sets MDMSigningCertificate / DeveloperCertificateIdentity when uploading.
	body.MDMSigningCertificate = nil
	body.DeveloperCertificateIdentity = nil
	body.MDMSigningCertificateDetails = nil
	body.DeveloperCertificateIdentityDetails = nil

	// personalDeviceEnrollmentType is deprecated and server-controlled; do not
	// resend it (leave nil to omit; the server keeps USERENROLLMENT).
	body.PersonalDeviceEnrollmentType = nil

	// ManagementUsername is a bare string (no omitempty). It is always sent;
	// only overwrite from plan when the plan value is known so Create does not
	// wipe the server's stored value.
	if v := overwriteString(plan.ManagementUsername); v != nil {
		body.ManagementUsername = *v
	}

	// General tab.
	overwriteBool(&body.InstallSingleProfile, plan.SkipCertificateInstallation)
	overwriteBool(&body.RestrictReenrollment, plan.RestrictReenrollment)
	overwriteBool(&body.SigningMDMProfileEnabled, plan.SigningMdmProfileEnabled)

	// Computers tab.
	overwriteBool(&body.MacOsEnterpriseEnrollmentEnabled, plan.EnableComputerEnrollment)
	overwriteBool(&body.CreateManagementAccount, plan.CreateManagementAccount)
	overwriteBool(&body.HideManagementAccount, plan.HideManagementAccount)
	overwriteBool(&body.AllowSshOnlyManagementAccount, plan.AllowSshOnlyManagementAccount)
	overwriteBool(&body.EnsureSshRunning, plan.EnsureSshRunning)
	overwriteBool(&body.LaunchSelfService, plan.LaunchSelfService)
	overwriteBool(&body.SignQuickAdd, plan.SignQuickaddPackage)
	overwriteBool(&body.AccountDrivenDeviceMacosEnrollmentEnabled, plan.AccountDrivenDeviceEnrollmentMac)

	// Devices tab.
	overwriteBool(&body.IosEnterpriseEnrollmentEnabled, plan.ProfileDrivenEnrollmentViaURLInstitutional)
	overwriteBool(&body.IosPersonalEnrollmentEnabled, plan.ProfileDrivenEnrollmentViaURLPersonal)
	overwriteBool(&body.AccountDrivenUserEnrollmentEnabled, plan.AccountDrivenUserEnrollment)
	overwriteBool(&body.AccountDrivenUserVisionosEnrollmentEnabled, plan.AccountDrivenUserEnrollmentVisionos)
	overwriteBool(&body.MaidUsernameMergeEnabled, plan.MergeManagedAppleAccountUsernames)
	overwriteBool(&body.AccountDrivenDeviceIosEnrollmentEnabled, plan.AccountDrivenDeviceEnrollmentIos)
	overwriteBool(&body.AccountDrivenDeviceVisionosEnrollmentEnabled, plan.AccountDrivenDeviceEnrollmentVisionos)

	return body
}

// overwriteBool sets *dst to the plan value when it is known (not null/unknown),
// otherwise leaves *dst unchanged (preserving the GET-derived fallback).
func overwriteBool(dst **bool, v types.Bool) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	b := v.ValueBool()
	*dst = &b
}

// overwriteString returns a pointer to the plan string when known, else nil.
func overwriteString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// defaultKeystoreFilename is sent as the keystore identity filename when the
// user does not supply keystore_file_name. Jamf Pro REQUIRES a non-empty
// filename on the keystore identity: the developer-certificate PUT returns HTTP
// 500 (UPDATE_FAILED) without one, and the MDM signing certificate is silently
// not ingested (subject / serial_number stay empty) without one. Both wire-
// confirmed 2026-05-29.
const defaultKeystoreFilename = "keystore.p12"

// buildCertificateIdentity builds a *CertificateIdentityV2 from a cert
// sub-block's WriteOnly keystore bytes + password (read from Config). Returns
// nil with a non-nil error when the base64 fails to decode.
func buildCertificateIdentity(cfg *certificateModel) (*pro.CertificateIdentityV2, error) {
	keystoreBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.KeystoreFile.ValueString()))
	if err != nil {
		return nil, err
	}
	out := &pro.CertificateIdentityV2{
		IdentityKeystore: &keystoreBytes,
	}
	if !cfg.KeystorePassword.IsNull() && !cfg.KeystorePassword.IsUnknown() {
		pw := strings.TrimSpace(cfg.KeystorePassword.ValueString())
		out.KeystorePassword = &pw
	}
	// Always send a filename — Jamf Pro requires it (see defaultKeystoreFilename).
	filename := defaultKeystoreFilename
	if !cfg.KeystoreFileName.IsNull() && !cfg.KeystoreFileName.IsUnknown() && cfg.KeystoreFileName.ValueString() != "" {
		filename = cfg.KeystoreFileName.ValueString()
	}
	out.Filename = &filename
	return out, nil
}

// buildAccessGroupInput converts a planned access-group model into the SDK
// preview type for a create or update call. The directory group id is resolved
// by the caller (from name + ldap_server_id) and passed in as resolvedGroupID;
// the model's directory_service_group_id is Computed and not authoritative. The
// server id is carried through when present (update path); create omits it.
func buildAccessGroupInput(m accessGroupModel, resolvedGroupID string) *pro.EnrollmentAccessGroupPreview {
	out := &pro.EnrollmentAccessGroupPreview{
		GroupID:      resolvedGroupID,
		LdapServerID: m.LdapServerID.ValueString(),
		Name:         m.Name.ValueString(),
	}
	if !m.ID.IsNull() && !m.ID.IsUnknown() && m.ID.ValueString() != "" {
		id := m.ID.ValueString()
		out.ID = &id
	}
	if !m.SiteID.IsNull() && !m.SiteID.IsUnknown() && m.SiteID.ValueString() != "" {
		site := m.SiteID.ValueString()
		out.SiteID = &site
	}
	out.EnterpriseEnrollmentEnabled = optionalBool(m.EnterpriseEnrollmentEnabled)
	out.PersonalEnrollmentEnabled = optionalBool(m.PersonalEnrollmentEnabled)
	out.AccountDrivenUserEnrollmentEnabled = optionalBool(m.AccountDrivenUserEnrollmentEnabled)
	out.RequireEula = optionalBool(m.RequireEula)
	return out
}

// optionalBool maps a Terraform Bool to *bool, returning nil for null/unknown.
func optionalBool(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}
