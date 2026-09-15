// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// impactFakeClient builds a non-network jamfplatform.Client for the impact
// wiring tests. No HTTP call is made: EnableImpactAlerts only constructs the
// cache, and the cache reads the tenant lazily.
func impactFakeClient() *jamfplatform.Client {
	return jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret")
}

func TestConfigureImpact_EnabledAfterEnableImpactAlerts(t *testing.T) {
	pd := New(impactFakeClient())
	pd.EnableImpactAlerts()

	c := ConfigureImpact(pd)
	if c == nil {
		t.Fatal("ConfigureImpact must return the cache once impact alerts are enabled")
	}
	if !c.Enabled() {
		t.Fatal("the cache handed to resources must report as enabled")
	}
	if pd.ImpactCache() != c {
		t.Fatal("ConfigureImpact and ImpactCache must hand out the same shared cache")
	}
}

func TestConfigureImpact_NilWithoutEnable(t *testing.T) {
	// impact_alerts unset is the default: the cache stays nil, and a nil cache is
	// how resources see "disabled" without a flag check of their own.
	pd := New(impactFakeClient())
	if c := ConfigureImpact(pd); c != nil {
		t.Fatalf("impact alerts were never enabled, got cache %v", c)
	}
	if pd.ImpactCache() != nil {
		t.Fatal("ImpactCache must be nil until EnableImpactAlerts is called")
	}
}

func TestConfigureImpact_NilProviderData(t *testing.T) {
	// The framework calls Configure with nil ProviderData during early lifecycle;
	// impact wiring must treat that as "off", never as an error or a panic.
	if c := ConfigureImpact(nil); c != nil {
		t.Fatalf("nil providerData must yield a nil cache, got %v", c)
	}
}

func TestConfigureImpact_WrongProviderDataType(t *testing.T) {
	// Impact reporting is advisory: an unexpected providerData type must degrade
	// to disabled rather than produce diagnostics or panic.
	if c := ConfigureImpact("not a *Data"); c != nil {
		t.Fatalf("a non-*Data providerData must yield a nil cache, got %v", c)
	}
}
