// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_adcs

import (
	"encoding/base64"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// connectorModeToOutbound maps the connector_mode enum to the wire `outbound`
// bool: INBOUND => false, OUTBOUND => true.
func connectorModeToOutbound(mode string) bool {
	return mode == connectorModeOutbound
}

// outboundToConnectorMode maps the wire `outbound` bool back to the enum.
func outboundToConnectorMode(outbound bool) string {
	if outbound {
		return connectorModeOutbound
	}
	return connectorModeInbound
}

// buildAdcsCreateInput builds the POST body for an AD CS create. On Create the
// mode is set (the discriminator) and the certificate blocks are included in full
// for INBOUND (decoding data_wo from base64). For OUTBOUND only api_client_id is
// carried. Scalar fields are emitted only when the plan declares them (drop
// null/unknown — merge-patch on the server preserves the rest, and on create the
// server simply defaults them).
//
// The WriteOnly secrets (data_wo / password_wo) are read from cfg (req.Config):
// the framework strips WriteOnly values from plan/state.
func buildAdcsCreateInput(plan, cfg AdcsResourceModel) (*pro.AdcsSettings, error) {
	mode := plan.ConnectorMode.ValueString()
	outbound := connectorModeToOutbound(mode)

	out := &pro.AdcsSettings{
		Outbound:          &outbound,
		DisplayName:       stringPtrOrNil(plan.DisplayName),
		CaName:            stringPtrOrNil(plan.CaName),
		Fqdn:              stringPtrOrNil(plan.Fqdn),
		RevocationEnabled: boolPtrOrNil(plan.RevocationEnabled),
	}

	if outbound {
		out.ApiClientID = stringPtrOrNil(plan.APIClientID)
		return out, nil
	}

	// INBOUND.
	out.AdcsURL = stringPtrOrNil(plan.AdcsURL)

	serverCert, err := buildServerCert(plan.ServerCertificate, cfg.ServerCertificate)
	if err != nil {
		return nil, err
	}
	out.ServerCert = serverCert

	clientCert, err := buildClientCert(plan.ClientCertificate, cfg.ClientCertificate)
	if err != nil {
		return nil, err
	}
	out.ClientCert = clientCert

	return out, nil
}

// buildAdcsUpdateInput builds the PATCH (merge-patch) body for an AD CS update.
//
// `outbound` is NEVER sent on update: the mode is immutable and RequiresReplace
// handles a change via recreate. Scalars are dropped when null/unknown so the
// server preserves them (genuine merge). A certificate block is included only
// when its wo_version changed versus state (per-cert rotation gate); the
// certificate is sent in full (data + filename + password) or not at all.
func buildAdcsUpdateInput(plan, state, cfg AdcsResourceModel) (*pro.AdcsSettings, error) {
	out := &pro.AdcsSettings{
		DisplayName:       stringPtrOrNil(plan.DisplayName),
		CaName:            stringPtrOrNil(plan.CaName),
		Fqdn:              stringPtrOrNil(plan.Fqdn),
		RevocationEnabled: boolPtrOrNil(plan.RevocationEnabled),
		AdcsURL:           stringPtrOrNil(plan.AdcsURL),
		ApiClientID:       stringPtrOrNil(plan.APIClientID),
	}

	if serverCertRotated(plan, state) {
		serverCert, err := buildServerCert(plan.ServerCertificate, cfg.ServerCertificate)
		if err != nil {
			return nil, err
		}
		out.ServerCert = serverCert
	}

	if clientCertRotated(plan, state) {
		clientCert, err := buildClientCert(plan.ClientCertificate, cfg.ClientCertificate)
		if err != nil {
			return nil, err
		}
		out.ClientCert = clientCert
	}

	return out, nil
}

// serverCertRotated reports whether the server_certificate block's wo_version
// changed between state and plan (so the certificate must be re-sent). When the
// block is absent in the plan there is nothing to send.
func serverCertRotated(plan, state AdcsResourceModel) bool {
	if plan.ServerCertificate == nil {
		return false
	}
	if state.ServerCertificate == nil {
		return true
	}
	return !plan.ServerCertificate.WoVersion.Equal(state.ServerCertificate.WoVersion)
}

// clientCertRotated reports whether the client_certificate block's wo_version
// changed between state and plan.
func clientCertRotated(plan, state AdcsResourceModel) bool {
	if plan.ClientCertificate == nil {
		return false
	}
	if state.ClientCertificate == nil {
		return true
	}
	return !plan.ClientCertificate.WoVersion.Equal(state.ClientCertificate.WoVersion)
}

// buildServerCert decodes the server certificate input block into an SDK
// AdcsCertificate. data_wo is read from cfg (WriteOnly); filename from plan.
// Returns nil when the block is absent (nothing to send).
func buildServerCert(plan *adcsCertInputModel, cfg *adcsCertInputModel) (*pro.AdcsCertificate, error) {
	if plan == nil || cfg == nil {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(cfg.DataWo.ValueString())
	if err != nil {
		return nil, fmt.Errorf("server_certificate.data_wo is not valid base64 (use filebase64(\"server.pem\")): %w", err)
	}
	return &pro.AdcsCertificate{
		Data:     data,
		Filename: plan.Filename.ValueString(),
	}, nil
}

// buildClientCert decodes the client certificate input block into an SDK
// AdcsCertificate, including the keystore password. data_wo + password_wo are
// read from cfg (WriteOnly); filename from plan. Returns nil when absent.
func buildClientCert(plan *adcsClientCertInput, cfg *adcsClientCertInput) (*pro.AdcsCertificate, error) {
	if plan == nil || cfg == nil {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(cfg.DataWo.ValueString())
	if err != nil {
		return nil, fmt.Errorf("client_certificate.data_wo is not valid base64 (use filebase64(\"client.p12\")): %w", err)
	}
	out := &pro.AdcsCertificate{
		Data:     data,
		Filename: plan.Filename.ValueString(),
	}
	if !cfg.PasswordWo.IsNull() && !cfg.PasswordWo.IsUnknown() {
		pw := cfg.PasswordWo.ValueString()
		out.Password = &pw
	}
	return out, nil
}

// stringPtrOrNil returns a *string for a known, non-null types.String, else nil
// (omitted on the wire — preserved on merge-patch).
func stringPtrOrNil(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// boolPtrOrNil returns a *bool for a known, non-null types.Bool, else nil.
func boolPtrOrNil(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}
