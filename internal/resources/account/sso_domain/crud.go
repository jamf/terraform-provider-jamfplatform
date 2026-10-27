// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   account.CreateDomain
//   account.ListDomains        (read, update refresh, data sources, list resource, collision check in collision.go)
//   account.DeleteDomain
//   account.GetDomainAllocation (singular data source)
//
// Deliberately absent: there is no read-by-identifier and no update. Both routes
// exist in the published spec and both answer 403 BAD_PERMISSIONS on every
// credential probed, alongside a read of the same claim through the collection
// answering 200 — so they are unmapped, not unprivileged. account.VerifyDomain is
// also absent on purpose: verification is fire-and-forget, rate-limited, and
// belongs to the jamfplatform_account_sso_domain_verify action rather than to
// this resource's lifecycle.
//
// Uniqueness: domain, organization-wide, enforced (409 CONFLICT), case-insensitive. Shared domains count.
//
// Status: current. Last reviewed 2026-10-25.

package sso_domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create claims a DNS domain for the organization.
//
// The claim response carries the whole stored representation — identifier,
// verification token, status and every timestamp — so there is no read-back. That
// is not merely an optimisation: the verification token is minted by this one
// call, and a claim can only be re-read by scanning the collection, so a
// read-back would cost a full listing to learn nothing new.
func (r *DomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State, path.Root("domain"))
	var plan DomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	domain := plan.Domain.ValueString()

	created, err := r.client.CreateDomain(createCtx, buildDomainRequest(plan))
	if err != nil {
		if !appendClaimDiagnosticsWithHint(&resp.Diagnostics, domain, err, r.collisions.CreateConflictHint(err, resourceTypeName)) {
			resp.Diagnostics.AddError("Error claiming Jamf Account SSO domain", helpers.APIErrorDetail(err))
		}
		return
	}

	assignDomainResourceModel(&plan, created)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, domainIdentityModel{Domain: plan.Domain})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "claimed Jamf Account SSO domain", map[string]any{"domain": plan.Domain.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest representation of the claim.
//
// The lookup is a scan of the organization's domain collection matched on the
// domain name, because Jamf Account exposes no read of a single claim. The
// identity branch serves the identity-only refresh Terraform performs when it
// holds an identity and no state; the ordinary branch serves both a normal
// refresh and the read that follows a `terraform import`, where the domain name
// has already been written into state. Nothing here is gated on which path it
// arrived by — the whole stored representation is adopted either way — so the
// usual trap of `req.State.Raw.IsNull()` being false after an import does not
// apply.
//
// The one thing the scan is gated on is ownership. The collection returns
// shared domains alongside the organization's own claims, and a match on the
// name alone would let `terraform import` adopt one; helpers.go says why a
// shared domain cannot be a managed resource. Refusing here covers both the
// import and the case of a domain shared in after Terraform adopted it.
func (r *DomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State, path.Root("domain"))
	var state DomainResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this Jamf Account SSO domain without existing state or identity data, so the provider cannot determine which domain to read.",
			)
			return
		}
		var identity domainIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.Domain.IsNull() || identity.Domain.IsUnknown() || identity.Domain.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing domain name",
				"The resource identity did not include a 'domain' attribute, so the provider cannot refresh the Jamf Account SSO domain.",
			)
			return
		}
		state.Domain = identity.Domain
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(domainTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.Domain.IsNull() || state.Domain.ValueString() == "" {
		resp.Diagnostics.AddError("Missing domain name", "Cannot read a Jamf Account SSO domain without a domain name.")
		return
	}

	domains, err := r.client.ListDomains(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Account SSO domains", helpers.APIErrorDetail(err))
		return
	}

	found := findDomain(domains, state.Domain.ValueString())
	if found == nil {
		tflog.Info(ctx, "Jamf Account SSO domain not found, removing from state", map[string]any{"domain": state.Domain.ValueString()})
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, domainIdentityModel{Domain: state.Domain})...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.State.RemoveResource(ctx)
		return
	}

	if appendSharedDomainDiagnostics(&resp.Diagnostics, found) {
		return
	}

	assignDomainResourceModel(&state, found)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, domainIdentityModel{Domain: state.Domain})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update refreshes state without issuing any write.
//
// `domain` is the only attribute a practitioner sets and it is RequiresReplace,
// so the only change that reaches Update is a timeouts edit, which needs no call
// of its own. A claim cannot be modified in any case — the routes that would do
// it are unmapped. The listing exists to re-hydrate the read-only attributes,
// two of which move without Terraform's involvement whenever the verification is
// run. It refuses a shared domain for the same reason Read does, so that a
// refresh Terraform skipped cannot slip one into state through this path.
func (r *DomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	domains, err := r.client.ListDomains(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Account SSO domains", helpers.APIErrorDetail(err))
		return
	}

	found := findDomain(domains, plan.Domain.ValueString())
	if found == nil {
		resp.Diagnostics.AddError(
			"Jamf Account SSO domain no longer claimed",
			"The domain \""+plan.Domain.ValueString()+"\" is no longer claimed by this organization, so Terraform "+
				"cannot refresh it. Run `terraform apply -refresh-only` to drop it from state, then apply again "+
				"to claim it afresh.",
		)
		return
	}

	if appendSharedDomainDiagnostics(&resp.Diagnostics, found) {
		return
	}

	assignDomainResourceModel(&plan, found)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, domainIdentityModel{Domain: plan.Domain})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete withdraws the claim.
//
// Withdrawing a claim also removes the domain from every SSO connection that
// names it, silently shrinking those connections' domain lists. Nothing blocks
// it and no reverse lookup exists on the claim itself, so this is a documented
// hazard rather than something the resource can guard: the assignment lookup on
// the data source is the read that shows what a destroy will affect, and it is
// deliberately not called here — an advisory call on the destroy path would add
// a failure mode to an operation that otherwise cannot fail for that reason.
//
// A shared domain is the one thing that is refused up front rather than at the
// wire: it belongs to another organization, no request can withdraw it, and the
// `shared` value already in state settles it without a call. That check keys on
// state rather than on an error code because the code Jamf answers a
// cross-organization withdrawal with was never probed — probing it would have
// meant a destructive request against another organization's domain.
func (r *DomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if state.Shared.ValueBool() {
		appendSharedDomainDeleteDiagnostics(&resp.Diagnostics, state.Domain.ValueString())
		return
	}

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Jamf Account SSO domain identifier",
			"Terraform has no identifier recorded for the domain \""+state.Domain.ValueString()+"\", and "+
				"withdrawing a claim needs one. Run `terraform apply -refresh-only` to populate it, then destroy "+
				"again.",
		)
		return
	}

	if err := r.client.DeleteDomain(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Account SSO domain already withdrawn", map[string]any{"domain": state.Domain.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error withdrawing Jamf Account SSO domain", helpers.APIErrorDetail(err))
	}
}
