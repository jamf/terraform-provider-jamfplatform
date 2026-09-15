// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// extractDeploymentSettings is a test helper that extracts a DeploymentSettingsModel
// from a types.Object for assertion. Fails the test if extraction fails.
func extractDeploymentSettings(t *testing.T, obj types.Object) DeploymentSettingsModel {
	t.Helper()
	var m DeploymentSettingsModel
	if diags := obj.As(context.Background(), &m, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("failed to extract DeploymentSettingsModel from types.Object: %v", diags)
	}
	return m
}

// extractEndUserExperience is a test helper that extracts an EndUserExperienceModel
// from a types.Object for assertion.
func extractEndUserExperience(t *testing.T, obj types.Object) EndUserExperienceModel {
	t.Helper()
	var m EndUserExperienceModel
	if diags := obj.As(context.Background(), &m, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("failed to extract EndUserExperienceModel from types.Object: %v", diags)
	}
	return m
}

// TestAssignDeploymentSettings_AllNull verifies that an all-null SDK block
// normalizes to ObjectNull, preventing drift when the user omits the block.
func TestAssignDeploymentSettings_AllNull(t *testing.T) {
	got := assignDeploymentSettings(&pro.AppInstallersDeploymentProcessControls{})
	if !got.IsNull() {
		t.Errorf("all-null block should normalize to null types.Object, got %v", got)
	}
}

func TestAssignDeploymentSettings_NilBlock(t *testing.T) {
	got := assignDeploymentSettings(nil)
	if !got.IsNull() {
		t.Errorf("nil block should normalize to null types.Object")
	}
}

func TestAssignDeploymentSettings_Populated(t *testing.T) {
	batchSize := 1000
	batchFreq := 60
	fromTime := "08:00:00Z"
	toTime := "17:00:00Z"
	days := []string{"MONDAY", "WEDNESDAY"}
	s := &pro.AppInstallersDeploymentProcessControls{
		CommandsBatchSize:       &batchSize,
		BatchFrequencyInMinutes: &batchFreq,
		DaysOfWeek:              &days,
		FromTimeOfDay:           &fromTime,
		ToTimeOfDay:             &toTime,
	}
	got := assignDeploymentSettings(s)
	if got.IsNull() {
		t.Fatal("expected non-null block for populated response")
	}
	m := extractDeploymentSettings(t, got)
	if m.BatchSize.ValueInt64() != 1000 {
		t.Errorf("BatchSize = %d, want 1000", m.BatchSize.ValueInt64())
	}
	if m.BatchFrequency.ValueInt64() != 60 {
		t.Errorf("BatchFrequency = %d, want 60", m.BatchFrequency.ValueInt64())
	}
	if m.ServerTimeFrom.ValueString() != "08:00:00Z" {
		t.Errorf("ServerTimeFrom = %q, want 08:00:00Z", m.ServerTimeFrom.ValueString())
	}
	if m.ServerTimeTo.ValueString() != "17:00:00Z" {
		t.Errorf("ServerTimeTo = %q, want 17:00:00Z", m.ServerTimeTo.ValueString())
	}
	if m.Days.IsNull() {
		t.Error("Days should not be null for non-nil slice")
	}
	if len(m.Days.Elements()) != 2 {
		t.Errorf("Days len = %d, want 2", len(m.Days.Elements()))
	}
}

// TestAssignDays_NullVsEmpty verifies the probe-confirmed semantics:
// nil pointer → SetNull, &[]string{} → empty set (distinct from null).
func TestAssignDays_NullVsEmpty(t *testing.T) {
	nullSet := assignDays(nil)
	if !nullSet.IsNull() {
		t.Error("nil days pointer should yield null Set")
	}

	empty := []string{}
	emptySet := assignDays(&empty)
	if emptySet.IsNull() {
		t.Error("empty days slice should yield non-null empty Set")
	}
	if len(emptySet.Elements()) != 0 {
		t.Errorf("empty days set should have 0 elements, got %d", len(emptySet.Elements()))
	}
}

// TestAssignEndUserExperience_AllNull mirrors the deployment block test.
func TestAssignEndUserExperience_AllNull(t *testing.T) {
	got := assignEndUserExperience(pro.GlobalSettingsEndUserExperience{})
	if !got.IsNull() {
		t.Errorf("all-null EUX block should normalize to null types.Object, got %v", got)
	}
}

func TestAssignEndUserExperience_Populated(t *testing.T) {
	interval := int64(2)
	msg := "Update pending"
	deadline := int64(24)
	dlMsg := "Please quit and save"
	delay := int64(10)
	complete := "Update complete"
	relaunch := true
	suppress := false
	s := pro.GlobalSettingsEndUserExperience{
		NotificationInterval: &interval,
		NotificationMessage:  &msg,
		Deadline:             &deadline,
		DeadlineMessage:      &dlMsg,
		QuitDelay:            &delay,
		CompleteMessage:      &complete,
		Relaunch:             &relaunch,
		Suppress:             &suppress,
	}
	got := assignEndUserExperience(s)
	if got.IsNull() {
		t.Fatal("expected non-null EUX block for populated response")
	}
	m := extractEndUserExperience(t, got)
	if m.NotificationFrequency.ValueInt64() != 2 {
		t.Errorf("NotificationFrequency = %d, want 2", m.NotificationFrequency.ValueInt64())
	}
	if m.UpdateDeadline.ValueInt64() != 24 {
		t.Errorf("UpdateDeadline = %d, want 24", m.UpdateDeadline.ValueInt64())
	}
	if m.ForceQuitMessage.ValueString() != "Please quit and save" {
		t.Errorf("ForceQuitMessage = %q, want %q", m.ForceQuitMessage.ValueString(), "Please quit and save")
	}
	if m.ForceQuitGracePeriod.ValueInt64() != 10 {
		t.Errorf("ForceQuitGracePeriod = %d, want 10", m.ForceQuitGracePeriod.ValueInt64())
	}
	if !m.Relaunch.ValueBool() {
		t.Error("Relaunch should be true")
	}
	if m.Suppress.ValueBool() {
		t.Error("Suppress should be false")
	}
}

// TestAssignEndUserExperience_PartialPopulated verifies that a block with at
// least one non-nil field is not collapsed to null.
func TestAssignEndUserExperience_PartialPopulated(t *testing.T) {
	interval := int64(4)
	s := pro.GlobalSettingsEndUserExperience{
		NotificationInterval: &interval,
	}
	got := assignEndUserExperience(s)
	if got.IsNull() {
		t.Fatal("block with one non-nil field must not collapse to null")
	}
	m := extractEndUserExperience(t, got)
	if m.NotificationFrequency.ValueInt64() != 4 {
		t.Errorf("NotificationFrequency = %d, want 4", m.NotificationFrequency.ValueInt64())
	}
	if !m.UpdateDeadline.IsNull() {
		t.Error("unset UpdateDeadline should be null")
	}
}

// TestAssignResourceModel_DoesNotClobberID verifies the assigner leaves ID untouched.
func TestAssignResourceModel_DoesNotClobberID(t *testing.T) {
	state := AppInstallerSettingsResourceModel{
		ID: types.StringValue(helpers.SingletonID),
	}
	assignAppInstallerSettingsResourceModel(&state, &pro.AppInstallersGlobalSettings{})
	if state.ID.ValueString() != helpers.SingletonID {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), helpers.SingletonID)
	}
}
