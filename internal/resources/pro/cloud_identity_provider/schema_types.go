// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// Provider-name discriminator values. Kept in one place so the schema
// validator, the cross-field validators, and the CRUD dispatch share the
// source of truth.
//
// These are the Terraform-facing values (what users write in `provider_name`).
// `ENTRA_ID` is the modern Microsoft name; the Jamf Pro API still uses the
// legacy `AZURE` on the wire. On write, the Entra branch sends wireProviderAzure
// directly; on read, providerNameFromWire maps the response back. `GOOGLE` is
// identical on both sides.
const (
	providerGoogle = pro.CloudIDPCommonProviderNameGoogle

	// providerEntraID is provider-facing only: the API has no such value, so
	// there is no constant to alias.
	providerEntraID = "ENTRA_ID"

	// wireProviderAzure is the legacy value the Jamf Pro API uses for Microsoft
	// Entra ID (cloudIdPCommon.providerName and the /cloud-azure endpoints).
	wireProviderAzure = pro.CloudIDPCommonProviderNameAzure
)

// providerNameFromWire maps a Jamf Pro API providerName back to the
// Terraform-facing value. Only Entra ID differs (AZURE → ENTRA_ID).
func providerNameFromWire(wireValue string) string {
	if wireValue == wireProviderAzure {
		return providerEntraID
	}
	return wireValue
}

// cloudIdentityProviderTimeoutAttributeTypes defines the timeout attribute
// types for the resource operations.
var cloudIdentityProviderTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
