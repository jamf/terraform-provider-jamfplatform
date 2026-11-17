// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

// Schema deprecations of jamfplatform_pro_user_initiated_enrollment_settings, in one
// place. Each notice names the earliest removal date and carries the greppable marker
// the deprecation sweep reads. On or after the date, remove the attribute and its
// builders, drop it from prior state in UpgradeState, bump the schema Version, and
// delete the notice. See STYLE_GUIDE "Jamf Pro Terraform schema deprecation".
//
// DeprecationMessage reaches an operator through terraform plan and validate only.
// tfplugindocs does not render it, so each notice also prefixes the attribute's
// documentation with a callout built from the same text.

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=none — Jamf is removing QuickAdd package signing from user-initiated enrollment settings (estimated November 2026); Jamf Pro 11.34.0 already reverts the setting to false without a developer certificate.
const signQuickAddPackageDeprecation = "Deprecated: Jamf is removing QuickAdd package signing from user-initiated enrollment settings, with an estimated removal date of November 2026. " +
	"The provider will not remove `sign_quickadd_package` before 2027-02-15."

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=none — only used to sign the QuickAdd package, whose signing Jamf is removing.
const developerCertificateDeprecation = "Deprecated: the developer certificate only signs the QuickAdd package, and Jamf is removing QuickAdd package signing. " +
	"The provider will not remove `developer_certificate` before 2027-02-15."

const signQuickAddPackageDeprecatedDescription = "~> **Deprecated.** " + signQuickAddPackageDeprecation + "\n\n" +
	"Sign the QuickAdd package with a developer certificate. Matches the \"Sign QuickAdd Package\" checkbox. Supply the `developer_certificate` block to upload a signing identity. " +
	"From Jamf Pro 11.34.0, the provider rejects `true` without a `developer_certificate` at plan time, because Jamf Pro accepts the write and then reports `false`. " +
	"Omit to leave the current value untouched."

const developerCertificateDeprecatedDescriptionPrefix = "~> **Deprecated.** " + developerCertificateDeprecation + "\n\n"
