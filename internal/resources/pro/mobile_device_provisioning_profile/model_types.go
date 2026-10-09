// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// ProvisioningProfileResourceModel represents the Terraform resource model for a
// Jamf Pro mobile device provisioning profile.
type ProvisioningProfileResourceModel struct {
	ID                  types.String           `tfsdk:"id"`
	Name                types.String           `tfsdk:"name"`
	DisplayName         types.String           `tfsdk:"display_name"`
	ProfileData         types.String           `tfsdk:"profile_data"`
	UUID                types.String           `tfsdk:"uuid"`
	CreationDateUTC     types.String           `tfsdk:"creation_date_utc"`
	CreationDateEpoch   types.String           `tfsdk:"creation_date_epoch"`
	ExpirationDateUTC   types.String           `tfsdk:"expiration_date_utc"`
	ExpirationDateEpoch types.String           `tfsdk:"expiration_date_epoch"`
	Timeouts            resourceTimeouts.Value `tfsdk:"timeouts"`
}

// ProvisioningProfileDataSourceModel represents the Terraform data source model.
// Lookup is by id, name, or uuid — exactly one must be supplied (enforced by
// ExactlyOneOf at config validation).
type ProvisioningProfileDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	Name                types.String             `tfsdk:"name"`
	DisplayName         types.String             `tfsdk:"display_name"`
	UUID                types.String             `tfsdk:"uuid"`
	ProfileData         types.String             `tfsdk:"profile_data"`
	CreationDateUTC     types.String             `tfsdk:"creation_date_utc"`
	CreationDateEpoch   types.String             `tfsdk:"creation_date_epoch"`
	ExpirationDateUTC   types.String             `tfsdk:"expiration_date_utc"`
	ExpirationDateEpoch types.String             `tfsdk:"expiration_date_epoch"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// provisioningProfileIdentityModel represents the identity object for the
// resource and list results.
type provisioningProfileIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// ProvisioningProfileListResourceModel represents the config model for list
// queries. Classic has no RSQL — the filter shape is the shared client-side
// substring block.
type ProvisioningProfileListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
