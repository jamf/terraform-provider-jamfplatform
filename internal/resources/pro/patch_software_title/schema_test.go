// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPatchSoftwareTitleResource_Metadata pins the Terraform type name. It is
// the resource's public address, so a change to it breaks every existing
// configuration.
func TestPatchSoftwareTitleResource_Metadata(t *testing.T) {
	r := NewPatchSoftwareTitleResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*PatchSoftwareTitleResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_patch_software_title" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_patch_software_title", resp.TypeName)
	}
}

// TestPatchSoftwareTitleResource_SchemaVersion pins the schema version against
// the state upgrader's registered key. The two are coupled by hand: bumping the
// version without registering the matching upgrader makes every existing state
// file unreadable, and registering one without bumping means it never runs.
func TestPatchSoftwareTitleResource_SchemaVersion(t *testing.T) {
	r := NewPatchSoftwareTitleResource().(*PatchSoftwareTitleResource)
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Schema.Version != 1 {
		t.Errorf("expected schema version 1, got %d", resp.Schema.Version)
	}
	if _, ok := r.UpgradeState(context.Background())[0]; !ok {
		t.Error("schema version 1 needs an upgrader registered for version 0, or v0 state cannot be read")
	}
}

// TestPatchSoftwareTitleResource_Schema pins the attribute set and each
// attribute's Optional/Required/Computed mode, which together decide how the
// framework plans the resource. Four of these are load-bearing beyond the
// obvious: category_id / site_id / the notification bools are Optional+Computed
// because the server defaults them; version_packages is Optional-only so a
// managed subset never plans as a computed map; accept_extension_attributes is
// Optional-only because it is one-way user intent rather than server state; and
// extension_attributes is plain Computed so it goes Unknown when an accept
// flips accepted false→true inside a single apply.
func TestPatchSoftwareTitleResource_Schema(t *testing.T) {
	r := NewPatchSoftwareTitleResource()
	var resp resource.SchemaResponse
	r.(*PatchSoftwareTitleResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	want := []string{"id", "name", "name_id", "software_title_name", "source_id", "category_id", "site_id", "web_notification", "email_notification", "version_packages", "available_versions", "accept_extension_attributes", "extension_attributes", "timeouts"}
	for _, name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(s.Attributes) != len(want) {
		t.Errorf("expected exactly %d attributes, got %d", len(want), len(s.Attributes))
	}

	id := s.Attributes["id"]
	if id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}

	for _, req := range []string{"name", "name_id", "source_id"} {
		if !s.Attributes[req].IsRequired() {
			t.Errorf("%q must be required", req)
		}
	}

	for _, oc := range []string{"category_id", "site_id", "web_notification", "email_notification"} {
		a := s.Attributes[oc]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be optional+computed, got optional=%v computed=%v", oc, a.IsOptional(), a.IsComputed())
		}
	}

	for _, c := range []string{"software_title_name", "available_versions", "extension_attributes"} {
		a := s.Attributes[c]
		if a.IsOptional() || a.IsRequired() || !a.IsComputed() {
			t.Errorf("%q must be computed-only, got optional=%v required=%v computed=%v", c, a.IsOptional(), a.IsRequired(), a.IsComputed())
		}
	}

	vp := s.Attributes["version_packages"]
	if !vp.IsOptional() {
		t.Errorf("version_packages must be optional")
	}
	if vp.IsComputed() {
		t.Errorf("version_packages must NOT be computed (managed-subset map)")
	}

	accept := s.Attributes["accept_extension_attributes"]
	if !accept.IsOptional() || accept.IsComputed() || accept.IsRequired() {
		t.Errorf("accept_extension_attributes must be optional-only, got optional=%v computed=%v required=%v", accept.IsOptional(), accept.IsComputed(), accept.IsRequired())
	}
}

// TestPatchSoftwareTitleResource_SchemaDropsV0NameAttributes pins the removal
// the v1 schema exists for. The v3 configuration reports category and site as
// ids only, so re-adding either display name would need a lookup per read for a
// value Terraform never writes — and would need a second schema version to get
// back out of.
func TestPatchSoftwareTitleResource_SchemaDropsV0NameAttributes(t *testing.T) {
	r := NewPatchSoftwareTitleResource()
	var resp resource.SchemaResponse
	r.(*PatchSoftwareTitleResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	for _, gone := range removedInV1 {
		if _, ok := resp.Schema.Attributes[gone]; ok {
			t.Errorf("%q was removed at schema v1 and must not be declared", gone)
		}
	}
}

// TestPatchSoftwareTitleDataSource_Metadata pins the data source's public
// address, which shares the resource's type name.
func TestPatchSoftwareTitleDataSource_Metadata(t *testing.T) {
	d := NewPatchSoftwareTitleDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*PatchSoftwareTitleDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_patch_software_title" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_patch_software_title", resp.TypeName)
	}
}

// TestPatchSoftwareTitleDataSource_Schema pins that id and name are the two
// mutually-exclusive selectors — Optional so either can be supplied, Computed
// so the other is filled from the response — and that every remaining attribute
// is Computed-only, so nothing else can be mistaken for a lookup key.
// category_name and site_name are gone here too: the data source reads the same
// v3 body as the resource.
func TestPatchSoftwareTitleDataSource_Schema(t *testing.T) {
	d := NewPatchSoftwareTitleDataSource()
	var resp datasource.SchemaResponse
	d.(*PatchSoftwareTitleDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, sel := range []string{"id", "name"} {
		a := s.Attributes[sel]
		if !a.IsOptional() {
			t.Errorf("%q must be optional (id-or-name selector)", sel)
		}
		if !a.IsComputed() {
			t.Errorf("%q must be computed (populated from response)", sel)
		}
	}

	for _, c := range []string{"name_id", "software_title_name", "source_id", "category_id", "site_id", "web_notification", "email_notification", "version_packages", "available_versions", "kill_apps"} {
		a, ok := s.Attributes[c]
		if !ok {
			t.Errorf("missing attribute %q", c)
			continue
		}
		if a.IsOptional() || a.IsRequired() || !a.IsComputed() {
			t.Errorf("%q must be computed-only, got optional=%v required=%v computed=%v", c, a.IsOptional(), a.IsRequired(), a.IsComputed())
		}
	}

	for _, gone := range removedInV1 {
		if _, ok := s.Attributes[gone]; ok {
			t.Errorf("%q was removed with the v3 migration and must not be declared on the data source", gone)
		}
	}
}

// TestPatchSoftwareTitleDataSource_ConfigValidators_ExactlyOneSelector pins that
// the id-or-name choice is enforced at plan time. Without the validator a config
// supplying neither reaches the read and fails mid-apply against the API instead.
func TestPatchSoftwareTitleDataSource_ConfigValidators_ExactlyOneSelector(t *testing.T) {
	d := NewPatchSoftwareTitleDataSource().(*PatchSoftwareTitleDataSource)
	got := d.ConfigValidators(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

// TestPatchSoftwareTitleListResource_Metadata pins the list resource's public
// address, which shares the resource's type name.
func TestPatchSoftwareTitleListResource_Metadata(t *testing.T) {
	r := NewPatchSoftwareTitleListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*PatchSoftwareTitleListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_patch_software_title" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_patch_software_title", resp.TypeName)
	}
}

// TestPatchSoftwareTitleListResource_Schema pins the filter block's presence.
// The v3 configurations list takes no query parameters, so client-side
// substring filtering on the display name is the only selectivity the list
// resource has.
func TestPatchSoftwareTitleListResource_Schema(t *testing.T) {
	r := NewPatchSoftwareTitleListResource()
	var resp list.ListResourceSchemaResponse
	r.(*PatchSoftwareTitleListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}
