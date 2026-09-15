// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// PasscodePolicyComponent represents a strongly-typed passcode policy component.
type PasscodePolicyComponent struct {
	ChangeAtNextAuth             types.Bool   `tfsdk:"change_at_next_auth"`
	CustomRegexPattern           types.String `tfsdk:"custom_regex_pattern"`
	CustomRegexDescription       types.Map    `tfsdk:"custom_regex_description"`
	FailedAttemptsResetInMinutes types.Int64  `tfsdk:"failed_attempts_reset_in_minutes"`
	MaximumFailedAttempts        types.Int64  `tfsdk:"maximum_failed_attempts"`
	MaximumGracePeriodInMinutes  types.Int64  `tfsdk:"maximum_grace_period_in_minutes"`
	MaximumInactivityInMinutes   types.Int64  `tfsdk:"maximum_inactivity_in_minutes"`
	MaximumPasscodeAgeInDays     types.Int64  `tfsdk:"maximum_passcode_age_in_days"`
	MinimumComplexCharacters     types.Int64  `tfsdk:"minimum_complex_characters"`
	MinimumLength                types.Int64  `tfsdk:"minimum_length"`
	PasscodeReuseLimit           types.Int64  `tfsdk:"passcode_reuse_limit"`
	RequireAlphanumericPasscode  types.Bool   `tfsdk:"require_alphanumeric_passcode"`
	RequireComplexPasscode       types.Bool   `tfsdk:"require_complex_passcode"`
	RequirePasscode              types.Bool   `tfsdk:"require_passcode"`
}

// GetIdentifier returns the component identifier for passcode policy.
func (c *PasscodePolicyComponent) GetIdentifier() string {
	return "com.jamf.ddm.passcode-settings"
}

// PasscodePolicyComponentSchema returns the Terraform schema for passcode policy component.
func PasscodePolicyComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"change_at_next_auth": schema.BoolAttribute{
			MarkdownDescription: "Change at next auth.",
			Optional:            true,
		},
		"custom_regex_pattern": schema.StringAttribute{
			MarkdownDescription: "Custom regular expression for passcode validation. Maximum length: `2048`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.LengthAtMost(2048)},
		},
		"custom_regex_description": schema.MapAttribute{
			MarkdownDescription: "Localized descriptions for the custom regex. Map of OS language ID to description string. Use the `default` key for languages not explicitly listed.",
			Optional:            true,
			ElementType:         types.StringType,
		},
		"failed_attempts_reset_in_minutes": schema.Int64Attribute{
			MarkdownDescription: "Failed attempts reset in minutes. Minimum: `0`.",
			Optional:            true,
		},
		"maximum_failed_attempts": schema.Int64Attribute{
			MarkdownDescription: "Maximum failed attempts. Range: `2`-`11`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(2, 11)},
		},
		"maximum_grace_period_in_minutes": schema.Int64Attribute{
			MarkdownDescription: "Maximum grace period in minutes. Minimum: `0`.",
			Optional:            true,
		},
		"maximum_inactivity_in_minutes": schema.Int64Attribute{
			MarkdownDescription: "Maximum inactivity in minutes. Range: `0`-`15`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(0, 15)},
		},
		"maximum_passcode_age_in_days": schema.Int64Attribute{
			MarkdownDescription: "Maximum passcode age in days. Range: `0`-`730`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(0, 730)},
		},
		"minimum_complex_characters": schema.Int64Attribute{
			MarkdownDescription: "Minimum complex characters. Range: `0`-`4`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(0, 4)},
		},
		"minimum_length": schema.Int64Attribute{
			MarkdownDescription: "Minimum length. Range: `0`-`16`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(0, 16)},
		},
		"passcode_reuse_limit": schema.Int64Attribute{
			MarkdownDescription: "Passcode reuse limit. Range: `1`-`50`.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.Between(1, 50)},
		},
		"require_alphanumeric_passcode": schema.BoolAttribute{
			MarkdownDescription: "Require alphanumeric passcode.",
			Optional:            true,
		},
		"require_complex_passcode": schema.BoolAttribute{
			MarkdownDescription: "Require complex passcode.",
			Optional:            true,
		},
		"require_passcode": schema.BoolAttribute{
			MarkdownDescription: "Require passcode.",
			Optional:            true,
		},
	}
}

// boolPtrFromBool converts a configured types.Bool field to a *bool pointer with Included envelope.
func buildChangeAtNextAuth(field types.Bool) *blueprints.ChangeAtNextAuth {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := field.ValueBool()
	t := true
	return &blueprints.ChangeAtNextAuth{Included: &t, Value: &v}
}

// buildRequireBool builds a bool wrapper struct with Included envelope.
func buildRequireAlphanumeric(field types.Bool) *blueprints.RequireAlphanumericPasscode {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := field.ValueBool()
	t := true
	return &blueprints.RequireAlphanumericPasscode{Included: &t, Value: &v}
}

// buildRequireComplex builds a require complex passcode wrapper with Included envelope.
func buildRequireComplex(field types.Bool) *blueprints.RequireComplexPasscode {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := field.ValueBool()
	t := true
	return &blueprints.RequireComplexPasscode{Included: &t, Value: &v}
}

// buildRequirePasscode builds a require passcode wrapper with Included envelope.
func buildRequirePasscode(field types.Bool) *blueprints.RequirePasscode {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := field.ValueBool()
	t := true
	return &blueprints.RequirePasscode{Included: &t, Value: &v}
}

// buildFailedAttemptsReset builds a FailedAttemptsResetInMinutes wrapper.
func buildFailedAttemptsReset(field types.Int64) *blueprints.FailedAttemptsResetInMinutes {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.FailedAttemptsResetInMinutes{Included: &t, Value: &v}
}

// buildMaximumFailedAttempts builds a MaximumFailedAttempts wrapper.
func buildMaximumFailedAttempts(field types.Int64) *blueprints.MaximumFailedAttempts {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.MaximumFailedAttempts{Included: &t, Value: &v}
}

// buildMaximumGracePeriod builds a MaximumGracePeriodInMinutes wrapper.
func buildMaximumGracePeriod(field types.Int64) *blueprints.MaximumGracePeriodInMinutes {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.MaximumGracePeriodInMinutes{Included: &t, Value: &v}
}

// buildMaximumInactivity builds a MaximumInactivityInMinutes wrapper.
func buildMaximumInactivity(field types.Int64) *blueprints.MaximumInactivityInMinutes {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.MaximumInactivityInMinutes{Included: &t, Value: &v}
}

// buildMaximumPasscodeAge builds a MaximumPasscodeAgeInDays wrapper.
func buildMaximumPasscodeAge(field types.Int64) *blueprints.MaximumPasscodeAgeInDays {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.MaximumPasscodeAgeInDays{Included: &t, Value: &v}
}

// buildMinimumComplexChars builds a MinimumComplexCharacters wrapper.
func buildMinimumComplexChars(field types.Int64) *blueprints.MinimumComplexCharacters {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.MinimumComplexCharacters{Included: &t, Value: &v}
}

// buildMinimumLength builds a MinimumLength wrapper.
func buildMinimumLength(field types.Int64) *blueprints.MinimumLength {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.MinimumLength{Included: &t, Value: &v}
}

// buildPasscodeReuseLimit builds a PasscodeReuseLimit wrapper.
func buildPasscodeReuseLimit(field types.Int64) *blueprints.PasscodeReuseLimit {
	if !helpers.IsConfiguredValue(field) {
		return nil
	}
	v := int(field.ValueInt64())
	t := true
	return &blueprints.PasscodeReuseLimit{Included: &t, Value: &v}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *PasscodePolicyComponent) ToRawConfiguration() (json.RawMessage, error) {
	cfg := blueprints.PasscodeSettingsConfiguration{Version: 2}

	cfg.ChangeAtNextAuth = buildChangeAtNextAuth(c.ChangeAtNextAuth)
	cfg.FailedAttemptsResetInMinutes = buildFailedAttemptsReset(c.FailedAttemptsResetInMinutes)
	cfg.MaximumFailedAttempts = buildMaximumFailedAttempts(c.MaximumFailedAttempts)
	cfg.MaximumGracePeriodInMinutes = buildMaximumGracePeriod(c.MaximumGracePeriodInMinutes)
	cfg.MaximumInactivityInMinutes = buildMaximumInactivity(c.MaximumInactivityInMinutes)
	cfg.MaximumPasscodeAgeInDays = buildMaximumPasscodeAge(c.MaximumPasscodeAgeInDays)
	cfg.MinimumComplexCharacters = buildMinimumComplexChars(c.MinimumComplexCharacters)
	cfg.MinimumLength = buildMinimumLength(c.MinimumLength)
	cfg.PasscodeReuseLimit = buildPasscodeReuseLimit(c.PasscodeReuseLimit)
	cfg.RequireAlphanumericPasscode = buildRequireAlphanumeric(c.RequireAlphanumericPasscode)
	cfg.RequireComplexPasscode = buildRequireComplex(c.RequireComplexPasscode)
	cfg.RequirePasscode = buildRequirePasscode(c.RequirePasscode)

	hasCustomRegex := helpers.IsConfiguredValue(c.CustomRegexPattern) ||
		(!c.CustomRegexDescription.IsNull() && !c.CustomRegexDescription.IsUnknown())

	if hasCustomRegex {
		trueVal := true
		customRegex := &blueprints.CustomRegex{
			Included: &trueVal,
		}
		if helpers.IsConfiguredValue(c.CustomRegexPattern) {
			pattern := c.CustomRegexPattern.ValueString()
			customRegex.Regex = &pattern
		}
		if !c.CustomRegexDescription.IsNull() && !c.CustomRegexDescription.IsUnknown() {
			descMap := make(map[string]string)
			for k, v := range c.CustomRegexDescription.Elements() {
				if strVal, ok := v.(types.String); ok {
					descMap[k] = strVal.ValueString()
				}
			}
			customRegex.Description = &descMap
		}
		cfg.CustomRegex = customRegex
	}

	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *PasscodePolicyComponent) FromRawConfiguration(raw json.RawMessage) error {
	c.ChangeAtNextAuth = types.BoolNull()
	c.FailedAttemptsResetInMinutes = types.Int64Null()
	c.MaximumFailedAttempts = types.Int64Null()
	c.MaximumGracePeriodInMinutes = types.Int64Null()
	c.MaximumInactivityInMinutes = types.Int64Null()
	c.MaximumPasscodeAgeInDays = types.Int64Null()
	c.MinimumComplexCharacters = types.Int64Null()
	c.MinimumLength = types.Int64Null()
	c.PasscodeReuseLimit = types.Int64Null()
	c.RequireAlphanumericPasscode = types.BoolNull()
	c.RequireComplexPasscode = types.BoolNull()
	c.RequirePasscode = types.BoolNull()
	c.CustomRegexPattern = types.StringNull()
	c.CustomRegexDescription = types.MapNull(types.StringType)

	var cfg blueprints.PasscodeSettingsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	if f := cfg.ChangeAtNextAuth; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.ChangeAtNextAuth = types.BoolValue(*f.Value)
	}
	if f := cfg.FailedAttemptsResetInMinutes; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.FailedAttemptsResetInMinutes = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.MaximumFailedAttempts; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.MaximumFailedAttempts = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.MaximumGracePeriodInMinutes; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.MaximumGracePeriodInMinutes = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.MaximumInactivityInMinutes; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.MaximumInactivityInMinutes = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.MaximumPasscodeAgeInDays; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.MaximumPasscodeAgeInDays = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.MinimumComplexCharacters; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.MinimumComplexCharacters = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.MinimumLength; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.MinimumLength = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.PasscodeReuseLimit; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.PasscodeReuseLimit = types.Int64Value(int64(*f.Value))
	}
	if f := cfg.RequireAlphanumericPasscode; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.RequireAlphanumericPasscode = types.BoolValue(*f.Value)
	}
	if f := cfg.RequireComplexPasscode; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.RequireComplexPasscode = types.BoolValue(*f.Value)
	}
	if f := cfg.RequirePasscode; f != nil && f.Included != nil && *f.Included && f.Value != nil {
		c.RequirePasscode = types.BoolValue(*f.Value)
	}

	if cr := cfg.CustomRegex; cr != nil {
		if cr.Included == nil || *cr.Included {
			if cr.Regex != nil {
				c.CustomRegexPattern = types.StringValue(*cr.Regex)
			}
			if cr.Description != nil {
				elems := make(map[string]types.String, len(*cr.Description))
				for k, v := range *cr.Description {
					elems[k] = types.StringValue(v)
				}
				if len(elems) > 0 {
					tfMap, diags := types.MapValueFrom(context.Background(), types.StringType, elems)
					if !diags.HasError() {
						c.CustomRegexDescription = tfMap
					}
				}
			}
		}
	}

	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *PasscodePolicyComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
