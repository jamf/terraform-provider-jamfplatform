// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// TestConnectionComparisonsCoverEveryConfigurableAttribute is the guard that keeps
// replacement-on-change honest.
//
// An attribute an operator can set but connectionComparisons does not list plans
// as an in-place update, and Jamf's update endpoint refuses every one — so the
// omission surfaces as an apply failure on that single attribute, which no other
// test would catch. Adding an attribute without listing it fails here instead.
//
// Two names are exempt, for opposite reasons. `client_secret` is WriteOnly, so it
// is never in state and comparing it would report a change on every plan;
// rotation runs through `client_secret_wo_version`, which is compared. `timeouts`
// is provider-side configuration Jamf never sees, so changing it must not touch
// the connection.
func TestConnectionComparisonsCoverEveryConfigurableAttribute(t *testing.T) {
	exempt := map[string]string{
		"client_secret": "WriteOnly, never in state; rotation goes through client_secret_wo_version",
		"timeouts":      "provider-side configuration with no counterpart in Jamf Account",
		// These two carry stringplanmodifier.RequiresReplace() on the schema
		// already, because Jamf refuses to move a connection to another provider
		// family or region even when the endpoint works. They replace for a
		// reason that outlives the broken endpoint, so they stay in the schema
		// rather than moving into the temporary comparison list — and comparing
		// them in both places would be the one duplication this guard exists to
		// prevent.
		"connection_type": "already RequiresReplace in the schema, independently of the update endpoint",
		"hosting_region":  "already RequiresReplace in the schema, independently of the update endpoint",
	}

	compared := map[string]bool{}
	for _, comparison := range connectionComparisons(ConnectionResourceModel{}, ConnectionResourceModel{}) {
		compared[comparison.name] = true
	}

	var schemaResp resource.SchemaResponse
	NewConnectionResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	settable := map[string]bool{}
	for name, attribute := range schemaResp.Schema.Attributes {
		if attribute.IsRequired() || attribute.IsOptional() {
			settable[name] = true
		}
	}
	for name := range schemaResp.Schema.Blocks {
		settable[name] = true
	}
	if len(settable) == 0 {
		t.Fatal("the schema reported no settable attributes, so this guard would pass vacuously")
	}

	for name := range settable {
		if reason, ok := exempt[name]; ok {
			if compared[name] {
				t.Errorf("%s is compared but documented as exempt (%s)", name, reason)
			}
			continue
		}
		if !compared[name] {
			t.Errorf("%s can be set but is not compared, so changing it would plan an in-place update "+
				"that Jamf's update endpoint refuses; add it to connectionComparisons", name)
		}
	}

	for name := range compared {
		if !settable[name] {
			t.Errorf("%s is compared but is not a settable attribute; the list has drifted from the schema", name)
		}
	}
}

// connectionStringSet builds a set of strings, failing the test rather than
// returning a zero value a comparison would silently read as equal.
func connectionStringSet(t *testing.T, values ...string) types.Set {
	t.Helper()
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("building a set from %v: %v", values, diags)
	}
	return set
}

// baselineConnectionModel returns a connection with every compared attribute
// populated, for a test to change exactly one of.
//
// Nothing here is null, and that is the point. A test that started from a null
// and set a value would pass for two reasons at once — the change it made, and
// the null it changed from — so it could not tell a comparison that works from
// one that reports a change on every plan. Starting from a concrete value on
// both sides means the only difference is the one the test introduces.
func baselineConnectionModel(t *testing.T) ConnectionResourceModel {
	t.Helper()
	return ConnectionResourceModel{
		ID:                       types.StringValue(unitConnectionID),
		Name:                     types.StringValue(unitConnectionName),
		ConnectionType:           types.StringValue(connectionTypeGenericOIDC),
		HostingRegion:            types.StringValue(account.RegionUs),
		AuthMethod:               types.StringValue(authMethodClientSecret),
		ClientID:                 types.StringValue("probe-client-id"),
		ClientSecretWOVersion:    types.Int64Value(1),
		Scopes:                   types.StringValue("openid email profile"),
		PKCE:                     types.StringValue(pkceS256),
		SendNonce:                types.BoolValue(true),
		SyncAttributesAtLogin:    types.BoolValue(true),
		OmitLoginHint:            types.BoolValue(false),
		CustomUsernameClaimName:  types.StringValue("upn"),
		UsernameDomain:           types.StringValue("tf-unit.example"),
		AttributeMap:             types.StringValue(`{"mapping_mode":"bind_all"}`),
		GroupNameFilter:          &GroupNameFilterModel{Operator: types.StringValue(filterOperatorOr), Groups: connectionStringSet(t, "Engineering")},
		SessionDurationMinutes:   types.Int64Value(480),
		InactivityTimeoutMinutes: types.Int64Value(30),
		Domains:                  connectionStringSet(t, "tf-unit.example"),
		EnabledProducts: []EnabledProductModel{{
			Product:          types.StringValue(account.ProductSecurityCloud),
			Tenants:          connectionStringSet(t, "tenant-one"),
			ManagedAccountID: types.StringNull(),
		}},
		EnabledEnvironments: []EnabledEnvironmentModel{{
			Product:          types.StringValue(account.ProductSecurityCloud),
			Environments:     connectionStringSet(t, "environment-one"),
			ManagedAccountID: types.StringNull(),
		}},
		GenericOIDC: &GenericOIDCSettingsModel{
			IssuerURL:             types.StringValue("idp.example"),
			AuthorizationEndpoint: types.StringValue("idp.example/authorize"),
			TokenEndpoint:         types.StringValue("idp.example/token"),
			JWKSURI:               types.StringValue("idp.example/keys"),
			UserInfoEndpoint:      types.StringNull(),
		},
		Entra:           &EntraSettingsModel{Domain: types.StringValue("entra.example")},
		Okta:            &OktaSettingsModel{Domain: types.StringValue("okta.example")},
		GoogleWorkspace: &GoogleWorkspaceSettingsModel{Domain: types.StringValue("workspace.example")},
	}
}

// TestChangedConfigurablePaths_AnUnchangedPlanReplacesNothing pins the case that
// has to hold on every plan an operator runs against an unedited configuration:
// no attribute differs, so nothing is replaced.
//
// It is the floor the rest of these tests stand on. A comparison that reported a
// change here would replace every connection in the estate on the next plan, and
// no assertion about a *changed* attribute would notice, because both would
// report a change.
func TestChangedConfigurablePaths_AnUnchangedPlanReplacesNothing(t *testing.T) {
	if changed := changedConfigurablePaths(baselineConnectionModel(t), baselineConnectionModel(t)); len(changed) != 0 {
		t.Fatalf("changed paths = %v, want none for two identical connections", changed)
	}
}

// TestChangedConfigurablePaths_ReportsTheChangedAttributeAndNothingElse walks one
// change of each shape the model holds and checks the reported path is the
// changed attribute, alone.
//
// The shapes matter more than the attributes: a scalar, a number, a collection,
// a slice of nested objects and a pointer to a settings struct are five
// different things to reflect.DeepEqual, and the settings blocks are compared
// whole, so a change inside one has to surface as the block's own path rather
// than as nothing at all.
func TestChangedConfigurablePaths_ReportsTheChangedAttributeAndNothingElse(t *testing.T) {
	cases := []struct {
		name      string
		attribute string
		change    func(t *testing.T, plan *ConnectionResourceModel)
	}{
		{"a renamed connection", "name", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.Name = types.StringValue("tf-unit-renamed")
		}},
		{"a toggled flag", "send_nonce", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.SendNonce = types.BoolValue(false)
		}},
		{"a retimed session", "session_duration_minutes", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.SessionDurationMinutes = types.Int64Value(240)
		}},
		{"a rotated secret", "client_secret_wo_version", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.ClientSecretWOVersion = types.Int64Value(2)
		}},
		{"an added domain", "domains", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.Domains = connectionStringSet(t, "tf-unit.example", "second.example")
		}},
		{"a retenanted product", "enabled_products", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.EnabledProducts[0].Tenants = connectionStringSet(t, "tenant-two")
		}},
		{"a retargeted environment", "enabled_environments", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.EnabledEnvironments[0].Environments = connectionStringSet(t, "environment-two")
		}},
		{"an edited settings block", "entra", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.Entra = &EntraSettingsModel{Domain: types.StringValue("other.example")}
		}},
		{"an edited group filter", "group_name_filter", func(t *testing.T, plan *ConnectionResourceModel) {
			plan.GroupNameFilter = &GroupNameFilterModel{
				Operator: types.StringValue(filterOperatorAnd),
				Groups:   connectionStringSet(t, "Engineering"),
			}
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			state := baselineConnectionModel(t)
			plan := baselineConnectionModel(t)
			testCase.change(t, &plan)

			changed := changedConfigurablePaths(plan, state)
			if len(changed) != 1 || changed[0].String() != testCase.attribute {
				t.Fatalf("changed paths = %v, want exactly [%s]", changed, testCase.attribute)
			}
		})
	}
}

// TestChangedConfigurablePaths_AnUnknownPlannedValueIsNotAChange is the guard on
// the one exemption in planValueDiffers, and the most destructive regression this
// file can catch.
//
// Unknown is what the framework puts in the plan for a value it cannot resolve
// yet — most often one interpolated from another resource that has not been
// applied. Read as a change, it replaces the connection on every plan where any
// dependency is pending: an operator who adds a new SSO domain and references it
// here would be shown a destroy-and-recreate of a live connection, and every
// person signing in through it would be signed out on apply.
//
// The four shapes are not redundant. planValueDiffers exempts a value only if it
// type-asserts to attr.Value, so a String, a Bool, an Int64 and a Set each have
// to be checked: they are separate framework types, and a change to the assertion
// could keep one working while dropping the others.
func TestChangedConfigurablePaths_AnUnknownPlannedValueIsNotAChange(t *testing.T) {
	cases := []struct {
		name        string
		makeUnknown func(plan *ConnectionResourceModel)
	}{
		{"a string", func(plan *ConnectionResourceModel) { plan.Name = types.StringUnknown() }},
		{"a boolean", func(plan *ConnectionResourceModel) { plan.SendNonce = types.BoolUnknown() }},
		{"a number", func(plan *ConnectionResourceModel) { plan.SessionDurationMinutes = types.Int64Unknown() }},
		{"a collection", func(plan *ConnectionResourceModel) { plan.Domains = types.SetUnknown(types.StringType) }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			state := baselineConnectionModel(t)
			plan := baselineConnectionModel(t)
			testCase.makeUnknown(&plan)

			changed := changedConfigurablePaths(plan, state)
			if len(changed) != 0 {
				t.Fatalf("changed paths = %v, want none: an unresolved value in the plan is not an operator "+
					"changing one, and replacing the connection for it would sign every user out", changed)
			}
		})
	}
}

// TestChangedConfigurablePaths_ARemovedValueIsAChange is the complement to the
// Unknown exemption: a value the operator took away is a real change and must be
// reported.
//
// Null and Unknown are both "no value here" to read casually, and only one of
// them is exempt. An exemption widened from IsUnknown to IsNull, or to a nil
// check on the settings pointers, would make removing a group filter or clearing
// a domain plan as an in-place update — which Jamf's update endpoint refuses, so
// it fails during apply rather than quietly doing nothing.
func TestChangedConfigurablePaths_ARemovedValueIsAChange(t *testing.T) {
	cases := []struct {
		name      string
		attribute string
		remove    func(plan *ConnectionResourceModel)
	}{
		{"a removed settings block", "group_name_filter", func(plan *ConnectionResourceModel) { plan.GroupNameFilter = nil }},
		{"a removed provider block", "entra", func(plan *ConnectionResourceModel) { plan.Entra = nil }},
		{"a cleared scalar", "username_domain", func(plan *ConnectionResourceModel) { plan.UsernameDomain = types.StringNull() }},
		{"a cleared collection", "domains", func(plan *ConnectionResourceModel) { plan.Domains = types.SetNull(types.StringType) }},
		{"a dropped product list", "enabled_products", func(plan *ConnectionResourceModel) { plan.EnabledProducts = nil }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			state := baselineConnectionModel(t)
			plan := baselineConnectionModel(t)
			testCase.remove(&plan)

			changed := changedConfigurablePaths(plan, state)
			if len(changed) != 1 || changed[0].String() != testCase.attribute {
				t.Fatalf("changed paths = %v, want exactly [%s]", changed, testCase.attribute)
			}
		})
	}
}

// TestModifyPlan_AChangedAttributeReplacesTheConnection runs the whole mechanism
// through the framework's own plan and state values, rather than the models the
// comparison tests hand it directly.
//
// It is what pins the wiring between the two: the models the comparison tests
// build by hand cannot catch a plan the resource fails to decode, an attribute
// the schema and the model disagree about, or a changed path that is collected
// but never appended to RequiresReplace. The resource is built without a client
// on purpose — deciding on a replacement takes no request, and a version of this
// that did would fail here rather than at apply.
func TestModifyPlan_AChangedAttributeReplacesTheConnection(t *testing.T) {
	ctx := context.Background()
	r := &ConnectionResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	s := schemaResp.Schema

	recorded := connectionValue(ctx, t, s, withOIDCConfiguration(), withID(unitConnectionID))
	planned := connectionValue(ctx, t, s, withOIDCConfiguration(), withID(unitConnectionID),
		func(ctx context.Context, t *testing.T, state *tfsdk.State) {
			t.Helper()
			setAttribute(ctx, t, state, path.Root("name"), "tf-unit-renamed")
		})

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: s, Raw: recorded},
		Plan:  tfsdk.Plan{Schema: s, Raw: planned},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("plan diagnostics: %v", resp.Diagnostics)
	}
	if len(resp.RequiresReplace) != 1 || resp.RequiresReplace[0].String() != "name" {
		t.Fatalf("RequiresReplace = %v, want exactly [name]", resp.RequiresReplace)
	}
}

// TestModifyPlan_ACreateOrADestroyReplacesNothing pins the two lifecycle stages
// the modifier deliberately steps out of.
//
// On a create there is no recorded connection to replace, and on a destroy there
// is no planned one; the framework signals both with a null raw value, and
// decoding one into the model would report every attribute as changed. Getting
// this wrong turns a first apply or a clean destroy into a nonsensical
// replacement plan.
func TestModifyPlan_ACreateOrADestroyReplacesNothing(t *testing.T) {
	ctx := context.Background()
	r := &ConnectionResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	s := schemaResp.Schema

	absent := tftypes.NewValue(s.Type().TerraformType(ctx).(tftypes.Object), nil)
	present := connectionValue(ctx, t, s, withOIDCConfiguration(), withID(unitConnectionID))

	cases := []struct {
		name  string
		state tftypes.Value
		plan  tftypes.Value
	}{
		{"a create", absent, present},
		{"a destroy", present, absent},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var resp resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{
				State: tfsdk.State{Schema: s, Raw: testCase.state},
				Plan:  tfsdk.Plan{Schema: s, Raw: testCase.plan},
			}, &resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("plan diagnostics: %v", resp.Diagnostics)
			}
			if len(resp.RequiresReplace) != 0 {
				t.Fatalf("RequiresReplace = %v, want none on %s", resp.RequiresReplace, testCase.name)
			}
		})
	}
}

// TestModifyPlan_AnUnknownInsideANestedBlockReplacesNothing pins the exemption at
// depth, which is where the decoded model cannot express it.
//
// A settings block is a pointer to a struct and a product collection is a plain
// Go slice, so neither implements attr.Value and planValueDiffers cannot
// recognise an unknown inside one. An operator writing a settings block whose
// value comes from a resource that has not been applied yet therefore looked like
// a changed block, and a changed block plans a destroy and recreate of a live
// connection. ModifyPlan consults the raw plan instead, which carries
// unknown-ness at every depth.
func TestModifyPlan_AnUnknownInsideANestedBlockReplacesNothing(t *testing.T) {
	ctx := context.Background()
	r := &ConnectionResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	s := schemaResp.Schema

	recorded := connectionValue(ctx, t, s, withOIDCConfiguration(), withID(unitConnectionID))
	planned := connectionValue(ctx, t, s, withOIDCConfiguration(), withID(unitConnectionID),
		func(ctx context.Context, t *testing.T, state *tfsdk.State) {
			t.Helper()
			setAttribute(ctx, t, state, path.Root("generic_oidc").AtName("issuer_url"), types.StringUnknown())
		})

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: s, Raw: recorded},
		Plan:  tfsdk.Plan{Schema: s, Raw: planned},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
	}
	if len(resp.RequiresReplace) != 0 {
		t.Errorf("RequiresReplace = %v, want no replacement while the block holds an unresolved reference", resp.RequiresReplace)
	}
}

// TestAttributeMapReindentingIsNotAReplacement pins the fix for the most
// destructive defect this resource had: because every changed configurable
// attribute here forces replacement, and attribute_map was compared with
// reflect.DeepEqual over the raw strings, importing a connection and applying
// the configuration `-generate-config-out` produced destroyed and recreated a
// live SSO connection over JSON whitespace. Observed on a real org-scoped
// estate: "attribute_map = jsonencode( # whitespace changes force replacement".
func TestAttributeMapReindentingIsNotAReplacement(t *testing.T) {
	// The same map, as Jamf Account returns it and as jsonencode re-emits it.
	fromJamf := types.StringValue(`{"mapping_mode":"use_map","userinfo_scope":"openid email"}`)
	reEmitted := types.StringValue("{\n  \"userinfo_scope\": \"openid email\",\n  \"mapping_mode\": \"use_map\"\n}")

	if planValueDiffers(jsonComparable(reEmitted), jsonComparable(fromJamf)) {
		t.Error("reindenting and reordering attribute_map must not read as a change")
	}

	// A real change still is one.
	changed := types.StringValue(`{"mapping_mode":"bind_all"}`)
	if !planValueDiffers(jsonComparable(changed), jsonComparable(fromJamf)) {
		t.Error("a different mapping_mode must still read as a change")
	}
}

// TestJsonComparablePassesThroughWhatItCannotParse keeps the normalisation out
// of the way of the other rules: an unknown value must stay recognisable as
// unknown for planValueDiffers' own exemption, and a non-JSON value is the
// validator's business rather than this comparison's.
func TestJsonComparablePassesThroughWhatItCannotParse(t *testing.T) {
	for name, value := range map[string]types.String{
		"unknown":  types.StringUnknown(),
		"null":     types.StringNull(),
		"not json": types.StringValue("mapping_mode=bind_all"),
	} {
		if got := jsonComparable(value); got != value {
			t.Errorf("%s: jsonComparable returned %v, want the value unchanged", name, got)
		}
	}

	// And the unknown exemption still holds through the wrapper.
	if planValueDiffers(jsonComparable(types.StringUnknown()), jsonComparable(types.StringValue(`{"a":1}`))) {
		t.Error("an unknown planned value must not read as a change")
	}
}
