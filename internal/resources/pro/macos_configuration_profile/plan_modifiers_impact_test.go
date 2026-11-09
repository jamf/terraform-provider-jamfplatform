// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// impactTestModel builds a minimal all-computers profile model that survives
// a schema round-trip.
func impactTestModel(payload string) ResourceModel {
	nullSet := types.SetNull(types.StringType)
	return ResourceModel{
		ID: types.StringValue("1"),
		General: &GeneralModel{
			ID:       types.StringValue("1"),
			Name:     types.StringValue("impact-test"),
			Payloads: types.StringValue(payload),
		},
		Scope: &scope.ComputerScopeModel{
			Targets: &scope.ComputerScopeTargetsModel{
				AllComputers:     types.BoolValue(true),
				AllJssUsers:      types.BoolValue(false),
				ComputerIDs:      nullSet,
				ComputerGroupIDs: nullSet,
				BuildingIDs:      nullSet,
				DepartmentIDs:    nullSet,
				UserIDs:          nullSet,
				UserGroupIDs:     nullSet,
			},
		},
		Timeouts: helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes),
	}
}

// impactTestSource is a minimal impact.Source: a small managed estate with no
// groups, enough for an all-devices scope to resolve to an exact count
// without HTTP.
type impactTestSource struct{}

func (impactTestSource) Groups(context.Context) ([]impact.Group, error) { return nil, nil }
func (impactTestSource) Totals(context.Context) (impact.Totals, error) {
	return impact.Totals{ManagedComputers: 5, ManagedMobileDevices: 3}, nil
}
func (impactTestSource) Members(context.Context, string) ([]string, error) { return nil, nil }
func (impactTestSource) ComputerManagementIDs(context.Context, string) ([]string, error) {
	return nil, nil
}
func (impactTestSource) MobileManagementIDs(context.Context, string) ([]string, error) {
	return nil, nil
}
func (impactTestSource) PlaceNames(context.Context) (impact.Places, error) {
	return impact.Places{}, nil
}

// The A/B payload pair is byte-different but semantically equal: they differ
// only in PayloadUUID, which MaskPayload strips before comparison. The
// "changed" variant bumps PayloadVersion — a real edit that must survive the
// self-heal and keep the plan a genuine update.
const (
	impactPayloadA = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadUUID</key><string>AAAABBBB-1111-2222-3333-444455556666</string>
<key>PayloadIdentifier</key><string>com.example.impact-test</string>
<key>PayloadDisplayName</key><string>Impact Test</string>
<key>PayloadContent</key><array/>
</dict></plist>`
	impactPayloadB = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadUUID</key><string>CCCCDDDD-1111-2222-3333-444455556666</string>
<key>PayloadIdentifier</key><string>com.example.impact-test</string>
<key>PayloadDisplayName</key><string>Impact Test</string>
<key>PayloadContent</key><array/>
</dict></plist>`
	impactPayloadChanged = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>2</integer>
<key>PayloadUUID</key><string>AAAABBBB-1111-2222-3333-444455556666</string>
<key>PayloadIdentifier</key><string>com.example.impact-test</string>
<key>PayloadDisplayName</key><string>Impact Test</string>
<key>PayloadContent</key><array/>
</dict></plist>`
)

// runImpactModifyPlan drives ModifyPlan the way the framework does: resp.Plan
// pre-populated from req.Plan, private state empty (so the payload decision
// takes the two-way fallback — the post-import shape the finding reproduced),
// impact reporting enabled through a stub source.
func runImpactModifyPlan(t *testing.T, stateModel, planModel *ResourceModel) *resource.ModifyPlanResponse {
	t.Helper()
	ctx := context.Background()

	var sresp resource.SchemaResponse
	NewResource().Schema(ctx, resource.SchemaRequest{}, &sresp)
	if sresp.Diagnostics.HasError() {
		t.Fatalf("schema diags: %v", sresp.Diagnostics)
	}
	nullRaw := tftypes.NewValue(sresp.Schema.Type().TerraformType(ctx), nil)

	state := tfsdk.State{Schema: sresp.Schema, Raw: nullRaw}
	if stateModel != nil {
		if diags := state.Set(ctx, stateModel); diags.HasError() {
			t.Fatalf("state set: %v", diags)
		}
	}
	plan := tfsdk.Plan{Schema: sresp.Schema, Raw: nullRaw}
	if planModel != nil {
		if diags := plan.Set(ctx, planModel); diags.HasError() {
			t.Fatalf("plan set: %v", diags)
		}
	}

	r := &Resource{impact: impact.NewCache(impactTestSource{})}
	req := resource.ModifyPlanRequest{Plan: plan, State: state}
	resp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan diags: %v", resp.Diagnostics)
	}
	return resp
}

func hasImpactAlert(resp *resource.ModifyPlanResponse) bool {
	for _, d := range resp.Diagnostics {
		if strings.Contains(d.Summary(), "Impact alert") {
			return true
		}
	}
	return false
}

// A byte-different but semantically-equal payload self-heals the plan back to
// state — a no-op. The impact alert must stay silent, or `terraform plan`
// prints "Impact alert — … will receive the updated configuration profile"
// directly above "No changes".
func TestModifyPlanImpactAlertSkippedWhenPayloadSelfHeals(t *testing.T) {
	stateModel := impactTestModel(impactPayloadA)
	planModel := impactTestModel(impactPayloadB)
	resp := runImpactModifyPlan(t, &stateModel, &planModel)

	// Guard the test's own premise: the self-heal must have rewritten the
	// planned payload back to state.
	var healed ResourceModel
	if diags := resp.Plan.Get(context.Background(), &healed); diags.HasError() {
		t.Fatalf("healed plan get: %v", diags)
	}
	if healed.General.Payloads.ValueString() != impactPayloadA {
		t.Fatal("expected the plan to self-heal the payload back to state")
	}
	if hasImpactAlert(resp) {
		t.Fatal("impact alert emitted for a plan that self-healed to a no-op")
	}
}

// A genuine payload edit survives the self-heal and must still alert.
func TestModifyPlanImpactAlertOnGenuinePayloadChange(t *testing.T) {
	stateModel := impactTestModel(impactPayloadA)
	planModel := impactTestModel(impactPayloadChanged)
	resp := runImpactModifyPlan(t, &stateModel, &planModel)
	if !hasImpactAlert(resp) {
		t.Fatal("no impact alert for a genuine payload change")
	}
}

// A scope change with identical payload bytes must still alert.
func TestModifyPlanImpactAlertOnScopeChange(t *testing.T) {
	stateModel := impactTestModel(impactPayloadA)
	stateModel.Scope.Targets.AllComputers = types.BoolValue(false)
	planModel := impactTestModel(impactPayloadA)
	resp := runImpactModifyPlan(t, &stateModel, &planModel)
	if !hasImpactAlert(resp) {
		t.Fatal("no impact alert for a genuine scope change")
	}
}

// Creates and destroys skip the payload compare entirely, so the alert must
// be emitted before those early returns.
func TestModifyPlanImpactAlertOnCreateAndDestroy(t *testing.T) {
	model := impactTestModel(impactPayloadA)
	if resp := runImpactModifyPlan(t, nil, &model); !hasImpactAlert(resp) {
		t.Fatal("no impact alert on create")
	}
	if resp := runImpactModifyPlan(t, &model, nil); !hasImpactAlert(resp) {
		t.Fatal("no impact alert on destroy")
	}
}

// An imported profile that was never in Self Service holds a null
// self_service_display_name while its self_service block exists. A payload edit
// writes the block and Jamf Pro defaults the name, so the plan must say
// "known after apply" (issue 560). With no write the plan stays empty.
func TestModifyPlanSelfServiceDisplayNameUnknownOnRealWrite(t *testing.T) {
	ctx := context.Background()
	withSS := func(payload string) ResourceModel {
		m := impactTestModel(payload)
		m.SelfService = &SelfServiceModel{
			SelfServiceDisplayName:     types.StringNull(),
			InstallButtonText:          types.StringValue("Install"),
			SelfServiceDescription:     types.StringNull(),
			EnsureUsersViewDescription: types.BoolNull(),
			FeatureOnMainPage:          types.BoolNull(),
			DisplayNotifications:       types.BoolNull(),
			NotificationLocation:       types.StringValue("Self Service"),
			NotificationSubject:        types.StringNull(),
			NotificationMessage:        types.StringNull(),
			RemovalDisallowed:          types.StringValue("Never"),
		}
		return m
	}
	run := func(t *testing.T, payload string) types.String {
		t.Helper()
		var sresp resource.SchemaResponse
		NewResource().Schema(ctx, resource.SchemaRequest{}, &sresp)
		stateModel, planModel := withSS(impactPayloadA), withSS(payload)
		state := tfsdk.State{Schema: sresp.Schema}
		plan := tfsdk.Plan{Schema: sresp.Schema}
		config := tfsdk.Config{Schema: sresp.Schema}
		if d := state.Set(ctx, &stateModel); d.HasError() {
			t.Fatal(d)
		}
		if d := plan.Set(ctx, &planModel); d.HasError() {
			t.Fatal(d)
		}
		config.Raw = plan.Raw
		r := &Resource{impact: impact.NewCache(impactTestSource{})}
		resp := &resource.ModifyPlanResponse{Plan: plan}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: config, Plan: plan, State: state}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		var got types.String
		if d := resp.Plan.GetAttribute(ctx, path.Root("self_service").AtName("self_service_display_name"), &got); d.HasError() {
			t.Fatal(d)
		}
		return got
	}

	if got := run(t, impactPayloadChanged); !got.IsUnknown() {
		t.Fatalf("real write: self_service_display_name = %v, want unknown", got)
	}
	if got := run(t, impactPayloadB); !got.IsNull() {
		t.Fatalf("self-healed no-op: self_service_display_name = %v, want null", got)
	}
}
