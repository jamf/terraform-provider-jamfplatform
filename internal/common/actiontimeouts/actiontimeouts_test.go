// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package actiontimeouts_test

import (
	"context"
	"testing"
	"time"

	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var timeoutsType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{"invoke": tftypes.String}}

func configWith(t *testing.T, timeouts tftypes.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	schema := actionschema.Schema{Attributes: actiontimeouts.Add(ctx, nil)}
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

	ctx, cancel := actiontimeouts.Bound(context.Background(), cfg, &diags, 2*time.Minute)
	defer cancel()

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := remaining(t, ctx); got > 2*time.Minute || got < 2*time.Minute-5*time.Second {
		t.Fatalf("deadline in %s, want about 2m", got)
	}
}

func TestBound_UsesConfiguredInvoke(t *testing.T) {
	cfg := configWith(t, tftypes.NewValue(timeoutsType, map[string]tftypes.Value{
		"invoke": tftypes.NewValue(tftypes.String, "30s"),
	}))
	var diags diag.Diagnostics

	ctx, cancel := actiontimeouts.Bound(context.Background(), cfg, &diags, 2*time.Minute)
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
		"invoke": tftypes.NewValue(tftypes.String, "soon"),
	}))
	var diags diag.Diagnostics

	_, cancel := actiontimeouts.Bound(context.Background(), cfg, &diags, time.Minute)
	defer cancel()

	if !diags.HasError() {
		t.Fatal("a malformed duration must be reported, not silently replaced by the default")
	}
}

func TestBound_CancelFuncNeverNil(t *testing.T) {
	cfg := configWith(t, tftypes.NewValue(timeoutsType, map[string]tftypes.Value{
		"invoke": tftypes.NewValue(tftypes.String, "soon"),
	}))
	var diags diag.Diagnostics

	_, cancel := actiontimeouts.Bound(context.Background(), cfg, &diags, time.Minute)
	if cancel == nil {
		t.Fatal("cancel is nil on the error path; callers defer it unconditionally")
	}
	cancel()
}

func TestAdd_CopiesRatherThanMutates(t *testing.T) {
	base := map[string]actionschema.Attribute{"x": actionschema.StringAttribute{Optional: true}}

	got := actiontimeouts.Add(context.Background(), base)

	if _, ok := base[actiontimeouts.AttributeName]; ok {
		t.Fatal("Add mutated the caller's map")
	}
	if _, ok := got[actiontimeouts.AttributeName]; !ok {
		t.Fatal("Add did not add the timeouts attribute")
	}
	if _, ok := got["x"]; !ok {
		t.Fatal("Add dropped an existing attribute")
	}
}
