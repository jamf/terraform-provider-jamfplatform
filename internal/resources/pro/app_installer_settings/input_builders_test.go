// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestBuildAppInstallerSettingsInput_NilBlocks verifies that a plan with no
// blocks and no merge base produces a full-replace clear: the deployment-controls
// pointer is nil so the key is omitted, and the end-user-experience block — which
// the API declares as a required member of the body, so it has no omitted form —
// carries every field nil.
func TestBuildAppInstallerSettingsInput_NilBlocks(t *testing.T) {
	plan := AppInstallerSettingsResourceModel{}
	got := buildAppInstallerSettingsInput(plan)
	if got.DeploymentProcessControls != nil {
		t.Errorf("expected nil DeploymentProcessControls, got %+v", got.DeploymentProcessControls)
	}
	if got.EndUserExperienceSettings != (pro.GlobalSettingsEndUserExperience{}) {
		t.Errorf("expected the zero end-user-experience block, got %+v", got.EndUserExperienceSettings)
	}
}

// TestBuildDeploymentSettings_NilFields verifies that null TF values produce
// nil SDK pointers, not zero-value integers.
func TestBuildDeploymentSettings_NilFields(t *testing.T) {
	m := &DeploymentSettingsModel{
		BatchSize:      types.Int64Null(),
		BatchFrequency: types.Int64Null(),
		Days:           types.SetNull(types.StringType),
		ServerTimeFrom: types.StringNull(),
		ServerTimeTo:   types.StringNull(),
	}
	got := buildDeploymentSettings(m)
	if got == nil {
		t.Fatal("expected non-nil struct for non-nil model")
	}
	if got.CommandsBatchSize != nil {
		t.Errorf("null BatchSize should produce nil pointer, got %d", *got.CommandsBatchSize)
	}
	if got.DaysOfWeek != nil {
		t.Errorf("null Days should produce nil pointer")
	}
}

// TestBuildDeploymentSettings_Populated verifies round-trip for all fields.
func TestBuildDeploymentSettings_Populated(t *testing.T) {
	days := types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("MONDAY"),
		types.StringValue("FRIDAY"),
	})

	m := &DeploymentSettingsModel{
		BatchSize:      types.Int64Value(500),
		BatchFrequency: types.Int64Value(120),
		Days:           days,
		ServerTimeFrom: types.StringValue("09:00:00Z"),
		ServerTimeTo:   types.StringValue("18:00:00Z"),
	}
	got := buildDeploymentSettings(m)
	if got == nil {
		t.Fatal("expected non-nil struct")
	}
	if *got.CommandsBatchSize != 500 {
		t.Errorf("CommandsBatchSize = %d, want 500", *got.CommandsBatchSize)
	}
	if *got.BatchFrequencyInMinutes != 120 {
		t.Errorf("BatchFrequencyInMinutes = %d, want 120", *got.BatchFrequencyInMinutes)
	}
	if *got.FromTimeOfDay != "09:00:00Z" {
		t.Errorf("FromTimeOfDay = %q, want 09:00:00Z", *got.FromTimeOfDay)
	}
	if got.DaysOfWeek == nil || len(*got.DaysOfWeek) != 2 {
		t.Errorf("DaysOfWeek: expected 2 elements")
	}
}

// TestBuildDays_NullVsEmpty verifies the nil/empty distinction is preserved.
func TestBuildDays_NullVsEmpty(t *testing.T) {
	nullResult := buildDays(types.SetNull(types.StringType))
	if nullResult != nil {
		t.Error("null Set should produce nil pointer")
	}

	emptyResult := buildDays(types.SetValueMust(types.StringType, nil))
	if emptyResult == nil {
		t.Fatal("empty Set should produce non-nil empty slice")
	}
	if len(*emptyResult) != 0 {
		t.Errorf("empty Set should produce empty slice, got len %d", len(*emptyResult))
	}
}

// TestBuildEndUserExperience_NilFields verifies null TF values produce nil pointers.
func TestBuildEndUserExperience_NilFields(t *testing.T) {
	m := &EndUserExperienceModel{
		NotificationFrequency: types.Int64Null(),
		NotificationMessage:   types.StringNull(),
		UpdateDeadline:        types.Int64Null(),
		ForceQuitMessage:      types.StringNull(),
		ForceQuitGracePeriod:  types.Int64Null(),
		UpdateCompleteMessage: types.StringNull(),
		Relaunch:              types.BoolNull(),
		Suppress:              types.BoolNull(),
	}
	got := buildEndUserExperience(m)
	if got.NotificationInterval != nil {
		t.Error("null NotificationFrequency should produce nil pointer")
	}
	if got.Relaunch != nil {
		t.Error("null Relaunch should produce nil pointer")
	}
}

// mustDeploymentObject builds a deployment_settings types.Object from a model using
// the schema's own attribute types, so a hand-built plan carries the same shape the
// framework hands the production code path.
func mustDeploymentObject(t *testing.T, m DeploymentSettingsModel) types.Object {
	t.Helper()
	obj, diags := types.ObjectValueFrom(context.Background(), deploymentSettingsAttrTypes, m)
	if diags.HasError() {
		t.Fatalf("failed to build a deployment_settings types.Object: %v", diags)
	}
	return obj
}

// mustEndUserExperienceObject is the end_user_experience counterpart to
// mustDeploymentObject.
func mustEndUserExperienceObject(t *testing.T, m EndUserExperienceModel) types.Object {
	t.Helper()
	obj, diags := types.ObjectValueFrom(context.Background(), endUserExperienceAttrTypes, m)
	if diags.HasError() {
		t.Fatalf("failed to build an end_user_experience types.Object: %v", diags)
	}
	return obj
}

// currentServerSettings returns the settings body a GA tenant was probed returning,
// standing in for the merge base a real Update reads before it writes.
func currentServerSettings() *pro.AppInstallersGlobalSettings {
	batchSize := 500
	interval := int64(4)
	deadline := int64(48)
	relaunch := false
	suppress := true
	return &pro.AppInstallersGlobalSettings{
		DeploymentProcessControls: &pro.AppInstallersDeploymentProcessControls{
			CommandsBatchSize: &batchSize,
		},
		EndUserExperienceSettings: pro.GlobalSettingsEndUserExperience{
			NotificationInterval: &interval,
			Deadline:             &deadline,
			Relaunch:             &relaunch,
			Suppress:             &suppress,
		},
	}
}

// assertDeploymentPreserved fails unless the merged payload carries the merge base's
// deployment controls through unchanged.
func assertDeploymentPreserved(t *testing.T, got, current *pro.AppInstallersGlobalSettings) {
	t.Helper()
	if got.DeploymentProcessControls == nil {
		t.Fatal("an omitted deployment_settings block must pass the server's controls through, got nil (the server reads an omitted key as no change, but a nil pointer drops the merge base)")
	}
	if got.DeploymentProcessControls.CommandsBatchSize == nil {
		t.Fatal("an omitted deployment_settings block must preserve CommandsBatchSize, got nil")
	}
	if *got.DeploymentProcessControls.CommandsBatchSize != *current.DeploymentProcessControls.CommandsBatchSize {
		t.Errorf("CommandsBatchSize = %d, want the server's %d", *got.DeploymentProcessControls.CommandsBatchSize, *current.DeploymentProcessControls.CommandsBatchSize)
	}
}

// assertEndUserExperiencePreserved fails unless the merged payload carries the merge
// base's end-user-experience block through unchanged. The block has no omitted form
// on the wire, so a dropped merge base sends the all-null block the server reads as
// "clear every field" — which is exactly the silent wipe this asserts against.
func assertEndUserExperiencePreserved(t *testing.T, got, current *pro.AppInstallersGlobalSettings) {
	t.Helper()
	if got.EndUserExperienceSettings == (pro.GlobalSettingsEndUserExperience{}) {
		t.Fatal("an omitted end_user_experience block must pass the server's settings through, got the all-null block the server reads as a full clear")
	}
	if got.EndUserExperienceSettings.NotificationInterval == nil {
		t.Fatal("an omitted end_user_experience block must preserve NotificationInterval, got nil")
	}
	if *got.EndUserExperienceSettings.NotificationInterval != *current.EndUserExperienceSettings.NotificationInterval {
		t.Errorf("NotificationInterval = %d, want the server's %d", *got.EndUserExperienceSettings.NotificationInterval, *current.EndUserExperienceSettings.NotificationInterval)
	}
	if got.EndUserExperienceSettings.Deadline == nil || *got.EndUserExperienceSettings.Deadline != *current.EndUserExperienceSettings.Deadline {
		t.Errorf("Deadline = %v, want the server's %d", got.EndUserExperienceSettings.Deadline, *current.EndUserExperienceSettings.Deadline)
	}
	if got.EndUserExperienceSettings.Suppress == nil || *got.EndUserExperienceSettings.Suppress != *current.EndUserExperienceSettings.Suppress {
		t.Errorf("Suppress = %v, want the server's %t", got.EndUserExperienceSettings.Suppress, *current.EndUserExperienceSettings.Suppress)
	}
}

// TestBuildMergedInput_BothBlocksOmitted verifies the "omit = don't manage" contract
// on both blocks at once: a plan naming neither block must write the merge base back
// verbatim rather than clearing the tenant's settings.
func TestBuildMergedInput_BothBlocksOmitted(t *testing.T) {
	current := currentServerSettings()
	plan := AppInstallerSettingsResourceModel{
		DeploymentSettings: types.ObjectNull(deploymentSettingsAttrTypes),
		EndUserExperience:  types.ObjectNull(endUserExperienceAttrTypes),
	}

	got := buildMergedInput(current, plan)

	assertDeploymentPreserved(t, got, current)
	assertEndUserExperiencePreserved(t, got, current)
}

// TestBuildMergedInput_DeploymentManagedEndUserExperienceOmitted verifies the two
// blocks are merged independently: the managed block reflects the plan while the
// omitted one still reflects the server.
func TestBuildMergedInput_DeploymentManagedEndUserExperienceOmitted(t *testing.T) {
	current := currentServerSettings()
	plan := AppInstallerSettingsResourceModel{
		DeploymentSettings: mustDeploymentObject(t, DeploymentSettingsModel{
			BatchSize:      types.Int64Value(250),
			BatchFrequency: types.Int64Value(30),
			Days: types.SetValueMust(types.StringType, []attr.Value{
				types.StringValue("TUESDAY"),
			}),
			ServerTimeFrom: types.StringValue("09:00:00Z"),
			ServerTimeTo:   types.StringValue("17:00:00Z"),
		}),
		EndUserExperience: types.ObjectNull(endUserExperienceAttrTypes),
	}

	got := buildMergedInput(current, plan)

	if got.DeploymentProcessControls == nil {
		t.Fatal("a managed deployment_settings block must produce controls, got nil")
	}
	if got.DeploymentProcessControls.CommandsBatchSize == nil || *got.DeploymentProcessControls.CommandsBatchSize != 250 {
		t.Errorf("CommandsBatchSize = %v, want the plan's 250", got.DeploymentProcessControls.CommandsBatchSize)
	}
	if got.DeploymentProcessControls.FromTimeOfDay == nil || *got.DeploymentProcessControls.FromTimeOfDay != "09:00:00Z" {
		t.Errorf("FromTimeOfDay = %v, want the plan's 09:00:00Z", got.DeploymentProcessControls.FromTimeOfDay)
	}
	assertEndUserExperiencePreserved(t, got, current)
}

// TestBuildMergedInput_EndUserExperienceManagedDeploymentOmitted is the mirror of
// TestBuildMergedInput_DeploymentManagedEndUserExperienceOmitted.
func TestBuildMergedInput_EndUserExperienceManagedDeploymentOmitted(t *testing.T) {
	current := currentServerSettings()
	plan := AppInstallerSettingsResourceModel{
		DeploymentSettings: types.ObjectNull(deploymentSettingsAttrTypes),
		EndUserExperience: mustEndUserExperienceObject(t, EndUserExperienceModel{
			NotificationFrequency: types.Int64Value(8),
			NotificationMessage:   types.StringValue("An update is ready to install"),
			UpdateDeadline:        types.Int64Value(24),
			ForceQuitMessage:      types.StringNull(),
			ForceQuitGracePeriod:  types.Int64Null(),
			UpdateCompleteMessage: types.StringNull(),
			Relaunch:              types.BoolValue(true),
			Suppress:              types.BoolValue(false),
		}),
	}

	got := buildMergedInput(current, plan)

	if got.EndUserExperienceSettings.NotificationInterval == nil || *got.EndUserExperienceSettings.NotificationInterval != 8 {
		t.Errorf("NotificationInterval = %v, want the plan's 8", got.EndUserExperienceSettings.NotificationInterval)
	}
	if got.EndUserExperienceSettings.Suppress == nil || *got.EndUserExperienceSettings.Suppress {
		t.Errorf("Suppress = %v, want the plan's false", got.EndUserExperienceSettings.Suppress)
	}
	assertDeploymentPreserved(t, got, current)
}

// TestBuildMergedInput_UnknownBlocksPreserveCurrent covers the plan shape an omitted
// Optional+Computed block actually takes before the state-for-unknown modifier
// resolves it: unknown, not null. Both spellings must pass the merge base through.
func TestBuildMergedInput_UnknownBlocksPreserveCurrent(t *testing.T) {
	current := currentServerSettings()
	plan := AppInstallerSettingsResourceModel{
		DeploymentSettings: types.ObjectUnknown(deploymentSettingsAttrTypes),
		EndUserExperience:  types.ObjectUnknown(endUserExperienceAttrTypes),
	}

	got := buildMergedInput(current, plan)

	assertDeploymentPreserved(t, got, current)
	assertEndUserExperiencePreserved(t, got, current)
}
