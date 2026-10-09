// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// AppRequestFormFieldResourceModel represents the Terraform resource model for a Jamf Pro
// App Request form field. Maps to the wire AppRequestFormInputField object.
type AppRequestFormFieldResourceModel struct {
	ID          types.String           `tfsdk:"id"`
	Title       types.String           `tfsdk:"title"`
	Description types.String           `tfsdk:"description"`
	Priority    types.Int64            `tfsdk:"priority"`
	Timeouts    resourceTimeouts.Value `tfsdk:"timeouts"`
}

// AppRequestFormFieldDataSourceModel represents the Terraform data source model for a Jamf
// Pro App Request form field. Either id or title must be supplied (enforced by
// ExactlyOneOf at config validation). Titles are not unique on the server, so a by-title
// lookup that matches more than one field returns an ambiguous-match error.
type AppRequestFormFieldDataSourceModel struct {
	ID          types.String             `tfsdk:"id"`
	Title       types.String             `tfsdk:"title"`
	Description types.String             `tfsdk:"description"`
	Priority    types.Int64              `tfsdk:"priority"`
	Timeouts    datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// appRequestFormFieldIdentityModel represents the identity object for App Request form
// field resources and list results.
type appRequestFormFieldIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// AppRequestFormFieldListResourceModel represents the config model for App Request form
// field list queries. The /app-request/form-input-fields endpoint accepts no query
// parameters, so the optional `filter` block is the shared client-side substring block.
type AppRequestFormFieldListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
