// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package automated_device_enrollment

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// AutomatedDeviceEnrollmentResourceModel represents the Terraform resource
// model for a Jamf Pro Automated Device Enrollment (ADE) instance.
//
// `ServerToken` and `ServerTokenWoVersion` together form the WriteOnly token
// rotation pair: the plaintext base64 server token is sent on the wire but
// never persisted in Terraform state; bumping `ServerTokenWoVersion` triggers
// a `ReplaceDeviceEnrollmentTokenV1` call on Update.
type AutomatedDeviceEnrollmentResourceModel struct {
	ID                    types.String           `tfsdk:"id"`
	Name                  types.String           `tfsdk:"name"`
	ServerToken           types.String           `tfsdk:"server_token"`
	ServerTokenWoVersion  types.Int64            `tfsdk:"server_token_wo_version"`
	TokenFileName         types.String           `tfsdk:"token_file_name"`
	SiteID                types.String           `tfsdk:"site_id"`
	SupervisionIdentityID types.String           `tfsdk:"supervision_identity_id"`
	AdminID               types.String           `tfsdk:"admin_id"`
	OrgName               types.String           `tfsdk:"org_name"`
	OrgEmail              types.String           `tfsdk:"org_email"`
	OrgPhone              types.String           `tfsdk:"org_phone"`
	OrgAddress            types.String           `tfsdk:"org_address"`
	ServerName            types.String           `tfsdk:"server_name"`
	ServerUUID            types.String           `tfsdk:"server_uuid"`
	TokenExpirationDate   types.String           `tfsdk:"token_expiration_date"`
	Timeouts              resourceTimeouts.Value `tfsdk:"timeouts"`
}

// automatedDeviceEnrollmentIdentityModel represents the identity object for the
// automated device enrollment resource. Used for import and list results.
type automatedDeviceEnrollmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// AutomatedDeviceEnrollmentDataSourceModel is the Terraform data source model
// for the singular `jamfplatform_pro_automated_device_enrollment` data source.
// Lookup is by `id` OR `name` (ExactlyOneOf). Mirrors the resource shape
// minus the WriteOnly / upload-only attributes (`server_token`,
// `server_token_wo_version`, `token_file_name`) which the Jamf Pro GET
// response never echoes back.
type AutomatedDeviceEnrollmentDataSourceModel struct {
	ID                    types.String             `tfsdk:"id"`
	Name                  types.String             `tfsdk:"name"`
	SiteID                types.String             `tfsdk:"site_id"`
	SupervisionIdentityID types.String             `tfsdk:"supervision_identity_id"`
	AdminID               types.String             `tfsdk:"admin_id"`
	OrgName               types.String             `tfsdk:"org_name"`
	OrgEmail              types.String             `tfsdk:"org_email"`
	OrgPhone              types.String             `tfsdk:"org_phone"`
	OrgAddress            types.String             `tfsdk:"org_address"`
	ServerName            types.String             `tfsdk:"server_name"`
	ServerUUID            types.String             `tfsdk:"server_uuid"`
	TokenExpirationDate   types.String             `tfsdk:"token_expiration_date"`
	Timeouts              datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// AutomatedDeviceEnrollmentListResourceModel is the config model for the list
// resource. The Pro `/v1/device-enrollments` list endpoint accepts no RSQL
// filter, so the optional `filter` block reuses the shared client-side
// substring matcher.
type AutomatedDeviceEnrollmentListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
