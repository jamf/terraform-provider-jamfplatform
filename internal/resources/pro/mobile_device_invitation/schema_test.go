// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestMobileDeviceInvitationResource_Metadata(t *testing.T) {
	r := NewMobileDeviceInvitationResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*MobileDeviceInvitationResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_mobile_device_invitation" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_mobile_device_invitation", resp.TypeName)
	}
}

func TestMobileDeviceInvitationResource_Schema(t *testing.T) {
	r := NewMobileDeviceInvitationResource()
	var resp resource.SchemaResponse
	r.(*MobileDeviceInvitationResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	want := []string{
		"id", "invitation", "invitation_type", "expiration_date",
		"expiration_date_epoch", "expiration_date_utc",
		"enroll_into_site_id", "enroll_into_site_name",
		"keep_existing_site_membership", "multiple_uses_allowed", "require_login",
		"subject", "message", "reply_to", "sent_from", "sent_to", "username",
		"target_ios", "last_action", "date_sent", "date_sent_utc",
		"date_sent_epoch", "timeouts",
	}
	for _, name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	// invitation_type is Required.
	if !s.Attributes["invitation_type"].IsRequired() {
		t.Errorf("invitation_type must be required")
	}

	// id is computed-only.
	id := s.Attributes["id"]
	if id.IsRequired() || id.IsOptional() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v optional=%v computed=%v", id.IsRequired(), id.IsOptional(), id.IsComputed())
	}

	// Server-derived fields must be computed-only (never Optional+Computed).
	// enroll_into_site_name is derived from enroll_into_site_id, so it is here.
	for _, comp := range []string{"invitation", "expiration_date_epoch", "expiration_date_utc", "last_action", "date_sent", "date_sent_utc", "date_sent_epoch", "enroll_into_site_name"} {
		a := s.Attributes[comp]
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%q must be computed-only, got required=%v optional=%v", comp, a.IsRequired(), a.IsOptional())
		}
		if !a.IsComputed() {
			t.Errorf("%q must be computed", comp)
		}
	}

	// Optional-only inputs (not Computed): the email surface. These fields
	// collapse the empty-string echo to null, so none leaks a server default —
	// no Computed needed.
	for _, opt := range []string{"subject", "message", "reply_to", "sent_from", "sent_to", "username"} {
		a := s.Attributes[opt]
		if !a.IsOptional() {
			t.Errorf("%q must be optional", opt)
		}
		if a.IsComputed() {
			t.Errorf("%q must NOT be computed", opt)
		}
	}

	// Optional+Computed server-defaulted inputs. expiration_date and target_ios
	// are here (not Optional-only): when omitted the server assigns a value
	// (e.g. Unlimited / iOS 4) that must be adoptable into state without a
	// post-apply inconsistency.
	for _, oc := range []string{"expiration_date", "enroll_into_site_id", "target_ios", "keep_existing_site_membership", "multiple_uses_allowed", "require_login"} {
		a := s.Attributes[oc]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be Optional+Computed, got optional=%v computed=%v", oc, a.IsOptional(), a.IsComputed())
		}
	}
}

func TestMobileDeviceInvitationDataSource_Metadata(t *testing.T) {
	d := NewMobileDeviceInvitationDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*MobileDeviceInvitationDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_mobile_device_invitation" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_mobile_device_invitation", resp.TypeName)
	}
}

func TestMobileDeviceInvitationDataSource_Schema(t *testing.T) {
	d := NewMobileDeviceInvitationDataSource()
	var resp datasource.SchemaResponse
	d.(*MobileDeviceInvitationDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

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
}

func TestMobileDeviceInvitationDataSource_ConfigValidators_ExactlyOneSelector(t *testing.T) {
	d := NewMobileDeviceInvitationDataSource().(*MobileDeviceInvitationDataSource)
	got := d.ConfigValidators(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestMobileDeviceInvitationListResource_Metadata(t *testing.T) {
	r := NewMobileDeviceInvitationListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*MobileDeviceInvitationListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_mobile_device_invitation" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_mobile_device_invitation", resp.TypeName)
	}
}

func TestMobileDeviceInvitationListResource_Schema_NoFilter(t *testing.T) {
	r := NewMobileDeviceInvitationListResource()
	var resp list.ListResourceSchemaResponse
	r.(*MobileDeviceInvitationListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; ok {
		t.Error("mobile device invitations carry no name, so the list schema must not expose a filter")
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok {
		t.Error("expected the list schema to expose a timeouts attribute")
	}
	if len(resp.Schema.Attributes) != 1 {
		t.Errorf("expected only the timeouts attribute, got %d attributes", len(resp.Schema.Attributes))
	}
}
