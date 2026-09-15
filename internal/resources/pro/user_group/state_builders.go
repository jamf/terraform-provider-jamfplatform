// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignUserGroupResourceModel populates a resource model from a UserGroup
// response. Members are derived from the response's <users> block when the
// group is static and the caller has indicated members are managed
// (manageMembers=true). Smart groups always set members=null in state — the
// server-resolved user list is informational and would cause endless drift
// if surfaced as a managed attribute.
//
// hydrating releases the manageMembers ownership gate on first-time import,
// where members arrives null and would otherwise stay null however many users
// the group holds — leaving a declared members list planning as an addition on
// the first plan after import. It adopts the list only when the server actually
// returns members: adopting an empty one would store [] where a create that
// never declared the attribute stores null, which breaks ImportStateVerify.
// See importHydration for the signal.
func assignUserGroupResourceModel(
	ctx context.Context,
	state *UserGroupResourceModel,
	ug *proclassic.UserGroup,
	manageMembers bool,
	hydrating bool,
) diag.Diagnostics {
	var diags diag.Diagnostics
	if ug == nil {
		return diags
	}

	if ug.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(ug.ID)
	}
	if ug.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(ug.Name)
	}
	state.GroupType = groupTypeFromIsSmart(ug.IsSmart)
	// notify_on_membership_change is Optional+Computed with a static default.
	// Server is authoritative: take the API value directly.
	// ReconcileOptionalBoolPointer would return null when current is null
	// (import path), causing ImportStateVerify drift even though the API
	// returned the value.
	state.NotifyOnMembershipChange = helpers.BoolPointerValueOrNull(ug.IsNotifyOnChange)

	siteID, siteName := scope.FlattenSiteObject(ug.Site)
	state.SiteID = helpers.ReconcileOptionalStringPointer(siteID, state.SiteID)
	state.SiteName = helpers.StringPointerValueOrNull(siteName)

	state.Criteria = flattenCriteria(ug.Criteria)

	memberIDs, memberCount := flattenMembers(ug.Users)
	state.MemberCount = types.Int64Value(int64(memberCount))

	isSmart := ug.IsSmart != nil && *ug.IsSmart
	switch {
	case isSmart:
		state.Members = types.SetNull(types.StringType)
	case manageMembers, hydrating && len(memberIDs) > 0:
		// Coerce nil → empty slice. types.SetValueFrom(ctx, T, nil) returns a
		// null set, which conflicts with an explicit empty-set plan (the user
		// transitioned members from non-empty to []) and surfaces as the
		// framework's "produced inconsistent result after apply" error. Mirror
		// the device_group state_builders.go pattern.
		if memberIDs == nil {
			memberIDs = []string{}
		}
		set, setDiags := types.SetValueFrom(ctx, types.StringType, memberIDs)
		diags.Append(setDiags...)
		if !diags.HasError() {
			state.Members = set
		}
	default:
		state.Members = types.SetNull(types.StringType)
	}

	return diags
}

// importHydration reports whether this Read is the first one to write state for
// the group, which is the only time the members ownership gate may be released.
//
// stateAbsent covers the identity-only import path (Terraform 1.12+), where the
// framework hands Read no prior state at all. The name check covers
// `terraform import <addr> <id>`, where ImportStatePassthroughID leaves a
// sparse-but-non-null state carrying only the id — so req.State.Raw.IsNull() is
// false and cannot be used on its own. name is schema-Required, so Create and
// Update always populate it before any Read; it can only be null here on a
// first-time hydration. Sample it before assignUserGroupResourceModel runs,
// which overwrites it from the wire.
func importHydration(stateAbsent bool, name types.String) bool {
	return stateAbsent || name.IsNull()
}

// assignUserGroupDataSourceModel populates a data source model from a UserGroup
// response. Symmetric with the resource builder but always surfaces the full
// user list as the Computed `users` block — DS users are read-only.
func assignUserGroupDataSourceModel(state *UserGroupDataSourceModel, ug *proclassic.UserGroup) {
	if ug == nil {
		return
	}
	if ug.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(ug.ID)
	}
	if ug.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(ug.Name)
	}
	state.GroupType = groupTypeFromIsSmart(ug.IsSmart)
	state.NotifyOnMembershipChange = helpers.BoolPointerValueOrNull(ug.IsNotifyOnChange)

	siteID, siteName := scope.FlattenSiteObject(ug.Site)
	state.SiteID = helpers.StringPointerValueOrNull(siteID)
	state.SiteName = helpers.StringPointerValueOrNull(siteName)

	state.Criteria = flattenCriteria(ug.Criteria)
	state.Users = flattenUsers(ug.Users)

	_, memberCount := flattenMembers(ug.Users)
	state.MemberCount = types.Int64Value(int64(memberCount))
}

// groupTypeFromIsSmart maps the SDK bool flag to the schema's discriminator string.
func groupTypeFromIsSmart(isSmart *bool) types.String {
	if isSmart == nil {
		return types.StringNull()
	}
	if *isSmart {
		return types.StringValue("smart")
	}
	return types.StringValue("static")
}

// flattenCriteria converts the SDK criteria wrapper into the Terraform model
// slice. Server is authoritative for every field on a criterion — Required
// (name, search_type, value), Computed-with-defaults (priority, and_or,
// has_opening_parenthesis, has_closing_parenthesis). No Reconcile* helpers
// here: Reconcile returns null when the API value is empty AND state was
// previously null, which is exactly the import path — and that mismatches
// the post-apply refresh path which sees the same API value but had a
// configured-default state. Direct copy keeps both paths in sync.
func flattenCriteria(wrapper *proclassic.UserGroupCriteria) []UserGroupCriterionModel {
	if wrapper == nil || wrapper.Criterion == nil || len(*wrapper.Criterion) == 0 {
		return nil
	}
	src := *wrapper.Criterion
	out := make([]UserGroupCriterionModel, len(src))
	for i, c := range src {
		// priority is Computed — server is authoritative and always returns it
		// (Jamf orders criteria by priority internally). Direct copy.
		priority := types.Int64Null()
		if c.Priority != nil {
			priority = types.Int64Value(int64(*c.Priority))
		}

		var name types.String
		if c.Name != nil && *c.Name != "" {
			name = types.StringValue(*c.Name)
		} else {
			name = types.StringNull()
		}

		var searchType types.String
		if c.SearchType != nil && *c.SearchType != "" {
			searchType = types.StringValue(*c.SearchType)
		} else {
			searchType = types.StringNull()
		}

		// criterion.value is Required and the server always returns the element,
		// empty for criteria like an unset "before (yyyy-mm-dd)" date. Copy it
		// verbatim (mapping a nil/empty echo to "" rather than null) so Required
		// stays satisfied and import lands the same value as a post-apply refresh.
		value := types.StringValue("")
		if c.Value != nil {
			value = types.StringValue(*c.Value)
		}

		var andOr types.String
		if c.AndOr != nil && *c.AndOr != "" {
			andOr = types.StringValue(*c.AndOr)
		} else {
			andOr = types.StringNull()
		}

		// has_opening_parenthesis / has_closing_parenthesis are Optional+Computed
		// with static defaults. Server is authoritative — take the API value
		// directly. ReconcileOptionalBoolPointer would return null on the import
		// path (current null) even when the API returned the value.
		out[i] = UserGroupCriterionModel{
			Priority:              priority,
			Name:                  name,
			SearchType:            searchType,
			Value:                 value,
			AndOr:                 andOr,
			HasOpeningParenthesis: helpers.BoolPointerValueOrNull(c.OpeningParen),
			HasClosingParenthesis: helpers.BoolPointerValueOrNull(c.ClosingParen),
		}
	}
	return out
}

// flattenMembers returns (user IDs as strings, count) from a UserGroupUsers
// wrapper. Order is preserved as returned by the API.
func flattenMembers(wrapper *proclassic.UserGroupUsers) ([]string, int) {
	if wrapper == nil || wrapper.User == nil {
		return nil, 0
	}
	users := *wrapper.User
	ids := make([]string, 0, len(users))
	for _, u := range users {
		if u.ID == nil {
			continue
		}
		ids = append(ids, strconv.Itoa(*u.ID))
	}
	return ids, len(users)
}

// flattenUsers converts the SDK users wrapper into the data source's
// Computed users slice.
func flattenUsers(wrapper *proclassic.UserGroupUsers) []UserGroupUserModel {
	if wrapper == nil || wrapper.User == nil {
		return nil
	}
	src := *wrapper.User
	out := make([]UserGroupUserModel, 0, len(src))
	for _, u := range src {
		var idVal types.String
		if u.ID != nil {
			idVal = types.StringValue(strconv.Itoa(*u.ID))
		} else {
			idVal = types.StringNull()
		}
		out = append(out, UserGroupUserModel{
			ID:           idVal,
			Username:     helpers.StringPointerValueOrNull(u.Username),
			FullName:     helpers.StringPointerValueOrNull(u.FullName),
			PhoneNumber:  helpers.StringPointerValueOrNull(u.PhoneNumber),
			EmailAddress: helpers.StringPointerValueOrNull(u.EmailAddress),
		})
	}
	return out
}
