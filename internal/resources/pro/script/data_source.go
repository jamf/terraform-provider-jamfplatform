// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ScriptDataSource implements the Terraform data source for Jamf Pro scripts.
type ScriptDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &ScriptDataSource{}
	_ datasource.DataSourceWithConfigure = &ScriptDataSource{}
)

// NewScriptDataSource returns a new instance of ScriptDataSource.
func NewScriptDataSource() datasource.DataSource {
	return &ScriptDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ScriptDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_script"
}

// Schema returns the data source schema.
func (d *ScriptDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro script by ID." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Script ID to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Script display name.",
				Computed:            true,
			},
			"category_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Jamf Pro category this script belongs to.",
				Computed:            true,
			},
			"category_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the category referenced by `category_id`.",
				Computed:            true,
			},
			"info": schema.StringAttribute{
				MarkdownDescription: "Informational text shown to end users.",
				Computed:            true,
			},
			"notes": schema.StringAttribute{
				MarkdownDescription: "Administrator-only notes.",
				Computed:            true,
			},
			"os_requirements": schema.StringAttribute{
				MarkdownDescription: "macOS version constraints (e.g. `13.0.x,14.0.x`).",
				Computed:            true,
			},
			"priority": schema.StringAttribute{
				MarkdownDescription: "Execution order. Valid values: `BEFORE`, `AFTER`, `AT_REBOOT`.",
				Computed:            true,
			},
			"parameter_4":  schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 4.", Computed: true},
			"parameter_5":  schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 5.", Computed: true},
			"parameter_6":  schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 6.", Computed: true},
			"parameter_7":  schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 7.", Computed: true},
			"parameter_8":  schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 8.", Computed: true},
			"parameter_9":  schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 9.", Computed: true},
			"parameter_10": schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 10.", Computed: true},
			"parameter_11": schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 11.", Computed: true},
			"script_contents": schema.StringAttribute{
				MarkdownDescription: "Raw script contents.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *ScriptDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_script")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a script by ID and populates Terraform state.
func (d *ScriptDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ScriptDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "The id attribute must be provided to read a Jamf Pro script.")
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := d.client.GetScriptV1(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro script", helpers.APIErrorDetail(err))
		return
	}
	assignScriptDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro script data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
