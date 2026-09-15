// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// serverZone is the shape a read returns, with the domain list in the sorted
// order Jamf Security Cloud stores rather than the order it was authored in.
func serverZone() *securitycloud.Zone {
	return &securitycloud.Zone{
		ID:      "f5734162-26d4-4d0f-a3ae-62f952b3688f",
		Name:    "Test Zone",
		Domains: []string{"*.testdomain.com", "testdomain.com"},
		NameServers: []securitycloud.NameServer{
			{IP: "203.0.113.53", GatewayID: "a7d2"},
			{IP: "198.51.100.53", GatewayID: "1cbb"},
		},
	}
}

func TestAssignDNSZoneResourceModel_PopulatesEveryField(t *testing.T) {
	var state DNSZoneResourceModel
	diags := assignDNSZoneResourceModel(context.Background(), &state, serverZone())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "f5734162-26d4-4d0f-a3ae-62f952b3688f" {
		t.Errorf("ID = %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Test Zone" {
		t.Errorf("Name = %q", state.Name.ValueString())
	}
	if len(state.Domains.Elements()) != 2 {
		t.Errorf("Domains = %v, want 2 entries", state.Domains.Elements())
	}
	if len(state.NameServers.Elements()) != 2 {
		t.Errorf("NameServers = %v, want 2 entries", state.NameServers.Elements())
	}
}

// TestAssignDNSZoneResourceModel_PreservesIDWhenResponseOmitsIt guards the
// post-create read: Create sets ID from the create response, and a read that came
// back without one must not blank it.
func TestAssignDNSZoneResourceModel_PreservesIDWhenResponseOmitsIt(t *testing.T) {
	state := DNSZoneResourceModel{ID: types.StringValue("kept-id")}
	zone := serverZone()
	zone.ID = ""

	diags := assignDNSZoneResourceModel(context.Background(), &state, zone)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "kept-id" {
		t.Errorf("ID = %q, want the value already in state", state.ID.ValueString())
	}
}

func TestAssignDNSZoneDataSourceModel_UsesLists(t *testing.T) {
	var state DNSZoneDataSourceModel
	diags := assignDNSZoneDataSourceModel(context.Background(), &state, serverZone())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.Domains.IsNull() || len(state.Domains.Elements()) != 2 {
		t.Errorf("Domains = %v, want a 2-element list", state.Domains)
	}
	if state.NameServers.IsNull() || len(state.NameServers.Elements()) != 2 {
		t.Errorf("NameServers = %v, want a 2-element list", state.NameServers)
	}
}

// TestAssignDNSZoneDataSourceModel_PreservesServerOrder pins the data source
// contract: the read-only lists echo the server's order verbatim, including the
// sort Jamf Security Cloud applies to `domains`.
func TestAssignDNSZoneDataSourceModel_PreservesServerOrder(t *testing.T) {
	var state DNSZoneDataSourceModel
	diags := assignDNSZoneDataSourceModel(context.Background(), &state, serverZone())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	var domains []string
	if convDiags := state.Domains.ElementsAs(context.Background(), &domains, false); convDiags.HasError() {
		t.Fatalf("converting domains: %v", convDiags)
	}
	want := []string{"*.testdomain.com", "testdomain.com"}
	for i := range want {
		if domains[i] != want[i] {
			t.Errorf("domains = %v, want %v", domains, want)
			break
		}
	}
}

func TestBuildDNSZonesResultModel_PopulatesEveryField(t *testing.T) {
	got, diags := buildDNSZonesResultModel(context.Background(), *serverZone())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if got.ID.ValueString() == "" || got.Name.ValueString() == "" {
		t.Errorf("result model lost an identity field: %+v", got)
	}
	if len(got.Domains.Elements()) != 2 || len(got.NameServers.Elements()) != 2 {
		t.Errorf("result model lost a collection: %+v", got)
	}
}

func TestNameServerSetValue_EmptyServersYieldsEmptySet(t *testing.T) {
	set, diags := nameServerSetValue(nil)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if set.IsNull() {
		t.Error("an empty name server slice must produce an empty set, not a null one")
	}
	if len(set.Elements()) != 0 {
		t.Errorf("set = %v, want no elements", set.Elements())
	}
}
