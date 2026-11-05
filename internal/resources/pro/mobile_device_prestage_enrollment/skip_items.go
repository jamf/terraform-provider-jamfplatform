// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/prestageskip"
)

// skipItem is one Setup Assistant pane. name is the Terraform attribute, wire the key Jamf Pro
// stores it under, and label the text the admin UI shows. deprecated is the older attribute
// for the same wire key, kept for the deprecation window.
type skipItem struct {
	name       string
	wire       string
	label      string
	field      func(*SkipSetupItemsModel) *types.Bool
	deprecated *skipAlias
}

// skipAlias is a deprecated attribute for a skipItem.
type skipAlias struct {
	name    string
	field   func(*SkipSetupItemsModel) *types.Bool
	message string
}

// skipItems is the whole checklist, in the order the admin UI shows it (47 panes).
var skipItems = []skipItem{
	{name: "location", wire: "Location", label: "Location Services", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Location }, deprecated: nil},
	{name: "siri", wire: "Siri", label: "Siri", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Siri }, deprecated: nil},
	{name: "apple_id", wire: "AppleID", label: "Apple ID and iCloud", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AppleID }, deprecated: nil},
	{name: "terms_and_conditions", wire: "TOS", label: "Terms and Conditions", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TermsAndConditions }, deprecated: &skipAlias{name: "tos", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TOS }, message: skipDeprecationTOS}},
	{name: "app_analytics", wire: "Diagnostics", label: "App Analytics", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AppAnalytics }, deprecated: &skipAlias{name: "diagnostics", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Diagnostics }, message: skipDeprecationDiagnostics}},
	{name: "privacy", wire: "Privacy", label: "Privacy", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Privacy }, deprecated: nil},
	{name: "apps_and_data", wire: "Restore", label: "Apps & Data", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AppsAndData }, deprecated: &skipAlias{name: "restore", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Restore }, message: skipDeprecationRestore}},
	{name: "android", wire: "Android", label: "Move from Android", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Android }, deprecated: nil},
	{name: "passcode", wire: "Passcode", label: "Passcode Lock", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Passcode }, deprecated: nil},
	{name: "add_cellular_plan", wire: "SIMSetup", label: "Add Cellular Plan", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AddCellularPlan }, deprecated: &skipAlias{name: "sim_setup", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SIMSetup }, message: skipDeprecationSIMSetup}},
	{name: "home_button_sensitivity", wire: "HomeButtonSensitivity", label: "Home Button Sensitivity", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.HomeButtonSensitivity }, deprecated: nil},
	{name: "biometric", wire: "Biometric", label: "Touch ID / Face ID", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Biometric }, deprecated: nil},
	{name: "enable_lockdown_mode", wire: "EnableLockdownMode", label: "Enable Lockdown Mode", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.EnableLockdownMode }, deprecated: nil},
	{name: "payment", wire: "Payment", label: "Apple Pay", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Payment }, deprecated: nil},
	{name: "display_zoom", wire: "Zoom", label: "Display Zoom", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.DisplayZoom }, deprecated: &skipAlias{name: "zoom", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Zoom }, message: skipDeprecationZoom}},
	{name: "true_tone_display", wire: "DisplayTone", label: "True Tone Display", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TrueToneDisplay }, deprecated: &skipAlias{name: "display_tone", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.DisplayTone }, message: skipDeprecationDisplayTone}},
	{name: "cloud_storage", wire: "CloudStorage", label: "Cloud Storage", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.CloudStorage }, deprecated: nil},
	{name: "apple_watch", wire: "WatchMigration", label: "Apple Watch", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AppleWatch }, deprecated: &skipAlias{name: "watch_migration", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.WatchMigration }, message: skipDeprecationWatchMigration}},
	{name: "new_feature_highlights", wire: "OnBoarding", label: "New Feature Highlights", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.NewFeatureHighlights }, deprecated: &skipAlias{name: "onboarding", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.OnBoarding }, message: skipDeprecationOnBoarding}},
	{name: "imessage_and_facetime", wire: "iMessageAndFaceTime", label: "iMessage and FaceTime", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.IMessageAndFaceTime }, deprecated: nil},
	{name: "screen_time", wire: "ScreenTime", label: "Screen Time", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ScreenTime }, deprecated: nil},
	{name: "software_update", wire: "SoftwareUpdate", label: "Software Update", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SoftwareUpdate }, deprecated: nil},
	{name: "choose_your_look", wire: "Appearance", label: "Choose your Look", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ChooseYourLook }, deprecated: &skipAlias{name: "appearance", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Appearance }, message: skipDeprecationAppearance}},
	{name: "express_language", wire: "ExpressLanguage", label: "Express Language Setup", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ExpressLanguage }, deprecated: nil},
	{name: "preferred_language", wire: "PreferredLanguage", label: "Preferred Language Order", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.PreferredLanguage }, deprecated: nil},
	{name: "get_started", wire: "Welcome", label: "Get Started", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.GetStarted }, deprecated: &skipAlias{name: "welcome", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Welcome }, message: skipDeprecationWelcome}},
	{name: "transfer_data", wire: "TransferData", label: "Transfer Data", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TransferData }, deprecated: nil},
	{name: "restore_completed", wire: "RestoreCompleted", label: "Restore Completed", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.RestoreCompleted }, deprecated: nil},
	{name: "software_update_completed", wire: "UpdateCompleted", label: "Software Update Completed", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SoftwareUpdateCompleted }, deprecated: &skipAlias{name: "update_completed", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.UpdateCompleted }, message: skipDeprecationUpdateCompleted}},
	{name: "emergency_sos", wire: "Safety", label: "Emergency SOS", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.EmergencySOS }, deprecated: &skipAlias{name: "safety", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Safety }, message: skipDeprecationSafety}},
	{name: "safety_and_handling", wire: "SafetyAndHandling", label: "Safety and Handling", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SafetyAndHandling }, deprecated: nil},
	{name: "terms_of_address", wire: "TermsOfAddress", label: "Terms of Address", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TermsOfAddress }, deprecated: nil},
	{name: "voice_selection", wire: "VoiceSelection", label: "Voice Selection", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.VoiceSelection }, deprecated: nil},
	{name: "intelligence", wire: "Intelligence", label: "Intelligence", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Intelligence }, deprecated: nil},
	{name: "action_button", wire: "ActionButton", label: "Action Button", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ActionButton }, deprecated: nil},
	{name: "camera_button", wire: "CameraButton", label: "Camera Button", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.CameraButton }, deprecated: nil},
	{name: "multitasking", wire: "Multitasking", label: "Multitasking", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Multitasking }, deprecated: nil},
	{name: "os_showcase", wire: "OSShowcase", label: "OS Showcase", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.OSShowcase }, deprecated: nil},
	{name: "dictation", wire: "SpokenLanguage", label: "Dictation", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Dictation }, deprecated: &skipAlias{name: "spoken_language", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SpokenLanguage }, message: skipDeprecationSpokenLanguage}},
	{name: "keyboard", wire: "Keyboard", label: "Keyboard", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Keyboard }, deprecated: nil},
	{name: "device_features_tour", wire: "DeviceFeaturesTour", label: "Device Features Tour", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.DeviceFeaturesTour }, deprecated: nil},
	{name: "liquid_glass", wire: "LiquidGlass", label: "Liquid Glass", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.LiquidGlass }, deprecated: nil},
	{name: "set_up_your_apple_tv", wire: "TapToSetup", label: "Set Up Your Apple TV", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SetUpYourAppleTV }, deprecated: &skipAlias{name: "tap_to_setup", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TapToSetup }, message: skipDeprecationTapToSetup}},
	{name: "see_the_world_screensaver", wire: "ScreenSaver", label: "See the World Screensaver", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SeeTheWorldScreensaver }, deprecated: &skipAlias{name: "screen_saver", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ScreenSaver }, message: skipDeprecationScreenSaver}},
	{name: "one_home_screen_for_every_apple_tv", wire: "TVHomeScreenSync", label: "One Home Screen for Every Apple TV", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.OneHomeScreenForEveryAppleTV }, deprecated: &skipAlias{name: "tv_home_screen_sync", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TVHomeScreenSync }, message: skipDeprecationTVHomeScreenSync}},
	{name: "tv_provider_sign_in", wire: "TVProviderSignIn", label: "Sign In to Your TV Provider", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TVProviderSignIn }, deprecated: nil},
	{name: "where_is_this_apple_tv", wire: "TVRoom", label: "Where is this Apple TV?", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.WhereIsThisAppleTV }, deprecated: &skipAlias{name: "tv_room", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TVRoom }, message: skipDeprecationTVRoom}},
}

// skipAliases lists the deprecated attributes with their replacements, for prestageskip.
func skipAliases() []prestageskip.Alias {
	var out []prestageskip.Alias
	for _, it := range skipItems {
		if it.deprecated != nil {
			out = append(out, prestageskip.Alias{Old: it.deprecated.name, New: it.name})
		}
	}
	return out
}

// skipSetupItemsSchema returns the Setup Assistant pane-skip checklist. The block is Optional
// only because the model is a typed pointer; each pane inside it is Optional and Computed.
func skipSetupItemsSchema() schema.SingleNestedAttribute {
	attrs := make(map[string]schema.Attribute, len(skipItems)*2)
	for _, it := range skipItems {
		desc := "**\"" + it.label + "\"** pane."
		if it.deprecated != nil {
			desc += " Replaces `" + it.deprecated.name + "`; set one or the other."
			attrs[it.deprecated.name] = schema.BoolAttribute{
				MarkdownDescription: "~> " + it.deprecated.message + "\n\n" + "**\"" + it.label + "\"** pane.",
				DeprecationMessage:  it.deprecated.message,
				Optional:            true,
				Computed:            true,
				Validators:          prestageskip.Conflicts(it.name),
			}
		}
		attrs[it.name] = prestageskip.PaneBool(desc)
	}
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Setup Assistant panes to skip during enrolment. Each attribute matches a pane in the *Skip Setup Items* checklist of the Jamf Pro admin UI and takes its name from the pane's label. Set `true` to skip the pane. Supply the block, even empty (`skip_setup_items = {}`), to manage this section. Without it, the next refresh shows drift.",
		Optional:            true,
		Attributes:          attrs,
	}
}

// buildSkipSetupItemsMap builds the wire map for a create, sending every pane. A null or unknown
// value serialises as false. Returns nil when the user omitted the block.
func buildSkipSetupItemsMap(m *SkipSetupItemsModel) *map[string]bool {
	return buildSkipSetupItemsMapOver(m, nil)
}

// buildSkipSetupItemsMapOver builds the wire map for an update. server is the map Jamf Pro
// returned just before the write: a pane the plan leaves unknown or null keeps that value, so a
// plan made with -refresh=false on state that predates a pane cannot switch on a pane someone
// else turned on in the admin UI.
func buildSkipSetupItemsMapOver(m *SkipSetupItemsModel, server map[string]bool) *map[string]bool {
	if m == nil {
		return nil
	}
	out := make(map[string]bool, len(skipItems))
	for _, it := range skipItems {
		held := server[it.wire]
		if it.deprecated != nil {
			out[it.wire] = prestageskip.EffectiveOr(*it.field(m), *it.deprecated.field(m), held)
			continue
		}
		out[it.wire] = prestageskip.EffectiveOr(*it.field(m), types.BoolNull(), held)
	}
	return &out
}

// flattenSkipSetupItems maps the wire map onto the model. prior is the model being written (the
// plan on Create and Update, prior state on Read); it decides whether a deprecated attribute is
// populated, so import and config generation produce only the current names.
func flattenSkipSetupItems(m map[string]bool, prior *SkipSetupItemsModel) *SkipSetupItemsModel {
	if m == nil {
		return nil
	}
	out := &SkipSetupItemsModel{}
	for _, it := range skipItems {
		*it.field(out) = types.BoolValue(m[it.wire])
		if it.deprecated != nil {
			old := types.BoolNull()
			if prior != nil {
				old = *it.deprecated.field(prior)
			}
			*it.deprecated.field(out) = prestageskip.CarryAlias(old, m[it.wire])
		}
	}
	return out
}

// diffSkipSetupItems reports each pane whose planned value differs from the wire.
func diffSkipSetupItems(plan *SkipSetupItemsModel, got map[string]bool, out *[]string) {
	if plan == nil || got == nil {
		return
	}
	check := func(name, wireKey string, v types.Bool) {
		if v.IsNull() || v.IsUnknown() {
			return
		}
		if v.ValueBool() != got[wireKey] {
			*out = append(*out, "skip_setup_items."+name)
		}
	}
	for _, it := range skipItems {
		check(it.name, it.wire, *it.field(plan))
		if it.deprecated != nil {
			check(it.deprecated.name, it.wire, *it.deprecated.field(plan))
		}
	}
}

// SkipSetupWireKeys returns the wire keys of every pane this resource manages, sorted. The
// skip-key canary in scripts/skipkeycanary diffs them against what Jamf Pro returns.
func SkipSetupWireKeys() []string {
	keys := make([]string, 0, len(skipItems))
	for _, it := range skipItems {
		keys = append(keys, it.wire)
	}
	sort.Strings(keys)
	return keys
}
