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

// SafariSettingsComponent represents a strongly-typed Safari settings component.
type SafariSettingsComponent struct {
	AcceptCookies              types.String `tfsdk:"accept_cookies"`
	AllowDisablingFraudWarning types.Bool   `tfsdk:"allow_disabling_fraud_warning"`
	AllowHistoryClearing       types.Bool   `tfsdk:"allow_history_clearing"`
	AllowJavaScript            types.Bool   `tfsdk:"allow_javascript"`
	AllowPrivateBrowsing       types.Bool   `tfsdk:"allow_private_browsing"`
	AllowPopups                types.Bool   `tfsdk:"allow_popups"`
	AllowSummary               types.Bool   `tfsdk:"allow_summary"`
	NewTabStartPageType        types.String `tfsdk:"new_tab_start_page_type"`
	NewTabStartPageHomepageURL types.String `tfsdk:"new_tab_start_page_homepage_url"`
	NewTabStartPageExtensionID types.String `tfsdk:"new_tab_start_page_extension_id"`
}

// GetIdentifier returns the component identifier for Safari settings.
func (c *SafariSettingsComponent) GetIdentifier() string {
	return "com.jamf.ddm.safari-settings"
}

// SafariSettingsComponentSchema returns the Terraform schema for Safari settings component.
func SafariSettingsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"accept_cookies": schema.StringAttribute{
			MarkdownDescription: "The policy Safari uses for managing cookies. Valid values are `Never`, `CurrentWebsite`, `VisitedWebsites`, `Always`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.AcceptCookiesValueValues()...)},
		},
		"allow_disabling_fraud_warning": schema.BoolAttribute{
			MarkdownDescription: "If false, the system forces fraud warnings on in Safari.",
			Optional:            true,
		},
		"allow_history_clearing": schema.BoolAttribute{
			MarkdownDescription: "If false, the system disables clearing history in Safari.",
			Optional:            true,
		},
		"allow_javascript": schema.BoolAttribute{
			MarkdownDescription: "If false, the system disables JavaScript in Safari.",
			Optional:            true,
		},
		"allow_private_browsing": schema.BoolAttribute{
			MarkdownDescription: "If false, the system disables private browsing in Safari.",
			Optional:            true,
		},
		"allow_popups": schema.BoolAttribute{
			MarkdownDescription: "If false, the system disables popups in Safari.",
			Optional:            true,
		},
		"allow_summary": schema.BoolAttribute{
			MarkdownDescription: "If false, the system disables summarization of content in Safari.",
			Optional:            true,
		},
		"new_tab_start_page_type": schema.StringAttribute{
			MarkdownDescription: "Sets the start page type in Safari. Valid values are `Start`, `Home`, `Extension`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.OneOf(blueprints.NewTabStartPagePageTypeValues()...)},
		},
		"new_tab_start_page_homepage_url": schema.StringAttribute{
			MarkdownDescription: "The URL of the homepage which needs to start with `https://` or `http://`. Required when page type is `Home`.",
			Optional:            true,
		},
		"new_tab_start_page_extension_id": schema.StringAttribute{
			MarkdownDescription: "The composed identifier of the extension that provides the start page. Required when page type is `Extension`. Format: `com.example.extension (ABC1234567)`.",
			Optional:            true,
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *SafariSettingsComponent) ToRawConfiguration() (json.RawMessage, error) {
	cfg := blueprints.SafariSettingsConfiguration{}

	if helpers.IsConfiguredValue(c.AcceptCookies) {
		v := c.AcceptCookies.ValueString()
		t := true
		cfg.AcceptCookies = &blueprints.AcceptCookies{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.AllowDisablingFraudWarning) {
		v := c.AllowDisablingFraudWarning.ValueBool()
		t := true
		cfg.AllowDisablingFraudWarning = &blueprints.AllowDisablingFraudWarning{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.AllowHistoryClearing) {
		v := c.AllowHistoryClearing.ValueBool()
		t := true
		cfg.AllowHistoryClearing = &blueprints.AllowHistoryClearing{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.AllowJavaScript) {
		v := c.AllowJavaScript.ValueBool()
		t := true
		cfg.AllowJavaScript = &blueprints.AllowJavaScript{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.AllowPrivateBrowsing) {
		v := c.AllowPrivateBrowsing.ValueBool()
		t := true
		cfg.AllowPrivateBrowsing = &blueprints.AllowPrivateBrowsing{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.AllowPopups) {
		v := c.AllowPopups.ValueBool()
		t := true
		cfg.AllowPopups = &blueprints.AllowPopups{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.AllowSummary) {
		v := c.AllowSummary.ValueBool()
		t := true
		cfg.AllowSummary = &blueprints.AllowSummary{Included: &t, Value: &v}
	}

	if helpers.IsConfiguredValue(c.NewTabStartPageType) ||
		helpers.IsConfiguredValue(c.NewTabStartPageHomepageURL) ||
		helpers.IsConfiguredValue(c.NewTabStartPageExtensionID) {

		trueVal := true
		ntsp := &blueprints.NewTabStartPage{
			Included: &trueVal,
		}
		if helpers.IsConfiguredValue(c.NewTabStartPageType) {
			ntsp.PageType = c.NewTabStartPageType.ValueStringPointer()
		}
		if helpers.IsConfiguredValue(c.NewTabStartPageHomepageURL) {
			u := c.NewTabStartPageHomepageURL.ValueString()
			ntsp.HomepageURL = &u
		}
		if helpers.IsConfiguredValue(c.NewTabStartPageExtensionID) {
			e := c.NewTabStartPageExtensionID.ValueString()
			ntsp.ExtensionIdentifier = &e
		}
		cfg.NewTabStartPage = ntsp
	}

	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *SafariSettingsComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg blueprints.SafariSettingsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	c.AcceptCookies = types.StringNull()
	c.AllowDisablingFraudWarning = types.BoolNull()
	c.AllowHistoryClearing = types.BoolNull()
	c.AllowJavaScript = types.BoolNull()
	c.AllowPrivateBrowsing = types.BoolNull()
	c.AllowPopups = types.BoolNull()
	c.AllowSummary = types.BoolNull()
	c.NewTabStartPageType = types.StringNull()
	c.NewTabStartPageHomepageURL = types.StringNull()
	c.NewTabStartPageExtensionID = types.StringNull()

	if f := cfg.AcceptCookies; f != nil && f.Value != nil {
		c.AcceptCookies = types.StringValue(*f.Value)
	}
	if f := cfg.AllowDisablingFraudWarning; f != nil && f.Value != nil {
		c.AllowDisablingFraudWarning = types.BoolValue(*f.Value)
	}
	if f := cfg.AllowHistoryClearing; f != nil && f.Value != nil {
		c.AllowHistoryClearing = types.BoolValue(*f.Value)
	}
	if f := cfg.AllowJavaScript; f != nil && f.Value != nil {
		c.AllowJavaScript = types.BoolValue(*f.Value)
	}
	if f := cfg.AllowPrivateBrowsing; f != nil && f.Value != nil {
		c.AllowPrivateBrowsing = types.BoolValue(*f.Value)
	}
	if f := cfg.AllowPopups; f != nil && f.Value != nil {
		c.AllowPopups = types.BoolValue(*f.Value)
	}
	if f := cfg.AllowSummary; f != nil && f.Value != nil {
		c.AllowSummary = types.BoolValue(*f.Value)
	}

	if ntsp := cfg.NewTabStartPage; ntsp != nil {
		c.NewTabStartPageType = types.StringPointerValue(ntsp.PageType)
		if ntsp.HomepageURL != nil {
			c.NewTabStartPageHomepageURL = types.StringValue(*ntsp.HomepageURL)
		}
		if ntsp.ExtensionIdentifier != nil {
			c.NewTabStartPageExtensionID = types.StringValue(*ntsp.ExtensionIdentifier)
		}
	}

	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *SafariSettingsComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
