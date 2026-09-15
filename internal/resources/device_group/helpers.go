// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// importHydration reports whether this Read is the one Terraform issues after an
// import, with state holding the group identifier and nothing else. A null state
// value cannot answer that: the plugin framework writes the passthrough id into
// state before calling Read, so on the import path Read receives a populated
// object and `Raw.IsNull()` returns false. A null `name` does answer it, because
// the attribute is Required and every other path into Read sets it. Reading the
// wrong signal left `description` and `members` null after an import, unmanaged
// with no later plan to surface the gap, because the reconcile they pass through
// treats an unset prior value as a configuration that does not own the attribute
// (issue #372).
//
// The name Read passes here comes off the immutable request state, never off the
// model Read is assembling. `assignDeviceGroupModel` nulls `name` when the wire
// value is empty, so a signal taken from the model would answer differently
// depending on where in Read it was sampled, and the request state is the only
// source no later assignment can reach.
func importHydration(stateAbsent bool, name types.String) bool {
	return stateAbsent || name.IsNull()
}

// manageMembersOnRead reports whether a Read writes the group's membership to
// state. A configuration that declared `members` owns it. An import owns it only
// when the group has members: adopting an empty group would store `members = []`
// where creating the same group without the attribute stores null, so the import
// round-trip would report a difference over nothing. The criteria flatten holds
// to the same rule for a collection the group does not have.
func manageMembersOnRead(hydrating bool, prior types.Set, members []string) bool {
	if helpers.IsConfiguredValue(prior) {
		return true
	}
	return hydrating && len(members) > 0
}

// refreshDeviceGroupState retrieves the device group from the API and updates the Terraform state.
func (r *DeviceGroupResource) refreshDeviceGroupState(ctx context.Context, id string, model *DeviceGroupResourceModel, manageMembers bool, manageDescription bool, diags *diag.Diagnostics) bool {
	var grp *devicegroups.DeviceGroupReadRepresentationV1
	err := jamfplatform.PollUntil(ctx, deviceGroupCreateRetryDelay, func(pollCtx context.Context) (bool, error) {
		var err error
		grp, err = r.client.GetDeviceGroup(pollCtx, id)
		if err == nil {
			return true, nil
		}
		if !helpers.IsNotFoundError(err) {
			return false, err
		}

		tflog.Debug(pollCtx, "device group not yet available, retrying", map[string]any{
			"id": id,
		})
		return false, nil
	})
	if err != nil {
		diags.AddError("Error reading device group", helpers.APIErrorDetail(err))
		return false
	}

	var members []string
	if strings.EqualFold(grp.GroupType, devicegroups.GroupTypeV1Static) && manageMembers {
		var err error
		members, err = r.client.ListDeviceGroupMembers(ctx, grp.ID)
		if err != nil {
			diags.AddError("Error reading device group members", helpers.APIErrorDetail(err))
			return false
		}
	}

	diags.Append(assignDeviceGroupModel(ctx, model, grp, members, manageMembers, manageDescription)...)
	return !diags.HasError()
}

// validateDeviceGroupPlan checks that the plan is valid based on group type.
func validateDeviceGroupPlan(plan *DeviceGroupResourceModel) error {
	groupType := strings.ToLower(plan.GroupType.ValueString())

	switch groupType {
	case "smart":
		if helpers.IsConfiguredValue(plan.Members) {
			return fmt.Errorf("members cannot be set for smart groups")
		}
	case "static":
		if len(plan.Criteria) > 0 {
			return fmt.Errorf("criteria cannot be set for static groups")
		}
		if plan.Members.IsUnknown() {
			return fmt.Errorf("members cannot be unknown when provided")
		}
	default:
		if groupType == "" {
			return fmt.Errorf("group_type must be provided")
		}
		return fmt.Errorf("unsupported group_type %q", groupType)
	}

	return nil
}

// diffStringSlices returns added/removed values between current and desired sets.
func diffStringSlices(current, desired []string) (added, removed []string) {
	currentSet := make(map[string]struct{}, len(current))
	for _, v := range current {
		currentSet[v] = struct{}{}
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, v := range desired {
		desiredSet[v] = struct{}{}
		if _, ok := currentSet[v]; !ok {
			added = append(added, v)
		}
	}
	for _, v := range current {
		if _, ok := desiredSet[v]; !ok {
			removed = append(removed, v)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return
}
