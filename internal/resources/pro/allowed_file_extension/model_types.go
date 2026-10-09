// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// AllowedFileExtensionResourceModel represents the Terraform resource model for a Jamf
// Pro allowed file extension. Extension maps to the wire `extension` field.
type AllowedFileExtensionResourceModel struct {
	ID        types.String           `tfsdk:"id"`
	Extension types.String           `tfsdk:"extension"`
	Timeouts  resourceTimeouts.Value `tfsdk:"timeouts"`
}

// AllowedFileExtensionDataSourceModel represents the Terraform data source model for a
// Jamf Pro allowed file extension. Either id or extension must be supplied (enforced by
// ExactlyOneOf at config validation).
type AllowedFileExtensionDataSourceModel struct {
	ID        types.String             `tfsdk:"id"`
	Extension types.String             `tfsdk:"extension"`
	Timeouts  datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// allowedFileExtensionIdentityModel represents the identity object for allowed file
// extension resources and list results.
type allowedFileExtensionIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// AllowedFileExtensionListResourceModel represents the config model for allowed file
// extension list queries. Classic has no RSQL — the filter shape is the shared
// client-side substring block.
type AllowedFileExtensionListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
