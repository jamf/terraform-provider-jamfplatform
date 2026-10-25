// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_patch_software_title"

const noSiteID = "-1"

// titleKey renders the composite uniqueness key of a title. Jamf Pro refuses a
// second title with 409 when the patch catalog key (name_id), the patch source and
// the site all match: the display name is not part of the key, and the same name_id
// on another site or another source is accepted (wire-probed 2026-10-25). The text
// is what the diagnostic shows, and two keys are equal exactly when their parts are.
func titleKey(nameID string, sourceID int64, siteID string) string {
	site := "no site"
	if siteID != "" && siteID != noSiteID {
		site = "site " + siteID
	}
	return fmt.Sprintf("%s from source %d, %s", nameID, sourceID, site)
}

// patchSoftwareTitleNamespace is the uniqueness rule for patch software titles. The
// configurations list names a title's patch source but never numbers it, so each
// entry's source is resolved from the tenant's source catalogues, and an entry whose
// source cannot be resolved is left out of the comparison.
func patchSoftwareTitleNamespace(classic *proclassic.Client, client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.patch-software-titles",
		Product:   "Jamf Pro",
		KeyLabel:  "software title",
		Enforced:  true,
		Collation: collision.CollationExact,
		Sources: []collision.Source{{
			Noun:         "patch software title",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				catalogues, err := fetchPatchSourceCatalogues(ctx, classic)
				if err != nil {
					return nil, err
				}
				configs, err := client.ListPatchSoftwareTitleConfigurationsV3(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(configs))
				for _, c := range configs {
					sourceID, err := sourceIDFromCatalogues(catalogues, c.PatchSourceName)
					if err != nil || sourceID.IsNull() {
						continue
					}
					entries = append(entries, collision.Entry{ID: c.ID, Key: titleKey(c.SoftwareTitleNameID, sourceID.ValueInt64(), c.SiteID)})
				}
				return entries, nil
			},
		}},
	}
}

// reportTitleCollision reports a title that another configuration already defines
// for the same patch source and site. A site set from another resource is unknown
// until that resource exists, so the check waits for the apply-time plan. A site
// left out of the configuration is the one the title already has, or no site on a
// create.
func (r *PatchSoftwareTitleResource) reportTitleCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !r.collisions.Enabled() || r.client == nil || r.proClient == nil || req.Plan.Raw.IsNull() {
		return
	}

	var nameID, siteID types.String
	var sourceID types.Int64
	for _, a := range []struct {
		name string
		dst  any
	}{{"name_id", &nameID}, {"source_id", &sourceID}, {"site_id", &siteID}} {
		if diags := req.Plan.GetAttribute(ctx, path.Root(a.name), a.dst); diags.HasError() {
			return
		}
	}
	if nameID.IsNull() || nameID.IsUnknown() || sourceID.IsNull() || sourceID.IsUnknown() {
		return
	}

	var priorKeys []string
	var selfID string
	priorSite := noSiteID
	if !req.State.Raw.IsNull() {
		var stateNameID, stateSiteID, id types.String
		var stateSourceID types.Int64
		for _, a := range []struct {
			name string
			dst  any
		}{{"name_id", &stateNameID}, {"source_id", &stateSourceID}, {"site_id", &stateSiteID}, {"id", &id}} {
			if diags := req.State.GetAttribute(ctx, path.Root(a.name), a.dst); diags.HasError() {
				return
			}
		}
		selfID = id.ValueString()
		if !stateSiteID.IsNull() && !stateSiteID.IsUnknown() {
			priorSite = stateSiteID.ValueString()
		}
		if !stateNameID.IsNull() && !stateSourceID.IsNull() {
			priorKeys = []string{titleKey(stateNameID.ValueString(), stateSourceID.ValueInt64(), priorSite)}
		}
	}

	var configSite types.String
	if diags := req.Config.GetAttribute(ctx, path.Root("site_id"), &configSite); diags.HasError() || configSite.IsUnknown() {
		return
	}

	site := priorSite
	if !siteID.IsNull() && !siteID.IsUnknown() {
		site = siteID.ValueString()
	}

	resp.Diagnostics.Append(r.collisions.Check(ctx, collision.Request{
		Namespace:    patchSoftwareTitleNamespace(r.client, r.proClient),
		ResourceType: resourceTypeName,
		Path:         path.Root("name_id"),
		Keys:         []string{titleKey(nameID.ValueString(), sourceID.ValueInt64(), site)},
		PriorKeys:    priorKeys,
		SelfID:       selfID,
	})...)
}
