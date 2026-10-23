// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// listEnvironments runs List against a stub serving body from the collection
// endpoint, and returns everything the stream pushed. The schemas come from the
// managed resource because ListRequest carries its schema and identity schema.
func listEnvironments(t *testing.T, body string, limit int64) []list.ListResult {
	t.Helper()
	ctx := context.Background()

	r := &PlatformEnvironmentListResource{client: newStubClient(t, serve(http.StatusOK, body))}

	managed := &PlatformEnvironmentResource{}
	var schemaResp resource.SchemaResponse
	managed.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	managed.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	var configResp list.ListResourceSchemaResponse
	r.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &configResp)
	configType := configResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	var stream list.ListResultsStream
	r.List(ctx, list.ListRequest{
		Config: tfsdk.Config{Schema: configResp.Schema, Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
			"timeouts": tftypes.NewValue(configType.AttributeTypes["timeouts"], nil),
		})},
		IncludeResource:        true,
		Limit:                  limit,
		ResourceSchema:         schemaResp.Schema,
		ResourceIdentitySchema: identityResp.IdentitySchema,
	}, &stream)

	if stream.Results == nil {
		t.Fatal("List left the stream unset, so Terraform receives neither results nor a diagnostic")
	}

	var results []list.ListResult
	for result := range stream.Results {
		if result.Diagnostics.HasError() {
			t.Fatalf("list diagnostics: %v", result.Diagnostics)
		}
		results = append(results, result)
	}
	return results
}

func displayNames(results []list.ListResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.DisplayName)
	}
	return names
}

func TestList_NoLimitStreamsEveryEnvironment(t *testing.T) {
	results := listEnvironments(t, twoEnvironmentsListBody, 0)

	if got := displayNames(results); len(got) != 2 || got[0] != "tf-unit" || got[1] != "tf-unit-global" {
		t.Fatalf("streamed %v, want both environments in Jamf Account's order", got)
	}

	var identity platformEnvironmentIdentityModel
	if diags := results[0].Identity.Get(context.Background(), &identity); diags.HasError() {
		t.Fatalf("reading back the identity: %v", diags)
	}
	if identity.ID.ValueString() != "env-1" {
		t.Errorf("identity id = %q, want the identifier", identity.ID.ValueString())
	}

	var state PlatformEnvironmentResourceModel
	if diags := results[0].Resource.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the resource: %v", diags)
	}
	if state.Name.ValueString() != "tf-unit" || state.Type.ValueString() != "SANDBOX" || state.Region.ValueString() != "EU" {
		t.Errorf("resource = %+v, want the environment hydrated for a bulk import", state)
	}
}

func TestList_LimitCutsTheStreamShort(t *testing.T) {
	if got := displayNames(listEnvironments(t, twoEnvironmentsListBody, 1)); len(got) != 1 || got[0] != "tf-unit" {
		t.Errorf("streamed %v, want exactly the first environment", got)
	}
}

func TestList_LimitAboveTheCollectionStreamsEverything(t *testing.T) {
	if got := displayNames(listEnvironments(t, twoEnvironmentsListBody, 50)); len(got) != 2 {
		t.Errorf("streamed %v, want both environments", got)
	}
}

func TestList_EmptyCollectionStreamsNoResults(t *testing.T) {
	if got := displayNames(listEnvironments(t, emptyListBody, 0)); len(got) != 0 {
		t.Errorf("streamed %v, want nothing", got)
	}
}
