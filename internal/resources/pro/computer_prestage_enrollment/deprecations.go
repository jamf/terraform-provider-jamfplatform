// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

// Schema deprecations of the skip_setup_items attributes the admin UI names differently, in
// one place. Each notice names its replacement and the earliest removal date, and carries the
// greppable marker the deprecation sweep reads. On or after the date, remove the attribute
// and its alias entry, drop it from prior state in UpgradeState, bump the schema Version, and
// delete the notice. See STYLE_GUIDE "Jamf Pro Terraform schema deprecation".
//
// DeprecationMessage reaches an operator through terraform plan and validate only.
// tfplugindocs does not render it, so each attribute's documentation opens with a callout
// built from the same text.

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.get_started — renamed to match the admin UI label "Get Started".
const skipDeprecationWelcome = "Deprecated: use `get_started` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `welcome` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.transfer_information — renamed to match the admin UI label "Transfer Information".
const skipDeprecationRestore = "Deprecated: use `transfer_information` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `restore` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.terms_and_conditions — renamed to match the admin UI label "Terms and Conditions".
const skipDeprecationTOS = "Deprecated: use `terms_and_conditions` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `tos` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.app_analytics — renamed to match the admin UI label "App Analytics".
const skipDeprecationDiagnostics = "Deprecated: use `app_analytics` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `diagnostics` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.all_your_files_in_icloud — renamed to match the admin UI label "All Your Files in iCloud".
const skipDeprecationICloudStorage = "Deprecated: use `all_your_files_in_icloud` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `icloud_storage` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.choose_your_look — renamed to match the admin UI label "Choose your Look".
const skipDeprecationAppearance = "Deprecated: use `choose_your_look` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `appearance` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.true_tone_display — renamed to match the admin UI label "True Tone Display".
const skipDeprecationDisplayTone = "Deprecated: use `true_tone_display` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `display_tone` before 2027-02-03."
