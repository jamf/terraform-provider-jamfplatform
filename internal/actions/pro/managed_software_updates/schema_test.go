// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

// --- Plan action ---

func TestPlanAction_Metadata(t *testing.T) {
	a := NewPlanAction()
	req := action.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp action.MetadataResponse
	a.(*PlanAction).Metadata(context.Background(), req, &resp)

	const want = "jamfplatform_pro_managed_software_update_plan"
	if resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

func TestPlanAction_Schema(t *testing.T) {
	a := NewPlanAction()
	var resp action.SchemaResponse
	a.(*PlanAction).Schema(context.Background(), action.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	want := []string{
		"group_id", "object_type", "update_action", "version_type",
		"specific_version", "build_version", "force_install_local_date_time", "max_deferrals",
	}
	for _, name := range want {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	for _, name := range []string{"group_id", "object_type", "update_action", "version_type"} {
		if !resp.Schema.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
}

func TestPlanAction_ConfigValidatorsWired(t *testing.T) {
	a := NewPlanAction()
	validators := a.(*PlanAction).ConfigValidators(context.Background())
	if len(validators) != 2 {
		t.Fatalf("expected two ConfigValidators (specific_version required, build_version CUSTOM_VERSION-only), got %d", len(validators))
	}

	var haveSpecificVersion, haveBuildVersion bool
	for _, v := range validators {
		switch v.(type) {
		case specificVersionRequiredValidator:
			haveSpecificVersion = true
		case buildVersionCustomOnlyValidator:
			haveBuildVersion = true
		}
	}
	if !haveSpecificVersion {
		t.Error("specificVersionRequiredValidator not wired")
	}
	if !haveBuildVersion {
		t.Error("buildVersionCustomOnlyValidator not wired")
	}
}

// --- Abandon action ---

func TestAbandonFeatureToggleAction_Metadata(t *testing.T) {
	a := NewAbandonFeatureToggleAction()
	req := action.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp action.MetadataResponse
	a.(*AbandonFeatureToggleAction).Metadata(context.Background(), req, &resp)

	const want = "jamfplatform_pro_managed_software_update_abandon"
	if resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

func TestAbandonFeatureToggleAction_Schema(t *testing.T) {
	a := NewAbandonFeatureToggleAction()
	var resp action.SchemaResponse
	a.(*AbandonFeatureToggleAction).Schema(context.Background(), action.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes[actiontimeouts.AttributeName]; len(resp.Schema.Attributes) != 1 || !ok {
		t.Errorf("abandon action takes no input beyond timeouts; got %d attributes", len(resp.Schema.Attributes))
	}
}

// --- specificVersionRequiredValidator ---

// validatorConfigSchema is a minimal stand-in schema carrying just the two attributes the
// validator reads, so a tfsdk.Config can be hand-built for unit testing.
var validatorConfigSchema = schema.Schema{Attributes: map[string]schema.Attribute{
	"version_type":     schema.StringAttribute{Optional: true},
	"specific_version": schema.StringAttribute{Optional: true},
}}

var validatorObjType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"version_type":     tftypes.String,
	"specific_version": tftypes.String,
}}

func strVal(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
func strNull() tftypes.Value        { return tftypes.NewValue(tftypes.String, nil) }
func strUnknown() tftypes.Value     { return tftypes.NewValue(tftypes.String, tftypes.UnknownValue) }

func runSpecificVersionValidator(versionType, specificVersion tftypes.Value) bool {
	cfg := tfsdk.Config{
		Schema: validatorConfigSchema,
		Raw: tftypes.NewValue(validatorObjType, map[string]tftypes.Value{
			"version_type":     versionType,
			"specific_version": specificVersion,
		}),
	}
	var resp action.ValidateConfigResponse
	specificVersionRequiredValidator{}.ValidateAction(
		context.Background(),
		action.ValidateConfigRequest{Config: cfg},
		&resp,
	)
	return resp.Diagnostics.HasError()
}

func TestSpecificVersionRequiredValidator(t *testing.T) {
	cases := []struct {
		name            string
		versionType     tftypes.Value
		specificVersion tftypes.Value
		wantErr         bool
	}{
		{"specific_missing_version", strVal("SPECIFIC_VERSION"), strNull(), true},
		{"custom_missing_version", strVal("CUSTOM_VERSION"), strNull(), true},
		{"specific_empty_version", strVal("SPECIFIC_VERSION"), strVal(""), true},
		{"specific_with_version", strVal("SPECIFIC_VERSION"), strVal("15.1"), false},
		{"custom_with_version", strVal("CUSTOM_VERSION"), strVal("15.1.1"), false},
		{"latest_any_no_version", strVal("LATEST_ANY"), strNull(), false},
		{"latest_major_no_version", strVal("LATEST_MAJOR"), strNull(), false},
		{"unknown_version_type_defers", strUnknown(), strNull(), false},
		{"unknown_specific_version_defers", strVal("SPECIFIC_VERSION"), strUnknown(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runSpecificVersionValidator(c.versionType, c.specificVersion); got != c.wantErr {
				t.Errorf("hasError = %v, want %v", got, c.wantErr)
			}
		})
	}
}
