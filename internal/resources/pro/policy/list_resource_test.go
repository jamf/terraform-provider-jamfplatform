// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// newStubProClassicClient serves the classic policy endpoints from handle.
// testhelpers.NewMockClient does the same, but internal/testhelpers imports
// the provider, which imports this package, so an in-package test cannot use
// it.
func newStubProClassicClient(t *testing.T, handle http.HandlerFunc) *proclassic.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	return proclassic.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

const listedPoliciesXML = `<?xml version="1.0" encoding="UTF-8"?><policies><size>3</size>` +
	`<policy><id>1</id><name>Unreadable</name></policy>` +
	`<policy><id>2</id><name>Repeats</name></policy>` +
	`<policy><id>3</id><name>Clean</name></policy></policies>`

const repeatsPolicyXML = `<?xml version="1.0" encoding="UTF-8"?><policy><general><id>2</id><name>Repeats</name><enabled>false</enabled></general>` +
	`<scripts><size>2</size>` +
	`<script><id>10</id><name>S</name><priority>After</priority><parameter4>x</parameter4></script>` +
	`<script><id>10</id><name>S</name><priority>After</priority><parameter4>x</parameter4></script></scripts>` +
	`<package_configuration><packages><size>2</size>` +
	`<package><id>20</id><name>P</name><action>Install</action><fut>false</fut><feu>false</feu><update_autorun>false</update_autorun></package>` +
	`<package><id>20</id><name>P</name><action>Install</action><fut>false</fut><feu>false</feu><update_autorun>false</update_autorun></package>` +
	`</packages></package_configuration>` +
	`<printers><size>2</size><printer><id>30</id><name>Pr</name><action>install</action><make_default>false</make_default></printer>` +
	`<printer><id>30</id><name>Pr</name><action>install</action><make_default>false</make_default></printer></printers>` +
	`<dock_items><size>2</size><dock_item><id>40</id><name>D</name><action>Add To End</action></dock_item>` +
	`<dock_item><id>40</id><name>D</name><action>Add To End</action></dock_item></dock_items>` +
	`<account_maintenance><directory_bindings><size>2</size><binding><id>50</id><name>B</name></binding>` +
	`<binding><id>50</id><name>B</name></binding></directory_bindings></account_maintenance></policy>`

const cleanPolicyXML = `<?xml version="1.0" encoding="UTF-8"?><policy><general><id>3</id><name>Clean</name><enabled>true</enabled></general></policy>`

// runPolicyList runs List with config generation on against the stub and
// returns every result the stream pushed.
func runPolicyList(t *testing.T, client *proclassic.Client) []list.ListResult {
	t.Helper()
	ctx := context.Background()
	r := &PolicyListResource{client: client}

	managed := NewPolicyResource()
	var schemaResp resource.SchemaResponse
	managed.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	managed.(resource.ResourceWithIdentity).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	var configResp list.ListResourceSchemaResponse
	r.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &configResp)
	configType := configResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	var stream list.ListResultsStream
	r.List(ctx, list.ListRequest{
		Config: tfsdk.Config{Schema: configResp.Schema, Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
			"filter":   tftypes.NewValue(configType.AttributeTypes["filter"], nil),
			"timeouts": tftypes.NewValue(configType.AttributeTypes["timeouts"], nil),
		})},
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

// TestPolicyList_OnePolicyCannotFailTheQuery pins issue #568's isolation
// requirement: a policy whose read fails is left out and named, a policy with
// repeated entries hydrates, and the clean policy after both is still
// returned.
func TestPolicyList_OnePolicyCannotFailTheQuery(t *testing.T) {
	client := newStubProClassicClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/proclassic/policies":
			_, _ = w.Write([]byte(listedPoliciesXML))
		case "/proclassic/policies/id/1":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<html><body>boom</body></html>`))
		case "/proclassic/policies/id/2":
			_, _ = w.Write([]byte(repeatsPolicyXML))
		case "/proclassic/policies/id/3":
			_, _ = w.Write([]byte(cleanPolicyXML))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	results := runPolicyList(t, client)

	if len(results) != 3 {
		t.Fatalf("got %d results, want two policies and one trailing warning", len(results))
	}
	for _, res := range results[:2] {
		if res.Diagnostics.HasError() {
			t.Errorf("%s: unexpected error diagnostics: %v", res.DisplayName, res.Diagnostics)
		}
	}
	if results[0].DisplayName != "Repeats" || results[1].DisplayName != "Clean" {
		t.Errorf("results = %q, %q; want Repeats then Clean", results[0].DisplayName, results[1].DisplayName)
	}

	var state PolicyResourceModel
	if diags := results[0].Resource.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the repeats policy: %v", diags)
	}
	if len(state.ScriptAssignments) != 2 || len(state.Packages.Packages) != 2 || len(state.PrinterAssignments) != 2 ||
		len(state.DockItemAssignments) != 2 || len(state.DirectoryBindings) != 2 {
		t.Errorf("repeated entries were not all kept: %d scripts, %d packages, %d printers, %d dock items, %d bindings",
			len(state.ScriptAssignments), len(state.Packages.Packages), len(state.PrinterAssignments), len(state.DockItemAssignments), len(state.DirectoryBindings))
	}

	trailing := results[2]
	if trailing.Identity != nil || trailing.Resource != nil || trailing.DisplayName != "" {
		t.Error("the trailing result must carry diagnostics only, or the framework turns it into an error")
	}
	if trailing.Diagnostics.HasError() || len(trailing.Diagnostics) != 1 {
		t.Fatalf("trailing diagnostics = %v, want exactly one warning", trailing.Diagnostics)
	}
	detail := trailing.Diagnostics[0].Detail()
	for _, want := range []string{"Unreadable (id 1)", "Unable to read Jamf Pro policy"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning detail does not name %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "Repeats") || strings.Contains(detail, "Clean") {
		t.Errorf("warning names a policy that was returned:\n%s", detail)
	}
}

// TestPolicyList_EveryPolicySkippedStillWarns guards the empty-result branch:
// with every policy skipped the stream must carry the warning, not the empty
// iterator.
func TestPolicyList_EveryPolicySkippedStillWarns(t *testing.T) {
	client := newStubProClassicClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path == "/proclassic/policies" {
			_, _ = w.Write([]byte(`<policies><size>1</size><policy><id>1</id><name>Unreadable</name></policy></policies>`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	results := runPolicyList(t, client)
	if len(results) != 1 || len(results[0].Diagnostics) != 1 || results[0].Identity != nil {
		t.Fatalf("got %d results (%v), want the trailing warning alone", len(results), results)
	}
}
