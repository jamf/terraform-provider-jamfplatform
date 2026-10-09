// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
)

func synchronizeSchema(t *testing.T) actionschema.Schema {
	t.Helper()
	a := NewSynchronizeAction()
	var resp action.SchemaResponse
	a.(*SynchronizeAction).Schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestSynchronizeAction_Metadata(t *testing.T) {
	a := NewSynchronizeAction()
	var resp action.MetadataResponse
	a.(*SynchronizeAction).Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	want := "jamfplatform_security_cloud_uem_connect_synchronize"
	if resp.TypeName != want {
		t.Errorf("type name = %q, want %q", resp.TypeName, want)
	}
}

// TestSynchronizeAction_IDIsOptional pins the shape the whole ergonomics rest on: a
// tenant holds at most one integration, so requiring its opaque identifier would be
// friction with nothing to disambiguate. The attribute stays available because
// naming the resource's ID is how a configuration orders the action after it.
func TestSynchronizeAction_IDIsOptional(t *testing.T) {
	s := synchronizeSchema(t)

	attr, ok := s.Attributes["uem_connect_id"]
	if !ok {
		t.Fatal("missing attribute uem_connect_id")
	}
	if attr.IsRequired() {
		t.Error("uem_connect_id must be optional — a tenant has only one integration to act on")
	}
	if !attr.IsOptional() {
		t.Error("uem_connect_id must be optional")
	}
}

// TestSynchronizeAction_NoOtherAttributes pins that the action takes nothing else.
// An action is fire-once with no state, so every attribute added here is a
// parameter a caller has to get right for something they cannot observe the result
// of.
func TestSynchronizeAction_NoOtherAttributes(t *testing.T) {
	s := synchronizeSchema(t)

	if len(s.Attributes) != 2 {
		names := make([]string, 0, len(s.Attributes))
		for name := range s.Attributes {
			names = append(names, name)
		}
		t.Errorf("expected only uem_connect_id and timeouts, got %v", names)
	}
}

// TestSynchronizeAction_DescriptionSaysItDoesNotReport pins the honesty that matters
// most for a fire-once action over an asynchronous operation: the request is
// accepted, not completed, so the description must not imply the sync has finished
// or that its outcome is available here.
func TestSynchronizeAction_DescriptionSaysItDoesNotReport(t *testing.T) {
	s := synchronizeSchema(t)
	desc := strings.TrimSuffix(s.MarkdownDescription, synchronizePrivileges)

	for _, fragment := range []string{"background", "does not wait", "latest_sync"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("description does not mention %q:\n%s", fragment, desc)
		}
	}
}

// TestSynchronizeAction_DescriptionsAreUIAligned pins STYLE_GUIDE §User-facing
// descriptions are UI-aligned, not wire-aligned.
func TestSynchronizeAction_DescriptionsAreUIAligned(t *testing.T) {
	s := synchronizeSchema(t)

	descriptions := []string{strings.TrimSuffix(s.MarkdownDescription, synchronizePrivileges)}
	for _, attr := range s.Attributes {
		descriptions = append(descriptions, attr.GetMarkdownDescription())
	}

	for _, desc := range descriptions {
		lower := strings.ToLower(desc)
		for _, banned := range []string{"endpoint", "/v1/", " sdk", "202", "http"} {
			if strings.Contains(lower, banned) {
				t.Errorf("description contains wire vocabulary %q:\n%s", banned, desc)
			}
		}
	}
}

func deploySchema(t *testing.T) actionschema.Schema {
	t.Helper()
	a := NewDeployActivationProfileAction()
	var resp action.SchemaResponse
	a.(*DeployActivationProfileAction).Schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestDeployActivationProfileAction_Metadata(t *testing.T) {
	a := NewDeployActivationProfileAction()
	var resp action.MetadataResponse
	a.(*DeployActivationProfileAction).Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	want := "jamfplatform_security_cloud_activation_profile_deploy"
	if resp.TypeName != want {
		t.Errorf("type name = %q, want %q", resp.TypeName, want)
	}
}

// TestDeployActivationProfileAction_Attributes pins the shape: the code and the
// operating system are both required because nothing can resolve either, and the
// groups are optional because the deploy is legal without them — though the
// description has to say what that leaves behind.
func TestDeployActivationProfileAction_Attributes(t *testing.T) {
	s := deploySchema(t)

	required := []string{"activation_profile_code", "os"}
	for _, name := range required {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing attribute %s", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}

	groups, ok := s.Attributes["jamf_pro_group_ids"]
	if !ok {
		t.Fatal("missing attribute jamf_pro_group_ids")
	}
	if !groups.IsOptional() || groups.IsRequired() {
		t.Error("jamf_pro_group_ids must be optional")
	}

	if len(s.Attributes) != 4 {
		names := make([]string, 0, len(s.Attributes))
		for name := range s.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Errorf("expected exactly the three documented attributes and timeouts, got %v", names)
	}
}

// TestDeployActivationProfileAction_NoUEMAttribute pins the deliberate omission.
// The admin UI offers thirteen UEM solutions and the API accepts only Jamf Pro, so
// an attribute here would be a choice with one legal answer — and a caller who set
// it to anything else would get an unattributed refusal mid-apply.
func TestDeployActivationProfileAction_NoUEMAttribute(t *testing.T) {
	s := deploySchema(t)

	for _, name := range []string{"uem", "uem_vendor", "uem_platform"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("schema declares %q, which has exactly one legal value and belongs in a constant", name)
		}
	}
}

// TestDeployActivationProfileAction_DescriptionWarnsAboutScope is the honesty test
// that matters most here, and the one the wire probes exist to justify. Two
// behaviours are invisible from a successful apply: scope accumulates and is never
// cleared, and a first deployment with no groups leaves the configuration profile
// reaching nothing. Both are reported as success.
func TestDeployActivationProfileAction_DescriptionWarnsAboutScope(t *testing.T) {
	s := deploySchema(t)
	desc := strings.TrimSuffix(s.MarkdownDescription, deployActivationProfilePrivileges)

	for _, fragment := range []string{"only ever accumulates", "never removes", "scopes the configuration profile to " +
		"nothing", "reaches no devices"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("description does not warn about %q:\n%s", fragment, desc)
		}
	}
}

// TestDeployActivationProfileAction_OSValuesDocumented pins that every accepted
// value is named in the description. The validator and the list are built from the
// same function, so this catches the list being replaced by a hand-written one.
func TestDeployActivationProfileAction_OSValuesDocumented(t *testing.T) {
	s := deploySchema(t)

	attr, ok := s.Attributes["os"]
	if !ok {
		t.Fatal("missing attribute os")
	}
	desc := attr.GetMarkdownDescription()

	for _, value := range osValues() {
		if !strings.Contains(desc, "`"+value+"`") {
			t.Errorf("os description does not document accepted value %q:\n%s", value, desc)
		}
	}
}

// TestDeployActivationProfileAction_GroupDescriptionSaysWhichKind pins the one
// cross-field rule a caller cannot discover from the schema: which kind of Jamf Pro
// group each operating system takes. It cannot be validated at plan time — the
// group's kind is only knowable from the tenant — so the description carries it.
func TestDeployActivationProfileAction_GroupDescriptionSaysWhichKind(t *testing.T) {
	s := deploySchema(t)

	attr, ok := s.Attributes["jamf_pro_group_ids"]
	if !ok {
		t.Fatal("missing attribute jamf_pro_group_ids")
	}
	desc := attr.GetMarkdownDescription()

	for _, fragment := range []string{"Computer groups", "mobile device groups", "`" + macOSValue + "`"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("jamf_pro_group_ids description does not mention %q:\n%s", fragment, desc)
		}
	}
}

// TestDeployActivationProfileAction_DescriptionsAreUIAligned pins STYLE_GUIDE
// §User-facing descriptions are UI-aligned, not wire-aligned.
func TestDeployActivationProfileAction_DescriptionsAreUIAligned(t *testing.T) {
	s := deploySchema(t)

	descriptions := []string{strings.TrimSuffix(s.MarkdownDescription, deployActivationProfilePrivileges)}
	for _, attr := range s.Attributes {
		descriptions = append(descriptions, attr.GetMarkdownDescription())
	}

	for _, desc := range descriptions {
		lower := strings.ToLower(desc)
		for _, banned := range []string{"endpoint", "/v1/", " sdk", "204", "http", "uemgroups", "payload"} {
			if strings.Contains(lower, banned) {
				t.Errorf("description contains wire vocabulary %q:\n%s", banned, desc)
			}
		}
	}
}
