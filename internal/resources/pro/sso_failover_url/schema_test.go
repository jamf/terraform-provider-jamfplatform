// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_failover_url

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestSsoFailoverURLResource_Metadata checks the resource type name.
func TestSsoFailoverURLResource_Metadata(t *testing.T) {
	r := NewSsoFailoverURLResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*SsoFailoverURLResource).Metadata(context.Background(), req, &resp)
	if resp.TypeName != "jamfplatform_pro_sso_failover_url" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_sso_failover_url", resp.TypeName)
	}
}

// TestSsoFailoverURLResource_Schema asserts required/computed attributes.
func TestSsoFailoverURLResource_Schema(t *testing.T) {
	r := NewSsoFailoverURLResource()
	var resp resource.SchemaResponse
	r.(*SsoFailoverURLResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	rt, ok := resp.Schema.Attributes["regeneration_trigger"]
	if !ok || !rt.IsOptional() {
		t.Errorf("regeneration_trigger must be Optional")
	}
	if ok && rt.IsRequired() {
		t.Errorf("regeneration_trigger must not be Required — Jamf never returns it, so Required breaks the import/generate-config-out round-trip")
	}
	for _, name := range []string{"failover_url", "generation_time", "generation_time_utc", "id"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("missing attribute %q", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("attribute %q must be Computed", name)
		}
	}
	if !resp.Schema.Attributes["failover_url"].IsSensitive() {
		t.Errorf("failover_url must be Sensitive")
	}
}

// TestSsoFailoverURLDataSource_Metadata checks the DS type name.
func TestSsoFailoverURLDataSource_Metadata(t *testing.T) {
	d := NewSsoFailoverURLDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*SsoFailoverURLDataSource).Metadata(context.Background(), req, &resp)
	if resp.TypeName != "jamfplatform_pro_sso_failover_url" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_sso_failover_url", resp.TypeName)
	}
}

// TestFormatGenerationTimeUTC covers the ms→RFC3339 helper.
func TestFormatGenerationTimeUTC(t *testing.T) {
	got := formatGenerationTimeUTC(1779818738978)
	want := "2026-05-26T18:05:38Z"
	if got != want {
		t.Errorf("formatGenerationTimeUTC(1779818738978) = %q, want %q", got, want)
	}
}

// TestAssignResourceModel covers the SDK→state mapping.
func TestAssignResourceModel(t *testing.T) {
	state := &SsoFailoverURLResourceModel{}
	wire := &pro.SsoFailoverData{
		FailoverURL:    "https://tenant.jamfcloud.com/?failover=abc",
		GenerationTime: 1779818738978,
	}
	assignResourceModel(state, wire)
	if state.FailoverURL.ValueString() != wire.FailoverURL {
		t.Errorf("failover_url = %q, want %q", state.FailoverURL.ValueString(), wire.FailoverURL)
	}
	if state.GenerationTime.ValueInt64() != wire.GenerationTime {
		t.Errorf("generation_time = %d, want %d", state.GenerationTime.ValueInt64(), wire.GenerationTime)
	}
	if state.GenerationTimeUTC.IsNull() {
		t.Errorf("generation_time_utc must be populated")
	}
}
