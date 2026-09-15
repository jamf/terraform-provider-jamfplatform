// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// platformSet builds the plan value for the platforms attribute.
func platformSet(t *testing.T, labels ...string) types.Set {
	t.Helper()
	elements := make([]attr.Value, 0, len(labels))
	for _, l := range labels {
		elements = append(elements, types.StringValue(l))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("building platform set: %v", diags)
	}
	return set
}

// TestBuildCreateRequest_NetworkSecurityFansOutToBothWireFields is the load-bearing
// test in this file.
//
// Jamf Security Cloud declares networkSecurity and vulnerabilityManagement
// separately and refuses any request where they disagree, without the schema
// saying so. This provider models the pair as the single network_security
// attribute the console shows, so the fan-out here is what makes a valid request.
func TestBuildCreateRequest_NetworkSecurityFansOutToBothWireFields(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:      types.StringValue("example"),
		Platforms: platformSet(t, "mac"),
		Capabilities: &CapabilitiesModel{
			ContentControls: types.BoolValue(false),
			NetworkSecurity: types.BoolValue(true),
			Note:            types.StringNull(),
		},
		DeviceGroup: types.StringNull(),
	}

	request, diags := buildCreateRequest(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("buildCreateRequest: %v", diags)
	}
	if request.Capabilities.NetworkSecurity == nil || !*request.Capabilities.NetworkSecurity {
		t.Error("networkSecurity not set")
	}
	if request.Capabilities.VulnerabilityManagement == nil || !*request.Capabilities.VulnerabilityManagement {
		t.Error("vulnerabilityManagement not set — the server refuses a request where it disagrees with networkSecurity")
	}
	if request.Capabilities.DataPolicy == nil || *request.Capabilities.DataPolicy {
		t.Error("dataPolicy should be false when content_controls is false")
	}
}

// TestBuildCreateRequest_NetworkSecurityDisabledClearsBoth checks the other half
// of the coupling: both wire fields go false together.
func TestBuildCreateRequest_NetworkSecurityDisabledClearsBoth(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:      types.StringValue("example"),
		Platforms: platformSet(t, "ios"),
		Capabilities: &CapabilitiesModel{
			ContentControls: types.BoolValue(true),
			NetworkSecurity: types.BoolValue(false),
			Note:            types.StringNull(),
		},
	}

	request, diags := buildCreateRequest(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("buildCreateRequest: %v", diags)
	}
	if *request.Capabilities.NetworkSecurity || *request.Capabilities.VulnerabilityManagement {
		t.Error("both coupled fields should be false")
	}
	if !*request.Capabilities.DataPolicy {
		t.Error("dataPolicy should be true when content_controls is true")
	}
}

// TestBuildCreateRequest_OriginIsAlwaysTheSDKConstant guards that origin is sent
// and is never a literal. The server refuses a create without it, and reports an
// out-of-enum value as "Origin not provided.", which misattributes the cause.
func TestBuildCreateRequest_OriginIsAlwaysTheSDKConstant(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:         types.StringValue("example"),
		Platforms:    platformSet(t, "mac"),
		Capabilities: &CapabilitiesModel{ContentControls: types.BoolValue(true), NetworkSecurity: types.BoolValue(false), Note: types.StringNull()},
	}
	request, diags := buildCreateRequest(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("buildCreateRequest: %v", diags)
	}
	if request.Origin != securitycloud.PublicApiCreateActivationProfileRequestOriginPublicApi {
		t.Errorf("Origin = %q, want the SDK PUBLIC_API constant", request.Origin)
	}
}

// TestBuildCreateRequest_PlatformsTranslateAndSortStably keeps the request body
// deterministic, so a re-plan does not churn on element order.
func TestBuildCreateRequest_PlatformsTranslateAndSortStably(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:         types.StringValue("example"),
		Platforms:    platformSet(t, "mac", "ios"),
		Capabilities: &CapabilitiesModel{ContentControls: types.BoolValue(true), NetworkSecurity: types.BoolValue(false), Note: types.StringNull()},
	}
	request, diags := buildCreateRequest(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("buildCreateRequest: %v", diags)
	}
	want := []string{
		securitycloud.PublicApiCreateActivationProfileRequestPlatformsIOS,
		securitycloud.PublicApiCreateActivationProfileRequestPlatformsMac,
	}
	if !slices.Equal(request.Platforms, want) {
		t.Errorf("Platforms = %v, want %v", request.Platforms, want)
	}
}

// TestBuildCreateRequest_OptionalFieldsOmittedWhenNull keeps a null note and a
// null device group out of the body entirely rather than sending "".
func TestBuildCreateRequest_OptionalFieldsOmittedWhenNull(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:         types.StringValue("example"),
		Platforms:    platformSet(t, "mac"),
		Capabilities: &CapabilitiesModel{ContentControls: types.BoolValue(true), NetworkSecurity: types.BoolValue(false), Note: types.StringNull()},
		DeviceGroup:  types.StringNull(),
	}
	request, diags := buildCreateRequest(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("buildCreateRequest: %v", diags)
	}
	if request.Capabilities.Note != nil {
		t.Errorf("Note = %q, want omitted", *request.Capabilities.Note)
	}
	if request.GroupID != nil {
		t.Errorf("GroupID = %q, want omitted", *request.GroupID)
	}
}

// TestBuildCreateRequest_OptionalFieldsSentWhenSet is the counterpart.
func TestBuildCreateRequest_OptionalFieldsSentWhenSet(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:         types.StringValue("example"),
		Platforms:    platformSet(t, "mac"),
		Capabilities: &CapabilitiesModel{ContentControls: types.BoolValue(true), NetworkSecurity: types.BoolValue(false), Note: types.StringValue("compliance")},
		DeviceGroup:  types.StringValue("group-1"),
	}
	request, diags := buildCreateRequest(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("buildCreateRequest: %v", diags)
	}
	if request.Capabilities.Note == nil || *request.Capabilities.Note != "compliance" {
		t.Error("Note not sent")
	}
	if request.GroupID == nil || *request.GroupID != "group-1" {
		t.Error("GroupID not sent")
	}
}

// TestBuildCreateRequest_UnknownPlatformIsADiagnostic checks an unmappable label
// raises a diagnostic rather than silently sending an empty platform.
func TestBuildCreateRequest_UnknownPlatformIsADiagnostic(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:         types.StringValue("example"),
		Platforms:    platformSet(t, "windows"),
		Capabilities: &CapabilitiesModel{ContentControls: types.BoolValue(true), NetworkSecurity: types.BoolValue(false), Note: types.StringNull()},
	}
	if _, diags := buildCreateRequest(context.Background(), model); !diags.HasError() {
		t.Error("expected a diagnostic for an unsupported platform")
	}
}

// TestBuildCreateRequest_NoCapabilityEnabledIsAFieldAttributedDiagnostic covers
// the case the config validator cannot reach.
//
// A capability taken from another resource's computed attribute is unknown at
// plan time, so the validator defers; if it resolves to false the server refuses
// the create with an unattributed 400 that names no field. This is the last place
// the rule can be enforced with an attribute path attached, so the diagnostic has
// to carry one.
func TestBuildCreateRequest_NoCapabilityEnabledIsAFieldAttributedDiagnostic(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:      types.StringValue("example"),
		Platforms: platformSet(t, "mac"),
		Capabilities: &CapabilitiesModel{
			ContentControls: types.BoolValue(false),
			NetworkSecurity: types.BoolValue(false),
			Note:            types.StringNull(),
		},
	}

	request, diags := buildCreateRequest(context.Background(), model)
	if request != nil {
		t.Error("a request the server is certain to refuse must not be built")
	}
	if !diags.HasError() {
		t.Fatal("expected a diagnostic when no capability is enabled")
	}
	if diags[0].Summary() != "No service capability enabled" {
		t.Errorf("summary = %q, want the shared no-capability summary", diags[0].Summary())
	}
	withPath, ok := diags[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("diagnostic %T carries no path; it must point at the capabilities block", diags[0])
	}
	if !withPath.Path().Equal(path.Root("capabilities")) {
		t.Errorf("path = %s, want capabilities", withPath.Path())
	}
}

// TestBuildCreateRequest_MissingCapabilitiesBlockIsTheSameFailure keeps the nil
// block from being a silent omission. The block is Required in the schema, so nil
// is unreachable from a real plan, but sending no capabilities at all is exactly
// the request the server refuses — so it fails the same way rather than being
// built and refused on the wire.
func TestBuildCreateRequest_MissingCapabilitiesBlockIsTheSameFailure(t *testing.T) {
	model := &ActivationProfileResourceModel{
		Name:      types.StringValue("example"),
		Platforms: platformSet(t, "mac"),
	}

	request, diags := buildCreateRequest(context.Background(), model)
	if request != nil {
		t.Error("a request with no capabilities must not be built")
	}
	if !diags.HasError() {
		t.Fatal("expected a diagnostic when the capabilities block is absent")
	}
	if diags[0].Summary() != "No service capability enabled" {
		t.Errorf("summary = %q, want the shared no-capability summary", diags[0].Summary())
	}
}
