// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// lookupLAPSAccountGUID turns a username into the GUID of that account on one
// computer, through the LAPS accounts list. Jamf Pro queues
// SET_AUTO_ADMIN_PASSWORD for any GUID without checking it (wire-probed
// 2026-11-10: the all-zero GUID answered 201, as did an iPad), so a typo fails
// silently on the Mac; resolving here turns it into an error naming the device.
//
// It reports a problem as text rather than adding a diagnostic because callers
// run it concurrently across a group and gather every failure into one report.
func (a *mdmAction) lookupLAPSAccountGUID(ctx context.Context, managementID, username string) (guid, problem string) {
	list, err := a.client.ListLocalAdminPasswordAccountsV2(ctx, managementID)
	if err != nil {
		return "", fmt.Sprintf("unable to list the managed local administrator accounts: %s", helpers.APIErrorDetail(err))
	}
	return pickLAPSAccountGUID(list.Results, username)
}

// pickLAPSAccountGUID matches username exactly (macOS short names are case
// sensitive) and refuses to guess when the name is absent or appears under both
// account sources.
func pickLAPSAccountGUID(accounts []pro.LapsUserV2, username string) (guid, problem string) {
	var matches []pro.LapsUserV2
	for _, acct := range accounts {
		if acct.Username == username {
			matches = append(matches, acct)
		}
	}

	switch len(matches) {
	case 1:
		if matches[0].Guid == "" {
			return "", fmt.Sprintf("account %q is listed without a GUID", username)
		}
		return matches[0].Guid, ""
	case 0:
		names := make([]string, 0, len(accounts))
		for _, acct := range accounts {
			names = append(names, fmt.Sprintf("%s (%s)", acct.Username, acct.UserSource))
		}
		sort.Strings(names)
		found := "none"
		if len(names) > 0 {
			found = strings.Join(names, ", ")
		}
		return "", fmt.Sprintf("no managed local administrator account named %q (Jamf Pro lists: %s)", username, found)
	default:
		sources := make([]string, 0, len(matches))
		for _, m := range matches {
			sources = append(sources, m.UserSource)
		}
		sort.Strings(sources)
		return "", fmt.Sprintf("account %q is listed %d times (%s), so it is ambiguous", username, len(matches), strings.Join(sources, ", "))
	}
}
