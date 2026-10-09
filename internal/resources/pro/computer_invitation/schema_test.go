// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

func TestComputerInvitationResource_Metadata(t *testing.T) {
	r := NewComputerInvitationResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*ComputerInvitationResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_computer_invitation" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_computer_invitation", resp.TypeName)
	}
}

func TestComputerInvitationResource_Schema(t *testing.T) {
	r := NewComputerInvitationResource()
	var resp resource.SchemaResponse
	r.(*ComputerInvitationResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	want := []string{
		"id", "invitation", "invitation_type", "expiration_date",
		"expiration_date_epoch", "expiration_date_utc",
		"enroll_into_site_id", "enroll_into_site_name",
		"keep_existing_site_membership", "multiple_uses_allowed",
		"create_account_if_does_not_exist", "hide_account", "lock_down_ssh",
		"ssh_username", "ssh_password", "ssh_password_wo_version",
		"invitation_status", "times_used", "invited_user_uuid", "timeouts",
	}
	for _, name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	// invitation_type and ssh_username are Required (the endpoint 500s a create
	// without an SSH username, for both invitation types).
	for _, req := range []string{"invitation_type", "ssh_username"} {
		if !s.Attributes[req].IsRequired() {
			t.Errorf("%q must be required", req)
		}
	}

	// id is computed-only.
	id := s.Attributes["id"]
	if id.IsRequired() || id.IsOptional() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v optional=%v computed=%v", id.IsRequired(), id.IsOptional(), id.IsComputed())
	}

	// Server-derived fields must be computed-only (never Optional+Computed).
	// enroll_into_site_name is derived from enroll_into_site_id, so it is here.
	for _, comp := range []string{"invitation", "expiration_date_epoch", "expiration_date_utc", "invitation_status", "times_used", "invited_user_uuid", "enroll_into_site_name"} {
		a := s.Attributes[comp]
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%q must be computed-only, got required=%v optional=%v", comp, a.IsRequired(), a.IsOptional())
		}
		if !a.IsComputed() {
			t.Errorf("%q must be computed", comp)
		}
	}

	// Optional-only inputs (not Computed): ssh_password, ssh_password_wo_version.
	for _, opt := range []string{"ssh_password", "ssh_password_wo_version"} {
		a := s.Attributes[opt]
		if !a.IsOptional() {
			t.Errorf("%q must be optional", opt)
		}
		if a.IsComputed() {
			t.Errorf("%q must NOT be computed", opt)
		}
	}

	// Optional+Computed server-defaulted inputs. expiration_date is here (not
	// Optional-only): when omitted the server assigns a value (e.g. Unlimited)
	// that must be adoptable into state without a post-apply inconsistency.
	for _, oc := range []string{"expiration_date", "enroll_into_site_id", "keep_existing_site_membership", "multiple_uses_allowed", "create_account_if_does_not_exist", "hide_account", "lock_down_ssh"} {
		a := s.Attributes[oc]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be Optional+Computed, got optional=%v computed=%v", oc, a.IsOptional(), a.IsComputed())
		}
	}

	// ssh_password must be WriteOnly + Sensitive.
	pw := s.Attributes["ssh_password"]
	if !pw.IsSensitive() {
		t.Errorf("ssh_password must be sensitive")
	}
	if !pw.IsWriteOnly() {
		t.Errorf("ssh_password must be write-only")
	}
}

func TestComputerInvitationDataSource_Metadata(t *testing.T) {
	d := NewComputerInvitationDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*ComputerInvitationDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_computer_invitation" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_computer_invitation", resp.TypeName)
	}
}

func TestComputerInvitationDataSource_Schema(t *testing.T) {
	d := NewComputerInvitationDataSource()
	var resp datasource.SchemaResponse
	d.(*ComputerInvitationDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, sel := range []string{"id", "invitation"} {
		a := s.Attributes[sel]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be Optional+Computed selector, got optional=%v computed=%v", sel, a.IsOptional(), a.IsComputed())
		}
	}
	// ssh_password must NOT be exposed by the read-only data source.
	if _, ok := s.Attributes["ssh_password"]; ok {
		t.Errorf("data source must not expose ssh_password")
	}
}

func TestComputerInvitationDataSource_ConfigValidators_ExactlyOneSelector(t *testing.T) {
	d := NewComputerInvitationDataSource().(*ComputerInvitationDataSource)
	got := d.ConfigValidators(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestComputerInvitationListResource_Metadata(t *testing.T) {
	r := NewComputerInvitationListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*ComputerInvitationListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_computer_invitation" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_computer_invitation", resp.TypeName)
	}
}

func TestComputerInvitationListResource_Schema_NoFilter(t *testing.T) {
	r := NewComputerInvitationListResource()
	var resp list.ListResourceSchemaResponse
	r.(*ComputerInvitationListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	// Computer invitations carry no name, so there is deliberately no filter.
	if _, ok := resp.Schema.Attributes["filter"]; ok {
		t.Errorf("expected no filter attribute in the list config schema")
	}
	if _, ok := resp.Schema.Attributes[listtimeouts.AttributeName]; !ok {
		t.Errorf("expected a %q attribute in the list config schema", listtimeouts.AttributeName)
	}
	if len(resp.Schema.Attributes) != 1 {
		t.Errorf("expected only the timeouts attribute in the list config schema, got %d attributes", len(resp.Schema.Attributes))
	}
}
