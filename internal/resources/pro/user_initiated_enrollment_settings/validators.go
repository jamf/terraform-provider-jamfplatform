// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// mdmSigningCertificateRequiredValidator enforces the server invariant: when
// signing_mdm_profile_enabled = true, an MDM signing certificate must be
// present. Jamf Pro rejects an apply with signing_mdm_profile_enabled = true
// and no certificate (HTTP 400 FIELD_REQUIRED mdmSigningCertificateIdentity).
//
// This is a config-only check (ValidateConfig cannot see prior state), so it
// requires the mdm_signing_certificate block in configuration whenever
// signing_mdm_profile_enabled = true. The "a certificate was uploaded on a
// previous apply" exception is handled at apply time by NOT re-uploading when
// nothing changed — not by loosening this validator.
type mdmSigningCertificateRequiredValidator struct{}

var _ resource.ConfigValidator = mdmSigningCertificateRequiredValidator{}

// Description returns the validator description.
func (mdmSigningCertificateRequiredValidator) Description(_ context.Context) string {
	return "signing_mdm_profile_enabled = true requires the mdm_signing_certificate block"
}

// MarkdownDescription returns the markdown description.
func (v mdmSigningCertificateRequiredValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements resource.ConfigValidator.
func (mdmSigningCertificateRequiredValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var enabled types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("signing_mdm_profile_enabled"), &enabled)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if enabled.IsNull() || enabled.IsUnknown() || !enabled.ValueBool() {
		return
	}

	var cert types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("mdm_signing_certificate"), &cert)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Defer when the certificate block is unknown (e.g. driven by a variable,
	// for_each, or another resource): config-time validation cannot see its
	// eventual value, and erroring here would break every non-literal config.
	// Only a genuinely-absent (null) block is an error.
	if cert.IsUnknown() {
		return
	}
	if !cert.IsNull() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("mdm_signing_certificate"),
		"mdm_signing_certificate required when signing_mdm_profile_enabled = true",
		"Using a third-party signing certificate requires an uploaded certificate. Supply the mdm_signing_certificate block (keystore_file + keystore_password), or set signing_mdm_profile_enabled = false.",
	)
}

const (
	// signQuickAddRevertsFrom is the first Jamf Pro version on which a PUT with
	// signQuickAdd = true and no developer certificate is accepted with 200 and then
	// read back as false. Wire-verified on 11.34.0, 2026-11-14. Earlier versions
	// kept the value, which is what the DeveloperCert acceptance test relied on. The
	// change may have landed before 11.34.0: 11.33 was not probed. With a developer
	// certificate present the write was not probed, because no real Apple Developer
	// ID certificate was available, so the refusal below covers the no-certificate
	// case only. Jamf lists the whole feature as being removed in November 2026.
	signQuickAddRevertsFrom = "11.34.0"

	// signQuickAddRevertsBefore is an upper bound no real version reaches, so the
	// strict range helper doubles as a fail-closed "at least" test.
	signQuickAddRevertsBefore = "1000.0.0"
)

// signQuickAddReverts reports whether the tenant version is one that reverts an
// uncertified sign_quickadd_package = true. Fail-closed: an unreadable version
// returns false, because this gates a refusal and a refusal must be certain.
func signQuickAddReverts(version string) bool {
	return helpers.JamfProVersionInRangeStrict(version, signQuickAddRevertsFrom, signQuickAddRevertsBefore)
}

// signQuickAddDiagnostic is the plan-time refusal of sign_quickadd_package = true
// with no developer_certificate on a tenant that would revert it. Without it the
// apply succeeds on the wire and then fails with "Provider produced inconsistent
// result after apply", which names neither the cause nor a fix.
func signQuickAddDiagnostic(version string) diag.Diagnostic {
	return diag.NewAttributeErrorDiagnostic(
		path.Root("sign_quickadd_package"),
		"QuickAdd package signing needs a developer certificate on this Jamf Pro version",
		"Jamf Pro "+version+" accepts sign_quickadd_package = true without a developer_certificate but then reports false, "+
			"so the apply would fail with an inconsistent result. Jamf is removing QuickAdd package signing from "+
			"user-initiated enrollment settings. Remove sign_quickadd_package, or set it to false.",
	)
}

// validateSignQuickAddAccepted refuses sign_quickadd_package = true with no
// developer_certificate block on a tenant that reverts it. It reads the
// configuration, since the attribute is Optional+Computed and an omitted one is
// unknown in the plan. An unreadable version, or one below the cut-off, is not an
// error.
func validateSignQuickAddAccepted(ctx context.Context, pd *providerdata.Data, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if pd == nil {
		return
	}
	var sign types.Bool
	if diags := req.Config.GetAttribute(ctx, path.Root("sign_quickadd_package"), &sign); diags.HasError() {
		return
	}
	if sign.IsNull() || sign.IsUnknown() || !sign.ValueBool() {
		return
	}
	var cert types.Object
	if diags := req.Config.GetAttribute(ctx, path.Root("developer_certificate"), &cert); diags.HasError() {
		return
	}
	if !cert.IsNull() {
		return
	}
	version, err := pd.GetJamfProVersion(ctx)
	if err != nil || !signQuickAddReverts(version) {
		return
	}
	resp.Diagnostics.Append(signQuickAddDiagnostic(version))
}
