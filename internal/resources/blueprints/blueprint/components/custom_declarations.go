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
)

// CustomDeclarationsComponent represents a strongly-typed custom DDM declarations component.
//
// Prefer apple_declarations: it carries the same declarations to the same devices, and Jamf renders
// those as typed forms generated from Apple's schemas rather than as an opaque JSON blob. This
// component's payloads are checked against Apple's schemas during plan for the same reason — the
// platform validates none of it — and there is no switch to turn that off. A declaration that
// should not be checked belongs in raw_component, which is the escape hatch.
type CustomDeclarationsComponent struct {
	Declarations []CustomDeclarationModel `tfsdk:"declaration"`
}

// CustomDeclarationModel represents a single custom DDM declaration.
type CustomDeclarationModel struct {
	ChannelType types.String `tfsdk:"channel"`
	Kind        types.String `tfsdk:"kind"`
	Payload     types.String `tfsdk:"payload"`
	Type        types.String `tfsdk:"type"`
}

// CustomDeclarationsComponentSchema returns the Terraform schema for custom declarations component.
func CustomDeclarationsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"declaration": schema.SetNestedAttribute{
			MarkdownDescription: "Custom DDM declaration.",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"channel": schema.StringAttribute{
						MarkdownDescription: "The channel type for the declaration. Valid values are `SYSTEM`, `USER`.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.OneOf(blueprints.DeclarationChannelTypeSystem, blueprints.DeclarationChannelTypeUser)},
					},
					"kind": schema.StringAttribute{
						MarkdownDescription: "The kind of declaration. Valid values are `CONFIGURATION`, `ASSET`. " +
							"An activation or management declaration cannot be expressed here: deliver one with " +
							"`apple_declarations`, which derives the kind from the declaration type.",
						Required:   true,
						Validators: []validator.String{stringvalidator.OneOf(blueprints.DeclarationKindConfiguration, blueprints.DeclarationKindAsset)},
					},
					"payload": schema.StringAttribute{
						MarkdownDescription: "JSON-encoded payload object for the declaration." + AppleDeclarationsBehaviour,
						Required:            true,
					},
					"type": schema.StringAttribute{
						MarkdownDescription: "The declaration type identifier (e.g., `com.apple.configuration.softwareupdate.settings`).",
						Required:            true,
					},
				},
			},
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
// Empty Declarations marshals to `{}` to preserve the canonical no-op shape;
// otherwise builds blueprints.CustomDeclarationsConfiguration so the on-wire
// payload is driven by the SDK schema rather than ad-hoc maps.
func (c *CustomDeclarationsComponent) ToRawConfiguration() (json.RawMessage, error) {
	if len(c.Declarations) == 0 {
		return json.RawMessage(`{}`), nil
	}

	declarations := make([]blueprints.CustomDeclaration, 0, len(c.Declarations))
	for idx, declaration := range c.Declarations {
		var payload map[string]any
		if err := json.Unmarshal([]byte(declaration.Payload.ValueString()), &payload); err != nil {
			return nil, err
		}
		declarations = append(declarations, blueprints.CustomDeclaration{
			ChannelType: declaration.ChannelType.ValueString(),
			Kind:        declaration.Kind.ValueString(),
			Payload:     payload,
			PayloadKey:  idx + 1,
			Type:        declaration.Type.ValueString(),
		})
	}

	return json.Marshal(blueprints.CustomDeclarationsConfiguration{Declarations: declarations})
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *CustomDeclarationsComponent) FromRawConfiguration(raw json.RawMessage) error {
	var config blueprints.CustomDeclarationsConfiguration
	if err := json.Unmarshal(raw, &config); err != nil {
		return err
	}

	declarations := make([]CustomDeclarationModel, 0, len(config.Declarations))
	for _, declaration := range config.Declarations {
		payloadJSON, err := json.Marshal(declaration.Payload)
		if err != nil {
			return err
		}
		declarations = append(declarations, CustomDeclarationModel{
			ChannelType: types.StringValue(declaration.ChannelType),
			Kind:        types.StringValue(declaration.Kind),
			Payload:     types.StringValue(string(payloadJSON)),
			Type:        types.StringValue(declaration.Type),
		})
	}

	c.Declarations = declarations
	return nil
}

// GetIdentifier returns the component identifier for custom declarations.
func (c *CustomDeclarationsComponent) GetIdentifier() string {
	return "com.jamf.ddm.custom-declarations"
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *CustomDeclarationsComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
