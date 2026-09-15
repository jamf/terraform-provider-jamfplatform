// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestNoTrailingRootDot pins the one grammar rule this endpoint does not share with
// its sibling. Wire-probed 2026-08-29: the custom hostname mappings endpoint stores
// `Trail.Example.COM.` as `Trail.Example.COM`, while the search domain endpoint
// stores the dot verbatim. The shared commonvalidators.DNSHostname deliberately
// accepts the dot, so without this validator a dot-terminated host name reaches the
// write and the read-back cannot match the plan.
func TestNoTrailingRootDot(t *testing.T) {
	for name, tc := range map[string]struct {
		value   types.String
		wantErr bool
	}{
		"plain host name":              {types.StringValue("corp.example.com"), false},
		"single label":                 {types.StringValue("corp"), false},
		"interior dots only":           {types.StringValue("a.b.c.example.com"), false},
		"trailing root dot":            {types.StringValue("corp.example.com."), true},
		"single label trailing dot":    {types.StringValue("corp."), true},
		"null defers to the server":    {types.StringNull(), false},
		"unknown defers to the server": {types.StringUnknown(), false},
	} {
		t.Run(name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			NoTrailingRootDot().ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("mappings"),
				ConfigValue: tc.value,
			}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %t, want %t (diags: %v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestNoTrailingRootDotExplainsWhy keeps the diagnostic pointed at the cause. The
// server's own refusal for a malformed name is an unattributed 400, and this rule is
// not a server refusal at all — a dot-terminated name is accepted and silently
// rewritten — so a message that only said "invalid" would leave the operator with no
// way to work out why their apply failed.
func TestNoTrailingRootDotExplainsWhy(t *testing.T) {
	resp := &validator.StringResponse{}
	NoTrailingRootDot().ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("mappings"),
		ConfigValue: types.StringValue("corp.example.com."),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"without the trailing dot", "corp.example.com."} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q, want it to mention %q", detail, want)
		}
	}
}

// TestStoredMappingsMatchPlan pins the comparison Create's preflight uses to tell its
// own interrupted write from an administrator's mappings. It has to be
// order-insensitive on both the collection and each address list, because the server
// returns the set in an order of its own and dedupes addresses.
func TestStoredMappingsMatchPlan(t *testing.T) {
	ctx := context.Background()
	planned := mappingSet(t,
		writeMappingObject(t, "a.example.com", addressSet(t, "10.0.0.1", "10.0.0.2"), addressSet(t, "2001:db8::1"), true, false),
		writeMappingObject(t, "b.example.com", addressSet(t, "10.0.1.1"), types.SetNull(types.StringType), false, true),
	)

	yes, no := true, false
	stored := func(mappings ...securitycloud.Mapping) *securitycloud.MappingList {
		return &securitycloud.MappingList{Results: mappings, TotalCount: len(mappings)}
	}
	a := securitycloud.Mapping{
		Hostname: "a.example.com", ARecords: &[]string{"10.0.0.1", "10.0.0.2"},
		AaaaRecords: &[]string{"2001:db8::1"}, Ztna: &yes, SecureDns: &no,
	}
	aReordered := securitycloud.Mapping{
		Hostname: "a.example.com", ARecords: &[]string{"10.0.0.2", "10.0.0.1"},
		AaaaRecords: &[]string{"2001:db8::1"}, Ztna: &yes, SecureDns: &no,
	}
	b := securitycloud.Mapping{
		Hostname: "b.example.com", ARecords: &[]string{"10.0.1.1"},
		AaaaRecords: &[]string{}, Ztna: &no, SecureDns: &yes,
	}

	for name, tc := range map[string]struct {
		existing *securitycloud.MappingList
		want     bool
	}{
		"identical":                        {stored(a, b), true},
		"mappings in the server's order":   {stored(b, a), true},
		"addresses in the server's order":  {stored(aReordered, b), true},
		"nil list":                         {nil, false},
		"empty list":                       {stored(), false},
		"one mapping short":                {stored(a), false},
		"different hostname":               {stored(a, securitycloud.Mapping{Hostname: "c.example.com", ARecords: &[]string{"10.0.1.1"}, AaaaRecords: &[]string{}, Ztna: &no, SecureDns: &yes}), false},
		"different address":                {stored(a, securitycloud.Mapping{Hostname: "b.example.com", ARecords: &[]string{"10.0.1.9"}, AaaaRecords: &[]string{}, Ztna: &no, SecureDns: &yes}), false},
		"different traffic vectoring flag": {stored(a, securitycloud.Mapping{Hostname: "b.example.com", ARecords: &[]string{"10.0.1.1"}, AaaaRecords: &[]string{}, Ztna: &yes, SecureDns: &yes}), false},
	} {
		t.Run(name, func(t *testing.T) {
			got, diags := storedMappingsMatchPlan(ctx, planned, tc.existing)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got != tc.want {
				t.Fatalf("storedMappingsMatchPlan() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestStoredMappingsMatchPlanTreatsAbsentAndEmptyAlike guards the one asymmetry the
// wire forces: an omitted address list reads back as `[]`, so a plan that omits
// ipv6_addresses must still match a stored mapping carrying an empty aaaaRecords.
// Without this the preflight would refuse to adopt its own interrupted write on every
// mapping that sets only one address family.
func TestStoredMappingsMatchPlanTreatsAbsentAndEmptyAlike(t *testing.T) {
	ctx := context.Background()
	planned := mappingSet(t,
		writeMappingObject(t, "only4.example.com", addressSet(t, "10.0.0.1"), types.SetNull(types.StringType), false, false),
	)
	no := false
	for name, aaaa := range map[string]*[]string{
		"empty slice": {},
		"nil pointer": nil,
	} {
		t.Run(name, func(t *testing.T) {
			existing := &securitycloud.MappingList{
				Results: []securitycloud.Mapping{{
					Hostname: "only4.example.com", ARecords: &[]string{"10.0.0.1"},
					AaaaRecords: aaaa, Ztna: &no, SecureDns: &no,
				}},
				TotalCount: 1,
			}
			got, diags := storedMappingsMatchPlan(ctx, planned, existing)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !got {
				t.Fatal("storedMappingsMatchPlan() = false, want true — absent and empty are the same on this endpoint")
			}
		})
	}
}
