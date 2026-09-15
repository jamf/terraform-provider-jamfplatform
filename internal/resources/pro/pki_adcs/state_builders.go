// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_adcs

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignAdcsResourceModel populates a resource model from an SDK GET response.
//
// The WriteOnly certificate bytes/password are never touched (the framework
// excludes WriteOnly values from state and the server never returns them). The
// input certificate blocks' `filename` is refreshed in place from the server
// echo when the block is present, leaving `data_wo` / `wo_version` as the plan
// values so the rotation gate keeps working. The two *_details blocks are
// rebuilt as types.Object from the server metadata; connector_mode is derived
// from the wire `outbound` flag.
//
// hydrating allocates an absent input block on a first-time hydration, from the
// one attribute the server does echo — `filename`, which is Required inside the
// block. That one value is the whole point: it is what makes the block non-nil
// in state, and serverCertRotated / clientCertRotated read a nil prior block as
// "rotate". Without it, the apply that follows an import re-uploaded both
// certificates whether or not the practitioner had touched a rotation trigger,
// and when the trigger was left undeclared no plan diff said so — reproduced
// live on 2026-09-07, where that apply's request body carried the full .pem and
// .p12 bytes (issue #399). With the block hydrated the predicates fall through
// to comparing `wo_version`, which re-sends only on a declared bump and shows
// the diff that means it.
//
// The block is allocated only when the server actually holds a certificate, so
// a hydration never fabricates one. An INBOUND integration always has both (a
// config validator requires the blocks), and an OUTBOUND one has neither.
// importHydration reports whether this Read is the first one to write state for
// the integration, which is the only time an absent certificate block may be
// allocated.
//
// stateAbsent covers the identity-only import path (Terraform 1.12+), where the
// framework hands Read no prior state. The connector_mode check covers
// `terraform import <addr> <id>`, where ImportStatePassthroughID leaves a
// sparse-but-non-null state carrying only the id, so req.State.Raw.IsNull() is
// false and cannot be used alone. connector_mode is schema-Required, so Create
// and Update always populate it before any Read; it can only be null here on a
// first-time hydration. Sample it before assignAdcsResourceModel runs, which
// overwrites it from the wire.
func importHydration(stateAbsent bool, connectorMode types.String) bool {
	return stateAbsent || connectorMode.IsNull()
}

func assignAdcsResourceModel(ctx context.Context, state *AdcsResourceModel, s *pro.AdcsSettingsResponse, hydrating bool) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ConnectorMode = types.StringValue(outboundToConnectorMode(s.Outbound))
	state.DisplayName = types.StringValue(s.DisplayName)
	state.CaName = types.StringValue(s.CaName)
	state.Fqdn = types.StringValue(s.Fqdn)
	state.RevocationEnabled = types.BoolValue(s.RevocationEnabled)
	state.AdcsURL = types.StringValue(s.AdcsURL)
	state.APIClientID = optionalString(s.ApiClientID)
	state.ConnectorLastCheckIn = adcsTimestamp(s.ConnectorLastCheckInTimestamp)

	if hydrating && state.ServerCertificate == nil && s.ServerCert != nil {
		state.ServerCertificate = &adcsCertInputModel{}
	}
	if hydrating && state.ClientCertificate == nil && s.ClientCert != nil {
		state.ClientCertificate = &adcsClientCertInput{}
	}
	if state.ServerCertificate != nil && s.ServerCert != nil {
		state.ServerCertificate.Filename = types.StringValue(s.ServerCert.Filename)
	}
	if state.ClientCertificate != nil && s.ClientCert != nil {
		state.ClientCertificate.Filename = types.StringValue(s.ClientCert.Filename)
	}

	serverDetails, d := certDetailsObject(ctx, s.ServerCert)
	diags.Append(d...)
	state.ServerCertificateDetails = serverDetails

	clientDetails, d := certDetailsObject(ctx, s.ClientCert)
	diags.Append(d...)
	state.ClientCertificateDetails = clientDetails

	return diags
}

// assignAdcsDataSourceModel populates a data source model from an SDK GET
// response. The *_details blocks are typed-pointer (no plan/apply Unknown cycle
// in a data source).
func assignAdcsDataSourceModel(state *AdcsDataSourceModel, s *pro.AdcsSettingsResponse) {
	state.ConnectorMode = types.StringValue(outboundToConnectorMode(s.Outbound))
	state.DisplayName = types.StringValue(s.DisplayName)
	state.CaName = types.StringValue(s.CaName)
	state.Fqdn = types.StringValue(s.Fqdn)
	state.RevocationEnabled = types.BoolValue(s.RevocationEnabled)
	state.AdcsURL = types.StringValue(s.AdcsURL)
	state.APIClientID = optionalString(s.ApiClientID)
	state.ConnectorLastCheckIn = adcsTimestamp(s.ConnectorLastCheckInTimestamp)
	state.ServerCertificateDetails = certDetailsModel(s.ServerCert)
	state.ClientCertificateDetails = certDetailsModel(s.ClientCert)
}

// certDetailsObject builds a Computed *_details types.Object from a certificate
// metadata response, or ObjectNull when the server returned no certificate.
func certDetailsObject(ctx context.Context, c *pro.AdcsCertificateResponse) (types.Object, diag.Diagnostics) {
	if c == nil {
		return types.ObjectNull(adcsCertDetailsAttributeTypes), nil
	}
	return types.ObjectValueFrom(ctx, adcsCertDetailsAttributeTypes, certDetailsModel(c))
}

// certDetailsModel maps a certificate metadata response into the details model.
func certDetailsModel(c *pro.AdcsCertificateResponse) *adcsCertDetailsModel {
	if c == nil {
		return nil
	}
	return &adcsCertDetailsModel{
		Filename:       types.StringValue(c.Filename),
		SerialNumber:   types.StringValue(c.SerialNumber),
		Subject:        types.StringValue(c.Subject),
		Issuer:         types.StringValue(c.Issuer),
		ExpirationDate: adcsCertExpiration(c),
	}
}

// adcsCertExpiration maps a certificate's expiry into a Computed types.String.
// AdcsCertificateResponse.ExpirationDate is a *string holding Jamf Pro's
// offset-less wire datetime (e.g. "2036-06-06T17:42:41") — round-tripped
// verbatim, no parsing (the SDK fix changed it from *time.Time, which failed
// RFC3339 parse on the zone-less value). The connector check-in timestamp
// (adcsTimestamp) remains a genuine offset-bearing *time.Time.
func adcsCertExpiration(c *pro.AdcsCertificateResponse) types.String {
	if c == nil || c.ExpirationDate == nil {
		return types.StringNull()
	}
	return types.StringValue(*c.ExpirationDate)
}

// adcsTimestamp maps a *time.Time into a Computed RFC3339 types.String, Null when
// the server omitted it (e.g. the connector has never checked in).
func adcsTimestamp(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339))
}

// optionalString maps a possibly-empty server string into a Computed types.String
// — Null when empty (e.g. apiClientId is "" on an INBOUND record), else the value.
func optionalString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
