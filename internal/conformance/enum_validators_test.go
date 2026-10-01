// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/provider"
)

// TestEnumAttributesRejectUnknownValues pins plan-time validation on Jamf Pro
// attributes whose server answers a 2xx for a value it then ignores, coerces
// or stores verbatim, so the validator is the only place a typo surfaces.
// Each row names one value the attribute must accept; every row must reject
// "not-a-value", plus its own invalid value when one is named. Wire-probed 2026-10-01 against Jamf Pro 11.32.0 (see the
// vocabulary doc comment beside each attribute).
func TestEnumAttributesRejectUnknownValues(t *testing.T) {
	cases := []struct {
		resource, path, valid, invalid string
	}{
		{"jamfplatform_pro_cloud_identity_provider", "google.mappings.user_mappings.search_scope", "ALL_SUBTREES", ""},
		{"jamfplatform_pro_cloud_identity_provider", "google.mappings.user_mappings.object_class_limitation", "ANY_OBJECT_CLASSES", ""},
		{"jamfplatform_pro_cloud_identity_provider", "google.mappings.group_mappings.search_scope", "FIRST_LEVEL_ONLY", ""},
		{"jamfplatform_pro_cloud_identity_provider", "google.mappings.group_mappings.object_class_limitation", "ALL_OBJECT_CLASSES", ""},
		{"jamfplatform_pro_ebook", "general.file_type", "IBOOKS", "IBOOK"},
		{"jamfplatform_pro_ebook", "general.file_type", "ePub", ""},
		{"jamfplatform_pro_ebook", "self_service.notification_method", "Self Service", ""},
		{"jamfplatform_pro_mac_app_store_app", "self_service.notification_method", "Self Service", ""},
		{"jamfplatform_pro_licensed_software", "licenses.license_type", "Site License", ""},
		{"jamfplatform_pro_patch_policy", "user_interaction.notifications.type", "Self Service and Notification Center", ""},
		{"jamfplatform_pro_directory_binding", "active_directory.network_protocol", "smb", ""},
		{"jamfplatform_pro_directory_binding", "admitmac.network_protocol", "afp", ""},
		{"jamfplatform_pro_macos_configuration_profile", "general.redeploy_on_update", "Newly Assigned", ""},
		{"jamfplatform_pro_mobile_device_configuration_profile", "general.redeploy_on_update", "All", ""},
		{"jamfplatform_pro_disk_encryption_configuration", "institutional_recovery_key.certificate_type", "PKCS12", ""},
	}
	schemas := resourceSchemas(t)
	for _, tc := range cases {
		s, ok := schemas[tc.resource]
		if !ok {
			t.Errorf("%s: resource not registered", tc.resource)
			continue
		}
		a, err := stringAttributeAt(s.Attributes, strings.Split(tc.path, "."))
		if err != "" {
			t.Errorf("%s.%s: %s", tc.resource, tc.path, err)
			continue
		}
		if !passes(a.Validators, tc.valid) {
			t.Errorf("%s.%s rejected valid value %q", tc.resource, tc.path, tc.valid)
		}
		for _, bad := range []string{"not-a-value", tc.invalid} {
			if bad != "" && passes(a.Validators, bad) {
				t.Errorf("%s.%s accepted invalid value %q", tc.resource, tc.path, bad)
			}
		}
	}
}

func resourceSchemas(t *testing.T) map[string]rschema.Schema {
	t.Helper()
	ctx := context.Background()
	out := map[string]rschema.Schema{}
	for _, ctor := range provider.New("test")().Resources(ctx) {
		r := ctor()
		meta := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, meta)
		resp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, resp)
		out[meta.TypeName] = resp.Schema
	}
	return out
}

// stringAttributeAt descends by attribute name through single-nested objects
// and the element object of nested lists and sets.
func stringAttributeAt(attrs map[string]rschema.Attribute, names []string) (rschema.StringAttribute, string) {
	for i, name := range names {
		a, ok := attrs[name]
		if !ok {
			return rschema.StringAttribute{}, "no attribute " + strings.Join(names[:i+1], ".")
		}
		if i == len(names)-1 {
			s, ok := a.(rschema.StringAttribute)
			if !ok {
				return rschema.StringAttribute{}, "not a string attribute"
			}
			return s, ""
		}
		switch n := a.(type) {
		case rschema.SingleNestedAttribute:
			attrs = n.Attributes
		case rschema.ListNestedAttribute:
			attrs = n.NestedObject.Attributes
		case rschema.SetNestedAttribute:
			attrs = n.NestedObject.Attributes
		default:
			return rschema.StringAttribute{}, strings.Join(names[:i+1], ".") + " is not a nested attribute"
		}
	}
	return rschema.StringAttribute{}, "empty path"
}

func passes(validators []validator.String, v string) bool {
	resp := &validator.StringResponse{}
	for _, val := range validators {
		val.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(v)}, resp)
	}
	return !resp.Diagnostics.HasError()
}
