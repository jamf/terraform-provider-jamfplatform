// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// oidcConnectionRead is a generic OpenID Connect connection as Jamf returns it.
func oidcConnectionRead() *account.Connection {
	return &account.Connection{
		ID:                               unitConnectionID,
		Name:                             unitConnectionName,
		Type:                             new(account.ConnectionTypeOidc),
		Region:                           new(account.RegionUs),
		ClientID:                         new("probe-client-id"),
		Domains:                          []string{"tf-unit.example"},
		Scopes:                           new("openid email profile"),
		PkceAuthType:                     new(account.PkceAuthTypeDisabled),
		TokenEndpointAuthMethod:          new(account.TokenEndpointAuthMethodClientSecretPost),
		SendNonce:                        false,
		SyncUserProfileAttributesAtLogin: true,
		AliasLoginHintToIdp:              true,
		AttributeMap:                     new(`{"mapping_mode":"bind_all"}`),
		SessionInfo:                      &account.SessionInfo{},
		OidcOptions: &account.OidcOptions{
			IssuerURL:             new("idp.example"),
			AuthorizationEndpoint: new("idp.example/authorize"),
			TokenEndpoint:         new("idp.example/token"),
			JwksUri:               new("idp.example/keys"),
		},
	}
}

// oidcSummaryRead is the collection entry for the same connection.
func oidcSummaryRead() *account.ConnectionSummary {
	return &account.ConnectionSummary{
		ID:                  unitConnectionID,
		Name:                unitConnectionName,
		Type:                new(account.ConnectionTypeOidc),
		Region:              new(account.RegionUs),
		Domains:             []string{"tf-unit.example"},
		EnabledApplications: []string{account.ProductAccount, account.ProductPro},
	}
}

// TestAssignConnectionResourceModel_TranslatesTheRenamedVocabularies pins the
// read side of every rename. A rename applied on the way out and not undone on
// the way in reads back as Jamf's spelling and gives a difference on every plan.
func TestAssignConnectionResourceModel_TranslatesTheRenamedVocabularies(t *testing.T) {
	var state ConnectionResourceModel
	if diags := assignConnectionResourceModel(&state, oidcConnectionRead(), oidcSummaryRead(), true); diags.HasError() {
		t.Fatalf("assigning state: %v", diags)
	}

	if got := state.ConnectionType.ValueString(); got != connectionTypeGenericOIDC {
		t.Errorf("connection_type = %q, want the console's own name", got)
	}
	if got := state.AuthMethod.ValueString(); got != authMethodClientSecret {
		t.Errorf("auth_method = %q, want the console's own name", got)
	}
	if got := state.PKCE.ValueString(); got != pkceDisabled {
		t.Errorf("pkce = %q, want the console's own name", got)
	}
	if got := state.HostingRegion.ValueString(); got != account.RegionUs {
		t.Errorf("hosting_region = %q, want Jamf's own value kept", got)
	}
}

// TestAssignConnectionResourceModel_InvertsTheLoginHint is the other half of the
// pair that matters most in this package. Together with the input-builder test
// this rules out the failure neither test alone could: a pair that inverts twice
// looks exactly like one that never inverts.
func TestAssignConnectionResourceModel_InvertsTheLoginHint(t *testing.T) {
	for name, tc := range map[string]struct {
		aliasToIdp bool
		wantOmit   bool
	}{
		"forwarded": {true, false},
		"withheld":  {false, true},
	} {
		t.Run(name, func(t *testing.T) {
			read := oidcConnectionRead()
			read.AliasLoginHintToIdp = tc.aliasToIdp

			var state ConnectionResourceModel
			if diags := assignConnectionResourceModel(&state, read, oidcSummaryRead(), true); diags.HasError() {
				t.Fatalf("assigning state: %v", diags)
			}
			if got := state.OmitLoginHint.ValueBool(); got != tc.wantOmit {
				t.Errorf("omit_login_hint = %v, want %v — the two senses are opposites", got, tc.wantOmit)
			}
		})
	}
}

// TestOmitLoginHintRoundTrips is the direct statement of the same thing, and the
// one a reader can check at a glance: whatever goes in has to come back.
func TestOmitLoginHintRoundTrips(t *testing.T) {
	for _, omit := range []bool{true, false} {
		wire := omitLoginHintToWire(types.BoolValue(omit))
		if got := omitLoginHintFromWire(wire).ValueBool(); got != omit {
			t.Errorf("omit_login_hint %v round-tripped to %v through %v", omit, got, wire)
		}
		if wire == omit {
			t.Errorf("omit_login_hint %v was sent as %v — the two senses must be opposites", omit, wire)
		}
	}
}

// TestAssignConnectionResourceModel_KeepsTheConfiguredName pins the reason `name`
// and `internal_name` are two attributes. Eighteen of the twenty-two connections
// read carry a suffix Jamf added, and overwriting the configured value with the
// stored one would give every such connection a difference on every plan.
func TestAssignConnectionResourceModel_KeepsTheConfiguredName(t *testing.T) {
	read := oidcConnectionRead()
	read.Name = unitConnectionName + "-uniquified"

	state := ConnectionResourceModel{Name: types.StringValue(unitConnectionName)}
	if diags := assignConnectionResourceModel(&state, read, oidcSummaryRead(), false); diags.HasError() {
		t.Fatalf("assigning state: %v", diags)
	}

	if got := state.Name.ValueString(); got != unitConnectionName {
		t.Errorf("name = %q, want the configured value untouched", got)
	}
	if got := state.InternalName.ValueString(); got != unitConnectionName+"-uniquified" {
		t.Errorf("internal_name = %q, want the stored value", got)
	}
}

// TestAssignConnectionResourceModel_AdoptionRecoversTheConfiguredName covers the
// one case where `name` comes from Jamf at all: an import has no configured value
// to protect. What is adopted is the configured form recovered from the stored
// name, never the stored name itself — `name` is validated against
// nameAllowedPattern and forces a replacement when it differs from state, so
// adopting the stored value would put a name in state that no configuration can
// hold and let the first apply after an import destroy a live connection.
// `internal_name` is what keeps the stored value whole.
//
// The names here are alphanumeric, unlike the package's own fixture, because that
// is the only shape Jamf accepts on a create — and the whole reason the split at
// the first hyphen is unambiguous.
func TestAssignConnectionResourceModel_AdoptionRecoversTheConfiguredName(t *testing.T) {
	for name, tc := range map[string]struct {
		stored string
		want   string
	}{
		"a suffix Jamf appended":    {"tfUnitOidc-jqxld7tl4m454ed7s35647nmjssypo", "tfUnitOidc"},
		"no suffix at all":          {"tfUnitOidc", "tfUnitOidc"},
		"a suffix carrying hyphens": {"tfUnitOidc-jqxld7-tl4m454-ed7s35647nmjssypo", "tfUnitOidc"},
	} {
		t.Run(name, func(t *testing.T) {
			read := oidcConnectionRead()
			read.Name = tc.stored

			var state ConnectionResourceModel
			if diags := assignConnectionResourceModel(&state, read, oidcSummaryRead(), true); diags.HasError() {
				t.Fatalf("assigning state: %v", diags)
			}

			if got := state.Name.ValueString(); got != tc.want {
				t.Errorf("name = %q, want the configured form %q — the stored value is a name no configuration can hold", got, tc.want)
			}
			if got := state.InternalName.ValueString(); got != tc.stored {
				t.Errorf("internal_name = %q, want the stored value whole", got)
			}
		})
	}
}

// TestAssignConnectionResourceModel_GatesBlocksOnTheTargetModel pins
// STYLE_GUIDE §`SingleNestedAttribute` blocks: a block absent from the model
// being written stays absent, however much Jamf returned for it. Populating one
// the plan said was empty breaks the framework's consistency contract, and it
// fails only under a real apply.
func TestAssignConnectionResourceModel_GatesBlocksOnTheTargetModel(t *testing.T) {
	read := oidcConnectionRead()
	read.GroupNameFilter = new(`{"op":"OR","groups":"jamf"}`)

	var state ConnectionResourceModel
	if diags := assignConnectionResourceModel(&state, read, oidcSummaryRead(), false); diags.HasError() {
		t.Fatalf("assigning state: %v", diags)
	}

	if state.GenericOIDC != nil {
		t.Error("a settings block the model did not carry must stay absent on a refresh")
	}
	if state.GroupNameFilter != nil {
		t.Error("a group filter the model did not carry must stay absent on a refresh")
	}

	state = ConnectionResourceModel{
		Name:            types.StringValue(unitConnectionName),
		GenericOIDC:     &GenericOIDCSettingsModel{},
		GroupNameFilter: &GroupNameFilterModel{},
	}
	if diags := assignConnectionResourceModel(&state, read, oidcSummaryRead(), false); diags.HasError() {
		t.Fatalf("assigning state: %v", diags)
	}
	if state.GenericOIDC == nil || state.GenericOIDC.IssuerURL.ValueString() != "idp.example" {
		t.Errorf("generic_oidc = %+v, want a block the model carried refreshed", state.GenericOIDC)
	}
	if state.GroupNameFilter == nil || len(state.GroupNameFilter.Groups.Elements()) != 1 {
		t.Errorf("group_name_filter = %+v, want a filter the model carried refreshed", state.GroupNameFilter)
	}
}

// TestAssignConnectionResourceModel_LeavesTheProductAssignmentsAlone pins the
// configuration-authoritative collections. Nothing Jamf returns echoes the
// tenants, so adopting anything would mean inventing it — and overwriting a
// configured value with a guess is worse than leaving it.
func TestAssignConnectionResourceModel_LeavesTheProductAssignmentsAlone(t *testing.T) {
	state := ConnectionResourceModel{
		Name: types.StringValue(unitConnectionName),
		EnabledProducts: []EnabledProductModel{{
			Product: types.StringValue(account.ProductPro),
			Tenants: mustStringSet("tenant-one"),
		}},
		EnabledEnvironments: []EnabledEnvironmentModel{{
			Product:      types.StringValue(account.ProductProtect),
			Environments: mustStringSet("environment-one"),
		}},
	}

	if diags := assignConnectionResourceModel(&state, oidcConnectionRead(), oidcSummaryRead(), false); diags.HasError() {
		t.Fatalf("assigning state: %v", diags)
	}

	if len(state.EnabledProducts) != 1 || state.EnabledProducts[0].Product.ValueString() != account.ProductPro {
		t.Errorf("enabled_products = %+v, want the configured value untouched", state.EnabledProducts)
	}
	if len(state.EnabledEnvironments) != 1 {
		t.Errorf("enabled_environments = %+v, want the configured value untouched", state.EnabledEnvironments)
	}
	if len(state.EnabledProductNames.Elements()) != 2 {
		t.Errorf("enabled_product_names = %s, want the products the collection reports", state.EnabledProductNames)
	}
}

// TestAssignConnectionResourceModel_WithoutACollectionEntry pins the partial
// case. A connection readable on its own identifier but missing from the
// collection leaves the two attributes only the collection supplies empty rather
// than wrong, which is better than failing the refresh over the lesser half.
func TestAssignConnectionResourceModel_WithoutACollectionEntry(t *testing.T) {
	var state ConnectionResourceModel
	if diags := assignConnectionResourceModel(&state, oidcConnectionRead(), nil, true); diags.HasError() {
		t.Fatalf("assigning state: %v", diags)
	}

	if state.EnabledProductNames.IsNull() || len(state.EnabledProductNames.Elements()) != 0 {
		t.Errorf("enabled_product_names = %s, want a known empty set", state.EnabledProductNames)
	}
	if !state.TicketURL.IsNull() {
		t.Errorf("ticket_url = %s, want nothing", state.TicketURL)
	}
	if state.ID.ValueString() != unitConnectionID {
		t.Error("the rest of the connection must still be adopted")
	}
}

// TestCollectionDerivedValues_FallsBackToTheGoogleSettings pins the consent
// ticket's second source. It appears on the collection entry for every family and
// again inside a Google connection's own settings, so a Google connection can
// supply it even when the collection entry is unavailable.
func TestCollectionDerivedValues_FallsBackToTheGoogleSettings(t *testing.T) {
	_, ticket := collectionDerivedValues(nil, &account.GoogleOptions{
		TicketURL: new("consent/request/one"),
	})
	if ticket.ValueString() != "consent/request/one" {
		t.Errorf("ticket_url = %s, want the value from the Google settings", ticket)
	}

	summary := oidcSummaryRead()
	summary.TicketURL = new("consent/request/two")
	_, ticket = collectionDerivedValues(summary, &account.GoogleOptions{
		TicketURL: new("consent/request/one"),
	})
	if ticket.ValueString() != "consent/request/two" {
		t.Errorf("ticket_url = %s, want the collection entry to win", ticket)
	}
}

// TestParseGroupNameFilter pins the read side of the filter document, including
// the two things it must never do: collapse an operator with no groups into
// nothing, and read a document it does not understand back as no filter — which
// would let the next apply clear a live filter.
func TestParseGroupNameFilter(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		for _, raw := range []*string{nil, new(""), new("   ")} {
			filter, diags := parseGroupNameFilter(raw)
			if diags.HasError() {
				t.Fatalf("parsing %v: %v", raw, diags)
			}
			if filter != nil {
				t.Errorf("parsed %v into %+v, want nothing", raw, filter)
			}
		}
	})

	t.Run("an operator with no groups", func(t *testing.T) {
		filter, diags := parseGroupNameFilter(new(`{"op":"OR","groups":""}`))
		if diags.HasError() {
			t.Fatalf("parsing: %v", diags)
		}
		if filter == nil {
			t.Fatal("an operator with no groups is a real filter and must not collapse to nothing")
		}
		if got := filter.Operator.ValueString(); got != filterOperatorOr {
			t.Errorf("operator = %q, want the renamed value", got)
		}
		if filter.Groups.IsNull() || len(filter.Groups.Elements()) != 0 {
			t.Errorf("groups = %s, want a known empty set", filter.Groups)
		}
	})

	t.Run("groups split and trimmed", func(t *testing.T) {
		filter, diags := parseGroupNameFilter(new(`{"op":"AND","groups":"apple, mango ,zebra"}`))
		if diags.HasError() {
			t.Fatalf("parsing: %v", diags)
		}
		if got := filter.Operator.ValueString(); got != filterOperatorAnd {
			t.Errorf("operator = %q", got)
		}
		if len(filter.Groups.Elements()) != 3 {
			t.Errorf("groups = %s, want three members", filter.Groups)
		}
	})

	t.Run("unparseable is reported", func(t *testing.T) {
		_, diags := parseGroupNameFilter(new("not json"))
		if !diags.HasError() {
			t.Fatal("a filter this provider cannot read must be reported, not read back as no filter")
		}
	})

	t.Run("an unknown operator is reported", func(t *testing.T) {
		_, diags := parseGroupNameFilter(new(`{"op":"XOR","groups":"jamf"}`))
		if !diags.HasError() {
			t.Fatal("an operator this provider does not recognise must be reported")
		}
	})
}

// TestPreferEquivalentJSON pins the claim-mapping reconcile. Jamf re-serialises
// the document and `jsonencode` sorts keys, so the two forms differ constantly
// while meaning the same — and a difference that is only formatting must not
// churn state.
func TestPreferEquivalentJSON(t *testing.T) {
	planned := types.StringValue(`{"mapping_mode":"use_map","userinfo_scope":"profile"}`)
	reordered := `{"userinfo_scope":"profile","mapping_mode":"use_map"}`

	if got := preferEquivalentJSON(planned, new(reordered)); got.ValueString() != planned.ValueString() {
		t.Errorf("preferEquivalentJSON = %s, want the planned bytes kept", got)
	}

	changed := `{"mapping_mode":"bind_all"}`
	if got := preferEquivalentJSON(planned, new(changed)); got.ValueString() != changed {
		t.Errorf("preferEquivalentJSON = %s, want a genuine change adopted", got)
	}

	if got := preferEquivalentJSON(types.StringNull(), new(changed)); got.ValueString() != changed {
		t.Errorf("preferEquivalentJSON = %s, want Jamf's value where nothing was planned", got)
	}
	if got := preferEquivalentJSON(planned, nil); !got.IsNull() {
		t.Errorf("preferEquivalentJSON = %s, want nothing where Jamf holds nothing", got)
	}
}

// TestPreferEquivalentScopes pins the scope reconcile, which exists because the
// order of OAuth scopes carries no meaning and Jamf's copy of them was never seen
// written back. A re-ordering costs nothing to absorb; a genuine change is
// adopted and shows up as one.
func TestPreferEquivalentScopes(t *testing.T) {
	planned := types.StringValue("openid email profile")

	if got := preferEquivalentScopes(planned, new("profile openid email")); got.ValueString() != planned.ValueString() {
		t.Errorf("preferEquivalentScopes = %s, want the planned value kept", got)
	}
	if got := preferEquivalentScopes(planned, new("openid email profile groups")); got.ValueString() != "openid email profile groups" {
		t.Errorf("preferEquivalentScopes = %s, want an added scope adopted", got)
	}
	if got := preferEquivalentScopes(planned, new("openid email")); got.ValueString() != "openid email" {
		t.Errorf("preferEquivalentScopes = %s, want a removed scope adopted", got)
	}
	if got := preferEquivalentScopes(planned, nil); !got.IsNull() {
		t.Errorf("preferEquivalentScopes = %s, want nothing where Jamf holds nothing", got)
	}
}

// TestBuildEntraStateModel_FlattensTheNestedGroupOptions pins the one place the
// read and write shapes disagree about nesting: three of the Entra options sit a
// level deeper in Jamf's copy than in the settings it accepts.
func TestBuildEntraStateModel_FlattensTheNestedGroupOptions(t *testing.T) {
	block := buildEntraStateModel(&account.EntraOptions{
		Domain:       new("contoso.example"),
		TenantDomain: new("contoso.example"),
		BasicProfile: new(true),
		MaxGroups:    new(250),
		IdentityApi:  new(account.EntraIdentityApiMicrosoftIdentityPlatformV2),
		ExtOptions: &account.EntraExtendedOptions{
			ExtendedProfile: new(true),
			Groups:          new(true),
			NestedGroups:    new(false),
		},
	})

	if block == nil {
		t.Fatal("the Entra block must be built from Jamf's own settings")
	}
	if !block.ExtendedProfile.ValueBool() {
		t.Error("extended_profile must come from the nested options")
	}
	if !block.GetUserGroups.ValueBool() {
		t.Error("get_user_groups must come from the nested options")
	}
	if block.IncludeNestedGroups.IsNull() || block.IncludeNestedGroups.ValueBool() {
		t.Errorf("include_nested_groups = %s, want the nested value", block.IncludeNestedGroups)
	}
	if !block.BasicProfile.ValueBool() {
		t.Error("basic_profile must be reported, since it is always on and never a choice")
	}
}

// TestBuildEntraStateModel_WithoutTheNestedOptions pins the absent-inner-object
// case, which leaves the three deeper options empty rather than guessing at
// false.
func TestBuildEntraStateModel_WithoutTheNestedOptions(t *testing.T) {
	block := buildEntraStateModel(&account.EntraOptions{Domain: new("contoso.example")})
	if block == nil {
		t.Fatal("the Entra block must be built")
	}
	for name, value := range map[string]types.Bool{
		"extended_profile":      block.ExtendedProfile,
		"get_user_groups":       block.GetUserGroups,
		"include_nested_groups": block.IncludeNestedGroups,
	} {
		if !value.IsNull() {
			t.Errorf("%s = %s, want nothing where Jamf sent no nested options", name, value)
		}
	}
}

// TestBuildGoogleStateModel_MapsTheGroupSwitches pins the Google read mapping,
// which is a trap: Jamf's copy calls the group switch extGroups and the directory
// read extGroupsExtended, so the shorter name is not the simpler option.
func TestBuildGoogleStateModel_MapsTheGroupSwitches(t *testing.T) {
	block := buildGoogleStateModel(&account.GoogleOptions{
		Domain:            new("tf-unit.example"),
		ExtGroups:         new(true),
		ExtGroupsExtended: new(false),
		EnableUsersApi:    new(true),
	})

	if block == nil {
		t.Fatal("the Google block must be built from Jamf's own settings")
	}
	if !block.GetUserGroups.ValueBool() {
		t.Error("get_user_groups must come from the group switch")
	}
	if block.ExtendedGroups.ValueBool() {
		t.Error("extended_groups must come from the directory read, not from the group switch")
	}
	if !block.EnableUsersAPI.ValueBool() {
		t.Error("enable_users_api must be mapped")
	}
}

// TestBuildStateModels_WithoutSettings pins that a block Jamf did not return is
// left absent rather than built empty, which is what keeps only the family's own
// block populated.
func TestBuildStateModels_WithoutSettings(t *testing.T) {
	if buildOIDCStateModel(nil) != nil {
		t.Error("the generic block must be absent when Jamf returned none")
	}
	if buildEntraStateModel(nil) != nil {
		t.Error("the Entra block must be absent when Jamf returned none")
	}
	if buildOktaStateModel(nil) != nil {
		t.Error("the Okta block must be absent when Jamf returned none")
	}
	if buildGoogleStateModel(nil) != nil {
		t.Error("the Google block must be absent when Jamf returned none")
	}
}

// TestSessionMinutes pins the two session limits, including the absent-object
// case: Jamf reports the object with explicit absences, and no object at all is
// still no limits rather than zero.
func TestSessionMinutes(t *testing.T) {
	duration, inactivity := sessionMinutes(nil)
	if !duration.IsNull() || !inactivity.IsNull() {
		t.Errorf("no session object gave %s/%s, want nothing", duration, inactivity)
	}

	duration, inactivity = sessionMinutes(&account.SessionInfo{})
	if !duration.IsNull() || !inactivity.IsNull() {
		t.Errorf("explicit absences gave %s/%s, want nothing", duration, inactivity)
	}

	duration, inactivity = sessionMinutes(&account.SessionInfo{
		MaxSessionTimeInMinutes:    new(480),
		MaxInactivityTimeInMinutes: new(30),
	})
	if duration.ValueInt64() != 480 || inactivity.ValueInt64() != 30 {
		t.Errorf("session limits gave %s/%s, want 480/30", duration, inactivity)
	}
}

// TestRenameFromWire_CarriesThroughAnUnknownValue pins the read-side fallback. A
// value Jamf adds before this provider names it is reported as Jamf spelled it
// rather than read back as nothing, which would look like the connection had no
// such setting.
func TestRenameFromWire_CarriesThroughAnUnknownValue(t *testing.T) {
	if got := renameFromWire(new("SOMETHING_NEW"), connectionTypeFromWire); got.ValueString() != "SOMETHING_NEW" {
		t.Errorf("renameFromWire = %s, want Jamf's own spelling carried through", got)
	}
	if got := renameFromWire(nil, connectionTypeFromWire); !got.IsNull() {
		t.Errorf("renameFromWire = %s, want nothing", got)
	}
	if got := renameFromWire(new(""), connectionTypeFromWire); !got.IsNull() {
		t.Errorf("renameFromWire = %s, want nothing for an empty value", got)
	}
}

// TestBuildConnectionsResultModel pins the plural result, whose shape is Jamf's
// rather than this provider's: the collection read returns no per-provider
// settings, so the subset is what can be reported without an extra read per
// connection.
func TestBuildConnectionsResultModel(t *testing.T) {
	result, diags := buildConnectionsResultModel(*oidcSummaryRead())
	if diags.HasError() {
		t.Fatalf("building the result: %v", diags)
	}

	if result.ID.ValueString() != unitConnectionID {
		t.Errorf("id = %q", result.ID.ValueString())
	}
	if result.ConnectionType.ValueString() != connectionTypeGenericOIDC {
		t.Errorf("connection_type = %q, want the renamed value", result.ConnectionType.ValueString())
	}
	if len(result.Domains.Elements()) != 1 {
		t.Errorf("domains = %s, want the one domain", result.Domains)
	}
	if len(result.EnabledProductNames.Elements()) != 2 {
		t.Errorf("enabled_product_names = %s, want both products", result.EnabledProductNames)
	}
}

// TestAssignConnectionResourceModel_PKCEFollowsTheConnectionType pins the one
// attribute Jamf reports for every connection and this resource accepts for only
// half of them.
//
// The console offers a PKCE setting for a generic OpenID Connect or an Okta
// connection and for neither of the other two, which validators.go enforces by
// refusing a configured value there. Recording Jamf's value anyway made the
// resource contradict itself through `terraform plan -generate-config-out`, which
// writes state back out as configuration: an Entra connection came back with
// `pkce = "disabled"` written in and the generated file was refused at plan time
// (issue #379).
//
// All four families are covered on a refresh and on an adoption, since an import
// is the path a practitioner reaches this by and the two share nothing but this
// function.
func TestAssignConnectionResourceModel_PKCEFollowsTheConnectionType(t *testing.T) {
	for _, tc := range []struct {
		wireType string
		wantPKCE bool
	}{
		{account.ConnectionTypeOidc, true},
		{account.ConnectionTypeOkta, true},
		{account.ConnectionTypeWaad, false},
		{account.ConnectionTypeGoogleApps, false},
	} {
		for form, adopt := range map[string]bool{"refresh": false, "adoption": true} {
			t.Run(tc.wireType+"/"+form, func(t *testing.T) {
				read := oidcConnectionRead()
				read.Type = new(tc.wireType)
				read.PkceAuthType = new(account.PkceAuthTypeDisabled)

				var state ConnectionResourceModel
				if diags := assignConnectionResourceModel(&state, read, oidcSummaryRead(), adopt); diags.HasError() {
					t.Fatalf("assigning state: %v", diags)
				}

				if !tc.wantPKCE {
					if !state.PKCE.IsNull() {
						t.Errorf("pkce = %q, want nothing: the console offers no PKCE setting for this connection type, and this resource refuses a configured one",
							state.PKCE.ValueString())
					}
					return
				}
				if got := state.PKCE.ValueString(); got != pkceDisabled {
					t.Errorf("pkce = %q, want the stored value %q", got, pkceDisabled)
				}
			})
		}
	}
}

// TestAssignConnectionDataSourceModel_ReportsEveryGatedAttribute is the other half
// of the asymmetry, for both attributes the resource gates. A data source owns no
// configuration to contradict, so it reports whatever Jamf holds — including
// `pkce` for the two families a managed connection records none for, and
// `auth_method` for Google Workspace, which Jamf returns as CLIENT_SECRET_POST
// like every other family (probed 2026-09-05).
func TestAssignConnectionDataSourceModel_ReportsEveryGatedAttribute(t *testing.T) {
	for _, wireType := range []string{
		account.ConnectionTypeOidc,
		account.ConnectionTypeOkta,
		account.ConnectionTypeWaad,
		account.ConnectionTypeGoogleApps,
	} {
		t.Run(wireType, func(t *testing.T) {
			read := oidcConnectionRead()
			read.Type = new(wireType)

			var state ConnectionDataSourceModel
			if diags := assignConnectionDataSourceModel(&state, read, oidcSummaryRead()); diags.HasError() {
				t.Fatalf("assigning state: %v", diags)
			}
			if got := state.PKCE.ValueString(); got != pkceDisabled {
				t.Errorf("pkce = %q, want the stored value %q", got, pkceDisabled)
			}
			if state.AuthMethod.IsNull() {
				t.Error("auth_method must be reported for every connection type, including the Google " +
					"Workspace one the resource records nothing for")
			}
		})
	}
}

// TestBuildEntraStateModel_GroupsOffDropsTheGroupOptions covers rule 14's read
// side. validateEntraGroupOptions refuses entra.groups_scope and a true
// entra.include_nested_groups when get_user_groups is off, so adopting what Jamf
// returned there would commit values this resource's own validator rejects, and
// `-generate-config-out` would write a configuration that cannot plan.
//
// This is the read half of the rule. Whether Jamf actually returns a
// groups_scope alongside groups-off is not settleable from the mockingbirduat
// estate: a connection needs a verified domain, and every verified domain there
// already carries one. The provider's job is to not commit the combination
// whatever Jamf sends, which is what this pins.
func TestBuildEntraStateModel_GroupsOffDropsTheGroupOptions(t *testing.T) {
	off, on, nested := false, true, true
	block := buildEntraStateModel(&account.EntraOptions{
		Domain:      new("contoso.example"),
		GroupsScope: new("DIRECTORY_READ_ALL"),
		ExtOptions: &account.EntraExtendedOptions{
			Groups:       &off,
			NestedGroups: &nested,
		},
	})
	if block == nil {
		t.Fatal("the Entra block must be built")
	}
	if !block.GroupsScope.IsNull() {
		t.Errorf("groups_scope = %s, want nothing when group membership is off", block.GroupsScope)
	}
	// false rather than null: the attribute is Optional+Computed with
	// UseNonNullStateForUnknown, where a null would fight the plan modifier, and
	// false is both what the validator accepts and what Jamf does with nested
	// groups when membership is off.
	if block.IncludeNestedGroups.IsNull() || block.IncludeNestedGroups.ValueBool() {
		t.Errorf("include_nested_groups = %s, want false when group membership is off", block.IncludeNestedGroups)
	}

	// Positive control: with membership on, both are adopted as sent.
	block = buildEntraStateModel(&account.EntraOptions{
		Domain:      new("contoso.example"),
		GroupsScope: new("GROUP_READ_ALL"),
		ExtOptions: &account.EntraExtendedOptions{
			Groups:       &on,
			NestedGroups: &nested,
		},
	})
	if got := block.GroupsScope.ValueString(); got != "GROUP_READ_ALL" {
		t.Errorf("groups_scope = %q, want it adopted when membership is on", got)
	}
	if !block.IncludeNestedGroups.ValueBool() {
		t.Error("include_nested_groups must be adopted when membership is on")
	}
}

// TestBuildEntraStateModel_UnreportedGroupsStillDropsTheScope separates the two
// groups-off shapes. Jamf sending no group settings at all leaves
// include_nested_groups empty rather than guessing at false, but groups_scope is
// still dropped: the validator reads an absent get_user_groups as off and would
// refuse a scope sitting beside it.
func TestBuildEntraStateModel_UnreportedGroupsStillDropsTheScope(t *testing.T) {
	block := buildEntraStateModel(&account.EntraOptions{
		Domain:      new("contoso.example"),
		GroupsScope: new("DIRECTORY_READ_ALL"),
	})
	if block == nil {
		t.Fatal("the Entra block must be built")
	}
	if !block.GroupsScope.IsNull() {
		t.Errorf("groups_scope = %s, want nothing where Jamf reported no group settings", block.GroupsScope)
	}
	if !block.IncludeNestedGroups.IsNull() {
		t.Errorf("include_nested_groups = %s, want nothing where Jamf sent no nested options", block.IncludeNestedGroups)
	}
}
