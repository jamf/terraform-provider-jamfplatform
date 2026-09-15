// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_certificate_authority

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the certificate-authority data
// source's Read path calls. It mirrors the client.<Method> calls in
// data_source.go and drives the "Required Jamf permissions" table appended to
// the data source MarkdownDescription. permissions_test.go asserts this list
// stays in sync with the actual client calls and with the SDK privilege
// registry.
var dataSourceSDKMethods = []string{
	"GetCertificateAuthorityV1",
	"DownloadCertificateAuthorityPemV1",
	"GetActiveCertificateAuthorityV1",
	"DownloadActiveCertificateAuthorityPemV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the certificate-authority data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
