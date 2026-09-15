// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// TestGeneratedConfigurationPassesItsOwnValidators is a class guard, not a case
// guard.
//
// `terraform plan -generate-config-out` writes state back out as configuration, so
// any attribute the read commits that a per-connection-type validator then refuses
// produces a file that cannot be planned. This resource has been caught by that
// twice: `pkce`, recorded for an Entra connection the console offers no PKCE
// setting for (issue #379), and `auth_method`, recorded for a Google Workspace
// connection the console offers no authentication method for — adjacent lines of
// the same state builder, one gated and one not.
//
// Fixing them one at a time is what let the second follow the first. So rather
// than assert the two attributes, this builds the resource model from a wire read
// for EVERY connection type, turns it into a configuration, and runs the
// resource's own cross-field validator over it. Any future attribute that a type
// forbids and the read adopts fails here, named by the validator's own diagnostic,
// whether or not anyone thought to test it.
//
// The wire reads deliberately populate every field Jamf returns for every type,
// including the ones a given type forbids: that is what Jamf actually does, and a
// fixture that omitted them would pass while the tenant failed. Probed against an
// organization-scoped integration on 2026-09-05, five connections covering all
// four families:
//
//	tokenEndpointAuthMethod   CLIENT_SECRET_POST on all five, GOOGLE_APPS included
//	pkceAuthType              returned for every family
//	scopes                    null on both WAAD connections, set on the other three
//
// The scopes result is why `scopes` needs no gate even though validateScopes
// refuses a configured value on an Entra connection: Jamf returns none there, so
// preferEquivalentScopes yields null on its own.
//
// # One rule this does not cover, and why
//
// validateEntraGroupOptions (rule 14) refuses `entra.groups_scope` and
// `entra.include_nested_groups` when `entra.get_user_groups` is false, and
// buildEntraStateModel adopts all three ungated — so that combination would
// generate configuration this resource refuses. It is left alone because the wire
// has not been seen to produce it: both Entra connections probed return
// groups:true with a scope set, and no read has shown groups:false alongside
// either field. Gating it on that evidence would be inventing a rule, and it is
// not even clear which side is wrong — if Jamf does return the pair, dropping two
// real values may be worse than relaxing rule 14. Settling it needs one probe: an
// Entra connection with group membership turned off after a scope was set.
func TestGeneratedConfigurationPassesItsOwnValidators(t *testing.T) {
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	NewConnectionResource().(*ConnectionResource).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}

	for _, tc := range []struct {
		name string
		read *account.Connection
	}{
		{"generic_oidc", genericOIDCWireRead()},
		{"entra", entraWireRead()},
		{"okta", oktaWireRead()},
		{"google_workspace", googleWireRead()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ConnectionResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(connectionTimeoutAttributeTypes),
			}
			if diags := assignConnectionResourceModel(&state, tc.read, oidcSummaryRead(), true); diags.HasError() {
				t.Fatalf("building state: %v", diags)
			}

			generated := tfsdk.State{Schema: schemaResp.Schema}
			if diags := generated.Set(ctx, &state); diags.HasError() {
				t.Fatalf("rendering the generated configuration: %v", diags)
			}

			resp := &validator.StringResponse{}
			connectionSettingsValidator{}.ValidateString(ctx, validator.StringRequest{
				Path:           path.Root("connection_type"),
				PathExpression: path.MatchRoot("connection_type"),
				Config:         tfsdk.Config{Schema: schemaResp.Schema, Raw: generated.Raw},
				ConfigValue:    state.ConnectionType,
			}, resp)

			for _, d := range resp.Diagnostics.Errors() {
				at := ""
				if withPath, ok := d.(interface{ Path() path.Path }); ok {
					at = withPath.Path().String() + ": "
				}
				t.Errorf("configuration generated for a %q connection is refused by this resource's own "+
					"validators — %s%s: %s", tc.name, at, d.Summary(), d.Detail())
			}
		})
	}
}

// wireRead returns a connection populated the way Jamf answers for every family,
// with the type-specific settings block left to the caller.
//
// Every field Jamf returns regardless of type is set here, including the two this
// resource gates on the connection type. Leaving those out of a fixture is how a
// gap of this class hides.
func wireRead(connectionType string) *account.Connection {
	return &account.Connection{
		ID:                               unitConnectionID,
		Name:                             unitConnectionName,
		Type:                             new(connectionType),
		Region:                           new(account.RegionUs),
		ClientID:                         new("probe-client-id"),
		Domains:                          []string{"tf-unit.example"},
		PkceAuthType:                     new(account.PkceAuthTypeDisabled),
		TokenEndpointAuthMethod:          new(account.TokenEndpointAuthMethodClientSecretPost),
		SendNonce:                        false,
		SyncUserProfileAttributesAtLogin: true,
		AliasLoginHintToIdp:              true,
		SessionInfo:                      &account.SessionInfo{},
	}
}

func genericOIDCWireRead() *account.Connection {
	c := wireRead(account.ConnectionTypeOidc)
	c.Scopes = new("openid email profile")
	c.OidcOptions = &account.OidcOptions{
		IssuerURL:             new("idp.example"),
		AuthorizationEndpoint: new("idp.example/authorize"),
		TokenEndpoint:         new("idp.example/token"),
		JwksUri:               new("idp.example/keys"),
	}
	return c
}

func oktaWireRead() *account.Connection {
	c := wireRead(account.ConnectionTypeOkta)
	c.Scopes = new("openid email profile")
	c.OktaOptions = &account.OktaOptions{Domain: new("example.okta.com"), IssuerURL: new("example.okta.com")}
	return c
}

// entraWireRead carries no scopes, which is what Jamf returns for an Entra
// connection and what the resource requires — validateScopes refuses a configured
// value there.
func entraWireRead() *account.Connection {
	c := wireRead(account.ConnectionTypeWaad)
	c.AzureOptions = &account.EntraOptions{
		Domain:       new("example.onmicrosoft.com"),
		TenantDomain: new("example.onmicrosoft.com"),
		GroupsScope:  new("GroupMember.Read.All"),
		ExtOptions:   &account.EntraExtendedOptions{Groups: new(true), NestedGroups: new(true)},
	}
	return c
}

func googleWireRead() *account.Connection {
	c := wireRead(account.ConnectionTypeGoogleApps)
	c.GoogleOptions = &account.GoogleOptions{Domain: new("example.com")}
	return c
}
