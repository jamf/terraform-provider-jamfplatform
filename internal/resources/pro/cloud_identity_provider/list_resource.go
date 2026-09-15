// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	pro.ListCloudIdpV1
//	pro.GetCloudAzureV1
//
// Status: current. Last reviewed 2026-09-05.
package cloud_identity_provider

import (
	"context"
	"errors"
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

// defaultListTimeout caps how long the list operation will wait on the
// Pro Cloud Identity Provider registry (/v1/cloud-idp) endpoint. The list resource schema does not
// expose a user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item read issued when IncludeResource
// is requested (config generation). Every provider gets its own deadline rather
// than a share of the list-fetch budget, so a slow read cannot exhaust a
// deadline the remaining providers still need. A provider whose read fails or
// times out is dropped from the generated configuration rather than aborting
// the whole resource type.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &CloudIdentityProviderListResource{}
	_ list.ListResourceWithConfigure = &CloudIdentityProviderListResource{}
)

// CloudIdentityProviderListResource implements Terraform query list support for
// Jamf Pro Cloud Identity Providers. The Pro Cloud Identity Provider registry (/v1/cloud-idp) list
// endpoint has no filter or RSQL parameters, so the optional `filter` block is
// applied client-side via filters.ApplyClassicFilter after the full list is
// fetched.
//
// A plain query is served from that one round trip: the registry returns the
// CloudIDPCommonResponse shape, which is everything an identity and a display
// name need. Config generation is not. The registry carries no connection
// settings at all, so a managed-resource state built from it alone leaves the
// provider-specific block empty, and the resource's own cross-field validator
// then refuses the generated configuration for naming a provider with no block
// to go with it. So when IncludeResource is requested each Microsoft Entra ID
// provider is additionally read individually and folded in through the shared
// assignAzureState builder.
//
// A Google provider is enumerated by a plain query and left out of a generating
// one entirely — not merely left unhydrated. Google Secure LDAP authenticates
// with a client certificate and its password, both schema-Required and
// write-only, and Jamf Pro returns neither, so no generated configuration for
// one can ever plan.
//
// The drop is total rather than identity-only because the framework treats a
// null Resource on an IncludeResource request as a provider bug: it emits the
// result and attaches its own "Incomplete List Result … This is always a
// problem with the provider. Please report this to the provider developers."
// warning. Streaming an identity-only Google row would hand the operator that
// on every generating query, and it generates no HCL anyway (ResourceObject is
// omitempty), so the import block it produced could not be planned either.
// googleSkipWarningDetail is therefore the only thing that names a dropped
// provider, and it carries the id an operator needs to import one by hand.
type CloudIdentityProviderListResource struct {
	client *pro.Client
}

// NewCloudIdentityProviderListResource returns a list resource for Jamf Pro
// Cloud Identity Provider queries.
func NewCloudIdentityProviderListResource() list.ListResource {
	return &CloudIdentityProviderListResource{}
}

// Metadata sets the list resource type name.
func (r *CloudIdentityProviderListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_identity_provider"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *CloudIdentityProviderListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_identity_provider")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *CloudIdentityProviderListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro Cloud Identity Providers (Google Secure LDAP and Microsoft Entra ID). " +
			"Supply an optional case-insensitive `name_substring` filter applied locally after the full list is fetched. " +
			"A plain query returns each provider's id and display name. When Terraform is generating configuration, every Microsoft Entra ID provider is read in full so its `entra_id` block is complete and the result can be planned. " +
			"A plain query lists a Google Secure LDAP provider; a query generating configuration leaves it out altogether and warns, naming each one it omitted and its id. Google Secure LDAP signs in with a client certificate and the password protecting it, neither of which Jamf Pro returns, so that resource has to be written by hand and then imported." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams Cloud Identity Provider identities back
// to Terraform.
func (r *CloudIdentityProviderListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config CloudIdentityProviderListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	all, err := r.client.ListCloudIdpV1(listCtx, nil)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro Cloud Identity Providers", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	all = filters.ApplyClassicFilter(all, filter, cloudIdentityProviderListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(all)) {
		maxResults = int64(len(all))
	}

	results := make([]list.ListResult, 0, maxResults)
	var skippedGoogle, skippedUnreadable []skippedCloudIdentityProvider

	for i := range all {
		if int64(len(results)) >= maxResults {
			break
		}
		item := all[i]

		result := req.NewListResult(ctx)
		result.DisplayName = item.DisplayName

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, cloudIdentityProviderIdentityModel{
			ID: types.StringValue(item.ID),
		})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancelItem := context.WithTimeout(ctx, defaultItemReadTimeout)
			read := func(readCtx context.Context, id string) (*pro.AzureConfiguration, error) {
				return r.client.GetCloudAzureV1(readCtx, id)
			}
			state, skip := hydrateListedCloudIdentityProvider(itemCtx, item, read)
			cancelItem()

			if skip != nil {
				switch skip.reason {
				case skipUnrepresentableGoogle:
					tflog.Debug(ctx, "Generating no configuration for a Google Secure LDAP Cloud Identity Provider", map[string]any{
						"id":   skip.id,
						"name": skip.name,
					})
					skippedGoogle = append(skippedGoogle, *skip)
				default:
					tflog.Warn(ctx, "Skipping Jamf Pro Cloud Identity Provider from generated configuration after a per-item read failure", map[string]any{
						"id":    skip.id,
						"name":  skip.name,
						"error": skip.err.Error(),
					})
					skippedUnreadable = append(skippedUnreadable, *skip)
				}
				continue
			}

			result.Diagnostics.Append(result.Resource.Set(ctx, state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro Cloud Identity Providers", map[string]any{
		"name_substring":     filter.NameSubstring.ValueString(),
		"limit":              req.Limit,
		"returned":           len(results),
		"skipped_google":     len(skippedGoogle),
		"skipped_unreadable": len(skippedUnreadable),
	})

	skipDiags := skippedCloudIdentityProviderDiagnostics(skippedGoogle, skippedUnreadable)

	if len(results) == 0 && len(skipDiags) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if len(skipDiags) > 0 {
			push(list.ListResult{Diagnostics: skipDiags})
		}
	}
}

// cloudIdentityProviderListItemName is the name accessor passed to
// filters.ApplyClassicFilter. DisplayName is a plain string field.
func cloudIdentityProviderListItemName(item pro.CloudIDPCommonResponse) string {
	return item.DisplayName
}

// azureConfigReader reads one Microsoft Entra ID configuration by id.
//
// The read is injected rather than reached for off the client so that the whole
// per-item decision in hydrateListedCloudIdentityProvider can be exercised
// without a tenant. The closure supplying it stays inside the IncludeResource
// block, which is where internal/conformance checks that a per-item read holds
// its own deadline.
type azureConfigReader func(ctx context.Context, id string) (*pro.AzureConfiguration, error)

// cloudIdentityProviderSkipReason says why an enumerated provider carries no
// generated configuration. The two causes need different explanations, because
// the operator's next move differs: a Google provider has to be written by
// hand, whereas a provider that could not be read is a permission or a lifetime
// problem to look into.
type cloudIdentityProviderSkipReason int

const (
	skipUnrepresentableGoogle cloudIdentityProviderSkipReason = iota
	skipUnreadable
)

// skippedCloudIdentityProvider records a provider the registry enumerated that
// carries no generated configuration, so the omission is reported once at the
// end of the stream rather than per item.
type skippedCloudIdentityProvider struct {
	id     string
	name   string
	reason cloudIdentityProviderSkipReason
	err    error
}

// errNoEntraConfiguration is the drop reason for an Entra ID provider whose
// read returned no connection settings. Folding that into state would put an
// empty `entra_id` block into the generated configuration, which is the exact
// failure this hydration exists to prevent, so it is treated as a failed read
// rather than a partial success.
var errNoEntraConfiguration = errors.New("the read returned no Entra ID connection details")

// hydrateListedCloudIdentityProvider builds the managed-resource state for one
// enumerated provider, or reports why that provider carries none. It is the
// hydrate-or-drop decision the IncludeResource path turns on, named so it can
// be tested without a live client or a framework list request.
//
// Anything that is not Google is read as Entra ID. The registry serves exactly
// two provider types, and a third would be dropped by the read that fails on it
// rather than by a guess made here.
//
// `mappings` is hydrated from the wire (hydrating=true). There is no plan to
// stay consistent with on a config-generation run, and the Read that follows an
// import of the same provider now hydrates it too, so the generated
// configuration and the state that import produces agree. Both used to leave it
// null, which agreed with each other but made a connection's real mappings
// unrepresentable (issue #391). A connection created without the block echoes
// eleven empty strings rather than server defaults, and hydration skips those,
// so most generated configurations still carry no mappings block.
func hydrateListedCloudIdentityProvider(ctx context.Context, item pro.CloudIDPCommonResponse, read azureConfigReader) (*CloudIdentityProviderResourceModel, *skippedCloudIdentityProvider) {
	if item.ProviderName == providerGoogle {
		return nil, &skippedCloudIdentityProvider{
			id:     item.ID,
			name:   item.DisplayName,
			reason: skipUnrepresentableGoogle,
		}
	}

	got, err := read(ctx, item.ID)
	if err == nil && (got == nil || got.Server == nil) {
		err = errNoEntraConfiguration
	}
	if err != nil {
		return nil, &skippedCloudIdentityProvider{
			id:     item.ID,
			name:   item.DisplayName,
			reason: skipUnreadable,
			err:    err,
		}
	}

	state := &CloudIdentityProviderResourceModel{
		ID:           types.StringValue(item.ID),
		DisplayName:  types.StringValue(item.DisplayName),
		ProviderName: types.StringValue(providerNameFromWire(item.ProviderName)),
		Timeouts:     helpers.NewResourceTimeoutsNullValue(cloudIdentityProviderTimeoutAttributeTypes),
	}
	assignAzureState(state, got, true)
	return state, nil
}

// skippedCloudIdentityProviderDiagnostics builds the trailing warnings for the
// providers left out of a config-generation run. A dropped result is silent
// otherwise: it generates no HCL and the stream has no diagnostics channel of
// its own, so a generated configuration would come back quietly short of the
// registry.
func skippedCloudIdentityProviderDiagnostics(google, unreadable []skippedCloudIdentityProvider) diag.Diagnostics {
	var diags diag.Diagnostics
	if len(google) > 0 {
		diags.AddWarning(
			"No configuration was generated for the Google Secure LDAP providers",
			googleSkipWarningDetail(google),
		)
	}
	if len(unreadable) > 0 {
		diags.AddWarning(
			"Some Jamf Pro Cloud Identity Providers were left out of the results",
			unreadableSkipWarningDetail(unreadable),
		)
	}
	return diags
}

// googleSkipWarningDetail names every Google provider dropped from a
// config-generation run. Terraform cannot generate one at all, so the message
// hands the operator the manual route instead of describing an obstacle and
// stopping there.
func googleSkipWarningDetail(skipped []skippedCloudIdentityProvider) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d Google Secure LDAP provider(s) were listed, but Terraform cannot generate a configuration for them:\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(&b, "  - %s (id %s)\n", s.name, s.id)
	}
	b.WriteString("\nGoogle Secure LDAP signs in with a client certificate and the password protecting it. Jamf Pro never returns either one, so a generated configuration would be missing both and Terraform would refuse to plan it. Write the resource by hand with a `google` block supplying the keystore, then run `terraform import` to bring the existing provider under management. Microsoft Entra ID providers are not affected.")
	return b.String()
}

// unreadableSkipWarningDetail names every provider the registry enumerated that
// could not be read individually, with the error that dropped it, so a short
// generated configuration is never mistaken for a complete one.
func unreadableSkipWarningDetail(skipped []skippedCloudIdentityProvider) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d Cloud Identity Provider(s) were listed by Jamf Pro but could not be read individually, so they carry no configuration and are not in the results:\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(&b, "  - %s (id %s): %s\n", s.name, s.id, s.err)
	}
	b.WriteString("\nCheck that the API integration holds permission to read Cloud Identity Providers, and that each provider still exists. One deleted between the two reads reports the same way.")
	return b.String()
}
