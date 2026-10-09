// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

func schemaOf(t *testing.T, a action.Action) action.SchemaResponse {
	t.Helper()
	var resp action.SchemaResponse
	a.Schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp
}

func metadataOf(t *testing.T, a action.Action) string {
	t.Helper()
	var resp action.MetadataResponse
	a.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	return resp.TypeName
}

func TestActionTypeNames(t *testing.T) {
	for want, a := range map[string]action.Action{
		"jamfplatform_pro_retry_app_installer_installations":     NewRetryInstallationsAction(),
		"jamfplatform_pro_retry_all_app_installer_installations": NewRetryAllInstallationsAction(),
		"jamfplatform_pro_update_app_installer_version":          NewUpdateVersionAction(),
	} {
		if got := metadataOf(t, a); got != want {
			t.Errorf("expected type name %q, got %q", want, got)
		}
	}
}

func TestRetryInstallationsAction_Schema(t *testing.T) {
	s := schemaOf(t, NewRetryInstallationsAction()).Schema

	// deployment_id is Required deliberately: the tenant-wide retry is its own
	// action, so forgetting the ID here cannot silently widen the blast radius.
	dep, ok := s.Attributes["deployment_id"]
	if !ok {
		t.Fatal("missing deployment_id")
	}
	if !dep.IsRequired() {
		t.Error("deployment_id must be Required so the tenant-wide retry cannot be reached by omission")
	}

	ids, ok := s.Attributes["computer_ids"]
	if !ok {
		t.Fatal("missing computer_ids")
	}
	if !ids.IsOptional() {
		t.Error("computer_ids must be Optional")
	}
}

// The tenant-wide retry takes no arguments; an attribute would imply it could be
// narrowed, which it cannot.
func TestRetryAllInstallationsAction_TakesNoArguments(t *testing.T) {
	s := schemaOf(t, NewRetryAllInstallationsAction()).Schema
	if _, ok := s.Attributes[actiontimeouts.AttributeName]; len(s.Attributes) != 1 || !ok {
		t.Errorf("expected only the timeouts attribute, got %v", s.Attributes)
	}
}

func TestUpdateVersionAction_Schema(t *testing.T) {
	s := schemaOf(t, NewUpdateVersionAction()).Schema
	for _, name := range []string{"deployment_id", "version"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing attribute %q", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("%s must be Required", name)
		}
	}
}

// Every action must implement Configure, or Invoke runs with a nil client.
func TestActionsImplementConfigure(t *testing.T) {
	for _, a := range []action.Action{
		NewRetryInstallationsAction(),
		NewRetryAllInstallationsAction(),
		NewUpdateVersionAction(),
	} {
		if _, ok := a.(action.ActionWithConfigure); !ok {
			t.Errorf("%T does not implement action.ActionWithConfigure", a)
		}
	}
}
