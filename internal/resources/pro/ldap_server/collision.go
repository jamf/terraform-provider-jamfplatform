// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_ldap_server"

// ldapServerNamespace is the uniqueness rule for LDAP server names. Jamf Pro refuses a
// duplicate with 409, and the Classic API trims the name and compares it without regard to case or surrounding spaces but with accents significant.
func ldapServerNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.ldap-servers",
		Product:   "Jamf Pro",
		KeyLabel:  "display name",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "LDAP server",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListLDAPServers(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed.LdapServers))
				for _, item := range listed.LdapServers {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a LDAP server name that another LDAP server already holds.
func (r *LdapServerResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: ldapServerNamespace(r.client),
			Path:      path.Root("connection_settings").AtName("display_name"),
		}},
	})
}

var _ resource.ResourceWithModifyPlan = &LdapServerResource{}

// ModifyPlan reports a LDAP server name that another LDAP server already holds.
func (r *LdapServerResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
}
