// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	jamfprovider "github.com/jamf/terraform-provider-jamfplatform/internal/provider"
)

// deadlineExempt names constructs that carry no `timeouts` attribute, each with the reason. It is
// empty and a new entry needs a justification a reviewer would accept: the SDK stopped capping a
// request at 60s of response-header wait, so a construct that gives its calls no deadline can hang a
// plan or apply indefinitely on a stalled gateway.
var deadlineExempt = map[string]string{}

// Every resource, data source and action must expose a `timeouts` attribute or block, which is
// what its SDK calls are bounded by. Resources and data sources resolve it through
// helpers.ResolveTimeout; actions through actiontimeouts.Bound.
//
// This is a schema-level guard, so it proves a construct offers the knob rather than that every
// call site uses it. It still closes the gap this exists for — a group of data sources and
// every action shipped with no deadline at all and nothing noticed — because a construct that
// never declared the attribute has nothing to resolve a deadline from.
func TestEveryConstructDeclaresTimeouts(t *testing.T) {
	ctx := context.Background()
	p, ok := jamfprovider.New("test")().(interface {
		provider.Provider
		provider.ProviderWithActions
	})
	if !ok {
		t.Fatal("provider no longer implements ProviderWithActions")
	}

	var missing []string
	var checked int
	note := func(kind, name string, hasTimeouts bool) {
		checked++
		if hasTimeouts {
			return
		}
		if _, exempt := deadlineExempt[kind+" "+name]; exempt {
			return
		}
		missing = append(missing, kind+" "+name)
	}

	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &meta)
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		_, attr := resp.Schema.Attributes[actiontimeouts.AttributeName]
		_, block := resp.Schema.Blocks[actiontimeouts.AttributeName]
		note("resource", meta.TypeName, attr || block)
	}

	for _, newDataSource := range p.DataSources(ctx) {
		d := newDataSource()
		var meta datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &meta)
		var resp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &resp)
		_, attr := resp.Schema.Attributes[actiontimeouts.AttributeName]
		_, block := resp.Schema.Blocks[actiontimeouts.AttributeName]
		note("data source", meta.TypeName, attr || block)
	}

	for _, newAction := range p.Actions(ctx) {
		a := newAction()
		var meta action.MetadataResponse
		a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "jamfplatform"}, &meta)
		var resp action.SchemaResponse
		a.Schema(ctx, action.SchemaRequest{}, &resp)
		_, attr := resp.Schema.Attributes[actiontimeouts.AttributeName]
		note("action", meta.TypeName, attr)
	}

	// A walk that finds nothing reports perfect agreement, so insist it saw the provider.
	if checked < 200 {
		t.Fatalf("inspected only %d constructs; the provider registers far more, so this scan is broken", checked)
	}

	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s declares no `timeouts` attribute, so its SDK calls have no deadline", name)
	}
}
