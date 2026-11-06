// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"bufio"
	"context"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/deprecations"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/prestagealias"
)

type M = MobileDevicePrestageEnrollmentResourceModel

func renamedSchema() schema.Schema {
	attrs := map[string]schema.Attribute{}
	addRenamedAttributes(attrs)
	return schema.Schema{Attributes: attrs}
}

func fullSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := NewMobileDevicePrestageEnrollmentResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func TestRenames_EveryDeprecatedAttributeNamesItsReplacementAndTheRemovalDate(t *testing.T) {
	t.Parallel()
	s := fullSchema(t)
	for _, r := range boolRenames() {
		oldA, ok := s.Attributes[r.oldName].(schema.BoolAttribute)
		if !ok {
			t.Fatalf("%s: not a bool attribute in the schema", r.oldName)
		}
		if _, ok := s.Attributes[r.newName].(schema.BoolAttribute); !ok {
			t.Fatalf("%s: replacement missing", r.newName)
		}
		for _, text := range []string{oldA.DeprecationMessage, oldA.MarkdownDescription} {
			if !strings.Contains(text, "`"+r.newName+"`") || !strings.Contains(text, "2027-02-03") {
				t.Errorf("%s: %q must name %s and 2027-02-03", r.oldName, text, r.newName)
			}
		}
		if !strings.HasPrefix(oldA.MarkdownDescription, "~> ") {
			t.Errorf("%s: description must open with the deprecation callout", r.oldName)
		}
		if !oldA.Optional || !oldA.Computed {
			t.Errorf("%s: deprecated alias must stay Optional and Computed", r.oldName)
		}
		newA := s.Attributes[r.newName].(schema.BoolAttribute)
		if newA.DeprecationMessage != "" || !newA.Optional || !newA.Computed {
			t.Errorf("%s: replacement must be Optional, Computed and not deprecated", r.newName)
		}
		if !strings.HasPrefix(newA.MarkdownDescription, "**\""+r.label+"\"**") {
			t.Errorf("%s: description must open with the admin UI label %q, got %q", r.newName, r.label, newA.MarkdownDescription)
		}
	}
	a := authenticationMessageRename()
	oldA := s.Attributes[a.oldName].(schema.StringAttribute)
	if !strings.Contains(oldA.DeprecationMessage, "`authentication_message`") || !strings.Contains(oldA.DeprecationMessage, "2027-02-03") {
		t.Errorf("authentication_prompt message = %q", oldA.DeprecationMessage)
	}
}

func TestRenames_EveryDeprecationHasAParsableMarker(t *testing.T) {
	t.Parallel()
	f, err := os.Open("deprecations.go")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	found := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m, detail, ok := deprecations.Parse(sc.Text())
		if !ok {
			continue
		}
		if detail != "" {
			t.Errorf("malformed marker: %s", detail)
		}
		if m.RemoveAfter != "2027-02-03" {
			t.Errorf("marker %s removes after %s", m.ReplacedBy, m.RemoveAfter)
		}
		found[m.ReplacedBy] = true
	}
	for _, r := range boolRenames() {
		if !found[r.newName] {
			t.Errorf("no marker replaced-by=%s", r.newName)
		}
	}
	if !found["authentication_message"] {
		t.Error("no marker replaced-by=authentication_message")
	}
}

func TestRenames_PairsMatchTheTable(t *testing.T) {
	t.Parallel()
	pairs := renamePairs()
	if len(pairs) != len(boolRenames())+1 {
		t.Fatalf("pairs = %d, want %d", len(pairs), len(boolRenames())+1)
	}
	inverted := 0
	for _, p := range pairs {
		if p.Invert {
			inverted++
			if p.Old != "mdm_removable" || p.New != "prevent_unenrollment" {
				t.Errorf("unexpected inverted pair %+v", p)
			}
		}
	}
	if inverted != 1 {
		t.Errorf("inverted pairs = %d, want 1", inverted)
	}
}

func TestRenames_SettingBothNamesIsAConfigurationError(t *testing.T) {
	t.Parallel()
	s := fullSchema(t)
	conflicts := func(name, other string, bothSet bool) int {
		attrs := map[string]any{name: true, other: nullBool{}}
		if bothSet {
			attrs[other] = true
		}
		cfg := buildConfig(t, attrs)
		a := s.Attributes[name].(schema.BoolAttribute)
		req := validator.BoolRequest{
			Path:           path.Root(name),
			PathExpression: path.MatchRoot(name),
			ConfigValue:    types.BoolValue(true),
			Config:         cfg,
		}
		var resp validator.BoolResponse
		a.Validators[0].ValidateBool(context.Background(), req, &resp)
		return len(resp.Diagnostics)
	}
	for _, r := range boolRenames() {
		if conflicts(r.oldName, r.newName, true) == 0 {
			t.Errorf("%s with %s must conflict", r.oldName, r.newName)
		}
		if conflicts(r.oldName, r.newName, false) != 0 {
			t.Errorf("%s alone must not conflict", r.oldName)
		}
	}
}

func TestResolveRenames_OldOnlyNewOnlyAndInverted(t *testing.T) {
	t.Parallel()
	m := M{Mandatory: types.BoolValue(true), MdmRemovable: types.BoolValue(true), AuthenticationPrompt: types.StringValue("hi")}
	resolveRenames(&m)
	if !m.MakeMdmProfileMandatory.ValueBool() {
		t.Error("deprecated mandatory must fold into the replacement")
	}
	if m.PreventUnenrollment.ValueBool() {
		t.Error("mdm_removable = true is prevent_unenrollment = false")
	}
	if m.AuthenticationMessage.ValueString() != "hi" {
		t.Error("authentication_prompt must fold into authentication_message")
	}

	m = M{PreventUnenrollment: types.BoolValue(true), MdmRemovable: types.BoolValue(true)}
	resolveRenames(&m)
	if !m.PreventUnenrollment.ValueBool() {
		t.Error("the replacement wins over the deprecated spelling")
	}

	m = M{MdmRemovable: types.BoolValue(false)}
	resolveRenames(&m)
	if !m.PreventUnenrollment.ValueBool() {
		t.Error("mdm_removable = false is prevent_unenrollment = true")
	}
}

func TestBuildInputs_DeprecatedAndReplacementNamesReachTheSameWireFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	oldPlan := M{
		Mandatory: types.BoolValue(true), MdmRemovable: types.BoolValue(true), RequireAuthentication: types.BoolValue(true),
		Supervised: types.BoolValue(true), AutoAdvanceSetup: types.BoolValue(true), ConfigureDeviceBeforeSetupAssistant: types.BoolValue(true),
		InstallAppsDuringEnrollment: types.BoolValue(true), DefaultPrestage: types.BoolValue(true), SendTimezone: types.BoolValue(true),
		MultiUser: types.BoolValue(true), RtsEnabled: types.BoolValue(true), KeepExistingSiteMembership: types.BoolValue(true),
		KeepExistingLocationInformation: types.BoolValue(true), AuthenticationPrompt: types.StringValue("msg"),
	}
	newPlan := M{
		MakeMdmProfileMandatory: types.BoolValue(true), PreventUnenrollment: types.BoolValue(false), RequireCredentialsForEnrollment: types.BoolValue(true),
		SuperviseDevicesWithIos12OrEarlier: types.BoolValue(true), AutomaticallyAdvanceThroughSetupAssistant: types.BoolValue(true),
		InstallConfigurationProfilesBeforeSetupAssistant: types.BoolValue(true), InstallManagedAppsBeforeSetupAssistant: types.BoolValue(true),
		AutomaticallyAssignNewDevices: types.BoolValue(true), SetTimeZone: types.BoolValue(true), EnableSharedIPad: types.BoolValue(true),
		AllowReturnToService: types.BoolValue(true), UseExistingSiteMembership: types.BoolValue(true),
		UseExistingLocationInformation: types.BoolValue(true), AuthenticationMessage: types.StringValue("msg"),
	}
	for name, plan := range map[string]M{"deprecated": oldPlan, "replacement": newPlan} {
		post, d := buildPostInput(ctx, plan, M{})
		if d.HasError() {
			t.Fatal(d)
		}
		put, d := buildPutInput(ctx, plan, M{})
		if d.HasError() {
			t.Fatal(d)
		}
		checks := []struct {
			field string
			post  bool
			put   bool
		}{
			{"Mandatory", post.Mandatory, put.Mandatory},
			{"MDMRemovable", post.MDMRemovable, put.MDMRemovable},
			{"RequireAuthentication", post.RequireAuthentication, put.RequireAuthentication},
			{"Supervised", post.Supervised, put.Supervised},
			{"AutoAdvanceSetup", post.AutoAdvanceSetup, put.AutoAdvanceSetup},
			{"ConfigureDeviceBeforeSetupAssistant", post.ConfigureDeviceBeforeSetupAssistant, put.ConfigureDeviceBeforeSetupAssistant},
			{"DefaultPrestage", post.DefaultPrestage, put.DefaultPrestage},
			{"SendTimezone", post.SendTimezone, put.SendTimezone},
			{"MultiUser", post.MultiUser, put.MultiUser},
			{"KeepExistingSiteMembership", post.KeepExistingSiteMembership, put.KeepExistingSiteMembership},
			{"KeepExistingLocationInformation", post.KeepExistingLocationInformation, put.KeepExistingLocationInformation},
			{"RtsEnabled", deref(post.RtsEnabled), deref(put.RtsEnabled)},
			{"InstallAppsDuringEnrollment", deref(post.InstallAppsDuringEnrollment), deref(put.InstallAppsDuringEnrollment)},
		}
		for _, c := range checks {
			if !c.post || !c.put {
				t.Errorf("%s: %s must reach the wire as true (post=%v put=%v)", name, c.field, c.post, c.put)
			}
		}
		if post.AuthenticationPrompt != "msg" || put.AuthenticationPrompt != "msg" {
			t.Errorf("%s: authentication message lost", name)
		}
	}
}

func deref(b *bool) bool { return b != nil && *b }

func TestBuildInputs_PreventUnenrollmentIsTheOppositeOfMdmRemovable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name      string
		plan      M
		removable bool
	}{
		{"prevent true", M{PreventUnenrollment: types.BoolValue(true)}, false},
		{"prevent false", M{PreventUnenrollment: types.BoolValue(false)}, true},
		{"deprecated true", M{MdmRemovable: types.BoolValue(true)}, true},
		{"deprecated false", M{MdmRemovable: types.BoolValue(false)}, false},
		{"neither set sends false as before the rename", M{PreventUnenrollment: types.BoolUnknown()}, false},
	}
	for _, tc := range cases {
		post, _ := buildPostInput(ctx, tc.plan, M{})
		put, _ := buildPutInput(ctx, tc.plan, M{})
		if post.MDMRemovable != tc.removable || put.MDMRemovable != tc.removable {
			t.Errorf("%s: MDMRemovable post=%v put=%v, want %v", tc.name, post.MDMRemovable, put.MDMRemovable, tc.removable)
		}
	}
}

func wireFor(got *pro.GetMobileDevicePrestageV3) map[string]bool {
	return map[string]bool{
		"mandatory": got.Mandatory, "mdm_removable": got.MDMRemovable, "require_authentication": got.RequireAuthentication,
		"supervised": got.Supervised, "auto_advance_setup": got.AutoAdvanceSetup,
		"configure_device_before_setup_assistant": got.ConfigureDeviceBeforeSetupAssistant,
		"install_apps_during_enrollment":          got.InstallAppsDuringEnrollment, "default_prestage": got.DefaultPrestage,
		"send_timezone": got.SendTimezone, "multi_user": got.MultiUser, "rts_enabled": got.RtsEnabled,
		"keep_existing_site_membership":      got.KeepExistingSiteMembership,
		"keep_existing_location_information": got.KeepExistingLocationInformation,
	}
}

func TestAssignRenamed_ImportAndStateWithoutHistoryNeverProduceTheDeprecatedNames(t *testing.T) {
	t.Parallel()
	got := &pro.GetMobileDevicePrestageV3{Mandatory: true, MDMRemovable: false, AuthenticationPrompt: "x"}
	m := M{}
	assignRenamed(&m, wireFor(got), got.AuthenticationPrompt)
	if !m.MakeMdmProfileMandatory.ValueBool() || !m.PreventUnenrollment.ValueBool() || m.AuthenticationMessage.ValueString() != "x" {
		t.Errorf("replacements must take the wire values: %+v", m)
	}
	for _, r := range boolRenames() {
		if !r.oldField(&m).IsNull() {
			t.Errorf("%s must stay null on import, got %s", r.oldName, r.oldField(&m))
		}
	}
	if !m.AuthenticationPrompt.IsNull() {
		t.Errorf("authentication_prompt must stay null on import")
	}
}

func TestAssignRenamed_StateHoldingTheDeprecatedNameKeepsItInSync(t *testing.T) {
	t.Parallel()
	got := &pro.GetMobileDevicePrestageV3{Mandatory: true, MDMRemovable: false, AuthenticationPrompt: "x"}
	m := M{Mandatory: types.BoolValue(false), MdmRemovable: types.BoolValue(true), AuthenticationPrompt: types.StringValue("old")}
	assignRenamed(&m, wireFor(got), got.AuthenticationPrompt)
	if !m.Mandatory.ValueBool() || m.MdmRemovable.ValueBool() || m.AuthenticationPrompt.ValueString() != "x" {
		t.Errorf("deprecated attributes must follow the wire: %+v", m)
	}
	if !m.PreventUnenrollment.ValueBool() {
		t.Errorf("mdm_removable = false on the wire is prevent_unenrollment = true")
	}
}

func reconcileFixture(t *testing.T, cfg, prior map[string]attrVal) (planAfter tfsdk.Plan) {
	t.Helper()
	ctx := context.Background()
	s := renamedSchema()
	typ := s.Type().TerraformType(ctx)
	mk := func(vals map[string]attrVal) tftypes.Value {
		obj := map[string]tftypes.Value{}
		for name, a := range s.Attributes {
			obj[name] = tftypes.NewValue(a.GetType().TerraformType(ctx), nil)
		}
		for k, v := range vals {
			obj[k] = v.tf()
		}
		return tftypes.NewValue(typ, obj)
	}
	planVals := map[string]attrVal{}
	maps.Copy(planVals, cfg)
	plan := tfsdk.Plan{Schema: s, Raw: mk(planVals)}
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: s, Raw: mk(cfg)},
		Plan:   plan,
		State:  tfsdk.State{Schema: s, Raw: tftypes.NewValue(typ, nil)},
	}
	if prior != nil {
		req.State = tfsdk.State{Schema: s, Raw: mk(prior)}
	}
	resp := &resource.ModifyPlanResponse{Plan: plan}
	prestagealias.ReconcilePlan(ctx, req, resp, renamePairs())
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp.Plan
}

type attrVal struct {
	b *bool
	s *string
}

func (a attrVal) tf() tftypes.Value {
	switch {
	case a.s != nil:
		return tftypes.NewValue(tftypes.String, *a.s)
	case a.b != nil:
		return tftypes.NewValue(tftypes.Bool, *a.b)
	}
	return tftypes.NewValue(tftypes.Bool, nil)
}

func bv(b bool) attrVal   { return attrVal{b: &b} }
func sv(s string) attrVal { return attrVal{s: &s} }

func planBool(t *testing.T, p tfsdk.Plan, name string) types.Bool {
	t.Helper()
	var v types.Bool
	if d := p.GetAttribute(context.Background(), path.Root(name), &v); d.HasError() {
		t.Fatal(d)
	}
	return v
}

func TestReconcilePlan_DeprecatedNameOnlyFillsTheReplacement(t *testing.T) {
	t.Parallel()
	p := reconcileFixture(t, map[string]attrVal{"mdm_removable": bv(true), "mandatory": bv(false)}, nil)
	if planBool(t, p, "prevent_unenrollment").ValueBool() {
		t.Error("mdm_removable = true must plan prevent_unenrollment = false")
	}
	if planBool(t, p, "make_mdm_profile_mandatory").ValueBool() {
		t.Error("mandatory = false must plan make_mdm_profile_mandatory = false")
	}
	if !planBool(t, p, "mdm_removable").ValueBool() {
		t.Error("the configured deprecated value must stay")
	}
}

func TestReconcilePlan_ReplacementOnlyKeepsTheDeprecatedNameAbsent(t *testing.T) {
	t.Parallel()
	p := reconcileFixture(t, map[string]attrVal{"prevent_unenrollment": bv(true)}, nil)
	if !planBool(t, p, "mdm_removable").IsNull() {
		t.Error("a configuration that never used mdm_removable must plan it null")
	}
}

func TestReconcilePlan_StateHoldingTheDeprecatedNameFollowsTheReplacementInverted(t *testing.T) {
	t.Parallel()
	p := reconcileFixture(t,
		map[string]attrVal{"prevent_unenrollment": bv(true)},
		map[string]attrVal{"prevent_unenrollment": bv(false), "mdm_removable": bv(true)})
	if planBool(t, p, "mdm_removable").ValueBool() {
		t.Error("prevent_unenrollment = true must plan the held mdm_removable as false")
	}
}

func TestReconcilePlan_AuthenticationMessageAliasCopiesTheString(t *testing.T) {
	t.Parallel()
	p := reconcileFixture(t, map[string]attrVal{"authentication_prompt": sv("welcome")}, nil)
	var got types.String
	if d := p.GetAttribute(context.Background(), path.Root("authentication_message"), &got); d.HasError() {
		t.Fatal(d)
	}
	if got.ValueString() != "welcome" {
		t.Errorf("authentication_message = %s", got)
	}
}

func TestOldStateWithoutTheNewAttributesPlansTheirValuesFromTheWire(t *testing.T) {
	t.Parallel()
	for _, r := range boolRenames() {
		s := fullSchema(t)
		newA := s.Attributes[r.newName].(schema.BoolAttribute)
		if len(newA.PlanModifiers) == 0 {
			t.Errorf("%s: replacement needs a plan modifier so a refreshed plan stays empty", r.newName)
		}
	}
}
