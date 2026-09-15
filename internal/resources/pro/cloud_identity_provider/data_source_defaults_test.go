// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// defaultsSchema compiles the defaults data source schema, failing the test on
// any diagnostic.
func defaultsSchema(t *testing.T) dsschema.Schema {
	t.Helper()
	d := NewCloudIdentityProviderDefaultsDataSource()
	var resp datasource.SchemaResponse
	d.(*CloudIdentityProviderDefaultsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// resourceSchemaForComparison compiles the managed resource's schema.
func resourceSchemaForComparison(t *testing.T) rschema.Schema {
	t.Helper()
	r := NewCloudIdentityProviderResource()
	var resp resource.SchemaResponse
	r.(*CloudIdentityProviderResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// dsNestedAttributeNames returns the sorted attribute names of a nested
// attribute reached by walking the given path through a data source schema.
func dsNestedAttributeNames(t *testing.T, s dsschema.Schema, path ...string) []string {
	t.Helper()
	attrs := s.Attributes
	for i, name := range path {
		attr, ok := attrs[name]
		if !ok {
			t.Fatalf("data source schema has no attribute %q at depth %d", name, i)
		}
		nested, ok := attr.(dsschema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("data source attribute %q is not a single nested attribute", name)
		}
		attrs = nested.Attributes
	}
	return sortedKeys(len(attrs), func(yield func(string)) {
		for k := range attrs {
			yield(k)
		}
	})
}

// rNestedAttributeNames is dsNestedAttributeNames for the resource schema.
func rNestedAttributeNames(t *testing.T, s rschema.Schema, path ...string) []string {
	t.Helper()
	attrs := s.Attributes
	for i, name := range path {
		attr, ok := attrs[name]
		if !ok {
			t.Fatalf("resource schema has no attribute %q at depth %d", name, i)
		}
		nested, ok := attr.(rschema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("resource attribute %q is not a single nested attribute", name)
		}
		attrs = nested.Attributes
	}
	return sortedKeys(len(attrs), func(yield func(string)) {
		for k := range attrs {
			yield(k)
		}
	})
}

// sortedKeys collects the yielded names into a sorted slice, so two schemas'
// attribute sets compare independently of map iteration order.
func sortedKeys(n int, each func(yield func(string))) []string {
	out := make([]string, 0, n)
	each(func(k string) { out = append(out, k) })
	sort.Strings(out)
	return out
}

// TestDefaultsDataSource_Metadata verifies the data source type name.
func TestDefaultsDataSource_Metadata(t *testing.T) {
	d := NewCloudIdentityProviderDefaultsDataSource()
	var resp datasource.MetadataResponse
	d.(*CloudIdentityProviderDefaultsDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if want := "jamfplatform_pro_cloud_identity_provider_defaults"; resp.TypeName != want {
		t.Errorf("type name = %q, want %q", resp.TypeName, want)
	}
}

// TestDefaultsDataSource_SchemaIsFullyComputed pins that the data source takes
// no arguments. A defaults lookup has nothing to select: both products' defaults
// exist whatever the tenant holds, and the one path parameter involved has a
// single legal value.
func TestDefaultsDataSource_SchemaIsFullyComputed(t *testing.T) {
	s := defaultsSchema(t)

	for _, name := range []string{"entra_id", "google"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("missing top-level attribute %q", name)
		}
		if attr.IsRequired() || attr.IsOptional() {
			t.Errorf("%q must be computed-only", name)
		}
		if !attr.IsComputed() {
			t.Errorf("%q must be computed", name)
		}
	}

	if _, ok := s.Attributes["timeouts"]; !ok {
		t.Error("missing timeouts attribute")
	}
	if _, ok := s.Attributes["id"]; ok {
		t.Error("the defaults data source selects nothing, so it must not carry an id")
	}
}

// TestDefaultsSchema_MappingsMatchTheResource is the copy-across guarantee. The
// point of the data source is seeding a managed resource's mappings block, and
// `mappings = data.<...>.entra_id.mappings` only works while both objects carry
// the same attributes. The models are shared, so a mismatch here means a schema
// was edited on one side alone.
func TestDefaultsSchema_MappingsMatchTheResource(t *testing.T) {
	ds := defaultsSchema(t)
	rs := resourceSchemaForComparison(t)

	cases := []struct {
		name    string
		dsPath  []string
		resPath []string
	}{
		{"entra_id mappings", []string{"entra_id", "mappings"}, []string{"entra_id", "mappings"}},
		{"google mappings", []string{"google", "mappings"}, []string{"google", "mappings"}},
		{"google user mappings", []string{"google", "mappings", "user_mappings"}, []string{"google", "mappings", "user_mappings"}},
		{"google group mappings", []string{"google", "mappings", "group_mappings"}, []string{"google", "mappings", "group_mappings"}},
		{"google membership mappings", []string{"google", "mappings", "membership_mappings"}, []string{"google", "mappings", "membership_mappings"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dsNestedAttributeNames(t, ds, tc.dsPath...)
			want := rNestedAttributeNames(t, rs, tc.resPath...)
			if len(got) != len(want) {
				t.Fatalf("attribute count %d, resource has %d\n data source: %v\n resource:    %v", len(got), len(want), got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("attribute %d = %q, resource has %q", i, got[i], want[i])
				}
			}
		})
	}
}

// TestAssignEntraIDDefaults_FoldsTheResponse covers the Entra ID fold, mappings
// included: that response carries them inline, which is why the deprecated
// mappings endpoint is never called.
func TestAssignEntraIDDefaults_FoldsTheResponse(t *testing.T) {
	got := assignEntraIDDefaults(&pro.AzureServerConfiguration{
		Type:                                     "PUBLIC",
		SearchTimeout:                            30,
		TransitiveMembershipEnabled:              true,
		TransitiveMembershipUserField:            "userPrincipalName",
		TransitiveDirectoryMembershipEnabled:     false,
		MembershipCalculationOptimizationEnabled: true,
		Mappings: &pro.AzureMappings{
			UserID:    "id",
			UserName:  "userPrincipalName",
			Building:  "",
			GroupName: "displayName",
		},
	})

	if got == nil {
		t.Fatal("expected a populated Entra ID block")
	}
	if got.Type.ValueString() != "PUBLIC" {
		t.Errorf("type = %q, want PUBLIC", got.Type.ValueString())
	}
	if got.SearchTimeout.ValueInt64() != 30 {
		t.Errorf("search_timeout = %d, want 30", got.SearchTimeout.ValueInt64())
	}
	if !got.TransitiveMembershipEnabled.ValueBool() || !got.MembershipCalculationOptimizationEnabled.ValueBool() {
		t.Errorf("the two enabled toggles must carry through, got %+v", got)
	}
	if got.Mappings == nil {
		t.Fatal("expected the mappings the server-configuration response carries inline")
	}
	if got.Mappings.UserID.ValueString() != "id" || got.Mappings.GroupName.ValueString() != "displayName" {
		t.Errorf("mappings did not fold: %+v", got.Mappings)
	}
	if got.Mappings.Building.IsNull() || got.Mappings.Building.ValueString() != "" {
		t.Errorf("an empty default must land as an empty string, not null, got %v", got.Mappings.Building)
	}
}

// TestAssignEntraIDDefaults_NilResponse pins that a nil response yields a nil
// block rather than a zero-valued one, so nothing invents a default Jamf Pro
// never reported.
func TestAssignEntraIDDefaults_NilResponse(t *testing.T) {
	if got := assignEntraIDDefaults(nil); got != nil {
		t.Errorf("expected nil for a nil response, got %+v", got)
	}
}

// TestAssignGoogleDefaults_FoldsBothReads covers the Google fold, which stitches
// two responses into one block.
func TestAssignGoogleDefaults_FoldsBothReads(t *testing.T) {
	searchBase := "ou=Extra"
	got := assignGoogleDefaults(
		&pro.CloudLdapServerResponse{
			ServerURL:         "ldap.google.com",
			Port:              636,
			ConnectionType:    "LDAPS",
			ConnectionTimeout: 15,
			SearchTimeout:     60,
			UseWildcards:      true,
		},
		&pro.CloudLdapMappingsResponse{
			UserMappings: &pro.UserMappings{
				ObjectClasses:        "inetOrgPerson",
				SearchBase:           "ou=Users",
				AdditionalSearchBase: &searchBase,
				UserID:               "mail",
				Username:             "uid",
			},
			GroupMappings:      &pro.GroupMappings{ObjectClasses: "groupOfNames", GroupID: "cn"},
			MembershipMappings: &pro.MembershipMappings{GroupMembershipMapping: "memberOf"},
		},
	)

	if got == nil || got.Server == nil || got.Mappings == nil {
		t.Fatalf("expected both halves populated, got %+v", got)
	}
	if got.Server.ServerURL.ValueString() != "ldap.google.com" || got.Server.Port.ValueInt64() != 636 {
		t.Errorf("server defaults did not fold: %+v", got.Server)
	}
	if got.Server.ConnectionType.ValueString() != "LDAPS" || !got.Server.UseWildcards.ValueBool() {
		t.Errorf("server defaults did not fold: %+v", got.Server)
	}
	if got.Mappings.UserMappings == nil || got.Mappings.UserMappings.Username.ValueString() != "uid" {
		t.Errorf("user mappings did not fold: %+v", got.Mappings.UserMappings)
	}
	if got.Mappings.UserMappings.AdditionalSearchBase.ValueString() != searchBase {
		t.Errorf("additional_search_base = %v, want %q", got.Mappings.UserMappings.AdditionalSearchBase, searchBase)
	}
	if got.Mappings.GroupMappings == nil || got.Mappings.GroupMappings.GroupID.ValueString() != "cn" {
		t.Errorf("group mappings did not fold: %+v", got.Mappings.GroupMappings)
	}
	if got.Mappings.MembershipMappings == nil || got.Mappings.MembershipMappings.GroupMembershipMapping.ValueString() != "memberOf" {
		t.Errorf("membership mappings did not fold: %+v", got.Mappings.MembershipMappings)
	}
}

// TestAssignGoogleDefaults_AdditionalSearchBaseIsNull covers the one nullable
// field in the Google mappings, in both the shapes Jamf Pro can send. It comes
// back as an empty string today, and an empty string is what the server then
// rejects on a write ("Passed value is not in DN format"), so reporting null for
// both is the honest answer: there is no default to copy. Empty and absent
// deliberately collapse to the same value, and the attribute description says
// so.
func TestAssignGoogleDefaults_AdditionalSearchBaseIsNull(t *testing.T) {
	empty := ""
	cases := map[string]*pro.UserMappings{
		"absent": {UserID: "mail"},
		"empty":  {UserID: "mail", AdditionalSearchBase: &empty},
	}

	for name, um := range cases {
		t.Run(name, func(t *testing.T) {
			got := assignGoogleDefaults(
				&pro.CloudLdapServerResponse{ServerURL: "ldap.google.com"},
				&pro.CloudLdapMappingsResponse{UserMappings: um},
			)
			if got.Mappings == nil || got.Mappings.UserMappings == nil {
				t.Fatalf("expected user mappings, got %+v", got)
			}
			if !got.Mappings.UserMappings.AdditionalSearchBase.IsNull() {
				t.Errorf("additional_search_base must be null, got %v", got.Mappings.UserMappings.AdditionalSearchBase)
			}
		})
	}
}

// TestAssignGoogleDefaults_PartialResponses pins that each half stands alone, so
// one absent response cannot blank the other.
func TestAssignGoogleDefaults_PartialResponses(t *testing.T) {
	serverOnly := assignGoogleDefaults(&pro.CloudLdapServerResponse{ServerURL: "ldap.google.com"}, nil)
	if serverOnly.Server == nil || serverOnly.Mappings != nil {
		t.Errorf("expected server only, got %+v", serverOnly)
	}

	mappingsOnly := assignGoogleDefaults(nil, &pro.CloudLdapMappingsResponse{MembershipMappings: &pro.MembershipMappings{GroupMembershipMapping: "memberOf"}})
	if mappingsOnly.Server != nil || mappingsOnly.Mappings == nil {
		t.Errorf("expected mappings only, got %+v", mappingsOnly)
	}
}
