// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

type fakeSearcher struct {
	results []pro.LdapGroup
	err     error
}

func (f *fakeSearcher) SearchLdapGroupsV1(_ context.Context, q string) (*pro.LdapGroupSearchResults, error) {
	if f.err != nil {
		return nil, f.err
	}
	// Emulate the contains-match server: return everything; ResolveByName
	// filters to exact-name matches.
	return &pro.LdapGroupSearchResults{Results: f.results, TotalCount: len(f.results)}, nil
}

func strSet(vals ...string) types.Set {
	elems := make([]types.String, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	out, _ := types.SetValueFrom(context.Background(), types.StringType, elems)
	return out
}

var preflightPath = path.Root("scope").AtName("limitations").AtName("directory_service_user_group_names")

func TestValidateDSGroups_KnownGroupPasses(t *testing.T) {
	f := &fakeSearcher{results: []pro.LdapGroup{{Name: "Admins", ID: "1", LdapServerID: 31}}}
	diags := ValidateDirectoryServiceUserGroupNames(context.Background(), f, strSet("Admins"), preflightPath)
	if diags.HasError() {
		t.Fatalf("known group should not error: %v", diags)
	}
	if diags.WarningsCount() != 0 {
		t.Errorf("known group should not warn: %v", diags)
	}
}

func TestValidateDSGroups_UnknownGroupWarnsNotErrors(t *testing.T) {
	f := &fakeSearcher{results: []pro.LdapGroup{{Name: "Admins", ID: "1", LdapServerID: 31}}}
	diags := ValidateDirectoryServiceUserGroupNames(context.Background(), f, strSet("Nope"), preflightPath)
	// Advisory: a no-match must NOT block the plan (bootstrap case), only warn.
	if diags.HasError() {
		t.Fatalf("unknown group should warn, not error: %v", diags)
	}
	if diags.WarningsCount() == 0 {
		t.Fatal("unknown group should produce a warning diagnostic")
	}
}

func TestValidateDSGroups_SearchErrorWarnsNotErrors(t *testing.T) {
	f := &fakeSearcher{err: errors.New("ldap unreachable")}
	diags := ValidateDirectoryServiceUserGroupNames(context.Background(), f, strSet("Admins"), preflightPath)
	if diags.HasError() {
		t.Fatalf("search error must not block the plan: %v", diags)
	}
	if diags.WarningsCount() == 0 {
		t.Error("search error should surface a warning")
	}
}

func TestValidateDSGroups_NilSearcherAndNullSetNoop(t *testing.T) {
	if d := ValidateDirectoryServiceUserGroupNames(context.Background(), nil, strSet("Admins"), preflightPath); len(d) != 0 {
		t.Errorf("nil searcher should be a no-op, got %v", d)
	}
	f := &fakeSearcher{}
	if d := ValidateDirectoryServiceUserGroupNames(context.Background(), f, types.SetNull(types.StringType), preflightPath); len(d) != 0 {
		t.Errorf("null set should be a no-op, got %v", d)
	}
	if d := ValidateDirectoryServiceUserGroupNames(context.Background(), f, types.SetUnknown(types.StringType), preflightPath); len(d) != 0 {
		t.Errorf("unknown set should be a no-op, got %v", d)
	}
}

func TestValidateDSGroups_UnknownElementSkipped(t *testing.T) {
	f := &fakeSearcher{results: []pro.LdapGroup{{Name: "Admins", ID: "1", LdapServerID: 31}}}
	// A set holding only an unknown element (e.g. a name interpolated from a
	// not-yet-known attribute) cannot be validated at plan time and must be a
	// no-op rather than a false "not found".
	set := types.SetValueMust(types.StringType, []attr.Value{types.StringUnknown()})
	if d := ValidateDirectoryServiceUserGroupNames(context.Background(), f, set, preflightPath); len(d) != 0 {
		t.Errorf("unknown element should be skipped, got %v", d)
	}
}
