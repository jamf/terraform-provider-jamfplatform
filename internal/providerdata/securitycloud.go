// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// ConfigureSecurityCloud is the shared Configure boilerplate for every Jamf
// Security Cloud resource, data source, list resource and action. It type-asserts
// providerData into *Data, gates on the API integration scope, and returns a
// *securitycloud.Client.
//
// It deliberately does NOT go through configureSub. That helper fetches the Jamf
// Pro tenant version and can surface the provider-floor advisory, which is
// meaningless here: Security Cloud is a separate product with its own release
// train, and a tenant can hold it without holding Jamf Pro at all. Routing
// Security Cloud through configureSub would make every Security Cloud construct
// depend on a Jamf Pro read it has no use for — and, on a Pro-less tenant, turn
// a working construct into a Configure failure.
//
// Scope: environment first (the preferred integration scope), then tenant. Both
// reach Security Cloud — the DNS surface was wire-verified under a tenant-scoped
// integration against production EU on 2026-08-27. Organization scope is
// rejected, as everywhere else.
//
// resourceType is the fully-qualified Terraform type name used in diagnostics
// (e.g. "jamfplatform_security_cloud_dns_zone"). Returns (nil, nil) when
// providerData is nil — the framework calls Configure with a nil ProviderData
// during early lifecycle, and the construct simply stays unconfigured until a
// later call supplies it.
func ConfigureSecurityCloud(_ context.Context, providerData any, resourceType string) (*securitycloud.Client, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	pd, ok := providerData.(*Data)
	if !ok {
		diags.AddError(
			"Unexpected Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, diags
	}
	if scopeDiags := pd.RequireScope(resourceType, SecurityCloudScopes...); scopeDiags.HasError() {
		diags.Append(scopeDiags...)
		return nil, diags
	}
	return securitycloud.New(pd.Client), diags
}
