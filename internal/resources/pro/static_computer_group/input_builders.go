// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// buildStaticComputerGroupInput builds the create and update payload.
//
// Every scalar is emitted unconditionally. These endpoints full-replace their
// scalars, so a body that omits description clears it to the empty string and
// one that omits siteId clears the site to the no-site sentinel; nothing here
// relies on omission to retain a value.
//
// assignments is emitted unconditionally for a harder reason: it is mandatory,
// and a body without it answers 500 with an empty errors array and applies
// nothing. The caller decides what belongs in it, because the choice depends on
// whether the configuration manages membership and on whether this is a create
// or an update. See the package doc.
func buildStaticComputerGroupInput(plan StaticComputerGroupResourceModel, assignments []string) *pro.StaticComputerGroupAssignment {
	if assignments == nil {
		assignments = []string{}
	}
	description := plan.Description.ValueString()
	return &pro.StaticComputerGroupAssignment{
		Name:        plan.Name.ValueString(),
		Description: &description,
		SiteID:      progroups.SiteIDForWrite(plan.SiteID),
		Assignments: &assignments,
	}
}

// configuredAssignments returns the computer identifiers a configuration
// declares, as the write wants them.
//
// A null or unknown set yields an empty slice rather than nil, because both
// callers that reach it want the write to be explicit: a create that manages
// nothing makes an empty group, and a configuration that says `[]` empties one.
// The "leave membership alone" case never gets here.
func configuredAssignments(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if set.IsNull() || set.IsUnknown() {
		return []string{}, diags
	}
	ids, idDiags := helpers.SetToStringSlice(ctx, set)
	diags.Append(idDiags...)
	if diags.HasError() {
		return nil, diags
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, diags
}
