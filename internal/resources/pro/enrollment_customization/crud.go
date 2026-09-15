// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateEnrollmentCustomizationV2
//   pro.GetEnrollmentCustomizationV2
//   pro.UpdateEnrollmentCustomizationV2
//   pro.DeleteEnrollmentCustomizationV2
//   pro.UploadEnrollmentCustomizationImageV2
//   pro.ListEnrollmentCustomizationPanelsV1
//   pro.CreateEnrollmentCustomizationTextPanelV1
//   pro.GetEnrollmentCustomizationTextPanelV1
//   pro.UpdateEnrollmentCustomizationTextPanelV1
//   pro.CreateEnrollmentCustomizationLdapPanelV1
//   pro.GetEnrollmentCustomizationLdapPanelV1
//   pro.UpdateEnrollmentCustomizationLdapPanelV1
//   pro.CreateEnrollmentCustomizationSsoPanelV1
//   pro.GetEnrollmentCustomizationSsoPanelV1
//   pro.UpdateEnrollmentCustomizationSsoPanelV1
//   pro.DeleteEnrollmentCustomizationPanelV1
//   pro.ResolveEnrollmentCustomizationV2ByName              (data source name lookup)
//   pro.ListEnrollmentCustomizationsV2                      (list resource)
//
// Status: current. Last reviewed 2026-05-28.

package enrollment_customization

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions a new enrollment customization on Jamf Pro.
//
// Order of operations:
//  1. If `icon_source` is set, upload the bytes and capture the returned URL.
//  2. POST the parent record with the upload URL (or the user-supplied
//     `branding_settings.icon_url`, or an empty string when neither is set).
//  3. POST each pane in the supplied order. Any panel creation failure
//     triggers a best-effort parent DELETE so we do not leak a half-built
//     customization on the tenant.
//  4. Refresh state via GET + per-pane GET so server-canonical values are
//     what land in TF state (the create responses are not authoritative).
func (r *EnrollmentCustomizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EnrollmentCustomizationResourceModel
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

	uploadedURL := ""
	if !plan.IconSource.IsNull() && !plan.IconSource.IsUnknown() && plan.IconSource.ValueString() != "" {
		url, hash, err := uploadIconForPlan(createCtx, r.client, plan.IconSource.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error uploading enrollment customization icon", helpers.APIErrorDetail(err))
			return
		}
		uploadedURL = url
		plan.IconSourceHash = types.StringValue(hash)
	} else {
		plan.IconSourceHash = types.StringNull()
	}

	parentResp, err := r.client.CreateEnrollmentCustomizationV2(createCtx, buildParentInput(plan, uploadedURL))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
		return
	}
	if parentResp == nil || parentResp.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing enrollment customization ID",
			"Jamf Pro returned success on the POST but did not include an ID; cannot persist state.",
		)
		return
	}
	id := parentResp.ID
	plan.ID = types.StringValue(id)

	createDiags := createAllPanels(createCtx, r.client, id, plan)
	resp.Diagnostics.Append(createDiags...)
	if createDiags.HasError() {
		// Use a fresh context for rollback. The createCtx deadline may have
		// expired during panel creation (e.g. on timeout), which would also
		// kill the DELETE and leak the half-built parent on the tenant.
		rollbackCtx, rollbackCancel := context.WithTimeout(ctx, defaultDeleteTimeout)
		defer rollbackCancel()
		if delErr := r.client.DeleteEnrollmentCustomizationV2(rollbackCtx, id); delErr != nil {
			tflog.Warn(ctx, "rollback delete failed after panel-create error", map[string]any{"id": id, "error": delErr.Error()})
		}
		return
	}

	resp.Diagnostics.Append(refreshState(createCtx, r.client, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, EnrollmentCustomizationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro enrollment customization", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest customization
// representation.
func (r *EnrollmentCustomizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EnrollmentCustomizationResourceModel
	isImport := req.State.Raw.IsNull()
	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this enrollment customization without existing state or identity data; cannot determine which customization to read.",
			)
			return
		}
		var identity EnrollmentCustomizationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing enrollment customization ID",
				"The resource identity did not include an 'id' attribute; cannot refresh.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(enrollmentCustomizationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro enrollment customization without ID.")
		return
	}

	got, err := r.client.GetEnrollmentCustomizationV2(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro enrollment customization not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
		return
	}
	assignParentToResource(&state, got)

	if err := hydratePanels(readCtx, r.client, &state); err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro enrollment customization panels", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, EnrollmentCustomizationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles plan against state. The parent endpoint is full-replace
// per wire-probe, so we always PUT the parent. Panels are diffed by their
// server-assigned id (an unknown id in the plan = new pane to create).
//
// An unknown icon_source_hash is ModifyPlan's request to re-upload the icon,
// and the only thing that produces one: nothing writes a known hash into a
// plan, so the hash committed here is always taken from the bytes the upload
// received rather than from an earlier read of the source (issue #373).
func (r *EnrollmentCustomizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state EnrollmentCustomizationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
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

	id := plan.ID.ValueString()
	if id == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Pro enrollment customization without ID.")
		return
	}

	uploadedURL := ""
	uploadWanted := plan.IconSourceHash.IsUnknown()
	if uploadWanted && !plan.IconSource.IsNull() && !plan.IconSource.IsUnknown() && plan.IconSource.ValueString() != "" {
		url, hash, err := uploadIconForPlan(updateCtx, r.client, plan.IconSource.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error uploading enrollment customization icon", helpers.APIErrorDetail(err))
			return
		}
		uploadedURL = url
		plan.IconSourceHash = types.StringValue(hash)
	}

	if _, err := r.client.UpdateEnrollmentCustomizationV2(updateCtx, id, buildParentInput(plan, uploadedURL)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(reconcilePanels(updateCtx, r.client, id, plan, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(refreshState(updateCtx, r.client, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, EnrollmentCustomizationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the customization. Parent DELETE cascades all panels
// server-side (verified by wire-probe), so no explicit per-panel cleanup is
// required.
func (r *EnrollmentCustomizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EnrollmentCustomizationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro enrollment customization without ID.")
		return
	}

	if err := r.client.DeleteEnrollmentCustomizationV2(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro enrollment customization already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
	}
}

// uploadIconForPlan opens the user-supplied source, streams it to the SDK's
// image upload endpoint, and returns the resulting URL together with the
// content hash so the caller can stamp it on the plan.
//
// The source is hashed by streaming it once, then rewound and uploaded, so the
// hash describes exactly what was sent without the bytes being buffered. The
// rewound *os.File stays an io.Seeker, which is what lets the SDK precompute
// Content-Length and retry a 429; handing it a plain reader would forfeit both.
func uploadIconForPlan(ctx context.Context, client *pro.Client, source string) (string, string, error) {
	file, filename, cleanup, err := files.OpenUploadSource(ctx, source, files.DefaultMaxBytes)
	if err != nil {
		return "", "", err
	}
	defer cleanup()

	hash, hashErr := files.HashStreamSHA256(file)
	if hashErr != nil {
		return "", "", hashErr
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return "", "", seekErr
	}

	uploaded, err := client.UploadEnrollmentCustomizationImageV2(ctx, filename, file)
	if err != nil {
		return "", "", err
	}
	if uploaded == nil {
		return "", "", fmt.Errorf("upload returned a nil response")
	}
	return uploaded.URL, hash, nil
}

// createAllPanels creates every pane on the supplied parent in plan order.
// Errors are returned as a diag.Diagnostics so the caller can append them and
// trigger rollback when HasError() is true.
func createAllPanels(ctx context.Context, client *pro.Client, parentID string, plan EnrollmentCustomizationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, p := range plan.TextPanes {
		if _, err := client.CreateEnrollmentCustomizationTextPanelV1(ctx, parentID, buildTextPanelInput(p)); err != nil {
			diags.AddError("Error creating text pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	for _, p := range plan.LdapPanes {
		if _, err := client.CreateEnrollmentCustomizationLdapPanelV1(ctx, parentID, buildLdapPanelInput(p)); err != nil {
			diags.AddError("Error creating LDAP pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	for _, p := range plan.SsoPanes {
		if _, err := client.CreateEnrollmentCustomizationSsoPanelV1(ctx, parentID, buildSsoPanelInput(p)); err != nil {
			diags.AddError("Error creating SSO pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	return diags
}

// reconcilePanels diffs the plan against state by pane type. For each pane in
// the plan: a present `id` matched in state means "update in place"; an
// unknown or absent `id` means "create". Any pane id in state but not in the
// plan is deleted via the generic /all/{panelID} endpoint.
//
// Phase ordering matters: Jamf Pro enforces a server-side at-most-one-auth-
// pane invariant (ALREADY_HAS_AUTH), so a cross-auth swap (ldap → sso or
// sso → ldap) must delete the outgoing auth pane *before* creating the
// incoming one. To handle that uniformly, this function performs all stale
// deletions first across all three pane types, then issues creates/updates.
func reconcilePanels(ctx context.Context, client *pro.Client, parentID string, plan, state EnrollmentCustomizationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	stateText := indexByID(state.TextPanes, func(p textPaneModel) string { return p.ID.ValueString() })
	planTextIDs := planIDSet(plan.TextPanes, func(p textPaneModel) string { return p.ID.ValueString() })
	stateLdap := indexByID(state.LdapPanes, func(p ldapPaneModel) string { return p.ID.ValueString() })
	planLdapIDs := planIDSet(plan.LdapPanes, func(p ldapPaneModel) string { return p.ID.ValueString() })
	stateSso := indexByID(state.SsoPanes, func(p ssoPaneModel) string { return p.ID.ValueString() })
	planSsoIDs := planIDSet(plan.SsoPanes, func(p ssoPaneModel) string { return p.ID.ValueString() })

	// Phase 1: delete every pane in state that is no longer in the plan.
	// Auth panes (ldap/sso) must be removed before any new auth pane is
	// created; deleting text panes here too keeps the ordering uniform.
	for id := range stateText {
		if _, kept := planTextIDs[id]; kept {
			continue
		}
		if err := client.DeleteEnrollmentCustomizationPanelV1(ctx, parentID, id); err != nil && !helpers.IsNotFoundError(err) {
			diags.AddError("Error deleting text pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	for id := range stateLdap {
		if _, kept := planLdapIDs[id]; kept {
			continue
		}
		if err := client.DeleteEnrollmentCustomizationPanelV1(ctx, parentID, id); err != nil && !helpers.IsNotFoundError(err) {
			diags.AddError("Error deleting LDAP pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	for id := range stateSso {
		if _, kept := planSsoIDs[id]; kept {
			continue
		}
		if err := client.DeleteEnrollmentCustomizationPanelV1(ctx, parentID, id); err != nil && !helpers.IsNotFoundError(err) {
			diags.AddError("Error deleting SSO pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}

	// Phase 2: create new panes and update existing ones in plan order.
	for _, p := range plan.TextPanes {
		id := p.ID.ValueString()
		if id == "" || stateText[id] == nil {
			if _, err := client.CreateEnrollmentCustomizationTextPanelV1(ctx, parentID, buildTextPanelInput(p)); err != nil {
				diags.AddError("Error creating text pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
				return diags
			}
			continue
		}
		if _, err := client.UpdateEnrollmentCustomizationTextPanelV1(ctx, parentID, id, buildTextPanelInput(p)); err != nil {
			diags.AddError("Error updating text pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	for _, p := range plan.LdapPanes {
		id := p.ID.ValueString()
		if id == "" || stateLdap[id] == nil {
			if _, err := client.CreateEnrollmentCustomizationLdapPanelV1(ctx, parentID, buildLdapPanelInput(p)); err != nil {
				diags.AddError("Error creating LDAP pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
				return diags
			}
			continue
		}
		if _, err := client.UpdateEnrollmentCustomizationLdapPanelV1(ctx, parentID, id, buildLdapPanelInput(p)); err != nil {
			diags.AddError("Error updating LDAP pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	for _, p := range plan.SsoPanes {
		id := p.ID.ValueString()
		if id == "" || stateSso[id] == nil {
			if _, err := client.CreateEnrollmentCustomizationSsoPanelV1(ctx, parentID, buildSsoPanelInput(p)); err != nil {
				diags.AddError("Error creating SSO pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
				return diags
			}
			continue
		}
		if _, err := client.UpdateEnrollmentCustomizationSsoPanelV1(ctx, parentID, id, buildSsoPanelInput(p)); err != nil {
			diags.AddError("Error updating SSO pane on Jamf Pro enrollment customization", helpers.APIErrorDetail(err))
			return diags
		}
	}
	return diags
}

// planIDSet returns the set of non-empty IDs present in plan-side panes for a
// given pane type. Used by reconcilePanels to detect which state-side panes
// have been dropped from the plan and must be deleted.
func planIDSet[T any](items []T, key func(T) string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, it := range items {
		k := key(it)
		if k == "" {
			continue
		}
		out[k] = struct{}{}
	}
	return out
}

// indexByID builds a map keyed by the element's stringified panel ID. Empty
// IDs are skipped — they belong to plan-side entries that have not been
// persisted yet.
func indexByID[T any](items []T, key func(T) string) map[string]*T {
	out := make(map[string]*T, len(items))
	for i := range items {
		k := key(items[i])
		if k == "" {
			continue
		}
		out[k] = &items[i]
	}
	return out
}

// refreshState re-reads the parent + every panel and rebinds plan accordingly.
func refreshState(ctx context.Context, client *pro.Client, plan *EnrollmentCustomizationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	got, err := client.GetEnrollmentCustomizationV2(ctx, plan.ID.ValueString())
	if err != nil {
		diags.AddError("Error reading Jamf Pro enrollment customization after write", helpers.APIErrorDetail(err))
		return diags
	}
	assignParentToResource(plan, got)
	if err := hydratePanels(ctx, client, plan); err != nil {
		diags.AddError("Error reading Jamf Pro enrollment customization panels", helpers.APIErrorDetail(err))
	}
	return diags
}

// hydratePanels lists every pane on the parent, fetches the typed body for
// each, and populates the plan's three pane slices.
func hydratePanels(ctx context.Context, client *pro.Client, plan *EnrollmentCustomizationResourceModel) error {
	list, err := client.ListEnrollmentCustomizationPanelsV1(ctx, plan.ID.ValueString())
	if err != nil {
		return err
	}
	if list == nil {
		plan.TextPanes = nil
		plan.LdapPanes = nil
		plan.SsoPanes = nil
		return nil
	}
	idx := buildPanelIndex(list.Panels)

	text := make([]textPaneModel, 0, len(idx.Text))
	for _, t := range idx.Text {
		full, err := client.GetEnrollmentCustomizationTextPanelV1(ctx, plan.ID.ValueString(), strconv.Itoa(t.ID))
		if err != nil {
			return err
		}
		text = append(text, assignTextPanel(full))
	}
	sortTextByRank(text)
	if len(text) == 0 {
		plan.TextPanes = nil
	} else {
		plan.TextPanes = text
	}

	ldap := make([]ldapPaneModel, 0, len(idx.Ldap))
	for _, l := range idx.Ldap {
		full, err := client.GetEnrollmentCustomizationLdapPanelV1(ctx, plan.ID.ValueString(), strconv.Itoa(l.ID))
		if err != nil {
			return err
		}
		ldap = append(ldap, assignLdapPanel(full))
	}
	sortLdapByRank(ldap)
	if len(ldap) == 0 {
		plan.LdapPanes = nil
	} else {
		plan.LdapPanes = ldap
	}

	sso := make([]ssoPaneModel, 0, len(idx.Sso))
	for _, s := range idx.Sso {
		full, err := client.GetEnrollmentCustomizationSsoPanelV1(ctx, plan.ID.ValueString(), strconv.Itoa(s.ID))
		if err != nil {
			return err
		}
		sso = append(sso, assignSsoPanel(full))
	}
	sortSsoByRank(sso)
	if len(sso) == 0 {
		plan.SsoPanes = nil
	} else {
		plan.SsoPanes = sso
	}
	return nil
}
