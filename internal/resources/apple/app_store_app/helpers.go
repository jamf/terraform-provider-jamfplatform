// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/jamf/jamfplatform-go-sdk/appstore"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildQuery turns the configuration into the SDK query, resolving the storefront from
// country_region, then the store URL, then appstore.DefaultCountry. A store URL and a
// country_region naming different storefronts is an error on country_region, because silently
// preferring either would look up a listing the practitioner did not ask for.
func buildQuery(m *AppStoreAppDataSourceModel) (appstore.Query, diag.Diagnostics) {
	var diags diag.Diagnostics
	q := appstore.Query{
		Platform: targetPlatforms[m.Target.ValueString()],
		Language: m.Language.ValueString(),
	}
	if q.Platform == "" {
		diags.AddAttributeError(path.Root("target"), "Unsupported target",
			fmt.Sprintf("target must be one of %s, got %q.", strings.Join(targetValues(), ", "), m.Target.ValueString()))
		return q, diags
	}

	var urlCountry string
	switch {
	case !m.ID.IsNull() && m.ID.ValueString() != "":
		q.ID = m.ID.ValueString()
	case !m.BundleID.IsNull() && m.BundleID.ValueString() != "":
		q.BundleID = m.BundleID.ValueString()
	case !m.StoreURL.IsNull() && m.StoreURL.ValueString() != "":
		parsed, err := appstore.ParseStoreURL(m.StoreURL.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("store_url"), "Invalid App Store link", storeURLDetail(m.StoreURL.ValueString()))
			return q, diags
		}
		q.ID = parsed.ID
		urlCountry = parsed.Country
	default:
		diags.AddError("Missing app selector", "Exactly one of id, bundle_id or store_url must be set.")
		return q, diags
	}

	configured := m.CountryRegion.ValueString()
	switch {
	case configured != "" && urlCountry != "" && configured != urlCountry:
		diags.AddAttributeError(path.Root("country_region"), "Country or region conflicts with the App Store link",
			fmt.Sprintf("country_region is %q but store_url is a %s App Store link. Remove country_region to use the link's, or use a link for %s.", configured, urlCountry, configured))
		return q, diags
	case configured != "":
		q.Country = configured
	case urlCountry != "":
		q.Country = urlCountry
	default:
		q.Country = appstore.DefaultCountry
	}
	return q, diags
}

// storeURLDetail is the detail for a store link the provider cannot read an app id from.
func storeURLDetail(raw string) string {
	return fmt.Sprintf("%q is not an App Store app link. Use the link from the app's App Store page, such as https://apps.apple.com/us/app/xcode/id497799835. It must be on apps.apple.com and end in id followed by the app's number.", raw)
}

// lookupErrorDiagnostic translates a lookup failure into a diagnostic that names its causes.
func lookupErrorDiagnostic(q appstore.Query, target string, err error) diag.Diagnostic {
	var notFound *appstore.NotFoundError
	var mismatch *appstore.StorefrontMismatchError
	switch {
	case errors.As(err, &notFound):
		return diag.NewErrorDiagnostic("App not found on the App Store", notFoundDetail(q, target))
	case errors.As(err, &mismatch):
		return diag.NewAttributeErrorDiagnostic(path.Root("country_region"), "Unrecognised App Store country or region",
			fmt.Sprintf("The App Store does not recognise %q and answered from the %s App Store instead. Use a two-letter code the App Store sells in, such as US, GB or DE.", mismatch.Requested, mismatch.Answered))
	default:
		return diag.NewErrorDiagnostic("Unable to read the App Store listing", helpers.APIErrorDetail(err))
	}
}

// notFoundDetail lists every reason the App Store answers a lookup with no listing.
func notFoundDetail(q appstore.Query, target string) string {
	key := fmt.Sprintf("id %s", q.ID)
	if q.ID == "" {
		key = fmt.Sprintf("bundle_id %q", q.BundleID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The App Store has no %s app with %s in the %s App Store. The usual causes:\n\n", target, key, q.Country)
	b.WriteString("- The id or bundle_id is wrong.\n")
	fmt.Fprintf(&b, "- The app is not sold in the %s App Store. Set country_region to a country or region that sells it.\n", q.Country)
	if target == targetMac {
		b.WriteString("- The app has no Mac version. An iPhone and iPad app is not found with target = \"mac\".\n")
		b.WriteString("- Apple has merged some Mac apps into their iPhone and iPad listing, which retires the old Mac id. Look the app up by bundle_id, or use the id in its current App Store link.\n")
	} else {
		b.WriteString("- The app is Mac only. Use target = \"mac\".\n")
	}
	b.WriteString("- The app is a custom app distributed to one organization. Custom apps are not listed publicly, so they cannot be looked up.")
	return b.String()
}

// bundleIDCaseDiagnostic reports a configured bundle_id that differs from the listing's, which the
// App Store tolerates in letter case and Jamf Pro does not: it stores a bundle identifier verbatim.
func bundleIDCaseDiagnostic(configured, listed string) diag.Diagnostics {
	var diags diag.Diagnostics
	if configured == "" || configured == listed {
		return diags
	}
	diags.AddAttributeError(path.Root("bundle_id"), "Bundle identifier spelling differs from the App Store",
		fmt.Sprintf("The App Store lists this app as %q, and bundle_id is %q. Jamf Pro stores a bundle identifier exactly as written, so set bundle_id = %q.", listed, configured, listed))
	return diags
}
