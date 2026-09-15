// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// AIGovernanceComponent delivers published AI Governance policy versions to the devices a blueprint
// targets. It is what makes a jamfplatform_ai_governance_policy reach a Mac.
//
// The blueprints SDK carries no type for this component's configuration — the component catalogue
// advertises a name and description and no payload schema — so the wire shape below was established
// by probing and is marshalled here. Wire-verified against the EU sandbox on 2026-08-30:
//
//	{"policies": [{"policyId": "<uuid>", "versionNumber": 1}]}
//
// An omitted policies list is refused with INVALID_CONFIGURATION naming
// configuration.policies, an empty one likewise, and a bare string in place of an object with
// VALIDATION_FAILURE. A policy or version that does not exist is refused with
// POLICY_VERSION_NOT_FOUND, and an archived policy with POLICY_ARCHIVED — both against
// configuration.policies[n].policyId regardless of which of the two is actually wrong.
//
// Two further facts govern a component carrying more than one policy, wire-verified on the same
// date. Order is preserved verbatim: a list sent reverse-sorted by policyId is returned
// reverse-sorted, and a PATCH to sorted order and another back both round-trip faithfully — so
// policies is a ListNestedAttribute, and a Set would lose an ordering the service keeps. And a
// repeated policyId is refused rather than collapsed, with INVALID_CONFIGURATION naming
// "duplicate policyId" against configuration.policies[1], which is why the schema below carries a
// plan-time uniqueness validator: the same configuration would otherwise fail mid-apply.
type AIGovernanceComponent struct {
	Policies []AIGovernancePolicyReference `tfsdk:"policies"`
}

// AIGovernancePolicyReference pins one published policy version.
type AIGovernancePolicyReference struct {
	PolicyID types.String `tfsdk:"policy_id"`
	Version  types.Int64  `tfsdk:"version"`
}

// aiGovernanceConfiguration is the component's wire configuration.
type aiGovernanceConfiguration struct {
	Policies []aiGovernancePolicyEntry `json:"policies"`
}

// aiGovernancePolicyEntry is one entry in the wire configuration's policies array.
type aiGovernancePolicyEntry struct {
	PolicyID      string `json:"policyId"`
	VersionNumber int64  `json:"versionNumber"`
}

// GetIdentifier returns the component identifier for AI Governance.
func (c *AIGovernanceComponent) GetIdentifier() string {
	return "com.jamf.ai-governance"
}

// AIGovernanceComponentSchema returns the Terraform schema for the AI Governance component.
func AIGovernanceComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"policies": schema.ListNestedAttribute{
			MarkdownDescription: "The published AI policy versions to deliver to the devices this blueprint targets. " +
				"At least one entry is required.",
			Required: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"policy_id": schema.StringAttribute{
						MarkdownDescription: "ID of the AI policy to deliver: the `id` of a " +
							"`jamfplatform_ai_governance_policy`.",
						Required: true,
					},
					"version": schema.Int64Attribute{
						MarkdownDescription: "Which published version of the policy to deliver. A blueprint pins a " +
							"version rather than tracking the policy, so this is normally the policy's " +
							"`published_version`. The version must already be published: an unpublished policy cannot " +
							"be delivered.",
						Required: true,
					},
				},
			},
			Validators: []validator.List{
				listvalidator.SizeAtLeast(1),
				commonvalidators.UniqueStringFieldList("policy_id"),
			},
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *AIGovernanceComponent) ToRawConfiguration() (json.RawMessage, error) {
	cfg := aiGovernanceConfiguration{Policies: make([]aiGovernancePolicyEntry, 0, len(c.Policies))}
	for _, reference := range c.Policies {
		cfg.Policies = append(cfg.Policies, aiGovernancePolicyEntry{
			PolicyID:      reference.PolicyID.ValueString(),
			VersionNumber: reference.Version.ValueInt64(),
		})
	}
	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *AIGovernanceComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg aiGovernanceConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	c.Policies = make([]AIGovernancePolicyReference, 0, len(cfg.Policies))
	for _, entry := range cfg.Policies {
		c.Policies = append(c.Policies, AIGovernancePolicyReference{
			PolicyID: types.StringValue(entry.PolicyID),
			Version:  types.Int64Value(entry.VersionNumber),
		})
	}
	return nil
}

// ToClientComponent converts the typed component to an SDK component.
func (c *AIGovernanceComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, fmt.Errorf("marshal AI Governance configuration: %w", err)
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
