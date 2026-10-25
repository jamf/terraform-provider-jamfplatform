// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_security_cloud_dns_zone"

// dnsZoneNamespace is the uniqueness rule for DNS zone domains. A domain belongs
// to one zone and Jamf Security Cloud refuses a second with 409 DOMAIN_CONFLICT;
// the zone name is not a key. The match ignores case and nothing else, and a
// wildcard entry is a different domain from its apex (wire-probed 2026-10-25). A
// domain with a trailing space, a trailing dot or an accent is refused as invalid
// before it can collide.
func dnsZoneNamespace(client *securitycloud.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "securitycloud.dns-zone-domains",
		Product:   "Jamf Security Cloud",
		KeyLabel:  "domain",
		Enforced:  true,
		Collation: collision.Collation{IgnoreCase: true},
		Sources: []collision.Source{{
			Noun:         "DNS zone",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				zones, err := client.ListDnsZonesV1(ctx, defaultZoneSort)
				if err != nil {
					return nil, err
				}
				var entries []collision.Entry
				for _, z := range zones.Results {
					for _, d := range z.Domains {
						entries = append(entries, collision.Entry{ID: z.ID, Key: d})
					}
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a domain that another DNS zone already serves.
func (r *DNSZoneResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[DNSZoneResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[DNSZoneResourceModel]{{
			Namespace: dnsZoneNamespace(r.client),
			Path:      path.Root("domains"),
			Keys:      func(m *DNSZoneResourceModel) []types.String { return collision.SetKeys(m.Domains) },
		}},
	})
}
