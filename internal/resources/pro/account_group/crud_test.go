// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

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
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// readTestGroupID is the identifier the Read tests below hydrate. Jamf Pro
// numbers account groups, so this is a small integer rather than a UUID.
const readTestGroupID = "7"

// administratorGroupXML is the group accountprivileges.Discover sources the
// tenant's grantable catalog from: the privilege set whose grid is the catalog
// closure. It grants two object privileges and nothing else, so "Read Knobs" —
// the privilege an 11.x tenant expands a preset set into but refuses to grant —
// is provably outside the catalog.
const administratorGroupXML = `<group>
  <id>1</id>
  <name>tf-acc-administrators</name>
  <access_level>Full Access</access_level>
  <privilege_set>Administrator</privilege_set>
  <privileges>
    <jss_objects>
      <privilege>Read Computers</privilege>
      <privilege>Update Computers</privilege>
    </jss_objects>
  </privileges>
</group>`

// groupXML renders the group under test with the given privilege set and the
// grid Jamf Pro returns for it. Both callers use the same grid: what separates
// them is the privilege set, which is the whole point of the gate.
func groupXML(privilegeSet string) string {
	return `<group>
  <id>` + readTestGroupID + `</id>
  <name>tf-acc-account-group</name>
  <access_level>Full Access</access_level>
  <privilege_set>` + privilegeSet + `</privilege_set>
  <privileges>
    <jss_objects>
      <privilege>Read Computers</privilege>
      <privilege>Read Knobs</privilege>
    </jss_objects>
  </privileges>
</group>`
}

// groupReadResource returns a resource wired to a stub server answering the
// three classic calls a first hydration makes: the group itself, the account
// list, and the Administrator group the catalog is discovered from. The seam is
// the HTTP boundary rather than an injected interface, matching
// jamfplatform_pro_account's own Read tests: Read holds a concrete SDK client,
// and an interface introduced only for a test would be a bigger change than the
// behaviour it pins.
func groupReadResource(t *testing.T, privilegeSet string) *AccountGroupResource {
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
		case strings.HasSuffix(r.URL.Path, "/accounts/groupid/1"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(administratorGroupXML))
		case strings.Contains(r.URL.Path, "/accounts/groupid/"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(groupXML(privilegeSet)))
		default:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<accounts>
  <groups>
    <group><id>1</id><name>tf-acc-administrators</name></group>
    <group><id>` + readTestGroupID + `</id><name>tf-acc-account-group</name></group>
  </groups>
</accounts>`))
		}
	}))
	t.Cleanup(server.Close)

	client := jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0))
	return &AccountGroupResource{client: proclassic.New(client)}
}

// readGroupAfterImport drives Read with the state Terraform hands a resource
// after `terraform import`: the framework's empty state with the passthrough
// identifier written into it, which is what ImportStatePassthroughID produces.
// display_name is null there and populated on every managed refresh, which is
// the firstHydration signal Read gates on.
func readGroupAfterImport(t *testing.T, r *AccountGroupResource) AccountGroupResourceModel {
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
	if diags := stub.SetAttribute(ctx, path.Root("id"), readTestGroupID); diags.HasError() {
		t.Fatalf("seeding the imported identifier: %v", diags)
	}

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: stub.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{State: stub}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state AccountGroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the hydrated state: %v", diags)
	}
	return state
}

// TestRead_PresetPrivilegeSetAdoptsNoPrivileges pins the gate that filtering
// alone did not close. Jamf Pro expands a preset privilege_set into a full grid
// and discards any grid sent alongside one, so adopting the expansion put around
// 135 server-derived privileges under Terraform management that no write could
// change: adding a grantable privilege to the adopted block sent nothing, the
// next Read intersected the addition away, and the plan never converged.
func TestRead_PresetPrivilegeSetAdoptsNoPrivileges(t *testing.T) {
	state := readGroupAfterImport(t, groupReadResource(t, proclassic.GroupPrivilegeSetAuditor))

	if got := state.PrivilegeSet.ValueString(); got != proclassic.GroupPrivilegeSetAuditor {
		t.Fatalf("privilege_set = %q, want the preset set the stub returned", got)
	}
	if state.Privileges != nil {
		t.Errorf("privileges = %+v, want null: a preset set's grid is server-owned and unwritable", state.Privileges)
	}
}

// TestRead_CustomPrivilegeSetFiltersToTheCatalog is the control on that gate: a
// Custom grid is still adopted, and still filtered to the tenant's grantable
// catalog. "Read Knobs" is what an 11.x tenant returns but refuses to grant, and
// keeping it in state hands ModifyPlan's own Validate a configuration it rejects.
func TestRead_CustomPrivilegeSetFiltersToTheCatalog(t *testing.T) {
	ctx := context.Background()
	state := readGroupAfterImport(t, groupReadResource(t, proclassic.GroupPrivilegeSetCustom))

	if state.Privileges == nil {
		t.Fatal("privileges = null; a Custom group must carry the grid the classic endpoint returned")
	}
	var objects []string
	if diags := state.Privileges.JamfProServerObjects.ElementsAs(ctx, &objects, false); diags.HasError() {
		t.Fatalf("reading the hydrated object privileges: %v", diags)
	}
	if len(objects) != 1 || objects[0] != "Read Computers" {
		t.Errorf("jamf_pro_server_objects = %v, want only the privilege the catalog grants", objects)
	}
}
