// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// UserExtensionAttributeResourceModel is the Terraform resource model for a Jamf
// Pro user extension attribute (Classic API). User EAs are the simplest of the
// three EA types: name, description, data type and an input type. The Classic
// wire nests input_type as `<input_type><type>…</type><popup_choices>…</popup_choices></input_type>`;
// the schema flattens that to `input_type` + `popup_menu_choices` (matching the
// admin UI and the computer/mobile EA resources), translating at the boundary.
//
// popup_menu_choices is valid only when input_type = "Pop-up Menu"
// (inputTypeConfigValidator enforces this at plan time).
type UserExtensionAttributeResourceModel struct {
	ID               types.String           `tfsdk:"id"`
	Name             types.String           `tfsdk:"name"`
	Description      types.String           `tfsdk:"description"`
	DataType         types.String           `tfsdk:"data_type"`
	InputType        types.String           `tfsdk:"input_type"`
	PopupMenuChoices types.List             `tfsdk:"popup_menu_choices"`
	Timeouts         resourceTimeouts.Value `tfsdk:"timeouts"`
}

// UserExtensionAttributeDataSourceModel is the Terraform data source model.
// Either id or name must be supplied (ExactlyOneOf at config validation).
type UserExtensionAttributeDataSourceModel struct {
	ID               types.String             `tfsdk:"id"`
	Name             types.String             `tfsdk:"name"`
	Description      types.String             `tfsdk:"description"`
	DataType         types.String             `tfsdk:"data_type"`
	InputType        types.String             `tfsdk:"input_type"`
	PopupMenuChoices types.List               `tfsdk:"popup_menu_choices"`
	Timeouts         datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// userExtensionAttributeIdentityModel is the identity object for the resource and
// list results.
type userExtensionAttributeIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// UserExtensionAttributeListResourceModel is the config model for list queries.
// Classic has no RSQL — the filter shape is the shared client-side substring block.
type UserExtensionAttributeListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
