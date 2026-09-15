// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// strings returns a pointer to a string slice, as the SDK's optional address lists
// take.
func stringsPtr(values ...string) *[]string {
	out := values
	return &out
}

// TestAssignHostnameMappingsResourceModel_CollapsesEmptyToNull is the round-trip that
// keeps a configuration from diffing against itself.
//
// An omitted address list reads back as `[]`, so writing an empty set into state would
// leave a config that omits ipv4_addresses permanently at odds with its own refresh.
// The schema refuses an explicitly empty collection, which is what makes collapsing
// empty to null lossless rather than a guess.
func TestAssignHostnameMappingsResourceModel_CollapsesEmptyToNull(t *testing.T) {
	cases := map[string]struct {
		aRecords    *[]string
		aaaaRecords *[]string
		wantV4Null  bool
		wantV6Null  bool
	}{
		"ipv4 only, ipv6 empty": {stringsPtr("10.0.0.1"), stringsPtr(), false, true},
		"ipv4 only, ipv6 nil":   {stringsPtr("10.0.0.1"), nil, false, true},
		"ipv6 only, ipv4 empty": {stringsPtr(), stringsPtr("2001:db8::1"), true, false},
		"both populated":        {stringsPtr("10.0.0.1"), stringsPtr("2001:db8::1"), false, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := &HostnameMappingsResourceModel{}
			diags := assignHostnameMappingsResourceModel(context.Background(), model, &securitycloud.MappingList{
				TotalCount: 1,
				Results: []securitycloud.Mapping{{
					Hostname:    "a.example.com",
					ARecords:    tc.aRecords,
					AaaaRecords: tc.aaaaRecords,
				}},
			})
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}

			var mappings []MappingModel
			if d := model.Mappings.ElementsAs(context.Background(), &mappings, false); d.HasError() {
				t.Fatalf("decoding mappings: %v", d)
			}
			if len(mappings) != 1 {
				t.Fatalf("expected 1 mapping, got %d", len(mappings))
			}
			if got := mappings[0].IPv4Addresses.IsNull(); got != tc.wantV4Null {
				t.Errorf("ipv4_addresses null = %t, want %t", got, tc.wantV4Null)
			}
			if got := mappings[0].IPv6Addresses.IsNull(); got != tc.wantV6Null {
				t.Errorf("ipv6_addresses null = %t, want %t", got, tc.wantV6Null)
			}
		})
	}
}

// TestAssignHostnameMappingsResourceModel_BooleansDefaultToFalse pins that an omitted
// wire flag reads back as false, not null. Both attributes are Required, so a null
// here would be an invalid state value rather than merely an odd one.
//
// The explicitly-false rows are what make this test about the value rather than about
// the pointer. With only nil and pointer-to-true rows, an assigner that reported
// presence — `value != nil` — agrees with one that dereferences on every input, so a
// flag the tenant has switched off would read back as on with nothing failing.
func TestAssignHostnameMappingsResourceModel_BooleansDefaultToFalse(t *testing.T) {
	yes := true
	no := false
	cases := map[string]struct {
		ztna      *bool
		secureDNS *bool
		wantZTNA  bool
		wantDNS   bool
	}{
		"both absent":                 {nil, nil, false, false},
		"ztna set":                    {&yes, nil, true, false},
		"secure dns":                  {nil, &yes, false, true},
		"ztna explicitly false":       {&no, nil, false, false},
		"secure dns explicitly false": {nil, &no, false, false},
		"both explicitly false":       {&no, &no, false, false},
		"ztna false, secure dns true": {&no, &yes, false, true},
		"ztna true, secure dns false": {&yes, &no, true, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := &HostnameMappingsResourceModel{}
			diags := assignHostnameMappingsResourceModel(context.Background(), model, &securitycloud.MappingList{
				Results: []securitycloud.Mapping{{
					Hostname:  "a.example.com",
					ARecords:  stringsPtr("10.0.0.1"),
					Ztna:      tc.ztna,
					SecureDns: tc.secureDNS,
				}},
			})
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}

			var mappings []MappingModel
			if d := model.Mappings.ElementsAs(context.Background(), &mappings, false); d.HasError() {
				t.Fatalf("decoding mappings: %v", d)
			}
			if mappings[0].ConnectToZTNA.IsNull() || mappings[0].ConnectToSecureDNS.IsNull() {
				t.Fatal("neither boolean may be null; both are Required in the schema")
			}
			if got := mappings[0].ConnectToZTNA.ValueBool(); got != tc.wantZTNA {
				t.Errorf("connect_to_ztna = %t, want %t", got, tc.wantZTNA)
			}
			if got := mappings[0].ConnectToSecureDNS.ValueBool(); got != tc.wantDNS {
				t.Errorf("connect_to_secure_dns = %t, want %t", got, tc.wantDNS)
			}
		})
	}
}

// TestAssignHostnameMappingsResourceModel_LeavesIDAlone pins the division of labour:
// the CRUD handler stamps helpers.SingletonID.
func TestAssignHostnameMappingsResourceModel_LeavesIDAlone(t *testing.T) {
	model := &HostnameMappingsResourceModel{ID: types.StringValue("pre-existing")}
	diags := assignHostnameMappingsResourceModel(context.Background(), model, &securitycloud.MappingList{
		Results: []securitycloud.Mapping{{Hostname: "a.example.com", ARecords: stringsPtr("10.0.0.1")}},
	})
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if model.ID.ValueString() != "pre-existing" {
		t.Errorf("assigner clobbered ID: got %q", model.ID.ValueString())
	}
}

// TestAssignHostnameMappingsDataSourceModel_KeepsEmptyAsEmpty is the deliberate
// difference from the resource: a data source has no configuration to diff against, so
// an empty collection is easier to consume than a null one.
func TestAssignHostnameMappingsDataSourceModel_KeepsEmptyAsEmpty(t *testing.T) {
	model := &HostnameMappingsDataSourceModel{}
	diags := assignHostnameMappingsDataSourceModel(context.Background(), model, &securitycloud.MappingList{
		Results: []securitycloud.Mapping{{
			Hostname:    "a.example.com",
			ARecords:    stringsPtr("10.0.0.1"),
			AaaaRecords: nil,
		}},
	})
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if model.Mappings[0].IPv6Addresses.IsNull() {
		t.Error("data source ipv6_addresses must be an empty list, not null")
	}
	if len(model.Mappings[0].IPv6Addresses.Elements()) != 0 {
		t.Error("data source ipv6_addresses must be empty when the wire returns none")
	}
}

// TestAssignHostnameMappingsResourceModel_EmptyResults keeps the empty case explicit:
// Read treats it as deleted, so the assigner must still produce a well-formed empty
// set rather than a null one that would fail the Required schema.
func TestAssignHostnameMappingsResourceModel_EmptyResults(t *testing.T) {
	model := &HostnameMappingsResourceModel{}
	diags := assignHostnameMappingsResourceModel(context.Background(), model, &securitycloud.MappingList{})
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if model.Mappings.IsNull() {
		t.Error("mappings must be an empty set, not null")
	}
	if len(model.Mappings.Elements()) != 0 {
		t.Error("mappings must be empty")
	}
}
