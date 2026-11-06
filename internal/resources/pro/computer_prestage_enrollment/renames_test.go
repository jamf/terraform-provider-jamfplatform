// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

const removalDate = "2027-02-03"

func resourceSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewComputerPrestageEnrollmentResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func nullRaw(s schema.Schema) tftypes.Value {
	ot := s.Type().TerraformType(context.Background()).(tftypes.Object)
	m := make(map[string]tftypes.Value, len(ot.AttributeTypes))
	for k, at := range ot.AttributeTypes {
		m[k] = tftypes.NewValue(at, nil)
	}
	return tftypes.NewValue(ot, m)
}

type attrValue struct {
	name  string
	value any
}

func stateWith(t *testing.T, s schema.Schema, vals ...attrValue) tfsdk.State {
	t.Helper()
	st := tfsdk.State{Schema: s, Raw: nullRaw(s)}
	for _, v := range vals {
		if d := st.SetAttribute(context.Background(), path.Root(v.name), v.value); d.HasError() {
			t.Fatalf("set %s: %v", v.name, d)
		}
	}
	return st
}

func stringSet(vals ...string) types.Set {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elems)
}

func modifyPlan(t *testing.T, s schema.Schema, cfg, state tfsdk.State) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: s, Raw: cfg.Raw}
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: s, Raw: cfg.Raw},
		Plan:   plan,
		State:  state,
	}
	resp := &resource.ModifyPlanResponse{Plan: plan}
	(&ComputerPrestageEnrollmentResource{}).ModifyPlan(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
	}
	return resp.Plan
}

func absentState(s schema.Schema) tfsdk.State {
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
}

func getBool(t *testing.T, p tfsdk.Plan, name string) types.Bool {
	t.Helper()
	var v types.Bool
	if d := p.GetAttribute(context.Background(), path.Root(name), &v); d.HasError() {
		t.Fatalf("get %s: %v", name, d)
	}
	return v
}

func validate(t *testing.T, a schema.Attribute, p path.Path, cfg tfsdk.Config, value attr.Value) string {
	t.Helper()
	ctx := context.Background()
	var sb strings.Builder
	note := func(d diag.Diagnostics) {
		for _, e := range d.Errors() {
			sb.WriteString(e.Summary() + " " + e.Detail())
		}
	}
	switch x := a.(type) {
	case schema.BoolAttribute:
		for _, v := range x.Validators {
			resp := &validator.BoolResponse{}
			v.ValidateBool(ctx, validator.BoolRequest{Path: p, PathExpression: p.Expression(), Config: cfg, ConfigValue: value.(types.Bool)}, resp)
			note(resp.Diagnostics)
		}
	case schema.StringAttribute:
		for _, v := range x.Validators {
			resp := &validator.StringResponse{}
			v.ValidateString(ctx, validator.StringRequest{Path: p, PathExpression: p.Expression(), Config: cfg, ConfigValue: value.(types.String)}, resp)
			note(resp.Diagnostics)
		}
	case schema.SetAttribute:
		for _, v := range x.Validators {
			resp := &validator.SetResponse{}
			v.ValidateSet(ctx, validator.SetRequest{Path: p, PathExpression: p.Expression(), Config: cfg, ConfigValue: value.(types.Set)}, resp)
			note(resp.Diagnostics)
		}
	}
	return sb.String()
}

func TestRenames_EveryDeprecatedAttributeNamesItsReplacementAndTheRemovalDate(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	account := s.Attributes["account_settings"].(schema.SingleNestedAttribute).Attributes

	check := func(name string, a schema.Attribute, replacement string) {
		t.Helper()
		msg := a.GetDeprecationMessage()
		if msg == "" {
			t.Errorf("%s: must be Deprecated", name)
			return
		}
		if !strings.Contains(msg, "`"+replacement+"`") || !strings.Contains(msg, removalDate) || !strings.Contains(msg, "`"+name+"`") {
			t.Errorf("%s: message must name `%s`, the attribute and %s; got %q", name, replacement, removalDate, msg)
		}
		if !strings.HasPrefix(a.GetMarkdownDescription(), "~> Deprecated") {
			t.Errorf("%s: the registry page needs the leading callout; got %q", name, a.GetMarkdownDescription())
		}
	}
	for _, p := range topLevelPairs {
		check(p.Old, s.Attributes[p.Old], p.New)
		if s.Attributes[p.New].GetDeprecationMessage() != "" {
			t.Errorf("%s: the replacement must not be deprecated", p.New)
		}
	}
	for _, p := range accountSettingsPairs {
		check(p.Old, account[p.Old], p.New)
	}
	check("install_profiles_during_setup", s.Attributes["install_profiles_during_setup"], "configuration_profile_ids")
	check("payload_configured", account["payload_configured"], "account_settings")
}

func TestRenames_DerivedOnlyAttributesAreOptionalNotComputed(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	account := s.Attributes["account_settings"].(schema.SingleNestedAttribute).Attributes
	for name, a := range map[string]schema.Attribute{
		"install_profiles_during_setup": s.Attributes["install_profiles_during_setup"],
		"payload_configured":            account["payload_configured"],
	} {
		if !a.IsOptional() || a.IsComputed() || a.IsRequired() {
			t.Errorf("%s: state must hold only what the author wrote, so it is Optional and not Computed", name)
		}
	}
}

func TestRenames_FormerlyRequiredNamesAreOptionalAndOneMustBeSet(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	cfgNeither := tfsdk.Config{Schema: s, Raw: nullRaw(s)}

	for _, p := range topLevelPairs {
		oldAttr, newAttr := s.Attributes[p.Old], s.Attributes[p.New]
		if oldAttr.IsRequired() || newAttr.IsRequired() {
			t.Errorf("%s/%s: neither name may be Required", p.Old, p.New)
		}
		var nullOf attr.Value = types.BoolNull()
		switch newAttr.(type) {
		case schema.StringAttribute:
			nullOf = types.StringNull()
		case schema.SetAttribute:
			nullOf = types.SetNull(types.StringType)
		}
		got := validate(t, newAttr, path.Root(p.New), cfgNeither, nullOf)
		formerlyRequired := map[string]bool{
			"mandatory": true, "mdm_removable": true, "auto_advance_setup": true,
			"keep_existing_location_information": true, "keep_existing_site_membership": true,
		}[p.Old]
		if formerlyRequired && got == "" {
			t.Errorf("%s: setting neither name must fail validation, as Required did", p.New)
		}
		if !formerlyRequired && got != "" {
			t.Errorf("%s: was Optional, so setting neither must pass; got %q", p.New, got)
		}
	}
}

func TestRenames_SettingBothNamesIsAConflict(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	st := stateWith(t, s,
		attrValue{"mandatory", types.BoolValue(true)},
		attrValue{"make_mdm_profile_mandatory", types.BoolValue(true)},
		attrValue{"authentication_prompt", types.StringValue("a")},
		attrValue{"authentication_message", types.StringValue("b")},
		attrValue{"prestage_installed_profile_ids", stringSet("1")},
		attrValue{"configuration_profile_ids", stringSet("1")},
	)
	cfg := tfsdk.Config{Schema: s, Raw: st.Raw}
	if got := validate(t, s.Attributes["mandatory"], path.Root("mandatory"), cfg, types.BoolValue(true)); got == "" {
		t.Error("mandatory with make_mdm_profile_mandatory must conflict")
	}
	if got := validate(t, s.Attributes["authentication_prompt"], path.Root("authentication_prompt"), cfg, types.StringValue("a")); got == "" {
		t.Error("authentication_prompt with authentication_message must conflict")
	}
	if got := validate(t, s.Attributes["prestage_installed_profile_ids"], path.Root("prestage_installed_profile_ids"), cfg, stringSet("1")); got == "" {
		t.Error("prestage_installed_profile_ids with configuration_profile_ids must conflict")
	}
}

func TestRenames_NestedBothNamesConflictAndNeitherPasses(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	account := s.Attributes["account_settings"].(schema.SingleNestedAttribute).Attributes
	both := stateWith(t, s, attrValue{"account_settings", &AccountSettingsModel{
		LocalAdminAccountEnabled:               types.BoolValue(true),
		CreateManagedLocalAdministratorAccount: types.BoolValue(true),
	}})
	p := path.Root("account_settings").AtName("local_admin_account_enabled")
	if got := validate(t, account["local_admin_account_enabled"], p, tfsdk.Config{Schema: s, Raw: both.Raw}, types.BoolValue(true)); got == "" {
		t.Error("both names in account_settings must conflict")
	}
	only := stateWith(t, s, attrValue{"account_settings", &AccountSettingsModel{CreateManagedLocalAdministratorAccount: types.BoolValue(true)}})
	np := path.Root("account_settings").AtName("create_managed_local_administrator_account")
	if got := validate(t, account["create_managed_local_administrator_account"], np, tfsdk.Config{Schema: s, Raw: only.Raw}, types.BoolValue(true)); got != "" {
		t.Errorf("one name must pass: %s", got)
	}
}

func TestRenames_InstallProfilesFalseWithProfilesIsRejected(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	a := s.Attributes["install_profiles_during_setup"]
	p := path.Root("install_profiles_during_setup")
	for name, ids := range map[string]attrValue{
		"new name":        {"configuration_profile_ids", stringSet("11")},
		"deprecated name": {"prestage_installed_profile_ids", stringSet("11")},
	} {
		st := stateWith(t, s, attrValue{"install_profiles_during_setup", types.BoolValue(false)}, ids)
		if got := validate(t, a, p, tfsdk.Config{Schema: s, Raw: st.Raw}, types.BoolValue(false)); got == "" {
			t.Errorf("%s: false with a profile must be rejected at plan time", name)
		}
	}
	st := stateWith(t, s, attrValue{"install_profiles_during_setup", types.BoolValue(false)}, attrValue{"configuration_profile_ids", stringSet()})
	if got := validate(t, a, p, tfsdk.Config{Schema: s, Raw: st.Raw}, types.BoolValue(false)); got != "" {
		t.Errorf("false with no profile is consistent: %s", got)
	}
	st = stateWith(t, s, attrValue{"install_profiles_during_setup", types.BoolValue(true)}, attrValue{"configuration_profile_ids", stringSet()})
	if got := validate(t, a, p, tfsdk.Config{Schema: s, Raw: st.Raw}, types.BoolValue(true)); got != "" {
		t.Errorf("true with no profile is accepted by Jamf Pro: %s", got)
	}
}

func TestRenames_ModifyPlanCopiesTheConfiguredNameIntoTheOther(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)

	t.Run("deprecated only fills the replacement", func(t *testing.T) {
		cfg := stateWith(t, s, attrValue{"mandatory", types.BoolValue(true)})
		plan := modifyPlan(t, s, cfg, absentState(s))
		if got := getBool(t, plan, "make_mdm_profile_mandatory"); got.IsNull() || !got.ValueBool() {
			t.Errorf("replacement must follow the deprecated name, got %s", got)
		}
		if got := getBool(t, plan, "mandatory"); !got.ValueBool() {
			t.Errorf("deprecated name keeps its configured value, got %s", got)
		}
	})

	t.Run("replacement only leaves the deprecated name null when state never held it", func(t *testing.T) {
		cfg := stateWith(t, s, attrValue{"make_mdm_profile_mandatory", types.BoolValue(true)})
		plan := modifyPlan(t, s, cfg, absentState(s))
		if got := getBool(t, plan, "mandatory"); !got.IsNull() {
			t.Errorf("deprecated name must stay absent for a new configuration, got %s", got)
		}
	})

	t.Run("replacement only keeps the deprecated name in step when state holds it", func(t *testing.T) {
		cfg := stateWith(t, s, attrValue{"make_mdm_profile_mandatory", types.BoolValue(false)})
		prior := stateWith(t, s, attrValue{"mandatory", types.BoolValue(true)}, attrValue{"make_mdm_profile_mandatory", types.BoolValue(true)})
		plan := modifyPlan(t, s, cfg, prior)
		if got := getBool(t, plan, "mandatory"); got.IsNull() || got.ValueBool() {
			t.Errorf("deprecated name must follow the replacement to false, got %s", got)
		}
	})

	t.Run("set pair copies the deprecated set", func(t *testing.T) {
		cfg := stateWith(t, s, attrValue{"prestage_installed_profile_ids", stringSet("7")})
		plan := modifyPlan(t, s, cfg, absentState(s))
		var got types.Set
		if d := plan.GetAttribute(context.Background(), path.Root("configuration_profile_ids"), &got); d.HasError() {
			t.Fatal(d)
		}
		if !got.Equal(stringSet("7")) {
			t.Errorf("configuration_profile_ids = %s, want [7]", got)
		}
	})

	t.Run("string pair copies the deprecated value", func(t *testing.T) {
		cfg := stateWith(t, s, attrValue{"authentication_prompt", types.StringValue("hello")})
		plan := modifyPlan(t, s, cfg, absentState(s))
		var got types.String
		if d := plan.GetAttribute(context.Background(), path.Root("authentication_message"), &got); d.HasError() {
			t.Fatal(d)
		}
		if got.ValueString() != "hello" {
			t.Errorf("authentication_message = %s, want hello", got)
		}
	})
}

func TestRenames_ModifyPlanReconcilesAccountSettingsOnlyWhenTheBlockIsConfigured(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	ctx := context.Background()

	absent := modifyPlan(t, s, stateWith(t, s), absentState(s))
	var blk types.Object
	if d := absent.GetAttribute(ctx, path.Root("account_settings"), &blk); d.HasError() || !blk.IsNull() {
		t.Fatalf("an omitted account_settings block must stay null: %v %v", blk, d)
	}

	cfg := stateWith(t, s, attrValue{"account_settings", &AccountSettingsModel{LocalAdminAccountEnabled: types.BoolValue(true)}})
	plan := modifyPlan(t, s, cfg, absentState(s))
	var got types.Bool
	if d := plan.GetAttribute(ctx, path.Root("account_settings").AtName("create_managed_local_administrator_account"), &got); d.HasError() {
		t.Fatal(d)
	}
	if got.IsNull() || !got.ValueBool() {
		t.Errorf("replacement must follow the deprecated name inside the block, got %s", got)
	}
}

func TestRenames_BuildersSendWhicheverNameIsSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	old := ComputerPrestageEnrollmentResourceModel{
		Mandatory:                       types.BoolValue(true),
		MdmRemovable:                    types.BoolValue(true),
		AutoAdvanceSetup:                types.BoolValue(true),
		KeepExistingLocationInformation: types.BoolValue(true),
		KeepExistingSiteMembership:      types.BoolValue(true),
		AuthenticationPrompt:            types.StringValue("old"),
		PrestageInstalledProfileIds:     stringSet("3"),
	}
	post, d := buildPostInput(ctx, old, ComputerPrestageEnrollmentResourceModel{})
	if d.HasError() {
		t.Fatal(d)
	}
	if !post.Mandatory || !post.MDMRemovable || !post.AutoAdvanceSetup || !post.KeepExistingLocationInformation || !post.KeepExistingSiteMembership {
		t.Errorf("deprecated names must still write: %+v", post)
	}
	if post.AuthenticationPrompt != "old" || len(post.PrestageInstalledProfileIds) != 1 {
		t.Errorf("deprecated string and set must still write: %+v", post)
	}

	replaced := ComputerPrestageEnrollmentResourceModel{
		MakeMdmProfileMandatory:                   types.BoolValue(true),
		AllowMdmProfileRemoval:                    types.BoolValue(true),
		AutomaticallyAdvanceThroughSetupAssistant: types.BoolValue(true),
		UseExistingLocationInformation:            types.BoolValue(true),
		UseExistingSiteMembership:                 types.BoolValue(true),
		AuthenticationMessage:                     types.StringValue("new"),
		ConfigurationProfileIds:                   stringSet("4", "5"),
	}
	put, d := buildPutInput(ctx, replaced, ComputerPrestageEnrollmentResourceModel{}, &pro.GetComputerPrestageV3{}, false, false)
	if d.HasError() {
		t.Fatal(d)
	}
	if !put.Mandatory || !put.MDMRemovable || !put.AutoAdvanceSetup || !put.KeepExistingLocationInformation || !put.KeepExistingSiteMembership {
		t.Errorf("replacement names must write: %+v", put)
	}
	if put.AuthenticationPrompt != "new" || len(put.PrestageInstalledProfileIds) != 2 {
		t.Errorf("replacement string and set must write: %+v", put)
	}

	both := replaced
	both.Mandatory = types.BoolValue(false)
	post, _ = buildPostInput(ctx, both, ComputerPrestageEnrollmentResourceModel{})
	if !post.Mandatory {
		t.Error("the replacement wins when the plan holds both")
	}
}

func TestRenames_InstallProfilesFlagIsDerivedFromTheProfileSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		plan ComputerPrestageEnrollmentResourceModel
		want bool
	}{
		"profiles listed":            {ComputerPrestageEnrollmentResourceModel{ConfigurationProfileIds: stringSet("1")}, true},
		"deprecated set listed":      {ComputerPrestageEnrollmentResourceModel{PrestageInstalledProfileIds: stringSet("1")}, true},
		"none listed":                {ComputerPrestageEnrollmentResourceModel{ConfigurationProfileIds: stringSet()}, false},
		"neither attribute":          {ComputerPrestageEnrollmentResourceModel{}, false},
		"explicit true, none":        {ComputerPrestageEnrollmentResourceModel{InstallProfilesDuringSetup: types.BoolValue(true), ConfigurationProfileIds: stringSet()}, true},
		"explicit false, none":       {ComputerPrestageEnrollmentResourceModel{InstallProfilesDuringSetup: types.BoolValue(false), ConfigurationProfileIds: stringSet()}, false},
		"explicit true with profile": {ComputerPrestageEnrollmentResourceModel{InstallProfilesDuringSetup: types.BoolValue(true), ConfigurationProfileIds: stringSet("1")}, true},
	} {
		post, d := buildPostInput(ctx, tc.plan, ComputerPrestageEnrollmentResourceModel{})
		if d.HasError() {
			t.Fatalf("%s: %v", name, d)
		}
		if post.InstallProfilesDuringSetup != tc.want {
			t.Errorf("%s: sent %v, want %v", name, post.InstallProfilesDuringSetup, tc.want)
		}
	}
}

func TestRenames_PayloadConfiguredIsDerivedFromTheConfiguredAccountSettings(t *testing.T) {
	t.Parallel()
	configured := func(plan, cfg *AccountSettingsModel) *bool {
		return buildAccountSettingsRequest(plan, nil, cfg, false).PayloadConfigured
	}

	empty := &AccountSettingsModel{}
	if got := configured(empty, empty); got != nil {
		t.Errorf("an empty block leaves the switch unset, got %v", *got)
	}

	cfg := &AccountSettingsModel{CreateManagedLocalAdministratorAccount: types.BoolValue(true)}
	plan := &AccountSettingsModel{CreateManagedLocalAdministratorAccount: types.BoolValue(true), AdminUsername: types.StringValue("carried")}
	if got := configured(plan, cfg); got == nil || !*got {
		t.Error("a configured account setting must turn the switch on")
	}

	onlyCarried := &AccountSettingsModel{UserAccountType: types.StringValue("STANDARD"), HiddenAdminAccount: types.BoolValue(false)}
	if got := configured(onlyCarried, empty); got != nil {
		t.Errorf("values the plan merely carried from state must not turn it on, got %v", *got)
	}

	explicit := &AccountSettingsModel{PayloadConfigured: types.BoolValue(false), AdminUsername: types.StringValue("x")}
	if got := configured(explicit, explicit); got == nil || *got {
		t.Error("an explicit payload_configured wins")
	}

	deprecatedName := &AccountSettingsModel{LocalAdminAccountEnabled: types.BoolValue(false)}
	if got := configured(deprecatedName, deprecatedName); got == nil || !*got {
		t.Error("a configured deprecated name counts as a configured setting")
	}
}

func TestRenames_AccountSettingsBuilderSendsWhicheverNameIsSet(t *testing.T) {
	t.Parallel()
	oldNames := buildAccountSettingsRequest(&AccountSettingsModel{
		LocalAdminAccountEnabled:           types.BoolValue(true),
		HiddenAdminAccount:                 types.BoolValue(true),
		LocalUserManaged:                   types.BoolValue(true),
		PreventPrefillInfoFromModification: types.BoolValue(true),
	}, nil, nil, false)
	newNames := buildAccountSettingsRequest(&AccountSettingsModel{
		CreateManagedLocalAdministratorAccount:         types.BoolValue(true),
		HideManagedLocalAdministratorAccount:           types.BoolValue(true),
		MakeManagedLocalAdministratorAccountMdmEnabled: types.BoolValue(true),
		LockPrimaryAccountInformation:                  types.BoolValue(true),
	}, nil, nil, false)
	for name, got := range map[string]*pro.AccountSettingsRequest{"deprecated": oldNames, "replacement": newNames} {
		if got.LocalAdminAccountEnabled == nil || !*got.LocalAdminAccountEnabled ||
			got.HiddenAdminAccount == nil || !*got.HiddenAdminAccount ||
			got.LocalUserManaged == nil || !*got.LocalUserManaged ||
			got.PreventPrefillInfoFromModification == nil || !*got.PreventPrefillInfoFromModification {
			t.Errorf("%s names must reach the wire: %+v", name, got)
		}
	}
}

func wireGet() *pro.GetComputerPrestageV3 {
	return &pro.GetComputerPrestageV3{
		ID:                              "9",
		Mandatory:                       true,
		MDMRemovable:                    true,
		AutoAdvanceSetup:                true,
		KeepExistingLocationInformation: true,
		KeepExistingSiteMembership:      true,
		AuthenticationPrompt:            "hi",
		InstallProfilesDuringSetup:      true,
		PrestageInstalledProfileIds:     []string{"11"},
		AccountSettings: &pro.AccountSettingsResponse{
			PayloadConfigured: true, LocalAdminAccountEnabled: true, HiddenAdminAccount: true,
			LocalUserManaged: true, PreventPrefillInfoFromModification: true,
		},
	}
}

func TestRenames_ImportWritesOnlyTheReplacementNames(t *testing.T) {
	t.Parallel()
	state := ComputerPrestageEnrollmentResourceModel{ID: types.StringValue("9")}
	if d := assignGetToResource(context.Background(), &state, state, wireGet()); d.HasError() {
		t.Fatal(d)
	}
	if !state.MakeMdmProfileMandatory.ValueBool() || !state.AllowMdmProfileRemoval.ValueBool() ||
		!state.AutomaticallyAdvanceThroughSetupAssistant.ValueBool() || !state.UseExistingLocationInformation.ValueBool() ||
		!state.UseExistingSiteMembership.ValueBool() || state.AuthenticationMessage.ValueString() != "hi" ||
		len(state.ConfigurationProfileIds.Elements()) != 1 {
		t.Errorf("replacement names must hold the wire values: %+v", state)
	}
	if !state.Mandatory.IsNull() || !state.MdmRemovable.IsNull() || !state.AutoAdvanceSetup.IsNull() ||
		!state.KeepExistingLocationInformation.IsNull() || !state.KeepExistingSiteMembership.IsNull() ||
		!state.AuthenticationPrompt.IsNull() || !state.PrestageInstalledProfileIds.IsNull() {
		t.Errorf("an import must leave every deprecated name null: %+v", state)
	}
	if !state.InstallProfilesDuringSetup.IsNull() {
		t.Errorf("install_profiles_during_setup is derived and must not be read from the wire, got %s", state.InstallProfilesDuringSetup)
	}
}

func TestRenames_StateHoldingTheDeprecatedNameKeepsItInStep(t *testing.T) {
	t.Parallel()
	state := ComputerPrestageEnrollmentResourceModel{
		ID:                          types.StringValue("9"),
		Mandatory:                   types.BoolValue(false),
		MdmRemovable:                types.BoolValue(false),
		AutoAdvanceSetup:            types.BoolValue(false),
		AuthenticationPrompt:        types.StringValue("stale"),
		PrestageInstalledProfileIds: stringSet("1"),
		InstallProfilesDuringSetup:  types.BoolValue(true),
	}
	if d := assignGetToResource(context.Background(), &state, state, wireGet()); d.HasError() {
		t.Fatal(d)
	}
	if !state.Mandatory.ValueBool() || !state.MdmRemovable.ValueBool() || !state.AutoAdvanceSetup.ValueBool() {
		t.Errorf("a deprecated name state already held must refresh to the wire value: %+v", state)
	}
	if state.AuthenticationPrompt.ValueString() != "hi" || !state.PrestageInstalledProfileIds.Equal(stringSet("11")) {
		t.Errorf("deprecated string and set must refresh: %+v", state)
	}
	if !state.InstallProfilesDuringSetup.ValueBool() {
		t.Error("the author's install_profiles_during_setup must be kept exactly")
	}
	if !state.KeepExistingSiteMembership.IsNull() {
		t.Error("a deprecated name state never held must stay null")
	}
}

func TestRenames_AccountSettingsFlattenCarriesOnlyWhatTheAuthorWrote(t *testing.T) {
	t.Parallel()
	got := wireGet().AccountSettings

	fresh := flattenAccountSettings(got, nil)
	if !fresh.CreateManagedLocalAdministratorAccount.ValueBool() || !fresh.HideManagedLocalAdministratorAccount.ValueBool() ||
		!fresh.MakeManagedLocalAdministratorAccountMdmEnabled.ValueBool() || !fresh.LockPrimaryAccountInformation.ValueBool() {
		t.Errorf("replacement names must hold the wire values: %+v", fresh)
	}
	if !fresh.LocalAdminAccountEnabled.IsNull() || !fresh.HiddenAdminAccount.IsNull() || !fresh.LocalUserManaged.IsNull() ||
		!fresh.PreventPrefillInfoFromModification.IsNull() {
		t.Errorf("deprecated names must stay null without history: %+v", fresh)
	}
	if !fresh.PayloadConfigured.IsNull() {
		t.Errorf("payload_configured is derived and must stay null, got %s", fresh.PayloadConfigured)
	}

	prior := &AccountSettingsModel{
		LocalAdminAccountEnabled: types.BoolValue(false),
		PayloadConfigured:        types.BoolValue(true),
	}
	carried := flattenAccountSettings(got, prior)
	if !carried.LocalAdminAccountEnabled.ValueBool() {
		t.Error("a deprecated name the plan holds must refresh to the wire value")
	}
	if !carried.PayloadConfigured.ValueBool() {
		t.Error("payload_configured must keep the author's value")
	}
	if !carried.HiddenAdminAccount.IsNull() {
		t.Error("a deprecated name the plan never held must stay null")
	}
}

func TestRenames_DataSourceKeepsDeprecatedNamesAlongsideTheNewOnes(t *testing.T) {
	t.Parallel()
	var resp datasource.SchemaResponse
	NewComputerPrestageEnrollmentDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	for _, p := range []struct{ old, new string }{
		{"mandatory", "make_mdm_profile_mandatory"},
		{"mdm_removable", "allow_mdm_profile_removal"},
		{"authentication_prompt", "authentication_message"},
	} {
		oa, ok := resp.Schema.Attributes[p.old]
		if !ok || oa.GetDeprecationMessage() == "" || !strings.Contains(oa.GetDeprecationMessage(), p.new) || !strings.Contains(oa.GetDeprecationMessage(), removalDate) {
			t.Errorf("data source %s must be deprecated in favour of %s", p.old, p.new)
		}
		if _, ok := resp.Schema.Attributes[p.new]; !ok {
			t.Errorf("data source must expose %s", p.new)
		}
	}
}

func TestRenames_StateFromBeforeTheRenameStillPlans(t *testing.T) {
	t.Parallel()
	s := resourceSchema(t)
	prior := stateWith(t, s,
		attrValue{"mandatory", types.BoolValue(true)},
		attrValue{"mdm_removable", types.BoolValue(false)},
		attrValue{"account_settings", &AccountSettingsModel{LocalAdminAccountEnabled: types.BoolValue(true)}},
	)
	cfg := stateWith(t, s,
		attrValue{"mandatory", types.BoolValue(true)},
		attrValue{"mdm_removable", types.BoolValue(false)},
		attrValue{"account_settings", &AccountSettingsModel{LocalAdminAccountEnabled: types.BoolValue(true)}},
	)
	plan := modifyPlan(t, s, cfg, prior)
	if got := getBool(t, plan, "make_mdm_profile_mandatory"); got.IsNull() || !got.ValueBool() {
		t.Errorf("replacement must be filled from the old name, got %s", got)
	}
	if got := getBool(t, plan, "allow_mdm_profile_removal"); got.IsNull() || got.ValueBool() {
		t.Errorf("replacement must carry an explicit false, got %s", got)
	}
	var created types.Bool
	if d := plan.GetAttribute(context.Background(), path.Root("account_settings").AtName("create_managed_local_administrator_account"), &created); d.HasError() || created.IsNull() || !created.ValueBool() {
		t.Errorf("nested replacement must be filled from the old name, got %s %v", created, d)
	}
}
