// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation waits on the platform
// environments collection. The list fetch is the only SDK call the list
// resource makes; items are built from the list response, so there is no
// per-item read.
const defaultListTimeout = 5 * time.Minute

var (
	_ list.ListResource              = &PlatformEnvironmentListResource{}
	_ list.ListResourceWithConfigure = &PlatformEnvironmentListResource{}
)

// NewPlatformEnvironmentListResource returns a list resource for Jamf Account
// platform environment queries.
func NewPlatformEnvironmentListResource() list.ListResource {
	return &PlatformEnvironmentListResource{}
}

// PlatformEnvironmentListResource implements Terraform query and bulk-import
// support for Jamf Account platform environments.
//
// The collection accepts neither a filter nor a sort expression, so there is no
// filter block and no ordering to pin: the resource streams the environments the
// organization holds, in the order Jamf Account returns them.
type PlatformEnvironmentListResource struct {
	client *account.Client
}

// Metadata sets the list resource type name.
func (r *PlatformEnvironmentListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_platform_environment"
}

// Configure wires the Jamf Account client into the list resource via the shared
// providerdata.ConfigureAccount helper.
func (r *PlatformEnvironmentListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_platform_environment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the list configuration, which holds only the
// shared `timeouts` attribute.
func (r *PlatformEnvironmentListResource) ListResourceConfigSchema(ctx context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists the platform environments your Jamf Account organization holds, for `terraform " +
			"query` and for importing existing ones in bulk. Jamf Account offers no search arguments for " +
			"platform environments, so this list resource takes no filter configuration." +
			listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
	resp.Schema.Attributes = listtimeouts.Add(ctx, resp.Schema.Attributes)
}

// List executes the query and streams platform environment identities back to
// Terraform.
func (r *PlatformEnvironmentListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config PlatformEnvironmentListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := listtimeouts.Bound(ctx, req.Config, &diags, defaultListTimeout)
	defer cancel()
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	environments, err := r.client.ListEnvironments(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Account platform environments", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(environments)) {
		maxResults = int64(len(environments))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, environment := range environments {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = environment.Name

		identity := platformEnvironmentIdentityModel{ID: types.StringValue(environment.ID)}
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, identity)...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := PlatformEnvironmentResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(platformEnvironmentTimeoutAttributeTypes),
			}
			assignResourceModel(&state, &environment)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Account platform environments", map[string]any{
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
