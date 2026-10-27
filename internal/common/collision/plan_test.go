// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func replacementReport(entries []Entry) AttributeReport {
	return attributeReport(widgetNamespace(entries, nil))
}

func TestReportAttributePlan_ReplacementDoesNotCollideWithItself(t *testing.T) {
	ctx := context.Background()
	rep := replacementReport([]Entry{{ID: "7", Key: "kept"}})
	obj := reportObject(str("7"), str("kept"))

	update := &resource.ModifyPlanResponse{RequiresReplace: path.Paths{path.Root("kind")}}
	ReportAttributePlan(ctx, planRequest(obj, obj), update, rep)
	if len(update.Diagnostics) != 0 {
		t.Fatalf("an unchanged name must not be checked, got %v", update.Diagnostics)
	}

	var create resource.ModifyPlanResponse
	ReportAttributePlan(ctx, planRequest(nullObject(), reportObject(unknownString, str("kept"))), &create, rep)
	if len(create.Diagnostics) != 0 {
		t.Fatalf("the object being replaced must not collide with itself, got %v", create.Diagnostics)
	}
}

func TestReportAttributePlan_NewResourceStillCollidesWithAnUnreplacedObject(t *testing.T) {
	ctx := context.Background()
	rep := replacementReport([]Entry{{ID: "7", Key: "kept"}})
	obj := reportObject(str("7"), str("kept"))

	var update resource.ModifyPlanResponse
	ReportAttributePlan(ctx, planRequest(obj, obj), &update, rep)

	var create resource.ModifyPlanResponse
	ReportAttributePlan(ctx, planRequest(nullObject(), reportObject(unknownString, str("kept"))), &create, rep)
	if !create.Diagnostics.HasError() {
		t.Fatal("a new resource taking the name of an object that is not being replaced must collide")
	}
}

func TestReportAttributePlan_ReplacementIsPerResourceType(t *testing.T) {
	ctx := context.Background()
	rep := replacementReport([]Entry{{ID: "7", Key: "kept"}})
	obj := reportObject(str("7"), str("kept"))

	update := &resource.ModifyPlanResponse{RequiresReplace: path.Paths{path.Root("kind")}}
	ReportAttributePlan(ctx, planRequest(obj, obj), update, rep)

	other := rep
	other.ResourceType = "jamfplatform_pro_other"
	var create resource.ModifyPlanResponse
	ReportAttributePlan(ctx, planRequest(nullObject(), reportObject(unknownString, str("kept"))), &create, other)
	if !create.Diagnostics.HasError() {
		t.Fatal("a replacement recorded for one resource type must not excuse another")
	}
}

func TestReportAttributePlan_ModifiedObjectIsNotAFalseCollisionWhenReplaced(t *testing.T) {
	ctx := context.Background()
	rep := replacementReport([]Entry{{ID: "7", Key: "kept"}})

	var update resource.ModifyPlanResponse
	ReportAttributePlan(ctx, planRequest(reportObject(str("7"), str("kept")), reportObject(unknownString, str("kept"))), &update, rep)

	var create resource.ModifyPlanResponse
	ReportAttributePlan(ctx, planRequest(nullObject(), reportObject(unknownString, str("kept"))), &create, rep)
	if len(create.Diagnostics) != 0 {
		t.Fatalf("an object whose plan differs from its state may be replaced and must not collide with itself, got %v", create.Diagnostics)
	}
}
