// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// eaElementType is the object type of one extension_attributes list element.
var eaElementType = types.ObjectType{AttrTypes: patchSoftwareTitleEAAttrTypes}

// refreshExtensionAttributes reads the title's extension attributes from the v3
// configuration endpoint (keyed by the same id as the classic title) and writes
// them onto the model. The read is best-effort: a failure here must not break
// classic CRUD, so it surfaces a Warning and leaves an empty (non-null) list —
// matching the available_titles convention on the source data sources.
func (r *PatchSoftwareTitleResource) refreshExtensionAttributes(ctx context.Context, id string, model *PatchSoftwareTitleResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	model.ExtensionAttributes = types.ListValueMust(eaElementType, []attr.Value{})

	if r.proClient == nil || id == "" {
		return diags
	}

	eas, err := r.proClient.ListPatchSoftwareTitleExtensionAttributesV3(ctx, id)
	if err != nil {
		diags.AddWarning(
			"Unable to read patch software title extension attributes",
			fmt.Sprintf("The title was read, but its extension attributes could not be retrieved: %v", err),
		)
		return diags
	}

	out := make([]PatchSoftwareTitleEAModel, 0, len(eas))
	for i := range eas {
		out = append(out, PatchSoftwareTitleEAModel{
			EaID:        types.StringValue(eas[i].EaID),
			DisplayName: types.StringValue(eas[i].DisplayName),
			Accepted:    types.BoolValue(eas[i].Accepted),
		})
	}

	list, listDiags := types.ListValueFrom(ctx, eaElementType, out)
	diags.Append(listDiags...)
	if listDiags.HasError() {
		return diags
	}
	model.ExtensionAttributes = list
	return diags
}

// acceptPendingExtensionAttributes accepts every not-yet-accepted extension
// attribute on the title via a v3 merge-patch. Accepting is one-way (the server
// rejects accepted=false), so this only ever sets accepted=true and skips the
// call entirely when nothing is pending. The caller invokes it only when the
// user set accept_extension_attributes=true; a failure is therefore fatal (the
// user explicitly asked to accept).
//
// categoryID is the title's user-facing category id (always "-1" for "no
// category" or a positive id) and is re-sent in the same merge-patch. This is
// load-bearing: the classic /patchsoftwaretitles endpoint clears a category with
// wire id 0, which leaves the shared configuration's categoryId as the literal
// "0" — a value the configurations endpoint then rejects on ANY write ("id field
// must be string of positive numeric value or -1"), wire-observed on v2 and
// carried forward by v3, which takes the same merge-patch body. Re-asserting the
// user-facing categoryId (which is always valid there) on the accept repairs that
// divergence so the merge-patch is accepted.
func (r *PatchSoftwareTitleResource) acceptPendingExtensionAttributes(ctx context.Context, id, categoryID string) error {
	if r.proClient == nil || id == "" {
		return nil
	}

	eas, err := r.proClient.ListPatchSoftwareTitleExtensionAttributesV3(ctx, id)
	if err != nil {
		return fmt.Errorf("reading extension attributes before accept: %w", err)
	}

	accepted := true
	pending := make([]pro.PatchSoftwareTitleConfigurationExtensionAttributes, 0, len(eas))
	for i := range eas {
		if eas[i].Accepted {
			continue
		}
		eaID := eas[i].EaID
		pending = append(pending, pro.PatchSoftwareTitleConfigurationExtensionAttributes{
			EaID:     &eaID,
			Accepted: &accepted,
		})
	}
	if len(pending) == 0 {
		return nil
	}

	cat := categoryID
	if cat == "" {
		cat = "-1"
	}
	if _, err := r.proClient.UpdatePatchSoftwareTitleConfigurationV3(ctx, id, &pro.PatchSoftwareTitleConfigurationPatch{
		CategoryID:          &cat,
		ExtensionAttributes: &pending,
	}); err != nil {
		return fmt.Errorf("accepting extension attributes: %w", err)
	}
	return nil
}
