// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package listtimeouts_test

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

var timeoutsType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{"list": tftypes.String}}

func configWith(t *testing.T, timeouts tftypes.Value) tfsdk.Config {
	t.Helper()
	schema := listschema.Schema{Attributes: listtimeouts.Add(context.Background(), nil)}
	return tfsdk.Config{
		Schema: schema,
		Raw: tftypes.NewValue(
			tftypes.Object{AttributeTypes: map[string]tftypes.Type{"timeouts": timeoutsType}},
			map[string]tftypes.Value{"timeouts": timeouts},
		),
	}
}

func remaining(t *testing.T, ctx context.Context) time.Duration {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("context has no deadline")
	}
	return time.Until(deadline)
}

func TestBound_DefaultsWhenUnset(t *testing.T) {
	cfg := configWith(t, tftypes.NewValue(timeoutsType, nil))
	var diags diag.Diagnostics

	ctx, cancel := listtimeouts.Bound(context.Background(), cfg, &diags, 2*time.Minute)
	defer cancel()

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := remaining(t, ctx); got > 2*time.Minute || got < 2*time.Minute-5*time.Second {
		t.Fatalf("deadline in %s, want about 2m", got)
	}
}

func TestBound_UsesConfiguredList(t *testing.T) {
	cfg := configWith(t, tftypes.NewValue(timeoutsType, map[string]tftypes.Value{
		"list": tftypes.NewValue(tftypes.String, "30s"),
	}))
	var diags diag.Diagnostics

	ctx, cancel := listtimeouts.Bound(context.Background(), cfg, &diags, 2*time.Minute)
	defer cancel()

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := remaining(t, ctx); got > 30*time.Second || got < 25*time.Second {
		t.Fatalf("deadline in %s, want about 30s", got)
	}
}

func TestBound_MalformedDurationIsAnError(t *testing.T) {
	cfg := configWith(t, tftypes.NewValue(timeoutsType, map[string]tftypes.Value{
		"list": tftypes.NewValue(tftypes.String, "soon"),
	}))
	var diags diag.Diagnostics

	_, cancel := listtimeouts.Bound(context.Background(), cfg, &diags, time.Minute)
	if cancel == nil {
		t.Fatal("cancel is nil on the error path; callers defer it unconditionally")
	}
	cancel()

	if !diags.HasError() {
		t.Fatal("a malformed duration must be reported, not silently replaced by the default")
	}
}

func TestBound_ZeroConfigFallsBackToDefault(t *testing.T) {
	var diags diag.Diagnostics

	ctx, cancel := listtimeouts.Bound(context.Background(), tfsdk.Config{}, &diags, time.Minute)
	defer cancel()

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := remaining(t, ctx); got > time.Minute || got < time.Minute-5*time.Second {
		t.Fatalf("deadline in %s, want about 1m", got)
	}
}

func TestAdd_CopiesAndAcceptsNil(t *testing.T) {
	base := map[string]listschema.Attribute{"x": listschema.StringAttribute{Optional: true}}

	got := listtimeouts.Add(context.Background(), base)

	if _, ok := base[listtimeouts.AttributeName]; ok {
		t.Fatal("Add mutated the caller's map")
	}
	if _, ok := got[listtimeouts.AttributeName]; !ok {
		t.Fatal("Add did not add the timeouts attribute")
	}
	if _, ok := got["x"]; !ok {
		t.Fatal("Add dropped an existing attribute")
	}
	if _, ok := listtimeouts.Add(context.Background(), nil)[listtimeouts.AttributeName]; !ok {
		t.Fatal("Add on a nil map did not add the timeouts attribute")
	}
}
