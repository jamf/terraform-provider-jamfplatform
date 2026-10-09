// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// VPPAssignmentResourceModel is the Terraform resource model for a Jamf Pro VPP
// assignment (the classic /vppassignments endpoint — user-based volume-purchasing
// content assignment). General-tab scalars are flattened to the top level; the
// three content collections (iOS apps / Mac apps / eBooks) are flat Set[Int64] of
// Apple catalog adam_ids; scope is the shared user-based subset.
//
// Wire mapping notes (UI label ← wire name):
//   - name                   → general.name                   (UI "Display Name")
//   - vpp_admin_account_id    → general.vpp_admin_account_id    (UI "Location")
//   - vpp_admin_account_name  → general.vpp_admin_account_name  (server-derived echo)
//   - ios_app_adam_ids        → ios_apps.ios_app[].adam_id      (UI Apps → "iOS Apps")
//   - mac_app_adam_ids        → mac_apps.mac_app[].adam_id      (UI Apps → "Mac Apps")
//   - ebook_adam_ids          → ebooks.ebook[].adam_id          (UI "eBooks")
//
// Content collections are OPT-OUT (not always-emit): a null set omits the wire
// block (server retains it); an empty set emits an empty element (clears it); a
// populated set full-replaces it. The three collections are independent.
type VPPAssignmentResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	VPPAdminAccountID   types.String `tfsdk:"vpp_admin_account_id"`
	VPPAdminAccountName types.String `tfsdk:"vpp_admin_account_name"`

	IosAppAdamIDs types.Set `tfsdk:"ios_app_adam_ids"`
	MacAppAdamIDs types.Set `tfsdk:"mac_app_adam_ids"`
	EbookAdamIDs  types.Set `tfsdk:"ebook_adam_ids"`

	Scope    *scope.UserScopeModel  `tfsdk:"scope"`
	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// VPPAssignmentDataSourceModel is the data source model. Lookup by exactly one of
// id / name. Content is surfaced as Computed Lists of {adam_id, name} objects
// (read-only → no Set-unknown correlation problem; gives discoverable names).
type VPPAssignmentDataSourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	VPPAdminAccountID   types.String `tfsdk:"vpp_admin_account_id"`
	VPPAdminAccountName types.String `tfsdk:"vpp_admin_account_name"`

	IosApps types.List `tfsdk:"ios_apps"` // Computed list of contentItem
	MacApps types.List `tfsdk:"mac_apps"` // Computed list of contentItem
	Ebooks  types.List `tfsdk:"ebooks"`   // Computed list of contentItem

	Scope    *scope.UserScopeModel    `tfsdk:"scope"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// vppAssignmentIdentityModel is the identity object for the resource + list.
type vppAssignmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// VPPAssignmentListResourceModel is the list config model. Classic has no RSQL —
// the filter shape is the shared client-side substring block.
type VPPAssignmentListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}

// contentAttrTypes is the attribute-type map for one read-only data-source
// content element ({adam_id, name}). adam_id is an Int64 (Apple catalog ids
// exceed int32).
var contentAttrTypes = map[string]attr.Type{
	"adam_id": types.Int64Type,
	"name":    types.StringType,
}

// contentObjectType is the element type for the Computed content lists on the
// data source.
var contentObjectType = types.ObjectType{AttrTypes: contentAttrTypes}

// vppAssignmentTimeoutAttributeTypes defines the timeout attribute types for the
// resource operations.
var vppAssignmentTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
