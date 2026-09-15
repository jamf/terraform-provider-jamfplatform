// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// AudioAccessorySettingsComponent represents a strongly-typed audio accessory settings component.
type AudioAccessorySettingsComponent struct {
	TemporaryPairingDisabled types.Bool   `tfsdk:"temporary_pairing_disabled"`
	UnpairingTimePolicy      types.String `tfsdk:"unpairing_time_policy"`
	UnpairingTimeHour        types.Int64  `tfsdk:"unpairing_time_hour"`
}

// GetIdentifier returns the component identifier for audio accessory settings.
func (c *AudioAccessorySettingsComponent) GetIdentifier() string {
	return "com.jamf.ddm.audio-accessory-settings"
}

// AudioAccessorySettingsComponentSchema returns the Terraform schema for audio accessory settings component.
func AudioAccessorySettingsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"temporary_pairing_disabled": schema.BoolAttribute{
			MarkdownDescription: "If true, temporary pairing of audio accessories is disabled.",
			Required:            true,
		},
		"unpairing_time_policy": schema.StringAttribute{
			MarkdownDescription: "Device's unpairing policy. Valid values are `None`, `Hour`. When set to `Hour`, `unpairing_time_hour` must also be provided.",
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(blueprints.UnpairingTimePolicyNone),
			Validators: []validator.String{
				stringvalidator.OneOf(blueprints.UnpairingTimePolicyValues()...),
			},
		},
		"unpairing_time_hour": schema.Int64Attribute{
			MarkdownDescription: "The local time hour (24-hour clock) when the device automatically unpairs temporarily paired audio accessories. Required when policy is `Hour`. Range: `0`-`23`.",
			Optional:            true,
			Computed:            true,
			Default:             int64default.StaticInt64(0),
			Validators: []validator.Int64{
				int64validator.Between(0, 23),
			},
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *AudioAccessorySettingsComponent) ToRawConfiguration() (json.RawMessage, error) {
	cfg := blueprints.AudioAccessorySettingsConfiguration{}

	hasTemporaryPairing := helpers.IsConfiguredValue(c.TemporaryPairingDisabled) ||
		helpers.IsConfiguredValue(c.UnpairingTimePolicy) ||
		helpers.IsConfiguredValue(c.UnpairingTimeHour)

	if hasTemporaryPairing {
		trueVal := true
		tp := &blueprints.TemporaryPairing{
			Included: &trueVal,
		}

		if helpers.IsConfiguredValue(c.TemporaryPairingDisabled) {
			disabled := c.TemporaryPairingDisabled.ValueBool()
			tp.Disabled = &disabled
		}

		hasUnpairingSettings := helpers.IsConfiguredValue(c.UnpairingTimePolicy) ||
			helpers.IsConfiguredValue(c.UnpairingTimeHour)

		if hasUnpairingSettings {
			unpairingTime := blueprints.UnpairingTime{}

			if helpers.IsConfiguredValue(c.UnpairingTimePolicy) {
				unpairingTime.Policy = c.UnpairingTimePolicy.ValueString()
			}

			if helpers.IsConfiguredValue(c.UnpairingTimeHour) {
				hour := int(c.UnpairingTimeHour.ValueInt64())
				unpairingTime.Hour = &hour
			}

			tp.Configuration = &blueprints.TemporaryPairingConfig{
				UnpairingTime: unpairingTime,
			}
		}

		cfg.TemporaryPairing = tp
	}

	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *AudioAccessorySettingsComponent) FromRawConfiguration(raw json.RawMessage) error {
	c.TemporaryPairingDisabled = types.BoolNull()
	c.UnpairingTimePolicy = types.StringNull()
	c.UnpairingTimeHour = types.Int64Null()

	var cfg blueprints.AudioAccessorySettingsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	if cfg.TemporaryPairing == nil {
		return nil
	}

	tp := cfg.TemporaryPairing
	if tp.Included == nil || !*tp.Included {
		return nil
	}

	if tp.Disabled != nil {
		c.TemporaryPairingDisabled = types.BoolValue(*tp.Disabled)
	}

	if tp.Configuration != nil {
		ut := tp.Configuration.UnpairingTime
		c.UnpairingTimePolicy = types.StringValue(ut.Policy)
		if ut.Hour != nil {
			c.UnpairingTimeHour = types.Int64Value(int64(*ut.Hour))
		}
	}

	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *AudioAccessorySettingsComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
