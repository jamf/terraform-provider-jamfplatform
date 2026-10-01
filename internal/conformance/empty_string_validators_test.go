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

// TestEmptyStringRejectedWhereJamfProDoesNotStoreIt pins plan-time rejection of
// "" on attributes where Jamf Pro refuses an empty value, replaces it with a
// default, or discards the object. Every row was wire-probed on 2026-10-01
// against Jamf Pro 11.32.0 (#445); before the validator each one failed at
// apply, on the server's error or on the post-apply consistency check. Each row
// must also accept a non-empty value, so the validator cannot be a blanket
// rejection.
func TestEmptyStringRejectedWhereJamfProDoesNotStoreIt(t *testing.T) {
	cases := []struct {
		resource, path, valid string
	}{
		{"jamfplatform_pro_policy", "local_accounts.username", "localadmin"},
		{"jamfplatform_pro_advanced_computer_search", "criteria.name", "Computer Name"},
		{"jamfplatform_pro_advanced_user_search", "criteria.name", "Username"},
		{"jamfplatform_pro_advanced_mobile_device_search", "criteria.name", "Display Name"},
		{"jamfplatform_pro_advanced_volume_purchasing_content_search", "criteria.name", "Content Name"},
		{"jamfplatform_pro_user_group", "criteria.name", "Username"},
		{"jamfplatform_pro_smart_computer_group", "criteria.name", "Computer Name"},
		{"jamfplatform_pro_smart_mobile_device_group", "criteria.name", "Display Name"},
		{"jamfplatform_pro_advanced_mobile_device_search", "site_id", "-1"},
		{"jamfplatform_pro_advanced_volume_purchasing_content_search", "site_id", "1"},
		{"jamfplatform_pro_disk_encryption_configuration", "institutional_recovery_key.data", "TUlJ"},
		{"jamfplatform_pro_pki_adcs", "api_client_id", "00000000-0000-0000-0000-000000000000"},
		{"jamfplatform_pro_mobile_device_invitation", "target_ios", "iOS 4"},
		{"jamfplatform_pro_printer", "category", "Printers"},
		{"jamfplatform_pro_mobile_device_provisioning_profile", "profile_data", "TUlJ"},
		{"jamfplatform_pro_ldap_server", "mappings_for_users.user_mappings.user_uuid", "objectGUID"},
		{"jamfplatform_pro_ldap_server", "mappings_for_users.user_group_mappings.group_uuid", "objectGUID"},
		{"jamfplatform_pro_directory_binding", "admitmac.home_location", "Local"},
	}
	schemas := emptyStringResourceSchemas(t)
	for _, tc := range cases {
		s, ok := schemas[tc.resource]
		if !ok {
			t.Errorf("%s: resource not registered", tc.resource)
			continue
		}
		a, err := emptyStringAttributeAt(s.Attributes, strings.Split(tc.path, "."))
		if err != "" {
			t.Errorf("%s.%s: %s", tc.resource, tc.path, err)
			continue
		}
		if !emptyStringPasses(a.Validators, tc.valid) {
			t.Errorf("%s.%s rejected valid value %q", tc.resource, tc.path, tc.valid)
		}
		if emptyStringPasses(a.Validators, "") {
			t.Errorf("%s.%s accepted an empty string", tc.resource, tc.path)
		}
	}
}

func emptyStringResourceSchemas(t *testing.T) map[string]rschema.Schema {
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

func emptyStringAttributeAt(attrs map[string]rschema.Attribute, names []string) (rschema.StringAttribute, string) {
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

func emptyStringPasses(validators []validator.String, v string) bool {
	resp := &validator.StringResponse{}
	for _, val := range validators {
		val.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(v)}, resp)
	}
	return !resp.Diagnostics.HasError()
}
