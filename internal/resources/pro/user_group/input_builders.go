// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"cmp"
	"context"
	"slices"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildUserGroupInput converts a plan model into the SDK UserGroup payload used
// for Create and Update. The discriminator is the model's GroupType ("smart"
// vs "static"); the SDK wire field is the IsSmart bool. Criteria are sent
// only for smart groups; Users (member IDs) are sent only for static groups.
// site_id is always serialised — server emits id="-1" / name="NONE" when
// omitted and we surface that sentinel so reads stay stable.
func buildUserGroupInput(ctx context.Context, plan UserGroupResourceModel) (*proclassic.UserGroup, diag.Diagnostics) {
	var diags diag.Diagnostics

	isSmart := plan.GroupType.ValueString() == "smart"
	ug := &proclassic.UserGroup{
		Name:             helpers.OptionalStringPointer(plan.Name),
		IsSmart:          &isSmart,
		IsNotifyOnChange: plan.NotifyOnMembershipChange.ValueBoolPointer(),
		Site:             scope.BuildSiteObject(plan.SiteID),
	}

	if isSmart {
		ug.Criteria = buildCriteriaWrapper(plan.Criteria)
	} else {
		users, userDiags := buildUsersWrapper(ctx, plan.Members)
		diags.Append(userDiags...)
		if diags.HasError() {
			return nil, diags
		}
		ug.Users = users
	}

	return ug, diags
}

// buildCriteriaWrapper expands the plan criteria slice into the SDK's wrapper
// struct. Priority is filled from the element index when omitted; output is
// sorted by Priority so the wire order matches user expectations.
func buildCriteriaWrapper(criteria []UserGroupCriterionModel) *proclassic.UserGroupCriteria {
	if len(criteria) == 0 {
		empty := []proclassic.Criterion{}
		return &proclassic.UserGroupCriteria{Criterion: &empty}
	}
	out := make([]proclassic.Criterion, 0, len(criteria))
	for idx, c := range criteria {
		priority := idx
		if !c.Priority.IsNull() && !c.Priority.IsUnknown() {
			priority = int(c.Priority.ValueInt64())
		}
		andOr := proclassic.CriterionAndOrAnd
		if !c.AndOr.IsNull() && !c.AndOr.IsUnknown() && c.AndOr.ValueString() != "" {
			andOr = c.AndOr.ValueString()
		}
		opening := false
		if !c.HasOpeningParenthesis.IsNull() && !c.HasOpeningParenthesis.IsUnknown() {
			opening = c.HasOpeningParenthesis.ValueBool()
		}
		closing := false
		if !c.HasClosingParenthesis.IsNull() && !c.HasClosingParenthesis.IsUnknown() {
			closing = c.HasClosingParenthesis.ValueBool()
		}

		name := c.Name.ValueString()
		searchType := c.SearchType.ValueString()
		value := c.Value.ValueString()

		out = append(out, proclassic.Criterion{
			Name:         &name,
			Priority:     &priority,
			AndOr:        &andOr,
			SearchType:   &searchType,
			Value:        &value,
			OpeningParen: &opening,
			ClosingParen: &closing,
		})
	}
	slices.SortStableFunc(out, func(a, b proclassic.Criterion) int {
		if a.Priority == nil || b.Priority == nil {
			return 0
		}
		return cmp.Compare(*a.Priority, *b.Priority)
	})
	return &proclassic.UserGroupCriteria{Criterion: &out}
}

// buildUsersWrapper expands the plan members set (user IDs as strings) into
// the SDK's wrapper struct. Returns nil wrapper when members is null/unknown
// — distinct from an empty set which sends an empty <user> list to drop all
// assignments.
func buildUsersWrapper(ctx context.Context, members types.Set) (*proclassic.UserGroupUsers, diag.Diagnostics) {
	var diags diag.Diagnostics
	if members.IsNull() || members.IsUnknown() {
		return nil, diags
	}

	ids, idDiags := helpers.SetToStringSlice(ctx, members)
	diags.Append(idDiags...)
	if diags.HasError() {
		return nil, diags
	}

	out := make([]proclassic.UserGroupUsersUserItem, 0, len(ids))
	for _, idStr := range ids {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			diags.AddError(
				"Invalid user ID in members",
				"Expected a positive integer string for each member, got "+strconv.Quote(idStr)+".",
			)
			return nil, diags
		}
		out = append(out, proclassic.UserGroupUsersUserItem{ID: &id})
	}
	return &proclassic.UserGroupUsers{User: &out}, diags
}
