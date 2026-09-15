// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package re_enrollment_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// reEnrollmentSettingsTimeoutAttributeTypes defines the timeout attribute types.
var reEnrollmentSettingsTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// clear_management_history enum values. These control how much of a device's
// pending/failed management command history is cleared when it re-enrolls.
const (
	clearManagementHistoryNothing                      = pro.EnrollmentSettingsV4FlushMDMCommandsOnReenrollDeleteNothing
	clearManagementHistoryErrors                       = pro.EnrollmentSettingsV4FlushMDMCommandsOnReenrollDeleteErrors
	clearManagementHistoryEverythingExceptAcknowledged = pro.EnrollmentSettingsV4FlushMDMCommandsOnReenrollDeleteEverythingExceptAcknowledged
	clearManagementHistoryEverything                   = pro.EnrollmentSettingsV4FlushMDMCommandsOnReenrollDeleteEverything
)

// validClearManagementHistory are the accepted clear_management_history values.
var validClearManagementHistory = []string{
	clearManagementHistoryNothing,
	clearManagementHistoryErrors,
	clearManagementHistoryEverythingExceptAcknowledged,
	clearManagementHistoryEverything,
}

// defaultClearManagementHistory matches the server-side default surfaced in the
// Re-enrollment page. The wire field carrying this value is always present in
// the request body (no omit), so the input builder substitutes this default
// when the user leaves clear_management_history unset rather than sending an
// empty string the server would reject.
const defaultClearManagementHistory = clearManagementHistoryEverythingExceptAcknowledged
