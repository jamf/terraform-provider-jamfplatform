// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Jamf-group "member of" criteria reference a Jamf group (computer / mobile-device
// / user group — NOT a directory-service group; that family lives in dsgroup.go)
// by NAME as their criterion VALUE. Jamf Pro 11.29 changed the read behaviour on
// two surfaces: a value authored by name comes back as the group's numeric ID,
// where it previously round-tripped the name. Left unmapped this trips "Provider
// produced inconsistent result after apply" and then a perpetual name-vs-id diff.
//
// The regression (PI-1394) is a WINDOW: 11.30.1 restored the pre-11.29 behaviour
// (the endpoint resolves the group name itself and echoes it back), wire-probed
// live — see GroupRefWorkaroundApplies. So the workaround engages only for
// [11.29.0, 11.30.1); 11.30.1+ and pre-11.29 pass the name through directly.
//
// This file maps the wire value back to the authored group name on read. The
// mapping is a FALLBACK, never a blind "the wire is an id": when the wire already
// equals the configured value it is kept verbatim with no lookup, so the read path
// serves pre-11.29 Jamf Pro (returns the name), the 11.29 window (returns the id),
// and 11.30.1+ (returns the name again) with no version branch — it simply
// short-circuits when the server echoes the name. See
// spike/JAMF_GROUP_MEMBER_OF_CRITERIA_SPIKE.md for the per-surface wire-probe
// matrix. Only two (surface, criterion) pairs regress and are wired:
// user_group/"User Group" and device_group/"Computer Group" (the COMPUTER device
// type — MOBILE is clean but shares device_group's code path, where wire==config
// holds so the resolver never fires). The three advanced searches are confirmed
// clean and intentionally NOT wired; if a future Jamf release flips one, the
// criterion name is already mapped per object class in jamfGroupCriterionName.
//
// The WRITE differs by surface. On classic /usergroups (user_group) the server
// accepts the group name verbatim and stores the id itself, so the write is a pure
// pass-through and the workaround only builds an id->name restore map for the
// post-create/update read-back within the window. The Platform /device-groups
// endpoint (device_group) instead REQUIRED the numeric id inside the window (PATCH
// rejected the name), so there the write resolves name->id — see
// resolveGroupRefWireIDs. Both write-side resolves are gated on
// GroupRefWorkaroundApplies so 11.30.1+ sends the name directly. The read-side
// reverse-resolve and the ModifyPlan no-op suppression (for a user who pastes the
// equivalent id) stay version-agnostic — they are soft no-ops when the server
// echoes the name.

// GroupRefRegressionVersion is the first Jamf Pro version to echo a Jamf-group
// member-of value back as the numeric group id (the 11.29 nested-smart-group
// rework regression — PI-1394; JPRO-20813 / JPRO-20814).
const GroupRefRegressionVersion = "11.29.0"

// GroupRefFixedVersion is the Jamf Pro version that resolved PI-1394 and restored
// the pre-11.29 behaviour: the group name round-trips again on both classic
// /usergroups and the Platform /device-groups endpoint (wire-probed live on 11.30.1
// — POST/PATCH by name round-trip the name; the numeric id is also accepted and
// normalised back to the name on read).
const GroupRefFixedVersion = "11.30.1"

// GroupRefWorkaroundApplies reports whether the Jamf-group member-of name<->id
// write workaround should engage for a tenant reporting version. True only inside
// the regressed window [GroupRefRegressionVersion, GroupRefFixedVersion): 11.30.1+
// and pre-11.29 pass the authored name through directly. FAIL-OPEN via
// helpers.JamfProVersionInRange (unknown/unparseable version -> true): the numeric
// id is accepted across the whole supported range (the fixed endpoint normalises it
// back to the name), so keeping the workaround engaged on an unknown version is
// safe, whereas sending the name would fail inside the window.
func GroupRefWorkaroundApplies(version string) bool {
	return helpers.JamfProVersionInRange(version, GroupRefRegressionVersion, GroupRefFixedVersion)
}

// GroupResolver maps a Jamf group between its name and numeric id for the
// ObjectType-appropriate collection (computer / mobile-device / user groups).
// Defined as an interface so the value-level helpers are unit-testable without a
// live client; NewProGroupResolver is the production proclassic-backed adapter.
type GroupResolver interface {
	// NameByID returns the canonical group name for a numeric id, or an error if
	// the id is non-numeric, not found, or the lookup fails. Read callers treat
	// any error as "cannot map" and surface the wire value as drift.
	NameByID(ctx context.Context, ot ObjectType, id string) (string, error)
	// IDByName returns the numeric id for an exact group name. Used only by
	// plan-time equivalence (recognising a name<->id no-op swap).
	IDByName(ctx context.Context, ot ObjectType, name string) (string, error)
}

// jamfGroupCriterionName is the per-object-class group-membership criterion name
// (1:1 with a group collection). A criterion with this name AND a member-of
// operator carries a group reference whose value Jamf Pro 11.29 may echo as an id.
var jamfGroupCriterionName = map[ObjectType]string{
	ObjectTypeComputer: "Computer Group",
	ObjectTypeMobile:   "Mobile Device Group",
	ObjectTypeUser:     "User Group",
}

// isMemberOfOperator reports whether searchType is the membership operator family.
// Case-insensitive: the shared CriterionModel stores the lowercase classic form
// ("member of"); device_group's own model also lowercases on flatten, but the
// Platform wire is UPPERCASE — EqualFold covers every surface.
func isMemberOfOperator(searchType string) bool {
	return strings.EqualFold(searchType, "member of") || strings.EqualFold(searchType, "not member of")
}

// IsJamfGroupCriterion reports whether a criterion is a Jamf-group membership
// reference for the given object class — its name matches the class's group
// criterion AND the operator is member-of / not-member-of. Such a criterion's
// value is a group name that Jamf Pro 11.29 reads back as a numeric id.
func IsJamfGroupCriterion(ot ObjectType, name, searchType string) bool {
	want, ok := jamfGroupCriterionName[ot]
	return ok && name == want && isMemberOfOperator(searchType)
}

// ReadGroupValue maps a Jamf-group criterion's wire value back to the authored
// group name. Version-agnostic and SOFT — any lookup failure surfaces the wire
// value as drift, never an error (a single server-side change must not break every
// subsequent `terraform plan`):
//
//   - prior == "" (import / data source) -> reverse-resolve wire id->name; fall
//     back to wire on failure (pre-11.29 wire is already a name, so resolving it
//     as an id fails and the name passes through unchanged).
//   - wire == prior (case-insensitive) -> keep prior. Covers a pre-11.29 name
//     round-trip and the steady state where state already holds the name; NO
//     lookup, so it works even without the group-read privilege. Case-preserving
//     (keeps the authored spelling) to suppress canonicalisation churn.
//   - wire != prior -> reverse-resolve wire as an id; if it names the prior group,
//     keep the prior (authored) value; else surface wire (genuine drift: the group
//     was renamed/changed, or the value is unresolvable).
func ReadGroupValue(ctx context.Context, resolver GroupResolver, ot ObjectType, wire, prior string) string {
	if prior == "" {
		if name, err := resolver.NameByID(ctx, ot, wire); err == nil && name != "" {
			return name
		}
		return wire
	}
	if strings.EqualFold(wire, prior) {
		return prior
	}
	if name, err := resolver.NameByID(ctx, ot, wire); err == nil && strings.EqualFold(name, prior) {
		return prior
	}
	return wire
}

// GroupValuesEquivalent reports whether two Jamf-group criterion values reference
// the same group — equal (case-insensitively), or one is the numeric id of the
// other. SOFT: any lookup failure -> false (show the diff, never block plan). Used
// by ModifyPlan to suppress a no-op name<->id swap.
func GroupValuesEquivalent(ctx context.Context, resolver GroupResolver, ot ObjectType, a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	ka := canonicalGroupID(ctx, resolver, ot, a)
	kb := canonicalGroupID(ctx, resolver, ot, b)
	return ka != "" && ka == kb
}

// canonicalGroupID reduces a value (name or id) to its numeric group id, or "" if
// it cannot be resolved. Tries name->id first (the common authored form); falls
// back to treating the value as an id validated via id->name.
//
// Precedence is NAME-first, and deliberately so: an ambiguous numeric value (e.g.
// "25", when a group is literally NAMED "25" and a different group has ID 25)
// resolves to the group with that NAME. The author-by-name model is the contract,
// so a group is always reachable by its name; a raw numeric only reaches a group
// named that number, or — if none is — is treated as a literal id. Every helper
// here (canonicalGroupID, the device_group write resolver) uses this same
// name-first order so write, read, and plan-time equivalence agree.
func canonicalGroupID(ctx context.Context, resolver GroupResolver, ot ObjectType, v string) string {
	if id, err := resolver.IDByName(ctx, ot, v); err == nil && id != "" {
		return id
	}
	if _, err := resolver.NameByID(ctx, ot, v); err == nil {
		return v
	}
	return ""
}

// ---- CriterionModel layer (user_group + the three advanced searches) ---------
//
// These wire the value-level helpers into the []CriterionModel path. device_group
// has its own criterion model and calls the value-level helpers directly (see
// internal/resources/device_group/groupref.go).

// ReadbackGroupRefCriteria maps each Jamf-group criterion's wire value back to the
// authored group name. Call in Read with prior=state, and after the post-write
// flatten in Create/Update with prior=plan. wireModels are the freshly flattened
// criteria; prior supplies the authored form, zipped by index + guarded by a name
// match (import / data source / reorder -> no aligned prior -> wire passes the
// import branch of ReadGroupValue). SOFT; preserves nil/empty.
func ReadbackGroupRefCriteria(ctx context.Context, resolver GroupResolver, ot ObjectType, wireModels, prior []CriterionModel) []CriterionModel {
	if len(wireModels) == 0 || resolver == nil {
		return wireModels // preserve nil/empty; never flip nil -> []
	}
	out := make([]CriterionModel, len(wireModels))
	copy(out, wireModels)
	for i := range out {
		name := out[i].Name.ValueString()
		if !IsJamfGroupCriterion(ot, name, out[i].SearchType.ValueString()) {
			continue
		}
		p := ""
		if i < len(prior) && prior[i].Name.ValueString() == name {
			p = prior[i].Value.ValueString()
		}
		out[i].Value = types.StringValue(ReadGroupValue(ctx, resolver, ot, out[i].Value.ValueString(), p))
	}
	return out
}

// ResolveAuthoredGroupRefMap (Create/Update, BEFORE the write) resolves each
// Jamf-group criterion's authored value to the numeric id a 11.29+ server echoes
// on read, returning a map id->authoredValue. RestoreAuthoredGroupRefCriteria uses
// it after the post-write flatten to put the authored name back independent of any
// server-side criteria reorder — mirroring dsgroup's byte-stable authored map,
// which keys by the base64 wire value. Best-effort and SOFT: an unresolvable
// criterion is omitted. A pre-11.29 server echoes the name (not an id), so the map
// simply misses on read-back and the authored name passes through unchanged —
// correct on both server versions with no version branch.
func ResolveAuthoredGroupRefMap(ctx context.Context, resolver GroupResolver, ot ObjectType, models []CriterionModel) map[string]string {
	out := map[string]string{}
	if resolver == nil {
		return out
	}
	for i := range models {
		name := models[i].Name.ValueString()
		if !IsJamfGroupCriterion(ot, name, models[i].SearchType.ValueString()) {
			continue
		}
		v := models[i].Value.ValueString()
		if id, err := resolver.IDByName(ctx, ot, v); err == nil && id != "" {
			out[id] = v // authored as a name -> the server stores this id
		} else if _, err := resolver.NameByID(ctx, ot, v); err == nil {
			out[v] = v // authored as an id already -> the server echoes it verbatim
		}
	}
	return out
}

// RestoreAuthoredGroupRefCriteria rewrites each Jamf-group criterion's flattened
// wire value back to the authored value using the map from
// ResolveAuthoredGroupRefMap. The wire id is by definition the id of the group we
// just wrote by name, so this restore is unconditional (no resolver call, no
// post-apply inconsistency risk). A criterion whose wire value is absent from the
// map (pre-11.29 name echo, or an authored value that did not resolve) is left as
// flattened. Pure; call in Create/Update after the flatten. Preserves nil/empty.
func RestoreAuthoredGroupRefCriteria(flattened []CriterionModel, authored map[string]string, ot ObjectType) []CriterionModel {
	if len(authored) == 0 || len(flattened) == 0 {
		return flattened
	}
	out := make([]CriterionModel, len(flattened))
	copy(out, flattened)
	for i := range out {
		if !IsJamfGroupCriterion(ot, out[i].Name.ValueString(), out[i].SearchType.ValueString()) {
			continue
		}
		if v, ok := authored[out[i].Value.ValueString()]; ok {
			out[i].Value = types.StringValue(v)
		}
	}
	return out
}

// SuppressEquivalentGroupRefValues resets a planned Jamf-group criterion value to
// the prior state value when both reference the same group (a name<->id swap),
// suppressing a no-op diff. Aligned by index + name match; SOFT. Reconciles
// siblings the plan left Unknown via reconcileCriterionToPrior (shared with
// dsgroup). Call from ModifyPlan. Preserves nil/empty.
func SuppressEquivalentGroupRefValues(ctx context.Context, resolver GroupResolver, ot ObjectType, planned, prior []CriterionModel) []CriterionModel {
	if len(planned) == 0 || resolver == nil {
		return planned // preserve nil/empty
	}
	out := make([]CriterionModel, len(planned))
	copy(out, planned)
	for i := range out {
		name := out[i].Name.ValueString()
		if !IsJamfGroupCriterion(ot, name, out[i].SearchType.ValueString()) {
			continue
		}
		if i >= len(prior) || prior[i].Name.ValueString() != name {
			continue
		}
		if GroupValuesEquivalent(ctx, resolver, ot, out[i].Value.ValueString(), prior[i].Value.ValueString()) {
			out[i] = reconcileCriterionToPrior(out[i], prior[i])
		}
	}
	return out
}

// ---- proclassic-backed resolver --------------------------------------------

// proGroupResolver implements GroupResolver against the proclassic SDK client. The
// classic group endpoints back every surface: device_group-COMPUTER membership
// stores CLASSIC computer-group ids (wire-probed), so even the Platform device
// group resolves its "Computer Group" reference here, not via Pro /v2/groups.
type proGroupResolver struct{ c *proclassic.Client }

// NewProGroupResolver returns the production GroupResolver. Construct it per
// resource from the shared SDK client: criteria.NewProGroupResolver(proclassic.New(pd.Client)).
func NewProGroupResolver(c *proclassic.Client) GroupResolver { return &proGroupResolver{c: c} }

func (r *proGroupResolver) NameByID(ctx context.Context, ot ObjectType, id string) (string, error) {
	switch ot {
	case ObjectTypeMobile:
		g, err := r.c.GetMobileDeviceGroupByID(ctx, id)
		if err != nil {
			return "", err
		}
		if g.Name == nil {
			return "", fmt.Errorf("mobile device group id %q has no name", id)
		}
		return *g.Name, nil
	case ObjectTypeUser:
		g, err := r.c.GetUserGroupByID(ctx, id)
		if err != nil {
			return "", err
		}
		if g.Name == nil {
			return "", fmt.Errorf("user group id %q has no name", id)
		}
		return *g.Name, nil
	default: // ObjectTypeComputer
		g, err := r.c.GetComputerGroupByID(ctx, id)
		if err != nil {
			return "", err
		}
		if g.Name == nil {
			return "", fmt.Errorf("computer group id %q has no name", id)
		}
		return *g.Name, nil
	}
}

func (r *proGroupResolver) IDByName(ctx context.Context, ot ObjectType, name string) (string, error) {
	switch ot {
	case ObjectTypeMobile:
		return r.c.ResolveMobileDeviceGroupIDByName(ctx, name)
	case ObjectTypeUser:
		return r.c.ResolveUserGroupIDByName(ctx, name)
	default: // ObjectTypeComputer
		return r.c.ResolveComputerGroupIDByName(ctx, name)
	}
}
