// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// policyTimeoutAttributeTypes defines the timeout attribute types for the
// policy resource operations.
var policyTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// packageActions is the accepted packages[].action set: the SDK's vocabulary
// plus "Uninstall", which the classic spec omits. Jamf Pro documents Uninstall
// as a Packages payload action (it works only for packages indexed by the
// retired Jamf Admin app), so leaving it out would reject a configuration the
// admin UI offers. The server stores any string here verbatim — "bogus" and
// "install" both persist, wire-probed 2026-10-01 — so plan time is the only
// place a typo is caught.
var packageActions = append(proclassic.PolicyPackageConfigurationPackagesPackageItemActionValues(), packageActionUninstall)

const packageActionUninstall = "Uninstall"

// diskEncryptionActions is the accepted disk_encryption.action set: the SDK's
// vocabulary plus "none", which the classic spec omits. "none" is what a fresh
// policy reads back and the only value that turns the payload off; any value
// Jamf Pro does not recognise, including "Apply", is silently stored as
// "none", wire-probed 2026-10-01.
var diskEncryptionActions = append(proclassic.PolicyPostDiskEncryptionActionValues(), diskEncryptionActionNone)

const diskEncryptionActionNone = "none"

// Restart actions the admin UI offers under Options ▸ Restart Options. The
// classic spec models both fields as plain strings, so the SDK has no constant
// for any of them. The two sets differ: "Restart" (which waits for the user)
// exists only while a user is logged in. Jamf Pro answers a 201 for any other
// value — wrong case, empty, or "Restart" on no_user_logged_in included — and
// keeps the previous one, wire-probed 2026-10-01 against 11.32.0.
const (
	restartActionDoNotRestart = "Do not restart"
	restartActionRestart      = "Restart"
	restartActionIfRequired   = "Restart if a package or update requires it"
	restartActionImmediately  = "Restart immediately"
)

var noUserLoggedInRestartActions = []string{restartActionDoNotRestart, restartActionImmediately, restartActionIfRequired}

var userLoggedInRestartActions = []string{restartActionDoNotRestart, restartActionRestart, restartActionIfRequired, restartActionImmediately}

// markdownValueList renders enum values as a backticked, comma-separated list
// for a MarkdownDescription, so the documented values and the OneOf validator
// come from one slice and cannot drift apart.
func markdownValueList(vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = "`" + v + "`"
	}
	return strings.Join(quoted, ", ")
}
