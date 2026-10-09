// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package listtimeouts gives every list resource a context deadline and a `timeouts` attribute.
//
// A list resource's List receives the raw Terraform context, which carries no deadline of its own,
// so without this a stalled gateway holds `terraform query` until the operator interrupts it.
// Resources and data sources bound their SDK calls through helpers.ResolveTimeout and a timeouts
// block; list resources do the same through terraform-plugin-framework-timeouts/list/timeouts,
// wrapped here so each list resource is two lines (Add in ListResourceConfigSchema, Bound at the
// top of List) plus a Timeouts field on its config model, which the framework requires once the
// schema declares the attribute.
package listtimeouts

import (
	"context"
	"maps"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/list/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// Value is the type a list resource's config model gives its `timeouts` field. The framework
// refuses to decode a config object into a struct that lacks a field for one of its attributes, so
// every model that Add's attribute reaches must carry `Timeouts listtimeouts.Value` tagged
// `tfsdk:"timeouts"`.
type Value = timeouts.Value

// AttributeName is the schema key every list resource exposes its deadline under.
const AttributeName = "timeouts"

// Add returns a copy of attrs with the `timeouts` attribute added. A nil attrs is valid. It copies
// rather than mutates because several list resources build their attributes from a shared helper.
func Add(ctx context.Context, attrs map[string]listschema.Attribute) map[string]listschema.Attribute {
	out := make(map[string]listschema.Attribute, len(attrs)+1)
	maps.Copy(out, attrs)
	out[AttributeName] = timeouts.AttributesWithOpts(ctx, timeouts.Opts{
		ListDescription: "How long the listing may run before it is cancelled, as a duration such as `30s` or " +
			"`10m`. Reading an individual result, when a resource is generated for each one, has its own " +
			"separate limit.",
	})
	return out
}

// Bound derives a context that expires after the configured `list` timeout, or after
// defaultTimeout when none is set. The returned cancel is never nil and must be deferred.
//
// The attribute is read by path rather than through the list resource's config model, so Bound
// itself does not depend on the model; the model still needs the Timeouts field for every other
// decode of the config. A malformed duration is reported on diags and the context is returned
// unbounded, so the caller must check diags.HasError() before using it.
func Bound(ctx context.Context, cfg tfsdk.Config, diags *diag.Diagnostics, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	if cfg.Schema == nil {
		return context.WithTimeout(ctx, defaultTimeout)
	}

	var configured timeouts.Value
	diags.Append(cfg.GetAttribute(ctx, path.Root(AttributeName), &configured)...)
	if diags.HasError() {
		return ctx, func() {}
	}

	timeout, timeoutDiags := configured.List(ctx, defaultTimeout)
	diags.Append(timeoutDiags...)
	if diags.HasError() {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}
