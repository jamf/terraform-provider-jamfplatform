// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// deploymentTypeSelfService and deploymentTypeAutomatic are the two wire values
// the classic /macapplications endpoint accepts for general.deployment_type.
// The literals must match the server bytes exactly, including the slash.
const (
	deploymentTypeSelfService = "Make Available in Self Service"
	deploymentTypeAutomatic   = "Install Automatically/Prompt Users to Install"
)

// extractMacAppID returns the assigned ID as a string from a Create/GET
// response. Classic returns the ID at the top level (<mac_application><id>) and
// echoes it inside <general>. Prefer the top-level reading, fall back to general.
func extractMacAppID(a *proclassic.MacApplication) string {
	if a == nil {
		return ""
	}
	if a.ID != nil {
		return strconv.Itoa(*a.ID)
	}
	if a.General != nil && a.General.ID != nil {
		return strconv.Itoa(*a.General.ID)
	}
	return ""
}

// buildMacAppNotification assembles the self_service notification_enabled /
// notification_method attributes into a single proclassic.NotificationValue.
// The classic wire carries two <notification> elements (a bool and a method
// string); NotificationValue.MarshalXML emits them method-first, the only
// order that round-trips (wire-probed). Returns nil when neither attribute is
// configured so the SDK omits the element entirely.
func buildMacAppNotification(enabled types.Bool, method types.String) *proclassic.NotificationValue {
	if !helpers.IsConfiguredValue(enabled) && !helpers.IsConfiguredValue(method) {
		return nil
	}
	n := &proclassic.NotificationValue{}
	if helpers.IsConfiguredValue(enabled) {
		v := enabled.ValueBool()
		n.Enabled = &v
	}
	if helpers.IsConfiguredValue(method) {
		m := method.ValueString()
		n.Method = &m
	}
	return n
}
