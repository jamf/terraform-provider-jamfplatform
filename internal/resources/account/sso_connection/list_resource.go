// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var (
	_ list.ListResource              = &ConnectionListResource{}
	_ list.ListResourceWithConfigure = &ConnectionListResource{}
)

// NewConnectionListResource returns a list resource for Jamf Account SSO
// connection queries.
func NewConnectionListResource() list.ListResource {
	return &ConnectionListResource{}
}

// ConnectionListResource implements Terraform query and bulk-import support for
// Jamf Account SSO connections.
//
// The connection collection accepts neither a filter nor a sort expression, so
// there is no filter block and no ordering to pin: the resource streams the
// connections the organization holds, in the order Jamf returns them.
//
// It reads each connection individually as well, which is the one design
// decision here worth stating. Two classes of connection cannot be managed —
// one built with Microsoft admin consent, and one Jamf's collection lists but
// cannot read on its own identifier — and *neither is distinguishable from the
// collection entry alone*: it carries no consent flag, and a connection that
// cannot be read individually looks entirely ordinary in it. Since this resource
// exists to offer connections for import, streaming one of those would offer an
// import whose result no apply could reconcile — the failure the sibling domain
// construct's shared-domain filter exists to prevent. So each entry is read, the
// two classes are dropped with a warning naming them, and the cost is one extra
// read per connection in the organization.
//
// A per-connection read that fails for any other reason is dropped the same way
// rather than failing the whole query: one connection answering a rate limit
// must not cost the operator every connection already read.
//
// req.Limit caps the results rather than the scan: it is clamped against the
// number of connections returned, which is an upper bound on the number kept, and
// the loop counts kept entries — so a limit of 5 against ten connections of which
// six are unmanageable yields all four manageable ones rather than stopping early.
type ConnectionListResource struct {
	client *account.Client
}

// Metadata sets the list resource type name.
func (r *ConnectionListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_connection"
}

// Configure wires the Jamf Account client into the list resource via the shared
// providerdata.ConfigureAccount helper.
func (r *ConnectionListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_connection")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *ConnectionListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists the SSO connections your Jamf Account organization holds, for `terraform query` and " +
			"for importing existing connections in bulk. Jamf Account exposes no search arguments for " +
			"connections, so this list resource takes no filter configuration.\n\n" +
			"Two kinds of connection are left out, each with a warning naming it: one built with Microsoft's " +
			"admin-consent flow, which cannot be written back and so cannot be managed as a " +
			"`jamfplatform_account_sso_connection`; and one your organization's list reports but which cannot be " +
			"read on its own identifier, which is a fault in Jamf Account. Importing either would leave an entry no " +
			"apply could reconcile. A connection whose individual read fails for any other reason is left " +
			"out the same way, with a warning carrying the error, so one unreadable connection does not " +
			"cost you the rest. Use the `jamfplatform_account_sso_connections` data source to see every " +
			"connection including those.\n\n" +
			"Neither kind can be told apart from the list alone, so this reads each connection individually: " +
			"expect one extra read per connection in your organization.\n\n" +
			"An imported connection cannot recover `enabled_products` or `enabled_environments`, because nothing " +
			"Jamf Account returns echoes the tenants back. Add those to the configuration by hand after importing." +
			listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams SSO connection identities back to
// Terraform.
//
// Every connection left out — whichever of the three reasons left it out — is
// reported through one trailing diagnostics-only result, a bare list.ListResult
// carrying only Diagnostics. That shape is deliberate:
// list.ListResultsStreamDiagnostics replaces the whole stream, so assigning it
// mid-loop would throw away every connection already read, and the framework
// offers no stream-level diagnostics channel to report a partial failure on. The
// trailing result must not be built with req.NewListResult, which sets a non-nil
// identity and resource and so turns the warning into a hard error. It trails the
// results rather than riding on the first one so that a connection skipped last
// is still reported. The three genuinely fatal cases below — an unconfigured
// client, a configuration read failure, and the collection read itself — all
// happen before any result exists, so replacing the stream is right for them.
func (r *ConnectionListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config ConnectionListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	summaries, err := r.client.ListConnections(ctx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Account SSO connections", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(summaries)) {
		maxResults = int64(len(summaries))
	}

	results := make([]list.ListResult, 0, maxResults)
	var skipped diag.Diagnostics

	for i := range summaries {
		if int64(len(results)) >= maxResults {
			break
		}
		summary := summaries[i]

		found, readErr := r.client.GetConnection(ctx, summary.ID)
		if readErr != nil {
			if helpers.IsNotFoundError(readErr) {
				skipped.AddWarning(
					"Jamf Account lists an SSO connection it cannot read",
					"The connection \""+summary.Name+"\" (identifier "+summary.ID+") is in your organization's "+
						"list but reading it on that identifier reports it missing, which is a disagreement "+
						"inside Jamf. It has been left out, because importing it would leave an entry Terraform "+
						"could never refresh. Raise it with Jamf Support, quoting the identifier.",
				)
				continue
			}
			skipped.AddWarning(
				"A Jamf Account SSO connection could not be read",
				"The connection \""+summary.Name+"\" (identifier "+summary.ID+") is in your organization's "+
					"list but could not be read, so it has been left out of the results. Check that the "+
					"integration holds the SSO read permission, then re-run the command — a transient refusal "+
					"such as a rate limit reports the same way. Underlying error: "+readErr.Error(),
			)
			continue
		}

		if found.ConsentFlow {
			skipped.AddWarning(
				"SSO connection using Microsoft admin consent left out",
				"The connection \""+found.Name+"\" (identifier "+found.ID+") was set up with Microsoft's "+
					"admin-consent flow, so it cannot be managed as a jamfplatform_account_sso_connection and "+
					"has been left out. "+consentFlowExplanation+" Read it with the "+
					"`jamfplatform_account_sso_connection` data source instead.",
			)
			continue
		}

		result := req.NewListResult(ctx)
		result.DisplayName = found.Name

		identity := connectionIdentityModel{ID: types.StringValue(found.ID)}
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, identity)...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := ConnectionResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(connectionTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignConnectionResourceModel(&state, found, &summary, true)...)
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

	tflog.Debug(ctx, "Listed Jamf Account SSO connections", map[string]any{
		"limit":    req.Limit,
		"scanned":  len(summaries),
		"returned": len(results),
		"skipped":  len(skipped),
	})

	if len(results) == 0 && len(skipped) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if len(skipped) > 0 {
			push(list.ListResult{Diagnostics: skipped})
		}
	}
}
