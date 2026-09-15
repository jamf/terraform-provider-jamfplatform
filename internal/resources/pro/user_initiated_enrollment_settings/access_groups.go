// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
)

// allDirectoryServiceUsersServerID and ...GroupID mark the built-in
// "All Directory Service Users" pseudo-group, which has no directory backing —
// both its server id and group id are "-1" and no LDAP lookup is performed.
const (
	allDirectoryServiceUsersServerID = "-1"
	allDirectoryServiceUsersGroupID  = "-1"
)

// accessGroupAction enumerates the reconcile operations for one planned/current
// Access Group.
type accessGroupAction int

const (
	accessGroupCreate accessGroupAction = iota
	accessGroupUpdate
	accessGroupDelete
)

// accessGroupOp is a single reconcile step: an action plus the payload (for
// create/update) or the server id (for delete/update).
type accessGroupOp struct {
	Action accessGroupAction
	ID     string
	Group  accessGroupModel
}

// planAccessGroupReconcile computes the create/update/delete operations needed
// to drive the tenant's current Access Groups (current) to the planned set
// (planned), diffing by server id with a natural-key fallback for new groups.
//
// Rules:
//   - The built-in "All Directory Service Users" group (id="1") is never
//     deleted. If the user declares it, it is updated; if not, it is left
//     untouched.
//   - A planned group carrying a server id matches the current group with that
//     id → update when fields differ.
//   - A planned group without an id is matched to a current group by the
//     natural key (directory_service_group_id + ldap_server_id); matched →
//     update, unmatched → create.
//   - A current group not matched by any planned group is deleted (except id=1).
func planAccessGroupReconcile(planned []accessGroupModel, current []pro.EnrollmentAccessGroupPreview) []accessGroupOp {
	var ops []accessGroupOp

	currentByID := make(map[string]pro.EnrollmentAccessGroupPreview, len(current))
	currentByKey := make(map[string]pro.EnrollmentAccessGroupPreview, len(current))
	for _, c := range current {
		id := pointerString(c.ID)
		if id != "" {
			currentByID[id] = c
		}
		currentByKey[naturalKey(c.Name, c.LdapServerID)] = c
	}

	matchedIDs := make(map[string]struct{})

	for _, p := range planned {
		planID := stringOrEmpty(p.ID)
		if planID == "" {
			// Match by natural key (name + server) for newly-declared groups —
			// directory_service_group_id is Computed (resolved at apply) so it
			// is not known at plan time.
			if c, ok := currentByKey[naturalKey(p.Name.ValueString(), p.LdapServerID.ValueString())]; ok {
				planID = pointerString(c.ID)
			}
		}

		if planID != "" {
			if cur, ok := currentByID[planID]; ok {
				matchedIDs[planID] = struct{}{}
				if accessGroupDiffers(p, cur) {
					ops = append(ops, accessGroupOp{Action: accessGroupUpdate, ID: planID, Group: p})
				}
				continue
			}
		}
		// No server match → create.
		ops = append(ops, accessGroupOp{Action: accessGroupCreate, Group: p})
	}

	// Delete current groups the plan no longer references, never the built-in.
	for _, c := range current {
		id := pointerString(c.ID)
		if id == "" || id == allDirectoryServiceUsersID {
			continue
		}
		if _, ok := matchedIDs[id]; ok {
			continue
		}
		ops = append(ops, accessGroupOp{Action: accessGroupDelete, ID: id})
	}

	return ops
}

// naturalKey builds the name + ldap_server_id natural key used to match a
// planned group to a current one. directory_service_group_id is NOT part of the
// key: it is Computed (resolved at apply), so it is not known at plan time.
func naturalKey(name, ldapServerID string) string {
	return name + "\x00" + ldapServerID
}

// accessGroupDiffers reports whether a planned group differs from the current
// server state on any writable field. directory_service_group_id is omitted
// (it is Computed/resolved, not user-authored, and equals the match key's
// server side). Unset (null/unknown) planned bool/site fields are treated as
// "no change" so Optional+Computed omissions do not churn.
func accessGroupDiffers(p accessGroupModel, c pro.EnrollmentAccessGroupPreview) bool {
	if p.Name.ValueString() != c.Name {
		return true
	}
	if p.LdapServerID.ValueString() != c.LdapServerID {
		return true
	}
	if siteDiffers(p, c) {
		return true
	}
	if boolDiffers(p.EnterpriseEnrollmentEnabled, c.EnterpriseEnrollmentEnabled) {
		return true
	}
	if boolDiffers(p.PersonalEnrollmentEnabled, c.PersonalEnrollmentEnabled) {
		return true
	}
	if boolDiffers(p.AccountDrivenUserEnrollmentEnabled, c.AccountDrivenUserEnrollmentEnabled) {
		return true
	}
	if boolDiffers(p.RequireEula, c.RequireEula) {
		return true
	}
	return false
}

// siteDiffers compares the planned site_id to the current value, treating an
// unset planned value as "no change".
func siteDiffers(p accessGroupModel, c pro.EnrollmentAccessGroupPreview) bool {
	if p.SiteID.IsNull() || p.SiteID.IsUnknown() {
		return false
	}
	return p.SiteID.ValueString() != pointerString(c.SiteID)
}

// boolDiffers compares a planned Bool to a current *bool, treating an unset
// planned value as "no change".
func boolDiffers(plan types.Bool, cur *bool) bool {
	if plan.IsNull() || plan.IsUnknown() {
		return false
	}
	if cur == nil {
		return true
	}
	return plan.ValueBool() != *cur
}

// pointerString safely dereferences a *string.
func pointerString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// stringOrEmpty returns the value of a non-null/unknown types.String, else "".
func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// resolveAccessGroupID resolves the directory's canonical group id for a
// planned access group from its name + ldap_server_id, mirroring the admin UI's
// "Resolve" action. The built-in "All Directory Service Users" pseudo-group
// (ldap_server_id "-1") resolves to "-1" without a directory lookup.
func (r *UserInitiatedEnrollmentSettingsResource) resolveAccessGroupID(ctx context.Context, g accessGroupModel) (string, error) {
	server := g.LdapServerID.ValueString()
	if server == allDirectoryServiceUsersServerID {
		return allDirectoryServiceUsersGroupID, nil
	}
	sid, err := strconv.Atoi(server)
	if err != nil {
		return "", fmt.Errorf("ldap_server_id %q must be an integer: %w", server, err)
	}
	grp, err := ldapgroups.Resolve(ctx, r.client, g.Name.ValueString(), sid)
	if err != nil {
		return "", err
	}
	return grp.ID, nil
}

// reconcileAccessGroups drives the tenant's Access Groups to the planned set.
// It runs OUTSIDE the /v4 enrollment write lock (the /v3 endpoints are a
// separate backing store). When the planned set is null (the user did not
// author the collection), the tenant's groups are left untouched.
func (r *UserInitiatedEnrollmentSettingsResource) reconcileAccessGroups(
	ctx context.Context,
	plan *UserInitiatedEnrollmentSettingsResourceModel,
	state *UserInitiatedEnrollmentSettingsResourceModel,
	diags *diag.Diagnostics,
) bool {
	// Unmanaged: user did not author the access_group collection.
	if plan.AccessGroups.IsNull() || plan.AccessGroups.IsUnknown() {
		return true
	}

	var planned []accessGroupModel
	diags.Append(plan.AccessGroups.ElementsAs(ctx, &planned, false)...)
	if diags.HasError() {
		return false
	}

	current, err := r.client.ListEnrollmentAccessGroupsV3(ctx, nil, true)
	if err != nil {
		diags.AddError("Error reading Jamf Pro enrollment Access Groups", helpers.APIErrorDetail(err))
		return false
	}

	for _, op := range planAccessGroupReconcile(planned, current) {
		switch op.Action {
		case accessGroupCreate:
			gid, err := r.resolveAccessGroupID(ctx, op.Group)
			if err != nil {
				diags.AddError("Error resolving directory-service group", helpers.APIErrorDetail(err))
				return false
			}
			if _, err := r.client.CreateEnrollmentAccessGroupV3(ctx, buildAccessGroupInput(op.Group, gid)); err != nil {
				diags.AddError("Error creating Jamf Pro enrollment Access Group", helpers.APIErrorDetail(err))
				return false
			}
		case accessGroupUpdate:
			gid, err := r.resolveAccessGroupID(ctx, op.Group)
			if err != nil {
				diags.AddError("Error resolving directory-service group", helpers.APIErrorDetail(err))
				return false
			}
			if _, err := r.client.UpdateEnrollmentAccessGroupV3(ctx, op.ID, buildAccessGroupInput(op.Group, gid)); err != nil {
				diags.AddError("Error updating Jamf Pro enrollment Access Group", helpers.APIErrorDetail(err))
				return false
			}
		case accessGroupDelete:
			if err := r.client.DeleteEnrollmentAccessGroupV3(ctx, op.ID); err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				diags.AddError("Error deleting Jamf Pro enrollment Access Group", helpers.APIErrorDetail(err))
				return false
			}
		}
	}
	return true
}
