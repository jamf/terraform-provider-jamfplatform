// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestComputerPrestageEnrollmentResource_Metadata(t *testing.T) {
	r := NewComputerPrestageEnrollmentResource()
	var resp resource.MetadataResponse
	r.(*ComputerPrestageEnrollmentResource).Metadata(context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if want := "jamfplatform_pro_computer_prestage_enrollment"; resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

func TestComputerPrestageEnrollmentResource_Schema(t *testing.T) {
	r := NewComputerPrestageEnrollmentResource()
	var resp resource.SchemaResponse
	r.(*ComputerPrestageEnrollmentResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	required := []string{
		"id", "display_name", "make_mdm_profile_mandatory", "allow_mdm_profile_removal",
		"require_authentication", "device_enrollment_program_instance_id",
		"use_existing_location_information", "use_existing_site_membership",
		"automatically_advance_through_setup_assistant", "configuration_profile_ids",
		"authentication_message",
		"prevent_activation_lock", "enable_device_based_activation_lock",
		"skip_setup_items", "location_information",
		"purchasing_information", "account_settings",
		"scope_serial_numbers", "timeouts",
	}
	for _, name := range required {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	if id := s.Attributes["id"]; id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only")
	}
	if dn := s.Attributes["display_name"]; !dn.IsRequired() {
		t.Errorf("display_name must be required")
	}
	if uuid := s.Attributes["profile_uuid"]; uuid.IsRequired() || uuid.IsOptional() || !uuid.IsComputed() {
		t.Errorf("profile_uuid must be computed-only")
	}

	// Per STYLE_GUIDE: SingleNestedAttribute backed by *StructModel must be
	// Optional-only (no Computed). Internal validation runs at apply time;
	// here we ValidateImplementation to surface schema-shape mistakes early.
	diags := s.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("ValidateImplementation diagnostics: %v", diags)
	}
}

func TestComputerPrestageEnrollmentDataSource_Metadata(t *testing.T) {
	d := NewComputerPrestageEnrollmentDataSource()
	var resp datasource.MetadataResponse
	d.(*ComputerPrestageEnrollmentDataSource).Metadata(context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if want := "jamfplatform_pro_computer_prestage_enrollment"; resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

func TestComputerPrestageEnrollmentDataSource_Schema(t *testing.T) {
	d := NewComputerPrestageEnrollmentDataSource()
	var resp datasource.SchemaResponse
	d.(*ComputerPrestageEnrollmentDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("data source schema diagnostics: %v", resp.Diagnostics)
	}
	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("data source ValidateImplementation diagnostics: %v", diags)
	}
}
