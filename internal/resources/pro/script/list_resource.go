// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait on the Jamf Pro
// scripts endpoint. The list resource schema does not expose a user-overridable
// timeout, so this is a fixed safety bound — large tenants returning many scripts
// should still complete well inside this window.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &ScriptListResource{}
var _ list.ListResourceWithConfigure = &ScriptListResource{}

// NewScriptListResource returns a list resource for Jamf Pro script queries.
func NewScriptListResource() list.ListResource {
	return &ScriptListResource{}
}

// ScriptListResource implements Terraform query list support for Jamf Pro scripts.
type ScriptListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *ScriptListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_script"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *ScriptListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_script")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *ScriptListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro scripts. Supply an optional `filter` block to narrow results. The supported selectors match the scripts data source." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(ScriptFilterSelectors),
				ScriptFilterSelectors,
			),
		},
	}
}

// List executes the query and streams script identities back to Terraform.
func (r *ScriptListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config ScriptListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(ScriptFilterSelectors))
	tflog.Debug(ctx, "script list filters", map[string]any{"filter": filterExpression})

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListScriptsV1(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro scripts", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, s := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = s.Name

		id := helpers.StringPointerValueOrNull(s.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, scriptIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := ScriptResourceModel{
				ID:             id,
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
				Timeouts:       helpers.NewResourceTimeoutsNullValue(scriptTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro scripts", map[string]any{
		"filter":   filterExpression,
		"limit":    req.Limit,
		"returned": len(results),
	})

	if len(results) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}
