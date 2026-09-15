// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var _ list.ListResource = &BlueprintListResource{}
var _ list.ListResourceWithConfigure = &BlueprintListResource{}

// NewBlueprintListResource returns a list resource for blueprint queries.
func NewBlueprintListResource() list.ListResource {
	return &BlueprintListResource{}
}

// BlueprintListResource implements terraform query list support for blueprints.
type BlueprintListResource struct {
	client *blueprints.Client
}

// Metadata sets the list resource type name.
func (r *BlueprintListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blueprints_blueprint"
}

// Configure wires the provider client into the list resource.
func (r *BlueprintListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected List Configure Type",
			"Expected *providerdata.Data. Please report this issue to the provider developers.",
		)
		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_blueprints_blueprint", providerdata.BlueprintsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client = blueprints.New(pd.Client)
}

// ListResourceConfigSchema describes the supported list filters.
func (r *BlueprintListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf Blueprints with an optional case-insensitive substring filter." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"search": listschema.StringAttribute{
				Optional:    true,
				Description: "Optional substring to match against blueprint name or description (case-insensitive).",
			},
		},
	}
}

// List executes the query and streams blueprint identities back to Terraform.
func (r *BlueprintListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config BlueprintListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	searchTerm, hasSearch := helpers.NormalizedFilterString(config.Search)
	searchLower := strings.ToLower(searchTerm)

	blueprints, err := r.client.ListBlueprints(ctx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unable to list blueprints",
				helpers.APIErrorDetail(err),
			),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(blueprints)) {
		maxResults = int64(len(blueprints))
	}

	results := make([]list.ListResult, 0, maxResults)
	var emitted int64

	for _, bp := range blueprints {
		if hasSearch {
			nameMatch := strings.Contains(strings.ToLower(bp.Name), searchLower)
			descMatch := strings.Contains(strings.ToLower(helpers.DerefString(bp.Description)), searchLower)
			if !nameMatch && !descMatch {
				continue
			}
		}

		if maxResults > 0 && emitted >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = bp.Name

		identity := blueprintIdentityModel{
			ID: types.StringValue(bp.ID),
		}
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, identity)...)

		if req.IncludeResource {
			detail, err := r.client.GetBlueprint(ctx, bp.ID)
			if err != nil {
				stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
					diag.NewErrorDiagnostic(
						"Unable to read blueprint",
						helpers.APIErrorDetail(err),
					),
				})
				return
			}

			var state BlueprintResourceModel
			result.Diagnostics.Append(updateModelFromAPIResponse(ctx, &state, detail)...)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
		}

		results = append(results, result)
		emitted++
	}

	tflog.Debug(ctx, "Listed blueprints", map[string]any{
		"search":   searchTerm,
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
