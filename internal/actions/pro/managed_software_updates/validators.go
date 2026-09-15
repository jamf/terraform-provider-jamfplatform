// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// forceInstallLocalDateTimePattern matches the local date/time form Jamf Pro
// parses for force_install_local_date_time. Wire-probed 2026-08-03: an
// unparseable value is rejected with INVALID_FORCE_INSTALL_LOCAL_DATE_TIME
// ("Please provide a valid local date/time in YYYY-mm-DDTHH:MM:SS format"), and
// that check runs before the target group is resolved, so it is a pure
// config-shape rule worth catching at plan time. Deliberately shape-only — the
// calendar validity of the date is left to the server.
var forceInstallLocalDateTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`)

// specificVersionRequiredValidator enforces the one cross-field rule the server enforces on
// the wire (wire-probed 2026-06-13): version_type SPECIFIC_VERSION or CUSTOM_VERSION requires
// specific_version (the API returns 400 INVALID_SPECIFIC_VERSION otherwise). The UI-implied
// couplings for max_deferrals/force_install_local_date_time are NOT server-enforced, so they
// are deliberately not validated here — the server accepts or ignores them.
type specificVersionRequiredValidator struct{}

func (v specificVersionRequiredValidator) Description(_ context.Context) string {
	return "specific_version is required when version_type is SPECIFIC_VERSION or CUSTOM_VERSION"
}

func (v specificVersionRequiredValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v specificVersionRequiredValidator) ValidateAction(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var versionType types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("version_type"), &versionType)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Defer when the discriminator is unknown/null — the server (and the OneOf validator)
	// settle it.
	if versionType.IsNull() || versionType.IsUnknown() {
		return
	}
	vt := versionType.ValueString()
	if vt != pro.PlanConfigurationPostVersionTypeSpecificVersion && vt != pro.PlanConfigurationPostVersionTypeCustomVersion {
		return
	}

	var specificVersion types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("specific_version"), &specificVersion)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if specificVersion.IsUnknown() {
		return
	}
	if specificVersion.IsNull() || specificVersion.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("specific_version"),
			"Missing specific_version",
			"specific_version must be set when version_type is SPECIFIC_VERSION or CUSTOM_VERSION.",
		)
	}
}

// buildVersionCustomOnlyValidator enforces the second cross-field rule the server
// enforces on the wire (wire-probed 2026-08-03): build_version may only be supplied
// when version_type is CUSTOM_VERSION. Any other version_type — including
// SPECIFIC_VERSION, which the field's own name suggests it pairs with — returns
// 400 INVALID_BUILD_VERSION ("buildVersion cannot be specified when versionType is
// anything other than CUSTOM_VERSION"). Like the specific_version rule, this fires
// before the target group is resolved, so plan time is the right place to catch it.
type buildVersionCustomOnlyValidator struct{}

func (v buildVersionCustomOnlyValidator) Description(_ context.Context) string {
	return "build_version may only be set when version_type is CUSTOM_VERSION"
}

func (v buildVersionCustomOnlyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v buildVersionCustomOnlyValidator) ValidateAction(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var buildVersion types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("build_version"), &buildVersion)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// This is a forbidden-when rule, so it fires on presence: an absent or
	// not-yet-resolved build_version has nothing to forbid.
	if buildVersion.IsNull() || buildVersion.IsUnknown() {
		return
	}

	var versionType types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("version_type"), &versionType)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Defer when the discriminator cannot be read yet — the server settles it.
	if versionType.IsNull() || versionType.IsUnknown() {
		return
	}
	if versionType.ValueString() == pro.PlanConfigurationPostVersionTypeCustomVersion {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("build_version"),
		"Unsupported build_version",
		"build_version may only be set when version_type is CUSTOM_VERSION; got version_type = "+versionType.ValueString()+".",
	)
}
