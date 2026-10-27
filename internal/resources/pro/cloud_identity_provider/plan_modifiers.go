// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ModifyPlan performs a best-effort, plan-time verification of the Google
// keystore so an invalid certificate/password surfaces at `terraform plan`
// rather than only at apply.
//
// Guardrails (spike §2.4):
//   - Skipped on destroy (null plan) and before the client is configured.
//   - GOOGLE-only — Azure has no keystore.
//   - Runs only when both `file` and `password` are known (they are WriteOnly,
//     so they are read from config, where they survive).
//   - NON-FATAL: any verify failure is a warning, not an error, so a
//     network-isolated `terraform plan` (e.g. CI) is never broken. Real
//     keystore problems still hard-fail server-side at apply.
func (r *CloudIdentityProviderResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy
	}
	if r.client == nil {
		return
	}

	r.reportNameCollision(ctx, req, resp)

	var cfg CloudIdentityProviderResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.ProviderName.IsNull() || cfg.ProviderName.IsUnknown() || cfg.ProviderName.ValueString() != providerGoogle {
		return
	}
	if cfg.Google == nil || cfg.Google.Server == nil || cfg.Google.Server.Keystore == nil {
		return
	}
	ks := cfg.Google.Server.Keystore
	if ks.File.IsNull() || ks.File.IsUnknown() || ks.Password.IsNull() || ks.Password.IsUnknown() {
		return
	}

	file, diags := buildKeystoreFile(ks, ks)
	if diags.HasError() {
		// Malformed base64 is a genuine config error — surface it.
		resp.Diagnostics.Append(diags...)
		return
	}

	if _, err := r.client.VerifyLdapKeystoreV1(ctx, &file); err != nil {
		resp.Diagnostics.AddWarning(
			"Could not verify Google keystore",
			"The provider could not validate the Google keystore (`google.server.keystore`) at plan time: "+err.Error()+
				"\n\nThis is a non-fatal check. If the tenant is unreachable from here (e.g. CI), ignore it. If the certificate or password is wrong, the apply will fail.",
		)
		return
	}
	tflog.Debug(ctx, "Google keystore verified at plan time")
}
