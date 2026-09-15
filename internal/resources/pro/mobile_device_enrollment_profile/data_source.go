// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

type EnrollmentProfileDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &EnrollmentProfileDataSource{}
	_ datasource.DataSourceWithConfigure        = &EnrollmentProfileDataSource{}
	_ datasource.DataSourceWithConfigValidators = &EnrollmentProfileDataSource{}
)

func NewEnrollmentProfileDataSource() datasource.DataSource {
	return &EnrollmentProfileDataSource{}
}

func (d *EnrollmentProfileDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_enrollment_profile"
}

func dsComputedString(desc string) dsschema.StringAttribute {
	return dsschema.StringAttribute{MarkdownDescription: desc, Computed: true}
}

func (d *EnrollmentProfileDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Look up a Jamf Pro mobile device enrollment profile by ID, name, or invitation. Exactly one selector must be supplied." + dataSourcePrivileges,
		Attributes: map[string]dsschema.Attribute{
			"id":          dsschema.StringAttribute{MarkdownDescription: "Profile ID. Mutually exclusive with `name`/`invitation`.", Optional: true, Computed: true},
			"name":        dsschema.StringAttribute{MarkdownDescription: "Profile name (exact match). Mutually exclusive with `id`/`invitation`.", Optional: true, Computed: true},
			"invitation":  dsschema.StringAttribute{MarkdownDescription: "Invitation code (exact match). Mutually exclusive with `id`/`name`.", Optional: true, Computed: true},
			"description": dsComputedString("Description."),
			"site_id":     dsComputedString("Site ID."),
			"site_name":   dsComputedString("Site name."),
			"uuid":        dsComputedString("Profile UUID."),
			"location": dsschema.SingleNestedAttribute{
				MarkdownDescription: "User and Location Information.",
				Computed:            true,
				Attributes: map[string]dsschema.Attribute{
					"username":      dsComputedString("Username."),
					"real_name":     dsComputedString("Full name."),
					"email_address": dsComputedString("Email address."),
					"phone_number":  dsComputedString("Phone number."),
					"department":    dsComputedString("Department."),
					"building":      dsComputedString("Building."),
					"room":          dsComputedString("Room."),
					"position":      dsComputedString("Position."),
				},
			},
			"purchasing": dsschema.SingleNestedAttribute{
				MarkdownDescription: "Purchasing Information.",
				Computed:            true,
				Attributes: map[string]dsschema.Attribute{
					"is_purchased":           dsschema.BoolAttribute{MarkdownDescription: "Whether purchased.", Computed: true},
					"is_leased":              dsschema.BoolAttribute{MarkdownDescription: "Whether leased.", Computed: true},
					"po_number":              dsComputedString("Purchase order number."),
					"po_date":                dsComputedString("Purchase order date."),
					"po_date_epoch":          dsComputedString("Purchase order date epoch ms."),
					"po_date_utc":            dsComputedString("Purchase order date UTC."),
					"vendor":                 dsComputedString("Vendor."),
					"warranty_expires":       dsComputedString("Warranty expiration."),
					"warranty_expires_epoch": dsComputedString("Warranty expiration epoch ms."),
					"warranty_expires_utc":   dsComputedString("Warranty expiration UTC."),
					"applecare_id":           dsComputedString("AppleCare ID."),
					"lease_expires":          dsComputedString("Lease expiration."),
					"lease_expires_epoch":    dsComputedString("Lease expiration epoch ms."),
					"lease_expires_utc":      dsComputedString("Lease expiration UTC."),
					"purchase_price":         dsComputedString("Purchase price."),
					"life_expectancy":        dsschema.Int64Attribute{MarkdownDescription: "Life expectancy in years.", Computed: true},
					"purchasing_account":     dsComputedString("Purchasing account."),
					"purchasing_contact":     dsComputedString("Purchasing contact."),
				},
			},
			"attachments": dsschema.ListNestedAttribute{
				MarkdownDescription: "Attachments on the profile.",
				Computed:            true,
				NestedObject: dsschema.NestedAttributeObject{
					Attributes: map[string]dsschema.Attribute{
						"id":       dsComputedString("Attachment ID."),
						"filename": dsComputedString("Attachment filename."),
						"uri":      dsComputedString("Attachment download URI."),
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

func (d *EnrollmentProfileDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"), path.MatchRoot("invitation")),
	}
}

func (d *EnrollmentProfileDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_enrollment_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

func (d *EnrollmentProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The provider client was not configured.")
		return
	}
	var data EnrollmentProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, d2 := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(d2...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var (
		got *proclassic.MobileDeviceEnrollmentProfile
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetMobileDeviceEnrollmentProfileByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetMobileDeviceEnrollmentProfileByName(readCtx, data.Name.ValueString())
	case !data.Invitation.IsNull() && data.Invitation.ValueString() != "":
		got, err = d.client.GetMobileDeviceEnrollmentProfileByInvitation(readCtx, data.Invitation.ValueString())
	default:
		resp.Diagnostics.AddError("Missing selector", "Exactly one of id, name, or invitation must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err))
		return
	}
	assignEnrollmentProfileDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro mobile device enrollment profile data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
