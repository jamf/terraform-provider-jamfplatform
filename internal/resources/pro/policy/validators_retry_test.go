// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func retrySchema() schema.Schema {
	return schema.Schema{Attributes: map[string]schema.Attribute{
		"general": schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"retry_event":    schema.StringAttribute{Optional: true},
				"retry_attempts": schema.Int64Attribute{Optional: true},
			},
		},
	}}
}

func retryConfig(ctx context.Context, event, attempts any) (tfsdk.Config, tftypes.Object) {
	s := retrySchema()
	obj := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"retry_event": tftypes.String, "retry_attempts": tftypes.Number}}
	root := s.Type().TerraformType(ctx).(tftypes.Object)
	raw := tftypes.NewValue(root, map[string]tftypes.Value{"general": tftypes.NewValue(obj, map[string]tftypes.Value{
		"retry_event":    tftypes.NewValue(tftypes.String, event),
		"retry_attempts": tftypes.NewValue(tftypes.Number, attempts),
	})})
	return tfsdk.Config{Schema: s, Raw: raw}, root
}

func TestRetryAttemptsMatchEvent(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		event    any
		attempts any
		wantErr  bool
	}{
		{"none with 5 reads back as -1", "none", 5, true},
		{"none with -1 is the stored value", "none", -1, false},
		{"check-in with -1 is refused", "check-in", -1, true},
		{"check-in with 0 is refused", "check-in", 0, true},
		{"check-in with 11 is refused", "check-in", 11, true},
		{"trigger with 10 is accepted", "trigger", 10, false},
		{"check-in with 1 is accepted", "check-in", 1, false},
		{"attempts alone is left alone", nil, 5, false},
		{"event alone is left alone", "check-in", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := retryConfig(ctx, tc.event, tc.attempts)
			resp := resource.ValidateConfigResponse{}
			retryAttemptsMatchEventValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: cfg}, &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (%v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestValidateRetryAttemptsOnCreate(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		event    any
		attempts any
		create   bool
		wantErr  bool
	}{
		{"event without attempts on create is refused", "check-in", nil, true, true},
		{"event with attempts on create is accepted", "trigger", 3, true, false},
		{"event none needs no attempts", "none", nil, true, false},
		{"no event configured is left alone", nil, nil, true, false},
		{"event without attempts on update may be carried from state", "check-in", nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, root := retryConfig(ctx, tc.event, tc.attempts)
			req := resource.ModifyPlanRequest{
				Config: cfg,
				Plan:   tfsdk.Plan(cfg),
				State:  tfsdk.State(cfg),
			}
			if tc.create {
				req.State.Raw = tftypes.NewValue(root, nil)
			}
			resp := resource.ModifyPlanResponse{}
			validateRetryAttemptsOnCreate(ctx, req, &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (%v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
