// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// EnrollmentProfileResourceModel is the Terraform resource model.
type EnrollmentProfileResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SiteID      types.String `tfsdk:"site_id"`
	SiteName    types.String `tfsdk:"site_name"`
	Invitation  types.String `tfsdk:"invitation"`
	UUID        types.String `tfsdk:"uuid"`

	Location    *LocationModel         `tfsdk:"location"`
	Purchasing  *PurchasingModel       `tfsdk:"purchasing"`
	Attachments types.List             `tfsdk:"attachments"` // Computed list of AttachmentModel
	Timeouts    resourceTimeouts.Value `tfsdk:"timeouts"`
}

// LocationModel mirrors the User and Location Information tab.
type LocationModel struct {
	Username     types.String `tfsdk:"username"`
	RealName     types.String `tfsdk:"real_name"`
	EmailAddress types.String `tfsdk:"email_address"`
	PhoneNumber  types.String `tfsdk:"phone_number"`
	Department   types.String `tfsdk:"department"`
	Building     types.String `tfsdk:"building"`
	Room         types.String `tfsdk:"room"`
	Position     types.String `tfsdk:"position"`
}

// PurchasingModel mirrors the Purchasing Information tab. The *_epoch / *_utc
// date siblings are server-derived from the date strings and are Computed.
type PurchasingModel struct {
	IsPurchased       types.Bool   `tfsdk:"is_purchased"`
	IsLeased          types.Bool   `tfsdk:"is_leased"`
	PONumber          types.String `tfsdk:"po_number"`
	PODate            types.String `tfsdk:"po_date"`
	PODateEpoch       types.String `tfsdk:"po_date_epoch"`
	PODateUTC         types.String `tfsdk:"po_date_utc"`
	Vendor            types.String `tfsdk:"vendor"`
	WarrantyExpires   types.String `tfsdk:"warranty_expires"`
	WarrantyEpoch     types.String `tfsdk:"warranty_expires_epoch"`
	WarrantyUTC       types.String `tfsdk:"warranty_expires_utc"`
	AppleCareID       types.String `tfsdk:"applecare_id"`
	LeaseExpires      types.String `tfsdk:"lease_expires"`
	LeaseEpoch        types.String `tfsdk:"lease_expires_epoch"`
	LeaseUTC          types.String `tfsdk:"lease_expires_utc"`
	PurchasePrice     types.String `tfsdk:"purchase_price"`
	LifeExpectancy    types.Int64  `tfsdk:"life_expectancy"`
	PurchasingAccount types.String `tfsdk:"purchasing_account"`
	PurchasingContact types.String `tfsdk:"purchasing_contact"`
}

// AttachmentModel is a read-only attachment listing element.
type AttachmentModel struct {
	ID       types.String `tfsdk:"id"`
	Filename types.String `tfsdk:"filename"`
	URI      types.String `tfsdk:"uri"`
}

// EnrollmentProfileDataSourceModel is the data source model. Lookup by exactly
// one of id / name / invitation.
type EnrollmentProfileDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Invitation  types.String `tfsdk:"invitation"`
	Description types.String `tfsdk:"description"`
	SiteID      types.String `tfsdk:"site_id"`
	SiteName    types.String `tfsdk:"site_name"`
	UUID        types.String `tfsdk:"uuid"`

	Location    *LocationModel           `tfsdk:"location"`
	Purchasing  *PurchasingModel         `tfsdk:"purchasing"`
	Attachments types.List               `tfsdk:"attachments"`
	Timeouts    datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// enrollmentProfileIdentityModel is the identity object for the resource + list.
type enrollmentProfileIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// EnrollmentProfileListResourceModel is the list config model.
type EnrollmentProfileListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
