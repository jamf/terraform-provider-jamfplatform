// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// deploymentTypeSelfService and deploymentTypeAutomatic are the two wire values
// the classic /ebooks endpoint accepts for general.deployment_type (UI
// "Distribution Method"). They must match the server bytes exactly, including
// the slash — which is the argument for taking them from the SDK rather than
// retyping them.
const (
	deploymentTypeSelfService = proclassic.EbookGeneralDeploymentTypeMakeAvailableInSelfService
	deploymentTypeAutomatic   = proclassic.EbookGeneralDeploymentTypeInstallAutomaticallyPromptUsersToInstall
)

// extractEbookID returns the assigned ID as a string from a Create/GET
// response. Classic returns the ID at the top level (<ebook><id>) and echoes it
// inside <general>. Prefer the top-level reading, fall back to general.
func extractEbookID(e *proclassic.Ebook) string {
	if e == nil {
		return ""
	}
	if e.ID != nil {
		return strconv.Itoa(*e.ID)
	}
	if e.General != nil && e.General.ID != nil {
		return strconv.Itoa(*e.General.ID)
	}
	return ""
}

// buildEbookNotification assembles the self_service notification_enabled /
// notification_method attributes into a single proclassic.NotificationValue.
// The classic wire carries two <notification> elements (a bool and a method
// string); NotificationValue.MarshalXML emits them method-first, the only order
// that round-trips (wire-probed — the live ebook GET returns
// <notification>false</notification><notification>Self Service</notification>).
// Returns nil when neither attribute is configured so the SDK omits the element.
func buildEbookNotification(enabled types.Bool, method types.String) *proclassic.NotificationValue {
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

// isAcceptedAsyncDelete reports whether err is the classic /ebooks DELETE
// reporting an accepted deletion through a misleading client error, rather than
// a genuine refusal.
//
// The endpoint answers an accepted async delete with a 4xx (wire-probed), so the
// switch in Delete treats a client error as success-with-a-warning. Three 4xx
// replies must not take that branch. A gateway-unrouted 404 means the request
// reached no Jamf service, so nothing was accepted and clearing state would
// leave a live ebook unmanaged — see helpers.IsGatewayUnrouted. An edge error
// page is the same fault one layer out, and CloudFront serves one with a 404
// among other statuses, so it clears state just as wrongly — see
// helpers.IsEdgeBlocked. A 403 is the integration lacking the delete privilege,
// which is a refusal the operator has to act on.
func isAcceptedAsyncDelete(err error) bool {
	return helpers.IsClientError(err) &&
		!helpers.IsGatewayUnrouted(err) &&
		!helpers.IsEdgeBlocked(err) &&
		!helpers.IsForbiddenError(err)
}
