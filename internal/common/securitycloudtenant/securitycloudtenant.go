// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package securitycloudtenant resolves the ID of the Jamf Security Cloud tenant the provider's
// credentials act on, which is what a ZTNA gateway's or grouped gateway's `tenantIds` names when
// the configuration leaves it out.
//
// The value is the Security Cloud tenant's own ID: the one Jamf Account lists for the Jamf
// Security Cloud service of a platform environment, the `customerId` in the admin console's URL,
// and the `customerId` a UEM Connect connector on that tenant reports. The gateway service calls
// the field `customerIds` in its own error messages. The server does not default it: a create
// omitting `tenantIds`, or sending `[]`, is refused `400 [INVALID_FIELD] customerIds: must not be
// empty` (wire-probed on the EU gateway under X-Environment-Id, 2026-09-30).
//
// Two sources are reliable, and [Resolve] tries them in order.
//
//   - Under tenant scope the configured `X-Tenant-Id` is the tenant, because a Security Cloud
//     construct is only reachable there with a Security Cloud tenant's ID.
//   - Under environment scope the UEM Connect connector's documented `customerId` is. A tenant
//     holds at most one connector, and it may hold none.
//
// Nothing else carries the ID. With the connector deleted, 2026-09-30, the connector list came
// back empty and no documented Security Cloud read returned the ID in a body or a header. The one
// exception is the `Link` header of the deprecated, spec-withdrawn `GET /v1/groups`, whose
// `Deprecation` date has passed; the SDK has no method for that route and exposes no response
// headers, so reading it would mean an unlogged hand-built request, and it is deliberately not a
// source. The account-organization API ("environments and tenants") is the lookup that would cover
// every case, but its gateway mount answers a plain-text 404, and every published Jamf Account
// namespace is reachable only under organization scope, which no Security Cloud construct runs
// under.
package securitycloudtenant

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ErrNoSource reports that the provider is environment-scoped and the tenant holds no UEM Connect
// connector, so nothing readable names the Security Cloud tenant.
var ErrNoSource = errors.New("no UEM Connect integration names the Security Cloud tenant")

// ConnectorLister reads the tenant's UEM Connect connectors. Callers pass a closure over their
// own client's ListUemConnectorsV1 so the call stays visible to their permissions check.
type ConnectorLister func(ctx context.Context) (*securitycloud.ConnectorPage, error)

// Resolve returns the Security Cloud tenant ID for scopeTenantID, the configured tenant-scope ID
// ("" under any other scope), falling back to the connectors list reports.
//
// More than one distinct `customerId` across the connectors is an error rather than a choice:
// a tenant holds one connector, so disagreement means the assumption this package rests on no
// longer holds, and picking one would grant a gateway to a tenant nobody named.
func Resolve(ctx context.Context, scopeTenantID string, list ConnectorLister) (string, error) {
	if scopeTenantID != "" {
		return scopeTenantID, nil
	}
	page, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("reading the UEM Connect integration: %w", err)
	}
	var ids []string
	if page != nil {
		for _, connector := range page.Results {
			if connector.CustomerID != "" && !slices.Contains(ids, connector.CustomerID) {
				ids = append(ids, connector.CustomerID)
			}
		}
	}
	switch len(ids) {
	case 0:
		return "", ErrNoSource
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("the UEM Connect integrations name %d different Security Cloud tenants (%s)", len(ids), strings.Join(ids, ", "))
	}
}

// AppendUnresolved adds the error for a tenant_ids that is unset and could not be resolved,
// attributed to attr.
func AppendUnresolved(diags *diag.Diagnostics, attr path.Path, err error) {
	detail := "`tenant_ids` is unset, so the provider looks up your Security Cloud tenant ID. "
	if errors.Is(err, ErrNoSource) {
		detail += "The provider uses an environment-scoped integration and this environment has no UEM Connect " +
			"integration, which is the only place the Jamf API reports that ID."
	} else {
		detail += "The lookup failed: " + helpers.APIErrorDetail(err)
	}
	detail += "\n\nSet `tenant_ids` yourself. In Jamf Account, open Platform environments, choose this " +
		"environment, and use Copy ID on its Jamf Security Cloud row."
	diags.AddAttributeError(attr, "Could not determine the Security Cloud tenant", detail)
}
