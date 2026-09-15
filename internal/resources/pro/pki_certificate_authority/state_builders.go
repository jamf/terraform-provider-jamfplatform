// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_certificate_authority

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignCertificateAuthorityDataSourceModel populates the data source model from an SDK
// CertificateRecord plus the downloaded PEM blob. All fields are read-only.
func assignCertificateAuthorityDataSourceModel(ctx context.Context, state *CertificateAuthorityDataSourceModel, r *pro.CertificateRecord, pem []byte) diag.Diagnostics {
	var diags diag.Diagnostics

	state.SubjectX500Principal = types.StringValue(r.SubjectX500Principal)
	state.IssuerX500Principal = types.StringValue(r.IssuerX500Principal)
	state.SerialNumber = types.StringValue(r.SerialNumber)
	state.Version = types.Int64Value(int64(r.Version))
	state.NotAfter = types.Int64Value(int64(r.NotAfter))
	state.NotBefore = types.Int64Value(int64(r.NotBefore))

	keyUsage, d := types.ListValueFrom(ctx, types.StringType, r.KeyUsage)
	diags.Append(d...)
	state.KeyUsage = keyUsage

	keyUsageExt, d := types.ListValueFrom(ctx, types.StringType, r.KeyUsageExtended)
	diags.Append(d...)
	state.KeyUsageExtended = keyUsageExt

	state.Sha1Fingerprint = types.StringValue(r.Sha1Fingerprint)
	state.Sha256Fingerprint = types.StringValue(r.Sha256Fingerprint)

	if r.Signature != nil {
		state.SignatureAlgorithm = types.StringValue(r.Signature.Algorithm)
		state.SignatureAlgorithmOid = types.StringValue(r.Signature.AlgorithmOid)
		state.SignatureValue = types.StringValue(r.Signature.Value)
	} else {
		state.SignatureAlgorithm = types.StringNull()
		state.SignatureAlgorithmOid = types.StringNull()
		state.SignatureValue = types.StringNull()
	}

	state.Pem = types.StringValue(string(pem))

	return diags
}
