// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/appstore"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignApp writes a listing into the model. Configured inputs are kept as written, because
// Terraform refuses a data source that changes a configured value; the Optional+Computed inputs
// are filled only when the configuration left them unset.
//
// The description loses its trailing whitespace. Jamf Pro drops whitespace at the end of a Self
// Service description on save, on both the Mac and the mobile device app record, so an untrimmed
// value fails the app resource with an inconsistent result after apply. Interior whitespace,
// including at the end of a line, is stored as sent and is kept. Wire-verified on 2026-10-16:
// Xcode (497799835) ends its listing in "\n" and Jamf Parent (1458797105) in " \n", and both
// came back without it.
func assignApp(ctx context.Context, m *AppStoreAppDataSourceModel, q appstore.Query, app *appstore.App) diag.Diagnostics {
	var diags diag.Diagnostics
	if app == nil {
		diags.AddError("Unable to read the App Store listing", "The lookup returned no listing and no error.")
		return diags
	}
	if !m.BundleID.IsNull() {
		diags.Append(bundleIDCaseDiagnostic(m.BundleID.ValueString(), app.BundleID)...)
		if diags.HasError() {
			return diags
		}
	}
	if app.Version == "" {
		diags.AddError("App Store listing has no version",
			fmt.Sprintf("The App Store lists %q (id %s) with no version on sale in the %s App Store, so there is nothing to set general.version to. This happens for an app on pre-order or withdrawn from sale.", app.Name, app.ID, q.Country))
		return diags
	}

	if m.ID.IsNull() || m.ID.IsUnknown() {
		m.ID = types.StringValue(app.ID)
	}
	if m.BundleID.IsNull() || m.BundleID.IsUnknown() {
		m.BundleID = types.StringValue(app.BundleID)
	}
	if m.CountryRegion.IsNull() || m.CountryRegion.IsUnknown() {
		m.CountryRegion = types.StringValue(q.Country)
	}
	m.Name = types.StringValue(app.Name)
	m.Version = types.StringValue(app.Version)
	m.URL = types.StringValue(app.URL)
	m.Description = types.StringValue(strings.TrimRightFunc(app.Description, unicode.IsSpace))
	m.IsFree = types.BoolValue(app.Free)
	m.VPPDeviceBasedLicensing = types.BoolValue(app.DeviceBasedLicensing)

	icon, err := iconURL(app)
	switch {
	case err == nil:
		m.IconURL = types.StringValue(icon)
	case errors.Is(err, appstore.ErrNoArtwork):
		m.IconURL = types.StringNull()
	default:
		diags.AddAttributeError(path.Root("icon_url"), "Unreadable App Store icon address",
			fmt.Sprintf("The App Store listing for %q carries an icon address in a form the provider does not recognise, so the provider cannot build an icon URL from it: %s", app.Name, helpers.APIErrorDetail(err)))
		return diags
	}

	genres := app.Genres
	if genres == nil {
		genres = []string{}
	}
	list, listDiags := types.ListValueFrom(ctx, types.StringType, genres)
	diags.Append(listDiags...)
	m.Genres = list
	return diags
}
