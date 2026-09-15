// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// fetchPatchSourceCatalogues reads the tenant's internal and external patch source
// catalogues into one snapshot.
//
// This is the only place the two reads happen. It backs the provider-instance cache
// (providerdata.ConfigurePatchSources, used by the data source and by list hydration) and
// the uncached resolveSourceID the resource's import path calls, so a change to how the
// catalogues are gathered lands in both.
//
// Both catalogues are always read, even when the first already matched: a name present in
// both is ambiguous, and only reading both can establish that. They are small — a tenant
// has one internal source and a handful of external ones at most.
func fetchPatchSourceCatalogues(ctx context.Context, c *proclassic.Client) (providerdata.PatchSourceCatalogues, error) {
	var out providerdata.PatchSourceCatalogues
	if c == nil {
		return out, errors.New("the Jamf Pro client is not configured")
	}

	internal, err := c.ListPatchInternalSources(ctx)
	if err != nil {
		return out, fmt.Errorf("listing internal patch sources: %w", err)
	}
	if internal != nil {
		out.Internal = internal.PatchInternalSources
	}

	external, err := c.ListPatchExternalSources(ctx)
	if err != nil {
		return out, fmt.Errorf("listing external patch sources: %w", err)
	}
	if external != nil {
		out.External = external.PatchExternalSources
	}
	return out, nil
}

// sourceIDFromCatalogues resolves a patch source's numeric id from the name a title
// reports, deciding over an already-read snapshot.
//
// The configurations endpoint names a title's patch source but never numbers it, while
// source_id is what defines a title on create and therefore what the schema exposes. The
// number is only ever needed where there is no prior state to carry it — an import, a
// data source read, or list hydration.
//
// This is the one place the resolution law lives: every caller decides here, so the
// ambiguous arm cannot mean "error" to one construct and "silently absent" to another. A
// name matching in both catalogues is refused rather than guessed, since the two id
// spaces are separate and a wrong number would put a destroy in the next plan of a
// resource whose source_id forces replacement.
//
// Deciding is separate from fetching so the arms are testable without a client, and the
// walk is linear rather than indexed because both catalogues are single-digit sized.
func sourceIDFromCatalogues(catalogues providerdata.PatchSourceCatalogues, name string) (types.Int64, error) {
	if name == "" {
		return types.Int64Null(), errors.New("this title reports no patch source name at all")
	}

	matches := matchSourceIDs(catalogues.Internal, name)
	matches = append(matches, matchSourceIDs(catalogues.External, name)...)

	switch len(matches) {
	case 0:
		return types.Int64Null(), fmt.Errorf("no patch source named %q in this tenant's internal or external patch sources", name)
	case 1:
		return types.Int64Value(int64(matches[0])), nil
	default:
		return types.Int64Null(), fmt.Errorf("patch source name %q is not unique across this tenant's internal and external patch sources (ids %v); rename one of them in Jamf Pro so the two names differ, then run again", name, matches)
	}
}

// resolveSourceID reads the catalogues straight from the tenant and resolves name
// against them, without the shared cache.
//
// The resource's import path is the caller: it holds no cache and runs at most once per
// import, so it pays the two reads there rather than threading a cache through the
// resource. Every other caller resolves from the cached snapshot through
// sourceIDFromCatalogues.
func resolveSourceID(ctx context.Context, c *proclassic.Client, name string) (types.Int64, error) {
	catalogues, err := fetchPatchSourceCatalogues(ctx, c)
	if err != nil {
		return types.Int64Null(), fmt.Errorf("reading the patch source catalogues while resolving %q: %w", name, err)
	}
	return sourceIDFromCatalogues(catalogues, name)
}

// matchSourceIDs returns the ids of every catalogue entry whose name matches
// exactly.
func matchSourceIDs(sources []proclassic.IDName, name string) []int {
	var out []int
	for i := range sources {
		if sources[i].Name == nil || *sources[i].Name != name {
			continue
		}
		if sources[i].ID == nil {
			continue
		}
		out = append(out, *sources[i].ID)
	}
	return out
}

// unresolvedSourceIDDetail is the shared diagnostic detail for a title whose patch source
// name did not resolve to exactly one id.
//
// An ambiguous name, a name matching nothing, and an unreadable catalogue all land here,
// so every call site says the same thing about the same failure and points a practitioner
// at the privileges the resolution needs without each construct restating them. A listing
// that cannot read the catalogues at all uses unreadableCataloguesWarningDetail instead,
// because that condition is not per title.
func unresolvedSourceIDDetail(title, sourceName string, err error) string {
	return fmt.Sprintf(
		"Jamf Pro reports the patch source for %q as %q, but the provider could not match that name to a single patch source ID: %v. "+
			"Resolving the name reads this tenant's internal and external patch source catalogues, so it also fails when the API integration does not hold "+
			"Internal patch sources: Read and External patch sources: Read.",
		title, sourceName, err,
	)
}

// unresolvedSourceIDWarningDetail is unresolvedSourceIDDetail plus what the caller did
// about it, for the two constructs where source_id is Computed and informational — the
// data source and a list preview. The managed resource does not use it: there source_id
// defines the title and forces replacement, so an unresolved name on import is fatal
// rather than null.
func unresolvedSourceIDWarningDetail(title, sourceName string, err error) string {
	return unresolvedSourceIDDetail(title, sourceName, err) + " source_id is null in this result; every other attribute is unaffected."
}

// unreadableCataloguesWarningDetail is the diagnostic detail for a listing whose patch
// source catalogues could not be read at all.
//
// It is separate from the per-title wording because the condition is list-wide: one
// unreadable catalogue nulls source_id for every title in the result, so a listing says
// so once instead of repeating a privilege problem beside each of a hundred titles.
func unreadableCataloguesWarningDetail(err error) string {
	return fmt.Sprintf(
		"source_id is null for every title in this result because the tenant's internal and external patch sources could not be read: %v. "+
			"Reading them requires the API integration to hold Internal patch sources: Read and External patch sources: Read. Every other attribute is unaffected.",
		err,
	)
}
