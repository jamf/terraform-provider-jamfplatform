// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenants

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

const tenantsBody = `{"totalCount":3,"results":[` +
	`{"id":"t-1","name":"corp","product":"pro","type":"management","url":"https://corp.example","metaRegion":"EU","environmentId":"env-1"},` +
	`{"id":"t-2","name":null,"product":"jsc","type":null,"url":null,"metaRegion":"US","environmentId":null},` +
	`{"id":"t-3"}]}`

func newStubClient(t *testing.T, status int, body string) *account.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return account.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

func schemaOf(t *testing.T) dsschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	NewTenantsDataSource().(*TenantsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func read(t *testing.T, status int, body string) (datasource.ReadResponse, TenantsDataSourceModel) {
	t.Helper()
	ctx := context.Background()
	d := &TenantsDataSource{client: newStubClient(t, status, body)}
	s := schemaOf(t)

	object := s.Type().TerraformType(ctx).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	config := tftypes.NewValue(object, values)

	resp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(object, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: config}}, &resp)

	var data TenantsDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &data); diags.HasError() {
			t.Fatalf("reading back the state: %v", diags)
		}
	}
	return resp, data
}

func TestMetadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewTenantsDataSource().(*TenantsDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != "jamfplatform_account_tenants" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestSchema(t *testing.T) {
	s := schemaOf(t)

	nested, ok := s.Attributes["tenants"].(dsschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("tenants must be a ListNestedAttribute, got %T", s.Attributes["tenants"])
	}
	want := []string{"id", "name", "product", "type", "url", "region", "platform_environment_id"}
	for _, name := range want {
		attr, present := nested.NestedObject.Attributes[name]
		if !present {
			t.Errorf("tenants missing nested attribute %q", name)
			continue
		}
		if !attr.IsComputed() || attr.IsOptional() || attr.IsRequired() {
			t.Errorf("tenants.%s must be computed only", name)
		}
	}
	if len(nested.NestedObject.Attributes) != len(want) {
		t.Errorf("tenants declares %d nested attributes, the test knows %d", len(nested.NestedObject.Attributes), len(want))
	}
	if _, ok := s.Attributes["environment_id"]; ok {
		t.Error("the data source takes no filter arguments")
	}
}

func TestRead_MapsEveryFieldAndKeepsAbsentOnesNull(t *testing.T) {
	resp, data := read(t, http.StatusOK, tenantsBody)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if data.ID.ValueString() != dataSourceID || len(data.Tenants) != 3 {
		t.Fatalf("data = %+v", data)
	}

	first := data.Tenants[0]
	if first.ID.ValueString() != "t-1" || first.Name.ValueString() != "corp" || first.Product.ValueString() != "pro" ||
		first.Type.ValueString() != "management" || first.URL.ValueString() != "https://corp.example" ||
		first.Region.ValueString() != "EU" || first.PlatformEnvironmentID.ValueString() != "env-1" {
		t.Errorf("first tenant = %+v", first)
	}

	unassigned := data.Tenants[1]
	if !unassigned.PlatformEnvironmentID.IsNull() || !unassigned.Name.IsNull() || !unassigned.URL.IsNull() || !unassigned.Type.IsNull() {
		t.Errorf("an unassigned tenant must carry nulls, not empty strings: %+v", unassigned)
	}

	bare := data.Tenants[2]
	if bare.ID.ValueString() != "t-3" || !bare.Region.IsNull() || !bare.Product.IsNull() {
		t.Errorf("a tenant with only an identifier must still map: %+v", bare)
	}
}

func TestRead_EmptyOrganizationIsAnEmptyList(t *testing.T) {
	resp, data := read(t, http.StatusOK, `{"totalCount":0,"results":[]}`)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if data.Tenants == nil || len(data.Tenants) != 0 {
		t.Errorf("tenants = %#v, want an empty non-null list", data.Tenants)
	}
}

func TestRead_FailureSurfaces(t *testing.T) {
	resp, _ := read(t, http.StatusInternalServerError, `{"httpStatus":500,"errors":[{"code":"UPSTREAM_ERROR","field":null,"description":"The request could not be completed"}]}`)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed list must be reported")
	}
}

func TestSDKMethods_MatchTheCallAndRegistry(t *testing.T) {
	if missing := permissions.Missing(account.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
	src, err := os.ReadFile("data_source.go")
	if err != nil {
		t.Fatal(err)
	}
	called := map[string]bool{}
	for _, m := range regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`).FindAllStringSubmatch(string(src), -1) {
		if _, ok := account.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	if len(called) != len(dataSourceSDKMethods) || !called["ListTenants"] {
		t.Errorf("data_source.go calls %v, declared %v", called, dataSourceSDKMethods)
	}
}

func TestPrivileges_RenderedAndNeverClaimNoPermissionNeeded(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "platform-environments:read") {
		t.Errorf("dataSourcePrivileges did not render platform-environments:read:\n%s", dataSourcePrivileges)
	}
	for _, other := range []string{"create", "update", "delete"} {
		if permissions.Renders(dataSourcePrivileges, "platform-environments:"+other) {
			t.Errorf("the tenants data source asks for the %s action", other)
		}
	}
}
