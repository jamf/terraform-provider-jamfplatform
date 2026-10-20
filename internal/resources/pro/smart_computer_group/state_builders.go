// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// assignSmartComputerGroupResourceModel folds a group response into the resource
// model.
//
// Neither identifier is touched. The response carries no id at all, so id and
// platform_id are the caller's to set: the create takes them from the create
// response and the bridge, and a refresh carries the values already in state.
func assignSmartComputerGroupResourceModel(state *SmartComputerGroupResourceModel, got *pro.SmartComputerGroupV3) {
	if got == nil {
		return
	}
	state.Name = types.StringValue(got.Name)
	state.Description = types.StringValue(helpers.DerefString(got.Description))
	state.SiteID = progroups.SiteIDForState(got.SiteID)
	state.Criteria = criteria.FlattenComputerSmartGroupCriteria(got.Criteria)
}

// writtenSmartComputerGroupState is the state to record for a group that was
// created and then could not be read back.
//
// It has to hold no unknown value anywhere, because an unknown cannot be
// written to state, and the plan carries one for every Computed attribute
// nothing has filled in yet. Two can reach here, and both resolve to what the
// write itself sent: an unset description is the empty string Jamf Pro stores
// for a group without one, and an unset criterion priority is the criterion's
// position in the list, which is what the shared builder emitted. The
// identifiers are already known — a create that got this far has both — and
// every other attribute is either configured or carries a schema default.
//
// A null criteria list stays null. Writing an empty list instead would disagree
// with a plan that declared no criteria and fail the apply a second time.
func writtenSmartComputerGroupState(plan SmartComputerGroupResourceModel) SmartComputerGroupResourceModel {
	plan.Description = types.StringValue(descriptionForWrite(plan.Description))
	if plan.Criteria == nil {
		return plan
	}
	recorded := make([]criteria.CriterionModel, len(plan.Criteria))
	copy(recorded, plan.Criteria)
	for i := range recorded {
		if recorded[i].Priority.IsNull() || recorded[i].Priority.IsUnknown() {
			recorded[i].Priority = types.Int64Value(int64(i))
		}
	}
	plan.Criteria = recorded
	return plan
}

// assignSmartComputerGroupDataSourceModel folds a group response into the
// singular data source model. The identifiers are the caller's, as above.
func assignSmartComputerGroupDataSourceModel(state *SmartComputerGroupDataSourceModel, got *pro.SmartComputerGroupV3) {
	if got == nil {
		return
	}
	state.Name = types.StringValue(got.Name)
	state.Description = types.StringValue(helpers.DerefString(got.Description))
	state.SiteID = progroups.SiteIDForState(got.SiteID)
	state.Criteria = criteria.FlattenComputerSmartGroupCriteria(got.Criteria)
}

// searchResultModel maps one search entry onto a plural data source result,
// taking the platform identifier from the map the caller resolved for the whole
// page. A group missing from that map reports a null platform_id, which costs
// nothing else and resolves on a later read.
func searchResultModel(item pro.SmartComputerGroupSearch, platformIDs map[string]string) SmartComputerGroupsDataSourceResultModel {
	siteID := item.SiteID
	return SmartComputerGroupsDataSourceResultModel{
		ID:          types.StringValue(item.ID),
		PlatformID:  progroups.PlatformIDValue(platformIDs, item.ID),
		Name:        types.StringValue(item.Name),
		Description: types.StringValue(item.Description),
		SiteID:      progroups.SiteIDForState(&siteID),
		MemberCount: types.Int64Value(int64(item.MembershipCount)),
	}
}

// smartComputerGroupReflectsPlan reports whether a group read back after an
// update carries the planned name, description, site and criteria, the settled
// test progroups.ReadBackAfterWrite needs to outlast Jamf Pro 11.32.0's
// one-write-behind read. It flattens into a copy of plan, so it compares
// exactly what Terraform will.
func smartComputerGroupReflectsPlan(plan SmartComputerGroupResourceModel, got *pro.SmartComputerGroupV3) bool {
	read := plan
	assignSmartComputerGroupResourceModel(&read, got)
	return progroups.PlannedValueReflected(plan.Name, read.Name) &&
		progroups.PlannedValueReflected(plan.Description, read.Description) &&
		progroups.PlannedValueReflected(plan.SiteID, read.SiteID) &&
		criteriaReflectPlan(plan.Criteria, read.Criteria)
}

// fallbackWriteReflectsPlan is smartComputerGroupReflectsPlan without the
// description, which the classic fallback write cannot carry: it reports
// whether that write is readable, so the description patch that follows it
// does not write back the group as it was before.
func fallbackWriteReflectsPlan(plan SmartComputerGroupResourceModel, got *pro.SmartComputerGroupV3) bool {
	read := plan
	assignSmartComputerGroupResourceModel(&read, got)
	return progroups.PlannedValueReflected(plan.Name, read.Name) &&
		progroups.PlannedValueReflected(plan.SiteID, read.SiteID) &&
		criteriaReflectPlan(plan.Criteria, read.Criteria)
}

// criteriaReflectPlan is PlannedValueReflected over every field of every
// criterion, so a planned Computed field that is still unknown, such as a
// priority Jamf Pro assigns, does not hold the read-back open.
func criteriaReflectPlan(planned, read []criteria.CriterionModel) bool {
	if len(planned) != len(read) {
		return false
	}
	for i := range planned {
		p, r := planned[i], read[i]
		if !progroups.PlannedValueReflected(p.Priority, r.Priority) ||
			!progroups.PlannedValueReflected(p.Name, r.Name) ||
			!progroups.PlannedValueReflected(p.SearchType, r.SearchType) ||
			!progroups.PlannedValueReflected(p.Value, r.Value) ||
			!progroups.PlannedValueReflected(p.AndOr, r.AndOr) ||
			!progroups.PlannedValueReflected(p.HasOpeningParenthesis, r.HasOpeningParenthesis) ||
			!progroups.PlannedValueReflected(p.HasClosingParenthesis, r.HasClosingParenthesis) {
			return false
		}
	}
	return true
}
