// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"context"
	"fmt"
	"strings"
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

// defaultListTimeout caps how long the list operation waits on the deployments
// endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

// maxListedTitleSkips caps how many deployment names the skipped-deployments
// warning spells out before summarising the remainder as a count, so a tenant
// whose App Catalog read fails outright reports one readable warning rather than
// a wall of every deployment it holds.
const maxListedTitleSkips = 10

var _ list.ListResource = &AppInstallerListResource{}
var _ list.ListResourceWithConfigure = &AppInstallerListResource{}

// NewAppInstallerListResource returns a list resource for App Installer
// deployment queries.
func NewAppInstallerListResource() list.ListResource {
	return &AppInstallerListResource{}
}

// AppInstallerListResource implements Terraform query list support for App
// Installer deployments. The deployments endpoint has no server-side filter, so
// the optional `filter` block is applied client-side as a case-insensitive name
// substring.
//
// A plain query streams identity (id) and display name. When IncludeResource is
// requested (config generation) each deployment is additionally fetched
// individually and hydrated through the shared Read state-builder with
// includeUnmanaged=true, populating every wire-present block (scalars,
// notification_settings, self_service_settings) so the generated config is
// complete rather than scalar-only.
//
// titles is the provider-instance App Catalog snapshot, shared with the resource,
// the data source and every other App Installer construct in the configuration.
// Hydration needs it because app_title_name is schema-Required and the deployment
// read reports the title only as an id, so a generated configuration missing the
// name is not merely incomplete — Terraform refuses the whole run with "Missing
// Configuration for Required Attribute", taking every other resource type in the
// query down with it. It is read only on the IncludeResource path, so a plain
// query still costs exactly one request.
type AppInstallerListResource struct {
	client *pro.Client
	titles *providerdata.AppTitleCatalogCache
}

// Metadata sets the list resource type name.
func (r *AppInstallerListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_installer"
}

// Configure wires the Jamf Pro client into the list resource, and takes the
// provider-instance App Catalog title cache alongside it. Registering the cache
// here rather than fetching a catalog snapshot up front keeps a plain query free
// of it: the cache reads lazily on its first use, which only ever happens on the
// IncludeResource path.
func (r *AppInstallerListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_installer")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.titles = providerdata.ConfigureAppTitleCatalog(req.ProviderData, readAppTitleCatalog)
}

// ListResourceConfigSchema describes the supported list filters.
func (r *AppInstallerListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists App Installer deployments. Supply an optional case-insensitive `name_substring` filter applied locally after the full list is fetched. A plain query returns each deployment's id and display name; when Terraform is generating configuration each deployment is read in full, including the name of the App Catalog title it deploys." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams deployment identities back to Terraform.
func (r *AppInstallerListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config AppInstallerListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	entries, err := r.client.ListAppInstallerDeploymentsV1(listCtx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list App Installer deployments", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	entries = filters.ApplyClassicFilter(entries, filter, appInstallerListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(entries)) {
		maxResults = int64(len(entries))
	}

	results := make([]list.ListResult, 0, maxResults)
	var skippedUnnamedTitle []string

	for _, e := range entries {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = e.Name

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, appInstallerIdentityModel{ID: helpers.StringPointerValueOrNull(&e.ID)})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetAppInstallerDeploymentV1(itemCtx, e.ID)
			if err != nil {
				cancel()
				tflog.Warn(ctx, "Skipping app installer deployment from generated config after per-item read failure", map[string]any{
					"id":    e.ID,
					"error": err.Error(),
				})
				continue
			}
			state := AppInstallerResourceModel{
				ID:       helpers.StringPointerValueOrNull(&e.ID),
				Timeouts: helpers.NewResourceTimeoutsNullValue(appInstallerTimeoutAttributeTypes),
			}
			assignAppInstallerResourceModel(&state, got, true)
			named := hydrateAppTitleName(itemCtx, catalogOrNil(r.titles), &state)
			cancel()
			if !named {
				tflog.Warn(ctx, "Skipping app installer deployment from generated config because its App Catalog title could not be named", map[string]any{
					"id":           e.ID,
					"name":         e.Name,
					"app_title_id": state.AppTitleID.ValueString(),
				})
				skippedUnnamedTitle = append(skippedUnnamedTitle, fmt.Sprintf("%s (id %s)", e.Name, e.ID))
				continue
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed App Installer deployments", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
		"skipped":        len(skippedUnnamedTitle),
	})

	titleWarning := appTitleNameSkipWarning(skippedUnnamedTitle)

	if len(results) == 0 && len(titleWarning) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if len(titleWarning) > 0 {
			push(list.ListResult{Diagnostics: titleWarning})
		}
	}
}

// appInstallerListItemName is the name accessor passed to filters.ApplyClassicFilter.
func appInstallerListItemName(e pro.AppTitleDeploymentSummary) string {
	return e.Name
}

// hydrateAppTitleName names the App Catalog title a hydrated deployment deploys,
// reporting whether the generated configuration is usable at all.
//
// It is the resolve-or-drop decision the IncludeResource path turns on, split out
// so it can be exercised without a live client or a framework list request. The
// deployment read reports the title only as an id, and app_title_name is
// schema-Required, so a deployment whose id cannot be named must be dropped
// rather than emitted: Terraform refuses a required attribute written as null
// with "Missing Configuration for Required Attribute" and abandons the whole
// generation run, so one unnameable deployment would otherwise cost the operator
// every other resource type in the query.
//
// false covers both ways naming can fail — the App Catalog could not be read, or
// it holds no title with that id because the title has been withdrawn — and the
// caller treats them alike, because neither yields a name and the remedy
// (re-run once the catalog is reachable, or manage the deployment by hand) is
// reported the same way.
func hydrateAppTitleName(ctx context.Context, catalog titleCatalog, state *AppInstallerResourceModel) bool {
	name, ok := titleNameForID(ctx, catalog, state.AppTitleID.ValueString())
	if !ok {
		return false
	}
	state.AppTitleName = types.StringValue(name)
	return true
}

// appTitleNameSkipWarning renders the deployments dropped from generated
// configuration into one warning for the whole query.
//
// One consolidated diagnostic rather than one per deployment because a failed
// App Catalog read fails every deployment at once, and because a dropped item's
// own diagnostics never reach the stream — the item is not streamed. An empty
// input renders nothing, so the caller can append unconditionally.
//
// It is carried by a trailing diagnostics-only list.ListResult, which is the
// framework's own shape for a result that reports rather than enumerates: a
// literal with no DisplayName, nil Identity and nil Resource passes straight
// through, where one built with req.NewListResult would set both pointers and be
// rejected for carrying an empty identity. Attaching the warning to the first
// streamed deployment instead would blame a tenant-wide failure on whichever one
// happened to sort first.
func appTitleNameSkipWarning(skipped []string) diag.Diagnostics {
	var diags diag.Diagnostics
	if len(skipped) == 0 {
		return diags
	}

	noun := "deployment"
	if len(skipped) > 1 {
		noun = "deployments"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "No configuration was generated for the following App Installer %s, because the App Catalog title each one deploys could not be named:\n\n", noun)
	shown := skipped
	if len(shown) > maxListedTitleSkips {
		shown = shown[:maxListedTitleSkips]
	}
	for _, s := range shown {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	if len(skipped) > len(shown) {
		fmt.Fprintf(&b, "  … and %d more\n", len(skipped)-len(shown))
	}
	b.WriteString("\nA generated configuration has to name the title, and the name is only available from the App Catalog, so a deployment whose title cannot be looked up would generate configuration Terraform refuses to accept. Every other deployment was generated as normal. Re-run once the App Catalog is reachable; a deployment whose title is no longer published in the App Catalog cannot be managed by title name.")

	diags.AddWarning(
		fmt.Sprintf("Skipped %d App Installer %s that Terraform cannot generate configuration for", len(skipped), noun),
		b.String(),
	)
	return diags
}
