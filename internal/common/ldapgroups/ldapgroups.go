// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ldapgroups resolves and validates Jamf Pro directory-service (LDAP /
// cloud-IdP) groups by name. It wraps the `/v1/ldap/groups?q=` search so
// resources can accept a human-friendly group name and resolve the directory's
// canonical group identifier — mirroring the admin UI's "Resolve" action.
//
// Two resource families consume it: the User-Initiated Enrollment Access Groups
// (resolving directory_service_group_id from name) and the scope-bearing
// resources' directory-service user-group preflight. The search is a CONTAINS
// match, so callers must filter for an exact name; this package does that and
// also disambiguates same-named groups across servers via the LDAP server id.
package ldapgroups

import (
	"context"
	"errors"
	"fmt"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// Searcher is the subset of *pro.Client this package needs. Declaring it as an
// interface keeps the package unit-testable without a live client.
type Searcher interface {
	SearchLdapGroupsV1(ctx context.Context, q string) (*pro.LdapGroupSearchResults, error)
}

// Group is a resolved directory-service group identity.
type Group struct {
	// ID is the directory's canonical group identifier (the value Jamf Pro
	// stores as groupId). For an on-prem AD server this is a numeric id; the
	// distinguished name is carried separately.
	ID                string
	Name              string
	LdapServerID      int
	DistinguishedName string
	UUID              string
}

// ErrGroupNotFound is returned when no directory group matches the name on the
// given server. ErrAmbiguousGroup is returned when more than one does (callers
// should surface it as a configuration error telling the user to disambiguate).
var (
	ErrGroupNotFound  = errors.New("directory group not found")
	ErrAmbiguousGroup = errors.New("directory group name is ambiguous")
)

// Resolve looks up a directory group by EXACT name on a specific LDAP server.
//
// The underlying search is a contains-match, so Resolve filters the results to
// an exact, case-sensitive Name equality scoped to ldapServerID. It returns:
//   - the single matching Group, or
//   - ErrGroupNotFound (wrapped) when nothing matches, or
//   - ErrAmbiguousGroup (wrapped) when several do.
func Resolve(ctx context.Context, c Searcher, name string, ldapServerID int) (*Group, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: empty group name", ErrGroupNotFound)
	}
	res, err := c.SearchLdapGroupsV1(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("searching directory groups for %q: %w", name, err)
	}

	var matches []pro.LdapGroup
	if res != nil {
		for _, g := range res.Results {
			if g.Name == name && g.LdapServerID == ldapServerID {
				matches = append(matches, g)
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("%w: %q on LDAP server %d", ErrGroupNotFound, name, ldapServerID)
	case 1:
		g := matches[0]
		return &Group{
			ID:                g.ID,
			Name:              g.Name,
			LdapServerID:      g.LdapServerID,
			DistinguishedName: g.DistinguishedName,
			UUID:              g.UUID,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q matches %d groups on LDAP server %d", ErrAmbiguousGroup, name, len(matches), ldapServerID)
	}
}

// ResolveByName looks up every directory group whose name EXACTLY matches the
// given name, across all LDAP servers. Unlike Resolve it is server-agnostic:
// it serves the classic scope surface, where directory_service_user_group_names
// carry a name only (no server id), so a name that exists on any configured
// server is valid.
//
// Returns the (possibly empty) slice of exact-name matches, or an error only
// when the underlying search fails. An empty result is not an error — callers
// decide whether "no match" is a problem.
func ResolveByName(ctx context.Context, c Searcher, name string) ([]Group, error) {
	if name == "" {
		return nil, nil
	}
	res, err := c.SearchLdapGroupsV1(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("searching directory groups for %q: %w", name, err)
	}
	var matches []Group
	if res != nil {
		for _, g := range res.Results {
			if g.Name == name {
				matches = append(matches, Group{
					ID:                g.ID,
					Name:              g.Name,
					LdapServerID:      g.LdapServerID,
					DistinguishedName: g.DistinguishedName,
					UUID:              g.UUID,
				})
			}
		}
	}
	return matches, nil
}

// Validate confirms a (name, ldapServerID, groupID) triple refers to a real
// directory group. It resolves by name+server and checks the resolved id equals
// the supplied groupID. Returns nil when consistent, ErrGroupNotFound /
// ErrAmbiguousGroup as Resolve does, or a mismatch error when the group exists
// but its canonical id differs from groupID.
func Validate(ctx context.Context, c Searcher, name string, ldapServerID int, groupID string) error {
	g, err := Resolve(ctx, c, name, ldapServerID)
	if err != nil {
		return err
	}
	if g.ID != groupID {
		return fmt.Errorf("directory group %q on LDAP server %d has id %q, not %q", name, ldapServerID, g.ID, groupID)
	}
	return nil
}
