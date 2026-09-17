// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Jamf Pro's smart-group endpoints validate a criterion's name and its operator,
// and their validation table disagrees with the rest of Jamf Pro in at least
// three ways. Each is a combination the admin UI offers, the older group surface
// stores, and the group endpoints refuse. All three were reproduced on Jamf Pro
// 11.32.0 on 2026-09-17, each against both surfaces so the refusal is attributed
// to the endpoint rather than to a malformed request:
//
//	patch reporting criterion, computer smart group  (PI-1286 / PI-1183 / PI-626)
//	  "Patch Reporting: 1Password"  is  "Latest Version"
//	  group endpoint  400 INVALID_FIELD "The criterion Patch Reporting: 1Password is not valid"
//	  older surface   201
//
//	`has` on an extension attribute, computer smart group  (PI-1032)
//	  "Notes"  has  "urgent"
//	  group endpoint  400 INVALID_FIELD "The operator has is not valid for extension attribute Notes"
//	  older surface   201
//
//	`has` on an extension attribute, MOBILE smart group  (PI-1032, wider than the ticket says)
//	  identical refusal and identical acceptance, so this is not computer-only
//
// PI-1032 is filed against Entra ID Groups specifically; it reproduces on a plain
// text extension attribute, so the ticket understates it. PI-626 diagnoses the
// cause of the patch case as validating the criterion name against
// patch_software_title_extension_attribute_scripts.keyId rather than
// smart_group_criteria.search_field, which is why these read as one defect family
// rather than three unrelated bugs.
//
// So the provider writes through the older surface when, and only when, the group
// endpoint refuses. Two probed facts make that safe rather than reckless.
//
// First, a refused write applies NOTHING. A request changing the name, the
// description, the site and the criteria at once, whose criteria were refused,
// left all four at their previous values. So the failed attempt is a clean no-op
// and the retry starts from an unchanged group — there is no partial write to
// reconcile.
//
// Second, the older surface still validates criterion NAMES: a misspelled
// "Operating Sistem Version" is refused there too (409, "Problem with criteria").
// So a genuine typo does not slip through the fallback; it fails on both, and
// BothRefusedDiagnostics reports the group endpoint's message, which names the
// offending criterion precisely where the older surface's does not.
//
// What the older surface does NOT check is whether an operator applies to the
// criterion it is paired with. `in less than x days` on a text extension
// attribute is accepted there and the group then matches nothing. The operator
// itself is already constrained at plan time against criteria.Operators, so this
// is narrower than it sounds — it reaches only a legal operator paired with a
// criterion that has no use for it — but it is the one thing an operator loses on
// a fallback write, and FallbackWarning says so.
const (
	// refusalCodeInvalidField is the code every refusal in this family carries.
	// The group specs type their error bodies as a bare ApiError whose `code` is
	// an unconstrained string and enumerate no values, so the SDK generates no
	// constant for it.
	refusalCodeInvalidField = "INVALID_FIELD"
	// refusalPhraseCriterion matches "The criterion X is not valid".
	refusalPhraseCriterion = "is not valid"
)

// refusalSubjects are the things a refusal in this family names. Matching the
// subject as well as the phrase keeps an unrelated INVALID_FIELD — a malformed
// andOr, say, which the older surface would not fix — from triggering a pointless
// second write.
var refusalSubjects = []string{"The criterion ", "The operator "}

// ModernWriteRefused reports whether err is the group endpoints' refusal of a
// criterion or operator combination that the older group surface accepts.
//
// It is deliberately narrow. An INVALID_FIELD that is not about a criterion or an
// operator — a malformed andOr value, for instance, which both surfaces reject —
// returns false, because retrying it would spend a second request to arrive at
// the same answer with a worse message.
func ModernWriteRefused(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(400) {
		return false
	}
	for _, detail := range apiErr.Errors {
		if !strings.EqualFold(detail.Code, refusalCodeInvalidField) {
			continue
		}
		if !strings.Contains(detail.Description, refusalPhraseCriterion) {
			continue
		}
		for _, subject := range refusalSubjects {
			if strings.Contains(detail.Description, subject) {
				return true
			}
		}
	}
	return false
}

// FallbackWarning is the notice a successful fallback write emits.
//
// It is a warning rather than silence because one guarantee is genuinely weaker
// on this path: the older surface does not check that an operator applies to the
// criterion it is paired with, so a criterion that cannot work simply matches
// nothing instead of being reported. An operator whose group comes back empty
// needs to know that is where to look.
//
// It is a warning rather than an error because the configuration is not wrong —
// the admin UI offers these combinations and Jamf Pro stores them. Refusing would
// mean the practitioner cannot manage a group Jamf Pro itself is happy to have.
func FallbackWarning(label string, modernErr error) diag.Diagnostic {
	return diag.NewWarningDiagnostic(
		fmt.Sprintf("Saved this %s through Jamf Pro's older group interface", label),
		"Jamf Pro would not accept one of this group's criteria the usual way, although it offers the same combination in the web interface, so the provider saved the group the older way instead. The group itself is correct and nothing further is needed."+
			"\n\nOne check is weaker on that path: Jamf Pro does not confirm that an operator suits the criterion it is paired with. If this group ends up with no members, an operator that does not apply to its criterion is the first thing to look at."+
			"\n\nWhat Jamf Pro said: "+helpers.APIErrorDetail(modernErr),
	)
}

// BothRefusedDiagnostics reports a write both surfaces refused.
//
// The group endpoint's message leads, because it is the precise one: it names the
// offending criterion or operator, where the older surface answers a bare
// "Problem with criteria". A misspelled criterion name is the case that lands
// here, and the operator needs to be told which name is wrong rather than that
// something, somewhere, is.
func BothRefusedDiagnostics(label string, modernErr, classicErr error) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.AddError(
		fmt.Sprintf("Jamf Pro rejected this %s's criteria", label),
		"Jamf Pro refused these criteria, and refused them again through its older group interface, so this is a problem with the criteria rather than the known Jamf Pro defect the provider works around."+
			"\n\nWhat Jamf Pro said: "+helpers.APIErrorDetail(modernErr)+
			"\n\nAnd through the older interface: "+helpers.APIErrorDetail(classicErr),
	)
	return diags
}

// PatchReportingCriterionPrefix is how Jamf Pro names the per-title criterion it
// generates for each configured patch software title: "Patch Reporting: "
// followed by the title's own name, e.g. "Patch Reporting: 1Password".
//
// It is a literal because there is nothing to take it from — the criterion
// vocabulary is data Jamf Pro derives from a tenant's own patch titles, so no
// specification enumerates it and the SDK generates no constant. Only the
// acceptance canary uses it, to build the criterion it expects to be refused;
// the fallback itself triggers on the refusal rather than on the name, so no
// write path depends on this spelling.
const PatchReportingCriterionPrefix = "Patch Reporting: "

// classicCriteriaRefusalPhrase is how Jamf Pro's older group interface says it
// will not accept a criterion. It answers 409 with this phrase and names
// nothing, which is why the group endpoint's message leads whenever both
// surfaces refuse.
const classicCriteriaRefusalPhrase = "problem with criteria"

// OlderInterfaceRefusedCriteria reports whether a fallback write failed because
// the older group interface refused the criteria as well.
//
// That surface still validates criterion NAMES, so a misspelling is refused on
// both paths, and that is the one failure whose diagnosis really is the
// criteria. Every other way the second write can fail — a name already in use, a
// refused site, a gateway timeout — has nothing to do with them.
func OlderInterfaceRefusedCriteria(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusConflict) {
		return false
	}
	for _, detail := range apiErr.Errors {
		if strings.Contains(strings.ToLower(detail.Description), classicCriteriaRefusalPhrase) {
			return true
		}
	}
	return false
}

// FallbackWriteDiagnostics turns a failed fallback write into diagnostics,
// blaming the criteria only where the older interface refused them too.
//
// Everything else goes through WriteDiagnostics, which names the duplicate name
// and the refused site Jamf Pro reports with a code. The criteria refusal that
// sent the write down this path travels with it as a warning: the request that
// failed is one nothing in the configuration asked for, so the error beside it
// would otherwise arrive with no account of why it was made.
//
// Saying "your criteria are wrong" for a duplicate name sends the operator to
// edit a criterion that was never the problem, which is the whole reason this
// classifies rather than reporting one message for every failure.
func FallbackWriteDiagnostics(label string, sitePath, membersPath path.Path, modernErr, classicErr error) diag.Diagnostics {
	if OlderInterfaceRefusedCriteria(classicErr) {
		return BothRefusedDiagnostics(label, modernErr, classicErr)
	}
	var diags diag.Diagnostics
	diags.AddWarning(
		fmt.Sprintf("Jamf Pro would not accept this %s's criteria the usual way", label),
		"Jamf Pro refused one of this group's criteria, although it offers the same combination in the web interface, so the provider repeated the write through Jamf Pro's older group interface. That second write then failed for its own reason, reported alongside this notice, and the criteria are not it."+
			"\n\nWhat Jamf Pro said about the criteria: "+helpers.APIErrorDetail(modernErr),
	)
	diags.Append(WriteDiagnostics(classicErr, label, sitePath, membersPath)...)
	return diags
}
