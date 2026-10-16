// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/appstore"
)

// pagesMac is the Mac flavour of Pages as decoded from the 2026-10-16 capture, trimmed.
func pagesMac() *appstore.App {
	return &appstore.App{
		ID:                     "361309726",
		BundleID:               "com.apple.Pages",
		Name:                   "Pages: Create Documents",
		Version:                "15.4",
		URL:                    "https://apps.apple.com/us/app/pages-create-documents/id361309726",
		Description:            "Create beautiful documents.",
		ArtworkURLTemplate:     "https://is1-ssl.mzstatic.com/image/thumb/Purple221/v4/AppIcon.png/{w}x{h}bb.{f}",
		IconArtworkURLTemplate: "https://is1-ssl.mzstatic.com/image/thumb/PurpleSource221/v4/Placeholder.mill/{w}x{h}{c}.{f}",
		Free:                   true,
		Genres:                 []string{"Productivity", "Business"},
		DeviceBasedLicensing:   true,
		Country:                "US",
	}
}

func TestAssignApp_FillsComputedAndUnsetInputs(t *testing.T) {
	m := model(targetMac)
	m.ID = types.StringValue("361309726")
	q := appstore.Query{ID: "361309726", Platform: appstore.PlatformMac, Country: "US"}
	if diags := assignApp(context.Background(), m, q, pagesMac()); diags.HasError() {
		t.Fatalf("assignApp: %v", diags)
	}
	checks := map[string]string{
		"id":             m.ID.ValueString(),
		"bundle_id":      m.BundleID.ValueString(),
		"country_region": m.CountryRegion.ValueString(),
		"name":           m.Name.ValueString(),
		"version":        m.Version.ValueString(),
		"url":            m.URL.ValueString(),
		"description":    m.Description.ValueString(),
		"icon_url":       m.IconURL.ValueString(),
	}
	want := map[string]string{
		"id":             "361309726",
		"bundle_id":      "com.apple.Pages",
		"country_region": "US",
		"name":           "Pages: Create Documents",
		"version":        "15.4",
		"url":            "https://apps.apple.com/us/app/pages-create-documents/id361309726",
		"description":    "Create beautiful documents.",
		"icon_url":       "https://is1-ssl.mzstatic.com/image/thumb/Purple221/v4/AppIcon.png/512x512bb.png",
	}
	for k, v := range want {
		if checks[k] != v {
			t.Errorf("%s = %q, want %q", k, checks[k], v)
		}
	}
	if !m.IsFree.ValueBool() || !m.VPPDeviceBasedLicensing.ValueBool() {
		t.Errorf("is_free = %v, vpp_device_based_licensing = %v", m.IsFree, m.VPPDeviceBasedLicensing)
	}
	var genres []string
	m.Genres.ElementsAs(context.Background(), &genres, false)
	if len(genres) != 2 || genres[0] != "Productivity" {
		t.Errorf("genres = %v", genres)
	}
	if !m.StoreURL.IsNull() || !m.Language.IsNull() {
		t.Error("an unset Optional-only input was written")
	}
}

// TestAssignApp_IconIsArtworkNotIconArtwork pins the maintainer's choice: icon_url is the artwork
// image, the one Jamf Pro pulls in when it adds an App Store app, never the masked iconArtwork.
func TestAssignApp_IconIsArtworkNotIconArtwork(t *testing.T) {
	m := model(targetMobileDevice)
	m.ID = types.StringValue("361309726")
	if diags := assignApp(context.Background(), m, appstore.Query{ID: "361309726", Platform: appstore.PlatformMobile, Country: "US"}, pagesMac()); diags.HasError() {
		t.Fatalf("assignApp: %v", diags)
	}
	if strings.Contains(m.IconURL.ValueString(), "Placeholder.mill") || !strings.HasSuffix(m.IconURL.ValueString(), "/AppIcon.png/512x512bb.png") {
		t.Errorf("icon_url = %q, want the artwork template filled at 512x512bb.png", m.IconURL.ValueString())
	}

	noArtwork := pagesMac()
	noArtwork.ArtworkURLTemplate = ""
	m = model(targetMac)
	if diags := assignApp(context.Background(), m, appstore.Query{ID: "1", Country: "US"}, noArtwork); diags.HasError() {
		t.Fatalf("no artwork: %v", diags)
	}
	if !m.IconURL.IsNull() {
		t.Errorf("icon_url = %q, want null when the listing has no artwork (and no fallback to iconArtwork)", m.IconURL.ValueString())
	}
}

func TestAssignApp_KeepsConfiguredInputs(t *testing.T) {
	m := model(targetMac)
	m.StoreURL = types.StringValue("https://apps.apple.com/gb/app/x/id361309726")
	m.CountryRegion = types.StringValue("GB")
	m.Language = types.StringValue("fr")
	app := pagesMac()
	app.Country = "GB"
	if diags := assignApp(context.Background(), m, appstore.Query{ID: "361309726", Country: "GB"}, app); diags.HasError() {
		t.Fatalf("assignApp: %v", diags)
	}
	if m.CountryRegion.ValueString() != "GB" || m.Language.ValueString() != "fr" || m.StoreURL.ValueString() != "https://apps.apple.com/gb/app/x/id361309726" {
		t.Errorf("configured inputs changed: %v %v %v", m.CountryRegion, m.Language, m.StoreURL)
	}
	if m.ID.ValueString() != "361309726" {
		t.Errorf("id = %v, want filled from the listing", m.ID)
	}
}

func TestAssignApp_Errors(t *testing.T) {
	ctx := context.Background()
	q := appstore.Query{ID: "1", Country: "US"}

	caseVariant := model(targetMobileDevice)
	caseVariant.BundleID = types.StringValue("COM.APPLE.PAGES")
	if d := assignApp(ctx, caseVariant, q, pagesMac()); !d.HasError() || !strings.Contains(d[0].Detail(), "com.apple.Pages") {
		t.Errorf("bundle-id case variant: %v", d)
	}

	noVersion := pagesMac()
	noVersion.Version = ""
	if d := assignApp(ctx, model(targetMac), q, noVersion); !d.HasError() || d[0].Summary() != "App Store listing has no version" {
		t.Errorf("no version: %v", d)
	}

	badTemplate := pagesMac()
	badTemplate.ArtworkURLTemplate = "https://x/{w}x{h}{c}.{f}"
	if d := assignApp(ctx, model(targetMac), q, badTemplate); !d.HasError() {
		t.Error("an unrecognised artwork template was published")
	}

	if d := assignApp(ctx, model(targetMac), q, nil); !d.HasError() {
		t.Error("nil listing accepted")
	}
}

func TestAssignApp_NilGenresIsEmptyList(t *testing.T) {
	app := pagesMac()
	app.Genres = nil
	m := model(targetMac)
	if d := assignApp(context.Background(), m, appstore.Query{ID: "1", Country: "US"}, app); d.HasError() {
		t.Fatal(d)
	}
	if m.Genres.IsNull() || len(m.Genres.Elements()) != 0 {
		t.Errorf("genres = %v, want an empty list", m.Genres)
	}
}

func TestAssignApp_TrimsTrailingDescriptionWhitespace(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Ends in a newline.\n", "Ends in a newline."},
		{"Line one \nand timeout \n", "Line one \nand timeout"},
		{"  Leading kept.", "  Leading kept."},
		{"Clean.", "Clean."},
	} {
		m := model(targetMac)
		app := pagesMac()
		app.Description = tc.in
		if diags := assignApp(context.Background(), m, appstore.Query{ID: app.ID, Country: "US"}, app); diags.HasError() {
			t.Fatalf("assignApp: %v", diags)
		}
		if got := m.Description.ValueString(); got != tc.want {
			t.Errorf("description %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}
