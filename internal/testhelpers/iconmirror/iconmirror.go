// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package iconmirror holds the shared plan-modifier assertion for attributes
// Jamf Pro derives from a Self Service icon id. It is a package of its own
// rather than part of testhelpers because testhelpers imports the provider under
// the acceptance tag, which would make every in-package unit test that uses it
// an import cycle.
package iconmirror

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// AssertPlan checks an attribute that Jamf Pro derives from the Self
// Service icon id (self_service.self_service_icon.<name>): the icon's CDN uri
// or stored filename. It is shared by every resource that carries that block,
// so the three of them cannot drift apart again (issue #468).
//
// The attribute under test is the real one taken from the resource's schema,
// placed into a minimal schema of the same shape, so a wrong path in the
// resource's modifier fails here rather than only against a live tenant.
//
// It asserts the schema shape (Computed only, because Jamf Pro ignores an
// authored value and derives both from the id) and then runs the plan
// modifiers over the transitions that matter:
//
//   - id unchanged: the prior value carries forward, so a plan that edits
//     something else does not show the attribute as "known after apply".
//   - id repointed: the attribute goes Unknown, so apply may return the new
//     icon's value. Carrying the old one is the "inconsistent result after
//     apply" failure.
//   - id unknown, as when the icon resource is replaced in the same apply:
//     Unknown, for the same reason.
//   - icon block omitted from config while state holds one: the prior value
//     carries forward.
//   - icon block declared where state has none: Unknown.
//   - create: Unknown.
func AssertPlan(t *testing.T, name string, attr schema.Attribute) {
	t.Helper()

	mirror, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a string attribute: %T", name, attr)
	}
	if !mirror.Computed || mirror.Optional || mirror.Required {
		t.Errorf("%s: Computed=%v Optional=%v Required=%v, want Computed only: Jamf Pro ignores an authored value", name, mirror.Computed, mirror.Optional, mirror.Required)
	}
	if len(mirror.PlanModifiers) == 0 {
		t.Fatalf("%s has no plan modifier: it would be Unknown in every plan that changes anything else", name)
	}

	ctx := context.Background()
	sch := schema.Schema{Attributes: map[string]schema.Attribute{
		"self_service": schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"self_service_icon": schema.SingleNestedAttribute{
					Optional: true,
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{Optional: true, Computed: true},
						name: mirror,
					},
				},
			},
		},
	}}

	iconType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"id": tftypes.String, name: tftypes.String}}
	ssType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"self_service_icon": iconType}}
	rootType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"self_service": ssType}}

	str := func(v string) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }
	nullStr := tftypes.NewValue(tftypes.String, nil)
	unknownStr := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	icon := func(id, derived tftypes.Value) tftypes.Value {
		return tftypes.NewValue(iconType, map[string]tftypes.Value{"id": id, name: derived})
	}
	nullIcon := tftypes.NewValue(iconType, nil)
	root := func(block tftypes.Value) tftypes.Value {
		return tftypes.NewValue(rootType, map[string]tftypes.Value{
			"self_service": tftypes.NewValue(ssType, map[string]tftypes.Value{"self_service_icon": block}),
		})
	}

	const prior = "carried"
	for _, tc := range []struct {
		name        string
		config      tftypes.Value
		state       tftypes.Value
		stateIsNull bool
		want        types.String
	}{
		{
			name:   "id unchanged: prior value carried",
			config: root(icon(str("5162"), nullStr)),
			state:  root(icon(str("5162"), str(prior))),
			want:   types.StringValue(prior),
		},
		{
			name:   "id repointed: Unknown",
			config: root(icon(str("5163"), nullStr)),
			state:  root(icon(str("5162"), str(prior))),
			want:   types.StringUnknown(),
		},
		{
			name:   "id unknown, icon replaced in the same apply: Unknown",
			config: root(icon(unknownStr, nullStr)),
			state:  root(icon(str("5162"), str(prior))),
			want:   types.StringUnknown(),
		},
		{
			name:   "icon block omitted, state holds one: prior value carried",
			config: root(nullIcon),
			state:  root(icon(str("5162"), str(prior))),
			want:   types.StringValue(prior),
		},
		{
			name:   "icon block declared where state has none: Unknown",
			config: root(icon(str("5162"), nullStr)),
			state:  root(nullIcon),
			want:   types.StringUnknown(),
		},
		{
			name:        "create: Unknown",
			config:      root(icon(str("5162"), nullStr)),
			state:       root(nullIcon),
			stateIsNull: true,
			want:        types.StringUnknown(),
		},
	} {
		t.Run(name+"/"+tc.name, func(t *testing.T) {
			planned := root(icon(unknownStr, unknownStr))
			state := tfsdk.State{Schema: sch, Raw: tc.state}
			if tc.stateIsNull {
				state.Raw = tftypes.NewValue(rootType, nil)
			}

			req := planmodifier.StringRequest{
				Path:       path.Root("self_service").AtName("self_service_icon").AtName(name),
				Config:     tfsdk.Config{Schema: sch, Raw: tc.config},
				Plan:       tfsdk.Plan{Schema: sch, Raw: planned},
				State:      state,
				StateValue: types.StringValue(prior),
				PlanValue:  types.StringUnknown(),
			}
			if tc.stateIsNull {
				req.StateValue = types.StringNull()
			}

			resp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
			for _, m := range mirror.PlanModifiers {
				m.PlanModifyString(ctx, req, resp)
				req.PlanValue = resp.PlanValue
			}

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(tc.want) {
				t.Errorf("plan value = %#v, want %#v", resp.PlanValue, tc.want)
			}
		})
	}
}
