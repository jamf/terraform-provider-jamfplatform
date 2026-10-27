// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateWebhookByID   (POST   /webhooks/id/0)
//   proclassic.GetWebhookByID      (GET    /webhooks/id/{id})
//   proclassic.UpdateWebhookByID   (PUT    /webhooks/id/{id})
//   proclassic.DeleteWebhookByID   (DELETE /webhooks/id/{id})
//   proclassic.ListWebhooks        (data source / list resource / collision check in collision.go)
//   proclassic.GetWebhookByName    (data source name lookup)
//
// Uniqueness: name, enforced (409), case-, leading-space-, trailing-space- and accent-insensitive.
//
// Status: current. Last reviewed 2026-09-06.
//
// Server invariants (wire-probed — WEBHOOK_SPIKE.md §5):
//   - Create POSTs to id="0"; the server allocates the integer ID and returns
//     it at the top level (<webhook><id>).
//   - PUT returns 201 with an empty body — Update must GET to refresh state.
//   - DELETE returns 200; a repeat DELETE / GET of a removed record returns 404,
//     so Read and Delete self-heal via helpers.IsNotFoundError.
//   - Server defaults (minimal create): enabled=true, content_type=text/xml,
//     connection_timeout=5, read_timeout=2, authentication_type=NONE,
//     hash_algorithm=SHA256.
//   - Auth fields are auto-cleared by the server when they do not match
//     authentication_type (silent, not 409); the plan-time ConfigValidators
//     prevent the resulting drift. The field the active type requires is
//     refused when empty — <header> under HEADER → 409 INVALID_REQUIRED,
//     <username> under BASIC → 409 "Username is required" (wire-probed
//     2026-09-06) — so buildWebhookInput sends it as configured there and
//     emits it empty under every other type, where the merge would otherwise
//     retain a value the config dropped (#384).
//   - smart_group_id is valid only for the three SmartGroup* events (else 409),
//     and a bogus group id 409s — so no preflight is needed. A smart event with
//     no group stores/returns the -1 sentinel; non-smart events omit the
//     element. buildWebhookInput emits -1 for a smart event with no configured
//     group so Update can clear a previously-set group under Classic's merge.
//   - display_fields is not writable (409 "Problem with display_fields"); only
//     enable_display_fields_for_group_object is. display_fields is Computed-only.
//
// Update semantics: like all classic endpoints the PUT is a partial-merge. The
// provider always sends the full plan payload, so in-place edits converge.

package webhook

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro webhook. Classic POSTs to id="0"; the server
// allocates the real integer ID and returns it.
func (r *WebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	created, err := r.client.CreateWebhookByID(createCtx, "0", buildWebhookInput(plan, helpers.OptionalStringPointer(cfg.Password)))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro webhook", helpers.APIErrorDetail(err))
		return
	}
	id := extractWebhookID(created)
	if id == "" {
		resp.Diagnostics.AddError(
			"Create response missing record ID",
			"Jamf Pro returned 201 Created with no webhook ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetWebhookByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro webhook", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignWebhookResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, webhookIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro webhook", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest record representation.
func (r *WebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WebhookResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this webhook without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity webhookIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing record ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the webhook.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(webhookTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro webhook without ID.")
		return
	}

	got, err := r.client.GetWebhookByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro webhook not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, webhookIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro webhook", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignWebhookResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, webhookIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro webhook. Classic UpdateWebhookByID returns 201 with
// an empty body — we must GET to refresh state.
func (r *WebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan WebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state WebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	// Only include the plaintext <password> on the wire when the user bumped
	// `password_wo_version`. Otherwise omit so the server retains the stored
	// secret under Classic's partial-merge semantics.
	var password *string
	if !plan.PasswordWoVersion.Equal(state.PasswordWoVersion) {
		password = helpers.OptionalStringPointer(cfg.Password)
	}

	if err := r.client.UpdateWebhookByID(updateCtx, plan.ID.ValueString(), buildWebhookInput(plan, password)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro webhook", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetWebhookByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro webhook", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignWebhookResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, webhookIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro webhook.
func (r *WebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WebhookResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro webhook without ID.")
		return
	}

	if err := r.client.DeleteWebhookByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro webhook already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro webhook", helpers.APIErrorDetail(err))
	}
}
