// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// readTestAccountID is the identifier the Read tests below pass through. Jamf Pro
// numbers admin accounts, so this is a small integer rather than a UUID.
const readTestAccountID = "7"

// accountReadResource returns a resource wired to a stub server that answers the
// Pro base-field read as JSON and the ProClassic privilege read as XML, which is
// the split the real Read makes. The seam is the HTTP boundary rather than an
// injected interface: Read holds two concrete SDK clients, and an interface
// introduced only for a test would be a bigger change than the behaviour it pins.
// The stub is local rather than testhelpers.NewMockClient because testhelpers
// reaches internal/provider, which imports this package.
func accountReadResource(t *testing.T, privilegesXML string) *AccountResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case strings.Contains(r.URL.Path, "/accounts/userid/"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(privilegesXML))
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             readTestAccountID,
				"username":       "tf-acc-admin",
				"realname":       "TF Acc Admin",
				"email":          "tf-acc-admin@example.invalid",
				"accessLevel":    pro.UserAccountAccessLevelFullAccess,
				"privilegeLevel": pro.UserAccountPrivilegeLevelCustom,
				"accountStatus":  "Enabled",
				"accountType":    "DEFAULT",
				"ldapServerId":   -1,
				"siteId":         -1,
			})
		}
	}))
	t.Cleanup(server.Close)

	client := jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0))
	return &AccountResource{proClient: pro.New(client), classicClient: proclassic.New(client)}
}

// customPrivilegesXML is a ProClassic account carrying a Custom privilege grid.
// jss_settings holds a privilege the configuration never declared, which is what
// Jamf Pro adds of its own accord when a declared object privilege depends on it.
const customPrivilegesXML = `<account>
  <id>7</id>
  <name>tf-acc-admin</name>
  <privileges>
    <jss_objects>
      <privilege>Read Computers</privilege>
      <privilege>Update Computers</privilege>
    </jss_objects>
    <jss_settings>
      <privilege>Read License Information</privilege>
    </jss_settings>
  </privileges>
</account>`

// readAccountAfterImport drives Read with the state Terraform hands a resource
// after an import: the framework's empty state with the passthrough identifier
// written into it, which is what ImportStatePassthroughID produces. Building the
// state that way rather than asserting on a flag is the point. Read receives a
// populated object on that path, so a null-state check cannot recognise it.
func readAccountAfterImport(t *testing.T, r *AccountResource) AccountResourceModel {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	stub := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	if diags := stub.SetAttribute(ctx, path.Root("id"), readTestAccountID); diags.HasError() {
		t.Fatalf("seeding the imported identifier: %v", diags)
	}
	if stub.Raw.IsNull() {
		t.Fatal("the imported state must be a populated object; a null one would make the old detector work")
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: stub.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{State: stub}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state AccountResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the hydrated state: %v", diags)
	}
	return state
}

// TestRead_ImportHydratesPrivilegeGrid is the regression test for the account
// half of issue #372. `assignClassicPrivileges` has an import arm and a
// prior-value arm and nothing else, so a false import signal against empty prior
// state left `privileges` null for every imported account, however much the
// classic endpoint returned.
func TestRead_ImportHydratesPrivilegeGrid(t *testing.T) {
	state := readAccountAfterImport(t, accountReadResource(t, customPrivilegesXML))

	if state.Privileges == nil {
		t.Fatal("privileges = null; an imported account must carry the grid the classic endpoint returned")
	}

	var objects []string
	if diags := state.Privileges.JamfProServerObjects.ElementsAs(context.Background(), &objects, false); diags.HasError() {
		t.Fatalf("reading the hydrated object privileges: %v", diags)
	}
	if len(objects) != 2 {
		t.Errorf("jamf_pro_server_objects = %v, want the two declared privileges", objects)
	}

	var settings []string
	if diags := state.Privileges.JamfProServerSettings.ElementsAs(context.Background(), &settings, false); diags.HasError() {
		t.Fatalf("reading the hydrated settings privileges: %v", diags)
	}
	if len(settings) != 1 || settings[0] != "Read License Information" {
		t.Errorf("jamf_pro_server_settings = %v, want the server-added privilege", settings)
	}
}

// TestRead_ImportHydrationSurvivesTheBaseFieldWrite pins the property that
// replaced an ordering constraint. assignProBaseFields writes `username` from the
// Pro response, so a signal sampled off the model Read is assembling would answer
// differently before and after that call; Read samples it off the immutable
// request state instead, and there is nothing left for a later assignment to
// change. Asserting the hydrated username proves the base fields did land, so the
// privilege assertion is evidence of that immunity rather than of a stub that
// answered nothing.
func TestRead_ImportHydrationSurvivesTheBaseFieldWrite(t *testing.T) {
	state := readAccountAfterImport(t, accountReadResource(t, customPrivilegesXML))

	if got := state.Username.ValueString(); got != "tf-acc-admin" {
		t.Errorf("username = %q, want the value the Pro read returned", got)
	}
	if state.Privileges == nil {
		t.Error("privileges = null, yet the base fields hydrated: the signal was taken from the model, not the request")
	}
}

// readAccountAfterIdentityImport drives Read the way Terraform does for an
// `import { identity = {...} }` block: the framework writes nothing into state, so
// `req.State.Raw` is genuinely null and the identifier arrives only in
// `req.Identity`. That is the other half of the hydration signal, and it is the
// half readAccountAfterImport cannot reach — the passthrough importer always
// leaves a populated object behind.
func readAccountAfterIdentityImport(t *testing.T, r *AccountResource) AccountResourceModel {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	stub := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	if !stub.Raw.IsNull() {
		t.Fatal("the identity-only state must be genuinely null; a populated one would exercise the other branch")
	}

	identity := tfsdk.ResourceIdentity{
		Schema: identityResp.IdentitySchema,
		Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
	}
	if diags := identity.SetAttribute(ctx, path.Root("id"), readTestAccountID); diags.HasError() {
		t.Fatalf("seeding the identity: %v", diags)
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: stub.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema, Raw: identity.Raw.Copy()},
	}
	r.Read(ctx, resource.ReadRequest{State: stub, Identity: &identity}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state AccountResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the hydrated state: %v", diags)
	}
	return state
}

// TestRead_IdentityOnlyRefreshHydratesPrivilegeGrid covers the identity-based
// import path end to end. `stateAbsent` carries it, and until this test the whole
// branch that rebuilds state out of `req.Identity` ran in no test at all — only as
// a boolean case handed straight to importHydration. An account adopted this way
// must come back with the same privilege grid the passthrough path adopts.
func TestRead_IdentityOnlyRefreshHydratesPrivilegeGrid(t *testing.T) {
	state := readAccountAfterIdentityImport(t, accountReadResource(t, customPrivilegesXML))

	if got := state.ID.ValueString(); got != readTestAccountID {
		t.Errorf("id = %q, want the identifier the identity carried", got)
	}
	if got := state.Username.ValueString(); got != "tf-acc-admin" {
		t.Errorf("username = %q, want the value the Pro read returned", got)
	}
	if state.Privileges == nil {
		t.Fatal("privileges = null; an account adopted by identity must carry the grid the classic endpoint returned")
	}

	var objects []string
	if diags := state.Privileges.JamfProServerObjects.ElementsAs(context.Background(), &objects, false); diags.HasError() {
		t.Fatalf("reading the hydrated object privileges: %v", diags)
	}
	if len(objects) != 2 {
		t.Errorf("jamf_pro_server_objects = %v, want the two declared privileges", objects)
	}

	var settings []string
	if diags := state.Privileges.JamfProServerSettings.ElementsAs(context.Background(), &settings, false); diags.HasError() {
		t.Fatalf("reading the hydrated settings privileges: %v", diags)
	}
	if len(settings) != 1 || settings[0] != "Read License Information" {
		t.Errorf("jamf_pro_server_settings = %v, want the server-added privilege", settings)
	}
}

// TestRead_RefreshLeavesUndeclaredCategoriesNull is the control on the fix. An
// ordinary refresh must keep honouring intersect-on-read, so a category the
// configuration never declared stays null and a server-added privilege in it
// never enters state.
func TestRead_RefreshLeavesUndeclaredCategoriesNull(t *testing.T) {
	ctx := context.Background()
	r := accountReadResource(t, customPrivilegesXML)

	declared, diags := types.SetValueFrom(ctx, types.StringType, []string{"Read Computers", "Update Computers"})
	if diags.HasError() {
		t.Fatalf("building the declared privilege set: %v", diags)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	prior := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	priorModel := AccountResourceModel{
		ID:           types.StringValue(readTestAccountID),
		Username:     types.StringValue("tf-acc-admin"),
		AccessLevel:  types.StringValue("Full Access"),
		PrivilegeSet: types.StringValue("Custom"),
		Timeouts:     helpers.NewResourceTimeoutsNullValue(accountTimeoutAttributeTypes),
	}
	priorModel.Privileges = &accountprivileges.Model{
		JamfProServerObjects:  declared,
		JamfProServerSettings: types.SetNull(types.StringType),
		JamfProServerActions:  types.SetNull(types.StringType),
		CasperAdmin:           types.SetNull(types.StringType),
		CasperRemote:          types.SetNull(types.StringType),
		CasperImaging:         types.SetNull(types.StringType),
		Recon:                 types.SetNull(types.StringType),
	}
	if diags := prior.Set(ctx, priorModel); diags.HasError() {
		t.Fatalf("seeding prior state: %v", diags)
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: prior.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{State: prior}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state AccountResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the refreshed state: %v", diags)
	}
	if state.Privileges == nil {
		t.Fatal("a declared privilege grid must survive a refresh")
	}
	if !state.Privileges.JamfProServerSettings.IsNull() {
		t.Errorf("jamf_pro_server_settings = %s, want null: an undeclared category stays unmanaged", state.Privileges.JamfProServerSettings)
	}
	var objects []string
	if diags := state.Privileges.JamfProServerObjects.ElementsAs(ctx, &objects, false); diags.HasError() {
		t.Fatalf("reading the refreshed object privileges: %v", diags)
	}
	if len(objects) != 2 {
		t.Errorf("jamf_pro_server_objects = %v, want the two declared privileges", objects)
	}
}

func TestImportHydration(t *testing.T) {
	cases := []struct {
		name        string
		stateAbsent bool
		username    types.String
		want        bool
	}{
		{"a post-import read carries the id and nothing else", false, types.StringNull(), true},
		{"an identity-only refresh has no state at all", true, types.StringNull(), true},
		{"an ordinary refresh of a managed account", false, types.StringValue("tf-acc-admin"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := importHydration(tc.stateAbsent, tc.username); got != tc.want {
				t.Errorf("importHydration = %v, want %v", got, tc.want)
			}
		})
	}
}
