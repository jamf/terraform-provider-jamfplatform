// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetEnrollmentSettingsV4
//   pro.UpdateEnrollmentSettingsV4
//   pro.ListEnrollmentAccessGroupsV3
//   pro.CreateEnrollmentAccessGroupV3
//   pro.GetEnrollmentAccessGroupV3
//   pro.UpdateEnrollmentAccessGroupV3
//   pro.DeleteEnrollmentAccessGroupV3
//   pro.ListEnrollmentLanguagesV3
//   pro.GetEnrollmentLanguageV3
//   pro.UpdateEnrollmentLanguageV3
//   pro.DeleteEnrollmentLanguageV3
//   pro.ListEnrollmentLanguageCodesV3
//
// Status: current. Last reviewed 2026-05-29.

package user_initiated_enrollment_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the singleton. The settings object always exists on the
// tenant, so Create is a read-merge-write followed by an Access-Group reconcile
// and a read-back.
func (r *UserInitiatedEnrollmentSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, cfg UserInitiatedEnrollmentSettingsResourceModel
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

	if !r.applySettings(createCtx, &plan, cfg, nil, &resp.Diagnostics) {
		return
	}
	if !r.reconcileAccessGroups(createCtx, &plan, nil, &resp.Diagnostics) {
		return
	}
	if !r.reconcileMessagingLanguages(createCtx, &plan, &resp.Diagnostics) {
		return
	}
	if !r.refresh(createCtx, &plan, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userInitiatedEnrollmentSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Trace(ctx, "applied Jamf Pro User-Initiated Enrollment settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings + access groups.
// Read is GET-only, so it does not take the enrollment write lock.
func (r *UserInitiatedEnrollmentSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state UserInitiatedEnrollmentSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)
	if isImport {
		state.ID = helpers.InitialSingletonID()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(userInitiatedEnrollmentSettingsTimeoutAttributeTypes)
		state.AccessGroups = types.SetNull(types.ObjectType{AttrTypes: accessGroupAttrTypes})
		state.MessagingLanguages = types.MapNull(types.ObjectType{AttrTypes: messagingLanguageAttrTypes})
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

	if !r.refresh(readCtx, &state, &resp.Diagnostics) {
		return
	}

	state.ID = helpers.InitialSingletonID()
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userInitiatedEnrollmentSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles settings + cert + access-group state.
func (r *UserInitiatedEnrollmentSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, state, cfg UserInitiatedEnrollmentSettingsResourceModel
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

	if !r.applySettings(updateCtx, &plan, cfg, &state, &resp.Diagnostics) {
		return
	}
	if !r.reconcileAccessGroups(updateCtx, &plan, &state, &resp.Diagnostics) {
		return
	}
	if !r.reconcileMessagingLanguages(updateCtx, &plan, &resp.Diagnostics) {
		return
	}
	if !r.refresh(updateCtx, &plan, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userInitiatedEnrollmentSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — `terraform destroy` removes the resource from
// Terraform state and leaves the settings, certificates and Access Groups on
// the tenant intact.
//
// The User-Initiated Enrollment settings object is a tenant-wide singleton that
// always exists and cannot be deleted; Access Groups are left in place to avoid
// disrupting enrollment on shared tenants.
func (r *UserInitiatedEnrollmentSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro User-Initiated Enrollment settings from Terraform state (no remote delete; settings retained on tenant)")
}

// applySettings performs the /v4 read-merge-write critical section under the
// shared enrollment write lock. state is the prior state (nil on Create) used
// to drive cert change-detection.
func (r *UserInitiatedEnrollmentSettingsResource) applySettings(
	ctx context.Context,
	plan *UserInitiatedEnrollmentSettingsResourceModel,
	cfg UserInitiatedEnrollmentSettingsResourceModel,
	state *UserInitiatedEnrollmentSettingsResourceModel,
	diags *diag.Diagnostics,
) bool {
	// Critical section: serialize the entire GET → merge → PUT against
	// concurrent enrollment-settings writes (the /v4 PUT is full-replace and
	// round-trips fields owned by the Re-enrollment resource).
	if r.enrollmentMu != nil {
		r.enrollmentMu.Lock()
		defer r.enrollmentMu.Unlock()
	}

	got, err := r.client.GetEnrollmentSettingsV4(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro User-Initiated Enrollment settings", helpers.APIErrorDetail(err))
		return false
	}
	if got == nil {
		got = &pro.EnrollmentSettingsV4{}
	}

	body := mergeEnrollmentSettingsInput(*got, *plan)

	// Inject cert identity objects only when the user is uploading/changing a
	// certificate this apply. Omitting them (nil) preserves the existing cert.
	if !applyCertToBody(plan.MdmSigningCertificate, certCfgView(plan.MdmSigningCertificate, cfg.MdmSigningCertificate), priorCert(state, true), &body.MDMSigningCertificate, diags) {
		return false
	}
	if !applyCertToBody(plan.DeveloperCertificate, certCfgView(plan.DeveloperCertificate, cfg.DeveloperCertificate), priorCert(state, false), &body.DeveloperCertificateIdentity, diags) {
		return false
	}

	if _, err := r.client.UpdateEnrollmentSettingsV4(ctx, &body); err != nil {
		diags.AddError("Error updating Jamf Pro User-Initiated Enrollment settings", helpers.APIErrorDetail(err))
		return false
	}
	return true
}

// applyCertToBody decides whether to populate *dst with a freshly-built cert
// identity object. It returns true on success (including the no-upload case
// where *dst stays nil so the server preserves the existing cert).
//
// An upload fires when the plan declares the cert block AND either there is no
// prior state cert (initial upload) or an upload input changed (filename or the
// _wo_version rotation companion). The WriteOnly keystore + password are read
// from cfg (Config), not plan.
func applyCertToBody(
	planCert *certificateModel,
	cfgCert *certificateModel,
	prior *certificateModel,
	dst **pro.CertificateIdentityV2,
	diags *diag.Diagnostics,
) bool {
	if planCert == nil {
		return true
	}
	if !certUploadChanged(planCert, prior, cfgCert) {
		return true
	}
	if cfgCert == nil || cfgCert.KeystoreFile.IsNull() || cfgCert.KeystoreFile.IsUnknown() || cfgCert.KeystoreFile.ValueString() == "" {
		// Block declared but no keystore bytes available — nothing to upload.
		// Leave the cert object omitted so the server preserves any existing
		// certificate.
		return true
	}
	identity, err := buildCertificateIdentity(cfgCert)
	if err != nil {
		diags.AddError("Invalid keystore_file base64", "The supplied keystore_file is not valid RFC 4648 base64: "+helpers.APIErrorDetail(err))
		return false
	}
	*dst = identity
	return true
}

// certUploadChanged reports whether the cert upload inputs differ from prior
// state. WriteOnly keystore bytes never land in state, so change-detection is
// driven off the keystore_file_name, the keystore_password_wo_version rotation
// companion, and the initial-create case (no prior state but config has bytes).
func certUploadChanged(plan, prior, cfg *certificateModel) bool {
	if plan == nil {
		return false
	}
	if prior == nil {
		// No prior cert in state: upload if config supplies a keystore.
		return cfg != nil && !cfg.KeystoreFile.IsNull() && !cfg.KeystoreFile.IsUnknown() && cfg.KeystoreFile.ValueString() != ""
	}
	if !plan.KeystoreFileName.Equal(prior.KeystoreFileName) {
		return true
	}
	if !plan.KeystorePasswordWoVersion.Equal(prior.KeystorePasswordWoVersion) {
		return true
	}
	// Rotation companion present but prior had none, with config bytes → upload.
	if prior.KeystorePasswordWoVersion.IsNull() && cfg != nil && !cfg.KeystoreFile.IsNull() && !cfg.KeystoreFile.IsUnknown() && cfg.KeystoreFile.ValueString() != "" {
		return true
	}
	return false
}

// certCfgView returns the Config view of a cert block (WriteOnly inputs survive
// only in Config). Falls back to the plan view when Config lacks the block.
func certCfgView(planCert, cfgCert *certificateModel) *certificateModel {
	if cfgCert != nil {
		return cfgCert
	}
	return planCert
}

// priorCert returns the prior-state cert sub-model. mdm selects the MDM signing
// cert; otherwise the developer cert. Returns nil when state is nil.
func priorCert(state *UserInitiatedEnrollmentSettingsResourceModel, mdm bool) *certificateModel {
	if state == nil {
		return nil
	}
	if mdm {
		return state.MdmSigningCertificate
	}
	return state.DeveloperCertificate
}

// refresh reads /v4 settings plus both /v3 nested collections (access groups and
// messaging languages) and folds them into state.
//
// Each nested collection's readback respects declared cardinality. When the
// incoming model carries a KNOWN set (the user manages the collection), state
// reflects ONLY the managed subset — the declared elements matched to the fresh
// server list — so the applied set cardinality equals the planned set and
// Terraform Core's plan-vs-apply consistency check holds. Undeclared elements
// (the always-present built-in access group, the English language) are left on
// the tenant but never echoed into state. When the set is null/unknown (the user
// did not author the collection) the full server list is reflected as a Computed
// value.
func (r *UserInitiatedEnrollmentSettingsResource) refresh(ctx context.Context, state *UserInitiatedEnrollmentSettingsResourceModel, diags *diag.Diagnostics) bool {
	got, err := r.client.GetEnrollmentSettingsV4(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro User-Initiated Enrollment settings", helpers.APIErrorDetail(err))
		return false
	}
	assignSettingsResourceModel(state, got)

	// Both nested /v3 collections are always refreshed, independently of whether
	// the other is managed — each folds its own Computed set into state.
	if !r.refreshAccessGroups(ctx, state, diags) {
		return false
	}
	return r.refreshMessagingLanguages(ctx, state, diags)
}

// refreshAccessGroups reads the /v3 access-group list and folds it into state,
// honouring declared cardinality. A managed (known) collection reflects only the
// declared subset matched to the fresh list; a null/unknown collection reflects
// the full server list as a Computed value.
func (r *UserInitiatedEnrollmentSettingsResource) refreshAccessGroups(ctx context.Context, state *UserInitiatedEnrollmentSettingsResourceModel, diags *diag.Diagnostics) bool {
	groups, err := r.client.ListEnrollmentAccessGroupsV3(ctx, nil, true)
	if err != nil {
		diags.AddError("Error reading Jamf Pro enrollment Access Groups", helpers.APIErrorDetail(err))
		return false
	}

	declared := state.AccessGroups
	if declared.IsNull() || declared.IsUnknown() {
		// Unmanaged: reflect the full server list as a Computed value.
		set, d := assignAccessGroupsState(ctx, groups)
		diags.Append(d...)
		if diags.HasError() {
			return false
		}
		state.AccessGroups = set
		return true
	}

	// Managed: project only the declared subset, matched to the fresh list.
	var declaredModels []accessGroupModel
	diags.Append(declared.ElementsAs(ctx, &declaredModels, false)...)
	if diags.HasError() {
		return false
	}
	managed := projectManagedAccessGroups(declaredModels, groups)
	set, d := assignAccessGroupsState(ctx, managed)
	diags.Append(d...)
	if diags.HasError() {
		return false
	}
	state.AccessGroups = set
	return true
}

// refreshMessagingLanguages reads the /v3 language list and folds it into state
// as a map keyed by language code, honouring declared cardinality exactly like
// the Access-Group readback: a managed (known) collection reflects only the
// declared subset matched to the fresh list, so the applied map keys equal the
// planned keys; a null/unknown collection reflects the full server list as a
// Computed value.
func (r *UserInitiatedEnrollmentSettingsResource) refreshMessagingLanguages(ctx context.Context, state *UserInitiatedEnrollmentSettingsResourceModel, diags *diag.Diagnostics) bool {
	langs, err := r.client.ListEnrollmentLanguagesV3(ctx, nil)
	if err != nil {
		diags.AddError("Error reading Jamf Pro enrollment languages", helpers.APIErrorDetail(err))
		return false
	}

	declared := state.MessagingLanguages
	if declared.IsNull() || declared.IsUnknown() {
		m, d := messagingLanguagesToMap(ctx, langs)
		diags.Append(d...)
		if diags.HasError() {
			return false
		}
		state.MessagingLanguages = m
		return true
	}

	var declaredModels map[string]messagingLanguageModel
	diags.Append(declared.ElementsAs(ctx, &declaredModels, false)...)
	if diags.HasError() {
		return false
	}
	managed := projectManagedMessagingLanguages(declaredModels, langs)
	m, d := messagingLanguagesToMap(ctx, managed)
	diags.Append(d...)
	if diags.HasError() {
		return false
	}
	state.MessagingLanguages = m
	return true
}

// projectManagedAccessGroups returns the subset of the fresh server list (in
// declared order) corresponding to the declared elements, matched by server id
// when present, else by the directory_service_group_id + ldap_server_id natural
// key. A declared element with no server match (should not happen after a
// successful reconcile) is dropped. The result has the same cardinality as the
// declared set, so the applied set matches the plan.
func projectManagedAccessGroups(declared []accessGroupModel, current []pro.EnrollmentAccessGroupPreview) []pro.EnrollmentAccessGroupPreview {
	byID := make(map[string]pro.EnrollmentAccessGroupPreview, len(current))
	byKey := make(map[string]pro.EnrollmentAccessGroupPreview, len(current))
	for _, c := range current {
		if id := pointerString(c.ID); id != "" {
			byID[id] = c
		}
		byKey[naturalKey(c.Name, c.LdapServerID)] = c
	}

	out := make([]pro.EnrollmentAccessGroupPreview, 0, len(declared))
	for _, d := range declared {
		if id := stringOrEmpty(d.ID); id != "" {
			if c, ok := byID[id]; ok {
				out = append(out, c)
				continue
			}
		}
		if c, ok := byKey[naturalKey(d.Name.ValueString(), d.LdapServerID.ValueString())]; ok {
			out = append(out, c)
		}
	}
	return out
}
