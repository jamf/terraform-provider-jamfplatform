// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package availabletitles holds the shared model, data source schema, and
// SDK-mapping for the patch "available titles" catalog returned by
// /patchavailabletitles/sourceid/{id}. The same catalog is surfaced by both the
// jamfplatform_pro_patch_external_source and jamfplatform_pro_patch_internal_source
// data sources, so the model + attributes + mapper live here to avoid drift
// (the 2-consumer extraction trigger). It is a read-only catalog: titles are
// published by the patch source and are not user-managed.
package availabletitles

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Model is one entry in a patch source's available-titles catalog. Every field
// is returned by Jamf Pro; the catalog is server-managed. It carries no timeouts
// field so the struct backs the nested list element of either data source.
type Model struct {
	NameID         types.String `tfsdk:"name_id"`
	AppName        types.String `tfsdk:"app_name"`
	CurrentVersion types.String `tfsdk:"current_version"`
	Publisher      types.String `tfsdk:"publisher"`
	LastModified   types.String `tfsdk:"last_modified"`
}

// DataSourceAttributes returns the Computed attributes for one catalog title,
// for embedding as the NestedObject of an available_titles list attribute.
func DataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name_id": schema.StringAttribute{
			MarkdownDescription: "Catalog key identifying the software title within its source. Use this value as `name_id` on `jamfplatform_pro_patch_software_title`. Free-form (e.g. `518`, `0F5`, or `com.cisco.anyconnect.gui`).",
			Computed:            true,
		},
		"app_name": schema.StringAttribute{
			MarkdownDescription: "Display name of the software title (e.g. `Jamf Composer`).",
			Computed:            true,
		},
		"current_version": schema.StringAttribute{
			MarkdownDescription: "Newest version the source currently publishes for this title.",
			Computed:            true,
		},
		"publisher": schema.StringAttribute{
			MarkdownDescription: "Publisher of the software title.",
			Computed:            true,
		},
		"last_modified": schema.StringAttribute{
			MarkdownDescription: "Timestamp (RFC 3339) the title's definition was last updated in the source.",
			Computed:            true,
		},
	}
}

// MapTitles flattens an SDK PatchAvailableTitles response into the model slice.
// The two intermediate pointer levels (AvailableTitles, then AvailableTitle) are
// both nil-checked: a source with no published titles omits them. The returned
// slice is always non-nil so an empty catalog serialises as [] rather than null.
func MapTitles(src *proclassic.PatchAvailableTitles) []Model {
	out := make([]Model, 0)
	if src == nil || src.AvailableTitles == nil || src.AvailableTitles.AvailableTitle == nil {
		return out
	}
	items := *src.AvailableTitles.AvailableTitle
	for i := range items {
		out = append(out, Model{
			NameID:         helpers.StringPointerValueOrNull(items[i].NameID),
			AppName:        helpers.StringPointerValueOrNull(items[i].AppName),
			CurrentVersion: helpers.StringPointerValueOrNull(items[i].CurrentVersion),
			Publisher:      helpers.StringPointerValueOrNull(items[i].Publisher),
			LastModified:   helpers.StringPointerValueOrNull(items[i].LastModified),
		})
	}
	return out
}
