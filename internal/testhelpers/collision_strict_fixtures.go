// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// The helpers below create, outside Terraform, an object of a type Jamf accepts
// duplicate names for. A strict name collision test then plans a second object
// against it. Each helper removes its object when the test ends.

// CreateOutOfBandBlueprint creates an empty, undeployed blueprint scoped to the
// given Platform device group and returns its ID. A blueprint refuses an empty scope.
func CreateOutOfBandBlueprint(t *testing.T, name, deviceGroupID string) string {
	t.Helper()
	c := blueprints.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateBlueprint(ctx, &blueprints.CreateBlueprintRequest{
		Name:  name,
		Scope: blueprints.CreateScope{DeviceGroups: []string{deviceGroupID}},
		Steps: []blueprints.BlueprintStep{},
	})
	if err != nil {
		t.Fatalf("creating blueprint %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteBlueprint(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandAIGovernancePolicy creates a draft AI Governance policy for the
// tool and returns its ID. Cleanup archives it, which is the policy's delete.
func CreateOutOfBandAIGovernancePolicy(t *testing.T, name string, tool aigovernance.ToolSummary) string {
	t.Helper()
	c := aigovernance.New(NewAcceptanceClient(t))
	ctx := context.Background()
	settings, err := json.Marshal(map[string]any{"verbose": true})
	if err != nil {
		t.Fatalf("encoding AI policy settings: %v", err)
	}
	created, err := c.CreatePolicy(ctx, &aigovernance.CreatePolicyRequest{
		Name:          name,
		ToolID:        tool.ID,
		SchemaVersion: tool.SchemaVersion,
		Settings:      settings,
	})
	if err != nil {
		t.Fatalf("creating AI policy %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.ArchivePolicy(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandEnrollmentCustomization creates an enrollment customization with
// no panes and returns its ID.
func CreateOutOfBandEnrollmentCustomization(t *testing.T, name string) string {
	t.Helper()
	c := pro.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateEnrollmentCustomizationV2(ctx, &pro.EnrollmentCustomizationV2{
		DisplayName: name,
		Description: "created outside Terraform by the acceptance suite",
		SiteID:      "-1",
		EnrollmentCustomizationBrandingSettings: pro.EnrollmentCustomizationBrandingSettings{
			BackgroundColor: "ffffff",
			ButtonColor:     "0066cc",
			ButtonTextColor: "ffffff",
			TextColor:       "333333",
		},
	})
	if err != nil {
		t.Fatalf("creating enrollment customization %q outside Terraform: %v", name, err)
	}
	id := created.ID
	t.Cleanup(func() { _ = c.DeleteEnrollmentCustomizationV2(ctx, id) })
	return id
}

// CreateOutOfBandClassicAdvancedComputerSearch creates an advanced computer
// search with no criteria through the Classic API and returns its ID.
func CreateOutOfBandClassicAdvancedComputerSearch(t *testing.T, name string) string {
	t.Helper()
	c := proclassic.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateAdvancedComputerSearchByID(ctx, "0", &proclassic.AdvancedComputerSearch{Name: &name})
	if err != nil || created.ID == nil {
		t.Fatalf("creating advanced computer search %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteAdvancedComputerSearchByID(ctx, id) })
	return id
}
