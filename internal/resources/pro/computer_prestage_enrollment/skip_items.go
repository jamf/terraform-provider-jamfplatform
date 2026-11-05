// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

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

// skipItems is the whole checklist, in the order the admin UI shows it (26 panes).
var skipItems = []skipItem{
	{name: "get_started", wire: "Welcome", label: "Get Started", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.GetStarted }, deprecated: &skipAlias{name: "welcome", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Welcome }, message: skipDeprecationWelcome}},
	{name: "location", wire: "Location", label: "Location Services", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Location }, deprecated: nil},
	{name: "transfer_information", wire: "Restore", label: "Transfer Information", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TransferInformation }, deprecated: &skipAlias{name: "restore", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Restore }, message: skipDeprecationRestore}},
	{name: "apple_id", wire: "AppleID", label: "Apple ID", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AppleID }, deprecated: nil},
	{name: "terms_and_conditions", wire: "TOS", label: "Terms and Conditions", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TermsAndConditions }, deprecated: &skipAlias{name: "tos", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TOS }, message: skipDeprecationTOS}},
	{name: "siri", wire: "Siri", label: "Siri", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Siri }, deprecated: nil},
	{name: "app_analytics", wire: "Diagnostics", label: "App Analytics", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AppAnalytics }, deprecated: &skipAlias{name: "diagnostics", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Diagnostics }, message: skipDeprecationDiagnostics}},
	{name: "biometric", wire: "Biometric", label: "Touch ID / Face ID", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Biometric }, deprecated: nil},
	{name: "enable_lockdown_mode", wire: "EnableLockdownMode", label: "Enable Lockdown Mode", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.EnableLockdownMode }, deprecated: nil},
	{name: "payment", wire: "Payment", label: "Apple Pay", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Payment }, deprecated: nil},
	{name: "registration", wire: "Registration", label: "Registration", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Registration }, deprecated: nil},
	{name: "privacy", wire: "Privacy", label: "Privacy", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Privacy }, deprecated: nil},
	{name: "additional_privacy_settings", wire: "AdditionalPrivacySettings", label: "Additional Privacy Settings", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AdditionalPrivacySettings }, deprecated: nil},
	{name: "filevault", wire: "FileVault", label: "FileVault", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.FileVault }, deprecated: nil},
	{name: "icloud_diagnostics", wire: "iCloudDiagnostics", label: "iCloud Diagnostics", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ICloudDiagnostics }, deprecated: nil},
	{name: "all_your_files_in_icloud", wire: "iCloudStorage", label: "All Your Files in iCloud", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.AllYourFilesInICloud }, deprecated: &skipAlias{name: "icloud_storage", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ICloudStorage }, message: skipDeprecationICloudStorage}},
	{name: "choose_your_look", wire: "Appearance", label: "Choose your Look", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ChooseYourLook }, deprecated: &skipAlias{name: "appearance", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Appearance }, message: skipDeprecationAppearance}},
	{name: "wallpaper", wire: "Wallpaper", label: "Wallpaper", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Wallpaper }, deprecated: nil},
	{name: "true_tone_display", wire: "DisplayTone", label: "True Tone Display", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TrueToneDisplay }, deprecated: &skipAlias{name: "display_tone", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.DisplayTone }, message: skipDeprecationDisplayTone}},
	{name: "screen_time", wire: "ScreenTime", label: "Screen Time", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.ScreenTime }, deprecated: nil},
	{name: "accessibility", wire: "Accessibility", label: "Accessibility", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Accessibility }, deprecated: nil},
	{name: "terms_of_address", wire: "TermsOfAddress", label: "Terms of Address", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.TermsOfAddress }, deprecated: nil},
	{name: "intelligence", wire: "Intelligence", label: "Intelligence", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.Intelligence }, deprecated: nil},
	{name: "software_update", wire: "SoftwareUpdate", label: "Software Update", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.SoftwareUpdate }, deprecated: nil},
	{name: "os_showcase", wire: "OSShowcase", label: "OS Showcase", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.OSShowcase }, deprecated: nil},
	{name: "liquid_glass", wire: "LiquidGlass", label: "Liquid Glass", field: func(m *SkipSetupItemsModel) *types.Bool { return &m.LiquidGlass }, deprecated: nil},
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
		MarkdownDescription: "Setup Assistant panes to skip during enrolment. Each attribute matches a pane in the *Skip Setup Items* checklist of the Jamf Pro admin UI and takes its name from the pane's label. Set `true` to skip the pane. Supply the block to manage this section, even empty as `skip_setup_items = {}`. Without it, the next refresh shows drift.",
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
