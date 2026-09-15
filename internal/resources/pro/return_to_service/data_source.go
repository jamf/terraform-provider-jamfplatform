// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package return_to_service

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ReturnToServiceDataSource implements the Terraform data source for Jamf Pro
// Return to Service configurations. Lookup is by ID or by exact display name —
// exactly one must be supplied. Display names are not unique, so a name lookup
// that matches more than one configuration surfaces an ambiguity error.
type ReturnToServiceDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &ReturnToServiceDataSource{}
	_ datasource.DataSourceWithConfigure        = &ReturnToServiceDataSource{}
	_ datasource.DataSourceWithConfigValidators = &ReturnToServiceDataSource{}
)

// NewReturnToServiceDataSource returns a new instance of the data source.
func NewReturnToServiceDataSource() datasource.DataSource {
	return &ReturnToServiceDataSource{}
}

// Metadata sets the data source type name.
func (d *ReturnToServiceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_return_to_service"
}

// Schema returns the data source schema.
func (d *ReturnToServiceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Return to Service configuration by ID or by exact display name. Exactly one of `id` or `display_name` must be supplied. Display names are not guaranteed unique: a name matching more than one configuration returns an error." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Return to Service configuration ID. Mutually exclusive with `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Return to Service configuration display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"wifi_profile_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Wi-Fi configuration profile a device rejoins during Return to Service.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / display_name is supplied.
func (d *ReturnToServiceDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("display_name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *ReturnToServiceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_return_to_service")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a Return to Service configuration by ID or by display name.
func (d *ReturnToServiceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ReturnToServiceDataSourceModel
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
		got *pro.ReturnToServiceConfiguration
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetReturnToServiceConfigurationV1(readCtx, data.ID.ValueString())
	case !data.DisplayName.IsNull() && data.DisplayName.ValueString() != "":
		got, err = d.client.ResolveReturnToServiceConfigurationV1ByName(readCtx, data.DisplayName.ValueString())
	default:
		resp.Diagnostics.AddError("Missing Return to Service configuration selector", "Exactly one of id or display_name must be supplied.")
		return
	}
	if err != nil {
		// Display names are not unique, so a name lookup can match more than one
		// configuration; surface that as a distinct, actionable diagnostic rather
		// than a generic not-found.
		if _, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err); ok {
			resp.Diagnostics.AddError(
				"Multiple Jamf Pro Return to Service configurations match this display name",
				helpers.APIErrorDetail(err)+". Look the configuration up by id instead.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to find Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
		return
	}

	assignReturnToServiceDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro Return to Service configuration data source", map[string]any{"id": data.ID.ValueString(), "display_name": data.DisplayName.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
