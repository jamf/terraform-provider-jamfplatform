// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamfprotectactions

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

func TestSyncPlansAction_Metadata(t *testing.T) {
	a := NewSyncPlansAction()
	req := action.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp action.MetadataResponse
	a.(*SyncPlansAction).Metadata(context.Background(), req, &resp)

	const want = "jamfplatform_pro_jamf_protect_plans_sync"
	if resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

func TestSyncPlansAction_Schema(t *testing.T) {
	a := NewSyncPlansAction()
	var resp action.SchemaResponse
	a.(*SyncPlansAction).Schema(context.Background(), action.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes[actiontimeouts.AttributeName]; len(resp.Schema.Attributes) != 1 || !ok {
		t.Errorf("sync plans action takes no input beyond timeouts; got %d attributes", len(resp.Schema.Attributes))
	}
}

// --- deployment retry ---

func TestRetryDeploymentAction_Metadata(t *testing.T) {
	a := NewRetryDeploymentAction()
	req := action.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp action.MetadataResponse
	a.(*RetryDeploymentAction).Metadata(context.Background(), req, &resp)

	const want = "jamfplatform_pro_jamf_protect_deployment_retry"
	if resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

func TestRetryDeploymentAction_Schema(t *testing.T) {
	a := NewRetryDeploymentAction()
	var resp action.SchemaResponse
	a.(*RetryDeploymentAction).Schema(context.Background(), action.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	want := []string{"deployment_id", "serial_number", "management_id", "udid", "task_ids", "all_failed", "only_failed"}
	for _, name := range want {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if attr, ok := resp.Schema.Attributes["deployment_id"]; !ok || !attr.IsRequired() {
		t.Errorf("deployment_id must be required")
	}
}

func TestRetryDeploymentAction_ConfigValidatorsWired(t *testing.T) {
	a := NewRetryDeploymentAction()
	validators := a.(*RetryDeploymentAction).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("expected exactly one ConfigValidator (exactly-one-target), got %d", len(validators))
	}
}

// --- exactlyOneTargetValidator ---

// retryValidatorSchema is a minimal stand-in schema carrying the target-mode
// attributes the validator reads, so a tfsdk.Config can be hand-built.
var retryValidatorSchema = actionschema.Schema{Attributes: actiontimeouts.Add(context.Background(), map[string]actionschema.Attribute{
	"deployment_id": actionschema.StringAttribute{Optional: true},
	"serial_number": actionschema.StringAttribute{Optional: true},
	"management_id": actionschema.StringAttribute{Optional: true},
	"udid":          actionschema.StringAttribute{Optional: true},
	"task_ids":      actionschema.ListAttribute{ElementType: types.StringType, Optional: true},
	"all_failed":    actionschema.BoolAttribute{Optional: true},
	"only_failed":   actionschema.BoolAttribute{Optional: true},
})}

var retryObjType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"deployment_id": tftypes.String,
	"serial_number": tftypes.String,
	"management_id": tftypes.String,
	"udid":          tftypes.String,
	"task_ids":      tftypes.List{ElementType: tftypes.String},
	"all_failed":    tftypes.Bool,
	"only_failed":   tftypes.Bool,
	"timeouts":      tftypes.Object{AttributeTypes: map[string]tftypes.Type{"invoke": tftypes.String}},
}}

func strV(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
func strN() tftypes.Value         { return tftypes.NewValue(tftypes.String, nil) }
func boolV(b bool) tftypes.Value  { return tftypes.NewValue(tftypes.Bool, b) }
func boolN() tftypes.Value        { return tftypes.NewValue(tftypes.Bool, nil) }
func listN() tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil)
}
func listV(items ...string) tftypes.Value {
	vals := make([]tftypes.Value, len(items))
	for i, it := range items {
		vals[i] = tftypes.NewValue(tftypes.String, it)
	}
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, vals)
}

func runRetryValidator(serial, management, udid, allFailed, taskIDs tftypes.Value) bool {
	cfg := tfsdk.Config{
		Schema: retryValidatorSchema,
		Raw: tftypes.NewValue(retryObjType, map[string]tftypes.Value{
			"deployment_id": strV("dep-uuid"),
			"serial_number": serial,
			"management_id": management,
			"udid":          udid,
			"task_ids":      taskIDs,
			"all_failed":    allFailed,
			"only_failed":   boolN(),
			"timeouts":      tftypes.NewValue(retryObjType.AttributeTypes["timeouts"], nil),
		}),
	}
	var resp action.ValidateConfigResponse
	exactlyOneTargetValidator{}.ValidateAction(
		context.Background(),
		action.ValidateConfigRequest{Config: cfg},
		&resp,
	)
	return resp.Diagnostics.HasError()
}

func TestExactlyOneTargetValidator(t *testing.T) {
	cases := []struct {
		name                                         string
		serial, management, udid, allFailed, taskIDs tftypes.Value
		wantErr                                      bool
	}{
		{"none", strN(), strN(), strN(), boolN(), listN(), true},
		{"serial_only", strV("C02X"), strN(), strN(), boolN(), listN(), false},
		{"management_only", strN(), strV("uuid-1"), strN(), boolN(), listN(), false},
		{"udid_only", strN(), strN(), strV("UDID-1"), boolN(), listN(), false},
		{"task_ids_only", strN(), strN(), strN(), boolN(), listV("82", "83"), false},
		{"all_failed_only", strN(), strN(), strN(), boolV(true), listN(), false},
		{"all_failed_false_is_no_mode", strN(), strN(), strN(), boolV(false), listN(), true},
		{"serial_and_all_failed", strV("C02X"), strN(), strN(), boolV(true), listN(), true},
		{"udid_and_task_ids", strN(), strN(), strV("UDID-1"), boolN(), listV("82"), true},
		{"task_ids_and_all_failed", strN(), strN(), strN(), boolV(true), listV("82"), true},
		{"empty_task_ids_is_no_mode", strN(), strN(), strN(), boolN(), listV(), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runRetryValidator(c.serial, c.management, c.udid, c.allFailed, c.taskIDs); got != c.wantErr {
				t.Errorf("hasError = %v, want %v", got, c.wantErr)
			}
		})
	}
}
