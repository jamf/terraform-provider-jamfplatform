// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_external_source

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/availabletitles"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// PatchExternalSourceResourceModel represents the Terraform resource model for a
// Jamf Pro patch external source.
type PatchExternalSourceResourceModel struct {
	ID                           types.String           `tfsdk:"id"`
	Name                         types.String           `tfsdk:"name"`
	Enabled                      types.Bool             `tfsdk:"enabled"`
	HostName                     types.String           `tfsdk:"host_name"`
	Port                         types.Int64            `tfsdk:"port"`
	SslEnabled                   types.Bool             `tfsdk:"ssl_enabled"`
	CertificateValidationEnabled types.Bool             `tfsdk:"certificate_validation_enabled"`
	Timeouts                     resourceTimeouts.Value `tfsdk:"timeouts"`
}

// PatchExternalSourceDataSourceModel represents the Terraform data source model.
// Either id or name must be supplied (enforced by ExactlyOneOf at config
// validation); all remaining attributes are populated from the SDK response.
type PatchExternalSourceDataSourceModel struct {
	ID                           types.String             `tfsdk:"id"`
	Name                         types.String             `tfsdk:"name"`
	Enabled                      types.Bool               `tfsdk:"enabled"`
	HostName                     types.String             `tfsdk:"host_name"`
	Port                         types.Int64              `tfsdk:"port"`
	SslEnabled                   types.Bool               `tfsdk:"ssl_enabled"`
	CertificateValidationEnabled types.Bool               `tfsdk:"certificate_validation_enabled"`
	AvailableTitles              []availabletitles.Model  `tfsdk:"available_titles"`
	Timeouts                     datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// patchExternalSourceIdentityModel represents the identity object for the
// resource and list results.
type patchExternalSourceIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// PatchExternalSourceListResourceModel represents the config model for list
// queries. Classic has no RSQL — the filter shape is the shared client-side
// substring block.
type PatchExternalSourceListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
