// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestValidateDeviceGroupPlan_SmartGroup(t *testing.T) {
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue("smart"),
		Criteria: []DeviceGroupCriteriaModel{
			{
				AttributeName:  types.StringValue("Device Name"),
				Operator:       types.StringValue("like"),
				AttributeValue: types.StringValue("Mac"),
				JoinType:       types.StringValue("and"),
			},
		},
		Members: types.SetNull(types.StringType),
	}

	if err := validateDeviceGroupPlan(plan); err != nil {
		t.Errorf("expected no error for valid smart group, got: %v", err)
	}
}

func TestValidateDeviceGroupPlan_SmartGroupNoCriteria(t *testing.T) {
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue("smart"),
		Criteria:  nil,
		Members:   types.SetNull(types.StringType),
	}

	if err := validateDeviceGroupPlan(plan); err != nil {
		t.Errorf("expected nil error for smart group with no criteria, got: %v", err)
	}
}

func TestValidateDeviceGroupPlan_SmartGroupWithMembers(t *testing.T) {
	members, _ := types.SetValueFrom(context.TODO(), types.StringType, []string{"device-1"})
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue("smart"),
		Criteria: []DeviceGroupCriteriaModel{
			{
				AttributeName:  types.StringValue("Device Name"),
				Operator:       types.StringValue("like"),
				AttributeValue: types.StringValue("Mac"),
			},
		},
		Members: members,
	}

	err := validateDeviceGroupPlan(plan)
	if err == nil {
		t.Fatal("expected error when smart group has members")
	}
	if err.Error() != "members cannot be set for smart groups" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDeviceGroupPlan_StaticGroup(t *testing.T) {
	members, _ := types.SetValueFrom(context.TODO(), types.StringType, []string{"device-1"})
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue("static"),
		Members:   members,
	}

	if err := validateDeviceGroupPlan(plan); err != nil {
		t.Errorf("expected no error for valid static group, got: %v", err)
	}
}

func TestValidateDeviceGroupPlan_StaticGroupWithCriteria(t *testing.T) {
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue("static"),
		Criteria: []DeviceGroupCriteriaModel{
			{
				AttributeName:  types.StringValue("Device Name"),
				Operator:       types.StringValue("like"),
				AttributeValue: types.StringValue("Mac"),
			},
		},
		Members: types.SetNull(types.StringType),
	}

	err := validateDeviceGroupPlan(plan)
	if err == nil {
		t.Fatal("expected error when static group has criteria")
	}
	if err.Error() != "criteria cannot be set for static groups" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDeviceGroupPlan_UnsupportedGroupType(t *testing.T) {
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue("dynamic"),
		Members:   types.SetNull(types.StringType),
	}

	err := validateDeviceGroupPlan(plan)
	if err == nil {
		t.Fatal("expected error for unsupported group type")
	}
	if err.Error() != "unsupported group_type \"dynamic\"" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDeviceGroupPlan_EmptyGroupType(t *testing.T) {
	plan := &DeviceGroupResourceModel{
		GroupType: types.StringValue(""),
		Members:   types.SetNull(types.StringType),
	}

	err := validateDeviceGroupPlan(plan)
	if err == nil {
		t.Fatal("expected error for empty group type")
	}
	if err.Error() != "group_type must be provided" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDiffStringSlices(t *testing.T) {
	tests := []struct {
		name        string
		current     []string
		desired     []string
		wantAdded   []string
		wantRemoved []string
	}{
		{
			name:        "no changes",
			current:     []string{"a", "b", "c"},
			desired:     []string{"a", "b", "c"},
			wantAdded:   nil,
			wantRemoved: nil,
		},
		{
			name:        "additions only",
			current:     []string{"a"},
			desired:     []string{"a", "b", "c"},
			wantAdded:   []string{"b", "c"},
			wantRemoved: nil,
		},
		{
			name:        "removals only",
			current:     []string{"a", "b", "c"},
			desired:     []string{"a"},
			wantAdded:   nil,
			wantRemoved: []string{"b", "c"},
		},
		{
			name:        "mixed changes",
			current:     []string{"a", "b"},
			desired:     []string{"b", "c"},
			wantAdded:   []string{"c"},
			wantRemoved: []string{"a"},
		},
		{
			name:        "empty to populated",
			current:     nil,
			desired:     []string{"a", "b"},
			wantAdded:   []string{"a", "b"},
			wantRemoved: nil,
		},
		{
			name:        "populated to empty",
			current:     []string{"a", "b"},
			desired:     nil,
			wantAdded:   nil,
			wantRemoved: []string{"a", "b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			added, removed := diffStringSlices(tc.current, tc.desired)

			if len(added) != len(tc.wantAdded) {
				t.Errorf("added: got %v, want %v", added, tc.wantAdded)
			} else {
				for i := range added {
					if added[i] != tc.wantAdded[i] {
						t.Errorf("added[%d]: got %q, want %q", i, added[i], tc.wantAdded[i])
					}
				}
			}

			if len(removed) != len(tc.wantRemoved) {
				t.Errorf("removed: got %v, want %v", removed, tc.wantRemoved)
			} else {
				for i := range removed {
					if removed[i] != tc.wantRemoved[i] {
						t.Errorf("removed[%d]: got %q, want %q", i, removed[i], tc.wantRemoved[i])
					}
				}
			}
		})
	}
}

func TestImportHydration(t *testing.T) {
	cases := []struct {
		name        string
		stateAbsent bool
		groupName   types.String
		want        bool
	}{
		{"a post-import read carries the id and nothing else", false, types.StringNull(), true},
		{"an identity-only refresh has no state at all", true, types.StringNull(), true},
		{"an ordinary refresh of a managed group", false, types.StringValue("tf-acc-group"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := importHydration(tc.stateAbsent, tc.groupName); got != tc.want {
				t.Errorf("importHydration = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestManageMembersOnRead(t *testing.T) {
	declared, diags := types.SetValueFrom(context.Background(), types.StringType, []string{"device-uuid"})
	if diags.HasError() {
		t.Fatalf("building the declared member set: %v", diags)
	}
	declaredEmpty, diags := types.SetValueFrom(context.Background(), types.StringType, []string{})
	if diags.HasError() {
		t.Fatalf("building the empty member set: %v", diags)
	}

	cases := []struct {
		name      string
		hydrating bool
		prior     types.Set
		members   []string
		want      bool
	}{
		{"declared members are owned", false, declared, []string{"device-uuid"}, true},
		{"a declared empty set is owned", false, declaredEmpty, nil, true},
		{"an import adopts the membership a group has", true, types.SetNull(types.StringType), []string{"device-uuid"}, true},
		{"an import leaves an empty group unmanaged", true, types.SetNull(types.StringType), nil, false},
		{"an undeclared set stays unmanaged on refresh", false, types.SetNull(types.StringType), []string{"device-uuid"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := manageMembersOnRead(tc.hydrating, tc.prior, tc.members); got != tc.want {
				t.Errorf("manageMembersOnRead = %v, want %v", got, tc.want)
			}
		})
	}
}

// readTestGroupID is the identifier the import-hydration Read tests below pass
// through, matching the UUID shape the platform mints.
const readTestGroupID = "9c43905e-d220-4f17-a5e1-869f7440e979"

// deviceGroupReadClient returns a device-group client pointed at a stub server
// answering the group read with grp and the member read with members. The seam is
// the HTTP boundary rather than an injected interface, matching
// pro_id_resolver_test.go, whose mock client this reuses.
func deviceGroupReadClient(t *testing.T, grp map[string]any, members []string) *devicegroups.Client {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/members") {
			_ = json.NewEncoder(w).Encode(map[string]any{"totalCount": len(members), "results": members})
			return
		}
		_ = json.NewEncoder(w).Encode(grp)
	})
	return devicegroups.New(newProIDMockClient(t, handler))
}

// readAfterImport drives Read with the state Terraform hands a resource after an
// import: the framework's empty state with the passthrough identifier written
// into it, which is what ImportStatePassthroughID produces. Building the state
// that way rather than asserting on a flag is the point. Read receives a
// populated object on that path, so a null-state check cannot recognise it.
func readAfterImport(t *testing.T, r *DeviceGroupResource) DeviceGroupResourceModel {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	stub := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	if diags := stub.SetAttribute(ctx, path.Root("id"), readTestGroupID); diags.HasError() {
		t.Fatalf("seeding the imported identifier: %v", diags)
	}
	if stub.Raw.IsNull() {
		t.Fatal("the imported state must be a populated object; a null one would make the old detector work")
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: stub.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{State: stub}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state DeviceGroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the hydrated state: %v", diags)
	}
	return state
}

// readWithIdentityOnly drives Read the way Terraform does for an
// `import { identity = {...} }` block: the framework writes nothing into state, so
// `req.State.Raw` is genuinely null and the identifier arrives only in
// `req.Identity`. That is the other half of the hydration signal, and it is the
// half `readAfterImport` cannot reach — the passthrough importer always leaves a
// populated object behind.
func readWithIdentityOnly(t *testing.T, r *DeviceGroupResource) DeviceGroupResourceModel {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	stub := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	if !stub.Raw.IsNull() {
		t.Fatal("the identity-only state must be genuinely null; a populated one would exercise the other branch")
	}

	identity := tfsdk.ResourceIdentity{
		Schema: identityResp.IdentitySchema,
		Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
	}
	if diags := identity.SetAttribute(ctx, path.Root("id"), readTestGroupID); diags.HasError() {
		t.Fatalf("seeding the identity: %v", diags)
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: stub.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema, Raw: identity.Raw.Copy()},
	}
	r.Read(ctx, resource.ReadRequest{State: stub, Identity: &identity}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state DeviceGroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the hydrated state: %v", diags)
	}
	return state
}

// TestRead_IdentityOnlyRefreshHydrates covers the identity-based import path end
// to end. `stateAbsent` carries it, and until this test the whole branch that
// rebuilds state out of `req.Identity` ran in no test at all — only as a boolean
// case handed straight to importHydration. A group adopted this way must come
// back with the same attributes the passthrough path adopts.
func TestRead_IdentityOnlyRefreshHydrates(t *testing.T) {
	r := &DeviceGroupResource{client: deviceGroupReadClient(t, map[string]any{
		"id":          readTestGroupID,
		"name":        "tf-acc-identity",
		"description": "Adopted by identity",
		"deviceType":  "COMPUTER",
		"groupType":   "STATIC",
		"memberCount": 1,
	}, []string{"db1c72d0-1620-44ae-a4cd-4992b713efcd"})}

	state := readWithIdentityOnly(t, r)

	if got := state.ID.ValueString(); got != readTestGroupID {
		t.Errorf("id = %q, want the identifier the identity carried", got)
	}
	if got := state.Description.ValueString(); got != "Adopted by identity" {
		t.Errorf("description = %q, want %q", got, "Adopted by identity")
	}
	var members []string
	if diags := state.Members.ElementsAs(context.Background(), &members, false); diags.HasError() {
		t.Fatalf("reading the hydrated members: %v", diags)
	}
	if len(members) != 1 || members[0] != "db1c72d0-1620-44ae-a4cd-4992b713efcd" {
		t.Errorf("members = %v, want one device", members)
	}
}

// TestRead_ImportHydratesDescription is the regression test for issue #372: an
// imported group came back with a null description whatever the API held, which
// left the attribute unmanaged and no later plan to show it.
func TestRead_ImportHydratesDescription(t *testing.T) {
	r := &DeviceGroupResource{client: deviceGroupReadClient(t, map[string]any{
		"id":          readTestGroupID,
		"name":        "tf-acc-import",
		"description": "Created outside Terraform",
		"deviceType":  "COMPUTER",
		"groupType":   "SMART",
		"memberCount": 0,
	}, nil)}

	state := readAfterImport(t, r)
	if got := state.Description.ValueString(); got != "Created outside Terraform" {
		t.Errorf("description = %q, want %q", got, "Created outside Terraform")
	}
}

// TestRead_ImportHydratesMembers covers the second attribute the same gate
// governed. A static group's membership was discarded on import for the same
// reason the description was.
func TestRead_ImportHydratesMembers(t *testing.T) {
	r := &DeviceGroupResource{client: deviceGroupReadClient(t, map[string]any{
		"id":          readTestGroupID,
		"name":        "tf-acc-import-static",
		"deviceType":  "COMPUTER",
		"groupType":   "STATIC",
		"memberCount": 1,
	}, []string{"db1c72d0-1620-44ae-a4cd-4992b713efcd"})}

	state := readAfterImport(t, r)
	var members []string
	if diags := state.Members.ElementsAs(context.Background(), &members, false); diags.HasError() {
		t.Fatalf("reading the hydrated members: %v", diags)
	}
	if len(members) != 1 || members[0] != "db1c72d0-1620-44ae-a4cd-4992b713efcd" {
		t.Errorf("members = %v, want one device", members)
	}
}

// TestRead_ImportLeavesAnEmptyGroupsMembersNull pins the other half of the
// membership rule. A group with no members imports as null rather than as an
// empty set, so importing it agrees with creating the same group without the
// attribute. Storing the empty set instead makes the import round-trip report a
// difference over nothing.
func TestRead_ImportLeavesAnEmptyGroupsMembersNull(t *testing.T) {
	r := &DeviceGroupResource{client: deviceGroupReadClient(t, map[string]any{
		"id":          readTestGroupID,
		"name":        "tf-acc-import-empty",
		"deviceType":  "COMPUTER",
		"groupType":   "STATIC",
		"memberCount": 0,
	}, nil)}

	state := readAfterImport(t, r)
	if !state.Members.IsNull() {
		t.Errorf("members = %s, want null", state.Members)
	}
}

// TestRead_RefreshLeavesAnUndeclaredDescriptionNull is the control on the fix. An
// ordinary refresh must keep honouring the resource's omit-means-unmanaged
// contract, so a description set outside Terraform stays out of state when the
// configuration never declared one.
func TestRead_RefreshLeavesAnUndeclaredDescriptionNull(t *testing.T) {
	ctx := context.Background()
	r := &DeviceGroupResource{client: deviceGroupReadClient(t, map[string]any{
		"id":          readTestGroupID,
		"name":        "tf-acc-refresh",
		"description": "Set in the admin UI",
		"deviceType":  "COMPUTER",
		"groupType":   "SMART",
		"memberCount": 0,
	}, nil)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	prior := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	if diags := prior.Set(ctx, DeviceGroupResourceModel{
		ID:          types.StringValue(readTestGroupID),
		Name:        types.StringValue("tf-acc-refresh"),
		Description: types.StringNull(),
		DeviceType:  types.StringValue("computer"),
		GroupType:   types.StringValue("smart"),
		Members:     types.SetNull(types.StringType),
		MemberCount: types.Int64Value(0),
		Timeouts:    helpers.NewResourceTimeoutsNullValue(deviceGroupTimeoutAttributeTypes),
	}); diags.HasError() {
		t.Fatalf("seeding prior state: %v", diags)
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: prior.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{State: prior}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state DeviceGroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the refreshed state: %v", diags)
	}
	if !state.Description.IsNull() {
		t.Errorf("description = %q, want null: an undeclared attribute stays unmanaged on refresh", state.Description.ValueString())
	}
}
