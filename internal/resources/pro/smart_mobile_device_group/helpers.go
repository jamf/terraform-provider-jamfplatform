// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// resolvePlatformIDForRead returns the platform identifier to store for a group
// whose Jamf Pro identifier is jamfProID.
//
// prior short-circuits it, and that is the common case rather than an
// optimisation: the identifier is a standalone server identity that never
// changes, so a refresh already holds it and spending a request to confirm it
// would add one read per group per plan. An import arrives without it, and so
// does an update to a group whose stored value is null — see Update, which hands
// a null prior deliberately so that a bridge miss collapses to null rather than
// returning the unknown a plan carries there.
//
// A bridge that cannot answer costs the caller nothing. It returns prior, which
// on that path is null, and the warning it raises is latched once per provider
// invocation inside progroups so a configuration holding forty groups reports
// the cause once.
func resolvePlatformIDForRead(ctx context.Context, client *pro.Client, pd *providerdata.Data, jamfProID, prior types.String) (types.String, diag.Diagnostics) {
	var diags diag.Diagnostics
	if helpers.IsConfiguredValue(prior) {
		return prior, diags
	}
	byPlatformID, bridgeDiags := progroups.PlatformIDsByJamfProID(ctx, client, pd, progroups.DeviceTypeMobile, progroups.GroupKindSmart)
	diags.Append(bridgeDiags...)
	resolved := progroups.PlatformIDValue(byPlatformID, jamfProID.ValueString())
	if resolved.IsNull() {
		return prior, diags
	}
	return resolved, diags
}

// criteriaAtPlanTime reads this group's criteria out of one side of a plan.
//
// It is a thin wrapper over criteria.CriteriaAtPlanTime, which carries the
// reasoning: the resource model's criteria field is a Go slice, and the
// framework cannot put an unknown into one without aborting the plan.
func criteriaAtPlanTime(ctx context.Context, from criteria.AttributeSource) ([]criteria.CriterionModel, bool, diag.Diagnostics) {
	return criteria.CriteriaAtPlanTime(ctx, from, path.Root("criteria"))
}

// criterionListsDiffer reports whether two criterion lists describe different
// membership.
//
// priority is excluded deliberately. It is the criterion's own position, which
// the endpoint requires and the builders emit from the list index, so it carries
// no information the position does not already carry and a plan that only
// renumbers it changes nobody's membership. Every other field is compared at its
// position, which is what catches a reorder: moving a criterion moves its name
// and value to a different index.
func criterionListsDiffer(a, b []criteria.CriterionModel) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if !a[i].Name.Equal(b[i].Name) ||
			!a[i].SearchType.Equal(b[i].SearchType) ||
			!a[i].Value.Equal(b[i].Value) ||
			!a[i].AndOr.Equal(b[i].AndOr) ||
			!a[i].HasOpeningParenthesis.Equal(b[i].HasOpeningParenthesis) ||
			!a[i].HasClosingParenthesis.Equal(b[i].HasClosingParenthesis) {
			return true
		}
	}
	return false
}

// fallbackApplies reports whether a failed group-endpoint write should be
// repeated through Jamf Pro's older group interface.
//
// It is a named gate consulted by both writes rather than the classifier called
// twice, so the trigger cannot drift between the create and the update, and so
// the day Jamf Pro fixes the defects this function and its two call sites are
// what there is to delete.
//
// The trigger is the refusal itself and never the criterion's name. Matching
// "Patch Reporting:" would cover one of the three known refusals and nothing
// that comes after them, and every other failure — a duplicate name, a site the
// integration cannot reach, a genuinely malformed criterion — has to surface as
// it does on the ordinary path. progroups.ModernWriteRefused carries the
// reproductions and the exact wording it matches.
func fallbackApplies(err error) bool {
	return progroups.ModernWriteRefused(err)
}

// errPlatformIDUnresolved stands for the one cause of a failed description write
// that is not a Jamf Pro response. The identifier that write takes is resolved
// through the platform bridge, which is best-effort and reports its own failure
// as a warning, so the description diagnostic points at that warning rather than
// restating it.
var errPlatformIDUnresolved = errors.New("the identifier the description write needs could not be resolved")

// platformIDForDescriptionWrite returns the identifier the description write
// takes, or the empty string when it cannot be resolved.
//
// That write is a merge patch keyed on the platform identifier; a Jamf Pro
// identifier answers 404 there. An update already holds the value, which is why
// known short-circuits the lookup. A fallback create does not and cannot ask for
// it, because the older interface has no platform-identifier option at all, so
// it resolves through the same bridge import uses.
//
// Both fallback paths then keep what it returns as platform_id, so a group whose
// description needs no write still resolves here. That is not a wasted request:
// the lookup only happens when the caller holds no identifier, which is exactly
// when platform_id has nothing to settle to either.
func platformIDForDescriptionWrite(ctx context.Context, client *pro.Client, pd *providerdata.Data, jamfProID string, known types.String) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if helpers.IsConfiguredValue(known) && known.ValueString() != "" {
		return known.ValueString(), diags
	}
	byPlatformID, bridgeDiags := progroups.PlatformIDsByJamfProID(ctx, client, pd, progroups.DeviceTypeMobile, progroups.GroupKindSmart)
	diags.Append(bridgeDiags...)
	resolved := progroups.PlatformIDValue(byPlatformID, jamfProID)
	if resolved.IsNull() {
		return "", diags
	}
	return resolved.ValueString(), diags
}

// descriptionNotSetAfterCreate reports a fallback create that stored the group
// and its criteria but could not set the description.
//
// It is an error rather than a warning because the group in Jamf Pro does not
// match the configuration. It is raised after state has been written, not
// instead of writing it: the group exists, and dropping it from state would
// leave it unmanaged with nothing naming it.
//
// What that combination means on a CREATE is not what it reads as, which is why
// this message and the update's differ. A create returning an error alongside
// state leaves Terraform holding the object as tainted, and a tainted object is
// planned for replacement rather than for repair — so the next apply destroys
// this group and makes another, and telling the operator to apply again to
// finish the change would describe a repair that never happens. Untainting
// first does produce that repair, because state records the description Jamf
// Pro holds rather than the configured one, so the next plan still shows the
// difference and the update path can take the second write on its own.
func descriptionNotSetAfterCreate(label string, cause error) diag.Diagnostic {
	return descriptionNotSetDiagnostic(
		label,
		cause,
		"Terraform has recorded the group and marked it tainted, so the next plan replaces it rather than completing it. Apply again to replace it, or run `terraform untaint` on it and then apply to set the description on the group that is already there.",
	)
}

// descriptionNotSetAfterUpdate reports a fallback update that stored the group's
// criteria but could not set the description.
//
// An update that returns an error alongside state leaves the object as it was,
// managed and not tainted, and state records the description Jamf Pro actually
// holds — so the next plan sees the difference and the next apply does finish
// the change.
func descriptionNotSetAfterUpdate(label string, cause error) diag.Diagnostic {
	return descriptionNotSetDiagnostic(
		label,
		cause,
		"Terraform has recorded the description Jamf Pro holds, so the next plan shows it as a change again; apply again to finish it.",
	)
}

// descriptionNotSetDiagnostic is the shared body of the two, taking the sentence
// that says what the operator does next. Only that sentence differs, because
// only the recovery does.
func descriptionNotSetDiagnostic(label string, cause error, recovery string) diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"Saved this "+label+" but could not set its description",
		"Jamf Pro would not accept this group's criteria the usual way, so the provider saved it the older way, which has no description field. Setting the description takes a second write and that write failed, so the group's description is not what this configuration says. "+recovery+
			"\n\nWhat went wrong: "+helpers.APIErrorDetail(cause),
	)
}
