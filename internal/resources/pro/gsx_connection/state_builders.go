// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package gsx_connection

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignGsxConnectionSettingsResourceModel populates a resource model from an SDK GET
// response. The three WriteOnly secrets (token, keystore bytes, keystore password) are
// never touched: the GSX API never returns them and the framework excludes WriteOnly
// values from state regardless. Only the non-secret and read-only-metadata fields are
// assigned from the authoritative GET-after-write.
func assignGsxConnectionSettingsResourceModel(state *GsxConnectionSettingsResourceModel, s *pro.GsxConnection) {
	state.Enabled = types.BoolValue(s.Enabled)
	state.Username = types.StringValue(s.Username)
	state.ServiceAccountNumber = types.StringValue(s.ServiceAccountNo)
	state.ShipToNumber = types.StringValue(stringPtrValue(s.ShipToNo))
	state.KeystoreName = types.StringValue(s.GsxKeystore.Name)
	state.KeystoreErrorMessage = optionalString(s.GsxKeystore.ErrorMessage)
	state.KeystoreExpirationEpoch = optionalInt64(s.GsxKeystore.ExpirationEpoch)
}

// assignGsxConnectionSettingsDataSourceModel populates a data source model from an SDK GET
// response. Same non-secret subset as the resource assigner.
func assignGsxConnectionSettingsDataSourceModel(state *GsxConnectionSettingsDataSourceModel, s *pro.GsxConnection) {
	state.Enabled = types.BoolValue(s.Enabled)
	state.Username = types.StringValue(s.Username)
	state.ServiceAccountNumber = types.StringValue(s.ServiceAccountNo)
	state.ShipToNumber = types.StringValue(stringPtrValue(s.ShipToNo))
	state.KeystoreName = types.StringValue(s.GsxKeystore.Name)
	state.KeystoreErrorMessage = optionalString(s.GsxKeystore.ErrorMessage)
	state.KeystoreExpirationEpoch = optionalInt64(s.GsxKeystore.ExpirationEpoch)
}

// stringPtrValue dereferences a *string, yielding "" for nil.
func stringPtrValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// optionalString maps a read-only *string into a Computed types.String — Null when the
// server omitted it (e.g. no certificate error), else the value.
func optionalString(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// optionalInt64 maps a read-only *int64 into a Computed types.Int64 — Null when the server
// omitted it (e.g. no certificate uploaded), else the value.
func optionalInt64(p *int64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*p)
}
