// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_venafi

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildVenafiInput converts the Terraform plan model into an SDK VenafiCaRecord
// payload for POST (create) and PATCH (update).
//
// The Venafi PATCH has MERGE semantics: omit = preserve, empty string ""
// clears. So this builder only emits a field when the plan value is known and
// non-null — null/unknown plan fields are dropped so Jamf Pro retains the
// stored value. `name` is always emitted (Required on POST and PATCH).
//
// `refreshToken` is supplied separately by the CRUD caller (the WriteOnly
// secret, sourced from req.Config, gated by refresh_token_wo_version) — it is
// never derived from plan/state here. proxy_trust_store is reconciled by the
// CRUD caller via the dedicated proxy-trust-store endpoints, not this payload.
func buildVenafiInput(plan PkiVenafiResourceModel, refreshToken *string) *pro.VenafiCaRecord {
	rec := &pro.VenafiCaRecord{
		Name: plan.Name.ValueString(),
	}

	if !plan.ProxyAddress.IsNull() && !plan.ProxyAddress.IsUnknown() {
		v := plan.ProxyAddress.ValueString()
		rec.ProxyAddress = &v
	}
	if !plan.ClientID.IsNull() && !plan.ClientID.IsUnknown() {
		v := plan.ClientID.ValueString()
		rec.ClientID = &v
	}
	if !plan.RevocationEnabled.IsNull() && !plan.RevocationEnabled.IsUnknown() {
		v := plan.RevocationEnabled.ValueBool()
		rec.RevocationEnabled = &v
	}
	if refreshToken != nil {
		rec.RefreshToken = refreshToken
	}

	return rec
}
