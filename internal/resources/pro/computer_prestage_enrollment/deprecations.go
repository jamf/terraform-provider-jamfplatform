// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import "github.com/jamf/terraform-provider-jamfplatform/internal/common/prestagealias"

// Schema deprecations of the attributes the admin UI names differently, in one place. Each
// notice names its replacement and the earliest removal date, and carries the greppable marker
// the deprecation sweep reads. On or after the date, remove the attribute
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

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=make_mdm_profile_mandatory — renamed to match the admin UI label "Make MDM Profile Mandatory".
const deprecationMandatory = "Deprecated: use `make_mdm_profile_mandatory` instead. " +
	"The provider will not remove `mandatory` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=allow_mdm_profile_removal — renamed to match the admin UI label "Allow MDM Profile Removal".
const deprecationMdmRemovable = "Deprecated: use `allow_mdm_profile_removal` instead. " +
	"The provider will not remove `mdm_removable` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=authentication_message — renamed to match the admin UI label "Authentication Message".
const deprecationAuthenticationPrompt = "Deprecated: use `authentication_message` instead. " +
	"The provider will not remove `authentication_prompt` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=automatically_advance_through_setup_assistant — renamed to match the admin UI label "Automatically advance through Setup Assistant".
const deprecationAutoAdvanceSetup = "Deprecated: use `automatically_advance_through_setup_assistant` instead. " +
	"The provider will not remove `auto_advance_setup` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=use_existing_location_information — renamed to match the admin UI label "Use existing location information, if applicable".
const deprecationKeepExistingLocationInformation = "Deprecated: use `use_existing_location_information` instead. " +
	"The provider will not remove `keep_existing_location_information` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=use_existing_site_membership — renamed to match the admin UI label "Use existing site membership, if applicable".
const deprecationKeepExistingSiteMembership = "Deprecated: use `use_existing_site_membership` instead. " +
	"The provider will not remove `keep_existing_site_membership` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=configuration_profile_ids — renamed to match the admin UI pane "Configuration Profiles".
const deprecationPrestageInstalledProfileIDs = "Deprecated: use `configuration_profile_ids` instead. " +
	"The provider will not remove `prestage_installed_profile_ids` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=configuration_profile_ids — Jamf Pro treats a non-empty profile list as the switch, and the admin UI has no control for it.
const deprecationInstallProfilesDuringSetup = "Deprecated: the provider now derives this from `configuration_profile_ids`, so remove it. " +
	"The provider will not remove `install_profiles_during_setup` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=account_settings.create_managed_local_administrator_account — renamed to match the admin UI label "Create a managed local administrator account during macOS Setup Assistant".
const deprecationLocalAdminAccountEnabled = "Deprecated: use `create_managed_local_administrator_account` instead. " +
	"The provider will not remove `local_admin_account_enabled` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=account_settings.make_managed_local_administrator_account_mdm_enabled — renamed to match the admin UI label "Make the managed local administrator account MDM-enabled".
const deprecationLocalUserManaged = "Deprecated: use `make_managed_local_administrator_account_mdm_enabled` instead. " +
	"The provider will not remove `local_user_managed` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=account_settings.hide_managed_local_administrator_account — renamed to match the admin UI label "Hide managed local administrator account in Users & Groups".
const deprecationHiddenAdminAccount = "Deprecated: use `hide_managed_local_administrator_account` instead. " +
	"The provider will not remove `hidden_admin_account` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=account_settings.lock_primary_account_information — renamed to match the admin UI label "Lock primary account information".
const deprecationPreventPrefillInfoFromModification = "Deprecated: use `lock_primary_account_information` instead. " +
	"The provider will not remove `prevent_prefill_info_from_modification` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=account_settings — Jamf Pro needs it true whenever the block sets anything, and the admin UI has no control for it.
const deprecationPayloadConfigured = "Deprecated: the provider now derives this from the other `account_settings` attributes, so remove it. " +
	"The provider will not remove `payload_configured` before 2027-02-03."

// accountSettingsPairs and topLevelPairs list the renamed attributes for prestagealias.
var accountSettingsPairs = []prestagealias.Pair{
	{Parent: "account_settings", Old: "local_admin_account_enabled", New: "create_managed_local_administrator_account"},
	{Parent: "account_settings", Old: "local_user_managed", New: "make_managed_local_administrator_account_mdm_enabled"},
	{Parent: "account_settings", Old: "hidden_admin_account", New: "hide_managed_local_administrator_account"},
	{Parent: "account_settings", Old: "prevent_prefill_info_from_modification", New: "lock_primary_account_information"},
}

var topLevelPairs = []prestagealias.Pair{
	{Old: "mandatory", New: "make_mdm_profile_mandatory"},
	{Old: "mdm_removable", New: "allow_mdm_profile_removal"},
	{Old: "authentication_prompt", New: "authentication_message"},
	{Old: "auto_advance_setup", New: "automatically_advance_through_setup_assistant"},
	{Old: "keep_existing_location_information", New: "use_existing_location_information"},
	{Old: "keep_existing_site_membership", New: "use_existing_site_membership"},
	{Old: "prestage_installed_profile_ids", New: "configuration_profile_ids"},
}
