// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// SafariExtensionsComponent represents a strongly-typed Safari extensions component.
//
// managed_extensions stays a set: on the wire it is a JSON object keyed by extension identifier, so
// the service has no order to keep and cannot hold one identifier twice. Each extension's
// allowed_domains and denied_domains are lists, because they are arrays the service stores as
// written: wire-probed on the EU gateway 2026-11-11 (issue #568), on POST and on merge-patch,
// insertion order was kept, a reorder was honoured and an identical domain listed twice was kept.
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
	return blueprints.SafariExtensionsComponentIdentifierComJamfDdmSafariExtensions
}

// uniqueExtensionIDs rejects two managed_extensions entries naming one extension_id during plan.
//
// The wire shape is a JSON object keyed by extension identifier, so two entries for one identifier
// cannot both be stored: ToRawConfiguration writes a Go map, and whichever entry the set yields last
// overwrites the other. The service behaves the same way on its own, wire-probed on the EU gateway
// 2026-11-11: a body repeating one identifier key was stored with the last occurrence only, in
// both send orders, on POST and on merge-patch. Unchecked, the dropped entry surfaces as
// "Provider produced inconsistent result after apply" (reproduced 2026-11-11, the AlwaysOff entry
// kept and the Allowed one lost), after the blueprint has already been written. It wraps the
// shared UniqueStringFieldSet so the unknown handling stays in one place, and rewords the finding
// to say what to do about it.
func uniqueExtensionIDs() validator.Set {
	return uniqueExtensionIDsValidator{inner: commonvalidators.UniqueStringFieldSet("extension_id")}
}

// uniqueExtensionIDsValidator is the validator uniqueExtensionIDs returns.
type uniqueExtensionIDsValidator struct {
	inner validator.Set
}

// Description returns the shared validator's description.
func (v uniqueExtensionIDsValidator) Description(ctx context.Context) string {
	return v.inner.Description(ctx)
}

// MarkdownDescription returns the shared validator's markdown description.
func (v uniqueExtensionIDsValidator) MarkdownDescription(ctx context.Context) string {
	return v.inner.MarkdownDescription(ctx)
}

// ValidateSet reports each repeated extension_id the shared validator finds, at the set's path.
func (v uniqueExtensionIDsValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	var inner validator.SetResponse
	v.inner.ValidateSet(ctx, req, &inner)
	for _, finding := range inner.Diagnostics.Errors() {
		resp.Diagnostics.AddAttributeError(req.Path,
			"Safari extension configured more than once",
			finding.Detail()+" Jamf Pro keeps one setting per extension and silently drops the others, "+
				"so merge these entries into one.",
		)
	}
}

// SafariExtensionsComponentSchema returns the Terraform schema for Safari extensions component.
func SafariExtensionsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"managed_extensions": schema.SetNestedAttribute{
			MarkdownDescription: "Set of managed Safari extensions. Each `extension_id` may appear only once.",
			Optional:            true,
			Validators:          []validator.Set{uniqueExtensionIDs()},
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
					"allowed_domains": schema.ListNestedAttribute{
						MarkdownDescription: "Allowed domains for this extension, kept in the order listed.",
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
					"denied_domains": schema.ListNestedAttribute{
						MarkdownDescription: "Denied domains for this extension, kept in the order listed.",
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
