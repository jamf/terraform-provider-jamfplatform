// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/prestagealias"
)

func useStateForUnknownBool() planmodifier.Bool { return boolplanmodifier.UseStateForUnknown() }

func existingBlockBoolModifier() planmodifier.Bool {
	return planmodifiers.UseStateForUnknownInExistingBlockBool()
}

func existingBlockStringModifier() planmodifier.String {
	return planmodifiers.UseStateForUnknownInExistingBlockString()
}

// boolRename is one boolean attribute the admin UI names differently from the attribute this
// resource shipped with. oldName is the deprecated attribute and newName the one that takes the
// admin UI's label. invert marks a pair that holds opposite values. detail is appended to the
// description of both attributes, and newValidators and mods apply to both.
type boolRename struct {
	oldName       string
	newName       string
	label         string
	detail        string
	message       string
	invert        bool
	oldField      func(*MobileDevicePrestageEnrollmentResourceModel) *types.Bool
	newField      func(*MobileDevicePrestageEnrollmentResourceModel) *types.Bool
	newValidators []validator.Bool
	mods          []planmodifier.Bool
}

// boolRenames lists every renamed boolean, in the order the admin UI shows them. The wire
// field behind each is the SDK field named in the Wire comment.
func boolRenames() []boolRename {
	return []boolRename{
		{
			// Wire field: mandatory.
			oldName: "mandatory", newName: "make_mdm_profile_mandatory",
			label:    "Make MDM Profile Mandatory for devices with iOS 12.x or earlier",
			message:  deprecationMandatory,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.Mandatory },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.MakeMdmProfileMandatory },
		},
		{
			// Wire field: mdmRemovable, which holds the opposite of the admin UI checkbox.
			oldName: "mdm_removable", newName: "prevent_unenrollment",
			label:    "Prevent Unenrollment",
			detail:   "Disallows the user from removing the MDM profile. `true` is the opposite of the deprecated `mdm_removable`.",
			message:  deprecationMdmRemovable,
			invert:   true,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.MdmRemovable },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.PreventUnenrollment },
		},
		{
			// Wire field: requireAuthentication.
			oldName: "require_authentication", newName: "require_credentials_for_enrollment",
			label:    "Require Credentials for Enrollment",
			detail:   "Jamf Pro ignores it for devices with iOS 13 or later when `enrollment_customization_id` names an enrollment customization. Configure authentication in the enrollment customization instead.",
			message:  deprecationRequireAuthentication,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.RequireAuthentication },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.RequireCredentialsForEnrollment
			},
		},
		{
			// Wire field: supervised.
			oldName: "supervised", newName: "supervise_devices_with_ios_12_or_earlier",
			label:    "Supervise Devices with iOS 12.x or earlier",
			message:  deprecationSupervised,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.Supervised },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.SuperviseDevicesWithIos12OrEarlier
			},
		},
		{
			// Wire field: autoAdvanceSetup.
			oldName: "auto_advance_setup", newName: "automatically_advance_through_setup_assistant",
			label:    "Automatically advance through Setup Assistant (tvOS only)",
			message:  deprecationAutoAdvanceSetup,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.AutoAdvanceSetup },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.AutomaticallyAdvanceThroughSetupAssistant
			},
		},
		{
			// Wire field: configureDeviceBeforeSetupAssistant.
			oldName: "configure_device_before_setup_assistant", newName: "install_configuration_profiles_before_setup_assistant",
			label:   "Install configuration profiles before Setup Assistant",
			message: deprecationConfigureDeviceBeforeSetupAssistant,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.ConfigureDeviceBeforeSetupAssistant
			},
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.InstallConfigurationProfilesBeforeSetupAssistant
			},
		},
		{
			// Wire field: installAppsDuringEnrollment.
			oldName: "install_apps_during_enrollment", newName: "install_managed_apps_before_setup_assistant",
			label:   "Install managed apps before Setup Assistant",
			message: deprecationInstallAppsDuringEnrollment,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.InstallAppsDuringEnrollment
			},
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.InstallManagedAppsBeforeSetupAssistant
			},
		},
		{
			// Wire field: defaultPrestage. Jamf Pro allows at most one default PreStage per device
			// type. Claiming it while another PreStage holds it is rejected with
			// 400 ALREADY_DEFAULT and nothing is stored, so ModifyPlan leaves the attribute alone and
			// diagnoseAlreadyDefault turns the error into an actionable diagnostic.
			oldName: "default_prestage", newName: "automatically_assign_new_devices",
			label:    "Automatically assign new devices",
			detail:   "When true, this PreStage becomes the tenant default for new devices. Jamf Pro allows at most one default PreStage and will not move the default automatically: if another PreStage already holds it, the apply fails. Set it to `false` on the current holder first. Where both are managed by Terraform, make the release land before the claim: apply it on its own, or add a `depends_on` from this resource to the one giving up the default.",
			message:  deprecationDefaultPrestage,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.DefaultPrestage },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.AutomaticallyAssignNewDevices
			},
			mods: []planmodifier.Bool{useStateForUnknownBool()},
		},
		{
			// Wire field: sendTimezone.
			oldName: "send_timezone", newName: "set_time_zone",
			label:    "Set Time Zone",
			detail:   "Sends `timezone` to the device.",
			message:  deprecationSendTimezone,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.SendTimezone },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.SetTimeZone },
		},
		{
			// Wire field: multiUser.
			oldName: "multi_user", newName: "enable_shared_ipad",
			label:         "Enable Shared iPad",
			detail:        "Requires `prevent_activation_lock = true` and supervision. Jamf Pro rejects Shared iPad without `prevent_activation_lock`, and turns it off without an error on a device that is not supervised.",
			message:       deprecationMultiUser,
			oldField:      func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.MultiUser },
			newField:      func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.EnableSharedIPad },
			newValidators: []validator.Bool{sharedIPadRequiresPreventActivationLock(), sharedIPadRequiresSupervision()},
			mods:          []planmodifier.Bool{useStateForUnknownBool()},
		},
		{
			// Wire field: rtsEnabled.
			oldName: "rts_enabled", newName: "allow_return_to_service",
			label:    "Allow use of the Return to Service option during device reset",
			message:  deprecationRtsEnabled,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.RtsEnabled },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.AllowReturnToService },
		},
		{
			// Wire field: keepExistingSiteMembership.
			oldName: "keep_existing_site_membership", newName: "use_existing_site_membership",
			label:    "Use existing site membership, if applicable",
			message:  deprecationKeepExistingSiteMembership,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.KeepExistingSiteMembership },
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool { return &m.UseExistingSiteMembership },
		},
		{
			// Wire field: keepExistingLocationInformation.
			oldName: "keep_existing_location_information", newName: "use_existing_location_information",
			label:   "Use existing location information, if applicable",
			message: deprecationKeepExistingLocationInformation,
			oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.KeepExistingLocationInformation
			},
			newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.Bool {
				return &m.UseExistingLocationInformation
			},
		},
	}
}

// stringRename is the one renamed string attribute.
type stringRename struct {
	oldName  string
	newName  string
	label    string
	detail   string
	message  string
	oldField func(*MobileDevicePrestageEnrollmentResourceModel) *types.String
	newField func(*MobileDevicePrestageEnrollmentResourceModel) *types.String
}

// authenticationMessageRename covers authentication_prompt. Wire field: authenticationPrompt.
func authenticationMessageRename() stringRename {
	return stringRename{
		oldName: "authentication_prompt", newName: "authentication_message",
		label:    "Authentication Message",
		detail:   "Shown to the user when `require_credentials_for_enrollment = true`.",
		message:  deprecationAuthenticationPrompt,
		oldField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.String { return &m.AuthenticationPrompt },
		newField: func(m *MobileDevicePrestageEnrollmentResourceModel) *types.String { return &m.AuthenticationMessage },
	}
}

// renamePairs lists every renamed attribute for prestagealias.
func renamePairs() []prestagealias.Pair {
	var out []prestagealias.Pair
	for _, r := range boolRenames() {
		out = append(out, prestagealias.Pair{Old: r.oldName, New: r.newName, Invert: r.invert})
	}
	s := authenticationMessageRename()
	return append(out, prestagealias.Pair{Old: s.oldName, New: s.newName})
}

// addRenamedAttributes writes the replacement and the deprecated attribute of every rename into
// attrs. Both are Optional and Computed and share one set of plan modifiers, so a configuration
// may use either name and a refreshed plan stays empty.
func addRenamedAttributes(attrs map[string]schema.Attribute) {
	for _, r := range boolRenames() {
		mods := r.mods
		if mods == nil {
			mods = []planmodifier.Bool{existingBlockBoolModifier()}
		}
		desc := "**\"" + r.label + "\"** in the Jamf Pro admin UI."
		if r.detail != "" {
			desc += " " + r.detail
		}
		attrs[r.newName] = schema.BoolAttribute{
			MarkdownDescription: desc + " Replaces `" + r.oldName + "`; set one or the other.",
			Optional:            true,
			Computed:            true,
			Validators:          r.newValidators,
			PlanModifiers:       mods,
		}
		attrs[r.oldName] = schema.BoolAttribute{
			MarkdownDescription: "~> " + r.message + "\n\n" + desc,
			DeprecationMessage:  r.message,
			Optional:            true,
			Computed:            true,
			Validators:          append(prestagealias.BoolConflicts(r.newName), r.newValidators...),
			PlanModifiers:       mods,
		}
	}
	s := authenticationMessageRename()
	desc := "**\"" + s.label + "\"** in the Jamf Pro admin UI. " + s.detail
	attrs[s.newName] = schema.StringAttribute{
		MarkdownDescription: desc + " Replaces `" + s.oldName + "`; set one or the other.",
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{existingBlockStringModifier()},
	}
	attrs[s.oldName] = schema.StringAttribute{
		MarkdownDescription: "~> " + s.message + "\n\n" + desc,
		DeprecationMessage:  s.message,
		Optional:            true,
		Computed:            true,
		Validators:          prestagealias.StringConflicts(s.newName),
		PlanModifiers:       []planmodifier.String{existingBlockStringModifier()},
	}
}

// pickBool returns the value the write should carry for one pair: the replacement when it holds
// one, then the deprecated attribute (negated when invert is set), and the replacement's own
// null or unknown otherwise.
func pickBool(replacement, deprecated types.Bool, invert bool) types.Bool {
	if !replacement.IsNull() && !replacement.IsUnknown() {
		return replacement
	}
	if !deprecated.IsNull() && !deprecated.IsUnknown() {
		return types.BoolValue(deprecated.ValueBool() != invert)
	}
	return replacement
}

func pickString(replacement, deprecated types.String) types.String {
	if !replacement.IsNull() && !replacement.IsUnknown() {
		return replacement
	}
	if !deprecated.IsNull() && !deprecated.IsUnknown() {
		return deprecated
	}
	return replacement
}

// resolveRenames folds every deprecated attribute into its replacement. Builders call it on their
// own copy of the model, so the rest of the write path reads only the current names.
func resolveRenames(m *MobileDevicePrestageEnrollmentResourceModel) {
	for _, r := range boolRenames() {
		*r.newField(m) = pickBool(*r.newField(m), *r.oldField(m), r.invert)
	}
	s := authenticationMessageRename()
	*s.newField(m) = pickString(*s.newField(m), *s.oldField(m))
}

// assignRenamed stores the wire values of every renamed attribute. The replacement always takes
// the wire value. The deprecated attribute takes it only while the model already holds one, so
// import and config generation produce the current names alone. Call it before the model's
// other fields are overwritten, because the model is the prior for the deprecated attribute.
func assignRenamed(m *MobileDevicePrestageEnrollmentResourceModel, wire map[string]bool, authMessage string) {
	for _, r := range boolRenames() {
		v := wire[r.oldName]
		*r.oldField(m) = prestagealias.CarryBool(*r.oldField(m), v != r.invert, r.invert)
		*r.newField(m) = types.BoolValue(v != r.invert)
	}
	s := authenticationMessageRename()
	*s.oldField(m) = prestagealias.CarryString(*s.oldField(m), authMessage)
	*s.newField(m) = types.StringValue(authMessage)
}
