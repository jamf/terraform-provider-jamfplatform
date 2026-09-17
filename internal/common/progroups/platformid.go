// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// forbiddenWarningKey and transientWarningKey latch the two advisories this
// bridge can raise, so a configuration holding forty of these groups reports
// each cause once per provider invocation rather than forty times. The strings
// are stable and shared by all four constructs and their data sources and list
// resources, which is the point — the operator has one thing to fix, not sixteen.
const (
	forbiddenWarningKey  = "progroups.platform_id.forbidden"
	transientWarningKey  = "progroups.platform_id.transient"
	ambiguousWarningKey  = "progroups.platform_id.ambiguous"
	unresolvedWarningKey = "progroups.platform_id.unresolved"
)

// ResolvePlatformID best-effort populates a group's platform_id — the identifier
// the jamfplatform_device_* constructs key on — from the Jamf Pro id these
// constructs key on.
//
// It is the mirror image of device_group's resolveJamfProID, which bridges the
// same two id spaces in the other direction, and it degrades the same way: every
// failure yields (null, warning) rather than an error, so a Create or Update that
// has already written the group is never discarded over a bridging call. A caller
// must tolerate a null result.
//
// The lookup is a filtered list rather than a direct read because /pro/v2/groups
// has no by-Jamf-Pro-id route — it answers on the platform id, which is the value
// being looked for. So the filter narrows the request by name and device type,
// and the match is then decided in the provider on byte equality of the name AND
// equality of groupJamfProId. That second test is what makes the result exact:
// Jamf Pro's RSQL name filter is case-insensitive and treats * as a glob (see
// STYLE_GUIDE §Referencing a server-managed catalog by name), so the filter alone
// could return a different group whose name differs only in case, and two groups
// of different device types may legitimately share a name. Confirming the id the
// caller already holds removes both.
//
// Anything other than exactly one confirmed match resolves to null with a latched
// warning rather than guessing.
func ResolvePlatformID(ctx context.Context, client *pro.Client, pd *providerdata.Data, dt DeviceType, jamfProID, name string) (types.String, diag.Diagnostics) {
	var diags diag.Diagnostics
	if client == nil || pd == nil || jamfProID == "" || name == "" {
		return types.StringNull(), diags
	}

	filter := fmt.Sprintf("groupName==%q and groupType==%q", name, groupTypeFilterValue(dt))
	found, err := client.ListGroupsV2(ctx, nil, filter)
	if err != nil {
		return types.StringNull(), bridgeFailureDiagnostics(ctx, pd, err, jamfProID)
	}

	var matches []pro.GroupDtoV1
	for _, g := range found {
		if g.GroupName == name && g.GroupJamfProID == jamfProID {
			matches = append(matches, g)
		}
	}

	switch len(matches) {
	case 1:
		if matches[0].GroupPlatformID == "" {
			return types.StringNull(), diags
		}
		return types.StringValue(matches[0].GroupPlatformID), diags
	case 0:
		if pd.FiredOnce(unresolvedWarningKey) {
			diags.AddWarning(
				"Could not match a Jamf Platform identifier to one or more groups; platform_id will be null.",
				"The provider looked a group up by name and Jamf Pro identifier to find the identifier the `jamfplatform_device_*` constructs use, and found no group matching both. This does not affect the group itself, and `platform_id` is only needed to reference it from a Jamf Platform construct. It resolves on a later apply if the mismatch was momentary.",
			)
		}
		tflog.Debug(ctx, "no platform group matched the Jamf Pro id and name; nulling platform_id", map[string]any{
			"jamf_pro_id": jamfProID,
		})
		return types.StringNull(), diags
	default:
		if pd.FiredOnce(ambiguousWarningKey) {
			diags.AddWarning(
				"More than one group matched while resolving a Jamf Platform identifier; platform_id will be null.",
				"Two or more groups report the same name and the same Jamf Pro identifier, so the provider cannot tell which Jamf Platform identifier belongs to this group and leaves `platform_id` null rather than guessing. Please report this to the provider developers — it should not be possible.",
			)
		}
		return types.StringNull(), diags
	}
}

// bridgeFailureDiagnostics classifies a failed /pro/v2/groups read.
//
// The forbidden branch is gated on helpers.IsEdgeBlocked being false, as
// STYLE_GUIDE requires of anything that classifies a status itself: a CDN, WAF or
// IP allowlist serves a 403 of its own, and naming a Jamf permission for one
// sends the operator to re-grant a privilege that was never missing while the
// real cause goes unreported. Such a reply falls to the transient branch, whose
// detail renders through helpers.APIErrorDetail and so carries the egress-IP
// guidance.
func bridgeFailureDiagnostics(ctx context.Context, pd *providerdata.Data, err error, jamfProID string) diag.Diagnostics {
	var diags diag.Diagnostics
	switch {
	case helpers.IsForbiddenError(err) && !helpers.IsEdgeBlocked(err):
		if pd.FiredOnce(forbiddenWarningKey) {
			diags.AddWarning(
				"API integration lacks the Device groups Read permission; platform_id will be null.",
				"The provider tried to resolve the identifier the `jamfplatform_device_*` constructs use for one or more Jamf Pro groups and was refused. In Jamf Account, grant the integration Inventory → Device groups → Read to populate `platform_id` on later applies. Nothing else about these groups is affected — `platform_id` is only needed to reference one from a Jamf Platform construct.",
			)
		}
	default:
		if pd.FiredOnce(transientWarningKey) {
			diags.AddWarning(
				"Failed to resolve a Jamf Platform identifier; platform_id will be null.",
				"The provider tried to resolve the identifier the `jamfplatform_device_*` constructs use for one or more Jamf Pro groups and the lookup failed: "+helpers.APIErrorDetail(err)+
					"\n\nThis is treated as a transient failure so the group operation that triggered it is not rolled back; later applies retry.",
			)
		}
	}
	tflog.Debug(ctx, "platform_id bridge failed; nulling platform_id", map[string]any{
		"jamf_pro_id": jamfProID,
		"error":       err.Error(),
	})
	return diags
}

// groupTypeFilterValue is the groupType the /pro/v2/groups filter expects for a
// device kind. The values come from the SDK's generated vocabulary rather than
// being restated: Jamf documents the filter as case-sensitive on this field, so a
// literal that drifted would silently match nothing and null every platform_id.
func groupTypeFilterValue(dt DeviceType) string {
	if dt == DeviceTypeMobile {
		return pro.GroupDtoV1GroupTypeMobile
	}
	return pro.GroupDtoV1GroupTypeComputer
}
