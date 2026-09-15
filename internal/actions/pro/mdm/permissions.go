// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	devSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// Each MDM command action declares the SDK methods it reaches at Invoke time.
// send_blank_push routes its serial-number resolution through the shared helpers
// in helpers.go (resolveSerialNumbers -> ListDevices); the other two call the SDK
// directly. The declared list is the full reachable set so the rendered
// "Required Jamf permissions" table is honest; permissions_test.go asserts each
// list stays in sync with the methods reachable from the action's own file
// (direct calls plus the helpers it invokes) and with the SDK privilege
// registries.

// resolveSerialMerged is the registry for actions that resolve a serial number
// through the Platform devices inventory in addition to calling Jamf Pro.
var resolveSerialMerged = permissions.Merge(pro.Privileges, devSDK.Privileges)

// sendBlankPush issues a blank-push command and resolves any serial numbers
// directly in its own file.
var sendBlankPushSDKMethods = []string{
	"SendMdmBlankPushV2",
	"ListDevices",
}
var sendBlankPushPrivileges = permissions.Section(resolveSerialMerged, sendBlankPushSDKMethods...)

// renewMdmProfile calls the Pro renew-profile endpoint directly.
var renewMdmProfileSDKMethods = []string{
	"RenewMdmProfileV1",
}
var renewMdmProfilePrivileges = permissions.Section(pro.Privileges, renewMdmProfileSDKMethods...)

// flushMdmCommands calls the Classic command-flush endpoint directly.
var flushMdmCommandsSDKMethods = []string{
	"DeleteCommandFlushByIDTypeIDStatus",
}
var flushMdmCommandsPrivileges = permissions.Section(proclassic.Privileges, flushMdmCommandsSDKMethods...)
