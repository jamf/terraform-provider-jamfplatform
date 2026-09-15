// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// serverGroupedGateway is the shape a read returns.
func serverGroupedGateway() *securitycloud.GroupedGateway {
	created := time.Date(2026, 8, 27, 11, 45, 56, 0, time.UTC)
	return &securitycloud.GroupedGateway{
		ID:                 "b6ed74d2-165c-4503-8572-07eb8ef44195",
		Name:               "tf-group",
		GatewayIds:         []string{"9702", "df90", "c6f0"},
		RoutingStrategy:    "NEAREST",
		RecoveryDelayInSec: 1800,
		TenantIds:          []string{probeTenant},
		CreatedAt:          created,
		UpdatedAt:          created.Add(time.Hour),
	}
}

func TestAssignGroupedGatewayResourceModel_PopulatesEveryField(t *testing.T) {
	var state GroupedGatewayResourceModel
	diags := assignGroupedGatewayResourceModel(context.Background(), &state, serverGroupedGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.Name.ValueString() != "tf-group" {
		t.Errorf("name = %q", state.Name.ValueString())
	}
	if state.RoutingStrategy.ValueString() != "Nearest" {
		t.Errorf("routing_strategy = %q, want the admin-UI label", state.RoutingStrategy.ValueString())
	}
	if state.RequiredGatewayStability.ValueString() != "30 minutes" {
		t.Errorf("required_gateway_stability = %q, want the admin-UI label", state.RequiredGatewayStability.ValueString())
	}
	if state.CreatedAt.ValueString() != "2026-08-27T11:45:56Z" {
		t.Errorf("created_at = %q, want an RFC 3339 timestamp", state.CreatedAt.ValueString())
	}
	if len(state.TenantIDs.Elements()) != 1 {
		t.Errorf("tenant_ids = %v, want 1 entry", state.TenantIDs.Elements())
	}
}

// TestAssignGroupedGatewayResourceModel_PreservesMemberOrder is the read-side half
// of the ordering contract: the server stores membership order verbatim, so state
// must reflect it in the same order rather than normalising.
func TestAssignGroupedGatewayResourceModel_PreservesMemberOrder(t *testing.T) {
	var state GroupedGatewayResourceModel
	diags := assignGroupedGatewayResourceModel(context.Background(), &state, serverGroupedGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	var ids []string
	if convDiags := state.GatewayIDs.ElementsAs(context.Background(), &ids, false); convDiags.HasError() {
		t.Fatalf("converting gateway_ids: %v", convDiags)
	}
	want := []string{"9702", "df90", "c6f0"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("gateway_ids = %v, want %v", ids, want)
		}
	}
}

// TestAssignGroupedGatewayResourceModel_PreservesIDWhenResponseOmitsIt guards the
// post-create read: Create sets the ID from the create response, and a read that
// came back without one must not blank it.
func TestAssignGroupedGatewayResourceModel_PreservesIDWhenResponseOmitsIt(t *testing.T) {
	state := GroupedGatewayResourceModel{ID: types.StringValue("kept-id")}
	group := serverGroupedGateway()
	group.ID = ""

	diags := assignGroupedGatewayResourceModel(context.Background(), &state, group)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "kept-id" {
		t.Errorf("ID = %q, want the value already in state", state.ID.ValueString())
	}
}

func TestAssignGroupedGatewayDataSourceModel_UsesLists(t *testing.T) {
	var state GroupedGatewayDataSourceModel
	diags := assignGroupedGatewayDataSourceModel(context.Background(), &state, serverGroupedGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.TenantIDs.IsNull() || len(state.TenantIDs.Elements()) != 1 {
		t.Errorf("tenant_ids = %v, want a 1-element list", state.TenantIDs)
	}
	if len(state.GatewayIDs.Elements()) != 3 {
		t.Errorf("gateway_ids = %v, want 3 entries", state.GatewayIDs.Elements())
	}
}

func TestBuildGroupedGatewaysResultModel_PopulatesEveryField(t *testing.T) {
	got, diags := buildGroupedGatewaysResultModel(context.Background(), *serverGroupedGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.ID.ValueString() == "" || got.Name.ValueString() == "" {
		t.Errorf("result model lost an identity field: %+v", got)
	}
	if len(got.GatewayIDs.Elements()) != 3 || got.CreatedAt.ValueString() == "" {
		t.Errorf("result model lost a field: %+v", got)
	}
}
