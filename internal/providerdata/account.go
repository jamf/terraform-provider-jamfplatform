// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// ConfigureAccount is the shared Configure boilerplate for every Jamf Account
// resource, data source, list resource and action. It type-asserts providerData
// into *Data, gates on the API integration scope, and returns an
// *account.Client.
//
// Like ConfigureSecurityCloud it deliberately bypasses configureSub: Jamf
// Account is organization-level and has no customer-tenant version, so the Jamf
// Pro version read that helper performs would be meaningless here — and fatal on
// an organization that holds no Jamf Pro tenant at all.
//
// Scope: ScopeOrganization alone, and this is the first construct family in the
// provider to require it. Wire-probed on 2026-09-02 against two independent
// organization-scoped integrations, which reach /sso/v1 while a tenant-scoped
// one is refused BAD_PERMISSIONS on the same URL in the same region — the
// refusal is provably an authorization decision rather than an unmapped route,
// because a different credential answers 200 there. Environment scope is
// deliberately *not* listed: it is untested, because the namespace exists only
// on the US gateway and no US environment-scoped integration was available.
// Widening the gate is therefore a fresh probe, not a one-token edit.
//
// An organization-scoped integration sends no scope header at all — the gateway
// resolves the organization from the access token, and /sso/v1 ignores
// X-Environment-Id and X-Tenant-Id entirely when one is present. So there is no
// organization ID for the provider to carry and none for the SDK to send.
//
// resourceType is the fully-qualified Terraform type name used in diagnostics
// (e.g. "jamfplatform_account_sso_domain"). Returns (nil, nil) when providerData
// is nil — the framework calls Configure with a nil ProviderData during early
// lifecycle, and the construct stays unconfigured until a later call supplies it.
func ConfigureAccount(_ context.Context, providerData any, resourceType string) (*account.Client, diag.Diagnostics) {
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
	if scopeDiags := pd.RequireScope(resourceType, AccountScopes...); scopeDiags.HasError() {
		diags.Append(scopeDiags...)
		return nil, diags
	}
	return account.New(pd.Client), diags
}
