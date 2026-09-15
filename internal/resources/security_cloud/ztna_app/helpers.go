// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes Jamf Security Cloud returns on the ZTNA app
// endpoint. Wire-probed against production EU on 2026-08-30; each one is translated
// into a diagnostic attached to the attribute that caused it, because the raw
// message names the code and not the fix.
//
// Only NOT_ENTITLED is in the SDK's generated `ApiErrorItemCode` enum, which the
// spec declares from the DNS schema and which carries none of the ZTNA app codes.
// The rest are therefore literals here, each exempted by name in
// enum_literals_test.go so that an SDK release adding one fails the guard instead of
// leaving a stale duplicate behind. Checked value by value rather than as a set: the
// generated enum holding the generic codes while holding none of this construct's is
// exactly the shape that has hidden this defect before.
const (
	codeHostnameConflict      = "HOSTNAME_CONFLICT"
	codeBareIPsConflict       = "BARE_IPS_CONFLICT"
	codeMissingCategoryName   = "MISSING_CATEGORY_NAME"
	codeMissingUserGroups     = "MISSING_USER_GROUPS"
	codePredefinedAppNotFound = "PREDEFINED_APP_NOT_FOUND"
	codeConflict              = "CONFLICT"
	codeNotEntitled           = securitycloud.ApiErrorItemCodeNotEntitled
)

// appendWriteDiagnostics turns a create/update failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// The codes worth translating are the ones whose cause is a tenant-wide constraint
// Terraform cannot see in the plan. Three name their offending value and three do
// not, which is why the descriptions differ in how much they have to explain:
//
//   - HOSTNAME_CONFLICT and PREDEFINED_APP_NOT_FOUND name the value; the diagnostic
//     mostly needs to say which other object holds it.
//   - MISSING_CATEGORY_NAME names the value but not that the accepted vocabulary is
//     the category *display* names rather than their internal names — the single
//     easiest mistake to make on this resource.
//   - BARE_IPS_CONFLICT names nothing at all, so the diagnostic has to say which
//     attribute to go and look at.
//   - CONFLICT is the undocumented one-app-per-template rule and its body is the
//     bare sentence "Resource already exists.", naming neither the field nor the
//     template. It is only translated when the write carried a predefined app ID,
//     because CONFLICT is a generic code and claiming that meaning for every
//     occurrence would mislabel a future one.
//
// NOT_ENTITLED is the other one worth naming: the credentials are valid and the
// tenant simply does not have the surface, which is invisible in a bare 403.
//
// Details this function does not translate — an unrecognised code, or a CONFLICT on a
// custom app — are collected rather than discarded, and reported verbatim once at
// least one sibling detail has been translated. Both callers add the raw
// `err.Error()` only when this returns false, so a body carrying one known and one
// unknown code would otherwise report the known one and drop the unknown entirely.
// Reporting them only alongside a translated sibling is deliberate: when nothing at
// all is recognised, returning false is strictly better than reporting the leftovers
// here, because the SDK's own error string carries the HTTP status, the trace ID and
// the request URL as well as every detail. Every one of the ~12 refusals probed on
// 2026-08-30 carried a single-element `errors` array, so the mixed-detail case is
// defensive rather than observed.
//
// INVALID_FIELD deliberately has no case of its own. The live API uses it for the
// routing refusal, the host name mutual-exclusivity refusal and a malformed
// `bareIps` entry alike, so only `field` disambiguates it — and the wire field names
// do not all map onto attribute names (`bareIps` is `direct_ips_and_subnets`). A
// single named case would therefore mislabel two refusals to point an attribute path
// at the third, while the untranslated path above already surfaces the server's own
// field and description.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error, hasPredefinedAppID bool) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	var untranslated []jamfplatform.ErrorDetail
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeHostnameConflict:
			diags.AddAttributeError(
				path.Root("hostnames"),
				"Host name already claimed by another access policy application",
				"Jamf Security Cloud allows a host name to match only one application across the whole tenant, "+
					"so two applications cannot list the same one. Remove it from the application that already "+
					"holds it, or drop it from this one — the `jamfplatform_security_cloud_ztna_apps` data source "+
					"lists what exists. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeBareIPsConflict:
			diags.AddAttributeError(
				path.Root("direct_ips_and_subnets"),
				"Address range overlaps another access policy application",
				"One of the ranges in `direct_ips_and_subnets` is already claimed by another application, and "+
					"Jamf Security Cloud allows a range to match only one. It does not say which range, so "+
					"compare this list against the other applications on the tenant. Reported by Jamf Security "+
					"Cloud: "+detail.Description,
			)
		case codeMissingCategoryName:
			diags.AddAttributeError(
				path.Root("category"),
				"Unknown application category",
				"`category` must exactly match a category Jamf Security Cloud already defines. Use the "+
					"`display_name` from the `jamfplatform_security_cloud_content_categories` data source, not "+
					"its `name` — the two differ, and the internal name is not accepted. The category list is "+
					"maintained by Jamf and can change. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeMissingUserGroups:
			diags.AddAttributeError(
				path.Root("device_group_ids"),
				"Referenced device group not found",
				"One of the device groups this application is assigned to does not exist in Jamf Security "+
					"Cloud. The group must exist before the application can reference it. Reported by Jamf "+
					"Security Cloud: "+detail.Description,
			)
		case codePredefinedAppNotFound:
			diags.AddAttributeError(
				path.Root("predefined_app_id"),
				"Predefined application not found",
				"`predefined_app_id` does not name a predefined application Jamf Security Cloud offers. Use the "+
					"`jamfplatform_security_cloud_ztna_predefined_apps` data source to look the ID up rather "+
					"than hard-coding it. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeConflict:
			if !hasPredefinedAppID {
				untranslated = append(untranslated, detail)
				continue
			}
			diags.AddAttributeError(
				path.Root("predefined_app_id"),
				"Predefined application already in use",
				"Jamf Security Cloud allows only one access policy application per predefined application, and "+
					"this tenant already has one for this `predefined_app_id`. Import the existing application "+
					"instead of creating a second, or point this one at a different predefined application. "+
					"Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeNotEntitled:
			diags.AddError(
				"Tenant not entitled to Jamf Security Cloud ZTNA",
				"The credentials authenticated successfully but this tenant does not have the ZTNA surface "+
					"enabled. Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		default:
			untranslated = append(untranslated, detail)
			continue
		}
		matched = true
	}
	if !matched {
		return false
	}
	for _, detail := range untranslated {
		diags.AddError(
			"Jamf Security Cloud refused the access policy application",
			"Reported by Jamf Security Cloud: "+reportedDetail(detail),
		)
	}
	return true
}

// reportedDetail renders one error detail the way the SDK's own error string does,
// so an untranslated detail reads the same whether it reaches the operator through
// this function or through the callers' raw-error fallback. Code and field are both
// optional on the wire — a bare 409 carries neither.
func reportedDetail(detail jamfplatform.ErrorDetail) string {
	rendered := detail.Description
	if detail.Field != "" {
		rendered = detail.Field + ": " + rendered
	}
	if detail.Code != "" {
		rendered = "[" + detail.Code + "] " + rendered
	}
	return rendered
}
