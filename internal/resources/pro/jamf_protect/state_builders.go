// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_protect

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignJamfProtectResourceModel populates the resource model from an SDK
// GET/POST/PUT response (the register POST body is byte-identical to a
// subsequent GET, so all three feed the same assigner).
//
// The wire echoes the registration's client ID as `apiClientId`; it maps back
// to the user-authored `client_id` so an out-of-band re-registration with
// different credentials surfaces as drift. The WriteOnly `password` and its
// `password_wo_version` companion are deliberately NOT assigned — the
// framework strips the plaintext from state, and the rotation trigger must
// round-trip from prior state (the wire never echoes it). The assigner also
// never writes state.ID — the CRUD handler stamps helpers.SingletonID.
func assignJamfProtectResourceModel(state *JamfProtectResourceModel, s *pro.ProtectSettingsResponse) {
	state.APIURL = types.StringValue(s.ProtectURL)
	state.ClientID = types.StringValue(s.ApiClientID)
	state.AutoInstall = types.BoolValue(s.AutoInstall)

	// Server-derived echoes — Computed only, server-authoritative.
	state.RegistrationID = types.StringValue(s.RegistrationID)
	state.APIClientName = types.StringValue(s.ApiClientName)
	state.PlatformPlanSync = types.BoolValue(s.PlatformPlanSync)
	state.LastSyncTime = types.StringValue(s.LastSyncTime)
	state.SyncStatus = types.StringValue(s.SyncStatus)
}

// assignJamfProtectPlanModel projects one synced plan row into the data
// source model.
func assignJamfProtectPlanModel(p *pro.JamfProtectPlan) JamfProtectPlanModel {
	return JamfProtectPlanModel{
		ID:               types.StringValue(p.ID),
		UUID:             types.StringValue(p.UUID),
		Name:             types.StringValue(p.Name),
		Description:      types.StringValue(p.Description),
		ProfileID:        types.Int64Value(int64(p.ProfileID)),
		ProfileName:      types.StringValue(p.ProfileName),
		ProfileVersion:   types.Int64Value(int64(p.ProfileVersion)),
		ScopeDescription: types.StringValue(p.ScopeDescription),
		SiteID:           types.StringValue(p.SiteID),
	}
}

// mapJamfProtectPlans maps every SDK plan into the data source model. The
// slice is always non-nil so an empty catalog serialises as an empty list,
// not null.
func mapJamfProtectPlans(plans []pro.JamfProtectPlan) []JamfProtectPlanModel {
	out := make([]JamfProtectPlanModel, 0, len(plans))
	for i := range plans {
		out = append(out, assignJamfProtectPlanModel(&plans[i]))
	}
	return out
}
