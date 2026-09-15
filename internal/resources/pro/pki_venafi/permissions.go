// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_venafi

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Venafi CA resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateVenafiV1",
	"GetVenafiV1",
	"UpdateVenafiV1",
	"DeleteVenafiV1",
	"GetVenafiJamfPublicKeyV1",
	"RegenerateVenafiJamfPublicKeyV1",
	"GetVenafiProxyTrustStoreV1",
	"UploadVenafiProxyTrustStoreV1",
	"DeleteVenafiProxyTrustStoreV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Venafi CA resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the Venafi CA data source's Read
// path calls. A data source documents only the read privileges it needs, not
// the resource's full CRUD set. permissions_test.go keeps it in sync with the
// client.<Method> calls in data_source.go and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetVenafiV1",
	"GetVenafiJamfPublicKeyV1",
	"GetVenafiProxyTrustStoreV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Venafi CA data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
