// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

// Why this package suppresses SA1019, in one place.
//
// Both Classic /activationcode verbs were deprecated on 2026-07-14
// (jamfplatform-go-sdk v1.1.0 carries the markers). Neither half has a usable
// successor for this construct:
//
//   - The read has no successor at all. The Jamf Pro API publishes no
//     GET /v1/activation-code in any environment — only PUT /v1/activation-code
//     and PATCH /v1/activation-code/organization-name — and no other Classic
//     operation returns the code. So the Classic GET is the only way to read it,
//     it still answers 200, and there is nothing to migrate to. Raised with the
//     Jamf Pro API team.
//   - The write has one, pro.UpdateActivationCodeV1, and it is deliberately not
//     adopted. It carries the code alone; the organization name moves to a
//     separate PATCH. That turns one atomic Classic write into two, so a partial
//     failure would commit the code and leave the organization name stale — a
//     worse contract than the endpoint being replaced, paid for a construct that
//     is being retired rather than migrated.
//
// Because the read cannot be replaced, the construct cannot outlive the endpoint.
// Both the resource and the data source therefore carry a whole-schema
// DeprecationMessage instead of a migration: they are removed when the endpoint
// is withdrawn, and no date is claimed here because no removal date is published.
//
// DeprecationMessage reaches an operator only through terraform plan and validate;
// tfplugindocs does not render it, so the Registry page would say nothing. That is what
// the matching callouts below are for — they go at the top of each construct's
// MarkdownDescription so the notice survives into the published documentation.
//
// The six call sites suppress SA1019 and point at this file. Two are acceptance-tagged,
// which `make lint` does not compile today, so they are suppressed for the day that row
// is added rather than left to fail it. They all go when the constructs go.
const (
	// resourceDeprecation is the whole-schema deprecation notice on
	// jamfplatform_pro_activation_code, the resource.
	resourceDeprecation = "The Classic /activationcode endpoint is deprecated as of 2026-07-14. The Jamf Pro API publishes no " +
		"replacement read, so this resource will be removed with it. Set the activation code in Jamf Pro instead."

	// dataSourceDeprecation is the whole-schema deprecation notice on
	// jamfplatform_pro_activation_code, the data source.
	dataSourceDeprecation = "The Classic /activationcode endpoint is deprecated as of 2026-07-14. The Jamf Pro API publishes no " +
		"replacement read, so this data source will be removed with it. Read the activation code in Jamf Pro instead."

	// resourceDeprecationCallout carries resourceDeprecation into the resource's rendered
	// documentation page.
	resourceDeprecationCallout = "~> **Deprecated.** " + resourceDeprecation + "\n\n"

	// dataSourceDeprecationCallout carries dataSourceDeprecation into the data source's
	// rendered documentation page.
	dataSourceDeprecationCallout = "~> **Deprecated.** " + dataSourceDeprecation + "\n\n"
)
