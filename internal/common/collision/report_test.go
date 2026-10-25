// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var reportSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"name":     schema.StringAttribute{Required: true},
		"criteria": schema.ListAttribute{Optional: true, ElementType: schema.StringAttribute{}.GetType()},
	},
}

func reportObject(id, name tftypes.Value) tftypes.Value {
	typ := reportSchema.Type().TerraformType(context.Background())
	return tftypes.NewValue(typ, map[string]tftypes.Value{
		"id":       id,
		"name":     name,
		"criteria": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, tftypes.UnknownValue),
	})
}

func nullObject() tftypes.Value {
	return tftypes.NewValue(reportSchema.Type().TerraformType(context.Background()), nil)
}

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

var unknownString = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

func planRequest(state, plan tftypes.Value) resource.ModifyPlanRequest {
	return resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: reportSchema, Raw: state},
		Plan:  tfsdk.Plan{Schema: reportSchema, Raw: plan},
	}
}

func attributeReport(ns Namespace) AttributeReport {
	return AttributeReport{
		Checker:      newChecker(ModeError, false),
		ResourceType: testResourceType,
		Rules:        []AttributeRule{{Namespace: ns, Path: path.Root("name")}},
	}
}

func TestReportAttributePlan_CreateCollidesDespiteUnknownSiblings(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "Lab"}}, nil)
	var resp resource.ModifyPlanResponse
	ReportAttributePlan(context.Background(), planRequest(nullObject(), reportObject(unknownString, str("lab"))), &resp, attributeReport(ns))
	if !resp.Diagnostics.HasError() {
		t.Fatal("a create must be checked even when another attribute is unknown, which a model decode would reject")
	}
}

func TestReportAttributePlan_UnknownNameSkipped(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "Lab"}}, nil)
	var resp resource.ModifyPlanResponse
	ReportAttributePlan(context.Background(), planRequest(nullObject(), reportObject(unknownString, unknownString)), &resp, attributeReport(ns))
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("an unknown name must wait for the apply-time re-plan, got %v", resp.Diagnostics)
	}
}

func TestReportAttributePlan_UnchangedNameSkipped(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "8", Key: "Lab"}}, nil)
	obj := reportObject(str("7"), str("Lab"))
	var resp resource.ModifyPlanResponse
	ReportAttributePlan(context.Background(), planRequest(obj, obj), &resp, attributeReport(ns))
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("an unchanged name is not a new key, got %v", resp.Diagnostics)
	}
}

func TestReportAttributePlan_RenameCollides(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "Mine"}, {ID: "8", Key: "Taken"}}, nil)
	var resp resource.ModifyPlanResponse
	ReportAttributePlan(context.Background(), planRequest(reportObject(str("7"), str("Mine")), reportObject(str("7"), str("taken"))), &resp, attributeReport(ns))
	if !resp.Diagnostics.HasError() {
		t.Fatal("a rename onto another object's name must collide")
	}
}

func TestReportAttributePlan_RenameToOwnNameFoldedIsSelf(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "mine"}}, nil)
	var resp resource.ModifyPlanResponse
	ReportAttributePlan(context.Background(), planRequest(reportObject(str("7"), str("old")), reportObject(str("7"), str("MINE"))), &resp, attributeReport(ns))
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("an object must not collide with itself, got %v", resp.Diagnostics)
	}
}

type reportModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Criteria types.List   `tfsdk:"criteria"`
}

func TestReportPlan_CaseOnlyRenameIsNotACollisionWithItself(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "mine"}}, nil)
	var resp resource.ModifyPlanResponse
	ReportPlan(context.Background(), planRequest(reportObject(str("7"), str("old")), reportObject(str("7"), str("MINE"))), &resp, PlanReport[reportModel]{
		Checker:      newChecker(ModeError, false),
		ResourceType: testResourceType,
		Rules: []Rule[reportModel]{{
			Namespace: ns,
			Path:      path.Root("name"),
			Keys:      func(m *reportModel) []types.String { return []types.String{m.Name} },
		}},
	})
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("the object's own ID must be read from state, got %v", resp.Diagnostics)
	}
}

func TestReportAttributePlan_DestroySkipped(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "Lab"}}, nil)
	var resp resource.ModifyPlanResponse
	ReportAttributePlan(context.Background(), planRequest(reportObject(str("7"), str("Lab")), nullObject()), &resp, attributeReport(ns))
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("a destroy is never a collision, got %v", resp.Diagnostics)
	}
}
