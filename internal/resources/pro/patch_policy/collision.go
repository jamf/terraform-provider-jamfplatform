// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_patch_policy"

// policyKey renders the composite uniqueness key of a patch policy. Jamf Pro
// refuses a second policy with 409 when the name matches within one software title
// configuration, and accepts the same name on another one (wire-probed 2026-10-25).
// It trims the name on save, so the name is trimmed here before the title is
// appended: the namespace's folding then drops case and accents only, and a space
// can never sit between the name and the title.
func policyKey(titleConfigID, name string) string {
	return fmt.Sprintf("%s on software title %s", strings.TrimSpace(name), titleConfigID)
}

// patchPolicyNamespace is the uniqueness rule for patch policy names. Jamf Pro
// compares the name without regard to case, leading and trailing spaces or accents.
func patchPolicyNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.patch-policies",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "patch policy",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				policies, err := client.ListPatchPoliciesV2(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(policies))
				for _, p := range policies {
					entries = append(entries, collision.Entry{ID: p.ID, Key: policyKey(p.SoftwareTitleConfigurationID, p.PolicyName)})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a policy name that another policy of the same
// software title configuration already holds. A title configuration set from
// another resource is unknown until that resource exists, so the check waits for the
// apply-time plan.
func (r *PatchPolicyResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !r.collisions.Enabled() || r.proClient == nil || req.Plan.Raw.IsNull() {
		return
	}

	var titleID, name types.String
	if diags := req.Plan.GetAttribute(ctx, path.Root("software_title_configuration_id"), &titleID); diags.HasError() {
		return
	}
	if diags := req.Plan.GetAttribute(ctx, path.Root("name"), &name); diags.HasError() {
		return
	}
	if titleID.IsNull() || titleID.IsUnknown() || name.IsNull() || name.IsUnknown() {
		return
	}

	var priorKeys []string
	var selfID string
	if !req.State.Raw.IsNull() {
		var stateTitleID, stateName, id types.String
		for _, a := range []struct {
			name string
			dst  *types.String
		}{{"software_title_configuration_id", &stateTitleID}, {"name", &stateName}, {"id", &id}} {
			if diags := req.State.GetAttribute(ctx, path.Root(a.name), a.dst); diags.HasError() {
				return
			}
		}
		selfID = id.ValueString()
		if !stateTitleID.IsNull() && !stateName.IsNull() {
			priorKeys = []string{policyKey(stateTitleID.ValueString(), stateName.ValueString())}
		}
	}

	resp.Diagnostics.Append(r.collisions.Check(ctx, collision.Request{
		Namespace:    patchPolicyNamespace(r.proClient),
		ResourceType: resourceTypeName,
		Path:         path.Root("name"),
		Keys:         []string{policyKey(titleID.ValueString(), name.ValueString())},
		PriorKeys:    priorKeys,
		SelfID:       selfID,
	})...)
}
