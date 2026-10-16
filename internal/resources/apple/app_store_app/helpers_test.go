// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/appstore"
)

func model(target string) *AppStoreAppDataSourceModel {
	return &AppStoreAppDataSourceModel{
		ID:            types.StringNull(),
		BundleID:      types.StringNull(),
		StoreURL:      types.StringNull(),
		Target:        types.StringValue(target),
		CountryRegion: types.StringNull(),
		Language:      types.StringNull(),
	}
}

func TestBuildQuery(t *testing.T) {
	cases := map[string]struct {
		set  func(*AppStoreAppDataSourceModel)
		want appstore.Query
	}{
		"id defaults to US": {
			func(m *AppStoreAppDataSourceModel) { m.ID = types.StringValue("497799835") },
			appstore.Query{ID: "497799835", Platform: appstore.PlatformMac, Country: "US"},
		},
		"bundle id, explicit country and language": {
			func(m *AppStoreAppDataSourceModel) {
				m.BundleID = types.StringValue("com.apple.Pages")
				m.CountryRegion = types.StringValue("DE")
				m.Language = types.StringValue("en")
			},
			appstore.Query{BundleID: "com.apple.Pages", Platform: appstore.PlatformMac, Country: "DE", Language: "en"},
		},
		"store url supplies country": {
			func(m *AppStoreAppDataSourceModel) {
				m.StoreURL = types.StringValue("https://apps.apple.com/gb/app/pages-create-documents/id361309726")
			},
			appstore.Query{ID: "361309726", Platform: appstore.PlatformMac, Country: "GB"},
		},
		"store url without country falls back to US": {
			func(m *AppStoreAppDataSourceModel) {
				m.StoreURL = types.StringValue("https://apps.apple.com/app/id361309726")
			},
			appstore.Query{ID: "361309726", Platform: appstore.PlatformMac, Country: "US"},
		},
		"store url and matching country": {
			func(m *AppStoreAppDataSourceModel) {
				m.StoreURL = types.StringValue("https://apps.apple.com/gb/app/x/id1")
				m.CountryRegion = types.StringValue("GB")
			},
			appstore.Query{ID: "1", Platform: appstore.PlatformMac, Country: "GB"},
		},
	}
	for name, tc := range cases {
		m := model(targetMac)
		tc.set(m)
		got, diags := buildQuery(m)
		if diags.HasError() {
			t.Errorf("%s: %v", name, diags)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestBuildQuery_MobileDeviceTarget(t *testing.T) {
	m := model(targetMobileDevice)
	m.ID = types.StringValue("361309726")
	got, diags := buildQuery(m)
	if diags.HasError() || got.Platform != appstore.PlatformMobile {
		t.Errorf("got %+v, %v", got, diags)
	}
}

func TestBuildQuery_Errors(t *testing.T) {
	conflict := model(targetMac)
	conflict.StoreURL = types.StringValue("https://apps.apple.com/gb/app/x/id1")
	conflict.CountryRegion = types.StringValue("US")
	_, diags := buildQuery(conflict)
	if !diags.HasError() || !strings.Contains(diags[0].Detail(), "store_url is a GB App Store link") {
		t.Errorf("conflict: %v", diags)
	}

	badTarget := model("ios")
	badTarget.ID = types.StringValue("1")
	if _, diags := buildQuery(badTarget); !diags.HasError() {
		t.Error("unknown target accepted")
	}

	none := model(targetMac)
	if _, diags := buildQuery(none); !diags.HasError() {
		t.Error("no selector accepted")
	}

	badURL := model(targetMac)
	badURL.StoreURL = types.StringValue("https://example.com/id1")
	if _, diags := buildQuery(badURL); !diags.HasError() {
		t.Error("bad store url accepted")
	}
}

func TestLookupErrorDiagnostic(t *testing.T) {
	q := appstore.Query{ID: "409201541", Platform: appstore.PlatformMac, Country: "US"}

	nf := lookupErrorDiagnostic(q, targetMac, &appstore.NotFoundError{Query: q})
	if nf.Summary() != "App not found on the App Store" {
		t.Errorf("not-found summary = %q", nf.Summary())
	}
	for _, want := range []string{"id 409201541", "US App Store", "no Mac version", "merged some Mac apps", "custom app"} {
		if !strings.Contains(nf.Detail(), want) {
			t.Errorf("not-found detail lacks %q:\n%s", want, nf.Detail())
		}
	}

	mobile := lookupErrorDiagnostic(appstore.Query{BundleID: "com.example", Country: "GB"}, targetMobileDevice, &appstore.NotFoundError{})
	if !strings.Contains(mobile.Detail(), `bundle_id "com.example"`) || !strings.Contains(mobile.Detail(), "Mac only") || strings.Contains(mobile.Detail(), "merged") {
		t.Errorf("mobile not-found detail:\n%s", mobile.Detail())
	}

	mm := lookupErrorDiagnostic(q, targetMac, &appstore.StorefrontMismatchError{Requested: "XX", Answered: "US"})
	type pathed interface{ Path() path.Path }
	if p, ok := mm.(pathed); !ok || !p.Path().Equal(path.Root("country_region")) {
		t.Errorf("mismatch diagnostic is not attached to country_region: %T", mm)
	}
	if !strings.Contains(mm.Detail(), `"XX"`) {
		t.Errorf("mismatch detail = %q", mm.Detail())
	}

	other := lookupErrorDiagnostic(q, targetMac, errors.New("dial tcp: timeout"))
	if other.Summary() != "Unable to read the App Store listing" || !strings.Contains(other.Detail(), "dial tcp") {
		t.Errorf("transport error diagnostic = %q / %q", other.Summary(), other.Detail())
	}
}

func TestBundleIDCaseDiagnostic(t *testing.T) {
	if d := bundleIDCaseDiagnostic("com.apple.Pages", "com.apple.Pages"); d.HasError() {
		t.Errorf("exact match: %v", d)
	}
	if d := bundleIDCaseDiagnostic("", "com.apple.Pages"); d.HasError() {
		t.Errorf("unset: %v", d)
	}
	d := bundleIDCaseDiagnostic("COM.APPLE.PAGES", "com.apple.Pages")
	if !d.HasError() || !strings.Contains(d[0].Detail(), `bundle_id = "com.apple.Pages"`) {
		t.Errorf("case variant: %v", d)
	}
}

func TestTargetValues(t *testing.T) {
	got := targetValues()
	if len(got) != 2 || got[0] != targetMac || got[1] != targetMobileDevice {
		t.Errorf("targetValues = %v", got)
	}
	for _, label := range got {
		if targetPlatforms[label] == "" {
			t.Errorf("%s maps to no platform", label)
		}
	}
	seen := map[appstore.Platform]bool{}
	for _, p := range targetPlatforms {
		seen[p] = true
	}
	for _, p := range appstore.PlatformValues() {
		if !seen[p] {
			t.Errorf("SDK platform %q has no target", p)
		}
	}
}
