// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait on the classic
// /computerinvitations endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &ComputerInvitationListResource{}
	_ list.ListResourceWithConfigure = &ComputerInvitationListResource{}
)

// NewComputerInvitationListResource returns a list resource for Jamf Pro
// computer invitation queries.
func NewComputerInvitationListResource() list.ListResource {
	return &ComputerInvitationListResource{}
}

// ComputerInvitationListResource implements Terraform query list support for
// Jamf Pro computer invitations. The classic /computerinvitations endpoint
// accepts no query parameters and the list-item type carries no name, so there
// is no filter block (unlike the inventory list resources) — the resource
// returns every invitation. Note: the LIST endpoint lags newly created
// invitations, so freshly created records may not appear here immediately even
// though GET-by-id returns them; the resource Read path uses GET-by-id and is
// unaffected.
type ComputerInvitationListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *ComputerInvitationListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_invitation"
}

// Configure wires the Jamf ProClassic client into the list resource via the
// shared providerdata.ConfigureProClassic helper.
func (r *ComputerInvitationListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_invitation")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *ComputerInvitationListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists all Jamf Pro computer enrollment invitations. Invitations carry no name and Jamf Pro exposes no filter parameters for them, so this list resource takes no filter configuration." + listResourcePrivileges,
		Attributes:  map[string]listschema.Attribute{},
	}
}

// List executes the query and streams computer invitation identities back to
// Terraform.
func (r *ComputerInvitationListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListComputerInvitations(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro computer invitations", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.ComputerInvitationsItemComputerInvitation{}
	if resp != nil {
		items = resp.ComputerInvitations
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, item := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		id := helpers.StringValueFromIntPtr(item.ID)

		result := req.NewListResult(ctx)
		result.DisplayName = id.ValueString()

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, computerInvitationIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The list endpoint returns only id / invitation / invitation_type /
			// expiration. Re-fetch the full record by id so the populated
			// resource state matches a managed resource. (LIST lag does not
			// apply to GET-by-id.)
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetComputerInvitationByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping computer invitation from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := ComputerInvitationResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(computerInvitationTimeoutAttributeTypes),
			}
			assignComputerInvitationResourceModel(&state, got)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro computer invitations", map[string]any{
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
