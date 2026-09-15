// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// ScriptFilterSelectors enumerates the RSQL selectors accepted by the scripts endpoint.
var ScriptFilterSelectors = []string{
	"id",
	"name",
	"info",
	"notes",
	"priority",
	"categoryId",
	"categoryName",
	"parameter4",
	"parameter5",
	"parameter6",
	"parameter7",
	"parameter8",
	"parameter9",
	"parameter10",
	"parameter11",
	"osRequirements",
	"scriptContents",
}

// ScriptsDataSource implements the Terraform data source for Jamf Pro script searches.
type ScriptsDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &ScriptsDataSource{}
	_ datasource.DataSourceWithConfigure = &ScriptsDataSource{}
)

// NewScriptsDataSource returns a new instance of ScriptsDataSource.
func NewScriptsDataSource() datasource.DataSource {
	return &ScriptsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ScriptsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_scripts"
}

// Schema returns the plural data source schema.
func (d *ScriptsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Search Jamf Pro scripts using optional RSQL filters." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(ScriptFilterSelectors),
				ScriptFilterSelectors,
			),
			"scripts": schema.ListNestedAttribute{
				MarkdownDescription: "Scripts matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{MarkdownDescription: "Script ID assigned by Jamf Pro.", Computed: true},
						"name":            schema.StringAttribute{MarkdownDescription: "Script display name.", Computed: true},
						"category_id":     schema.StringAttribute{MarkdownDescription: "ID of the Jamf Pro category.", Computed: true},
						"category_name":   schema.StringAttribute{MarkdownDescription: "Display name of the category.", Computed: true},
						"info":            schema.StringAttribute{MarkdownDescription: "Informational text shown to end users.", Computed: true},
						"notes":           schema.StringAttribute{MarkdownDescription: "Administrator-only notes.", Computed: true},
						"os_requirements": schema.StringAttribute{MarkdownDescription: "macOS version constraints.", Computed: true},
						"priority":        schema.StringAttribute{MarkdownDescription: "Execution order. One of `BEFORE`, `AFTER`, `AT_REBOOT`.", Computed: true},
						"parameter_4":     schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 4.", Computed: true},
						"parameter_5":     schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 5.", Computed: true},
						"parameter_6":     schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 6.", Computed: true},
						"parameter_7":     schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 7.", Computed: true},
						"parameter_8":     schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 8.", Computed: true},
						"parameter_9":     schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 9.", Computed: true},
						"parameter_10":    schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 10.", Computed: true},
						"parameter_11":    schema.StringAttribute{MarkdownDescription: "Label for script parameter slot 11.", Computed: true},
						"script_contents": schema.StringAttribute{MarkdownDescription: "Raw script contents.", Computed: true},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *ScriptsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_scripts")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches scripts matching the supplied filters and populates state.
func (d *ScriptsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ScriptsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(ScriptFilterSelectors))
	tflog.Debug(ctx, "scripts filter expression", map[string]any{"filter": filterExpression})

	list, err := d.client.ListScriptsV1(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro scripts", helpers.APIErrorDetail(err))
		return
	}

	results := make([]ScriptsDataSourceResultModel, 0, len(list))
	for _, s := range list {
		results = append(results, ScriptsDataSourceResultModel{
			ID:             helpers.StringPointerValueOrNull(s.ID),
			Name:           types.StringValue(s.Name),
			CategoryID:     helpers.StringPointerValueOrNull(s.CategoryID),
			CategoryName:   helpers.StringPointerValueOrNull(s.CategoryName),
			Info:           helpers.StringPointerValueOrNull(s.Info),
			Notes:          helpers.StringPointerValueOrNull(s.Notes),
			OsRequirements: helpers.StringPointerValueOrNull(s.OsRequirements),
			Priority:       helpers.StringPointerValueOrNull(s.Priority),
			Parameter4:     helpers.StringPointerValueOrNull(s.Parameter4),
			Parameter5:     helpers.StringPointerValueOrNull(s.Parameter5),
			Parameter6:     helpers.StringPointerValueOrNull(s.Parameter6),
			Parameter7:     helpers.StringPointerValueOrNull(s.Parameter7),
			Parameter8:     helpers.StringPointerValueOrNull(s.Parameter8),
			Parameter9:     helpers.StringPointerValueOrNull(s.Parameter9),
			Parameter10:    helpers.StringPointerValueOrNull(s.Parameter10),
			Parameter11:    helpers.StringPointerValueOrNull(s.Parameter11),
			ScriptContents: helpers.StringPointerValueOrNull(s.ScriptContents),
		})
	}

	data.Scripts = results
	data.ID = types.StringValue("scripts")

	tflog.Trace(ctx, "listed Jamf Pro scripts data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
