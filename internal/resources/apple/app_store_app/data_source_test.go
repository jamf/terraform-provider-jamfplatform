// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/jamfplatform-go-sdk/appstore"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// xcodeBody is a trimmed Mac listing in the shape the App Store answers with.
const xcodeBody = `{"isAuthenticated":false,"meta":{"language":{"tag":"en-us"},"storefront":{"cc":"US","id":"143441"}},
"results":{"497799835":{"id":"497799835","bundleId":"com.apple.dt.Xcode","name":"Xcode",
"url":"https://apps.apple.com/us/app/xcode/id497799835?mt=12","description":{"standard":"Xcode includes everything."},
"artwork":{"url":"https://is1-ssl.mzstatic.com/image/thumb/Purple/v4/AppIcon.png/{w}x{h}bb.{f}"},
"iconArtwork":{"url":"https://is1-ssl.mzstatic.com/image/thumb/PurpleSource/v4/Placeholder.mill/{w}x{h}{c}.{f}"},
"genreNames":["Developer Tools"],"isVppDeviceBasedLicensingEnabled":true,"kind":"desktopApp",
"offers":[{"price":0.0,"type":"get","version":{"display":"27.1"}}]}},"version":2}`

// appStoreServer answers every lookup with body and records the queries it saw.
func appStoreServer(t *testing.T, body string) (*providerdata.AppStoreCache, func() []url.Values) {
	t.Helper()
	var mu sync.Mutex
	var seen []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Query())
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return providerdata.NewAppStoreCache(appstore.New(appstore.WithEndpoint(srv.URL))), func() []url.Values {
		mu.Lock()
		defer mu.Unlock()
		return append([]url.Values(nil), seen...)
	}
}

// configValue builds a configuration object with the given string inputs and everything else null.
func configValue(t *testing.T, ds datasource.DataSource, inputs map[string]string) tfsdk.Config {
	t.Helper()
	var sresp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &sresp)
	objType, ok := sresp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	vals := map[string]tftypes.Value{}
	for name, typ := range objType.AttributeTypes {
		if v, ok := inputs[name]; ok {
			vals[name] = tftypes.NewValue(tftypes.String, v)
			continue
		}
		vals[name] = tftypes.NewValue(typ, nil)
	}
	return tfsdk.Config{Schema: sresp.Schema, Raw: tftypes.NewValue(objType, vals)}
}

// read runs the data source's Read against config and returns the response.
func read(t *testing.T, cache *providerdata.AppStoreCache, inputs map[string]string) (*datasource.ReadResponse, AppStoreAppDataSourceModel) {
	t.Helper()
	ds := &AppStoreAppDataSource{lookups: cache}
	cfg := configValue(t, ds, inputs)
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: cfg.Schema, Raw: cfg.Raw.Copy()}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: cfg}, resp)
	var got AppStoreAppDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	}
	return resp, got
}

func TestRead_MacByID(t *testing.T) {
	cache, seen := appStoreServer(t, xcodeBody)
	resp, got := read(t, cache, map[string]string{"id": "497799835", "target": "mac"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	if got.BundleID.ValueString() != "com.apple.dt.Xcode" || got.Version.ValueString() != "27.1" || got.CountryRegion.ValueString() != "US" {
		t.Errorf("state = %+v", got)
	}
	if got.IconURL.ValueString() != "https://is1-ssl.mzstatic.com/image/thumb/Purple/v4/AppIcon.png/512x512bb.png" {
		t.Errorf("icon_url = %q", got.IconURL.ValueString())
	}
	q := seen()
	if len(q) != 1 || q[0].Get("platform") != appstore.PlatformMac || q[0].Get("cc") != "us" || q[0].Get("id") != "497799835" {
		t.Errorf("queries = %v", q)
	}
}

func TestRead_StoreURLSuppliesStorefront(t *testing.T) {
	body := strings.ReplaceAll(xcodeBody, `"cc":"US"`, `"cc":"GB"`)
	cache, seen := appStoreServer(t, body)
	resp, got := read(t, cache, map[string]string{"store_url": "https://apps.apple.com/gb/app/xcode/id497799835?mt=12", "target": "mac"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	if got.ID.ValueString() != "497799835" || got.CountryRegion.ValueString() != "GB" {
		t.Errorf("id %q country_region %q", got.ID.ValueString(), got.CountryRegion.ValueString())
	}
	if q := seen(); q[0].Get("cc") != "gb" {
		t.Errorf("cc = %q, want gb", q[0].Get("cc"))
	}
}

func TestRead_MissIsANamedError(t *testing.T) {
	cache, _ := appStoreServer(t, `{"meta":{"storefront":{"cc":"US"}},"results":{}}`)
	resp, _ := read(t, cache, map[string]string{"id": "409201541", "target": "mac"})
	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "App not found on the App Store" {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
}

func TestRead_StorefrontFallbackIsAnError(t *testing.T) {
	cache, _ := appStoreServer(t, xcodeBody)
	resp, _ := read(t, cache, map[string]string{"id": "497799835", "target": "mac", "country_region": "XX"})
	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "Unrecognised App Store country or region" {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
}

func TestRead_BundleIDCaseVariantIsAnError(t *testing.T) {
	cache, _ := appStoreServer(t, strings.Replace(xcodeBody, `"497799835":{`, `"COM.APPLE.DT.XCODE":{`, 1))
	resp, _ := read(t, cache, map[string]string{"bundle_id": "COM.APPLE.DT.XCODE", "target": "mobile_device"})
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics[0].Detail(), `bundle_id = "com.apple.dt.Xcode"`) {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
}

func TestRead_UnconfiguredProvider(t *testing.T) {
	resp, _ := read(t, nil, map[string]string{"id": "1", "target": "mac"})
	if !resp.Diagnostics.HasError() {
		t.Fatal("Read without a configured provider reported no error")
	}
}

func TestConfigure(t *testing.T) {
	ds := &AppStoreAppDataSource{}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, &resp)
	if resp.Diagnostics.HasError() || ds.lookups != nil {
		t.Errorf("nil provider data: %v, lookups %v", resp.Diagnostics, ds.lookups)
	}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Error("wrong provider data type accepted")
	}
	pd := providerdata.New(nil)
	resp = datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: pd}, &resp)
	if resp.Diagnostics.HasError() || ds.lookups != pd.AppStoreCache() {
		t.Errorf("configure: %v", resp.Diagnostics)
	}
}
