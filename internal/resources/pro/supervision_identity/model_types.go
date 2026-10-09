// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// SupervisionIdentityResourceModel represents the Terraform resource model for a
// Jamf Pro supervision identity.
//
// Password and CertificateData are WriteOnly: the framework strips them from
// plan and state, so they are read from req.Config in Create. Jamf Pro never
// returns either value, so they are never reconciled on read. CommonName and
// ExpirationDate are baked into the certificate at create and surfaced read-only.
type SupervisionIdentityResourceModel struct {
	ID              types.String           `tfsdk:"id"`
	DisplayName     types.String           `tfsdk:"display_name"`
	Password        types.String           `tfsdk:"password"`
	CertificateData types.String           `tfsdk:"certificate_data"`
	CommonName      types.String           `tfsdk:"common_name"`
	ExpirationDate  types.String           `tfsdk:"expiration_date"`
	Timeouts        resourceTimeouts.Value `tfsdk:"timeouts"`
}

// SupervisionIdentityDataSourceModel represents the Terraform data source model.
// Lookup is by id or display_name — exactly one must be supplied. The data source
// exposes only the read fields; it never surfaces the password or certificate.
type SupervisionIdentityDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	DisplayName    types.String             `tfsdk:"display_name"`
	CommonName     types.String             `tfsdk:"common_name"`
	ExpirationDate types.String             `tfsdk:"expiration_date"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// supervisionIdentityIdentityModel represents the identity object for the
// resource and list results.
type supervisionIdentityIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// SupervisionIdentityListResourceModel represents the config model for list
// queries. The endpoint accepts no server-side filter, so the optional filter is
// the shared client-side substring block applied after the full list is fetched.
type SupervisionIdentityListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
