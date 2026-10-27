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

// dnsZoneNameNamespace is the strict-only uniqueness rule for DNS zone names.
// Jamf Security Cloud stores two zones with one name, whatever the case, trailing
// space or accent (wire-probed 2026-10-25), so the rule is Enforced false and
// applies only when include_unenforced is set. The comparison is
// collision.CollationFolded, the strictest comparison, because a zone accepted
// next to one whose name differs only by case or accent still reads as a
// duplicate in the Jamf Security Cloud console.
func dnsZoneNameNamespace(client *securitycloud.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "securitycloud.dns-zone-names",
		Product:   "Jamf Security Cloud",
		KeyLabel:  "name",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "DNS zone",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				zones, err := client.ListDnsZonesV1(ctx, defaultZoneSort)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(zones.Results))
				for _, z := range zones.Results {
					entries = append(entries, collision.Entry{ID: z.ID, Key: z.Name})
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a domain that another DNS zone already serves, and a zone
// name another zone already holds when include_unenforced is set.
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
		}, {
			Namespace: dnsZoneNameNamespace(r.client),
			Path:      path.Root("name"),
			Keys:      func(m *DNSZoneResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
