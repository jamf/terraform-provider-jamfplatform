// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// errTitleNotInCatalog reports that the App Catalog holds no title whose display
// name equals the requested one exactly. It is the resolver's own verdict rather
// than a server status, because Jamf Pro answers a name that matches nothing with
// an empty list and a 200.
var errTitleNotInCatalog = errors.New("no App Catalog title has that exact name")

// titleCatalog is the App Catalog title snapshot the name resolvers read.
// Declaring it as an interface keeps the resolvers unit-testable without a live
// client, and keeps them ignorant of where the snapshot came from — in production
// it is the provider-instance cache in internal/providerdata, read once per
// terraform invocation rather than once per resource instance.
type titleCatalog interface {
	Titles(ctx context.Context) ([]pro.AppTitle, error)
}

// catalogOrNil converts a possibly-nil cache pointer into a titleCatalog value,
// returning a nil interface rather than a non-nil interface holding a nil pointer.
//
// The resolvers read a nil catalog as "the provider is not configured yet, do
// nothing": validateAppTitleName returns no diagnostics and titleNameForID returns
// ok=false. A typed nil pointer would satisfy the interface, defeat that check and
// turn the framework's early-lifecycle Configure — which carries a nil
// ProviderData and so yields no cache — into a spurious plan-time warning.
func catalogOrNil(c *providerdata.AppTitleCatalogCache) titleCatalog {
	if c == nil {
		return nil
	}
	return c
}

// readAppTitleCatalog fetches the whole App Catalog title list, and is the read
// function every App Installer construct registers with the provider-instance
// cache.
//
// The filter is empty deliberately: the resolvers decide every match locally on
// byte equality, so a server-side filter would buy nothing and would tie the
// snapshot to one name. The catalog is a few hundred titles against an SDK page
// size of 2000, so it arrives whole in one round-trip.
func readAppTitleCatalog(ctx context.Context, client *pro.Client) ([]pro.AppTitle, error) {
	return client.ListAppInstallerTitlesV1(ctx, nil, "")
}

// resolveTitleIDByName resolves an App Catalog title display name to its catalog
// ID, matching the name EXACTLY.
//
// Jamf Pro's own `titleName` filter cannot be trusted to decide the match: it is a
// case-insensitive glob, so `titleName=="jamf composer"` and `titleName=="Jamf*"`
// both match "Jamf Composer" (wire-verified). Accepting the server's verdict would
// store the user's spelling, then reverse-resolve it to the canonical name on the
// next Read and produce a perpetual diff — the trap
// STYLE_GUIDE §"Referencing a server-managed catalog by name" warns about. So the
// match is decided here on byte equality, over the whole catalog: the candidate set
// is the cached full-catalog snapshot rather than a per-name filtered request, so
// no server-side filter is involved at all and every instance in a configuration
// shares one read.
//
// A name that matches nothing, or matches only case-insensitively, returns
// errTitleNotInCatalog; two titles sharing one exact name return an ambiguity
// error rather than an arbitrary pick.
func resolveTitleIDByName(ctx context.Context, catalog titleCatalog, name string) (string, error) {
	if catalog == nil {
		return "", errors.New("no App Catalog client configured")
	}
	if name == "" {
		return "", errTitleNotInCatalog
	}

	candidates, err := catalog.Titles(ctx)
	if err != nil {
		return "", err
	}

	var matches []pro.AppTitle
	for _, t := range candidates {
		if t.TitleName == name {
			matches = append(matches, t)
		}
	}

	switch len(matches) {
	case 0:
		return "", errTitleNotInCatalog
	case 1:
		return matches[0].ID, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return "", fmt.Errorf("the App Catalog holds %d titles named %q (IDs %s); reference one by ID instead", len(matches), name, strings.Join(ids, ", "))
	}
}

// errDeploymentNotFound reports that no deployment carries the requested name
// exactly. Like the title resolver, this is the resolver's own verdict: Jamf Pro
// answers a name that matches nothing with an empty list and a 200.
var errDeploymentNotFound = errors.New("no App Installer deployment has that exact name")

// deploymentLister is the subset of *pro.Client the deployment name resolver
// uses.
type deploymentLister interface {
	ListAppInstallerDeploymentsV1(ctx context.Context, sort []string, filter string) ([]pro.AppTitleDeploymentSummary, error)
}

// resolveDeploymentIDByName resolves a deployment name to its ID, matching the
// name EXACTLY for the same reason as resolveTitleIDByName: Jamf Pro's `name`
// filter is a case-insensitive glob, so the filter narrows the request and the
// match is decided here on byte equality. Two deployments may legitimately share
// a name, which is an ambiguity error rather than an arbitrary pick.
func resolveDeploymentIDByName(ctx context.Context, lister deploymentLister, name string) (string, error) {
	if lister == nil {
		return "", errors.New("no App Installer client configured")
	}
	if name == "" {
		return "", errDeploymentNotFound
	}

	candidates, err := lister.ListAppInstallerDeploymentsV1(ctx, nil, deploymentNameFilter(name))
	if err != nil {
		return "", err
	}

	var ids []string
	for _, d := range candidates {
		if d.Name == name {
			ids = append(ids, d.ID)
		}
	}

	switch len(ids) {
	case 0:
		return "", errDeploymentNotFound
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("%d App Installer deployments are named %q (IDs %s); look one up by id instead", len(ids), name, strings.Join(ids, ", "))
	}
}

// deploymentNameFilter renders an RSQL equality filter over a deployment name.
func deploymentNameFilter(name string) string {
	return fmt.Sprintf(`name=="%s"`, escapeRSQLString(name))
}

// escapeRSQLString escapes a value for an RSQL double-quoted string, so a
// backslash or double quote in a name cannot break the query. An asterisk is
// left alone: Jamf Pro reads it as a glob, and the deployment resolver decides
// the match on exact equality anyway, so a widened candidate set is harmless.
func escapeRSQLString(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}
