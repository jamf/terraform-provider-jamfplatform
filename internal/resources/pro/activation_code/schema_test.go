// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestActivationCodeResource_Metadata(t *testing.T) {
	r := NewActivationCodeResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*ActivationCodeResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_activation_code" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_activation_code", resp.TypeName)
	}
}

func TestActivationCodeResource_Schema(t *testing.T) {
	r := NewActivationCodeResource()
	var resp resource.SchemaResponse
	r.(*ActivationCodeResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema
	for _, name := range []string{"id", "organization_name", "code", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	id := s.Attributes["id"]
	if id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}

	if !s.Attributes["organization_name"].IsRequired() {
		t.Errorf("organization_name must be required")
	}

	code := s.Attributes["code"]
	if !code.IsRequired() {
		t.Errorf("code must be required")
	}
	if !code.IsSensitive() {
		t.Errorf("code must be sensitive (license secret)")
	}
}

func TestActivationCodeResource_IdentitySchema(t *testing.T) {
	r := NewActivationCodeResource()
	var resp resource.IdentitySchemaResponse
	r.(*ActivationCodeResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("identity schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Errorf("identity schema missing id attribute")
	}
}

func TestActivationCodeDataSource_Metadata(t *testing.T) {
	d := NewActivationCodeDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*ActivationCodeDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_activation_code" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_activation_code", resp.TypeName)
	}
}

func TestActivationCodeDataSource_Schema(t *testing.T) {
	d := NewActivationCodeDataSource()
	var resp datasource.SchemaResponse
	d.(*ActivationCodeDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema
	for _, name := range []string{"id", "organization_name", "code", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if !s.Attributes["id"].IsComputed() {
		t.Errorf("data source id must be computed for singleton")
	}
	if !s.Attributes["code"].IsSensitive() {
		t.Errorf("data source code must be sensitive")
	}
}

// TestActivationCodeConstructs_AreDeprecated pins the whole-schema deprecation on both
// constructs. The Classic /activationcode endpoint is deprecated with no replacement read
// (see deprecation.go), so neither construct may ship without the notice — a schema edit
// that drops it would quietly tell users this surface has a future.
func TestActivationCodeConstructs_AreDeprecated(t *testing.T) {
	var resourceResp resource.SchemaResponse
	NewActivationCodeResource().(*ActivationCodeResource).Schema(context.Background(), resource.SchemaRequest{}, &resourceResp)
	if got := resourceResp.Schema.DeprecationMessage; got != resourceDeprecation {
		t.Errorf("resource DeprecationMessage = %q, want %q", got, resourceDeprecation)
	}

	var dsResp datasource.SchemaResponse
	NewActivationCodeDataSource().(*ActivationCodeDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &dsResp)
	if got := dsResp.Schema.DeprecationMessage; got != dataSourceDeprecation {
		t.Errorf("data source DeprecationMessage = %q, want %q", got, dataSourceDeprecation)
	}

	if got := resourceResp.Schema.MarkdownDescription; !strings.HasPrefix(got, resourceDeprecationCallout) {
		t.Errorf("the resource description must open with the deprecation callout, because tfplugindocs does not render DeprecationMessage; got: %q", got)
	}
	if got := dsResp.Schema.MarkdownDescription; !strings.HasPrefix(got, dataSourceDeprecationCallout) {
		t.Errorf("the data source description must open with the deprecation callout, because tfplugindocs does not render DeprecationMessage; got: %q", got)
	}

	for name, msg := range map[string]string{"resource": resourceDeprecation, "data source": dataSourceDeprecation} {
		if !strings.Contains(msg, "/activationcode") {
			t.Errorf("%s deprecation must name the deprecated endpoint: %q", name, msg)
		}
		if !strings.Contains(msg, "2026-07-14") {
			t.Errorf("%s deprecation must carry the deprecation date: %q", name, msg)
		}
	}
}
