// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// This file uses Go 1.26's extended `new(v)` builtin (see directory_binding
// state_builders_test.go for the explanation).

package ldap_server

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func fullLdapServerResponse() *proclassic.LdapServer {
	return &proclassic.LdapServer{
		Connection: &proclassic.LdapServerConnection{
			ID:                 new(31),
			IsEnabled:          new(true),
			MigratedToID:       new(0),
			CertificatesUsed:   new(""),
			Name:               new("corp-ad"),
			Hostname:           new("ldap.example.com"),
			ServerType:         new(serverTypeActiveDirectory),
			Port:               new(636),
			UseSsl:             new(true),
			AuthenticationType: new(authTypeSimple),
			OpenCloseTimeout:   new(15),
			SearchTimeout:      new(60),
			ReferralResponse:   new(""),
			UseWildcards:       new(true),
			Account: &proclassic.LdapServerConnectionAccount{
				DistinguishedUsername: new("CN=svc,DC=example,DC=com"),
				PasswordSha256:        new("********************"),
			},
		},
		MappingsForUsers: &proclassic.LdapServerMappingsForUsers{
			UserMappings: &proclassic.LdapServerMappingsForUsersUserMappings{
				MapObjectClassToAnyOrAll: new(objectClassAny),
				ObjectClasses:            new("organizationalPerson"),
				SearchBase:               new("OU=Users,DC=example,DC=com"),
				SearchScope:              new(searchScopeAllSubtrees),
				MapUsername:              new("mail"),
				MapPhone:                 new("telephoneNumber"),
				MapUserUUID:              new("objectGUID"),
			},
			UserGroupMappings: &proclassic.LdapServerMappingsForUsersUserGroupMappings{
				MapGroupName: new("sAMAccountName"),
			},
			UserGroupMembershipMappings: &proclassic.LdapServerMappingsForUsersUserGroupMembershipMappings{
				UserGroupMembershipStoredIn:                      new(membershipGroupObject),
				MapUserMembershipToGroupField:                    new("member"),
				MapGroupMembershipToUserField:                    new("memberOf"),
				UseDn:                                            new(true),
				MembershipScopingOptimization:                    new(true),
				GroupMembershipEnabledWhenUserMembershipSelected: new(false),
			},
		},
	}
}

func TestAssignLdapServerResourceModel_RoundTrip(t *testing.T) {
	// Existing state carries a managed account with a rotation version that
	// must survive the refresh, plus a declared mappings shape (all three
	// sub-blocks) so the gating in assignLdapServerResourceModel populates
	// them from the server echo.
	state := LdapServerResourceModel{
		Connection: &ldapConnectionModel{
			Account: &ldapAccountModel{
				DistinguishedUsername: types.StringValue("CN=svc,DC=example,DC=com"),
				PasswordWoVersion:     types.Int64Value(2),
			},
		},
		MappingsForUsers: &ldapMappingsModel{
			UserMappings:                &ldapUserMappingsModel{},
			UserGroupMappings:           &ldapUserGroupMappingsModel{},
			UserGroupMembershipMappings: &ldapMembershipMappingsModel{},
		},
	}

	diags := assignLdapServerResourceModel(&state, fullLdapServerResponse(), true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// ID sourced from connection.id (nested), not the top-level <id>.
	if state.ID.ValueString() != "31" {
		t.Errorf("ID=%q, want 31", state.ID.ValueString())
	}
	if state.Connection.DisplayName.ValueString() != "corp-ad" {
		t.Errorf("display_name=%q", state.Connection.DisplayName.ValueString())
	}
	if state.Connection.Port.ValueInt64() != 636 {
		t.Errorf("port=%d", state.Connection.Port.ValueInt64())
	}
	if !state.Connection.IsEnabled.ValueBool() {
		t.Errorf("is_enabled must be true")
	}
	// Account dn echoed; password_wo_version preserved; password not set.
	if state.Connection.Account == nil {
		t.Fatal("expected account")
	}
	if state.Connection.Account.DistinguishedUsername.ValueString() != "CN=svc,DC=example,DC=com" {
		t.Errorf("account dn=%q", state.Connection.Account.DistinguishedUsername.ValueString())
	}
	if state.Connection.Account.PasswordWoVersion.ValueInt64() != 2 {
		t.Errorf("password_wo_version must be preserved (2), got %d", state.Connection.Account.PasswordWoVersion.ValueInt64())
	}
	if !state.Connection.Account.Password.IsNull() {
		t.Errorf("password must never be populated from read")
	}
	// Mappings: map_phone + membership *string field round-trip.
	if state.MappingsForUsers.UserMappings.Phone.ValueString() != "telephoneNumber" {
		t.Errorf("phone=%q", state.MappingsForUsers.UserMappings.Phone.ValueString())
	}
	if state.MappingsForUsers.UserGroupMembershipMappings.MemberUserMapping.ValueString() != "member" {
		t.Errorf("member_user_mapping=%q (want the Group Object member attr from map_user_membership_to_group_field)", state.MappingsForUsers.UserGroupMembershipMappings.MemberUserMapping.ValueString())
	}
	if state.MappingsForUsers.UserGroupMembershipMappings.GroupMembershipMapping.ValueString() != "memberOf" {
		t.Errorf("group_membership_mapping=%q", state.MappingsForUsers.UserGroupMembershipMappings.GroupMembershipMapping.ValueString())
	}
	if state.MappingsForUsers.UserGroupMembershipMappings.UseMemberFieldForSelectQueries.ValueBool() {
		t.Errorf("use_member_field_for_select_queries should be false")
	}
}

func TestAssignLdapServerResourceModel_GatesUndeclaredMappings(t *testing.T) {
	// User declared only user_mappings; the server echoes all three blocks.
	// State must keep only the declared block so the planned-null sibling
	// blocks match the result (no inconsistent-result-after-apply).
	state := LdapServerResourceModel{
		Connection:       &ldapConnectionModel{},
		MappingsForUsers: &ldapMappingsModel{UserMappings: &ldapUserMappingsModel{}},
	}
	diags := assignLdapServerResourceModel(&state, fullLdapServerResponse(), true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.MappingsForUsers == nil || state.MappingsForUsers.UserMappings == nil {
		t.Fatal("declared user_mappings must be populated")
	}
	if state.MappingsForUsers.UserGroupMappings != nil {
		t.Errorf("undeclared user_group_mappings must stay nil, got %+v", state.MappingsForUsers.UserGroupMappings)
	}
	if state.MappingsForUsers.UserGroupMembershipMappings != nil {
		t.Errorf("undeclared user_group_membership_mappings must stay nil")
	}
}

func TestAssignLdapServerResourceModel_NoMappingsDeclaredStaysNil(t *testing.T) {
	// User declared no mappings_for_users at all → the server echo is ignored.
	state := LdapServerResourceModel{Connection: &ldapConnectionModel{}}
	diags := assignLdapServerResourceModel(&state, fullLdapServerResponse(), true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.MappingsForUsers != nil {
		t.Errorf("mappings_for_users must stay nil when undeclared, got %+v", state.MappingsForUsers)
	}
}

func TestAssignLdapServerResourceModel_ImportPopulatesAllMappings(t *testing.T) {
	// Import path (gateMappingsToDeclared=false): no prior config, so every
	// mapping sub-block the server returns is populated for full fidelity.
	state := LdapServerResourceModel{}
	diags := assignLdapServerResourceModel(&state, fullLdapServerResponse(), false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.MappingsForUsers == nil {
		t.Fatal("import must populate mappings_for_users")
	}
	if state.MappingsForUsers.UserMappings == nil ||
		state.MappingsForUsers.UserGroupMappings == nil ||
		state.MappingsForUsers.UserGroupMembershipMappings == nil {
		t.Errorf("import must populate all three mapping sub-blocks (full fidelity)")
	}
}

func TestAssignAccountModel_AnonEmptyDNStaysNil(t *testing.T) {
	// Anonymous server: server echoes an empty account; with no prior account
	// in state, the model must stay nil so planned-null == state-null.
	out := assignAccountModel(&proclassic.LdapServerConnectionAccount{
		DistinguishedUsername: new(""),
		PasswordSha256:        new(""),
	}, nil)
	if out != nil {
		t.Errorf("empty-DN account with no prior state must map to nil, got %+v", out)
	}
}

func TestAssignAccountModel_PreservesWoVersion(t *testing.T) {
	existing := &ldapAccountModel{PasswordWoVersion: types.Int64Value(7)}
	out := assignAccountModel(&proclassic.LdapServerConnectionAccount{
		DistinguishedUsername: new("CN=svc"),
	}, existing)
	if out == nil {
		t.Fatal("expected account")
	}
	if out.PasswordWoVersion.ValueInt64() != 7 {
		t.Errorf("wo_version=%d, want 7 preserved", out.PasswordWoVersion.ValueInt64())
	}
	if out.DistinguishedUsername.ValueString() != "CN=svc" {
		t.Errorf("dn=%q", out.DistinguishedUsername.ValueString())
	}
}

func TestAssignConnectionModel_PreservesExplicitEmptyReferralResponse(t *testing.T) {
	// "" is the meaningful "use default from LDAP service" value, not "unset".
	// Classic echoes an empty <referral_response/> on every read regardless of
	// what was configured, so a bare StringPointerValueOrNull would collapse
	// this to Null and trip "Provider produced inconsistent result after
	// apply" against the planned "" — masked at connection_settings because of
	// the Sensitive account.password sibling (STYLE_GUIDE.md §4a).
	existing := &ldapConnectionModel{ReferralResponse: types.StringValue("")}
	out := assignConnectionModel(&proclassic.LdapServerConnection{
		ReferralResponse: new(""),
	}, existing)
	if out == nil {
		t.Fatal("expected connection model")
	}
	if out.ReferralResponse.IsNull() || out.ReferralResponse.ValueString() != "" {
		t.Errorf("referral_response = %#v, want explicit empty string preserved", out.ReferralResponse)
	}
}

func TestAssignConnectionModel_ReferralResponseWireValueWins(t *testing.T) {
	// A real (non-empty) wire value always wins over whatever was previously
	// known, regardless of what existing state held.
	existing := &ldapConnectionModel{ReferralResponse: types.StringValue("")}
	out := assignConnectionModel(&proclassic.LdapServerConnection{
		ReferralResponse: new(referralFollow),
	}, existing)
	if out == nil {
		t.Fatal("expected connection model")
	}
	if out.ReferralResponse.ValueString() != referralFollow {
		t.Errorf("referral_response=%q, want %q", out.ReferralResponse.ValueString(), referralFollow)
	}
}

func TestAssignConnectionModel_ReferralResponseUnknownExistingStaysNull(t *testing.T) {
	// Create with referral_response omitted from config: the plan modifier has
	// no prior state to fall back to, so the planned value carried into this
	// function is Unknown. An empty wire echo must resolve to Null, never leak
	// the Unknown through — that would fail apply with "unknown value after
	// apply" instead of the inconsistent-value bug being fixed here.
	existing := &ldapConnectionModel{ReferralResponse: types.StringUnknown()}
	out := assignConnectionModel(&proclassic.LdapServerConnection{
		ReferralResponse: new(""),
	}, existing)
	if out == nil {
		t.Fatal("expected connection model")
	}
	if !out.ReferralResponse.IsNull() {
		t.Errorf("referral_response=%#v, want Null (not leaked Unknown) when existing was Unknown", out.ReferralResponse)
	}
}

func TestAssignConnectionModel_ReferralResponseNullOnImport(t *testing.T) {
	// No prior state (import / data source): an empty wire echo has nothing to
	// reconcile against, so it stays Null rather than fabricating "".
	out := assignConnectionModel(&proclassic.LdapServerConnection{
		ReferralResponse: new(""),
	}, nil)
	if out == nil {
		t.Fatal("expected connection model")
	}
	if !out.ReferralResponse.IsNull() {
		t.Errorf("referral_response=%#v, want Null with no prior state", out.ReferralResponse)
	}
}

func TestAssignLdapServerDataSourceModel_SetsTopLevelNameAndID(t *testing.T) {
	var ds LdapServerDataSourceModel
	diags := assignLdapServerDataSourceModel(&ds, fullLdapServerResponse())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if ds.ID.ValueString() != "31" {
		t.Errorf("ds ID=%q", ds.ID.ValueString())
	}
	if ds.Name.ValueString() != "corp-ad" {
		t.Errorf("ds name=%q", ds.Name.ValueString())
	}
	if ds.Connection == nil || ds.Connection.Hostname.ValueString() != "ldap.example.com" {
		t.Errorf("ds connection hostname not populated")
	}
}
