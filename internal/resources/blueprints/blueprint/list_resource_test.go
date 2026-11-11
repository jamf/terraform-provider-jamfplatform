// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// newStubBlueprintsClient serves the blueprints endpoints from handle. testhelpers.NewMockClient does
// the same, but internal/testhelpers imports the provider, which imports this package, so an
// in-package test cannot use it.
func newStubBlueprintsClient(t *testing.T, handle http.HandlerFunc) *blueprints.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	return blueprints.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

const listedBlueprintsJSON = `{"totalCount":3,"results":[` +
	`{"id":"bp-1","name":"Unreadable","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"},` +
	`{"id":"bp-2","name":"Repeats","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"},` +
	`{"id":"bp-3","name":"Clean","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}]}`

const repeatsBlueprintJSON = `{"id":"bp-2","name":"Repeats","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z",` +
	`"deploymentState":{"state":"NOT_DEPLOYED"},"scope":{"deviceGroups":["00000000-0000-0000-0000-000000000001"]},` +
	`"steps":[{"name":"s","components":[{"identifier":"com.jamf.ddm.software-update-settings","configuration":` +
	`{"Beta":{"Included":true,"Value":{"ProgramEnrollment":"AlwaysOn","OfferPrograms":[{"Token":"t","Description":"d"},{"Token":"t","Description":"d"}]}}}}]}]}`

const cleanBlueprintJSON = `{"id":"bp-3","name":"Clean","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z",` +
	`"deploymentState":{"state":"NOT_DEPLOYED"},"scope":{"deviceGroups":["00000000-0000-0000-0000-000000000001"]},"steps":[]}`

// runBlueprintList runs List with config generation on against the stub and returns every result
// the stream pushed.
func runBlueprintList(t *testing.T, client *blueprints.Client) []list.ListResult {
	t.Helper()
	ctx := context.Background()
	r := &BlueprintListResource{client: client}

	managed := NewBlueprintResource()
	var schemaResp resource.SchemaResponse
	managed.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	managed.(resource.ResourceWithIdentity).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	var configResp list.ListResourceSchemaResponse
	r.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &configResp)
	configType := configResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, attrType := range configType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}

	var stream list.ListResultsStream
	r.List(ctx, list.ListRequest{
		Config:                 tfsdk.Config{Schema: configResp.Schema, Raw: tftypes.NewValue(configType, values)},
		IncludeResource:        true,
		ResourceSchema:         schemaResp.Schema,
		ResourceIdentitySchema: identityResp.IdentitySchema,
	}, &stream)
	if stream.Results == nil {
		t.Fatal("List left the stream unset")
	}
	var results []list.ListResult
	for result := range stream.Results {
		results = append(results, result)
	}
	return results
}

// TestBlueprintList_OneBlueprintCannotFailTheQuery pins issue #568's isolation requirement for
// blueprints: a blueprint whose read fails is left out and named, a blueprint with a repeated beta
// program hydrates, and the clean blueprint after both is still returned.
func TestBlueprintList_OneBlueprintCannotFailTheQuery(t *testing.T) {
	client := newStubBlueprintsClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blueprints/v1/blueprints":
			_, _ = w.Write([]byte(listedBlueprintsJSON))
		case "/blueprints/v1/blueprints/bp-1":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"httpStatus":500,"errors":[{"code":"INTERNAL","description":"boom"}]}`))
		case "/blueprints/v1/blueprints/bp-2":
			_, _ = w.Write([]byte(repeatsBlueprintJSON))
		case "/blueprints/v1/blueprints/bp-3":
			_, _ = w.Write([]byte(cleanBlueprintJSON))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	results := runBlueprintList(t, client)

	if len(results) != 3 {
		t.Fatalf("got %d results, want two blueprints and one trailing warning", len(results))
	}
	for _, res := range results[:2] {
		if res.Diagnostics.HasError() {
			t.Errorf("%s: unexpected error diagnostics: %v", res.DisplayName, res.Diagnostics)
		}
	}
	if results[0].DisplayName != "Repeats" || results[1].DisplayName != "Clean" {
		t.Errorf("results = %q, %q; want Repeats then Clean", results[0].DisplayName, results[1].DisplayName)
	}

	var state BlueprintResourceModel
	if diags := results[0].Resource.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the repeats blueprint: %v", diags)
	}
	if len(state.ComponentBlocks) != 1 || state.ComponentBlocks[0].SoftwareUpdateSettings == nil ||
		len(state.ComponentBlocks[0].SoftwareUpdateSettings.BetaOfferPrograms) != 2 {
		t.Errorf("the repeated beta program was not kept: %+v", state.ComponentBlocks)
	}

	trailing := results[2]
	if trailing.Identity != nil || trailing.Resource != nil || trailing.DisplayName != "" {
		t.Error("the trailing result must carry diagnostics only, or the framework turns it into an error")
	}
	if trailing.Diagnostics.HasError() || len(trailing.Diagnostics) != 1 {
		t.Fatalf("trailing diagnostics = %v, want exactly one warning", trailing.Diagnostics)
	}
	detail := trailing.Diagnostics[0].Detail()
	for _, want := range []string{"Unreadable (id bp-1)", "Unable to read blueprint"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning detail does not name %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "Repeats") || strings.Contains(detail, "Clean") {
		t.Errorf("warning names a blueprint that was returned:\n%s", detail)
	}
}

// TestBlueprintList_EveryBlueprintSkippedStillWarns guards the empty-result branch: with every
// blueprint skipped the stream must carry the warning, not the empty iterator.
func TestBlueprintList_EveryBlueprintSkippedStillWarns(t *testing.T) {
	client := newStubBlueprintsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blueprints/v1/blueprints" {
			_, _ = w.Write([]byte(`{"totalCount":1,"results":[{"id":"bp-1","name":"Unreadable","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	results := runBlueprintList(t, client)
	if len(results) != 1 || len(results[0].Diagnostics) != 1 || results[0].Identity != nil || results[0].Diagnostics.HasError() {
		t.Fatalf("got %d results (%v), want the trailing warning alone", len(results), results)
	}
}
