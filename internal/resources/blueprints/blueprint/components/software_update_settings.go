// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// SoftwareUpdateSettingsComponent represents a strongly-typed software update settings component.
type SoftwareUpdateSettingsComponent struct {
	AllowStandardUserOSUpdates           types.Bool         `tfsdk:"allow_standard_user_os_updates"`
	AutomaticDownload                    types.String       `tfsdk:"automatic_download"`
	AutomaticInstallOSUpdates            types.String       `tfsdk:"automatic_install_os_updates"`
	AutomaticInstallSecurityUpdate       types.String       `tfsdk:"automatic_install_security_updates"`
	BetaProgramEnrollment                types.String       `tfsdk:"beta_program_enrollment"`
	BetaOfferPrograms                    []BetaProgramModel `tfsdk:"beta_offer_programs"`
	BetaRequireProgramToken              types.String       `tfsdk:"beta_require_program_token"`
	BetaRequireProgramDescription        types.String       `tfsdk:"beta_require_program_description"`
	DeferralCombinedPeriod               types.Int64        `tfsdk:"deferral_combined_period_days"`
	DeferralMajorPeriod                  types.Int64        `tfsdk:"deferral_major_period_days"`
	DeferralMinorPeriod                  types.Int64        `tfsdk:"deferral_minor_period_days"`
	DeferralSystemPeriod                 types.Int64        `tfsdk:"deferral_system_period_days"`
	NotificationsEnabled                 types.Bool         `tfsdk:"notifications_enabled"`
	RapidSecurityResponseEnabled         types.Bool         `tfsdk:"rapid_security_response_enabled"`
	RapidSecurityResponseRollbackEnabled types.Bool         `tfsdk:"rapid_security_response_rollback_enabled"`
	RecommendedCadence                   types.String       `tfsdk:"recommended_cadence"`
}

// BetaProgramModel represents a beta program configuration.
type BetaProgramModel struct {
	Token       types.String `tfsdk:"token"`
	Description types.String `tfsdk:"description"`
}

// SoftwareUpdateSettingsComponentSchema returns the Terraform schema for software update component.
func SoftwareUpdateSettingsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"allow_standard_user_os_updates": schema.BoolAttribute{
			MarkdownDescription: "Allow standard users to install OS updates without administrator privileges.",
			Optional:            true,
		},
		"automatic_download": schema.StringAttribute{
			MarkdownDescription: "Automatic download behavior for updates. Valid values: `Allowed`, `AlwaysOn`, `AlwaysOff`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.AutomaticActionValueValues()...)},
		},
		"automatic_install_os_updates": schema.StringAttribute{
			MarkdownDescription: "Automatic installation behavior for OS updates. Valid values: `Allowed`, `AlwaysOn`, `AlwaysOff`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.AutomaticActionValueValues()...)},
		},
		"automatic_install_security_updates": schema.StringAttribute{
			MarkdownDescription: "Automatic installation behavior for security updates. Valid values: `Allowed`, `AlwaysOn`, `AlwaysOff`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.AutomaticActionValueValues()...)},
		},
		"beta_program_enrollment": schema.StringAttribute{
			MarkdownDescription: "Beta program enrollment setting. Valid values: `Allowed`, `AlwaysOn`, `AlwaysOff`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.AutomaticActionValueValues()...)},
		},
		"deferral_combined_period_days": schema.Int64Attribute{
			MarkdownDescription: "Number of days to defer combined updates. Range: `1`-`90`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(1, 90)},
		},
		"deferral_major_period_days": schema.Int64Attribute{
			MarkdownDescription: "Number of days to defer major updates. Range: `1`-`90`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(1, 90)},
		},
		"deferral_minor_period_days": schema.Int64Attribute{
			MarkdownDescription: "Number of days to defer minor updates. Range: `1`-`90`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(1, 90)},
		},
		"deferral_system_period_days": schema.Int64Attribute{
			MarkdownDescription: "Number of days to defer system updates. Range: `1`-`90`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(1, 90)},
		},
		"notifications_enabled": schema.BoolAttribute{
			MarkdownDescription: "Enable update notifications to users.",
			Optional:            true,
		},
		"rapid_security_response_enabled": schema.BoolAttribute{
			MarkdownDescription: "Enable Rapid Security Response updates.",
			Optional:            true,
		},
		"rapid_security_response_rollback_enabled": schema.BoolAttribute{
			MarkdownDescription: "Enable rollback capability for Rapid Security Response updates.",
			Optional:            true,
		},
		"recommended_cadence": schema.StringAttribute{
			MarkdownDescription: "Recommended update cadence policy. Valid values: `All`, `Oldest`, `Newest`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.RecommendedCadenceValueValues()...)},
		},
		"beta_require_program_token": schema.StringAttribute{
			MarkdownDescription: "Required beta program token (1–1000 characters). Must be specified with `beta_require_program_description`.",
			Optional:            true,
		},
		"beta_require_program_description": schema.StringAttribute{
			MarkdownDescription: "Required beta program description (1–1000 characters). Must be specified with `beta_require_program_token`.",
			Optional:            true,
		},
		"beta_offer_programs": schema.SetNestedAttribute{
			MarkdownDescription: "Beta programs to offer (max 100). Each program must have a token and description (1–1000 characters each).",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"token": schema.StringAttribute{
						MarkdownDescription: "Beta program token (1–1000 characters).",
						Required:            true,
					},
					"description": schema.StringAttribute{
						MarkdownDescription: "Beta program description (1–1000 characters).",
						Required:            true,
					},
				},
			},
		},
	}
}

// buildOptionallyEnabled builds an OptionallyEnabled wrapper for a bool field.
func buildOptionallyEnabled(field types.Bool, defaultValue bool) *blueprints.OptionallyEnabled {
	enabled := defaultValue
	if helpers.IsConfiguredValue(field) {
		enabled = field.ValueBool()
	}
	return &blueprints.OptionallyEnabled{
		Enabled:  enabled,
		Included: new(helpers.IsConfiguredValue(field)),
	}
}

// buildAutomaticAction builds an AutomaticAction wrapper for a string field.
func buildAutomaticAction(field types.String, defaultValue string) *blueprints.AutomaticAction {
	value := defaultValue
	if helpers.IsConfiguredValue(field) {
		value = field.ValueString()
	}
	return &blueprints.AutomaticAction{
		Included: new(helpers.IsConfiguredValue(field)),
		Value:    value,
	}
}

// buildOptionalPeriodInDays builds an OptionalPeriodInDays wrapper for an int64 field.
func buildOptionalPeriodInDays(field types.Int64) *blueprints.OptionalPeriodInDays {
	if !helpers.IsConfiguredValue(field) {
		return &blueprints.OptionalPeriodInDays{Included: new(false)}
	}
	v := int(field.ValueInt64())
	return &blueprints.OptionalPeriodInDays{Included: new(true), Value: &v}
}

// ToRawConfiguration converts the strongly-typed component to the OpenAPI nested format.
func (c *SoftwareUpdateSettingsComponent) ToRawConfiguration() (json.RawMessage, error) {
	cfg := blueprints.SoftwareUpdateSettingsConfiguration{}

	cfg.AllowStandardUserOSUpdates = buildOptionallyEnabled(c.AllowStandardUserOSUpdates, false)

	cfg.AutomaticActions = &blueprints.AutomaticActions{
		Download:              buildAutomaticAction(c.AutomaticDownload, blueprints.AutomaticActionValueAllowed),
		InstallOSUpdates:      buildAutomaticAction(c.AutomaticInstallOSUpdates, blueprints.AutomaticActionValueAllowed),
		InstallSecurityUpdate: buildAutomaticAction(c.AutomaticInstallSecurityUpdate, blueprints.AutomaticActionValueAllowed),
	}

	hasBetaSettings := helpers.IsConfiguredValue(c.BetaProgramEnrollment) ||
		len(c.BetaOfferPrograms) > 0 ||
		(helpers.IsConfiguredValue(c.BetaRequireProgramToken) && helpers.IsConfiguredValue(c.BetaRequireProgramDescription))

	if hasBetaSettings {
		trueVal := true
		betaValue := &blueprints.BetaSettings{}

		if helpers.IsConfiguredValue(c.BetaProgramEnrollment) {
			betaValue.ProgramEnrollment = c.BetaProgramEnrollment.ValueString()
		}

		if len(c.BetaOfferPrograms) > 0 {
			offerPrograms := make([]blueprints.BetaProgram, len(c.BetaOfferPrograms))
			for i, p := range c.BetaOfferPrograms {
				offerPrograms[i] = blueprints.BetaProgram{
					Token:       p.Token.ValueStringPointer(),
					Description: p.Description.ValueStringPointer(),
				}
			}
			betaValue.OfferPrograms = &offerPrograms
		}

		if helpers.IsConfiguredValue(c.BetaRequireProgramToken) && helpers.IsConfiguredValue(c.BetaRequireProgramDescription) {
			betaValue.RequireProgram = &blueprints.BetaProgram{
				Token:       c.BetaRequireProgramToken.ValueStringPointer(),
				Description: c.BetaRequireProgramDescription.ValueStringPointer(),
			}
		}

		cfg.Beta = &blueprints.Beta{Included: &trueVal, Value: betaValue}
	}

	cfg.Deferrals = &blueprints.Deferrals{
		CombinedPeriodInDays: buildOptionalPeriodInDays(c.DeferralCombinedPeriod),
		MajorPeriodInDays:    buildOptionalPeriodInDays(c.DeferralMajorPeriod),
		MinorPeriodInDays:    buildOptionalPeriodInDays(c.DeferralMinorPeriod),
		SystemPeriodInDays:   buildOptionalPeriodInDays(c.DeferralSystemPeriod),
	}

	cfg.Notifications = buildOptionallyEnabled(c.NotificationsEnabled, false)

	cfg.RapidSecurityResponse = &blueprints.RapidSecurityResponse{
		Enable:         buildOptionallyEnabled(c.RapidSecurityResponseEnabled, false),
		EnableRollback: buildOptionallyEnabled(c.RapidSecurityResponseRollbackEnabled, false),
	}

	cadenceValue := blueprints.RecommendedCadenceValueAll
	if helpers.IsConfiguredValue(c.RecommendedCadence) {
		cadenceValue = c.RecommendedCadence.ValueString()
	}
	cfg.RecommendedCadence = &blueprints.RecommendedCadence{
		Included: new(helpers.IsConfiguredValue(c.RecommendedCadence)),
		Value:    cadenceValue,
	}

	return json.Marshal(cfg)
}

// FromRawConfiguration populates the strongly-typed component from OpenAPI nested configuration.
func (c *SoftwareUpdateSettingsComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg blueprints.SoftwareUpdateSettingsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	c.AllowStandardUserOSUpdates = types.BoolNull()
	c.AutomaticDownload = types.StringNull()
	c.AutomaticInstallOSUpdates = types.StringNull()
	c.AutomaticInstallSecurityUpdate = types.StringNull()
	c.BetaProgramEnrollment = types.StringNull()
	c.BetaOfferPrograms = nil
	c.BetaRequireProgramToken = types.StringNull()
	c.BetaRequireProgramDescription = types.StringNull()
	c.DeferralCombinedPeriod = types.Int64Null()
	c.DeferralMajorPeriod = types.Int64Null()
	c.DeferralMinorPeriod = types.Int64Null()
	c.DeferralSystemPeriod = types.Int64Null()
	c.NotificationsEnabled = types.BoolNull()
	c.RapidSecurityResponseEnabled = types.BoolNull()
	c.RapidSecurityResponseRollbackEnabled = types.BoolNull()
	c.RecommendedCadence = types.StringNull()

	if f := cfg.AllowStandardUserOSUpdates; f != nil && f.Included != nil && *f.Included {
		c.AllowStandardUserOSUpdates = types.BoolValue(f.Enabled)
	}

	if aa := cfg.AutomaticActions; aa != nil {
		if f := aa.Download; f != nil && f.Included != nil && *f.Included {
			c.AutomaticDownload = types.StringValue(f.Value)
		}
		if f := aa.InstallOSUpdates; f != nil && f.Included != nil && *f.Included {
			c.AutomaticInstallOSUpdates = types.StringValue(f.Value)
		}
		if f := aa.InstallSecurityUpdate; f != nil && f.Included != nil && *f.Included {
			c.AutomaticInstallSecurityUpdate = types.StringValue(f.Value)
		}
	}

	if beta := cfg.Beta; beta != nil && beta.Included != nil && *beta.Included && beta.Value != nil {
		bv := beta.Value
		if bv.ProgramEnrollment != "" {
			c.BetaProgramEnrollment = types.StringValue(bv.ProgramEnrollment)
		}
		if bv.OfferPrograms != nil {
			programs := make([]BetaProgramModel, len(*bv.OfferPrograms))
			for i, p := range *bv.OfferPrograms {
				programs[i] = BetaProgramModel{
					Token:       types.StringPointerValue(p.Token),
					Description: types.StringPointerValue(p.Description),
				}
			}
			c.BetaOfferPrograms = programs
		}
		if bv.RequireProgram != nil {
			c.BetaRequireProgramToken = types.StringPointerValue(bv.RequireProgram.Token)
			c.BetaRequireProgramDescription = types.StringPointerValue(bv.RequireProgram.Description)
		}
	}

	if d := cfg.Deferrals; d != nil {
		if f := d.CombinedPeriodInDays; f != nil && f.Included != nil && *f.Included && f.Value != nil {
			c.DeferralCombinedPeriod = types.Int64Value(int64(*f.Value))
		}
		if f := d.MajorPeriodInDays; f != nil && f.Included != nil && *f.Included && f.Value != nil {
			c.DeferralMajorPeriod = types.Int64Value(int64(*f.Value))
		}
		if f := d.MinorPeriodInDays; f != nil && f.Included != nil && *f.Included && f.Value != nil {
			c.DeferralMinorPeriod = types.Int64Value(int64(*f.Value))
		}
		if f := d.SystemPeriodInDays; f != nil && f.Included != nil && *f.Included && f.Value != nil {
			c.DeferralSystemPeriod = types.Int64Value(int64(*f.Value))
		}
	}

	if f := cfg.Notifications; f != nil && f.Included != nil && *f.Included {
		c.NotificationsEnabled = types.BoolValue(f.Enabled)
	}

	if rsr := cfg.RapidSecurityResponse; rsr != nil {
		if f := rsr.Enable; f != nil && f.Included != nil && *f.Included {
			c.RapidSecurityResponseEnabled = types.BoolValue(f.Enabled)
		}
		if f := rsr.EnableRollback; f != nil && f.Included != nil && *f.Included {
			c.RapidSecurityResponseRollbackEnabled = types.BoolValue(f.Enabled)
		}
	}

	if f := cfg.RecommendedCadence; f != nil && f.Included != nil && *f.Included {
		c.RecommendedCadence = types.StringValue(f.Value)
	}

	return nil
}

// GetIdentifier returns the component identifier for software update settings.
func (c *SoftwareUpdateSettingsComponent) GetIdentifier() string {
	return "com.jamf.ddm.software-update-settings"
}

// ToClientComponent converts the strongly-typed component to a blueprints.Component.
func (c *SoftwareUpdateSettingsComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
