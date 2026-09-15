// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
)

func TestAssignDeviceGroupModel_StaticWithMembers(t *testing.T) {
	model := &DeviceGroupResourceModel{
		Members:     types.SetNull(types.StringType),
		Description: types.StringValue("original"),
	}

	grp := &devicegroups.DeviceGroupReadRepresentationV1{
		ID:          "grp-1",
		Name:        "Static Group",
		Description: "API description",
		DeviceType:  "COMPUTER",
		GroupType:   "STATIC",
		MemberCount: 2,
	}
	members := []string{"device-a", "device-b"}

	diags := assignDeviceGroupModel(context.Background(), model, grp, members, true, true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if model.ID.ValueString() != "grp-1" {
		t.Errorf("expected ID 'grp-1', got %q", model.ID.ValueString())
	}
	if model.Name.ValueString() != "Static Group" {
		t.Errorf("expected Name 'Static Group', got %q", model.Name.ValueString())
	}
	if model.Description.ValueString() != "API description" {
		t.Errorf("expected Description 'API description', got %q", model.Description.ValueString())
	}
	if model.DeviceType.ValueString() != "computer" {
		t.Errorf("expected DeviceType 'computer', got %q", model.DeviceType.ValueString())
	}
	if model.GroupType.ValueString() != "static" {
		t.Errorf("expected GroupType 'static', got %q", model.GroupType.ValueString())
	}
	if model.MemberCount.ValueInt64() != 2 {
		t.Errorf("expected MemberCount 2, got %d", model.MemberCount.ValueInt64())
	}
	if model.Members.IsNull() {
		t.Error("expected Members to be non-null for managed static group")
	}

	var memberValues []string
	model.Members.ElementsAs(context.Background(), &memberValues, false)
	if len(memberValues) != 2 {
		t.Errorf("expected 2 members, got %d", len(memberValues))
	}
}

func TestAssignDeviceGroupModel_StaticUnmanagedMembers(t *testing.T) {
	model := &DeviceGroupResourceModel{
		Members:     types.SetNull(types.StringType),
		Description: types.StringNull(),
	}

	grp := &devicegroups.DeviceGroupReadRepresentationV1{
		ID:          "grp-2",
		Name:        "Static Unmanaged",
		DeviceType:  "MOBILE",
		GroupType:   "STATIC",
		MemberCount: 10,
	}

	diags := assignDeviceGroupModel(context.Background(), model, grp, nil, false, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !model.Members.IsNull() {
		t.Error("expected Members to be null when not managing members")
	}
	if !model.Description.IsNull() {
		t.Error("expected Description to be null when not managing description")
	}
}

func TestAssignDeviceGroupModel_SmartGroup(t *testing.T) {
	model := &DeviceGroupResourceModel{
		Members:     types.SetNull(types.StringType),
		Description: types.StringValue("desc"),
	}

	grp := &devicegroups.DeviceGroupReadRepresentationV1{
		ID:         "grp-3",
		Name:       "Smart Group",
		DeviceType: "COMPUTER",
		GroupType:  "SMART",
		Criteria: &[]devicegroups.DeviceGroupCriteriaRepresentationV1{
			{
				Order:          0,
				AttributeName:  "Device Name",
				Operator:       "LIKE",
				AttributeValue: "Mac",
				JoinType:       "AND",
			},
		},
		MemberCount: 50,
	}

	diags := assignDeviceGroupModel(context.Background(), model, grp, nil, false, true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !model.Members.IsNull() {
		t.Error("expected Members to be null for smart group")
	}
	if len(model.Criteria) != 1 {
		t.Fatalf("expected 1 criterion, got %d", len(model.Criteria))
	}
	if model.Criteria[0].AttributeName.ValueString() != "Device Name" {
		t.Errorf("expected criterion attribute 'Device Name', got %q", model.Criteria[0].AttributeName.ValueString())
	}
	if model.Criteria[0].Operator.ValueString() != "like" {
		t.Errorf("expected operator 'like', got %q", model.Criteria[0].Operator.ValueString())
	}
	if model.MemberCount.ValueInt64() != 50 {
		t.Errorf("expected MemberCount 50, got %d", model.MemberCount.ValueInt64())
	}
}

func TestAssignDeviceGroupModel_EmptyName(t *testing.T) {
	model := &DeviceGroupResourceModel{
		Members: types.SetNull(types.StringType),
	}

	grp := &devicegroups.DeviceGroupReadRepresentationV1{
		ID:        "grp-4",
		GroupType: "STATIC",
	}

	diags := assignDeviceGroupModel(context.Background(), model, grp, nil, false, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !model.Name.IsNull() {
		t.Error("expected Name to be null for empty string")
	}
	if !model.DeviceType.IsNull() {
		t.Error("expected DeviceType to be null for empty string")
	}
}

func TestFlattenDeviceGroupCriteria(t *testing.T) {
	criteria := []devicegroups.DeviceGroupCriteriaRepresentationV1{
		{
			Order:                 0,
			AttributeName:         "Device Name",
			Operator:              "LIKE",
			AttributeValue:        "Mac",
			JoinType:              "AND",
			HasOpeningParenthesis: new(true),
			HasClosingParenthesis: new(false),
		},
		{
			Order:                 1,
			AttributeName:         "OS Version",
			Operator:              "GREATER THAN",
			AttributeValue:        "14.0",
			JoinType:              "OR",
			HasOpeningParenthesis: new(false),
			HasClosingParenthesis: new(true),
		},
	}

	result := flattenDeviceGroupCriteria(criteria, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(result))
	}

	c0 := result[0]
	if c0.AttributeName.ValueString() != "Device Name" {
		t.Errorf("expected AttributeName 'Device Name', got %q", c0.AttributeName.ValueString())
	}
	if c0.Operator.ValueString() != "like" {
		t.Errorf("expected Operator 'like', got %q", c0.Operator.ValueString())
	}
	if c0.AttributeValue.ValueString() != "Mac" {
		t.Errorf("expected AttributeValue 'Mac', got %q", c0.AttributeValue.ValueString())
	}
	if c0.JoinType.ValueString() != "and" {
		t.Errorf("expected JoinType 'and', got %q", c0.JoinType.ValueString())
	}
	if c0.Order.ValueInt64() != 0 {
		t.Errorf("expected Order 0, got %d", c0.Order.ValueInt64())
	}

	c1 := result[1]
	if c1.Operator.ValueString() != "greater than" {
		t.Errorf("expected Operator 'greater than', got %q", c1.Operator.ValueString())
	}
	if c1.JoinType.ValueString() != "or" {
		t.Errorf("expected JoinType 'or', got %q", c1.JoinType.ValueString())
	}
}

func TestFlattenDeviceGroupCriteria_Empty(t *testing.T) {
	// nil current → nil (null in state, user never set criteria)
	result := flattenDeviceGroupCriteria(nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil current, got %v", result)
	}

	// empty-slice current → empty slice (user set criteria = [], smart match-all)
	result = flattenDeviceGroupCriteria([]devicegroups.DeviceGroupCriteriaRepresentationV1{}, []DeviceGroupCriteriaModel{})
	if result == nil {
		t.Errorf("expected empty slice for empty-slice current, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected len 0, got %d", len(result))
	}
}

func TestFlattenDeviceGroupCriteria_StateAware(t *testing.T) {
	current := []DeviceGroupCriteriaModel{
		{
			Order:          types.Int64Value(0),
			AttributeValue: types.StringValue("Mac"),
		},
	}

	apiCriteria := []devicegroups.DeviceGroupCriteriaRepresentationV1{
		{
			Order:          0,
			AttributeName:  "Device Name",
			Operator:       "LIKE",
			AttributeValue: "Mac",
			JoinType:       "AND",
		},
	}

	result := flattenDeviceGroupCriteria(apiCriteria, current)
	if len(result) != 1 {
		t.Fatalf("expected 1 criterion, got %d", len(result))
	}

	if result[0].Order.ValueInt64() != 0 {
		t.Errorf("expected reconciled Order 0, got %d", result[0].Order.ValueInt64())
	}
	if result[0].AttributeValue.ValueString() != "Mac" {
		t.Errorf("expected reconciled value 'Mac', got %q", result[0].AttributeValue.ValueString())
	}
}

func TestFlattenDeviceGroupCriteria_NullFields(t *testing.T) {
	criteria := []devicegroups.DeviceGroupCriteriaRepresentationV1{
		{
			Order:    0,
			Operator: "",
			JoinType: "",
		},
	}

	result := flattenDeviceGroupCriteria(criteria, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 criterion, got %d", len(result))
	}

	if !result[0].AttributeName.IsNull() {
		t.Error("expected null AttributeName for empty string")
	}
	if !result[0].Operator.IsNull() {
		t.Error("expected null Operator for empty string")
	}
	if !result[0].JoinType.IsNull() {
		t.Error("expected null JoinType for empty string")
	}
}
