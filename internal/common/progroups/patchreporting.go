// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
)

// PatchReportingCriterionPrefix is how Jamf Pro names the per-title criterion it
// generates for each patch software title: "Patch Reporting: " followed by the
// title's own name, e.g. "Patch Reporting: Zulu OpenJDK 9".
//
// It is a literal because there is nothing to take it from — the criterion
// vocabulary is data Jamf Pro derives from a tenant's own patch titles, so no
// specification enumerates it and the SDK generates no constant.
const PatchReportingCriterionPrefix = "Patch Reporting: "

// ValidatePatchReportingCriteria refuses a patch-reporting criterion at plan time,
// because the smart-group endpoint accepts a group carrying one on read and then
// refuses to write it.
//
// PI-1183, open, and wire-confirmed on Jamf Pro 11.32.0 on 2026-09-17 by putting
// the same criterion through both surfaces with a real patch software title in
// place:
//
//   - POST /proclassic/computergroups/id/0 with
//     <name>Patch Reporting: Zulu OpenJDK 9</name> → 201 Created.
//   - PUT /pro/v3/computer-groups/smart-groups/{id} with the identical criterion
//     name and value → 400 INVALID_FIELD, "The criterion Patch Reporting: Zulu
//     OpenJDK 9 is not valid".
//
// So the name is right and the endpoint is wrong, which is the distinction that
// matters: a guess at the criterion's spelling would have produced the same 400.
//
// The shape this guards against is worse than a plain rejection. GET on the
// v3 endpoint returns the classic-made group and its patch criterion in full, so
// an import succeeds and lands clean, believable state — and then every later
// apply fails, including a re-write of the identical body and including a change
// to nothing but the name. Probed: PUT of the group's own unmodified criteria
// answers the same 400. Left unguarded, an operator would import a group and
// discover it is unmanageable only on the next unrelated edit.
//
// Refusing at plan time turns that into one clear message before anything is
// written. The alternative — writing through the classic endpoint instead —
// is deliberately not taken: it would mean a second write path, on a deprecated
// surface, for one criterion family, and it would hide a server defect that Jamf
// should fix.
//
// PatchReportingTripwireNote records what to do when they do fix it.
func ValidatePatchReportingCriteria(criteriaPath path.Path, models []criteria.CriterionModel) diag.Diagnostics {
	var diags diag.Diagnostics
	for i, c := range models {
		if c.Name.IsNull() || c.Name.IsUnknown() {
			continue
		}
		name := c.Name.ValueString()
		if !strings.HasPrefix(name, PatchReportingCriterionPrefix) {
			continue
		}
		diags.AddAttributeError(
			criteriaPath.AtListIndex(i).AtName("name"),
			"Jamf Pro cannot save a smart group built on patch reporting criteria",
			fmt.Sprintf(
				"Jamf Pro rejects %q when a smart group is saved this way, even though it offers the criterion and reports it on groups that already use it. This is a known Jamf Pro defect, tracked as PI-1183, and it has no workaround in Terraform.\n\nA group using patch reporting criteria has to be created and edited in Jamf Pro directly. Note that importing one into Terraform will appear to work and then fail on the next change, so leave such a group unmanaged until the defect is fixed.",
				name,
			),
		)
	}
	return diags
}

// PatchReportingTripwireNote is the instruction the acceptance tripwire prints
// when it fails, and it exists so the failure is self-explaining rather than
// looking like a regression in this provider.
//
// The tripwire asserts the DEFECT: that Jamf Pro still refuses a patch-reporting
// criterion on the smart-group write. It therefore fails when Jamf FIXES
// PI-1183, which is the point — a validator that refuses something the server has
// started accepting is a provider bug that nothing else would catch, because
// every other test in the suite passes precisely because the validator blocks
// before any request is made.
const PatchReportingTripwireNote = "Jamf Pro accepted a patch reporting criterion on the smart computer group write, so PI-1183 appears to be fixed. " +
	"This test asserts the defect on purpose and fails when it goes away. Remove progroups.ValidatePatchReportingCriteria and its call site, " +
	"drop the known-limitation paragraph from the smart computer group description, delete this tripwire, and re-run make generate."
