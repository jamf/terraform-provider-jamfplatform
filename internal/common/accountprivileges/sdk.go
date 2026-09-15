// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package accountprivileges

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// strSlicePtr returns a pointer to a copy of vals, or nil for an empty slice so
// an empty category marshals as <category/> rather than carrying stray entries.
func strSlicePtr(vals []string) *[]string {
	cp := append([]string(nil), vals...)
	return &cp
}

// derefStrSlice safely dereferences an SDK *[]string privilege list.
func derefStrSlice(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

// FromAccountPrivileges flattens an SDK account privilege grid into a map keyed
// by wire category. Absent categories are omitted from the map.
func FromAccountPrivileges(p *proclassic.AccountPrivileges) map[string][]string {
	out := make(map[string][]string)
	if p == nil {
		return out
	}
	if p.JssObjects != nil {
		out["jss_objects"] = derefStrSlice(p.JssObjects.Privilege)
	}
	if p.JssSettings != nil {
		out["jss_settings"] = derefStrSlice(p.JssSettings.Privilege)
	}
	if p.JssActions != nil {
		out["jss_actions"] = derefStrSlice(p.JssActions.Privilege)
	}
	if p.CasperAdmin != nil {
		out["casper_admin"] = derefStrSlice(p.CasperAdmin.Privilege)
	}
	if p.CasperRemote != nil {
		out["casper_remote"] = derefStrSlice(p.CasperRemote.Privilege)
	}
	if p.CasperImaging != nil {
		out["casper_imaging"] = derefStrSlice(p.CasperImaging.Privilege)
	}
	if p.Recon != nil {
		out["recon"] = derefStrSlice(p.Recon.Privilege)
	}
	return out
}

// FromGroupPrivileges flattens an SDK group privilege grid into a map keyed by
// wire category.
func FromGroupPrivileges(p *proclassic.GroupPrivileges) map[string][]string {
	out := make(map[string][]string)
	if p == nil {
		return out
	}
	if p.JssObjects != nil {
		out["jss_objects"] = derefStrSlice(p.JssObjects.Privilege)
	}
	if p.JssSettings != nil {
		out["jss_settings"] = derefStrSlice(p.JssSettings.Privilege)
	}
	if p.JssActions != nil {
		out["jss_actions"] = derefStrSlice(p.JssActions.Privilege)
	}
	if p.CasperAdmin != nil {
		out["casper_admin"] = derefStrSlice(p.CasperAdmin.Privilege)
	}
	if p.CasperRemote != nil {
		out["casper_remote"] = derefStrSlice(p.CasperRemote.Privilege)
	}
	if p.CasperImaging != nil {
		out["casper_imaging"] = derefStrSlice(p.CasperImaging.Privilege)
	}
	if p.Recon != nil {
		out["recon"] = derefStrSlice(p.Recon.Privilege)
	}
	return out
}

// ToAccountPrivileges builds an SDK account privilege grid from a wire-keyed
// map. Only categories present in the map are emitted, and a category present
// with an empty slice is emitted as an empty element. Because the classic
// endpoint replaces the whole grid on any sent <privileges> (see MergeGrid), an
// absent category and an empty one both end up cleared on the server; the map
// must therefore already be the full merged grid, not just the declared
// categories.
func ToAccountPrivileges(m map[string][]string) *proclassic.AccountPrivileges {
	if len(m) == 0 {
		return nil
	}
	out := &proclassic.AccountPrivileges{}
	if v, ok := m["jss_objects"]; ok {
		out.JssObjects = &proclassic.AccountPrivilegesJssObjects{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["jss_settings"]; ok {
		out.JssSettings = &proclassic.AccountPrivilegesJssSettings{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["jss_actions"]; ok {
		out.JssActions = &proclassic.AccountPrivilegesJssActions{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["casper_admin"]; ok {
		out.CasperAdmin = &proclassic.AccountPrivilegesCasperAdmin{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["casper_remote"]; ok {
		out.CasperRemote = &proclassic.AccountPrivilegesCasperRemote{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["casper_imaging"]; ok {
		out.CasperImaging = &proclassic.AccountPrivilegesCasperImaging{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["recon"]; ok {
		out.Recon = &proclassic.AccountPrivilegesRecon{Privilege: strSlicePtr(v)}
	}
	return out
}

// ToGroupPrivileges builds an SDK group privilege grid from a wire-keyed map,
// with the same full-grid expectation as ToAccountPrivileges.
func ToGroupPrivileges(m map[string][]string) *proclassic.GroupPrivileges {
	if len(m) == 0 {
		return nil
	}
	out := &proclassic.GroupPrivileges{}
	if v, ok := m["jss_objects"]; ok {
		out.JssObjects = &proclassic.GroupPrivilegesJssObjects{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["jss_settings"]; ok {
		out.JssSettings = &proclassic.GroupPrivilegesJssSettings{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["jss_actions"]; ok {
		out.JssActions = &proclassic.GroupPrivilegesJssActions{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["casper_admin"]; ok {
		out.CasperAdmin = &proclassic.GroupPrivilegesCasperAdmin{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["casper_remote"]; ok {
		out.CasperRemote = &proclassic.GroupPrivilegesCasperRemote{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["casper_imaging"]; ok {
		out.CasperImaging = &proclassic.GroupPrivilegesCasperImaging{Privilege: strSlicePtr(v)}
	}
	if v, ok := m["recon"]; ok {
		out.Recon = &proclassic.GroupPrivilegesRecon{Privilege: strSlicePtr(v)}
	}
	return out
}

// IntersectIntoState applies intersect-on-read. For each category that is
// non-null in prior (the user-declared/state set), the returned Model carries
// declared ∩ server. Categories null in prior stay null (unmanaged), so server-
// added dependency privileges never enter state. prior may be nil (import),
// in which case every category present on the server is materialised in full
// so the imported resource reflects the live grid.
func IntersectIntoState(ctx context.Context, prior *Model, server map[string][]string) (Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	var out Model

	for _, c := range Categories {
		serverVals := server[c.WireKey]

		if prior == nil {
			// Import: materialise the live category (or null if absent).
			if _, ok := server[c.WireKey]; !ok {
				*out.setPtr(c.WireKey) = types.SetNull(types.StringType)
				continue
			}
			set, d := NewStringSet(serverVals)
			diags.Append(d...)
			*out.setPtr(c.WireKey) = set
			continue
		}

		priorSet := prior.setPtr(c.WireKey)
		if priorSet.IsNull() || priorSet.IsUnknown() {
			// Unmanaged category: stays null regardless of server contents.
			*out.setPtr(c.WireKey) = types.SetNull(types.StringType)
			continue
		}

		declared, d := declaredStrings(ctx, *priorSet)
		diags.Append(d...)
		if diags.HasError() {
			return out, diags
		}

		set, d := NewStringSet(intersect(declared, serverVals))
		diags.Append(d...)
		*out.setPtr(c.WireKey) = set
	}
	return out, diags
}

// intersect returns the elements of declared that are also present in server,
// preserving declared's membership (order is normalised by NewStringSet).
func intersect(declared, server []string) []string {
	have := make(map[string]struct{}, len(server))
	for _, s := range server {
		have[s] = struct{}{}
	}
	var out []string
	for _, d := range declared {
		if _, ok := have[d]; ok {
			out = append(out, d)
		}
	}
	return out
}

// CategorizedSets converts a wire-keyed catalog map into a per-category map of
// string Sets (keyed by wire key) plus a flat union Set of every privilege
// across all categories. Every returned Set is sorted and de-duplicated via
// NewStringSet — mandatory because the classic Administrator grid echoes some
// privilege strings more than once within a category, which types.SetValue
// would otherwise reject with a "Duplicate Set Element" error (issue #290).
// Used by the account_privileges data source to project the discovered catalog
// into state.
func CategorizedSets(catalog map[string][]string) (map[string]types.Set, types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	sets := make(map[string]types.Set, len(Categories))
	var all []string
	for _, c := range Categories {
		vals := catalog[c.WireKey]
		all = append(all, vals...)
		s, d := NewStringSet(vals)
		diags.Append(d...)
		sets[c.WireKey] = s
	}
	allSet, d := NewStringSet(all)
	diags.Append(d...)
	return sets, allSet, diags
}

// FilterToCatalog drops from every populated category any privilege the tenant's
// Administrator grid does not offer, returning the filtered model.
//
// It exists for the import path. Jamf Pro expands a preset privilege_set
// ("Auditor", "Administrator") into a full privilege list on read, and that list
// can contain privileges the same tenant will not grant — "Read Knobs" is one on
// an 11.x tenant. Adopting them verbatim writes a state (and, through
// `-generate-config-out`, a configuration) that Validate then refuses, so an
// import of a preset-set group produces a plan that cannot run. Filtering makes
// the adopted set what the tenant would actually store, which is also what the
// next Read will report.
//
// A nil catalog means discovery failed; the model is returned unchanged rather
// than silently emptied.
func FilterToCatalog(ctx context.Context, m *Model, catalog *Catalog) (Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	var out Model
	if m == nil {
		return out, diags
	}
	if catalog == nil {
		return *m, diags
	}

	for _, c := range Categories {
		sp := m.setPtr(c.WireKey)
		if sp.IsNull() || sp.IsUnknown() {
			*out.setPtr(c.WireKey) = *sp
			continue
		}
		declared, d := declaredStrings(ctx, *sp)
		diags.Append(d...)
		if diags.HasError() {
			return out, diags
		}
		kept := make([]string, 0, len(declared))
		for _, v := range declared {
			if catalog.Contains(v) {
				kept = append(kept, v)
			}
		}
		set, d := NewStringSet(kept)
		diags.Append(d...)
		*out.setPtr(c.WireKey) = set
	}
	return out, diags
}
