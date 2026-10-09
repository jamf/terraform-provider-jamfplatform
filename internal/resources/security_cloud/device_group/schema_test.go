// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestDeviceGroupResource_Metadata(t *testing.T) {
	r := NewDeviceGroupResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*DeviceGroupResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_device_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_device_group", resp.TypeName)
	}
}

// TestDeviceGroupResource_Schema pins the whole writable surface. The group object
// is a name and an identifier and nothing else, so an attribute appearing here
// that is not in this list means something was modelled that the API discards.
func TestDeviceGroupResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	want := map[string]bool{"id": true, "name": true, "timeouts": true}
	for name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for name := range s.Attributes {
		if !want[name] {
			t.Errorf("unexpected attribute %q — the device group object carries only id and name", name)
		}
	}

	id := s.Attributes["id"]
	if id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}
}

// TestDeviceGroupResource_NameIsRequiredNotComputed pins the one schema decision
// that is easy to get wrong here. `name` is API-required — a create or update
// without it is refused — which is the carve-out from the Optional+Computed
// default in STYLE_GUIDE §Full-replace endpoints. Adding Computed would make an
// omitted name plan as Unknown and silently reuse the prior value.
func TestDeviceGroupResource_NameIsRequiredNotComputed(t *testing.T) {
	name := resourceSchema(t).Attributes["name"]

	if !name.IsRequired() {
		t.Errorf("name must be required")
	}
	if name.IsComputed() {
		t.Errorf("name must not be computed")
	}
	if name.IsOptional() {
		t.Errorf("name must not be optional")
	}
}

// TestDeviceGroupResource_NameCarriesGroupNameValidator guards the plan-time
// refusals the server would otherwise turn into an inconsistent-result error
// (surrounding whitespace) or a mid-apply 400 (the reserved name).
func TestDeviceGroupResource_NameCarriesGroupNameValidator(t *testing.T) {
	name, ok := resourceSchema(t).Attributes["name"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("name must be a StringAttribute, got %T", resourceSchema(t).Attributes["name"])
	}

	found := false
	for _, v := range name.Validators {
		if _, is := v.(groupNameValidator); is {
			found = true
		}
	}
	if !found {
		t.Errorf("name must declare GroupName(); validators were %#v", name.Validators)
	}
}

func TestDeviceGroupResource_IdentitySchema(t *testing.T) {
	r := NewDeviceGroupResource()
	var resp resource.IdentitySchemaResponse
	r.(*DeviceGroupResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Errorf("identity schema missing id attribute")
	}
}

func TestDeviceGroupDataSource_Metadata(t *testing.T) {
	d := NewDeviceGroupDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*DeviceGroupDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_device_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_device_group", resp.TypeName)
	}
}

func TestDeviceGroupDataSource_Schema(t *testing.T) {
	d := NewDeviceGroupDataSource()
	var resp datasource.SchemaResponse
	d.(*DeviceGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "name", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for _, name := range []string{"id", "name"} {
		attr := s.Attributes[name]
		if attr.IsRequired() {
			t.Errorf("data source %s must not be required — id and name are alternatives", name)
		}
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("data source %s must be optional+computed, got optional=%v computed=%v", name, attr.IsOptional(), attr.IsComputed())
		}
	}
}

// TestDeviceGroupDataSource_NameHasNoValidators pins that the data source does not
// inherit the resource's name validator. A lookup must not be refused because the
// group's stored name would fail a rule that only applies to names being written:
// a group created before this provider existed may well carry surrounding
// whitespace, and finding it is exactly what the data source is for.
func TestDeviceGroupDataSource_NameHasNoValidators(t *testing.T) {
	d := NewDeviceGroupDataSource()
	var resp datasource.SchemaResponse
	d.(*DeviceGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	name, ok := resp.Schema.Attributes["name"].(dsschema.StringAttribute)
	if !ok {
		t.Fatalf("data source name must be a StringAttribute, got %T", resp.Schema.Attributes["name"])
	}
	if len(name.Validators) != 0 {
		t.Errorf("data source name must declare no validators, got %#v", name.Validators)
	}
}

func TestDeviceGroupDataSource_ConfigValidators(t *testing.T) {
	d := NewDeviceGroupDataSource()
	validators := d.(*DeviceGroupDataSource).ConfigValidators(context.Background())
	if len(validators) == 0 {
		t.Fatal("data source must declare a config validator enforcing exactly one of id or name")
	}
}

func TestDeviceGroupListResource_Metadata(t *testing.T) {
	r := NewDeviceGroupListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*DeviceGroupListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_device_group" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_security_cloud_device_group", resp.TypeName)
	}
}

// TestDeviceGroupListResource_Schema asserts the config schema carries only the `timeouts` attribute. The group
// list endpoint accepts no query parameters at all — not even a sort — so an
// attribute appearing here would mean a filter was added without wiring it.
func TestDeviceGroupListResource_Schema(t *testing.T) {
	r := NewDeviceGroupListResource()
	var resp list.ListResourceSchemaResponse
	r.(*DeviceGroupListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok || len(resp.Schema.Attributes) != 1 {
		t.Errorf("list schema must take only the timeouts attribute, got %v", resp.Schema.Attributes)
	}
}

// resourceSchema builds the resource schema once per test.
func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := NewDeviceGroupResource()
	var resp resource.SchemaResponse
	r.(*DeviceGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}
