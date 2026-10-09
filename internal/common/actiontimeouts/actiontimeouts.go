// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package actiontimeouts gives every action a context deadline and a `timeouts` attribute.
//
// An action's Invoke receives the raw Terraform context, which carries no deadline of its own, so
// without this a stalled gateway holds `terraform apply` until the operator interrupts it. Resources
// and data sources bound their SDK calls through helpers.ResolveTimeout and a timeouts block; actions
// do the same through terraform-plugin-framework-timeouts/action/timeouts, wrapped here so each
// action is two lines (Add in Schema, Bound at the top of Invoke) and none of them has to carry a
// Timeouts field in its config model.
package actiontimeouts

import (
	"context"
	"maps"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/action/timeouts"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// Value is the type an action's config model gives its `timeouts` field. The framework refuses to
// decode a config object into a struct that lacks a field for one of its attributes, so every model
// that Add's attribute reaches must carry `Timeouts actiontimeouts.Value `+"`tfsdk:\"timeouts\"`"+`.
type Value = timeouts.Value

// AttributeName is the schema key every action exposes its deadline under.
const AttributeName = "timeouts"

// DefaultInvoke is the deadline an action runs under when the practitioner sets none. It is
// deliberately generous: it is a backstop against a hung gateway, not a performance budget, and an
// action that batches many requests (blank pushes, MDM command flushes) spends it across all of them.
const DefaultInvoke = 5 * time.Minute

// Add returns a copy of attrs with the `timeouts` attribute added. It copies rather than mutates
// because several actions build their attributes from a shared helper.
func Add(ctx context.Context, attrs map[string]actionschema.Attribute) map[string]actionschema.Attribute {
	out := make(map[string]actionschema.Attribute, len(attrs)+1)
	maps.Copy(out, attrs)
	out[AttributeName] = timeouts.AttributesWithOpts(ctx, timeouts.Opts{
		InvokeDescription: "How long the action may run before it is cancelled, as a duration such as `30s` or " +
			"`10m`. Defaults to " + strconv.FormatFloat(DefaultInvoke.Minutes(), 'f', -1, 64) + " minutes.",
	})
	return out
}

// Bound derives a context that expires after the configured `invoke` timeout, or after
// defaultTimeout when none is set. The returned cancel is never nil and must be deferred.
//
// The attribute is read by path rather than through the action's config model, so a model needs no
// Timeouts field. A malformed duration is reported on diags and the context is returned unbounded,
// so the caller must check diags.HasError() before using it.
func Bound(ctx context.Context, cfg tfsdk.Config, diags *diag.Diagnostics, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	// A zero Config has no schema to read from, which only a direct unit-test call produces: the
	// framework always supplies one. Fall back to the default rather than panic.
	if cfg.Schema == nil {
		return context.WithTimeout(ctx, defaultTimeout)
	}

	var configured timeouts.Value
	diags.Append(cfg.GetAttribute(ctx, path.Root(AttributeName), &configured)...)
	if diags.HasError() {
		return ctx, func() {}
	}

	timeout, timeoutDiags := configured.Invoke(ctx, defaultTimeout)
	diags.Append(timeoutDiags...)
	if diags.HasError() {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}
