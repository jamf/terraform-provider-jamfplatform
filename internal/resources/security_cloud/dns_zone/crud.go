// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.CreateDnsZoneV1
//   securitycloud.GetDnsZoneV1
//   securitycloud.UpdateDnsZoneV1
//   securitycloud.DeleteDnsZoneV1
//   securitycloud.ListDnsZonesV1 (data sources / list resource / collision check in collision.go)
//   securitycloud.ResolveDnsZoneV1ByName (singular data source, name lookup)
//
// Uniqueness: each domains entry, tenant-wide, enforced (409 DOMAIN_CONFLICT), case-insensitive. The name is not a key for the server: duplicate names are stored (wire-probed 2026-10-25, every case, trailing-space and accent variant), so the name is checked only under include_unenforced.
//
// Status: current. Last reviewed 2026-10-25.

package dns_zone

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Security Cloud custom DNS zone.
//
// The create response carries only the new ID and a canonical URL, so the zone is
// read back to populate state with the stored representation — which matters here
// because Jamf Security Cloud re-sorts the domain list.
//
// A read-back that fails after the create has succeeded still commits state. The zone
// exists on the tenant by then, and returning without state would orphan it: a domain
// may belong to only one custom DNS zone, so the retry is refused with a domain
// conflict against the zone the operator does not know they created. What is committed
// is the plan carrying the new ID — the sort order the server applies is what the next
// refresh reconciles, and an errored apply does not run Terraform's plan-consistency
// check, so a re-sorted collection is not a fault at that point. Nothing needs nulling
// first: the ID is this schema's only Computed value.
func (r *DNSZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DNSZoneResourceModel
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

	input, inputDiags := buildZoneWriteInput(ctx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateDnsZoneV1(createCtx, input)
	if err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error creating Jamf Security Cloud DNS zone", helpers.APIErrorDetail(err))
		}
		return
	}

	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetDnsZoneV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, dnsZoneIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Error reading created Jamf Security Cloud DNS zone",
			"The zone was created with ID \""+created.ID+"\" but could not be read back, so Terraform has "+
				"recorded its ID and the configured values without confirming what was stored. The next plan will "+
				"refresh it — do not re-create it: a domain may belong to only one custom DNS zone, so a second "+
				"create would be refused as a domain conflict with this one. Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}
	resp.Diagnostics.Append(assignDNSZoneResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, dnsZoneIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud DNS zone", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest DNS zone representation.
func (r *DNSZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DNSZoneResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this DNS zone without existing state or identity data, so the provider cannot determine which zone to read.",
			)
			return
		}
		var identity dnsZoneIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing DNS zone ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the DNS zone.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(dnsZoneTimeoutAttributeTypes)
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

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Security Cloud DNS zone without ID.")
		return
	}

	got, err := r.client.GetDnsZoneV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud DNS zone not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, dnsZoneIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud DNS zone", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignDNSZoneResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, dnsZoneIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Security Cloud custom DNS zone.
//
// The update returns no body, so the zone is read back — the same reason Create
// does: the stored domain order is the server's, not the plan's.
func (r *DNSZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DNSZoneResourceModel
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

	if plan.ID.IsNull() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Security Cloud DNS zone without ID.")
		return
	}

	input, inputDiags := buildZonePatchInput(ctx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateDnsZoneV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error updating Jamf Security Cloud DNS zone", helpers.APIErrorDetail(err))
		}
		return
	}

	got, err := r.client.GetDnsZoneV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Security Cloud DNS zone", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDNSZoneResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, dnsZoneIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Security Cloud custom DNS zone.
func (r *DNSZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DNSZoneResourceModel
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

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Security Cloud DNS zone without ID.")
		return
	}

	if err := r.client.DeleteDnsZoneV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud DNS zone already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Security Cloud DNS zone", helpers.APIErrorDetail(err))
	}
}
