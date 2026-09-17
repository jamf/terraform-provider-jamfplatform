// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// RequireSupportedJamfProVersion refuses to configure a Jamf Pro device-group
// construct on a tenant whose version falls inside the window in which a
// Jamf-group "member of" criterion value comes back as a numeric group id instead
// of the authored group name — PI-1394, the 11.29 nested-smart-group rework,
// fixed in 11.30.1.
//
// The window bounds are criteria.GroupRefRegressionVersion and
// criteria.GroupRefFixedVersion, so this gate and the workaround that the other
// criteria-bearing constructs carry can never disagree about where the window is.
//
// This family refuses rather than working around. criteria.GroupResolver exists
// and would map the value back, but whether these particular endpoints
// (/pro/v3/computer-groups, /pro/v2/mobile-device-groups) regressed inside the
// window is unprobeable: the regression was characterised on classic /usergroups
// and Platform /device-groups, and a tenant inside the window was not available.
// Shipping an unverified value mapping against an endpoint that may never have
// needed it risks rewriting a value the server returned correctly; refusing tells
// the operator plainly to upgrade to 11.30.1, which is a supported release.
//
// The refusal is deliberately CERTAIN-ONLY. It uses
// helpers.JamfProVersionInRangeStrict, not its fail-open sibling: a version string
// that will not parse must not block, because that would turn one unparseable
// response into an outage across every construct in the family. A tenant whose
// version cannot be read at all is likewise let through — the version fetch
// failing is ConfigurePro's business, and this gate adds nothing by duplicating
// its error.
//
// Scope note: the regression reaches only a Jamf-group "member of" criterion
// value, so it cannot affect a static group, which has no criteria. The gate is
// applied across the whole family anyway, by maintainer decision, so the family
// has one answer about which tenants it supports. Narrowing it to the two smart
// constructs later is deleting the two static call sites.
func RequireSupportedJamfProVersion(ctx context.Context, pd *providerdata.Data, resourceType string) diag.Diagnostics {
	var diags diag.Diagnostics
	if pd == nil {
		return diags
	}
	version, err := pd.GetJamfProVersion(ctx)
	if err != nil {
		return diags
	}
	return RefusalDiagnostics(version, resourceType)
}

// RefusalDiagnostics is the decision RequireSupportedJamfProVersion makes, split
// out from the version fetch so the table it implements can be tested without a
// client: refuse when version parses and lands inside the PI-1394 window, and say
// nothing otherwise — including when it will not parse.
func RefusalDiagnostics(version, resourceType string) diag.Diagnostics {
	var diags diag.Diagnostics
	if !helpers.JamfProVersionInRangeStrict(version, criteria.GroupRefRegressionVersion, criteria.GroupRefFixedVersion) {
		return diags
	}
	diags.AddError(
		"Jamf Pro version not supported by this resource",
		fmt.Sprintf(
			"%s does not support Jamf Pro %s. From Jamf Pro %s up to but not including %s, a group criterion that matches on membership of another Jamf group stores the referenced group's numeric identifier in place of its name, so Terraform cannot keep the configured value and the plan never settles. Upgrade the tenant to Jamf Pro %s or later. To manage groups on this version, use jamfplatform_device_group, which is unaffected — it has no Jamf Pro site support, which is the only thing this resource adds.",
			resourceType, version,
			criteria.GroupRefRegressionVersion, criteria.GroupRefFixedVersion,
			criteria.GroupRefFixedVersion,
		),
	)
	return diags
}
