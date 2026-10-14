// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var comboAttrTypes = map[string]tftypes.Type{
	"all_a":     tftypes.Bool,
	"all_b":     tftypes.Bool,
	"all_c":     tftypes.Bool,
	"buildings": tftypes.Set{ElementType: tftypes.String},
	"classes":   tftypes.Set{ElementType: tftypes.String},
}

var comboSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"all_a":     schema.BoolAttribute{Optional: true},
		"all_b":     schema.BoolAttribute{Optional: true},
		"all_c":     schema.BoolAttribute{Optional: true},
		"buildings": schema.SetAttribute{ElementType: types.StringType, Optional: true},
		"classes":   schema.SetAttribute{ElementType: types.StringType, Optional: true},
	},
}

func boolTF(v *bool) tftypes.Value {
	if v == nil {
		return tftypes.NewValue(tftypes.Bool, nil)
	}
	return tftypes.NewValue(tftypes.Bool, *v)
}

// runCombo drives AllFlagsConflictWith as all_a, with all_b and all_c as the
// sibling flags and buildings / classes as the conflicting sets.
func runCombo(t *testing.T, a types.Bool, aTF, bTF, cTF tftypes.Value, buildings, classes []string) validator.BoolResponse {
	t.Helper()
	cfg := tfsdk.Config{
		Schema: comboSchema,
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: comboAttrTypes}, map[string]tftypes.Value{
			"all_a":     aTF,
			"all_b":     bTF,
			"all_c":     cTF,
			"buildings": stringSetTFValue(buildings),
			"classes":   stringSetTFValue(classes),
		}),
	}
	req := validator.BoolRequest{
		Path:           path.Root("all_a"),
		PathExpression: path.MatchRoot("all_a"),
		Config:         cfg,
		ConfigValue:    a,
	}
	var resp validator.BoolResponse
	AllFlagsConflictWith(
		[]path.Expression{path.MatchRoot("all_b"), path.MatchRoot("all_c")},
		path.MatchRoot("buildings"),
		path.MatchRoot("classes"),
	).ValidateBool(context.Background(), req, &resp)
	return resp
}

func TestAllFlagsConflictWith_AllTrue_PopulatedConflicts_ErrorPerPath(t *testing.T) {
	yes := true
	resp := runCombo(t, types.BoolValue(true), boolTF(&yes), boolTF(&yes), boolTF(&yes), []string{"7"}, []string{"41"})
	if resp.Diagnostics.ErrorsCount() != 2 {
		t.Fatalf("expected 2 errors (buildings, classes), got %d: %v", resp.Diagnostics.ErrorsCount(), resp.Diagnostics)
	}
}

func TestAllFlagsConflictWith_AllTrue_OnlyPopulatedPathsReported(t *testing.T) {
	yes := true
	resp := runCombo(t, types.BoolValue(true), boolTF(&yes), boolTF(&yes), boolTF(&yes), []string{"7"}, nil)
	if resp.Diagnostics.ErrorsCount() != 1 {
		t.Fatalf("expected 1 error, got %d: %v", resp.Diagnostics.ErrorsCount(), resp.Diagnostics)
	}
	for _, d := range resp.Diagnostics.Errors() {
		if ad, ok := d.(interface{ Path() path.Path }); !ok || !ad.Path().Equal(path.Root("buildings")) {
			t.Errorf("expected the error on buildings, got %v", d)
		}
	}
}

func TestAllFlagsConflictWith_AllTrue_EmptyOrNullConflicts_NoViolation(t *testing.T) {
	yes := true
	resp := runCombo(t, types.BoolValue(true), boolTF(&yes), boolTF(&yes), boolTF(&yes), []string{}, nil)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no diagnostics, got: %v", resp.Diagnostics)
	}
}

func TestAllFlagsConflictWith_SiblingNullFalseOrUnknown_NoViolation(t *testing.T) {
	yes, no := true, false
	unknown := tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
	for name, sibling := range map[string]tftypes.Value{
		"null":    boolTF(nil),
		"false":   boolTF(&no),
		"unknown": unknown,
	} {
		t.Run(name, func(t *testing.T) {
			resp := runCombo(t, types.BoolValue(true), boolTF(&yes), boolTF(&yes), sibling, []string{"7"}, []string{"41"})
			if resp.Diagnostics.HasError() {
				t.Errorf("expected no diagnostics with the third flag %s, got: %v", name, resp.Diagnostics)
			}
		})
	}
}

func TestAllFlagsConflictWith_ThisFlagNotTrue_NoViolation(t *testing.T) {
	yes, no := true, false
	for name, v := range map[string]types.Bool{
		"false":   types.BoolValue(false),
		"null":    types.BoolNull(),
		"unknown": types.BoolUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			resp := runCombo(t, v, boolTF(&no), boolTF(&yes), boolTF(&yes), []string{"7"}, []string{"41"})
			if resp.Diagnostics.HasError() {
				t.Errorf("expected no diagnostics, got: %v", resp.Diagnostics)
			}
		})
	}
}

func TestAllFlagsConflictWith_Describer(t *testing.T) {
	v := AllFlagsConflictWith([]path.Expression{path.MatchRoot("all_b")}, path.MatchRoot("buildings"))
	if v.Description(context.Background()) == "" || v.MarkdownDescription(context.Background()) == "" {
		t.Error("expected non-empty descriptions")
	}
}

func TestOmitAllFlagNote_NamesThePairedCategories(t *testing.T) {
	got := OmitAllFlagNote("`computer_ids` or `computer_group_ids`")
	for _, want := range []string{"`computer_ids` or `computer_group_ids`", "turns it off", "outside Terraform"} {
		if !strings.Contains(got, want) {
			t.Errorf("note %q missing %q", got, want)
		}
	}
}
