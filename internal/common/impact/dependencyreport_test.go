// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// These tests drive ReportDependencyPlan through real framework request values, the
// way each of the six dependency resources calls it. The lifecycle branching is the
// whole point: the hook decides whether the tenant gets swept at all, and a sweep is
// the most expensive thing impact reporting does.

// dependencyPlanModel is a trivial dependency resource model: the server-assigned id
// the sweep looks up, a name for the prose, and a payload attribute so a diff can be
// expressed without touching either.
type dependencyPlanModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Payload types.String `tfsdk:"payload"`
}

func dependencyPlanSchema() rschema.Schema {
	return rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"id":      rschema.StringAttribute{Computed: true},
			"name":    rschema.StringAttribute{Optional: true},
			"payload": rschema.StringAttribute{Optional: true},
		},
	}
}

var dependencyPlanObjectType = tftypes.Object{
	AttributeTypes: map[string]tftypes.Type{
		"id":      tftypes.String,
		"name":    tftypes.String,
		"payload": tftypes.String,
	},
}

// dependencyPlanRaw builds a fully-known raw object for the trivial schema.
func dependencyPlanRaw(id, name, payload string) tftypes.Value {
	return tftypes.NewValue(dependencyPlanObjectType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, id),
		"name":    tftypes.NewValue(tftypes.String, name),
		"payload": tftypes.NewValue(tftypes.String, payload),
	})
}

// dependencyPlanUnknownIDRaw is the planned value for an update whose id the provider
// cannot promise to keep — the shape that makes reading the id from state rather than
// from the plan load-bearing.
func dependencyPlanUnknownIDRaw(name, payload string) tftypes.Value {
	return tftypes.NewValue(dependencyPlanObjectType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, name),
		"payload": tftypes.NewValue(tftypes.String, payload),
	})
}

// dependencyPlanNullRaw is the null object carried for the missing side of a create
// (no prior state) or a destroy (no planned configuration).
func dependencyPlanNullRaw() tftypes.Value {
	return tftypes.NewValue(dependencyPlanObjectType, nil)
}

func dependencyPlanIdentify(_ context.Context, m *dependencyPlanModel) (string, string) {
	return m.ID.ValueString(), m.Name.ValueString()
}

// dependencyPlanSource is one policy running script 500, scoped to Group One.
func dependencyPlanSource() *stubPolicySource {
	return &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1)),
		},
	}
}

// runReportDependencyPlan drives the shared hook exactly as a resource's ModifyPlan
// would, and returns both the diagnostics and the stub so a test can assert on what
// the tenant was asked for.
func runReportDependencyPlan(t *testing.T, cache *Cache, state, plan tftypes.Value) diag.Diagnostics {
	t.Helper()
	sch := dependencyPlanSchema()
	req := resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: sch, Raw: state},
		Plan:  tfsdk.Plan{Schema: sch, Raw: plan},
	}
	resp := &resource.ModifyPlanResponse{}
	ReportDependencyPlan(context.Background(), req, resp, DependencyPlanReport{
		Cache: cache,
		Path:  path.Empty(),
		Kind:  DependencyScript,
	}, dependencyPlanIdentify)
	return resp.Diagnostics
}

func TestReportDependencyPlan_CreateNeverSweeps(t *testing.T) {
	t.Parallel()
	// The laziness is the feature's entire cost story: the sweep reads every policy in
	// the tenant, and nothing can reference an id the tenant has not issued yet. A
	// create must therefore return before touching the policy source at all.
	src := dependencyPlanSource()
	diags := runReportDependencyPlan(t, depCache(twoGroupTenant(), src),
		dependencyPlanNullRaw(), dependencyPlanRaw("500", "my script", "a"))
	if len(diags) != 0 {
		t.Errorf("a create must not alert, got %v", diags)
	}
	if src.listCalls != 0 || len(src.getCalls) != 0 {
		t.Errorf("a create swept the tenant: listCalls=%d policyReads=%d", src.listCalls, len(src.getCalls))
	}
}

func TestReportDependencyPlan_UpdateWithADiffAlerts(t *testing.T) {
	t.Parallel()
	src := dependencyPlanSource()
	diags := runReportDependencyPlan(t, depCache(twoGroupTenant(), src),
		dependencyPlanRaw("500", "my script", "a"), dependencyPlanRaw("500", "my script", "b"))
	if len(diags) != 1 {
		t.Fatalf("a payload edit must alert exactly once, got %d: %v", len(diags), diags)
	}
	if diags[0].Severity() != diag.SeverityWarning {
		t.Errorf("impact alerts are advisory, got severity %v", diags[0].Severity())
	}
	summary := diags[0].Summary()
	if !strings.Contains(summary, "this script affects") || !strings.Contains(summary, "via 1 policy") {
		t.Errorf("summary = %q, want the update wording with the policy count", summary)
	}
	if !strings.Contains(diags[0].Detail(), "Alpha") {
		t.Errorf("detail must name the using policy:\n%s", diags[0].Detail())
	}
}

func TestReportDependencyPlan_UpdateWithNoDiffIsSilentAndDoesNotSweep(t *testing.T) {
	t.Parallel()
	// Terraform calls ModifyPlan for every resource in the configuration, so an
	// unchanged object must cost nothing — one sweep triggered by an untouched script
	// would read the whole tenant for an alert nobody needs.
	src := dependencyPlanSource()
	same := dependencyPlanRaw("500", "my script", "a")
	diags := runReportDependencyPlan(t, depCache(twoGroupTenant(), src), same, same)
	if len(diags) != 0 {
		t.Errorf("an unchanged resource must not alert, got %v", diags)
	}
	if src.listCalls != 0 {
		t.Errorf("an unchanged resource swept the tenant: listCalls=%d", src.listCalls)
	}
}

func TestReportDependencyPlan_DestroyUsesTheDeleteWording(t *testing.T) {
	t.Parallel()
	src := dependencyPlanSource()
	diags := runReportDependencyPlan(t, depCache(twoGroupTenant(), src),
		dependencyPlanRaw("500", "my script", "a"), dependencyPlanNullRaw())
	if len(diags) != 1 {
		t.Fatalf("a destroy must alert exactly once, got %d: %v", len(diags), diags)
	}
	if s := diags[0].Summary(); !strings.Contains(s, "removing this script affects") {
		t.Errorf("summary = %q, want the delete wording", s)
	}
}

func TestReportDependencyPlan_BothSidesNullIsSilent(t *testing.T) {
	t.Parallel()
	src := dependencyPlanSource()
	if diags := runReportDependencyPlan(t, depCache(twoGroupTenant(), src),
		dependencyPlanNullRaw(), dependencyPlanNullRaw()); len(diags) != 0 {
		t.Errorf("neither state nor plan must alert, got %v", diags)
	}
	if src.listCalls != 0 {
		t.Errorf("listCalls = %d, want 0", src.listCalls)
	}
}

func TestReportDependencyPlan_ReadsTheIDFromPriorState(t *testing.T) {
	t.Parallel()
	// The id is server-assigned, so on an update that replaces the object the planned
	// value is unknown and only prior state carries it. Reading the plan instead would
	// silence the alert on exactly the change with the widest reach.
	src := dependencyPlanSource()
	diags := runReportDependencyPlan(t, depCache(twoGroupTenant(), src),
		dependencyPlanRaw("500", "my script", "a"), dependencyPlanUnknownIDRaw("my script", "b"))
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Detail(), "Alpha") {
		t.Errorf("the alert must have resolved the prior-state id:\n%s", diags[0].Detail())
	}
}

func TestReportDependencyPlan_DisabledReportingIsSilent(t *testing.T) {
	t.Parallel()
	// Resources wire the hook unconditionally, so both disabled shapes — no cache at
	// all, and a cache built without dependency support — must produce nothing on every
	// lifecycle path.
	state, plan := dependencyPlanRaw("500", "my script", "a"), dependencyPlanRaw("500", "my script", "b")
	if diags := runReportDependencyPlan(t, nil, state, plan); len(diags) != 0 {
		t.Errorf("a nil cache must disable reporting, got %v", diags)
	}
	if diags := runReportDependencyPlan(t, NewCache(twoGroupTenant()), state, plan); len(diags) != 0 {
		t.Errorf("a cache without a policy source must disable reporting, got %v", diags)
	}
	if diags := runReportDependencyPlan(t, nil, state, dependencyPlanNullRaw()); len(diags) != 0 {
		t.Errorf("a nil cache must disable reporting on destroys too, got %v", diags)
	}
}

func TestReportDependencyPlan_UndecodableStateIsSilent(t *testing.T) {
	t.Parallel()
	// A model that will not decode is the resource's own problem to report; adding a
	// second diagnostic here would blame impact reporting for it. The schema used for
	// the request deliberately disagrees with the model the hook is instantiated with.
	src := dependencyPlanSource()
	sch := rschema.Schema{
		Attributes: map[string]rschema.Attribute{
			"other": rschema.StringAttribute{Optional: true},
		},
	}
	otherType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"other": tftypes.String}}
	raw := func(v string) tftypes.Value {
		return tftypes.NewValue(otherType, map[string]tftypes.Value{
			"other": tftypes.NewValue(tftypes.String, v),
		})
	}
	req := resource.ModifyPlanRequest{
		State: tfsdk.State{Schema: sch, Raw: raw("a")},
		Plan:  tfsdk.Plan{Schema: sch, Raw: raw("b")},
	}
	resp := &resource.ModifyPlanResponse{}
	ReportDependencyPlan(context.Background(), req, resp, DependencyPlanReport{
		Cache: depCache(twoGroupTenant(), src),
		Path:  path.Empty(),
		Kind:  DependencyScript,
	}, dependencyPlanIdentify)
	if len(resp.Diagnostics) != 0 {
		t.Errorf("an undecodable model must produce no impact diagnostics, got %v", resp.Diagnostics)
	}
}
