// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"strings"
)

// uiObjType is the tftypes shape of the user_interaction block, restricted to
// the deferral trio the validator reads.
var uiObjType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"deferral_type":      tftypes.String,
	"deferral_until_utc": tftypes.String,
	"deferral_days":      tftypes.Number,
}}

var rootObjType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"user_interaction": uiObjType,
}}

// attrState is the tri-state a fixture attribute can take.
type attrState int

const (
	stNull attrState = iota
	stUnknown
	stSet
)

// strVal builds a String tftypes value for the given state. The set value is
// the caller-supplied literal.
func strVal(s attrState, set string) tftypes.Value {
	switch s {
	case stUnknown:
		return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	case stSet:
		return tftypes.NewValue(tftypes.String, set)
	default:
		return tftypes.NewValue(tftypes.String, nil)
	}
}

// numVal builds a Number tftypes value for the given state.
func numVal(s attrState, set int64) tftypes.Value {
	switch s {
	case stUnknown:
		return tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	case stSet:
		return tftypes.NewValue(tftypes.Number, set)
	default:
		return tftypes.NewValue(tftypes.Number, nil)
	}
}

// deferralConfig assembles a tfsdk.Config carrying a user_interaction block
// with the three deferral attributes in the requested states, mirroring the
// nesting the validator walks via req.Path.ParentPath().AtName(...).
func deferralConfig(typeVal, untilVal tftypes.Value, daysVal tftypes.Value) tfsdk.Config {
	uiSchema := schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"deferral_type":      schema.StringAttribute{Optional: true},
			"deferral_until_utc": schema.StringAttribute{Optional: true},
			"deferral_days":      schema.Int64Attribute{Optional: true},
		},
	}

	return tfsdk.Config{
		Schema: schema.Schema{Attributes: map[string]schema.Attribute{
			"user_interaction": uiSchema,
		}},
		Raw: tftypes.NewValue(rootObjType, map[string]tftypes.Value{
			"user_interaction": tftypes.NewValue(uiObjType, map[string]tftypes.Value{
				"deferral_type":      typeVal,
				"deferral_until_utc": untilVal,
				"deferral_days":      daysVal,
			}),
		}),
	}
}

// runDeferralValidator drives deferralTypeCompanionsValidator against a config,
// setting req.ConfigValue + req.Path to the user_interaction.deferral_type
// attribute, and returns diagnostic summaries.
func runDeferralValidator(cfg tfsdk.Config, configValue types.String) []string {
	req := validator.StringRequest{
		Path:        path.Root("user_interaction").AtName("deferral_type"),
		ConfigValue: configValue,
		Config:      cfg,
	}
	var resp validator.StringResponse
	deferralTypeCompanionsValidator{}.ValidateString(context.Background(), req, &resp)

	out := make([]string, 0, len(resp.Diagnostics))
	for _, d := range resp.Diagnostics {
		out = append(out, d.Summary())
	}
	return out
}

// TestDeferral_DefersWhenTypeUnknown is the defer-on-unknown regression guard:
// deferral_type unknown (variable-driven) with a known companion set must
// DEFER, not reject the companion as orphaned. See STYLE_GUIDE "Config-time
// validators MUST defer on unknown values".
func TestDeferral_DefersWhenTypeUnknown(t *testing.T) {
	cfg := deferralConfig(
		strVal(stUnknown, ""),
		strVal(stSet, "2027-01-01T01:00:00.000+0000"),
		numVal(stNull, 0),
	)
	if out := runDeferralValidator(cfg, types.StringUnknown()); len(out) != 0 {
		t.Errorf("unknown deferral_type must defer, got %v", out)
	}
}

// TestDeferral_DateUntilUnknownDefers proves date + unknown deferral_until_utc
// defers rather than false-erroring "required".
func TestDeferral_DateUntilUnknownDefers(t *testing.T) {
	cfg := deferralConfig(
		strVal(stSet, "date"),
		strVal(stUnknown, ""),
		numVal(stNull, 0),
	)
	if out := runDeferralValidator(cfg, types.StringValue("date")); len(out) != 0 {
		t.Errorf("date + unknown deferral_until_utc must defer, got %v", out)
	}
}

// TestDeferral_DateUntilNullErrors proves the required-when check still fires
// when deferral_until_utc is genuinely null.
func TestDeferral_DateUntilNullErrors(t *testing.T) {
	cfg := deferralConfig(
		strVal(stSet, "date"),
		strVal(stNull, ""),
		numVal(stNull, 0),
	)
	if out := runDeferralValidator(cfg, types.StringValue("date")); len(out) == 0 {
		t.Error("date + null deferral_until_utc must error")
	}
}

// TestDeferral_DateUntilSetOK proves date + known deferral_until_utc passes.
func TestDeferral_DateUntilSetOK(t *testing.T) {
	cfg := deferralConfig(
		strVal(stSet, "date"),
		strVal(stSet, "2027-01-01T01:00:00.000+0000"),
		numVal(stNull, 0),
	)
	if out := runDeferralValidator(cfg, types.StringValue("date")); len(out) != 0 {
		t.Errorf("date + set deferral_until_utc should pass, got %v", out)
	}
}

// TestDeferral_DurationDaysUnknownDefers proves duration + unknown deferral_days
// defers rather than false-erroring "required" or ">= 1".
func TestDeferral_DurationDaysUnknownDefers(t *testing.T) {
	cfg := deferralConfig(
		strVal(stSet, "duration"),
		strVal(stNull, ""),
		numVal(stUnknown, 0),
	)
	if out := runDeferralValidator(cfg, types.StringValue("duration")); len(out) != 0 {
		t.Errorf("duration + unknown deferral_days must defer, got %v", out)
	}
}

// TestDeferral_DurationDaysNullErrors proves the required-when check still fires
// when deferral_days is genuinely null.
func TestDeferral_DurationDaysNullErrors(t *testing.T) {
	cfg := deferralConfig(
		strVal(stSet, "duration"),
		strVal(stNull, ""),
		numVal(stNull, 0),
	)
	if out := runDeferralValidator(cfg, types.StringValue("duration")); len(out) == 0 {
		t.Error("duration + null deferral_days must error")
	}
}

// TestDeferral_DurationDaysBounds proves the >= 1 rule still works for known
// values: 3 passes, 0 errors.
func TestDeferral_DurationDaysBounds(t *testing.T) {
	okCfg := deferralConfig(
		strVal(stSet, "duration"),
		strVal(stNull, ""),
		numVal(stSet, 3),
	)
	if out := runDeferralValidator(okCfg, types.StringValue("duration")); len(out) != 0 {
		t.Errorf("duration + deferral_days=3 should pass, got %v", out)
	}

	zeroCfg := deferralConfig(
		strVal(stSet, "duration"),
		strVal(stNull, ""),
		numVal(stSet, 0),
	)
	if out := runDeferralValidator(zeroCfg, types.StringValue("duration")); len(out) == 0 {
		t.Error("duration + deferral_days=0 must error (>= 1 rule)")
	}
}

// TestDeferral_NoneForbidsCompanion proves the none case still forbids a present
// companion (forbidden-when checks unchanged).
func TestDeferral_NoneForbidsCompanion(t *testing.T) {
	cfg := deferralConfig(
		strVal(stSet, "none"),
		strVal(stSet, "2027-01-01T01:00:00.000+0000"),
		numVal(stNull, 0),
	)
	if out := runDeferralValidator(cfg, types.StringValue("none")); len(out) == 0 {
		t.Error("none + deferral_until_utc set must error")
	}
}

// localAccountsSchema is the resource schema restricted to the two attributes
// createAccountRequiresHomeValidator reads, in the same spirit as uiObjType
// above: the validator reads attribute by attribute, so a minimal schema
// exercises it exactly as the full one would.
func localAccountsSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"local_accounts": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{Optional: true},
						"home":   schema.StringAttribute{Optional: true},
					},
				},
			},
		},
	}
}

// localAccountsConfig builds a config holding one local_accounts entry per
// (action, home) pair. A nil home is the attribute left out.
func localAccountsConfig(ctx context.Context, t *testing.T, entries [][2]*string) tfsdk.Config {
	t.Helper()
	s := localAccountsSchema()
	elementType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"action": tftypes.String,
		"home":   tftypes.String,
	}}
	elements := make([]tftypes.Value, 0, len(entries))
	for _, entry := range entries {
		value := func(in *string) tftypes.Value {
			if in == nil {
				return tftypes.NewValue(tftypes.String, nil)
			}
			return tftypes.NewValue(tftypes.String, *in)
		}
		elements = append(elements, tftypes.NewValue(elementType, map[string]tftypes.Value{
			"action": value(entry[0]),
			"home":   value(entry[1]),
		}))
	}
	raw := tftypes.NewValue(s.Type().TerraformType(ctx).(tftypes.Object), map[string]tftypes.Value{
		"local_accounts": tftypes.NewValue(tftypes.List{ElementType: elementType}, elements),
	})
	return tfsdk.Config{Schema: s, Raw: raw}
}

// TestCreateAccountRequiresHome pins the plan-time replacement for Jamf Pro's
// `409 Problem with create account fields`, which names no field.
func TestCreateAccountRequiresHome(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name      string
		entries   [][2]*string
		wantError bool
	}{
		{
			name:      "create without home is refused",
			entries:   [][2]*string{{new("Create"), nil}},
			wantError: true,
		},
		{
			name:    "create with home is accepted",
			entries: [][2]*string{{new("Create"), new("/Users/svc")}},
		},
		{
			name:    "a non-create action needs no home",
			entries: [][2]*string{{new("Reset"), nil}},
		},
		{
			name:    "an entry with no action at all is left alone",
			entries: [][2]*string{{nil, nil}},
		},
		{
			// The validator reads each element, so a good entry must not mask a
			// bad one sitting behind it in the list.
			name:      "a later create without home is still refused",
			entries:   [][2]*string{{new("Reset"), nil}, {new("Create"), nil}},
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := resource.ValidateConfigRequest{Config: localAccountsConfig(ctx, t, tc.entries)}
			resp := resource.ValidateConfigResponse{}
			createAccountRequiresHomeValidator{}.ValidateResource(ctx, req, &resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tc.wantError, resp.Diagnostics)
			}
			if !tc.wantError {
				return
			}
			if got := resp.Diagnostics.Errors()[0].Summary(); got != "home required for a Create account action" {
				t.Errorf("summary = %q", got)
			}
		})
	}
}

// TestCreateAccountRequiresHome_NoAccountsIsFine is the control: the validator
// must not fire on a policy that declares no local accounts at all.
func TestCreateAccountRequiresHome_NoAccountsIsFine(t *testing.T) {
	ctx := context.Background()
	s := localAccountsSchema()
	raw := tftypes.NewValue(s.Type().TerraformType(ctx).(tftypes.Object), map[string]tftypes.Value{
		"local_accounts": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"action": tftypes.String,
			"home":   tftypes.String,
		}}}, nil),
	})

	resp := resource.ValidateConfigResponse{}
	createAccountRequiresHomeValidator{}.ValidateResource(ctx,
		resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: s, Raw: raw}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("no local accounts must not error: %v", resp.Diagnostics)
	}
}

func TestFixDiskPermissionsDead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		version string
		want    bool
	}{
		{"11.31.0", false},
		{"11.32.0", false},
		{"11.33.0", false},
		{"11.33.9", false},
		{"11.34.0", true},
		{"11.34.0-t1793383479089", true},
		{"11.35.1", true},
		{"12.0.0", true},
		{"", false},
		{"not-a-version", false},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			t.Parallel()
			if got := fixDiskPermissionsDead(tc.version); got != tc.want {
				t.Errorf("fixDiskPermissionsDead(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

func TestFixDiskPermissionsDiagnosticNamesAttributeAndVersion(t *testing.T) {
	t.Parallel()
	d := fixDiskPermissionsDiagnostic("11.34.0")
	if !strings.Contains(d.Detail(), "11.34.0") || !strings.Contains(d.Detail(), "fix_disk_permissions") {
		t.Errorf("diagnostic must name the version and the attribute, got %q", d.Detail())
	}
}
