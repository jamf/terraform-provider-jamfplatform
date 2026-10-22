// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func readPlural(t *testing.T, status int, body string) (datasource.ReadResponse, PlatformEnvironmentsDataSourceModel) {
	t.Helper()
	ctx := context.Background()
	d := &PlatformEnvironmentsDataSource{client: newStubClient(t, serve(status, body))}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

	config := objectRaw(ctx, schemaResp.Schema.Type(), nil, nil)
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(config.Type(), nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, &resp)

	var data PlatformEnvironmentsDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &data); diags.HasError() {
			t.Fatalf("reading back the state: %v", diags)
		}
	}
	return resp, data
}

func TestPluralDataSource_ListsEveryEnvironmentInOrder(t *testing.T) {
	resp, data := readPlural(t, http.StatusOK, twoEnvironmentsListBody)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if data.ID.ValueString() != pluralDataSourceID {
		t.Errorf("id = %q", data.ID.ValueString())
	}
	if len(data.PlatformEnvironments) != 2 || data.PlatformEnvironments[0].ID.ValueString() != "env-1" || data.PlatformEnvironments[1].ID.ValueString() != "env-2" {
		t.Fatalf("results = %+v, want both environments in Jamf Account's order", data.PlatformEnvironments)
	}
	if !data.PlatformEnvironments[1].Region.IsNull() {
		t.Error("a global environment must report a null region")
	}
}

func TestPluralDataSource_EmptyOrganizationIsAnEmptyList(t *testing.T) {
	resp, data := readPlural(t, http.StatusOK, emptyListBody)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if data.PlatformEnvironments == nil || len(data.PlatformEnvironments) != 0 {
		t.Errorf("results = %#v, want an empty non-null list", data.PlatformEnvironments)
	}
}

func TestPluralDataSource_FailureSurfaces(t *testing.T) {
	resp, _ := readPlural(t, http.StatusInternalServerError, serverFailureBody)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed list must be reported")
	}
}
