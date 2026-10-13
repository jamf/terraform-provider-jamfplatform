// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func deferralPlanRequest(ctx context.Context, planned, prior any) resource.ModifyPlanRequest {
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"user_interaction": schema.SingleNestedAttribute{
			Optional:   true,
			Attributes: map[string]schema.Attribute{"deferral_until_utc": schema.StringAttribute{Optional: true}},
		},
	}}
	obj := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"deferral_until_utc": tftypes.String}}
	root := s.Type().TerraformType(ctx).(tftypes.Object)
	build := func(v any) tftypes.Value {
		return tftypes.NewValue(root, map[string]tftypes.Value{"user_interaction": tftypes.NewValue(obj, map[string]tftypes.Value{
			"deferral_until_utc": tftypes.NewValue(tftypes.String, v),
		})})
	}
	req := resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: build(planned)},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(root, nil)},
	}
	if prior != "no-state" {
		req.State.Raw = build(prior)
	}
	return req
}

func TestValidateDeferralDateIsFuture(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	const past, future = "2026-10-13T11:00:00.000+0000", "2026-10-13T13:00:00.000+0000"
	for _, tc := range []struct {
		name           string
		planned, prior any
		wantErr        bool
	}{
		{"new past date on create is refused", past, "no-state", true},
		{"new future date on create is accepted", future, "no-state", false},
		{"changed to a past date is refused", past, future, true},
		{"unchanged past date is legal: it passed after it was set", past, past, false},
		{"no date configured is left alone", nil, past, false},
		{"unparseable date is left to the format validator", "tomorrow", "no-state", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := resource.ModifyPlanResponse{}
			validateDeferralDateIsFuture(ctx, deferralPlanRequest(ctx, tc.planned, tc.prior), &resp, now)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (%v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestOmitUnchangedDeferralDate(t *testing.T) {
	const day1, day2 = "2026-10-13T11:00:00.000+0000", "2027-01-01T00:00:00.000+0000"
	model := func(kind, until string) *PolicyResourceModel {
		return &PolicyResourceModel{UserInteraction: &PolicyUserInteractionModel{
			DeferralType: types.StringValue(kind), DeferralUntilUtc: types.StringValue(until),
		}}
	}
	for _, tc := range []struct {
		name        string
		plan, state *PolicyResourceModel
		wantSent    bool
	}{
		{"unchanged date is not sent", model("date", day1), model("date", day1), false},
		{"changed date is sent", model("date", day2), model("date", day1), true},
		{"switch to date from duration is sent", model("date", day1), model("duration", ""), true},
		{"duration is never touched", model("duration", ""), model("duration", ""), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			yes, until, mins := true, tc.plan.UserInteraction.DeferralUntilUtc.ValueString(), 0
			input := &proclassic.PolicyPost{UserInteraction: &proclassic.PolicyPostUserInteraction{
				AllowUsersToDefer: &yes, AllowDeferralUntilUtc: &until, AllowDeferralMinutes: &mins,
			}}
			omitUnchangedDeferralDate(input, tc.plan, tc.state)
			if sent := input.UserInteraction.AllowDeferralUntilUtc != nil; sent != tc.wantSent {
				t.Fatalf("date sent = %v, want %v", sent, tc.wantSent)
			}
			if sent := input.UserInteraction.AllowUsersToDefer != nil; sent != tc.wantSent {
				t.Errorf("allow_users_to_defer sent = %v, want %v", sent, tc.wantSent)
			}
		})
	}
}
