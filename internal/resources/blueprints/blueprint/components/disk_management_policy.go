// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// DiskManagementPolicyComponent represents a strongly-typed disk management policy component.
type DiskManagementPolicyComponent struct {
	ExternalStorage types.String `tfsdk:"external_storage"`
	NetworkStorage  types.String `tfsdk:"network_storage"`
}

// GetIdentifier returns the component identifier for disk management policy.
func (c *DiskManagementPolicyComponent) GetIdentifier() string {
	return "com.jamf.ddm.disk-management"
}

// DiskManagementPolicyComponentSchema returns the Terraform schema for disk management policy component.
func DiskManagementPolicyComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"external_storage": schema.StringAttribute{
			MarkdownDescription: "Storage mode for external storage. Valid values are `Allowed`, `Disallowed`, `ReadOnly`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.StorageModeValueValues()...)},
		},
		"network_storage": schema.StringAttribute{
			MarkdownDescription: "Storage mode for network storage. Valid values are `Allowed`, `Disallowed`, `ReadOnly`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.StorageModeValueValues()...)},
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *DiskManagementPolicyComponent) ToRawConfiguration() (json.RawMessage, error) {
	trueVal := true
	falseVal := false
	cfg := blueprints.DiskManagementSettingsConfiguration{Version: 2}
	restrictions := &blueprints.Restrictions{}

	if helpers.IsConfiguredValue(c.ExternalStorage) {
		restrictions.ExternalStorage = &blueprints.StorageMode{Included: &trueVal, Value: c.ExternalStorage.ValueString()}
	} else {
		restrictions.ExternalStorage = &blueprints.StorageMode{Included: &falseVal, Value: blueprints.StorageModeValueAllowed}
	}

	if helpers.IsConfiguredValue(c.NetworkStorage) {
		restrictions.NetworkStorage = &blueprints.StorageMode{Included: &trueVal, Value: c.NetworkStorage.ValueString()}
	} else {
		restrictions.NetworkStorage = &blueprints.StorageMode{Included: &falseVal, Value: blueprints.StorageModeValueAllowed}
	}

	cfg.Restrictions = restrictions
	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *DiskManagementPolicyComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg blueprints.DiskManagementSettingsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	if cfg.Restrictions != nil {
		if ext := cfg.Restrictions.ExternalStorage; ext != nil && ext.Included != nil && *ext.Included {
			c.ExternalStorage = types.StringValue(ext.Value)
		} else {
			c.ExternalStorage = types.StringNull()
		}
		if net := cfg.Restrictions.NetworkStorage; net != nil && net.Included != nil && *net.Included {
			c.NetworkStorage = types.StringValue(net.Value)
		} else {
			c.NetworkStorage = types.StringNull()
		}
	} else {
		c.ExternalStorage = types.StringNull()
		c.NetworkStorage = types.StringNull()
	}

	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *DiskManagementPolicyComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
