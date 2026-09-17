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

// The two identifiers, and why neither needs a name lookup.
//
// A Jamf Pro group has a Jamf Pro id (numeric, what every pro/ and classic route
// takes) and a platform id (a UUID, what the jamfplatform_device_* constructs
// key on). These constructs key on the Jamf Pro id and publish the UUID as
// Computed platform_id.
//
// Both come back from calls the resource already makes, so there is no bridging
// request on the happy path and no matching on names:
//
//   - Create passes `platform=true`, and the Pro create itself returns the UUID
//     in place of the Jamf Pro id. Wire-probed on all four creates, 2026-09-17.
//     This is the Pro endpoint's own response — not a call to /device-groups.
//   - Every /pro/v2 and /pro/v3 group route then accepts that UUID as {id}, so
//     the mandatory read-after-create runs against it unchanged and reports the
//     Jamf Pro id in its body. Three of the four do:
//     GET /v3/computer-groups/static-groups/{id} returns `id`, and both mobile
//     reads return `groupId`. The exception is the smart computer group, whose
//     GET returns name, description, criteria and siteId and NO identifier at
//     all — that one construct calls JamfProIDForPlatformID once on create.
//
// Only import and the plural read paths start from a Jamf Pro id with no UUID in
// hand, and PlatformIDsByJamfProID serves them from one request.
//
// What does NOT work, probed rather than assumed, because the obvious shapes all
// look plausible:
//
//   - GET /pro/v2/groups/{jamfProId} is 404. That route takes the UUID only.
//   - filter=groupJamfProId=="…" is 400 INVALID_FIELD. The refusal names the
//     filterable fields, and that list is the endpoint's own oracle — it carries
//     three fields the published spec and the SDK doc comment both omit
//     (groupId, siteId, groupPlatformId).
//   - Two of those three do not work. filter=groupId=="…" and
//     filter=siteId=="…" both answer 500 with an empty errors array, so the
//     oracle over-reports and a filter cannot be trusted because the server
//     listed it.
//   - filter=groupPlatformId=="…" is refused with "only supports =in= and =out=
//     operators"; the =in=(a,b) form does work, but it takes the UUID as input,
//     which is the value being looked for.
//
// So there is no route from a Jamf Pro id to a UUID, and the bridge instead
// narrows by group type and smartness — both documented and both verified — and
// matches on groupJamfProId in the provider. That is an exact match on an
// identifier, which matters: Jamf Pro's RSQL name filter is case-insensitive and
// treats * as a glob (STYLE_GUIDE §Referencing a server-managed catalog by name),
// so a name-based bridge could return a different group whose name differed only
// in case, and two groups of different types may legitimately share a name.

// forbiddenWarningKey and transientWarningKey latch the two advisories the bridge
// can raise, so a configuration holding forty of these groups reports each cause
// once per provider invocation rather than forty times. The strings are stable
// and shared by all sixteen constructs, which is the point — the operator has one
// thing to fix, not sixteen.
const (
	forbiddenWarningKey = "progroups.platform_id.forbidden"
	transientWarningKey = "progroups.platform_id.transient"
)

// JamfProIDForPlatformID reads a group's Jamf Pro id given its platform UUID.
//
// Only the smart computer group needs this: its own GET carries no identifier, so
// a create that asked for the UUID has no other way to learn the numeric id it
// must store as the resource's id. The other three read it out of the
// read-after-create they already perform.
//
// This one is NOT best-effort. The value becomes the resource id, so failing here
// means Create cannot record what it made, and returning a null id would orphan
// the group. The caller surfaces the error.
func JamfProIDForPlatformID(ctx context.Context, client *pro.Client, platformID string) (string, error) {
	if client == nil || platformID == "" {
		return "", fmt.Errorf("resolving the Jamf Pro identifier: no platform identifier to resolve")
	}
	group, err := client.GetGroupV2(ctx, platformID)
	if err != nil {
		return "", err
	}
	if group == nil || group.GroupJamfProID == "" {
		return "", fmt.Errorf("resolving the Jamf Pro identifier for platform identifier %s: Jamf Pro reported the group without one", platformID)
	}
	return group.GroupJamfProID, nil
}

// PlatformIDsByJamfProID returns platform UUIDs keyed by Jamf Pro id for every
// group of one type and smartness, in a single request.
//
// It serves the paths that hold a Jamf Pro id and no UUID: import, the plural
// data sources and the list resources. One request covers a whole page rather
// than one per group, because the filter narrows to the construct's own kind and
// the caller then looks each id up in the returned map.
//
// Best-effort by design, and the caller must tolerate a missing key. Every
// failure yields (partial-or-empty map, warning) rather than an error, so a Read
// that has already fetched the group is never discarded over an attribute that
// exists only to reference the group from a Jamf Platform construct — the same
// contract device_group.resolveJamfProID keeps bridging these two id spaces in
// the other direction.
func PlatformIDsByJamfProID(ctx context.Context, client *pro.Client, pd *providerdata.Data, dt DeviceType, kind GroupKind) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if client == nil {
		return nil, diags
	}
	filter := fmt.Sprintf("groupType==%q and isSmart==%q", groupTypeFilterValue(dt), smartFilterValue(kind))
	groups, err := client.ListGroupsV2(ctx, nil, filter)
	if err != nil {
		return nil, bridgeFailureDiagnostics(ctx, pd, err, filter)
	}
	out := make(map[string]string, len(groups))
	for _, g := range groups {
		if g.GroupJamfProID == "" || g.GroupPlatformID == "" {
			continue
		}
		out[g.GroupJamfProID] = g.GroupPlatformID
	}
	return out, diags
}

// PlatformIDValue renders a looked-up platform id for state, nulling a miss.
//
// A group absent from the bridge's map is reported as null rather than as an
// error: platform_id is only needed to reference the group from a Jamf Platform
// construct, so a gap costs nothing else and resolves on a later refresh.
func PlatformIDValue(byJamfProID map[string]string, jamfProID string) types.String {
	if id, ok := byJamfProID[jamfProID]; ok && id != "" {
		return types.StringValue(id)
	}
	return types.StringNull()
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
func bridgeFailureDiagnostics(ctx context.Context, pd *providerdata.Data, err error, filter string) diag.Diagnostics {
	var diags diag.Diagnostics
	switch {
	case helpers.IsForbiddenError(err) && !helpers.IsEdgeBlocked(err):
		if pd.FiredOnce(forbiddenWarningKey) {
			diags.AddWarning(
				"API integration lacks the Device groups Read permission; platform_id will be null.",
				"The provider tried to resolve the identifier the `jamfplatform_device_*` constructs use for one or more Jamf Pro groups and was refused. In Jamf Account, grant the integration Inventory → Device groups → Read to populate `platform_id` on later refreshes. Nothing else about these groups is affected — `platform_id` is only needed to reference one from a Jamf Platform construct.",
			)
		}
	default:
		if pd.FiredOnce(transientWarningKey) {
			diags.AddWarning(
				"Failed to resolve Jamf Platform identifiers; platform_id will be null.",
				"The provider tried to resolve the identifiers the `jamfplatform_device_*` constructs use for one or more Jamf Pro groups and the lookup failed: "+helpers.APIErrorDetail(err)+
					"\n\nThis is treated as a transient failure so the operation that triggered it is not rolled back; later refreshes retry.",
			)
		}
	}
	tflog.Debug(ctx, "platform_id bridge failed; platform_id will be null", map[string]any{
		"filter": filter,
		"error":  err.Error(),
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

// smartFilterValue is the isSmart the /pro/v2/groups filter expects for a group
// kind. The field is a boolean the endpoint takes quoted, and the SDK generates
// no vocabulary for a bare JSON boolean.
func smartFilterValue(kind GroupKind) string {
	if kind == GroupKindStatic {
		return "false"
	}
	return "true"
}
