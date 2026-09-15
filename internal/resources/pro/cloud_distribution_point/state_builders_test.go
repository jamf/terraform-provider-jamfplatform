// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignResourceModel_JamfCloud(t *testing.T) {
	resp := &pro.CloudDistributionPoint{
		CdnType:                 "JAMF_CLOUD",
		Master:                  new(false),
		Username:                "",
		Directory:               new(""),
		CdnURL:                  new(""),
		HasConnectionSucceeded:  true,
		Message:                 "Test connection success",
		SecondaryAuthStatusCode: new(200),
		SecondaryAuthTimeToLive: new(3600),
		ExpirationSeconds:       new(3600),
		InventoryID:             new("376"),
	}

	var state CloudDistributionPointResourceModel
	assignCloudDistributionPointResourceModel(&state, resp)

	if state.CdnType.ValueString() != "JAMF_CLOUD" {
		t.Errorf("CdnType = %q", state.CdnType.ValueString())
	}
	if state.Master.IsNull() || state.Master.ValueBool() {
		t.Errorf("Master = %v, want false", state.Master)
	}
	if !state.HasConnectionSucceeded.ValueBool() {
		t.Errorf("HasConnectionSucceeded must be true")
	}
	if state.Message.ValueString() != "Test connection success" {
		t.Errorf("Message = %q", state.Message.ValueString())
	}
	if state.InventoryID.ValueString() != "376" {
		t.Errorf("InventoryID = %q, want 376", state.InventoryID.ValueString())
	}
	if state.SecondaryAuthStatusCode.ValueInt64() != 200 {
		t.Errorf("SecondaryAuthStatusCode = %d", state.SecondaryAuthStatusCode.ValueInt64())
	}
}

// TestAssignResourceModel_NonePreservesNothingHarmful confirms the assigner maps
// the NONE state cleanly (Read treats NONE as resource-absent, but the assigner
// must still not panic on the NONE-shaped response with hasConnectionSucceeded
// false / message null collapsed to "").
func TestAssignResourceModel_NoneShape(t *testing.T) {
	resp := &pro.CloudDistributionPoint{
		CdnType:                 "NONE",
		Master:                  new(false),
		HasConnectionSucceeded:  false,
		Message:                 "",
		InventoryID:             new("0"),
		SecondaryAuthStatusCode: new(200),
	}
	var state CloudDistributionPointResourceModel
	assignCloudDistributionPointResourceModel(&state, resp)
	if state.CdnType.ValueString() != "NONE" {
		t.Errorf("CdnType = %q, want NONE", state.CdnType.ValueString())
	}
	if state.HasConnectionSucceeded.ValueBool() {
		t.Errorf("HasConnectionSucceeded must be false for NONE")
	}
}

// TestAssign_NilIntPreservesCurrent verifies int64FromIntPointer keeps the prior
// state value when the server omits a field.
func TestAssign_NilIntPreservesCurrent(t *testing.T) {
	current := types.Int64Value(99)
	if got := int64FromIntPointer(nil, current); got.ValueInt64() != 99 {
		t.Errorf("nil int should preserve current 99, got %d", got.ValueInt64())
	}
	if got := int64FromIntPointer(new(5), current); got.ValueInt64() != 5 {
		t.Errorf("non-nil int should win, got %d", got.ValueInt64())
	}
}

func TestAssignDataSourceModel(t *testing.T) {
	resp := &pro.CloudDistributionPoint{
		CdnType:     "JAMF_CLOUD",
		Master:      new(true),
		InventoryID: new("1"),
	}
	var ds CloudDistributionPointDataSourceModel
	assignCloudDistributionPointDataSourceModel(&ds, resp)
	if ds.CdnType.ValueString() != "JAMF_CLOUD" || !ds.Master.ValueBool() {
		t.Errorf("data source assign wrong: %+v", ds)
	}
	if ds.InventoryID.ValueString() != "1" {
		t.Errorf("InventoryID = %q", ds.InventoryID.ValueString())
	}
}
