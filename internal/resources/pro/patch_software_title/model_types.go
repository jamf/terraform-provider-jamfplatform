// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// PatchSoftwareTitleResourceModel represents the Terraform resource model for a
// Jamf Pro patch software title.
//
// VersionPackages is a managed-subset map keyed by software_version with
// package_id string values. The user only declares the version→package
// assignments they care about; the server holds the full catalog of versions
// (20+) for the title. Read reconciles only the keys the user declared, and
// Update folds them over the server's live set, so an assignment made outside
// Terraform survives an apply (see state_builders.go, input_builders.go).
type PatchSoftwareTitleResourceModel struct {
	ID                        types.String           `tfsdk:"id"`
	Name                      types.String           `tfsdk:"name"`
	NameID                    types.String           `tfsdk:"name_id"`
	SourceID                  types.Int64            `tfsdk:"source_id"`
	CategoryID                types.String           `tfsdk:"category_id"`
	SiteID                    types.String           `tfsdk:"site_id"`
	WebNotification           types.Bool             `tfsdk:"web_notification"`
	EmailNotification         types.Bool             `tfsdk:"email_notification"`
	VersionPackages           types.Map              `tfsdk:"version_packages"`
	AvailableVersions         types.List             `tfsdk:"available_versions"`
	AcceptExtensionAttributes types.Bool             `tfsdk:"accept_extension_attributes"`
	ExtensionAttributes       types.List             `tfsdk:"extension_attributes"`
	Timeouts                  resourceTimeouts.Value `tfsdk:"timeouts"`
}

// PatchSoftwareTitleEAModel is one Jamf-defined extension attribute attached to
// a patch software title. For some titles, Jamf creates an EA that collects the
// installed version on managed computers; it must be accepted before inventory
// is gathered. The list is read-only state (the catalog of EAs and their accept
// status); set accept_extension_attributes to accept any pending ones.
//
// ExtensionAttributes is modelled as types.List (not []PatchSoftwareTitleEAModel)
// because it is a Computed attribute with no plan modifier: it plans as Unknown,
// and a plain Go slice cannot represent an unknown value (Plan.Get would fail
// with "the target type cannot handle unknown values"). This struct is the
// element type used when converting to/from that list.
type PatchSoftwareTitleEAModel struct {
	EaID        types.String `tfsdk:"ea_id"`
	DisplayName types.String `tfsdk:"display_name"`
	Accepted    types.Bool   `tfsdk:"accepted"`
}

// patchSoftwareTitleEAAttrTypes is the object attribute schema for one EA list
// element, used to build the types.List value in refreshExtensionAttributes.
var patchSoftwareTitleEAAttrTypes = map[string]attr.Type{
	"ea_id":        types.StringType,
	"display_name": types.StringType,
	"accepted":     types.BoolType,
}

// PatchSoftwareTitleDataSourceModel represents the Terraform data source model.
// Lookup is by ID or by exact display name — exactly one of the two must be
// supplied. See data_source.go (lookupByName).
//
// SourceID is not read but resolved, from the patch source name the payload
// reports, and the resolution is best-effort: it is null with a warning whenever
// the name cannot be matched to exactly one patch source, including when the
// source catalogues cannot be read at all (see sourceIDFromCatalogues). The
// managed resource is the one construct where an unresolved name is fatal, on
// import, because there SourceID defines the title and forces replacement.
type PatchSoftwareTitleDataSourceModel struct {
	ID                types.String             `tfsdk:"id"`
	Name              types.String             `tfsdk:"name"`
	NameID            types.String             `tfsdk:"name_id"`
	SourceID          types.Int64              `tfsdk:"source_id"`
	CategoryID        types.String             `tfsdk:"category_id"`
	SiteID            types.String             `tfsdk:"site_id"`
	WebNotification   types.Bool               `tfsdk:"web_notification"`
	EmailNotification types.Bool               `tfsdk:"email_notification"`
	VersionPackages   types.Map                `tfsdk:"version_packages"`
	AvailableVersions types.List               `tfsdk:"available_versions"`
	Timeouts          datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// patchSoftwareTitleIdentityModel represents the identity object for the
// resource and list results.
type patchSoftwareTitleIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// PatchSoftwareTitleListResourceModel represents the config model for list
// queries. The v3 configurations list takes no query parameters, so the filter
// shape is the shared client-side substring block, matching each title's
// display name.
type PatchSoftwareTitleListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
