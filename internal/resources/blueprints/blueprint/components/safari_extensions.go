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

// SafariExtensionsComponent represents a strongly-typed Safari extensions component.
type SafariExtensionsComponent struct {
	ManagedExtensions []ManagedExtensionModel `tfsdk:"managed_extensions"`
}

// ManagedExtensionModel represents a managed Safari extension configuration.
type ManagedExtensionModel struct {
	ExtensionID     types.String                  `tfsdk:"extension_id"`
	State           types.String                  `tfsdk:"state"`
	PrivateBrowsing types.String                  `tfsdk:"private_browsing"`
	AllowedDomains  []ManagedExtensionDomainModel `tfsdk:"allowed_domains"`
	DeniedDomains   []ManagedExtensionDomainModel `tfsdk:"denied_domains"`
}

// ManagedExtensionDomainModel represents a domain configuration for managed extensions.
type ManagedExtensionDomainModel struct {
	Domain types.String `tfsdk:"domain"`
}

// GetIdentifier returns the component identifier for Safari extensions.
func (c *SafariExtensionsComponent) GetIdentifier() string {
	return "com.jamf.ddm.safari-extensions"
}

// SafariExtensionsComponentSchema returns the Terraform schema for Safari extensions component.
func SafariExtensionsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"managed_extensions": schema.SetNestedAttribute{
			MarkdownDescription: "Set of managed Safari extensions.",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"extension_id": schema.StringAttribute{
						MarkdownDescription: "The extension identifier (bundle ID).",
						Required:            true,
					},
					"state": schema.StringAttribute{
						MarkdownDescription: "Extension state. Valid values are `Allowed`, `AlwaysOn`, `AlwaysOff`.",
						Optional:            true,
						Validators:          []validator.String{stringvalidator.OneOf(blueprints.AutomaticActionValueValues()...)},
					},
					"private_browsing": schema.StringAttribute{
						MarkdownDescription: "Private browsing state. Valid values are `Allowed`, `AlwaysOn`, `AlwaysOff`.",
						Optional:            true,
						Validators:          []validator.String{stringvalidator.OneOf(blueprints.AutomaticActionValueValues()...)},
					},
					"allowed_domains": schema.SetNestedAttribute{
						MarkdownDescription: "Set of allowed domains for this extension.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"domain": schema.StringAttribute{
									MarkdownDescription: "Domain name.",
									Required:            true,
								},
							},
						},
					},
					"denied_domains": schema.SetNestedAttribute{
						MarkdownDescription: "Set of denied domains for this extension.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"domain": schema.StringAttribute{
									MarkdownDescription: "Domain name.",
									Required:            true,
								},
							},
						},
					},
				},
			},
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *SafariExtensionsComponent) ToRawConfiguration() (json.RawMessage, error) {
	managedExtensions := make(map[string]blueprints.ManagedExtension)

	for _, ext := range c.ManagedExtensions {
		if !helpers.IsConfiguredValue(ext.ExtensionID) {
			continue
		}

		extConfig := blueprints.ManagedExtension{}

		if helpers.IsConfiguredValue(ext.State) {
			s := ext.State.ValueString()
			extConfig.State = &s
		}
		if helpers.IsConfiguredValue(ext.PrivateBrowsing) {
			pb := ext.PrivateBrowsing.ValueString()
			extConfig.PrivateBrowsing = &pb
		}

		if len(ext.AllowedDomains) > 0 {
			allowed := make([]blueprints.ManagedExtensionDomain, 0, len(ext.AllowedDomains))
			for _, d := range ext.AllowedDomains {
				if helpers.IsConfiguredValue(d.Domain) {
					allowed = append(allowed, blueprints.ManagedExtensionDomain{Domain: d.Domain.ValueString()})
				}
			}
			if len(allowed) > 0 {
				extConfig.AllowedDomains = &allowed
			}
		}

		if len(ext.DeniedDomains) > 0 {
			denied := make([]blueprints.ManagedExtensionDomain, 0, len(ext.DeniedDomains))
			for _, d := range ext.DeniedDomains {
				if helpers.IsConfiguredValue(d.Domain) {
					denied = append(denied, blueprints.ManagedExtensionDomain{Domain: d.Domain.ValueString()})
				}
			}
			if len(denied) > 0 {
				extConfig.DeniedDomains = &denied
			}
		}

		managedExtensions[ext.ExtensionID.ValueString()] = extConfig
	}

	cfg := blueprints.SafariExtensionsConfiguration{ManagedExtensions: managedExtensions}
	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *SafariExtensionsComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg blueprints.SafariExtensionsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	extensions := make([]ManagedExtensionModel, 0, len(cfg.ManagedExtensions))
	for extensionID, extConfig := range cfg.ManagedExtensions {
		ext := ManagedExtensionModel{
			ExtensionID: types.StringValue(extensionID),
		}

		if extConfig.State != nil {
			ext.State = types.StringValue(*extConfig.State)
		}
		if extConfig.PrivateBrowsing != nil {
			ext.PrivateBrowsing = types.StringValue(*extConfig.PrivateBrowsing)
		}

		if extConfig.AllowedDomains != nil {
			allowed := make([]ManagedExtensionDomainModel, 0, len(*extConfig.AllowedDomains))
			for _, d := range *extConfig.AllowedDomains {
				allowed = append(allowed, ManagedExtensionDomainModel{Domain: types.StringValue(d.Domain)})
			}
			ext.AllowedDomains = allowed
		}

		if extConfig.DeniedDomains != nil {
			denied := make([]ManagedExtensionDomainModel, 0, len(*extConfig.DeniedDomains))
			for _, d := range *extConfig.DeniedDomains {
				denied = append(denied, ManagedExtensionDomainModel{Domain: types.StringValue(d.Domain)})
			}
			ext.DeniedDomains = denied
		}

		extensions = append(extensions, ext)
	}

	c.ManagedExtensions = extensions
	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *SafariExtensionsComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
