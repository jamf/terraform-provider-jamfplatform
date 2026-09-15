// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestInt64ValueFromPtr(t *testing.T) {
	if got := helpers.Int64FromIntPtr(nil); !got.IsNull() {
		t.Errorf("nil pointer must map to null, got %v", got)
	}
	if got := helpers.Int64FromIntPtr(new(445)); got.ValueInt64() != 445 {
		t.Errorf("expected 445, got %v", got)
	}
}

// TestAssignResourceModel_AdoptsServerValues confirms the server's values are
// adopted, the backup sentinel round-trips, and the WriteOnly passwords are
// left untouched (the response never carries them).
func TestAssignResourceModel_AdoptsServerValues(t *testing.T) {
	c := &pro.DistributionPoint{
		ID:                        new("80"),
		Name:                      "dp",
		ServerName:                "s.example.com",
		FileSharingConnectionType: "SMB",
		Principal:                 new(true),
		BackupDistributionPointID: new(cloudBackupSentinel),
		EnableLoadBalancing:       new(false),
		ShareName:                 new("CasperShare"),
		Port:                      new(445),
		ReadWriteUsername:         new("rw"),
		HttpsEnabled:              new(true),
		HttpsPort:                 new(443),
		HttpsSecurityType:         new(httpsSecurityNone),
	}

	state := FileShareDistributionPointResourceModel{
		// A WriteOnly password is never persisted; the version companion is.
		ReadWritePasswordWoVer: types.Int64Value(3),
	}
	assignFileShareDistributionPointResourceModel(&state, c)

	if state.ID.ValueString() != "80" || state.Name.ValueString() != "dp" {
		t.Errorf("id/name wrong: %+v", state)
	}
	if !state.Principal.ValueBool() {
		t.Errorf("principal must adopt server true")
	}
	if state.BackupDistributionPointID.ValueString() != cloudBackupSentinel {
		t.Errorf("backup sentinel must round-trip, got %v", state.BackupDistributionPointID)
	}
	if state.Port.ValueInt64() != 445 {
		t.Errorf("port = %v", state.Port)
	}
	if state.ReadWriteUsername.ValueString() != "rw" {
		t.Errorf("read_write_username = %v", state.ReadWriteUsername)
	}
	// assign must not invent a password value; the wo_version is preserved.
	if state.ReadWritePasswordWoVer.ValueInt64() != 3 {
		t.Errorf("wo_version must be preserved, got %v", state.ReadWritePasswordWoVer)
	}
}

// TestAssignResourceModel_ReconcileEmptyVsNull confirms the empty/null
// asymmetry: a field the user explicitly cleared ("") round-trips as "", while
// a never-set field (server null) stays null.
func TestAssignResourceModel_ReconcileEmptyVsNull(t *testing.T) {
	// User had set workgroup to "" (cleared); server echoes "".
	stateCleared := FileShareDistributionPointResourceModel{Workgroup: types.StringValue("")}
	cCleared := &pro.DistributionPoint{Name: "n", ServerName: "s", FileSharingConnectionType: "SMB", Workgroup: new("")}
	assignFileShareDistributionPointResourceModel(&stateCleared, cCleared)
	if stateCleared.Workgroup.IsNull() || stateCleared.Workgroup.ValueString() != "" {
		t.Errorf("explicitly cleared workgroup must stay \"\", got %v", stateCleared.Workgroup)
	}

	// Never-set workgroup: server returns null.
	stateUnset := FileShareDistributionPointResourceModel{Workgroup: types.StringNull()}
	cUnset := &pro.DistributionPoint{Name: "n", ServerName: "s", FileSharingConnectionType: "NONE"}
	assignFileShareDistributionPointResourceModel(&stateUnset, cUnset)
	if !stateUnset.Workgroup.IsNull() {
		t.Errorf("never-set workgroup must stay null, got %v", stateUnset.Workgroup)
	}
}
