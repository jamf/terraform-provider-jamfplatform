// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_venafi

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// idString renders the record's *int ID as the canonical string Terraform ID.
// Returns ("", false) when the pointer is nil so callers can guard.
func idString(id *int) (string, bool) {
	if id == nil {
		return "", false
	}
	return strconv.Itoa(*id), true
}

// assignVenafiServerFields populates the server-derived fields of a resource
// model from a VenafiCaRecord (the GET shape: id, name, proxyAddress, clientId,
// revocationEnabled, refreshTokenConfigured). It deliberately does NOT touch
// refresh_token_wo, refresh_token_wo_version, jamf_public_key,
// jamf_public_key_rotation, or proxy_trust_store — those are managed by the
// CRUD caller (WriteOnly secret / rotation triggers / separate endpoints).
//
// proxy_address and client_id use PreserveStringWhenWireEmpty: the Venafi API
// echoes a cleared field as absent/empty, but the plan may carry an explicit
// "" (the clear value) under an Optional+Computed attribute. Mapping wire-empty
// back to the model's current value keeps the planned "" rather than collapsing
// to null, which would trip Terraform Core's post-apply consistency check.
func assignVenafiServerFields(state *PkiVenafiResourceModel, rec *pro.VenafiCaRecord) {
	if s, ok := idString(rec.ID); ok {
		state.ID = types.StringValue(s)
	}
	state.Name = types.StringValue(rec.Name)
	state.ProxyAddress = helpers.PreserveStringWhenWireEmpty(rec.ProxyAddress, state.ProxyAddress)
	state.ClientID = helpers.PreserveStringWhenWireEmpty(rec.ClientID, state.ClientID)
	state.RevocationEnabled = types.BoolValue(boolPtrValue(rec.RevocationEnabled))
	state.RefreshTokenConfigured = types.BoolValue(boolPtrValue(rec.RefreshTokenConfigured))
}

// assignVenafiDataSourceModel populates a data source model from a
// VenafiCaRecord. The refresh token is never exposed.
func assignVenafiDataSourceModel(state *PkiVenafiDataSourceModel, rec *pro.VenafiCaRecord) {
	if s, ok := idString(rec.ID); ok {
		state.ID = types.StringValue(s)
	}
	state.Name = types.StringValue(rec.Name)
	state.ProxyAddress = stringPtrValue(rec.ProxyAddress)
	state.ClientID = stringPtrValue(rec.ClientID)
	state.RevocationEnabled = types.BoolValue(boolPtrValue(rec.RevocationEnabled))
	state.RefreshTokenConfigured = types.BoolValue(boolPtrValue(rec.RefreshTokenConfigured))
}

// boolPtrValue dereferences a *bool, treating nil as false.
func boolPtrValue(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// stringPtrValue maps a *string to a types.String (nil → null).
func stringPtrValue(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}
