// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// privatePayloadWriter is the subset of *resource.PrivateState used in
// Create/Update for stashing the two payload references the three-way
// compare consumes on the next plan.
type privatePayloadWriter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// Terraform's private-state API requires every stashed value to be valid
// JSON. Mobileconfig XML is not, so wrap the bytes in a JSON string at
// write time and decode at read time.
func encodePrivatePayload(b []byte) ([]byte, error) {
	encoded, err := json.Marshal(string(b))
	if err != nil {
		return nil, fmt.Errorf("JSON-encoding payload for private state: %w", err)
	}
	return encoded, nil
}

func decodePrivatePayload(jsonValue []byte) ([]byte, error) {
	if len(jsonValue) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(jsonValue, &s); err != nil {
		return nil, fmt.Errorf("JSON-decoding payload from private state: %w", err)
	}
	return []byte(s), nil
}

// privatePayloadReader is the subset of *resource.PrivateState used in
// Read for fetching the three-way references stashed at the last Apply.
type privatePayloadReader interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

// reconcileReadDrift overwrites state.Payloads with the server-canonical
// form when an out-of-band admin UI edit is detected. See the macOS
// resource doc for the full mechanics — same wiring, same trade-offs.
func reconcileReadDrift(ctx context.Context, priv privatePayloadReader, state *ResourceModel, rawServerCanonical []byte) diag.Diagnostics {
	var diags diag.Diagnostics
	if priv == nil || state == nil || state.General == nil || len(rawServerCanonical) == 0 {
		return diags
	}
	lastCanonicalRaw, d := priv.GetKey(ctx, privateKeyLastCanonical)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	lastCanonical, err := decodePrivatePayload(lastCanonicalRaw)
	if err != nil || len(lastCanonical) == 0 {
		return diags
	}
	canonicalisedServer := plisthelpers.CanonicalisePlistXML(rawServerCanonical)
	equal, err := payloadhelpers.PayloadsStructurallyEqual(lastCanonical, canonicalisedServer)
	if err != nil {
		diags.AddWarning("Read-side drift compare failed; leaving lenient self-healing in place", err.Error())
		return diags
	}
	if equal {
		return diags
	}
	tflog.Info(ctx, "payload Read-side drift: server diverged from last-applied canonical; overwriting state.payloads with server canonical", map[string]any{
		"server_bytes":         len(canonicalisedServer),
		"last_canonical_bytes": len(lastCanonical),
	})
	state.General.Payloads = types.StringValue(string(canonicalisedServer))
	return diags
}

// writePrivatePayloadRefs stashes the user-authored input bytes and the
// raw server-canonical bytes captured immediately after the just-completed
// Create or Update.
//
// IMPORTANT: rawServerCanonical must be the bytes returned by the SDK
// GET (before any self-healing in assignResourceModel mutates
// plan.General.Payloads). See the macOS resource for the full rationale.
func writePrivatePayloadRefs(ctx context.Context, w privatePayloadWriter, userAuthoredInput string, rawServerCanonical []byte) diag.Diagnostics {
	var diags diag.Diagnostics
	if w == nil {
		return diags
	}
	if userAuthoredInput != "" {
		encoded, err := encodePrivatePayload([]byte(userAuthoredInput))
		if err != nil {
			diags.AddError("Failed to encode last-applied input for private state", helpers.APIErrorDetail(err))
			return diags
		}
		diags.Append(w.SetKey(ctx, privateKeyLastInput, encoded)...)
	}
	if len(rawServerCanonical) > 0 {
		canonicalised := plisthelpers.CanonicalisePlistXML(rawServerCanonical)
		encoded, err := encodePrivatePayload(canonicalised)
		if err != nil {
			diags.AddError("Failed to encode last-applied canonical for private state", helpers.APIErrorDetail(err))
			return diags
		}
		diags.Append(w.SetKey(ctx, privateKeyLastCanonical, encoded)...)
		diags.Append(w.SetKey(ctx, privateKeyServerNow, encoded)...)
	}
	return diags
}

// writePrivateServerNow refreshes the serverNow reference from the
// current Read's server response.
func writePrivateServerNow(ctx context.Context, w privatePayloadWriter, rawServerCanonical []byte) diag.Diagnostics {
	var diags diag.Diagnostics
	if w == nil || len(rawServerCanonical) == 0 {
		return diags
	}
	canonicalised := plisthelpers.CanonicalisePlistXML(rawServerCanonical)
	encoded, err := encodePrivatePayload(canonicalised)
	if err != nil {
		diags.AddError("Failed to encode server-now canonical for private state", helpers.APIErrorDetail(err))
		return diags
	}
	diags.Append(w.SetKey(ctx, privateKeyServerNow, encoded)...)
	return diags
}

// privateKeyLastInput stashes the user-authored mobileconfig payload from
// the most recent successful Apply.
const privateKeyLastInput = "payload_last_input"

// privateKeyLastCanonical stashes the server-canonical mobileconfig
// payload returned immediately after the most recent successful Apply.
// Frozen at apply time — never overwritten on Read.
const privateKeyLastCanonical = "payload_last_canonical"

// privateKeyServerNow stashes the server-canonical mobileconfig payload
// observed at the most recent Read. Refreshed on every Read.
const privateKeyServerNow = "payload_server_now"

// preflightScopeGroups runs the plan-time directory-service user-group
// preflight on the scope limitations/exclusions, surfacing an unknown group as
// a clear plan error instead of the apply-time 409 ("Problem matching
// limitation user group"). No-op on destroy (null plan), when LDAP is not yet
// configured, and when no scope groups are declared.
func (r *Resource) preflightScopeGroups(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Scope == nil {
		return
	}
	scopeRoot := path.Root("scope")
	if plan.Scope.Limitations != nil {
		resp.Diagnostics.Append(scope.ValidateDirectoryServiceUserGroupNames(
			ctx, r.ldapSearcher, plan.Scope.Limitations.DirectoryServiceUserGroupNames,
			scopeRoot.AtName("limitations").AtName("directory_service_user_group_names"),
		)...)
	}
	if plan.Scope.Exclusions != nil {
		resp.Diagnostics.Append(scope.ValidateDirectoryServiceUserGroupNames(
			ctx, r.ldapSearcher, plan.Scope.Exclusions.DirectoryServiceUserGroupNames,
			scopeRoot.AtName("exclusions").AtName("directory_service_user_group_names"),
		)...)
	}
}

// ModifyPlan sequences the plan-time hooks so each one sees what it needs:
// the profile name collision check and the scope directory-service user-group
// preflight first (both must cover create plans too), then the payload-diff
// decision, then the impact alert.
//
// The impact alert's position is a correctness constraint, not taste:
// healPayloadPlan can rewrite a byte-different but semantically-equal payload
// back to state, self-healing the plan to a no-op, and a diagnostic once
// appended cannot be retracted. Creates and destroys are reported immediately
// (the payload compare does not apply to them, and an object entering or
// leaving management changes what its scope receives); updates are reported
// only after the self-heal, and skipped when the healed plan equals state —
// otherwise `terraform plan` would print an impact alert directly above
// "No changes".
func (r *Resource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}

	// Scope directory-service user-group preflight runs first so it covers
	// create-plans too (the payload compare below early-returns on create). It
	// is skipped only on destroy (null plan).
	r.preflightScopeGroups(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create has no state; Delete has no plan. Either way the payload
	// compare does not apply, so the impact alert is emitted right away.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		r.reportScopeImpact(ctx, req, resp)
		return
	}

	r.healPayloadPlan(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}

	// The shared impact hook derives its change flag from req.Plan — the
	// pre-heal plan — so a plan healed to a no-op must be skipped here, where
	// the healed resp.Plan is visible.
	if resp.Plan.Raw.Equal(req.State.Raw) {
		return
	}
	r.reportScopeImpact(ctx, req, resp)
}

// healPayloadPlan runs the payload-diff decision for an update plan. When
// both private-state references are present (post-first-Apply, non-imported),
// it runs the three-way compare to distinguish three cases:
//
//   - NoOp     — plan rewritten to state so Terraform reports no change.
//   - Apply    — diff propagates naturally so Update runs.
//   - Drift    — diff propagates so Terraform surfaces server-side change.
//
// When either reference is empty (freshly imported, pre-tracking resources)
// it falls back to the legacy two-way intersection compare via
// PayloadsSemanticallyEqual.
func (r *Resource) healPayloadPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.General == nil || state.General == nil {
		return
	}
	if plan.General.Payloads.IsNull() || plan.General.Payloads.IsUnknown() {
		return
	}
	if state.General.Payloads.IsNull() {
		return
	}

	planBytes := []byte(plan.General.Payloads.ValueString())
	stateBytes := []byte(state.General.Payloads.ValueString())
	if string(planBytes) == string(stateBytes) {
		return
	}

	lastInputRaw, diags := req.Private.GetKey(ctx, privateKeyLastInput)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	lastCanonicalRaw, diags := req.Private.GetKey(ctx, privateKeyLastCanonical)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	serverNowRaw, diags := req.Private.GetKey(ctx, privateKeyServerNow)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	lastInput, err := decodePrivatePayload(lastInputRaw)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not decode payload_last_input from private state; falling back to two-way compare", err.Error())
		lastInput = nil
	}
	lastCanonical, err := decodePrivatePayload(lastCanonicalRaw)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not decode payload_last_canonical from private state; falling back to two-way compare", err.Error())
		lastCanonical = nil
	}
	serverNow, err := decodePrivatePayload(serverNowRaw)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not decode payload_server_now from private state; falling back to two-way compare", err.Error())
		serverNow = nil
	}
	threeWayServerNow := serverNow
	if len(threeWayServerNow) == 0 {
		threeWayServerNow = stateBytes
	}

	if len(lastInput) > 0 && len(lastCanonical) > 0 {
		decision, err := payloadhelpers.ThreeWayCompare(planBytes, lastInput, lastCanonical, threeWayServerNow)
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Payload three-way compare failed; falling back to two-way compare",
				err.Error(),
			)
		} else {
			switch decision {
			case payloadhelpers.DecisionNoOp:
				tflog.Info(ctx, "payload three-way: NoOp", map[string]any{
					"plan_bytes":  len(planBytes),
					"state_bytes": len(stateBytes),
				})
				plan.General.Payloads = state.General.Payloads
				// The byte-only payload change made the framework propose an
				// update on `general`, which marked every Computed-null-config
				// sibling Unknown. UseStateForUnknown restores most of them, but
				// category_name / site_name intentionally lack it (§886: they must
				// go Unknown when their backing id changes). A payload-only NoOp
				// cannot have changed either id, so restore each derived name from
				// state when its id is unchanged — otherwise the spurious Unknown
				// keeps the plan a non-empty update. When an id *is* changing in the
				// same plan, leave the name Unknown so §886 still holds.
				if plan.General.CategoryID.Equal(state.General.CategoryID) {
					plan.General.CategoryName = state.General.CategoryName
				}
				if plan.General.SiteID.Equal(state.General.SiteID) {
					plan.General.SiteName = state.General.SiteName
				}
				resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
				return
			case payloadhelpers.DecisionApply:
				tflog.Info(ctx, "payload three-way: Apply (user HCL changed since last apply)", map[string]any{
					"plan_bytes":       len(planBytes),
					"last_input_bytes": len(lastInput),
					"state_bytes":      len(stateBytes),
				})
				return
			case payloadhelpers.DecisionDrift:
				tflog.Info(ctx, "payload three-way: Drift (server diverged from last-applied canonical)", map[string]any{
					"last_canonical_bytes": len(lastCanonical),
					"state_bytes":          len(stateBytes),
				})
				return
			}
		}
	}

	equal, err := payloadhelpers.PayloadsSemanticallyEqual(planBytes, stateBytes)
	if err != nil {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("general").AtName("payloads"),
			"Payload diff comparison failed; falling through to byte-level diff",
			err.Error(),
		)
		return
	}
	if equal {
		tflog.Info(ctx, "payload two-way fallback: semantically equal, suppressing diff",
			map[string]any{
				"plan_bytes":  len(planBytes),
				"state_bytes": len(stateBytes),
			})
		plan.General.Payloads = state.General.Payloads
		// Same collateral-Unknown restore as the three-way NoOp branch above.
		// The byte-only payload change made the framework propose an update on
		// `general`, marking category_name / site_name Unknown (they lack
		// UseStateForUnknown per §886). This fallback path runs on the first
		// plan post-import — when private state has no last_input/last_canonical
		// baseline — so without this restore the spurious Unknown keeps the plan
		// a non-empty update and forces a PUT that re-issues the profile to every
		// scoped device. A payload-only NoOp cannot have changed either id, so
		// restore each derived name from state when its id is unchanged; when an
		// id is genuinely changing, leave the name Unknown so §886 still holds.
		if plan.General.CategoryID.Equal(state.General.CategoryID) {
			plan.General.CategoryName = state.General.CategoryName
		}
		if plan.General.SiteID.Equal(state.General.SiteID) {
			plan.General.SiteName = state.General.SiteName
		}
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}
