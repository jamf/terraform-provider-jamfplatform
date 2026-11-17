// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetSsoFailoverV1
//   pro.GenerateSsoFailoverV1
//
// Status: current.

package sso_failover_url

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the failover URL. The Jamf Pro API exposes no Create
// endpoint — one failover record exists per tenant — so we call POST
// /generate to mint the URL and persist the response into state.
func (r *SsoFailoverURLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SsoFailoverURLResourceModel
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

	got, err := r.client.GenerateSsoFailoverV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error generating Jamf Pro SSO failover URL", helpers.APIErrorDetail(err))
		return
	}
	assignResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ssoFailoverURLIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "generated Jamf Pro SSO failover URL")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state from /v1/sso/failover.
func (r *SsoFailoverURLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SsoFailoverURLResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)
	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(ssoFailoverURLTimeoutAttributeTypes)
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

	got, err := r.client.GetSsoFailoverV1(readCtx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro SSO failover URL", helpers.APIErrorDetail(err))
		return
	}
	assignResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ssoFailoverURLIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reacts to `regeneration_trigger` changes by calling POST /generate
// to mint a fresh URL.
func (r *SsoFailoverURLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, state SsoFailoverURLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
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

	if !plan.RegenerationTrigger.Equal(state.RegenerationTrigger) {
		got, err := r.client.GenerateSsoFailoverV1(updateCtx)
		if err != nil {
			resp.Diagnostics.AddError("Error regenerating Jamf Pro SSO failover URL", helpers.APIErrorDetail(err))
			return
		}
		assignResourceModel(&plan, got)
		tflog.Trace(ctx, "regenerated Jamf Pro SSO failover URL")
	} else {
		got, err := r.client.GetSsoFailoverV1(updateCtx)
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro SSO failover URL", helpers.APIErrorDetail(err))
			return
		}
		assignResourceModel(&plan, got)
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ssoFailoverURLIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — the Jamf Pro API has no clear/disable endpoint for
// the failover URL. The URL remains live on the tenant after destroy.
func (r *SsoFailoverURLResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro SSO failover URL from Terraform state (no remote delete endpoint)")
}
