// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// assignStaticComputerGroupResourceState writes the scalars Jamf Pro reports
// onto a resource model. Membership is not one of them: this read carries none,
// which is the whole reason the classic read exists.
//
// description is Optional+Computed, so it is reconciled against what is already
// in state rather than copied from the response. Every write is a full replace
// of the scalars, so a body that omits the note clears it to the empty string
// and the read reports that empty string back; copying it would null an
// attribute whose plan held an authored "", which Terraform Core rejects as an
// inconsistent result after apply. Reconciling keeps an authored empty string
// and still lands null on import, where there is no authored value to keep.
func assignStaticComputerGroupResourceState(state *StaticComputerGroupResourceModel, got *pro.StaticComputerGroup) {
	if got == nil {
		return
	}
	if got.ID != "" {
		state.ID = types.StringValue(got.ID)
	}
	state.Name = types.StringValue(got.Name)
	state.Description = helpers.ReconcileOptionalStringPointer(got.Description, state.Description)
	state.SiteID = progroups.SiteIDForState(got.SiteID)
}

// assignStaticComputerGroupDataSourceState is the data source's counterpart.
// Every attribute there is Computed, so there is nothing to reconcile.
func assignStaticComputerGroupDataSourceState(state *StaticComputerGroupDataSourceModel, got *pro.StaticComputerGroup) {
	if got == nil {
		return
	}
	if got.ID != "" {
		state.ID = types.StringValue(got.ID)
	}
	state.Name = types.StringValue(got.Name)
	state.Description = helpers.StringPointerValueOrNull(got.Description)
	state.SiteID = progroups.SiteIDForState(got.SiteID)
}

// flattenStaticComputerGroupMembership returns the Jamf Pro computer
// identifiers a classic computer-group read reports, as strings.
//
// A group with no members is reported as an absent or empty <computers> block
// and comes back as nil here, which the callers distinguish from "read
// nothing": nil plus a successful read means the group is empty.
func flattenStaticComputerGroupMembership(group *proclassic.ComputerGroup) []string {
	if group == nil || group.Computers == nil || group.Computers.Computer == nil {
		return nil
	}
	items := *group.Computers.Computer
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.ID == nil {
			continue
		}
		ids = append(ids, strconv.Itoa(*item.ID))
	}
	return ids
}

// membershipSetValue renders computer identifiers as a set for state.
//
// nil becomes an empty set rather than a null one. types.SetValueFrom returns
// null for a nil slice, which collides with a configuration that asked for `[]`
// and surfaces as the framework's inconsistent-result error on apply; the
// caller decides nullness through its ownership gate, never through this.
func membershipSetValue(ctx context.Context, ids []string) (types.Set, diag.Diagnostics) {
	if ids == nil {
		ids = []string{}
	}
	return types.SetValueFrom(ctx, types.StringType, ids)
}
