// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_settings

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// fakeLister implements appStoreLocaleLister for unit testing without a live client.
type fakeLister struct {
	codes []pro.Country
	err   error
}

func (f fakeLister) ListAppStoreCountryCodesV1(ctx context.Context) (*pro.CountryCodes, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pro.CountryCodes{CountryCodes: f.codes}, nil
}

func usLister() fakeLister {
	return fakeLister{codes: []pro.Country{{Code: "US", Name: "United States"}, {Code: "GB", Name: "United Kingdom"}}}
}

func TestValidateAppStoreLocale(t *testing.T) {
	p := path.Root("app_store_locale")

	t.Run("nil lister is no-op", func(t *testing.T) {
		if d := validateAppStoreLocale(context.Background(), nil, types.StringValue("ZZ"), p); d.HasError() {
			t.Errorf("nil lister must not error: %v", d)
		}
	})
	t.Run("null/unknown deferred", func(t *testing.T) {
		if d := validateAppStoreLocale(context.Background(), usLister(), types.StringNull(), p); d.HasError() {
			t.Errorf("null must defer: %v", d)
		}
		if d := validateAppStoreLocale(context.Background(), usLister(), types.StringUnknown(), p); d.HasError() {
			t.Errorf("unknown must defer: %v", d)
		}
	})
	t.Run("exact deviceLocale sentinel allowed", func(t *testing.T) {
		if d := validateAppStoreLocale(context.Background(), usLister(), types.StringValue("deviceLocale"), p); d.HasError() {
			t.Errorf("deviceLocale must be allowed: %v", d)
		}
	})
	t.Run("non-canonical sentinel case errors", func(t *testing.T) {
		for _, v := range []string{"devicelocale", "DEVICELOCALE"} {
			if d := validateAppStoreLocale(context.Background(), usLister(), types.StringValue(v), p); !d.HasError() {
				t.Errorf("%q must error (must be exactly \"deviceLocale\")", v)
			}
		}
	})
	t.Run("valid canonical (upper-case) code allowed", func(t *testing.T) {
		for _, v := range []string{"US", "GB"} {
			if d := validateAppStoreLocale(context.Background(), usLister(), types.StringValue(v), p); d.HasError() {
				t.Errorf("%q must be valid: %v", v, d)
			}
		}
	})
	t.Run("non-canonical case errors", func(t *testing.T) {
		// Optional+Computed forbids the provider rewriting a user value, so a lowercase /
		// mixed-case code must be rejected with the canonical form rather than silently
		// canonicalised.
		for _, v := range []string{"us", "Us", "gb"} {
			if d := validateAppStoreLocale(context.Background(), usLister(), types.StringValue(v), p); !d.HasError() {
				t.Errorf("%q must error (non-canonical)", v)
			}
		}
	})
	t.Run("invalid code errors", func(t *testing.T) {
		if d := validateAppStoreLocale(context.Background(), usLister(), types.StringValue("ZZ"), p); !d.HasError() {
			t.Errorf("ZZ must error")
		}
	})
	t.Run("empty string errors (not a valid locale)", func(t *testing.T) {
		if d := validateAppStoreLocale(context.Background(), usLister(), types.StringValue(""), p); !d.HasError() {
			t.Errorf("empty string must error")
		}
	})
	t.Run("fetch error downgrades to warning", func(t *testing.T) {
		d := validateAppStoreLocale(context.Background(), fakeLister{err: errors.New("boom")}, types.StringValue("US"), p)
		if d.HasError() {
			t.Errorf("fetch error must not be an error diagnostic: %v", d)
		}
		if d.WarningsCount() != 1 {
			t.Errorf("expected 1 warning, got %d", d.WarningsCount())
		}
	})
}

func TestValidateEnabledRequiresRequesterGroup(t *testing.T) {
	p := path.Root("requester_user_group_id")

	cases := []struct {
		name      string
		enabled   types.Bool
		group     types.Int64
		wantError bool
	}{
		{"disabled", types.BoolValue(false), types.Int64Null(), false},
		{"enabled-null unknown defers", types.BoolUnknown(), types.Int64Null(), false},
		{"enabled true, group unknown defers", types.BoolValue(true), types.Int64Unknown(), false},
		{"enabled true, group null errors", types.BoolValue(true), types.Int64Null(), true},
		{"enabled true, group set ok", types.BoolValue(true), types.Int64Value(3), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := validateEnabledRequiresRequesterGroup(c.enabled, c.group, p)
			if d.HasError() != c.wantError {
				t.Errorf("got error=%v want=%v (%v)", d.HasError(), c.wantError, d)
			}
		})
	}
}

func TestValidateRequesterRequiresEnabled(t *testing.T) {
	p := path.Root("requester_user_group_id")

	cases := []struct {
		name      string
		enabled   types.Bool
		group     types.Int64
		wantError bool
	}{
		{"no group configured", types.BoolValue(false), types.Int64Null(), false},
		{"group set + enabled true ok", types.BoolValue(true), types.Int64Value(3), false},
		{"group set + enabled false errors", types.BoolValue(false), types.Int64Value(3), true},
		{"group set + enabled unknown defers", types.BoolUnknown(), types.Int64Value(3), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := validateRequesterRequiresEnabled(c.enabled, c.group, p)
			if d.HasError() != c.wantError {
				t.Errorf("got error=%v want=%v (%v)", d.HasError(), c.wantError, d)
			}
		})
	}
}
