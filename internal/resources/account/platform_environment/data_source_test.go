// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const duplicateNamesListBody = `{"totalCount":3,"results":[` +
	`{"id":"env-1","href":"/v1/environments/env-1","name":"shared-name","label":"SANDBOX","metaRegion":"EU"},` +
	`{"id":"env-2","href":"/v1/environments/env-2","name":"shared-name","label":"TRIAL","metaRegion":null},` +
	`{"id":"env-3","href":"/v1/environments/env-3","name":"unique","label":"PRODUCTION","metaRegion":"US"}]}`

func readDataSource(t *testing.T, body string, set map[string]string) datasource.ReadResponse {
	t.Helper()
	ctx := context.Background()
	d := &PlatformEnvironmentDataSource{client: newStubClient(t, serve(http.StatusOK, body))}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

	config := objectRaw(ctx, schemaResp.Schema.Type(), set, nil)
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(config.Type(), nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, &resp)
	return resp
}

func TestDataSource_LooksUpByID(t *testing.T) {
	resp := readDataSource(t, duplicateNamesListBody, map[string]string{"id": "env-3"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	var data PlatformEnvironmentDataSourceModel
	if diags := resp.State.Get(context.Background(), &data); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if data.Name.ValueString() != "unique" || data.Type.ValueString() != "PRODUCTION" || data.Region.ValueString() != "US" {
		t.Errorf("data = %+v, want env-3 adopted", data)
	}
}

func TestDataSource_LooksUpByName(t *testing.T) {
	resp := readDataSource(t, duplicateNamesListBody, map[string]string{"name": "unique"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	var data PlatformEnvironmentDataSourceModel
	if diags := resp.State.Get(context.Background(), &data); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if data.ID.ValueString() != "env-3" {
		t.Errorf("id = %q, want env-3", data.ID.ValueString())
	}
}

func TestDataSource_GlobalEnvironmentHasNullRegion(t *testing.T) {
	resp := readDataSource(t, duplicateNamesListBody, map[string]string{"id": "env-2"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	var data PlatformEnvironmentDataSourceModel
	if diags := resp.State.Get(context.Background(), &data); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if !data.Region.IsNull() {
		t.Errorf("region = %v, want null for a global environment", data.Region)
	}
}

func TestDataSource_DuplicateNameNamesEveryIdentifier(t *testing.T) {
	resp := readDataSource(t, duplicateNamesListBody, map[string]string{"name": "shared-name"})
	if !resp.Diagnostics.HasError() {
		t.Fatal("an ambiguous name must be refused")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"env-1", "env-2", "`id`"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
	if strings.Contains(detail, "env-3") {
		t.Errorf("detail %q names an environment that does not match", detail)
	}
}

func TestDataSource_UnknownKeysAreNamed(t *testing.T) {
	for name, set := range map[string]map[string]string{
		"id":   {"id": "env-404"},
		"name": {"name": "nothing-here"},
	} {
		resp := readDataSource(t, duplicateNamesListBody, set)
		if !resp.Diagnostics.HasError() {
			t.Errorf("a lookup by %s that matches nothing must fail", name)
			continue
		}
		if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "jamfplatform_account_platform_environments") {
			t.Errorf("detail %q does not point at the plural data source", detail)
		}
	}
}
