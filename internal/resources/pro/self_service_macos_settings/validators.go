// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_macos_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// installLocationRequiredValidator enforces, at plan time, the dependency Jamf Pro enforces
// on the wire: install_automatically = true requires a non-empty install_location. Wire-probed
// 2026-06-10 — HTTP 400 FIELD_REQUIRED ("Install Location must be set to a valid location if
// Install Automatically is true") when the location is blank while auto-install is on; a blank
// location is accepted when auto-install is off.
//
// Coverage is intentionally PARTIAL: a ConfigValidator reads the *config*, not the resolved
// plan, so it fires only when BOTH fields are explicitly declared. An applied value can also
// arrive via UseStateForUnknown (update) or the create-adopt GET — neither visible here — so
// any violation where a field is omitted/preserved falls through to the server 400 as the
// backstop. Defers (no error) when either field is null or unknown, per STYLE_GUIDE
// §Cross-field validation (a null value means "preserve the current server value", which may
// already satisfy the rule).
type installLocationRequiredValidator struct{}

// Description returns a plain-text description of the validator.
func (installLocationRequiredValidator) Description(context.Context) string {
	return "install_automatically = true requires a non-empty install_location."
}

// MarkdownDescription returns the markdown description.
func (v installLocationRequiredValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
func (installLocationRequiredValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	// Collect GetAttribute diagnostics locally: resp.Diagnostics may already carry an error
	// from the sibling validator, so guarding on resp.Diagnostics.HasError() would skip this
	// check after the other validator errors.
	var auto types.Bool
	var location types.String
	diags := req.Config.GetAttribute(ctx, path.Root("install_automatically"), &auto)
	diags.Append(req.Config.GetAttribute(ctx, path.Root("install_location"), &location)...)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if auto.IsNull() || auto.IsUnknown() || location.IsNull() || location.IsUnknown() {
		return
	}

	if auto.ValueBool() && location.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("install_location"),
			"Install location required",
			`"install_location" must be a non-empty path when "install_automatically" is true. Jamf Pro rejects a blank install location while automatic installation is enabled.`,
		)
	}
}

// samlRequiresLoginValidator rejects, at plan time, declaring authentication_type = "Saml"
// together with login_method = "NotRequired". Wire-probed 2026-06-10 (post-spike follow-up):
// with user login disabled the server never keeps Saml — it either rejects the write (HTTP
// 400 PREREQUISITE_NOT_MET "SAML is not enabled for MacOS" when changing Basic→Saml) or
// accepts it and silently coerces the stored value back to Basic (when Saml was already
// stored), which would fail the apply with "inconsistent result". Either way a declared
// Saml + NotRequired pair cannot converge.
//
// Same PARTIAL-coverage contract as the other validators: fires only when BOTH fields are
// explicitly declared; defers when either is null or unknown. The carried-not-declared case
// (login_method transitions to NotRequired while Saml rides in via UseStateForUnknown) is
// handled by ModifyPlan instead, which marks authentication_type Unknown so apply adopts
// the server's coercion.
type samlRequiresLoginValidator struct{}

// Description returns a plain-text description of the validator.
func (samlRequiresLoginValidator) Description(context.Context) string {
	return `authentication_type = "Saml" requires login_method to be "Anonymous" or "Required".`
}

// MarkdownDescription returns the markdown description.
func (v samlRequiresLoginValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
func (samlRequiresLoginValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var loginMethod, authType types.String
	diags := req.Config.GetAttribute(ctx, path.Root("login_method"), &loginMethod)
	diags.Append(req.Config.GetAttribute(ctx, path.Root("authentication_type"), &authType)...)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if loginMethod.IsNull() || loginMethod.IsUnknown() || authType.IsNull() || authType.IsUnknown() {
		return
	}

	if authType.ValueString() == pro.SelfServiceLoginSettingsAuthTypeSaml && loginMethod.ValueString() == pro.SelfServiceLoginSettingsUserLoginLevelNotRequired {
		resp.Diagnostics.AddAttributeError(
			path.Root("authentication_type"),
			"Single Sign-On requires user login",
			`"authentication_type" cannot be "Saml" while "login_method" is "NotRequired". Jamf Pro does not keep Single Sign-On while Self Service user login is disabled — it rejects the write or silently reverts to "Basic".`,
		)
	}
}

// categoryRequiresBrowseValidator defuses, at plan time, a silent server-side coercion:
// default_home_category_id only applies when default_landing_page = "BROWSE". Wire-probed
// 2026-06-10 — under any other landing page Jamf Pro silently resets ANY submitted category
// id to -1 (All Items) with no error, which would otherwise surface as an "inconsistent
// result after apply" failure (the provider refreshes state from the server after every
// write). Under BROWSE the id is validated instead (HTTP 400 INVALID_ID for an unknown
// category; ids below -4 are rejected — the int64validator.AtLeast(-4) on the attribute
// mirrors that floor).
//
// Same PARTIAL-coverage contract as installLocationRequiredValidator: fires only when BOTH
// fields are explicitly declared; defers when either is null or unknown (an omitted landing
// page preserves the server value, which may already be BROWSE).
type categoryRequiresBrowseValidator struct{}

// Description returns a plain-text description of the validator.
func (categoryRequiresBrowseValidator) Description(context.Context) string {
	return `default_home_category_id other than -1 requires default_landing_page = "BROWSE".`
}

// MarkdownDescription returns the markdown description.
func (v categoryRequiresBrowseValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
func (categoryRequiresBrowseValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var categoryID types.Int64
	var landingPage types.String
	diags := req.Config.GetAttribute(ctx, path.Root("default_home_category_id"), &categoryID)
	diags.Append(req.Config.GetAttribute(ctx, path.Root("default_landing_page"), &landingPage)...)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if categoryID.IsNull() || categoryID.IsUnknown() || landingPage.IsNull() || landingPage.IsUnknown() {
		return
	}

	if categoryID.ValueInt64() != -1 && landingPage.ValueString() != pro.SelfServiceInteractionSettingsDefaultLandingPageBrowse {
		resp.Diagnostics.AddAttributeError(
			path.Root("default_home_category_id"),
			"Default home category requires the Browse landing page",
			`"default_home_category_id" other than -1 requires "default_landing_page" to be "BROWSE". With any other landing page Jamf Pro silently resets the category to -1 (All Items), so the applied value would not match the configuration.`,
		)
	}
}
