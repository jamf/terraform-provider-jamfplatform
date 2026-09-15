// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package accountprivileges

import (
	"context"
	"fmt"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// administratorPrivilegeSet is the privilege_set label whose grid enumerates
// the full set of privileges grantable on the tenant — the catalog closure.
// Accounts and groups spell it identically; TestPrivilegeSetVocabulariesAgree
// pins that, since one constant serves both lookups.
const administratorPrivilegeSet = proclassic.AccountPrivilegeSetAdministrator

// Discoverer is the minimal classic client surface needed to discover the
// tenant's privilege catalog. *proclassic.Client satisfies it; tests supply a
// fake.
type Discoverer interface {
	ListAccounts(ctx context.Context) (*proclassic.Accounts, error)
	GetAccountGroupByID(ctx context.Context, id string) (*proclassic.Group, error)
	GetAccountByUserID(ctx context.Context, id string) (*proclassic.Account, error)
}

// Catalog is the set of privilege strings grantable on the tenant, discovered
// from an Administrator account or group. Membership is flat (union across
// categories): a privilege absent from an Administrator's grid is not grantable
// on this edition, so rejecting it at plan time is correct. Per-category
// placement is NOT enforced here — the server silently drops a misplaced
// privilege, which the provider's intersect-on-read degrades to a soft diff.
type Catalog struct {
	valid map[string]struct{}
}

// Contains reports whether priv is a grantable privilege on the tenant.
func (c *Catalog) Contains(priv string) bool {
	if c == nil {
		return false
	}
	_, ok := c.valid[priv]
	return ok
}

// Size returns the number of distinct grantable privileges discovered.
func (c *Catalog) Size() int {
	if c == nil {
		return 0
	}
	return len(c.valid)
}

// All returns the grantable privileges (unordered) — used for fuzzy
// "did you mean" suggestions.
func (c *Catalog) All() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.valid))
	for p := range c.valid {
		out = append(out, p)
	}
	return out
}

func catalogFromMap(m map[string][]string) *Catalog {
	cat := &Catalog{valid: make(map[string]struct{})}
	for _, privs := range m {
		for _, p := range privs {
			cat.valid[p] = struct{}{}
		}
	}
	return cat
}

// DiscoverCategorized returns the tenant privilege catalog as the categorised
// 7-bucket map (wire keys), sourced from an Administrator group (preferred) or
// account. Unlike Discover (which flattens to a membership set for validation),
// this preserves category placement for the account_privileges data source.
func DiscoverCategorized(ctx context.Context, d Discoverer) (map[string][]string, error) {
	if d == nil {
		return nil, fmt.Errorf("no classic client available")
	}
	accounts, err := d.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing accounts and groups: %w", err)
	}
	if accounts != nil && accounts.Groups != nil && accounts.Groups.Group != nil {
		for _, g := range *accounts.Groups.Group {
			if g.ID == nil {
				continue
			}
			group, err := d.GetAccountGroupByID(ctx, fmt.Sprint(*g.ID))
			if err != nil || group == nil {
				continue
			}
			if group.PrivilegeSet != nil && *group.PrivilegeSet == administratorPrivilegeSet {
				m := FromGroupPrivileges(group.Privileges)
				if len(m) > 0 {
					return m, nil
				}
			}
		}
	}
	if accounts != nil && accounts.Users != nil && accounts.Users.User != nil {
		for _, u := range *accounts.Users.User {
			if u.ID == nil {
				continue
			}
			acct, err := d.GetAccountByUserID(ctx, fmt.Sprint(*u.ID))
			if err != nil || acct == nil {
				continue
			}
			if acct.PrivilegeSet != nil && *acct.PrivilegeSet == administratorPrivilegeSet {
				m := FromAccountPrivileges(acct.Privileges)
				if len(m) > 0 {
					return m, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no Administrator account or group found to source the privilege catalog")
}

// Discover builds the tenant privilege catalog by reading an Administrator
// account group (preferred — few groups, full categorised grid) or, failing
// that, an Administrator account. Returns an error if no Administrator
// account/group can be found or read; callers should surface that as a loud
// warning (apply-time validation by the server still applies) rather than
// silently skipping. Every tenant has at least one Administrator, so this
// normally succeeds.
func Discover(ctx context.Context, d Discoverer) (*Catalog, error) {
	if d == nil {
		return nil, fmt.Errorf("no classic client available")
	}
	accounts, err := d.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing accounts and groups: %w", err)
	}

	// Prefer an Administrator group: groups are few and return the full
	// categorised grid.
	if accounts != nil && accounts.Groups != nil && accounts.Groups.Group != nil {
		for _, g := range *accounts.Groups.Group {
			if g.ID == nil {
				continue
			}
			group, err := d.GetAccountGroupByID(ctx, fmt.Sprint(*g.ID))
			if err != nil || group == nil {
				continue
			}
			if group.PrivilegeSet != nil && *group.PrivilegeSet == administratorPrivilegeSet {
				cat := catalogFromMap(FromGroupPrivileges(group.Privileges))
				if cat.Size() > 0 {
					return cat, nil
				}
			}
		}
	}

	// Fall back to an Administrator account.
	if accounts != nil && accounts.Users != nil && accounts.Users.User != nil {
		for _, u := range *accounts.Users.User {
			if u.ID == nil {
				continue
			}
			acct, err := d.GetAccountByUserID(ctx, fmt.Sprint(*u.ID))
			if err != nil || acct == nil {
				continue
			}
			if acct.PrivilegeSet != nil && *acct.PrivilegeSet == administratorPrivilegeSet {
				cat := catalogFromMap(FromAccountPrivileges(acct.Privileges))
				if cat.Size() > 0 {
					return cat, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no Administrator account or group found to source the privilege catalog")
}
