// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_digicert

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignDigicertServerFields populates the server-derived fields of a resource
// model from a DigiCertSettingResponse. It leaves the client_certificate INPUT
// block alone on an ordinary refresh — it carries WriteOnly bytes/password and a
// config-only wo_version the CRUD caller owns. The Computed
// client_certificate_details object is built from the response's certificate
// metadata.
//
// hydrating allocates the input block on a first-time hydration, from the one
// attribute the server echoes: `filename`, which is Required inside the block.
// That single value is the point of it — it is what makes the block non-nil in
// state, and shouldRotateCert reads a nil prior block as "rotate". Without it the
// apply that followed an import re-sent the certificate and password whether or
// not the practitioner had touched the rotation trigger, and when the trigger was
// left undeclared no plan diff said so (issue #399; reproduced live on the
// sibling pki_adcs, whose predicate has the same shape). With the block hydrated
// the predicate falls through to comparing wo_version, which re-sends only on a
// declared bump and shows the diff that means it.
//
// The block is allocated only when the server actually holds a certificate, so a
// hydration never fabricates one.
// importHydration reports whether this Read is the first one to write state for
// the integration, which is the only time an absent certificate block may be
// allocated.
//
// stateAbsent covers the identity-only import path (Terraform 1.12+), where the
// framework hands Read no prior state. The display_name check covers
// `terraform import <addr> <id>`, where ImportStatePassthroughID leaves a
// sparse-but-non-null state carrying only the id, so req.State.Raw.IsNull() is
// false and cannot be used alone. display_name is schema-Required, so Create and
// Update always populate it before any Read; it can only be null here on a
// first-time hydration. Sample it before assignDigicertServerFields runs, which
// overwrites it from the wire.
func importHydration(stateAbsent bool, displayName types.String) bool {
	return stateAbsent || displayName.IsNull()
}

func assignDigicertServerFields(state *DigicertResourceModel, resp *pro.DigiCertSettingResponse, hydrating bool) diag.Diagnostics {
	state.ID = types.StringValue(resp.ID)
	state.DisplayName = types.StringValue(resp.CaName)
	state.HostName = types.StringValue(resp.Fqdn)
	state.RevocationEnabled = types.BoolValue(resp.RevocationEnabled)

	if hydrating && state.ClientCertificate == nil && resp.ClientCert != nil {
		state.ClientCertificate = &DigicertClientCertModel{
			Filename: types.StringValue(resp.ClientCert.Filename),
		}
	}

	details, diags := clientCertificateDetailsObject(resp.ClientCert)
	if diags.HasError() {
		return diags
	}
	state.ClientCertificateDetails = details
	return diags
}

// assignDigicertDataSourceModel populates a data source model from a
// DigiCertSettingResponse.
func assignDigicertDataSourceModel(state *DigicertDataSourceModel, resp *pro.DigiCertSettingResponse) diag.Diagnostics {
	state.ID = types.StringValue(resp.ID)
	state.DisplayName = types.StringValue(resp.CaName)
	state.HostName = types.StringValue(resp.Fqdn)
	state.RevocationEnabled = types.BoolValue(resp.RevocationEnabled)

	details, diags := clientCertificateDetailsObject(resp.ClientCert)
	if diags.HasError() {
		return diags
	}
	state.ClientCertificateDetails = details
	return diags
}

// clientCertificateDetailsObject builds the Computed client_certificate_details
// types.Object from the response certificate metadata. Returns a null object when
// no certificate is stored.
func clientCertificateDetailsObject(cert *pro.CertificateResponse) (types.Object, diag.Diagnostics) {
	if cert == nil {
		return types.ObjectNull(clientCertificateDetailsAttrTypes), nil
	}
	attrs := map[string]attr.Value{
		"filename":        types.StringValue(cert.Filename),
		"serial_number":   types.StringValue(cert.SerialNumber),
		"subject":         types.StringValue(cert.Subject),
		"issuer":          types.StringValue(cert.Issuer),
		"expiration_date": expirationDate(cert.ExpirationDate),
	}
	return types.ObjectValue(clientCertificateDetailsAttrTypes, attrs)
}

// expirationDate maps the response *string (offset-less wire datetime, e.g.
// "2036-06-06T17:42:41") to a Computed string, null when absent. Round-tripped
// verbatim — Jamf Pro returns no timezone, so no parsing is attempted.
func expirationDate(t *string) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(*t)
}
