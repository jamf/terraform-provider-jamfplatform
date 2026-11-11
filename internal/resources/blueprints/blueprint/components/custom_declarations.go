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
		"declaration": schema.ListNestedAttribute{
			MarkdownDescription: "Custom DDM declarations, in order. A payload may reference another declaration in " +
				"this list with `$PAYLOAD_n`, where `n` is the referenced declaration's 1-based position, so moving a " +
				"declaration changes which one a reference names.",
			Optional: true,
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
//
// Each declaration's payloadKey is its 1-based position in the configuration, the same rule
// apple_declarations follows, so a `$PAYLOAD_n` reference names the n-th declaration listed. The
// service requires the key (a declaration without one is refused with `400 VALIDATION_FAILURE`) and
// otherwise stores it verbatim without interpreting it: wire-probed on the EU gateway 2026-11-11, it
// kept keys sent out of order (2, 1) in the order the declarations were sent rather than sorting
// by key, and accepted a repeated key (1, 1), gaps (9, 5), 0 and -1 alike, each echoed back as sent.
// So the key follows the configuration order and nothing else: reordering the list re-keys the
// declarations that move, and a `$PAYLOAD_n` reference written against the old position then names
// whichever declaration took it. The service stored a reference to a key it holds without resolving
// or checking it on write, so that is not caught on its side either. While the
// attribute was a set the key followed the set's own element order instead, which is why the first
// apply after upgrading may re-key declarations once. Whether a device treats a re-keyed declaration
// as removed and re-added is not observable through the API and was not verified on a device.
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
	return blueprints.CustomDeclarationsComponentIdentifierComJamfDdmCustomDeclarations
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
