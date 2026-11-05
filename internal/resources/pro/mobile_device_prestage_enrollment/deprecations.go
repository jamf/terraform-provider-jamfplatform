// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

// Schema deprecations of the skip_setup_items attributes the admin UI names differently, in
// one place. Each notice names its replacement and the earliest removal date, and carries the
// greppable marker the deprecation sweep reads. On or after the date, remove the attribute
// and its alias entry, drop it from prior state in UpgradeState, bump the schema Version, and
// delete the notice. See STYLE_GUIDE "Jamf Pro Terraform schema deprecation".
//
// DeprecationMessage reaches an operator through terraform plan and validate only.
// tfplugindocs does not render it, so each attribute's documentation opens with a callout
// built from the same text.

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.terms_and_conditions — renamed to match the admin UI label "Terms and Conditions".
const skipDeprecationTOS = "Deprecated: use `terms_and_conditions` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `tos` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.app_analytics — renamed to match the admin UI label "App Analytics".
const skipDeprecationDiagnostics = "Deprecated: use `app_analytics` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `diagnostics` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.apps_and_data — renamed to match the admin UI label "Apps & Data".
const skipDeprecationRestore = "Deprecated: use `apps_and_data` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `restore` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.add_cellular_plan — renamed to match the admin UI label "Add Cellular Plan".
const skipDeprecationSIMSetup = "Deprecated: use `add_cellular_plan` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `sim_setup` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.display_zoom — renamed to match the admin UI label "Display Zoom".
const skipDeprecationZoom = "Deprecated: use `display_zoom` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `zoom` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.true_tone_display — renamed to match the admin UI label "True Tone Display".
const skipDeprecationDisplayTone = "Deprecated: use `true_tone_display` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `display_tone` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.apple_watch — renamed to match the admin UI label "Apple Watch".
const skipDeprecationWatchMigration = "Deprecated: use `apple_watch` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `watch_migration` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.new_feature_highlights — renamed to match the admin UI label "New Feature Highlights".
const skipDeprecationOnBoarding = "Deprecated: use `new_feature_highlights` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `onboarding` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.choose_your_look — renamed to match the admin UI label "Choose your Look".
const skipDeprecationAppearance = "Deprecated: use `choose_your_look` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `appearance` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.get_started — renamed to match the admin UI label "Get Started".
const skipDeprecationWelcome = "Deprecated: use `get_started` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `welcome` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.software_update_completed — renamed to match the admin UI label "Software Update Completed".
const skipDeprecationUpdateCompleted = "Deprecated: use `software_update_completed` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `update_completed` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.emergency_sos — renamed to match the admin UI label "Emergency SOS".
const skipDeprecationSafety = "Deprecated: use `emergency_sos` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `safety` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.dictation — renamed to match the admin UI label "Dictation".
const skipDeprecationSpokenLanguage = "Deprecated: use `dictation` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `spoken_language` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.set_up_your_apple_tv — renamed to match the admin UI label "Set Up Your Apple TV".
const skipDeprecationTapToSetup = "Deprecated: use `set_up_your_apple_tv` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `tap_to_setup` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.see_the_world_screensaver — renamed to match the admin UI label "See the World Screensaver".
const skipDeprecationScreenSaver = "Deprecated: use `see_the_world_screensaver` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `screen_saver` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.one_home_screen_for_every_apple_tv — renamed to match the admin UI label "One Home Screen for Every Apple TV".
const skipDeprecationTVHomeScreenSync = "Deprecated: use `one_home_screen_for_every_apple_tv` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `tv_home_screen_sync` before 2027-02-03."

// PRO-DEPRECATED remove-after=2027-02-03 replaced-by=skip_setup_items.where_is_this_apple_tv — renamed to match the admin UI label "Where is this Apple TV?".
const skipDeprecationTVRoom = "Deprecated: use `where_is_this_apple_tv` instead, the same pane under the name the admin UI shows. " +
	"The provider will not remove `tv_room` before 2027-02-03."
