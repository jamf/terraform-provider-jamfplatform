// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	account.VerifyDomain
//	account.ListDomains (resolving a domain named by name)
//
// Status: current. Last reviewed 2026-09-02.
package ssodomainaction

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var (
	_ action.Action                     = (*VerifySSODomainAction)(nil)
	_ action.ActionWithConfigure        = (*VerifySSODomainAction)(nil)
	_ action.ActionWithConfigValidators = (*VerifySSODomainAction)(nil)
)

// VerifySSODomainAction re-checks the DNS ownership record of a claimed Jamf
// Account SSO domain.
type VerifySSODomainAction struct {
	ssoDomainAction
}

// VerifySSODomainActionModel is the action's configuration.
type VerifySSODomainActionModel struct {
	Domain   types.String `tfsdk:"domain"`
	DomainID types.String `tfsdk:"domain_id"`
}

// NewVerifySSODomainAction returns a new instance of VerifySSODomainAction.
func NewVerifySSODomainAction() action.Action {
	return &VerifySSODomainAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *VerifySSODomainAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_domain_verify"
}

// Schema returns the action schema.
func (a *VerifySSODomainAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**\"Verify\"** beside a claimed domain on Jamf Account's **Single Sign-On > " +
			"Domains** page. Asks Jamf Account to look up the DNS TXT record that proves your organization owns " +
			"the domain, and reports whether ownership is now proven.\n\nClaiming a domain and releasing it are " +
			"the `jamfplatform_account_sso_domain` resource's own lifecycle; verifying is a check Jamf Account " +
			"runs on demand, so it lives here. The usual sequence is: claim the domain, publish the TXT record " +
			"its `verification_txt_record` attribute exports, then invoke this action. A domain has to be " +
			"verified before an SSO connection can use it.\n\nA verification that does not succeed fails the " +
			"run. Jamf Account reports \"checked, and ownership is still not proven\" exactly the way it reports " +
			"success, so this action reads the domain's verification status back and raises an error whenever it " +
			"is not verified. Reporting success instead would leave a plan going green on a domain that is still " +
			"pending, and the consequence would surface much later as an unexplained refusal to create the " +
			"connection that needs it.\n\nJamf Account allows one verification every five minutes per domain, " +
			"counted from the last time the domain changed, and claiming it counts as a change. So a " +
			"verification invoked in the same run that claims the domain is refused, and this action reports " +
			"that refusal rather than waiting it out. Do not trigger this action from the claiming resource's " +
			"own lifecycle; that arrangement is refused every single time. Give the DNS record time to publish, " +
			"then invoke the action on a later run: `terraform apply " +
			"-invoke='action.jamfplatform_account_sso_domain_verify.corp'`.\n\nInvoking it is never free, even " +
			"when ownership is not proven. Every verification resets that five-minute window and moves the point " +
			"the domain's verification lapses out to 14 days from now, so the domain's `last_modified_at` and " +
			"`verification_expires_at` change on every invocation. Triggering it repeatedly against a domain " +
			"whose record is not published yet does nothing but " +
			"push those two forward." + verifySSODomainPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"domain": actionschema.StringAttribute{
				MarkdownDescription: "The claimed domain to verify, as it appears on Jamf Account's **Single " +
					"Sign-On > Domains** page, such as `example.com`. Case is ignored: Jamf Account stores a " +
					"claimed domain in lower case however it was claimed.\n\nThis is the identifier a " +
					"practitioner holds. Do not trigger this action from the lifecycle of the " +
					"`jamfplatform_account_sso_domain` resource that claims the domain: claiming it starts the " +
					"five-minute window, so a check in the same run is always refused. Invoke it on a later run, " +
					"once the TXT record is live: `terraform apply " +
					"-invoke='action.jamfplatform_account_sso_domain_verify.corp'`. Set this or `domain_id`, " +
					"never both.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"domain_id": actionschema.StringAttribute{
				MarkdownDescription: "The identifier Jamf Account assigned the claimed domain, as exported by " +
					"the `id` attribute of `jamfplatform_account_sso_domain`.\n\nJamf Account never shows this " +
					"identifier, so `domain` is usually the easier form. Naming the identifier skips the lookup " +
					"that resolving a name needs, so it also needs one permission fewer; see the table below. A " +
					"domain that is released and claimed again is issued a new identifier, so avoid hard-coding " +
					"one. Set this or `domain`, never both.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

// ConfigValidators enforces exactly one identifier.
//
// A ConfigValidator rather than per-attribute ConflictsWith so that supplying
// neither also fails at plan time, instead of reaching Invoke with nothing to act
// on.
func (a *VerifySSODomainAction) ConfigValidators(_ context.Context) []action.ConfigValidator {
	return []action.ConfigValidator{
		actionvalidator.ExactlyOneOf(
			path.MatchRoot("domain"),
			path.MatchRoot("domain_id"),
		),
	}
}

// Configure wires the Jamf Account client into the action.
func (a *VerifySSODomainAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// Invoke asks Jamf Account to re-check the domain's DNS ownership record and
// reports what it found.
//
// The call returning without error is not the outcome: a domain whose TXT record is
// absent or wrong comes back 200 with the full body and its status untouched, so
// the status is classified and an unproven domain becomes an error. See the package
// doc for the three wire facts behind that.
func (a *VerifySSODomainAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data VerifySSODomainActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainID, ok := a.resolveDomainID(ctx, data, &resp.Diagnostics)
	if !ok {
		return
	}

	target := configuredTarget(data, domainID)
	idPath := path.Root("domain")
	if !data.DomainID.IsNull() {
		idPath = path.Root("domain_id")
	}

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Asking Jamf Account to check the DNS ownership record for %s", target),
	})

	domain, err := a.client.VerifyDomain(ctx, domainID)
	if err != nil {
		if isAlreadyVerified(err) {
			resp.SendProgress(action.InvokeProgressEvent{
				Message: fmt.Sprintf("Domain %s is already verified; nothing to check", target),
			})
			return
		}
		if !appendInvokeDiagnostics(&resp.Diagnostics, err, target, idPath) {
			resp.Diagnostics.AddError(
				"Domain verification could not be run",
				fmt.Sprintf("Jamf Account refused to verify domain %s: %s", target, helpers.APIErrorDetail(err)),
			)
		}
		return
	}
	if domain == nil {
		resp.Diagnostics.AddError(
			"Domain verification returned nothing",
			fmt.Sprintf("Jamf Account accepted the verification of domain %s but reported nothing about the "+
				"domain, so whether ownership is proven is unknown. Please report this issue to the provider "+
				"developers.", target),
		)
		return
	}

	reportOutcome(resp, domain, target)
}

// configuredTarget names the domain in messages sent before the verification
// answers, using whichever identifier the configuration supplied.
func configuredTarget(data VerifySSODomainActionModel, domainID string) string {
	if name := strings.TrimSpace(data.Domain.ValueString()); name != "" {
		return name
	}
	return domainID
}

// reportOutcome turns a verification response into progress or a diagnostic.
//
// Not verified is an error rather than a warning, which is the single most
// consequential decision in this action. A warning is easy to miss in a run that
// otherwise succeeds, and the cost of missing it is not cosmetic: an SSO connection
// cannot be created without a verified domain, and the refusal when one tries is an
// opaque upstream failure naming neither the domain nor the reason. Failing here
// names both.
func reportOutcome(resp *action.InvokeResponse, domain *account.Domain, target string) {
	name := strings.TrimSpace(domain.Domain)
	if name == "" {
		name = target
	}
	status := statusText(domain.DomainStatus)

	switch classify(domain.DomainStatus) {
	case outcomeVerified:
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Ownership of %s is verified (%s)%s", name, status, lapseClause(domain)),
		})
	case outcomeNotVerified:
		resp.Diagnostics.AddError(
			"Domain ownership was not verified",
			fmt.Sprintf("Jamf Account checked the DNS records for %s and ownership is still not proven. The "+
				"domain's verification status is %s.\n\n"+
				"Publish a TXT record on %s whose value is the domain's `verification_txt_record`, wait for it "+
				"to become visible to Jamf Account, then invoke this action again. It allows one verification every "+
				"five minutes per domain, so an immediate retry is refused.\n\n"+
				"This attempt has already moved the domain's `last_modified_at` and `verification_expires_at` "+
				"forward: a verification that proves nothing counts against the five-minute limit exactly as a "+
				"successful one does.", name, status, name),
		)
	default:
		resp.Diagnostics.AddError(
			"Jamf Account reported a verification status this provider does not recognise",
			fmt.Sprintf("Domain %s came back with verification status %s, which this provider cannot read as "+
				"either verified or unverified, so whether ownership is proven is unknown. Check the domain on "+
				"Jamf Account's Single Sign-On > Domains page, and please report this issue to the provider "+
				"developers so the status can be handled.", name, status),
		)
	}
}

// lapseClause adds when the verification lapses, for the run log.
//
// Worth reporting on success because the answer is always 14 days from now and
// nothing renews it automatically — but only when Jamf reported one, since the
// field is optional on the wire.
func lapseClause(domain *account.Domain) string {
	if domain.VerificationExpirationDate == nil {
		return ""
	}
	return fmt.Sprintf(", and lapses on %s unless verified again",
		domain.VerificationExpirationDate.UTC().Format(time.RFC3339))
}
