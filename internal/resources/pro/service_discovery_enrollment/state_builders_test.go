// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package service_discovery_enrollment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func priorList(t *testing.T, models ...wellKnownSettingModel) types.List {
	t.Helper()
	list, diags := types.ListValueFrom(context.Background(), wellKnownSettingObjectType(), models)
	if diags.HasError() {
		t.Fatalf("priorList: %v", diags)
	}
	return list
}

func decodeRows(t *testing.T, list types.List) []wellKnownSettingModel {
	t.Helper()
	out := make([]wellKnownSettingModel, 0, len(list.Elements()))
	if diags := list.ElementsAs(context.Background(), &out, false); diags.HasError() {
		t.Fatalf("decodeRows: %v", diags)
	}
	return out
}

// Reconcile is by server_uuid key-match but emits rows in the INCOMING (config)
// order — the GET response order must not reorder the user's list.
func TestAssignResourceModel_ByKeyPreservesConfigOrder(t *testing.T) {
	ctx := context.Background()
	state := &ServiceDiscoveryEnrollmentResourceModel{
		WellKnownSetting: priorList(t,
			wellKnownSettingModel{ServerUUID: types.StringValue("UUID-A"), EnrollmentType: types.StringValue(enrollmentTypeMDMADDE), OrgName: types.StringUnknown()},
			wellKnownSettingModel{ServerUUID: types.StringValue("UUID-B"), EnrollmentType: types.StringValue(enrollmentTypeMDMBYOD), OrgName: types.StringUnknown()},
		),
	}
	// GET returns the rows in the opposite order, with authoritative org names.
	resp := &pro.WellKnownSettingsResponse{WellKnownSettings: []pro.WellKnownSetting{
		{ServerUUID: "UUID-B", EnrollmentType: enrollmentTypeMDMBYOD, OrgName: new("Org B")},
		{ServerUUID: "UUID-A", EnrollmentType: enrollmentTypeMDMADDE, OrgName: new("Org A")},
	}}

	diags := assignServiceDiscoveryEnrollmentResourceModel(ctx, state, resp)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	rows := decodeRows(t, state.WellKnownSetting)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].ServerUUID.ValueString() != "UUID-A" || rows[1].ServerUUID.ValueString() != "UUID-B" {
		t.Errorf("config order not preserved: %s, %s", rows[0].ServerUUID, rows[1].ServerUUID)
	}
	if rows[0].OrgName.ValueString() != "Org A" {
		t.Errorf("org_name not taken from wire: %s", rows[0].OrgName)
	}
}

// A declared server_uuid the server does not recognize is silently dropped from GET;
// the row is retained with a null org_name and a warning is emitted.
func TestAssignResourceModel_DeclaredButAbsentWarns(t *testing.T) {
	ctx := context.Background()
	state := &ServiceDiscoveryEnrollmentResourceModel{
		WellKnownSetting: priorList(t,
			wellKnownSettingModel{ServerUUID: types.StringValue("BOGUS"), EnrollmentType: types.StringValue(enrollmentTypeMDMADDE), OrgName: types.StringUnknown()},
		),
	}
	resp := &pro.WellKnownSettingsResponse{WellKnownSettings: []pro.WellKnownSetting{
		{ServerUUID: "REAL", EnrollmentType: enrollmentTypeNone, OrgName: new("Real Org")},
	}}

	diags := assignServiceDiscoveryEnrollmentResourceModel(ctx, state, resp)
	if diags.HasError() {
		t.Fatalf("unexpected error diags: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Errorf("want 1 warning for the unrecognized server_uuid, got %d", diags.WarningsCount())
	}
	rows := decodeRows(t, state.WellKnownSetting)
	if len(rows) != 1 || rows[0].ServerUUID.ValueString() != "BOGUS" {
		t.Fatalf("declared row should be retained, got %#v", rows)
	}
	if !rows[0].OrgName.IsNull() {
		t.Errorf("absent row org_name should be null, got %s", rows[0].OrgName)
	}
	if rows[0].EnrollmentType.ValueString() != enrollmentTypeMDMADDE {
		t.Errorf("planned enrollment_type should be retained, got %s", rows[0].EnrollmentType)
	}
}

// Import (no prior model) adopts every GET row in server order.
func TestAssignResourceModel_ImportAdoptsAll(t *testing.T) {
	ctx := context.Background()
	state := &ServiceDiscoveryEnrollmentResourceModel{
		WellKnownSetting: types.ListNull(wellKnownSettingObjectType()),
	}
	resp := &pro.WellKnownSettingsResponse{WellKnownSettings: []pro.WellKnownSetting{
		{ServerUUID: "UUID-A", EnrollmentType: enrollmentTypeMDMADDE, OrgName: new("Org A")},
		{ServerUUID: "UUID-B", EnrollmentType: enrollmentTypeNone, OrgName: new("Org B")},
	}}

	diags := assignServiceDiscoveryEnrollmentResourceModel(ctx, state, resp)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	rows := decodeRows(t, state.WellKnownSetting)
	if len(rows) != 2 || rows[0].ServerUUID.ValueString() != "UUID-A" || rows[1].ServerUUID.ValueString() != "UUID-B" {
		t.Errorf("import should adopt all rows in server order, got %#v", rows)
	}
}

func TestAssignDataSourceModel_AdoptsAll(t *testing.T) {
	ctx := context.Background()
	state := &ServiceDiscoveryEnrollmentDataSourceModel{}
	resp := &pro.WellKnownSettingsResponse{WellKnownSettings: []pro.WellKnownSetting{
		{ServerUUID: "UUID-A", EnrollmentType: enrollmentTypeMDMBYOD, OrgName: new("Org A")},
	}}
	diags := assignServiceDiscoveryEnrollmentDataSourceModel(ctx, state, resp)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	rows := decodeRows(t, state.WellKnownSetting)
	if len(rows) != 1 || rows[0].OrgName.ValueString() != "Org A" {
		t.Errorf("data source should surface all rows incl org_name, got %#v", rows)
	}
}
