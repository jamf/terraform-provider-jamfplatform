// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetEnrollmentCustomizationV2
//   pro.ResolveEnrollmentCustomizationV2ByName
// Status: current. Last reviewed 2026-05-28.

package enrollment_customization

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// EnrollmentCustomizationDataSource implements the Terraform data source for
// looking up a single enrollment customization by ID or by display name.
type EnrollmentCustomizationDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &EnrollmentCustomizationDataSource{}
	_ datasource.DataSourceWithConfigure        = &EnrollmentCustomizationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &EnrollmentCustomizationDataSource{}
)

// NewEnrollmentCustomizationDataSource returns a new instance of the data
// source.
func NewEnrollmentCustomizationDataSource() datasource.DataSource {
	return &EnrollmentCustomizationDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *EnrollmentCustomizationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_enrollment_customization"
}

// Schema returns the data source schema. The data source returns only the
// parent record (display name, description, site, branding palette + icon
// URL) — managing the panes is the resource's responsibility.
func (d *EnrollmentCustomizationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro enrollment customization by ID or by exact display name. Exactly one of `id` or `display_name` must be supplied. Display names are not enforced unique by Jamf Pro; the lookup surfaces an error when more than one customization matches." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Enrollment customization ID. Mutually exclusive with `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Customization display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Administrator-visible description.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site ID associated with the customization, or the sentinel `\"-1\"` when no site is set.",
				Computed:            true,
			},
			"branding_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Branding palette plus icon URL returned by Jamf Pro.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"body_text_color":   schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Body Text Color\"** in the Jamf Pro admin UI."},
					"button_color":      schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Button Color\"** in the Jamf Pro admin UI."},
					"button_text_color": schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Button Text Color\"** in the Jamf Pro admin UI."},
					"background_color":  schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Background Color\"** in the Jamf Pro admin UI."},
					"icon_url":          schema.StringAttribute{Computed: true, MarkdownDescription: "URL of the uploaded icon image."},
				},
			},
		},
	}
}

// ConfigValidators enforces that exactly one of id / display_name is
// supplied.
func (d *EnrollmentCustomizationDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("display_name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *EnrollmentCustomizationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_enrollment_customization")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a customization by ID or display name. Lookups by name return
// `*pro.AmbiguousMatchError` (via the SDK transport layer) when the name
// resolves to multiple customizations — the error string is preserved so the
// user sees the candidate set.
func (d *EnrollmentCustomizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data EnrollmentCustomizationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		got *pro.EnrollmentCustomizationV2
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetEnrollmentCustomizationV2(ctx, data.ID.ValueString())
	case !data.DisplayName.IsNull() && data.DisplayName.ValueString() != "":
		got, err = d.client.ResolveEnrollmentCustomizationV2ByName(ctx, data.DisplayName.ValueString())
	default:
		resp.Diagnostics.AddError("Missing enrollment customization selector", "Exactly one of id or display_name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
		return
	}

	assignParentToDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro enrollment customization data source", map[string]any{"id": data.ID.ValueString(), "display_name": data.DisplayName.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
