// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_account_sso_domain"

// ssoDomainNamespace is the uniqueness rule for SSO domains. A domain belongs to
// one Jamf Account organization and a second claim is refused with 409 CONFLICT.
// Jamf Account lower-cases a domain when it stores it, and a domain with a
// trailing space, a trailing dot or an accent is refused as invalid before it can
// collide (wire-probed 2026-10-25).
//
// The collection holds the organization's own claims next to domains other
// organizations shared with it. Both block a claim, but only an owned domain can
// be imported, so the two are separate sources over one listing. An entry's ID is
// the domain name, because that is what this resource imports by.
func ssoDomainNamespace(client *account.Client) collision.Namespace {
	list := newDomainLister(client)
	return collision.Namespace{
		ID:        "account.sso-domains",
		Product:   "Jamf Account",
		KeyLabel:  "domain",
		Enforced:  true,
		Collation: collision.Collation{IgnoreCase: true},
		Sources: []collision.Source{
			{
				Noun:         "SSO domain",
				ResourceType: resourceTypeName,
				List:         list.entries(false),
			},
			{
				Noun: "shared SSO domain",
				List: list.entries(true),
			},
		},
	}
}

// domainLister reads the domain collection once and serves both sources from it.
type domainLister struct {
	client  *account.Client
	once    sync.Once
	domains []account.Domain
	err     error
}

func newDomainLister(client *account.Client) *domainLister {
	return &domainLister{client: client}
}

func (l *domainLister) entries(shared bool) func(context.Context) ([]collision.Entry, error) {
	return func(ctx context.Context) ([]collision.Entry, error) {
		l.once.Do(func() { l.domains, l.err = l.client.ListDomains(ctx) })
		if l.err != nil {
			return nil, l.err
		}
		var entries []collision.Entry
		for _, d := range l.domains {
			if d.SharedDomain == shared {
				entries = append(entries, collision.Entry{ID: d.Domain, Key: d.Domain})
			}
		}
		return entries, nil
	}
}

// ModifyPlan reports a domain that is already claimed or shared.
func (r *DomainResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[DomainResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		IDPath:       path.Root("domain"),
		Rules: []collision.Rule[DomainResourceModel]{{
			Namespace: ssoDomainNamespace(r.client),
			Path:      path.Root("domain"),
			Keys:      func(m *DomainResourceModel) []types.String { return []types.String{m.Domain} },
		}},
	})
}
