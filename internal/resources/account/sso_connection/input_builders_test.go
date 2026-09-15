// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// oidcPlanModel returns a minimal generic OpenID Connect plan, so each test can
// change the one thing it is about.
func oidcPlanModel() ConnectionResourceModel {
	return ConnectionResourceModel{
		Name:           types.StringValue(unitConnectionName),
		ConnectionType: types.StringValue(connectionTypeGenericOIDC),
		HostingRegion:  types.StringValue(account.RegionUs),
		Scopes:         types.StringValue("openid email profile"),
		ClientID:       types.StringValue("probe-client-id"),
		Domains:        mustStringSet("tf-unit.example"),
		GenericOIDC: &GenericOIDCSettingsModel{
			IssuerURL:             types.StringValue("idp.example"),
			AuthorizationEndpoint: types.StringValue("idp.example/authorize"),
			TokenEndpoint:         types.StringValue("idp.example/token"),
			JWKSURI:               types.StringValue("idp.example/keys"),
			UserInfoEndpoint:      types.StringNull(),
		},
	}
}

// mustStringSet builds a known string set for a fixture.
func mustStringSet(values ...string) types.Set {
	set, diags := stringsToSet(values)
	if diags.HasError() {
		panic(diags)
	}
	return set
}

// buildRequest runs the request builder and fails on a diagnostic.
func buildRequest(t *testing.T, plan ConnectionResourceModel, secret types.String) *account.ConnectionRequest {
	t.Helper()
	request, diags := buildConnectionRequest(context.Background(), plan, secret)
	if diags.HasError() {
		t.Fatalf("building the request: %v", diags)
	}
	return request
}

// marshalSettings serialises the settings union the way the request would, which
// is also the check that exactly one variant is set — the union refuses to
// serialise two.
func marshalSettings(t *testing.T, request *account.ConnectionRequest) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(request.Connection)
	if err != nil {
		t.Fatalf("serialising the settings: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding the settings: %v", err)
	}
	return decoded
}

// TestBuildConnectionRequest_SelectsTheSettingsVariantTheTypeNames pins the
// union. Setting two variants makes it refuse to serialise, and setting the wrong
// one is an unattributable internal failure from Jamf, so this is the only place
// the pairing is checked at all after the validator.
func TestBuildConnectionRequest_SelectsTheSettingsVariantTheTypeNames(t *testing.T) {
	cases := map[string]struct {
		plan     ConnectionResourceModel
		wantType string
		variant  func(*account.ConnectionRequestConnection) bool
	}{
		"generic_oidc": {
			plan:     oidcPlanModel(),
			wantType: account.ConnectionTypeOidc,
			variant: func(c *account.ConnectionRequestConnection) bool {
				return c.OidcConnectionSettings != nil
			},
		},
		"entra": {
			plan: ConnectionResourceModel{
				Name:           types.StringValue("tf-unit-entra"),
				ConnectionType: types.StringValue(connectionTypeEntra),
				HostingRegion:  types.StringValue(account.RegionUs),
				Domains:        mustStringSet("tf-unit.example"),
				Entra: &EntraSettingsModel{
					Domain:       types.StringValue("contoso.example"),
					TenantDomain: types.StringValue("contoso.example"),
				},
			},
			wantType: account.ConnectionTypeWaad,
			variant: func(c *account.ConnectionRequestConnection) bool {
				return c.EntraConnectionSettings != nil
			},
		},
		"okta": {
			plan: ConnectionResourceModel{
				Name:           types.StringValue("tf-unit-okta"),
				ConnectionType: types.StringValue(connectionTypeOkta),
				HostingRegion:  types.StringValue(account.RegionUs),
				Scopes:         types.StringValue("openid"),
				Domains:        mustStringSet("tf-unit.example"),
				Okta:           &OktaSettingsModel{Domain: types.StringValue("example.okta.example")},
			},
			wantType: account.ConnectionTypeOkta,
			variant: func(c *account.ConnectionRequestConnection) bool {
				return c.OktaConnectionSettings != nil
			},
		},
		"google_workspace": {
			plan: ConnectionResourceModel{
				Name:            types.StringValue("tf-unit-google"),
				ConnectionType:  types.StringValue(connectionTypeGoogle),
				HostingRegion:   types.StringValue(account.RegionUs),
				Domains:         mustStringSet("tf-unit.example"),
				GoogleWorkspace: &GoogleWorkspaceSettingsModel{Domain: types.StringValue("tf-unit.example")},
			},
			wantType: account.ConnectionTypeGoogleApps,
			variant: func(c *account.ConnectionRequestConnection) bool {
				return c.GoogleConnectionSettings != nil
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			request := buildRequest(t, tc.plan, types.StringNull())
			if request.ConnectionType != tc.wantType {
				t.Errorf("connection type = %q, want %q", request.ConnectionType, tc.wantType)
			}
			if !tc.variant(&request.Connection) {
				t.Error("the settings variant the connection type names was not set")
			}
			marshalSettings(t, request)
		})
	}
}

// TestBuildConnectionRequest_OmitsTheClientSecretWhenAbsent pins the one
// documented exception to the replacement: leaving the secret out keeps the
// stored one, so it has to be absent from the body rather than present and empty.
// Spec-derived, not wire-verified.
func TestBuildConnectionRequest_OmitsTheClientSecretWhenAbsent(t *testing.T) {
	for name, secret := range map[string]types.String{
		"absent":  types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			settings := marshalSettings(t, buildRequest(t, oidcPlanModel(), secret))
			if _, present := settings["clientSecret"]; present {
				t.Error("an unsupplied client secret must be absent from the body, not present and empty")
			}
		})
	}

	settings := marshalSettings(t, buildRequest(t, oidcPlanModel(), types.StringValue("probe-client-secret")))
	if got := settings["clientSecret"]; got != "probe-client-secret" {
		t.Errorf("clientSecret = %v, want the supplied value", got)
	}
}

// TestBuildConnectionRequest_InvertsTheLoginHint is one half of the pair that
// matters most in this package. The attribute asks whether the hint is
// *withheld*; Jamf records whether it is *forwarded*. A pair that inverts twice
// is indistinguishable from one that never inverts, so both directions are pinned
// — the other half is in state_builders_test.go.
func TestBuildConnectionRequest_InvertsTheLoginHint(t *testing.T) {
	cases := map[string]struct {
		omit types.Bool
		want bool
	}{
		"withheld":                {types.BoolValue(true), false},
		"forwarded":               {types.BoolValue(false), true},
		"absent means forwarded":  {types.BoolNull(), true},
		"unknown means forwarded": {types.BoolUnknown(), true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plan := oidcPlanModel()
			plan.OmitLoginHint = tc.omit
			settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
			if got := settings["aliasLoginHintToIdp"]; got != tc.want {
				t.Errorf("aliasLoginHintToIdp = %v, want %v — the two senses are opposites", got, tc.want)
			}
		})
	}
}

// TestBuildConnectionRequest_RendersTheGroupFilter pins the document Jamf stores
// the filter in, and the distinction the block exists to preserve: an absent
// block sends no filter, and a present block with no groups sends an operator and
// an empty list, which is a different value and the shape most connections carry.
func TestBuildConnectionRequest_RendersTheGroupFilter(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		settings := marshalSettings(t, buildRequest(t, oidcPlanModel(), types.StringNull()))
		if _, present := settings["groupNameFilter"]; present {
			t.Error("an absent block must send no filter at all")
		}
	})

	t.Run("an operator with no groups", func(t *testing.T) {
		plan := oidcPlanModel()
		plan.GroupNameFilter = &GroupNameFilterModel{
			Operator: types.StringValue(filterOperatorOr),
			Groups:   mustStringSet(),
		}
		settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
		if got := settings["groupNameFilter"]; got != `{"op":"OR","groups":""}` {
			t.Errorf("groupNameFilter = %v, want an operator with an empty list", got)
		}
	})

	t.Run("groups joined in a stable order", func(t *testing.T) {
		plan := oidcPlanModel()
		plan.GroupNameFilter = &GroupNameFilterModel{
			Operator: types.StringValue(filterOperatorAnd),
			Groups:   mustStringSet("zebra", "apple", "mango"),
		}
		settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
		if got := settings["groupNameFilter"]; got != `{"op":"AND","groups":"apple,mango,zebra"}` {
			t.Errorf("groupNameFilter = %v, want the members sorted so the same set always sends the same value", got)
		}
	})
}

// TestBuildConnectionRequest_AlwaysSendsTheProductCollection pins the shape Jamf
// requires. It names the field in a refusal when it is missing while accepting an
// empty one, which is its documented way of saying the connection is enabled for
// no tenant-scoped product — so an unconfigured collection is sent empty rather
// than left out.
func TestBuildConnectionRequest_AlwaysSendsTheProductCollection(t *testing.T) {
	request := buildRequest(t, oidcPlanModel(), types.StringNull())
	if request.EnabledProducts == nil {
		t.Error("the product collection must be sent, empty rather than absent")
	}
	if len(request.EnabledProducts) != 0 {
		t.Errorf("enabledProducts = %v, want an empty collection", request.EnabledProducts)
	}
	if request.EnabledEnvironments != nil {
		t.Error("the environment collection is genuinely optional and must be left out when unconfigured")
	}
}

// TestBuildConnectionRequest_RendersTheProductAssignments pins the collection
// Terraform can write and never read back, including the partner-managed account
// identifier — which is a write-only field of a write-only collection, so this
// test is the only thing that will ever confirm it reaches Jamf.
func TestBuildConnectionRequest_RendersTheProductAssignments(t *testing.T) {
	plan := oidcPlanModel()
	plan.EnabledProducts = []EnabledProductModel{
		{
			Product:          types.StringValue(account.ProductPro),
			Tenants:          mustStringSet("tenant-one", "tenant-two"),
			ManagedAccountID: types.StringValue("managed-account-one"),
		},
		{
			Product: types.StringValue(account.ProductAccount),
		},
	}
	plan.EnabledEnvironments = []EnabledEnvironmentModel{
		{
			Product:      types.StringValue(account.ProductProtect),
			Environments: mustStringSet("environment-one"),
		},
	}

	request := buildRequest(t, plan, types.StringNull())

	if len(request.EnabledProducts) != 2 {
		t.Fatalf("enabledProducts = %v, want both entries", request.EnabledProducts)
	}
	first := request.EnabledProducts[0]
	if first.Product != account.ProductPro || len(first.EnabledTenants) != 2 {
		t.Errorf("first product = %+v, want the product and both tenants", first)
	}
	if first.ManagedAccountID == nil || *first.ManagedAccountID != "managed-account-one" {
		t.Errorf("managedAccountId = %v, want the partner-managed account carried through", first.ManagedAccountID)
	}
	second := request.EnabledProducts[1]
	if second.EnabledTenants == nil || len(second.EnabledTenants) != 0 {
		t.Errorf("second product tenants = %v, want an empty collection for an organization-scoped product", second.EnabledTenants)
	}
	if second.ManagedAccountID != nil {
		t.Error("an unconfigured partner-managed account must be left out")
	}

	if request.EnabledEnvironments == nil || len(*request.EnabledEnvironments) != 1 {
		t.Fatalf("enabledEnvironments = %v, want the one entry", request.EnabledEnvironments)
	}
	if got := (*request.EnabledEnvironments)[0]; got.Product != account.ProductProtect || len(got.EnabledEnvironments) != 1 {
		t.Errorf("environment entry = %+v, want the product and its environment", got)
	}
}

// TestBuildConnectionRequest_AlwaysSendsTheSessionObject pins that the session
// limits are sent as an object with explicit absences rather than left out. That
// is how Jamf reports them — every connection read carries the object with both
// properties present and empty where the default applies — so sending it the same
// way says "use the defaults" in Jamf's own words.
func TestBuildConnectionRequest_AlwaysSendsTheSessionObject(t *testing.T) {
	settings := marshalSettings(t, buildRequest(t, oidcPlanModel(), types.StringNull()))
	session, present := settings["sessionInfo"].(map[string]any)
	if !present {
		t.Fatalf("sessionInfo = %v, want the object present", settings["sessionInfo"])
	}
	if len(session) != 0 {
		t.Errorf("sessionInfo = %v, want no limits set", session)
	}

	plan := oidcPlanModel()
	plan.SessionDurationMinutes = types.Int64Value(480)
	plan.InactivityTimeoutMinutes = types.Int64Value(30)
	settings = marshalSettings(t, buildRequest(t, plan, types.StringNull()))
	session = settings["sessionInfo"].(map[string]any)
	if session["maxSessionTimeInMinutes"] != float64(480) || session["maxInactivityTimeInMinutes"] != float64(30) {
		t.Errorf("sessionInfo = %v, want both limits", session)
	}
}

// TestBuildConnectionRequest_TranslatesTheRenamedVocabularies pins that a value
// a practitioner writes in the console's words leaves as Jamf's. Jamf refuses an
// unrecognised value naming it but not the field it was on, so a translation this
// builder forgot would surface as an unattributed refusal.
func TestBuildConnectionRequest_TranslatesTheRenamedVocabularies(t *testing.T) {
	plan := oidcPlanModel()
	plan.AuthMethod = types.StringValue(authMethodPrivateKeyJWT)
	plan.PKCE = types.StringValue(pkceS256)

	settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
	if got := settings["tokenEndpointAuthMethod"]; got != account.TokenEndpointAuthMethodPrivateKeyJwt {
		t.Errorf("tokenEndpointAuthMethod = %v, want Jamf's own spelling", got)
	}
	if got := settings["pkceAuthType"]; got != account.PkceAuthTypeS256 {
		t.Errorf("pkceAuthType = %v, want Jamf's own spelling", got)
	}
}

// TestBuildEntraSettings_SuppliesTheRequiredDefaults pins the two values Jamf
// requires and documents no default for. They live in the builder rather than in
// the schema so they stay visible in one place and out of the way of the plan,
// where a schema default would compete with the value carried forward from prior
// state.
func TestBuildEntraSettings_SuppliesTheRequiredDefaults(t *testing.T) {
	plan := ConnectionResourceModel{
		Name:           types.StringValue("tf-unit-entra"),
		ConnectionType: types.StringValue(connectionTypeEntra),
		HostingRegion:  types.StringValue(account.RegionUs),
		Domains:        mustStringSet("tf-unit.example"),
		Entra: &EntraSettingsModel{
			Domain:       types.StringValue("contoso.example"),
			TenantDomain: types.StringValue("contoso.example"),
		},
	}

	settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
	if got := settings["identityApi"]; got != account.EntraIdentityApiMicrosoftIdentityPlatformV2 {
		t.Errorf("identityApi = %v, want the version every Entra connection read carried", got)
	}
	if got := settings["maxGroups"]; got != float64(defaultMaxGroups) {
		t.Errorf("maxGroups = %v, want the console's own default", got)
	}
	if got := settings["setEmailsVerified"]; got != true {
		t.Errorf("setEmailsVerified = %v, want the console's own default", got)
	}
	if got := settings["basicProfile"]; got != true {
		t.Errorf("basicProfile = %v, want it always on — the console shows it ticked and greyed out", got)
	}
}

// TestBuildEntraSettings_ConfiguredValuesWinOverTheDefaults is the other side of
// the previous test: a default supplied in the builder must not override
// something the operator asked for.
func TestBuildEntraSettings_ConfiguredValuesWinOverTheDefaults(t *testing.T) {
	plan := ConnectionResourceModel{
		Name:           types.StringValue("tf-unit-entra"),
		ConnectionType: types.StringValue(connectionTypeEntra),
		HostingRegion:  types.StringValue(account.RegionUs),
		Domains:        mustStringSet("tf-unit.example"),
		Entra: &EntraSettingsModel{
			Domain:            types.StringValue("contoso.example"),
			TenantDomain:      types.StringValue("contoso.example"),
			IdentityAPI:       types.StringValue(account.EntraIdentityApiAzureActiveDirectoryV1),
			MaxGroups:         types.Int64Value(500),
			SetEmailsVerified: types.BoolValue(false),
			GetUserGroups:     types.BoolValue(true),
		},
	}

	settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
	if got := settings["identityApi"]; got != account.EntraIdentityApiAzureActiveDirectoryV1 {
		t.Errorf("identityApi = %v, want the configured value", got)
	}
	if got := settings["maxGroups"]; got != float64(500) {
		t.Errorf("maxGroups = %v, want the configured value", got)
	}
	if got := settings["setEmailsVerified"]; got != false {
		t.Errorf("setEmailsVerified = %v, want the configured value", got)
	}
	if got := settings["groups"]; got != true {
		t.Errorf("groups = %v, want the renamed get_user_groups carried through", got)
	}
}

// TestBuildGoogleSettings_RenamesTheGroupSwitches pins the Google mapping, which
// is a trap worth a test of its own: Jamf's own read shape calls the group switch
// extGroups and the directory read extGroupsExtended, so the shorter name is not
// the simpler option and the write shape calls neither of them that.
func TestBuildGoogleSettings_RenamesTheGroupSwitches(t *testing.T) {
	plan := ConnectionResourceModel{
		Name:           types.StringValue("tf-unit-google"),
		ConnectionType: types.StringValue(connectionTypeGoogle),
		HostingRegion:  types.StringValue(account.RegionUs),
		Domains:        mustStringSet("tf-unit.example"),
		GoogleWorkspace: &GoogleWorkspaceSettingsModel{
			Domain:         types.StringValue("tf-unit.example"),
			GetUserGroups:  types.BoolValue(true),
			ExtendedGroups: types.BoolValue(true),
			EnableUsersAPI: types.BoolValue(true),
		},
	}

	settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
	if got := settings["groups"]; got != true {
		t.Errorf("groups = %v, want get_user_groups", got)
	}
	if got := settings["extendedGroups"]; got != true {
		t.Errorf("extendedGroups = %v, want extended_groups", got)
	}
	if got := settings["apiEnableUsers"]; got != true {
		t.Errorf("apiEnableUsers = %v, want enable_users_api", got)
	}
}

// TestBuildOktaSettings_SendsOnlyTheDomain pins that the four addresses are never
// sent. Jamf works them out from the org domain, which is why they are reported
// rather than declared — sending a stale copy back would be sending Jamf its own
// derivation as an instruction.
func TestBuildOktaSettings_SendsOnlyTheDomain(t *testing.T) {
	plan := ConnectionResourceModel{
		Name:           types.StringValue("tf-unit-okta"),
		ConnectionType: types.StringValue(connectionTypeOkta),
		HostingRegion:  types.StringValue(account.RegionUs),
		Scopes:         types.StringValue("openid"),
		Domains:        mustStringSet("tf-unit.example"),
		Okta: &OktaSettingsModel{
			Domain:                types.StringValue("example.okta.example"),
			IssuerURL:             types.StringValue("example.okta.example"),
			AuthorizationEndpoint: types.StringValue("example.okta.example/oauth2/v1/authorize"),
			TokenEndpoint:         types.StringValue("example.okta.example/oauth2/v1/token"),
			JWKSURI:               types.StringValue("example.okta.example/oauth2/v1/keys"),
		},
	}

	settings := marshalSettings(t, buildRequest(t, plan, types.StringNull()))
	if got := settings["domain"]; got != "example.okta.example" {
		t.Errorf("domain = %v, want the org domain", got)
	}
	for _, derived := range []string{"issuerUrl", "authorizationEndpoint", "tokenEndpoint", "jwksUri"} {
		if _, present := settings[derived]; present {
			t.Errorf("the settings carry %q, which Jamf works out from the domain itself", derived)
		}
	}
}

// TestBuildConnectionRequest_RefusesAnUnknownConnectionType pins the guard on the
// vocabulary translation. It can only fire through a provider defect — the
// accepted values come from the same table — so it reports one rather than
// sending Jamf an empty type.
func TestBuildConnectionRequest_RefusesAnUnknownConnectionType(t *testing.T) {
	plan := oidcPlanModel()
	plan.ConnectionType = types.StringValue("something_new")

	if _, diags := buildConnectionRequest(context.Background(), plan, types.StringNull()); !diags.HasError() {
		t.Fatal("a connection type with no Jamf equivalent must be reported, not sent as nothing")
	}
}

// TestBuildConnectionRequest_RefusesAMissingSettingsBlock pins the second such
// guard. The validator should have caught it, so this reports a provider defect
// rather than a configuration mistake — but sending no settings at all would
// reach Jamf as the one refusal it does attribute, which would blame the operator.
func TestBuildConnectionRequest_RefusesAMissingSettingsBlock(t *testing.T) {
	plan := oidcPlanModel()
	plan.GenericOIDC = nil

	if _, diags := buildConnectionRequest(context.Background(), plan, types.StringNull()); !diags.HasError() {
		t.Fatal("a missing settings block must be reported rather than sent as nothing")
	}
}

// TestAliasLoginHintIsAlwaysSerialised pins the reason the provider cannot hit the
// undocumented requirement that sank a day of raw-curl probing: the SDK types
// aliasLoginHintToIdp as a value bool with no omitempty, so it is on the wire even
// when the operator sets nothing.
func TestAliasLoginHintIsAlwaysSerialised(t *testing.T) {
	body, err := json.Marshal(&account.ConnectionRequest{
		ConnectionType:  account.ConnectionTypeOidc,
		Connection:      account.ConnectionRequestConnection{OidcConnectionSettings: &account.OidcConnectionSettings{Name: "probeName"}},
		Domains:         []string{"probe.example"},
		EnabledProducts: []account.EnabledProduct{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"aliasLoginHintToIdp"`) {
		t.Fatalf("aliasLoginHintToIdp absent from a zero-valued request; the provider could then hit the 500:\n%s", body)
	}
}
