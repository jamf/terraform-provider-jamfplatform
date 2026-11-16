// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetSmtpServerV2
//   pro.UpdateSmtpServerV2
//
// Every sender-settings validation Jamf Pro applies is gated on `enabled`
// (probed 2026-09-05, Jamf Pro 11.31, EU gateway). A disabled connection accepts
// an empty senderSettings.emailAddress and displayName and round-trips both,
// which is how a tenant that has never set up mail reads back. An enabled one
// refuses each independently: 400 [INVALID_EMAIL] for an address that is not in
// address format, 400 [INVALID_DISPLAY_NAME] for a display name that is empty or
// absent. validateSenderSettingsWhenEnabled catches the empty cases at plan
// time; smtpServerWriteErrorDiagnostic translates whatever reaches the wire.
//
// connectionSettings.host obeys the identical rule, probed in the same session.
// It reads back empty from an unconfigured tenant — the same read returned
// authenticationType NONE, enabled false, senderSettings.emailAddress "" and
// connectionSettings.host "" together — a disabled write carrying an empty host
// round-trips, and an enabled one is refused
// 400 [FIELD_REQUIRED_FOR_SMTP] connectionSettings.host, whose message names the
// condition as "not blank or empty when authentication is set to None or Basic
// Credentials". So the host carries no minimum-length validator either, and
// validateSenderSettingsWhenEnabled covers it alongside the sender fields.
//
// Status: current.

package smtp_server

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the SMTP Server settings singleton. The
// Jamf Pro API has no Create endpoint — one record per tenant already exists — so
// this reads the live settings (merge base for adopting undeclared fields like
// `enabled`), funnels into Update against the plan, then reads back authoritative
// state.
func (r *SmtpServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, cfg SmtpServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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

	current, err := r.client.GetSmtpServerV2(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro SMTP Server settings", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateSmtpServerV2(createCtx, buildSmtpServerInput(plan, current, createSecret(cfg))); err != nil {
		resp.Diagnostics.AddError(smtpServerWriteErrorDiagnostic("Error setting Jamf Pro SMTP Server settings", err))
		return
	}

	got, err := r.client.GetSmtpServerV2(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro SMTP Server settings after write", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignSmtpServerResourceModel(&plan, got, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smtpServerIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro SMTP Server settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *SmtpServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SmtpServerResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(smtpServerTimeoutAttributeTypes)
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

	got, err := r.client.GetSmtpServerV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro SMTP Server settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignSmtpServerResourceModel(&state, got, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smtpServerIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Full-replace PUT; the
// WriteOnly secret for the active mode is sent only when its rotation trigger
// changed.
func (r *SmtpServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, state, cfg SmtpServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
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

	if _, err := r.client.UpdateSmtpServerV2(updateCtx, buildSmtpServerInput(plan, nil, updateSecret(cfg, plan, state))); err != nil {
		resp.Diagnostics.AddError(smtpServerWriteErrorDiagnostic("Error updating Jamf Pro SMTP Server settings", err))
		return
	}

	got, err := r.client.GetSmtpServerV2(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro SMTP Server settings after update", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignSmtpServerResourceModel(&plan, got, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smtpServerIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted —
// the record persists on the tenant. Terraform removes the resource from state
// on its own after this handler returns.
func (r *SmtpServerResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro SMTP Server settings from Terraform state (singleton — no remote delete)")
}

// smtpServerWriteErrorDiagnostic names the two sender-identity refusals Jamf Pro
// raises on an enabled connection, so the operator is told which field to fill in
// rather than being handed a bare 400.
//
// It is the backstop behind validateSenderSettingsWhenEnabled, which cannot fire
// on a first apply that leaves display_name out: the attribute is
// Optional+Computed with no prior state to resolve from, so the plan value is
// Unknown and the write carries whatever the tenant already stored — empty, on a
// tenant that has never set up mail.
//
// The match is on the field names Jamf Pro attributes the failure to rather than
// its error codes: the codes are undocumented and the SDK generates no constants
// for them. So each branch has to cover every refusal that names its field — the
// host is refused both blank ([FIELD_REQUIRED_FOR_SMTP], probed) and malformed
// ([INVALID_HOSTNAME]), and the sender address likewise, which is why neither of
// those two messages claims the value was empty.
func smtpServerWriteErrorDiagnostic(summary string, err error) (string, string) {
	msg := err.Error()

	displayName := strings.Contains(msg, "senderSettings.displayName")
	email := strings.Contains(msg, "senderSettings.emailAddress")

	if strings.Contains(msg, "connectionSettings.host") {
		return "SMTP server address rejected by Jamf Pro",
			"Jamf Pro refused the write because an enabled connection needs a usable SMTP server address " +
				"whenever authentication_type is NONE or BASIC — it refuses an empty one with " +
				"[FIELD_REQUIRED_FOR_SMTP] and a malformed one with [INVALID_HOSTNAME]. Set " +
				"connection_settings.host to the server Jamf Pro should relay through, or leave enabled " +
				"false.\n\nJamf Pro reported: " + msg
	}

	switch {
	case displayName && email:
		return "Sender email address and display name required to enable the SMTP server",
			"Jamf Pro refused the write because an enabled connection needs both a sender email address and a " +
				"sender display name, and neither is set. Set sender_settings.email_address and " +
				"sender_settings.display_name, or leave enabled false.\n\nJamf Pro reported: " + msg
	case displayName:
		return "Sender display name required to enable the SMTP server",
			"Jamf Pro refused the write because an enabled connection needs a non-empty sender display name, and " +
				"the tenant's stored name is empty. Set sender_settings.display_name rather than omitting it, " +
				"since omitting it preserves the empty value.\n\nJamf Pro reported: " + msg
	case email:
		return "Sender email address rejected by Jamf Pro",
			"Jamf Pro refused the write because an enabled connection needs a sender email address in a valid " +
				"address format. Set sender_settings.email_address to the address Jamf Pro should send from, or " +
				"leave enabled false.\n\nJamf Pro reported: " + msg
	}

	return summary, msg
}
