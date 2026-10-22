// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platformenvironmentaction

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func assignSchema(t *testing.T) actionschema.Schema {
	t.Helper()
	var resp action.SchemaResponse
	NewAssignTenantsAction().(*AssignTenantsAction).Schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestMetadata(t *testing.T) {
	var resp action.MetadataResponse
	NewAssignTenantsAction().(*AssignTenantsAction).Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if want := "jamfplatform_account_platform_environment_assign_tenants"; resp.TypeName != want {
		t.Errorf("type name = %q, want %q", resp.TypeName, want)
	}
}

func TestAttributes(t *testing.T) {
	s := assignSchema(t)

	for _, name := range []string{"environment_id", "tenant_ids"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing attribute %s", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
	if _, ok := s.Attributes["timeouts"]; !ok {
		t.Error("missing the shared timeouts attribute")
	}
	if len(s.Attributes) != 3 {
		t.Errorf("schema declares %d attributes, want environment_id, tenant_ids and timeouts", len(s.Attributes))
	}
}

func TestTenantIDsRejectEmptyAndBlankMembers(t *testing.T) {
	s := assignSchema(t)
	attr := s.Attributes["tenant_ids"].(actionschema.SetAttribute)

	run := func(elements ...string) int {
		values := make([]types.String, 0, len(elements))
		for _, e := range elements {
			values = append(values, types.StringValue(e))
		}
		set, _ := types.SetValueFrom(context.Background(), types.StringType, values)
		errors := 0
		for _, v := range attr.Validators {
			resp := &validator.SetResponse{}
			v.ValidateSet(context.Background(), validator.SetRequest{Path: path.Root("tenant_ids"), ConfigValue: set}, resp)
			errors += resp.Diagnostics.ErrorsCount()
		}
		return errors
	}

	if run("t-1", "t-2") != 0 {
		t.Error("a valid set was refused")
	}
	if run() == 0 {
		t.Error("an empty set must be refused at plan time: the server answers it with an unattributed error")
	}
	if run("t-1", "") == 0 {
		t.Error("an empty identifier must be refused")
	}
}

func TestDescriptionStatesTheAssignmentIsPermanent(t *testing.T) {
	description := strings.TrimSuffix(assignSchema(t).MarkdownDescription, assignTenantsPrivileges)
	for _, want := range []string{"cannot be undone", "cannot be deleted"} {
		if !strings.Contains(description, want) {
			t.Errorf("the description does not say %q", want)
		}
	}
}

func TestDescriptionCarriesNoWireVocabulary(t *testing.T) {
	description := strings.TrimSuffix(assignSchema(t).MarkdownDescription, assignTenantsPrivileges)
	for _, banned := range []string{"AssignTenants", "tenantIds", "204", "endpoint", "SDK", "HTTP"} {
		if strings.Contains(description, banned) {
			t.Errorf("the description mentions %q", banned)
		}
	}
}
