// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetSsoSettingsV3
//   pro.UpdateSsoSettingsV3
//   pro.GetSsoCertificateV2
//   pro.UpdateSsoCertificateV2
//   pro.GenerateSsoCertificateV2
//   pro.DeleteSsoCertificateV2
//
// Status: current. Last reviewed 2026-05-26.

package sso_settings

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning. Settings PUT first (no /v3/sso Create
// endpoint), then the embedded signing certificate sub-block. Read-back
// captures authoritative state.
//
// The PUT is preceded by a read, because an SSO configuration always pre-exists
// and "create" is really adoption: the live settings are the merge base every
// setting the configuration leaves out takes its value from, so the first apply
// keeps what it was not asked to change (#405). Failing to read it is fatal —
// applying without the base is exactly the clobber this closes.
func (r *SsoSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SsoSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cfg SsoSettingsResourceModel
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

	current, err := r.client.GetSsoSettingsV3(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading the tenant's current Jamf Pro SSO settings", helpers.APIErrorDetail(err))
		return
	}

	if !applySettings(createCtx, r.client, plan, current, &resp.Diagnostics) {
		return
	}

	if !applyCertificateOnCreate(createCtx, r.client, plan, cfg, &resp.Diagnostics) {
		return
	}

	if !refreshSettingsAndCert(createCtx, r.client, &plan, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ssoSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro SSO settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings + cert from the API.
func (r *SsoSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SsoSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = helpers.InitialSingletonID()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(ssoSettingsTimeoutAttributeTypes)
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

	if !refreshSettingsAndCert(readCtx, r.client, &state, &resp.Diagnostics) {
		return
	}

	state.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ssoSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles settings + cert state.
//
// Like Create it reads the settings before writing them. UseStateForUnknown
// already carries an omitted Optional+Computed attribute's prior value into the
// plan, but the resource also carries Optional-only fields — inside
// enrollment_sso_config, and the SAML metadata file — which plan as null when
// the configuration omits them, and null resets. The read is what keeps those
// from being cleared by an apply that never mentioned them (#405).
func (r *SsoSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, state, cfg SsoSettingsResourceModel
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

	current, err := r.client.GetSsoSettingsV3(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading the tenant's current Jamf Pro SSO settings", helpers.APIErrorDetail(err))
		return
	}

	if !applySettings(updateCtx, r.client, plan, current, &resp.Diagnostics) {
		return
	}

	if !applyCertificateOnUpdate(updateCtx, r.client, plan, state, cfg, &resp.Diagnostics) {
		return
	}

	if !refreshSettingsAndCert(updateCtx, r.client, &plan, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ssoSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — `terraform destroy` removes the resource from
// Terraform state and leaves the SSO configuration on the tenant intact.
//
// Disabling SSO would break admin Jamf ID login on tenants where the
// Platform API requires SSO-enabled state (cross-product calls, Account-
// Driven Enrollment, etc.), so the provider does not flip `sso_enabled`
// to `false` on destroy. Users that genuinely want to disable SSO should
// set `sso_enabled = false` explicitly and run apply before destroy.
func (r *SsoSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro SSO settings from Terraform state (no remote delete; SSO configuration retained on tenant)")
}

// applySettings PUTs the /v3/sso payload built from plan, merged over current.
// Returns false if a diagnostic was emitted.
//
// current is the tenant's live settings, read by the caller before this runs.
// There is no separate create endpoint, so both paths land on the same PUT and
// both need the same merge base — see buildSsoSettingsInput.
//
// The SDK's SamlSettings type emits JSON `null` for nil pointer fields
// (omitempty was removed for SSO body types so a field can be reset rather than
// only omitted). Nulls are not a way to leave a field alone: an absent key and
// an explicit null both reset the field to its default, wire-probed 2026-09-08.
// What they are for is the FILE/URL mutex, where the cached bytes of the mode
// the tenant is leaving have to be actively cleared or they clash with the body.
func applySettings(ctx context.Context, client *pro.Client, plan SsoSettingsResourceModel, current *pro.SsoSettingsV3, diags *diag.Diagnostics) bool {
	body, d := buildSsoSettingsInput(ctx, plan, current)
	diags.Append(d...)
	if diags.HasError() {
		return false
	}
	if _, err := client.UpdateSsoSettingsV3(ctx, body); err != nil {
		diags.AddError("Error updating Jamf Pro SSO settings", helpers.APIErrorDetail(err))
		return false
	}
	return true
}

// applyCertificateOnCreate handles the cert sub-block during Create:
// transition (absent) → planned. Read the current cert state first because
// Jamf Pro singletons may carry an existing certificate that wasn't part of
// Terraform state.
func applyCertificateOnCreate(ctx context.Context, client *pro.Client, plan, cfg SsoSettingsResourceModel, diags *diag.Diagnostics) bool {
	if plan.SigningCertificate == nil {
		return true
	}
	prior := currentCertSetupType(ctx, client, diags)
	if diags.HasError() {
		return false
	}
	return reconcileCertificate(ctx, client, prior, plan.SigningCertificate, planCertConfigView(plan, cfg), nil, diags)
}

// applyCertificateOnUpdate reconciles the cert sub-block from prior state
// to plan. Handles block-removal (state set → plan nil → DELETE) and
// password rotation via the _wo_version companions.
//
// When the user has never authored a signing_certificate block (state and
// plan both nil), do nothing — Terraform must not delete a tenant cert it
// did not create. The block-removal DELETE only fires when state previously
// held a cert and the user actively removed the block from plan.
func applyCertificateOnUpdate(ctx context.Context, client *pro.Client, plan, state, cfg SsoSettingsResourceModel, diags *diag.Diagnostics) bool {
	if plan.SigningCertificate == nil && state.SigningCertificate == nil {
		return true
	}
	priorPlan := state.SigningCertificate
	priorSetupType := ""
	if priorPlan != nil && !priorPlan.SetupType.IsNull() && !priorPlan.SetupType.IsUnknown() {
		priorSetupType = priorPlan.SetupType.ValueString()
	}
	if priorSetupType == "" {
		priorSetupType = currentCertSetupType(ctx, client, diags)
		if diags.HasError() {
			return false
		}
	}
	return reconcileCertificate(ctx, client, priorSetupType, plan.SigningCertificate, planCertConfigView(plan, cfg), priorPlan, diags)
}

// reconcileCertificate is the single source of truth for the cert
// transition table. The matrix:
//
//	prior → plan        action
//	NONE  → nil         no-op (already absent)
//	NONE  → NONE        no-op
//	NONE  → GENERATED   POST
//	NONE  → UPLOADED    PUT
//	GEN   → nil         DELETE
//	GEN   → NONE        DELETE
//	GEN   → GEN         no-op (re-applying GENERATED is silent — avoids new serial each apply)
//	GEN   → UPLOADED    PUT (in-place replace)
//	UPLD  → nil         DELETE
//	UPLD  → NONE        DELETE
//	UPLD  → GENERATED   POST (in-place replace)
//	UPLD  → UPLOADED    PUT only if file/key/type/password rotation changed
func reconcileCertificate(ctx context.Context, client *pro.Client, prior string, plan *signingCertificateModel, certCfg *signingCertificateModel, priorState *signingCertificateModel, diags *diag.Diagnostics) bool {
	if prior == "" {
		prior = setupTypeNone
	}

	planned := setupTypeNone
	if plan != nil && !plan.SetupType.IsNull() && !plan.SetupType.IsUnknown() {
		planned = plan.SetupType.ValueString()
	}
	if plan == nil {
		planned = setupTypeNone
	}

	switch planned {
	case setupTypeNone:
		if prior == setupTypeNone {
			return true
		}
		return deleteCertificate(ctx, client, diags)
	case setupTypeGenerated:
		if prior == setupTypeGenerated {
			return true
		}
		return generateCertificate(ctx, client, diags)
	case setupTypeUploaded:
		if prior == setupTypeUploaded && !uploadInputsChanged(plan, priorState, certCfg) {
			return true
		}
		return uploadCertificate(ctx, client, plan, certCfg, diags)
	}
	return true
}

// generateCertificate calls POST /v2/sso/cert.
func generateCertificate(ctx context.Context, client *pro.Client, diags *diag.Diagnostics) bool {
	if _, err := client.GenerateSsoCertificateV2(ctx); err != nil {
		diags.AddError("Error generating Jamf Pro SSO signing certificate", helpers.APIErrorDetail(err))
		return false
	}
	tflog.Trace(ctx, "generated Jamf Pro SSO signing certificate")
	return true
}

// uploadCertificate calls PUT /v2/sso/cert with the plan-derived keystore.
// The password fields are read from the configuration (cfg) — WriteOnly
// attributes are excluded from plan; they are only available via Config.
func uploadCertificate(ctx context.Context, client *pro.Client, plan *signingCertificateModel, certCfg *signingCertificateModel, diags *diag.Diagnostics) bool {
	// Build from a merged view: plan provides non-WriteOnly values; cfg
	// provides the WriteOnly passwords.
	merged := *plan
	if certCfg != nil {
		merged.KeystorePassword = certCfg.KeystorePassword
		merged.Password = certCfg.Password
	}
	body, d := buildSsoCertificateInput(merged)
	diags.Append(d...)
	if diags.HasError() {
		return false
	}
	if _, err := client.UpdateSsoCertificateV2(ctx, body); err != nil {
		diags.AddError("Error uploading Jamf Pro SSO signing certificate", helpers.APIErrorDetail(err))
		return false
	}
	tflog.Trace(ctx, "uploaded Jamf Pro SSO signing certificate")
	return true
}

// deleteCertificate calls DELETE /v2/sso/cert.
func deleteCertificate(ctx context.Context, client *pro.Client, diags *diag.Diagnostics) bool {
	if err := client.DeleteSsoCertificateV2(ctx); err != nil {
		if helpers.IsNotFoundError(err) {
			return true
		}
		diags.AddError("Error deleting Jamf Pro SSO signing certificate", helpers.APIErrorDetail(err))
		return false
	}
	tflog.Trace(ctx, "deleted Jamf Pro SSO signing certificate")
	return true
}

// uploadInputsChanged reports whether any UPLOADED-mode input differs
// between plan and prior state. Returns true when any of:
//   - keystore_file bytes (compared by SHA-256 hash)
//   - keystore_file_name, key, or type changed
//   - either _wo_version companion changed (rotation trigger)
func uploadInputsChanged(plan, priorState, certCfg *signingCertificateModel) bool {
	if plan == nil || priorState == nil {
		return true
	}
	if !plan.Key.Equal(priorState.Key) {
		return true
	}
	if !plan.Type.Equal(priorState.Type) {
		return true
	}
	if !plan.KeystoreFileName.Equal(priorState.KeystoreFileName) {
		return true
	}
	if !plan.KeystorePasswordWoVersion.Equal(priorState.KeystorePasswordWoVersion) {
		return true
	}
	if !plan.PasswordWoVersion.Equal(priorState.PasswordWoVersion) {
		return true
	}
	if keystoreHash(plan.KeystoreFile) != keystoreHash(priorState.KeystoreFile) {
		return true
	}
	// If config has a non-null password and the version companion is
	// absent (initial Create case), force the upload too.
	if certCfg != nil {
		if priorState.KeystorePasswordWoVersion.IsNull() && !certCfg.KeystorePassword.IsNull() {
			return true
		}
		if priorState.PasswordWoVersion.IsNull() && !certCfg.Password.IsNull() {
			return true
		}
	}
	return false
}

// keystoreHash returns the SHA-256 of the decoded keystore bytes, hex
// encoded. Used for change detection without persisting the raw bytes.
func keystoreHash(s types.String) string {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s.ValueString()))
	if err != nil {
		// Treat malformed base64 as a marker that triggers a PUT (the
		// PUT will surface the proper error).
		return "malformed:" + s.ValueString()
	}
	sum := sha256.Sum256(decoded)
	return hex.EncodeToString(sum[:])
}

// currentCertSetupType reads the cert endpoint and returns the server's
// current setup type, or "" if unknown.
func currentCertSetupType(ctx context.Context, client *pro.Client, diags *diag.Diagnostics) string {
	got, err := client.GetSsoCertificateV2(ctx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			return setupTypeNone
		}
		diags.AddError("Error reading Jamf Pro SSO signing certificate", helpers.APIErrorDetail(err))
		return ""
	}
	if got == nil || got.Keystore == nil || got.Keystore.KeystoreSetupType == "" {
		return setupTypeNone
	}
	return got.Keystore.KeystoreSetupType
}

// refreshSettingsAndCert reads /v3/sso then /v2/sso/cert and folds both into
// state.
func refreshSettingsAndCert(ctx context.Context, client *pro.Client, state *SsoSettingsResourceModel, diags *diag.Diagnostics) bool {
	got, err := client.GetSsoSettingsV3(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro SSO settings", helpers.APIErrorDetail(err))
		return false
	}
	diags.Append(assignSsoSettingsResourceModel(ctx, state, got)...)
	if diags.HasError() {
		return false
	}

	cert, err := client.GetSsoCertificateV2(ctx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			return true
		}
		diags.AddError("Error reading Jamf Pro SSO signing certificate", helpers.APIErrorDetail(err))
		return false
	}
	diags.Append(assignSigningCertificateState(ctx, state, cert)...)
	return !diags.HasError()
}

// planCertConfigView returns the configuration's view of the signing_certificate
// sub-block (WriteOnly attributes survive only in Config, not Plan/State).
func planCertConfigView(plan, cfg SsoSettingsResourceModel) *signingCertificateModel {
	if cfg.SigningCertificate == nil {
		return plan.SigningCertificate
	}
	return cfg.SigningCertificate
}
