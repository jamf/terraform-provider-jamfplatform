// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item classic GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow account cannot
// exhaust a shared deadline part-way through a high-cardinality tenant.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &AccountListResource{}
var _ list.ListResourceWithConfigure = &AccountListResource{}

// NewAccountListResource returns a list resource for account queries.
func NewAccountListResource() list.ListResource {
	return &AccountListResource{}
}

// AccountListResource implements query list support for admin accounts via the
// Pro v1 /accounts endpoint. The optional `filter` block is applied client-side
// on the username after the full list is fetched. The classic client is held
// alongside the Pro one because the Custom privilege grid lives only on the
// classic account representation, and config generation has to materialise it.
type AccountListResource struct {
	client        *pro.Client
	classicClient *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *AccountListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_account"
}

// Configure wires both the Pro and ProClassic clients into the list resource
// (one underlying jamfplatform.Client serves both surfaces), mirroring
// AccountResource.Configure: hydrating a listed account for config generation
// needs the classic read as well as the Pro one.
func (r *AccountListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	classicClient, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.classicClient = classicClient
}

// ListResourceConfigSchema describes the supported list filters.
func (r *AccountListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro administrator accounts. Supply an optional case-insensitive `name_substring` filter on the username; filtering is applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams account identities to Terraform.
//
// When IncludeResource is set (config generation), an account whose privilege
// set is Custom and whose access level is Full Access has its privilege grid
// hydrated from the classic representation, because the Pro list carries base
// fields only and `privileges` is Optional: a generated config that omitted
// the block would bring the account under management with the whole grid out
// of state, and ModifyPlan returns early on a nil block so no later plan would
// reveal it. A privilege read that fails is reported as a warning on that
// item's own diagnostics and leaves its `privileges` block unset, rather than
// aborting the stream: one unreadable account must not cost the operator every
// other account in the listing, and the item is still emitted with its real
// base fields so the warning has something to point at (a diagnostics-only
// ListResult is silently dropped).
func (r *AccountListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil || r.classicClient == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unconfigured Provider", "The provider has not been configured yet."),
		})
		return
	}

	var config AccountListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	accounts, err := r.client.ListAccountsV1(listCtx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro accounts", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items := filters.ApplyClassicFilter(accounts, filter, accountUsername)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)
	for i := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		a := items[i]
		id := helpers.StringPointerValueOrNull(a.ID)
		result := req.NewListResult(ctx)
		result.DisplayName = accountUsername(a)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, accountIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := AccountResourceModel{Timeouts: helpers.NewResourceTimeoutsNullValue(accountTimeoutAttributeTypes)}
			assignProBaseFields(&state, &a)
			state.PasswordWOVersion = types.Int64Null()

			if custPrivApplicable(state.PrivilegeSet, state.AccessLevel) {
				itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
				classicGot, err := r.classicClient.GetAccountByUserID(itemCtx, id.ValueString())
				cancel()
				if err != nil {
					result.Diagnostics.AddWarning(
						"Unable to read Jamf Pro account privileges",
						"The privilege grid for account "+id.ValueString()+" could not be read, so the generated configuration omits its `privileges` block. "+
							"Add the block by hand before applying, or the account will be managed with its privileges left out of Terraform state. Error: "+err.Error(),
					)
				} else {
					result.Diagnostics.Append(assignClassicPrivileges(ctx, &state, classicGot, true)...)
					if result.Diagnostics.HasError() {
						stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
						return
					}
				}
			}

			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro accounts", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
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

// accountUsername is the username accessor for the client-side filter.
func accountUsername(a pro.UserAccount) string {
	if a.Username == nil {
		return ""
	}
	return *a.Username
}
