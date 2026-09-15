// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// AppRequestFormFieldDataSource implements the Terraform data source for Jamf Pro App
// Request form fields. The singular data source supports lookup by ID OR by title —
// exactly one of the two must be supplied. Titles are not unique on the server, so a
// by-title lookup that matches more than one field returns an ambiguous-match error.
type AppRequestFormFieldDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &AppRequestFormFieldDataSource{}
	_ datasource.DataSourceWithConfigure        = &AppRequestFormFieldDataSource{}
	_ datasource.DataSourceWithConfigValidators = &AppRequestFormFieldDataSource{}
)

// NewAppRequestFormFieldDataSource returns a new instance of AppRequestFormFieldDataSource.
func NewAppRequestFormFieldDataSource() datasource.DataSource {
	return &AppRequestFormFieldDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AppRequestFormFieldDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_request_form_field"
}

// Schema returns the data source schema.
func (d *AppRequestFormFieldDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro App Request form field by ID or by title. Exactly one of `id` or `title` must be supplied. Titles are not unique, so a by-title lookup that matches more than one field returns an error. Use `id` to disambiguate." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App Request form field ID. Mutually exclusive with `title`.",
				Optional:            true,
				Computed:            true,
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "Form field title. Mutually exclusive with `id`. Must match exactly one field.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Helper text shown beneath the field title on the App Request form.",
				Computed:            true,
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Display order of the field on the App Request form (ascending).",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / title is supplied.
func (d *AppRequestFormFieldDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("title"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *AppRequestFormFieldDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_request_form_field")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an App Request form field by ID or by title and populates state.
func (d *AppRequestFormFieldDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AppRequestFormFieldDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var (
		got *pro.AppRequestFormInputField
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetAppRequestFormInputFieldV1(readCtx, data.ID.ValueString())
	case !data.Title.IsNull() && data.Title.ValueString() != "":
		got, err = d.client.ResolveAppRequestFormInputFieldV1ByName(readCtx, data.Title.ValueString())
	default:
		resp.Diagnostics.AddError("Missing App Request form field selector", "Exactly one of id or title must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro App Request form field", helpers.APIErrorDetail(err))
		return
	}
	assignAppRequestFormFieldDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro App Request form field data source", map[string]any{"id": data.ID.ValueString(), "title": data.Title.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
