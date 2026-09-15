// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// serverIPSecGateway is the shape a read of an IPsec gateway returns: no secret,
// a read-only auth method on each endpoint, and the customer subnets in whatever
// order the server chose.
func serverIPSecGateway() *securitycloud.Gateway {
	tunnelState := "UP"
	return &securitycloud.Gateway{
		ID:           "c08e",
		Name:         "tf-ipsec",
		Datacenter:   "eu-west-2",
		Enabled:      true,
		Contact:      &securitycloud.GatewayContact{Name: "TF", Email: "tf@example.com"},
		TenantIds:    []string{probeTenant},
		DedicatedIps: &securitycloud.DedicatedIps{Enabled: false},
		Status:       &securitycloud.GatewayStatus{State: "UP", TunnelState: &tunnelState},
		Ipsec: &securitycloud.GatewayIpSec{
			KeyExchange: "ikev2",
			Ike:         &securitycloud.CipherSuiteConfig{Encryption: []string{"aes256"}, Integrity: []string{"sha512"}, DhGroups: []string{"modp2048"}, LifetimeInSec: 28800},
			Esp:         &securitycloud.CipherSuiteConfig{Encryption: []string{"aes256"}, Integrity: []string{"sha512"}, DhGroups: []string{"modp2048"}, LifetimeInSec: 28800},
			Left:        &securitycloud.ConnectionConfigLeftResponse{Auth: "psk", Host: "%any", ID: "wpa.wandera.com", Subnets: []string{"172.16.0.0/12"}},
			Right:       &securitycloud.ConnectionConfigRightResponse{Auth: "psk", Host: "198.51.100.7", ID: "peer.example.com", Subnets: []string{"192.168.5.0/24", "10.10.0.0/16"}, Vendor: "strongSwan"},
		},
	}
}

// serverInternetGateway is the shape a read of a dedicated internet gateway
// returns, with the egress addresses the server assigned.
func serverInternetGateway() *securitycloud.Gateway {
	ips := []string{"18.133.48.33", "18.168.96.167"}
	return &securitycloud.Gateway{
		ID:           "e045",
		Name:         "tf-internet",
		Datacenter:   "eu-west-2",
		Enabled:      true,
		Contact:      &securitycloud.GatewayContact{Name: "TF", Email: "tf@example.com"},
		TenantIds:    []string{probeTenant},
		DedicatedIps: &securitycloud.DedicatedIps{Enabled: true, Ips: &ips},
		Status:       &securitycloud.GatewayStatus{State: "PENDING"},
	}
}

func TestAssignGatewayResourceModel_IPSecGateway(t *testing.T) {
	state := ipsecGatewayPlan(t, 3)
	diags := assignGatewayResourceModel(context.Background(), &state, serverIPSecGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.IPSec == nil {
		t.Fatal("the IPsec block must survive the read")
	}
	if got := state.IPSec.Phase1.Encryption.ValueString(); got != "AES-256" {
		t.Errorf("phase_1.encryption = %q, want the admin-UI label for the single value collapsed out of the wire array", got)
	}
	if got := state.IPSec.Phase1.DiffieHellmanGroup.ValueString(); got != "Group 14 (modp2048)" {
		t.Errorf("phase_1.diffie_hellman_group = %q, want the admin-UI label", got)
	}
	if got := state.IPSec.KeyExchangeProtocol.ValueString(); got != "IKEv2" {
		t.Errorf("key_exchange_protocol = %q, want the admin-UI label", got)
	}
	if got := state.EgressRegion.ValueString(); got != "Europe - UK" {
		t.Errorf("egress_region = %q, want the admin-UI label", got)
	}
	if got := state.IPSec.JamfSide.Subnet.ValueString(); got != "172.16.0.0/12" {
		t.Errorf("jamf_side.subnet = %q", got)
	}
	if got := state.IPSec.JamfSide.AuthMethod.ValueString(); got != "psk" {
		t.Errorf("jamf_side.auth_method = %q, want the server value", got)
	}
	if len(state.IPSec.CustomerSide.Subnets.Elements()) != 2 {
		t.Errorf("customer_side.subnets = %v, want 2 entries", state.IPSec.CustomerSide.Subnets.Elements())
	}
}

// TestAssignGatewayResourceModel_PreservesRotationTrigger guards the field the
// wire cannot supply. Losing it would make the very next update look like a
// rotation and re-send whatever secret the config held.
func TestAssignGatewayResourceModel_PreservesRotationTrigger(t *testing.T) {
	state := ipsecGatewayPlan(t, 7)
	diags := assignGatewayResourceModel(context.Background(), &state, serverIPSecGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if got := state.IPSec.JamfSide.AuthenticationSecretWoVersion.ValueInt64(); got != 7 {
		t.Errorf("authentication_secret_wo_version = %d, want the value carried over from the prior model", got)
	}
}

// TestAssignGatewayResourceModel_InternetGatewayClearsIPSec covers the other form:
// a read of an internet gateway must leave no IPsec block behind, and must surface
// the egress addresses the server assigned.
func TestAssignGatewayResourceModel_InternetGatewayClearsIPSec(t *testing.T) {
	state := ipsecGatewayPlan(t, 1)
	diags := assignGatewayResourceModel(context.Background(), &state, serverInternetGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.IPSec != nil {
		t.Error("a gateway with no IPsec configuration must clear the block from state")
	}
	if len(state.DedicatedEgressIPAddresses.Elements()) != 2 {
		t.Errorf("dedicated_egress_ip_addresses = %v, want the 2 addresses the server assigned", state.DedicatedEgressIPAddresses.Elements())
	}
}

// TestAssignGatewayResourceModel_AbsentAvailabilityZonesStayNull pins the null-vs-
// empty distinction. The server answers null for a gateway created without zones;
// writing an empty set over a null config value would produce a permanent diff.
func TestAssignGatewayResourceModel_AbsentAvailabilityZonesStayNull(t *testing.T) {
	state := ipsecGatewayPlan(t, 1)
	diags := assignGatewayResourceModel(context.Background(), &state, serverIPSecGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !state.IPSecSourceIPAddresses.IsNull() {
		t.Errorf("availability_zones = %v, want null when the server reports none", state.IPSecSourceIPAddresses)
	}
}

// TestAssignGatewayResourceModel_PreservesIDWhenResponseOmitsIt guards the
// post-create read: Create sets the ID from the create response, and a read that
// came back without one must not blank it.
func TestAssignGatewayResourceModel_PreservesIDWhenResponseOmitsIt(t *testing.T) {
	state := GatewayResourceModel{ID: types.StringValue("kept-id")}
	gateway := serverIPSecGateway()
	gateway.ID = ""

	diags := assignGatewayResourceModel(context.Background(), &state, gateway)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "kept-id" {
		t.Errorf("ID = %q, want the value already in state", state.ID.ValueString())
	}
}

// TestStatusObjectValue_NoTunnelStateForInternetGateway covers the nullable field:
// a gateway with no tunnel reports none, and that has to reach state as null
// rather than an empty string.
func TestStatusObjectValue_NoTunnelStateForInternetGateway(t *testing.T) {
	status, diags := statusObjectValue(serverInternetGateway().Status)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	attrs := status.Attributes()
	if attrs["state"].(types.String).ValueString() != "PENDING" {
		t.Errorf("state = %v", attrs["state"])
	}
	if !attrs["tunnel_state"].(types.String).IsNull() {
		t.Errorf("tunnel_state = %v, want null on a gateway with no tunnel", attrs["tunnel_state"])
	}
}

// TestFirstOrNull_EmptyWireArray guards the collapse against a response that
// breaks the "exactly one element" contract: null is right, a panic is not.
func TestFirstOrNull_EmptyWireArray(t *testing.T) {
	if !firstOrNull(nil).IsNull() {
		t.Error("an empty wire array must collapse to null")
	}
	if got := firstOrNull([]string{"aes256", "aes128"}).ValueString(); got != "aes256" {
		t.Errorf("got %q, want the first element", got)
	}
}

func TestAssignGatewayDataSourceModel_ReportsGatewayForm(t *testing.T) {
	var internet GatewayDataSourceModel
	if diags := assignGatewayDataSourceModel(context.Background(), &internet, serverInternetGateway()); diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !internet.DedicatedEgressIPsEnabled.ValueBool() {
		t.Error("an internet gateway must report dedicated_egress_ips_enabled = true")
	}
	if !internet.IPSec.IsNull() {
		t.Error("an internet gateway must report a null ipsec block")
	}

	var ipsec GatewayDataSourceModel
	if diags := assignGatewayDataSourceModel(context.Background(), &ipsec, serverIPSecGateway()); diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if ipsec.DedicatedEgressIPsEnabled.ValueBool() {
		t.Error("an IPsec gateway must report dedicated_egress_ips_enabled = false")
	}
	if ipsec.IPSec.IsNull() {
		t.Error("an IPsec gateway must report its ipsec block")
	}
}

func TestBuildGatewaysResultModel_PopulatesEveryField(t *testing.T) {
	got, diags := buildGatewaysResultModel(context.Background(), *serverIPSecGateway())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.ID.ValueString() == "" || got.Name.ValueString() == "" {
		t.Errorf("result model lost an identity field: %+v", got)
	}
	if got.IPSec.IsNull() || got.Status.IsNull() || got.Contact.IsNull() {
		t.Errorf("result model lost a nested block: %+v", got)
	}
}
