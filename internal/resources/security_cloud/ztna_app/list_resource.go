// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var (
	_ list.ListResource              = &ZtnaAppListResource{}
	_ list.ListResourceWithConfigure = &ZtnaAppListResource{}
)

// NewZtnaAppListResource returns a list resource for Jamf Security Cloud ZTNA
// access policy application queries.
func NewZtnaAppListResource() list.ListResource {
	return &ZtnaAppListResource{}
}

// ZtnaAppListResource implements Terraform query list support for Jamf Security
// Cloud ZTNA access policy applications. The application list endpoint accepts
// neither a filter nor a sort expression, so there is no filter block — the resource
// returns every application on the tenant, in the server's order.
type ZtnaAppListResource struct {
	client *securitycloud.Client
}

// Metadata sets the list resource type name.
func (r *ZtnaAppListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_app"
}

// Configure wires the Jamf Security Cloud client into the list resource via the
// shared providerdata.ConfigureSecurityCloud helper.
func (r *ZtnaAppListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *ZtnaAppListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists every Jamf Security Cloud access policy application on the tenant. Jamf Security " +
			"Cloud exposes no filter parameters for applications, so this list resource takes no filter " +
			"configuration." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams application identities back to Terraform.
//
// Results carry no security block: each card is Optional-only on the resource and the
// state builder fills one only when the target already declares it, so a listed
// application arrives with security unset for the same reason an imported one does —
// adopting Jamf's defaults into state would put values there the configuration never
// wrote.
func (r *ZtnaAppListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config ZtnaAppListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	apps, err := r.client.ListZtnaAppsV1(ctx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Security Cloud access policy applications", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(apps)) {
		maxResults = int64(len(apps))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, app := range apps {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = displayNameFor(app)

		id := types.StringValue(app.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, ztnaAppIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := ZtnaAppResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(ztnaAppTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignAppResourceModel(ctx, &state, &app)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Security Cloud access policy applications", map[string]any{
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

// displayNameFor labels a list result. A predefined application has no name of its
// own, so its predefined app ID stands in rather than leaving the row blank.
func displayNameFor(app securitycloud.App) string {
	if app.Name != nil && *app.Name != "" {
		return *app.Name
	}
	if app.PredefinedAppID != nil && *app.PredefinedAppID != "" {
		return *app.PredefinedAppID
	}
	return app.ID
}
