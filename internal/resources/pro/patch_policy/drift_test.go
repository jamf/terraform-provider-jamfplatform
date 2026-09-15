// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAssignPatchPolicy_ReportsDrift pins the wire-authoritative read: an
// echoed value that differs from state must land in state so `terraform plan`
// reports the change. Every field asserted here round-trips through the classic
// /patchpolicies GET, software_title_configuration_id included — re-probed
// against Jamf Pro 11.31.1 on 2026-09-06, correcting an earlier recorded
// finding that the GET dropped it. See issue #387.
func TestAssignPatchPolicy_ReportsDrift(t *testing.T) {
	t.Parallel()
	state := &PatchPolicyResourceModel{
		SoftwareTitleConfigurationID: types.StringValue("9"),
		Name:                         types.StringValue("state name"),
		TargetVersion:                types.StringValue("1.0"),
		DistributionMethod:           types.StringValue("prompt"),
		Enabled:                      types.BoolValue(false),
		AllowDowngrade:               types.BoolValue(false),
		PatchUnknown:                 types.BoolValue(false),
		UserInteraction: &PatchPolicyUserInteractionModel{
			InstallButtonText:      types.StringValue("state install"),
			SelfServiceDescription: types.StringValue("state desc"),
			SelfServiceIconID:      types.StringValue("1"),
			Deadlines:              &PatchPolicyUserInteractionDeadlinesModel{Enabled: types.BoolValue(false), Period: types.Int64Value(1)},
			GracePeriod: &PatchPolicyUserInteractionGracePeriodModel{
				Duration:                  types.Int64Value(1),
				NotificationCenterSubject: types.StringValue("state gp subject"),
				Message:                   types.StringValue("state gp message"),
			},
		},
	}
	diags := assignPatchPolicyResourceModel(context.Background(), state, &proclassic.PatchPolicy{
		SoftwareTitleConfigurationID: new(1),
		General: &proclassic.PatchPolicyGeneral{
			Name:               new("wire name"),
			TargetVersion:      new("2.0"),
			DistributionMethod: new("selfservice"),
			Enabled:            new(true),
			AllowDowngrade:     new(true),
			PatchUnknown:       new(true),
		},
		UserInteraction: &proclassic.PatchPolicyUserInteraction{
			InstallButtonText:      new("wire install"),
			SelfServiceDescription: new("wire desc"),
			SelfServiceIcon:        &proclassic.PatchPolicyUserInteractionSelfServiceIcon{ID: new(237)},
			Deadlines:              &proclassic.PatchPolicyUserInteractionDeadlines{DeadlineEnabled: new(true), DeadlinePeriod: new(4)},
			GracePeriod: &proclassic.PatchPolicyUserInteractionGracePeriod{
				GracePeriodDuration:       new(17),
				NotificationCenterSubject: new("wire gp subject"),
				Message:                   new("wire gp message"),
			},
		},
	}, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	ui := state.UserInteraction
	for _, tc := range []struct{ name, want, got string }{
		{"software_title_configuration_id", "1", state.SoftwareTitleConfigurationID.ValueString()},
		{"name", "wire name", state.Name.ValueString()},
		{"target_version", "2.0", state.TargetVersion.ValueString()},
		{"distribution_method", "selfservice", state.DistributionMethod.ValueString()},
		{"user_interaction.install_button_text", "wire install", ui.InstallButtonText.ValueString()},
		{"user_interaction.self_service_description", "wire desc", ui.SelfServiceDescription.ValueString()},
		{"user_interaction.self_service_icon_id", "237", ui.SelfServiceIconID.ValueString()},
		{"user_interaction.grace_period.notification_center_subject", "wire gp subject", ui.GracePeriod.NotificationCenterSubject.ValueString()},
		{"user_interaction.grace_period.message", "wire gp message", ui.GracePeriod.Message.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
	for _, tc := range []struct {
		name string
		got  types.Bool
	}{
		{"enabled", state.Enabled},
		{"allow_downgrade", state.AllowDowngrade},
		{"patch_unknown", state.PatchUnknown},
		{"user_interaction.deadlines.enabled", ui.Deadlines.Enabled},
	} {
		if !tc.got.ValueBool() {
			t.Errorf("%s: wire value must win, got false", tc.name)
		}
	}
	if ui.Deadlines.Period.ValueInt64() != 4 {
		t.Errorf("user_interaction.deadlines.period: wire value must win, got %d", ui.Deadlines.Period.ValueInt64())
	}
	if ui.GracePeriod.Duration.ValueInt64() != 17 {
		t.Errorf("user_interaction.grace_period.duration: wire value must win, got %d", ui.GracePeriod.Duration.ValueInt64())
	}
}

// TestFlattenUserInteraction_StickyNotificationsIgnoreDrift pins the other half
// of the #387 split: <notifications> and every child of it, <reminders>
// included, are absent from the GET even immediately after the POST that stored
// them, so the whole sub-block keeps the value already in state. The empty
// PatchPolicyUserInteraction passed in is that shape: no <notifications> child
// at all.
func TestFlattenUserInteraction_StickyNotificationsIgnoreDrift(t *testing.T) {
	t.Parallel()
	state := &PatchPolicyUserInteractionModel{
		Notifications: &PatchPolicyUserInteractionNotificationsModel{
			Enabled: types.BoolValue(true),
			Subject: types.StringValue("state subject"),
			Message: types.StringValue("state message"),
			Type:    types.StringValue("Self Service"),
			Reminders: &PatchPolicyUserInteractionNotificationsRemindersModel{
				Enabled:   types.BoolValue(true),
				Frequency: types.Int64Value(2),
			},
		},
	}
	flattenUserInteraction(&proclassic.PatchPolicyUserInteraction{}, state, false)
	n := state.Notifications
	for _, tc := range []struct{ name, want, got string }{
		{"subject", "state subject", n.Subject.ValueString()},
		{"message", "state message", n.Message.ValueString()},
		{"type", "Self Service", n.Type.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("notifications.%s: toggle off, so state must be kept as %q, got %q", tc.name, tc.want, tc.got)
		}
	}
	if !n.Enabled.ValueBool() {
		t.Error("notifications.enabled: toggle off, so state must be kept as true")
	}
	if !n.Reminders.Enabled.ValueBool() {
		t.Error("notifications.reminders.enabled: toggle off, so state must be kept as true")
	}
	if n.Reminders.Frequency.ValueInt64() != 2 {
		t.Errorf("notifications.reminders.frequency: toggle off, so state must be kept as 2, got %d", n.Reminders.Frequency.ValueInt64())
	}
}

// TestFlattenUserInteraction_NotificationDriftWhenEchoed pins the split inside
// the notifications sub-block. While the tenant-level Self Service
// notifications toggle is on the GET returns notification_enabled,
// notification_subject and both reminders fields, so a divergent wire value
// must win and drift is reported. notification_message and notification_type
// are dropped even then — a GET taken straight after the POST that set all four
// returned the other three and omitted exactly these two — so they keep the
// value already in state.
func TestFlattenUserInteraction_NotificationDriftWhenEchoed(t *testing.T) {
	t.Parallel()
	state := &PatchPolicyUserInteractionModel{
		Notifications: &PatchPolicyUserInteractionNotificationsModel{
			Enabled: types.BoolValue(true),
			Subject: types.StringValue("state subject"),
			Message: types.StringValue("state message"),
			Type:    types.StringValue("Self Service"),
			Reminders: &PatchPolicyUserInteractionNotificationsRemindersModel{
				Enabled:   types.BoolValue(true),
				Frequency: types.Int64Value(2),
			},
		},
	}
	flattenUserInteraction(&proclassic.PatchPolicyUserInteraction{
		Notifications: &proclassic.PatchPolicyUserInteractionNotifications{
			NotificationEnabled: new(false),
			NotificationSubject: new("wire subject"),
			Reminders: &proclassic.PatchPolicyUserInteractionNotificationsReminders{
				NotificationRemindersEnabled:  new(false),
				NotificationReminderFrequency: new(9),
			},
		},
	}, state, false)

	n := state.Notifications
	if got := n.Subject.ValueString(); got != "wire subject" {
		t.Errorf("notifications.subject: wire value must win, got %q", got)
	}
	if n.Enabled.ValueBool() {
		t.Error("notifications.enabled: wire false must win over state true")
	}
	if n.Reminders.Enabled.ValueBool() {
		t.Error("notifications.reminders.enabled: wire false must win over state true")
	}
	if n.Reminders.Frequency.ValueInt64() != 9 {
		t.Errorf("notifications.reminders.frequency: wire value must win, got %d", n.Reminders.Frequency.ValueInt64())
	}
	if got := n.Message.ValueString(); got != "state message" {
		t.Errorf("notifications.message: never echoed, must keep state, got %q", got)
	}
	if got := n.Type.ValueString(); got != "Self Service" {
		t.Errorf("notifications.type: never echoed, must keep state, got %q", got)
	}
}
