// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignSiteResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := SiteResourceModel{
		ID:   types.StringValue("42"),
		Name: types.StringValue("Primary"),
	}
	name := "Primary refreshed"
	api := &proclassic.Site{ID: nil, Name: &name}

	assignSiteResourceModel(&state, api)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected state.ID preserved as %q, got %q", "42", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Primary refreshed" {
		t.Errorf("expected Name refreshed, got %q", state.Name.ValueString())
	}
}

func TestAssignSiteResourceModel_OverwritesIDWhenAPIPresent(t *testing.T) {
	state := SiteResourceModel{ID: types.StringValue("placeholder")}
	id := 99
	name := "Branch"
	api := &proclassic.Site{ID: &id, Name: &name}

	assignSiteResourceModel(&state, api)

	if state.ID.ValueString() != "99" {
		t.Errorf("expected state.ID overwritten to %q, got %q", "99", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Branch" {
		t.Errorf("expected Name %q, got %q", "Branch", state.Name.ValueString())
	}
}

func TestAssignSiteResourceModel_NilAPIIsNoop(t *testing.T) {
	state := SiteResourceModel{
		ID:   types.StringValue("7"),
		Name: types.StringValue("Keep"),
	}
	assignSiteResourceModel(&state, nil)
	if state.ID.ValueString() != "7" || state.Name.ValueString() != "Keep" {
		t.Errorf("expected state unchanged, got id=%q name=%q", state.ID.ValueString(), state.Name.ValueString())
	}
}

func TestAssignSiteDataSourceModel_PopulatesBoth(t *testing.T) {
	state := SiteDataSourceModel{}
	id := 11
	name := "Looked Up"
	api := &proclassic.Site{ID: &id, Name: &name}

	assignSiteDataSourceModel(&state, api)

	if state.ID.ValueString() != "11" {
		t.Errorf("expected ID %q, got %q", "11", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Looked Up" {
		t.Errorf("expected Name %q, got %q", "Looked Up", state.Name.ValueString())
	}
}

func TestAssignSiteDataSourceModel_PreservesSelectorOnNilAPIFields(t *testing.T) {
	// Caller looked the site up by name; the SDK responded with a nil Name field.
	// The DS must NOT overwrite the caller-supplied name with null. Same contract
	// as the resource model — symmetric guard.
	state := SiteDataSourceModel{
		ID:   types.StringNull(),
		Name: types.StringValue("Primary"),
	}
	id := 7
	api := &proclassic.Site{ID: &id, Name: nil}

	assignSiteDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID written, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Primary" {
		t.Errorf("expected Name preserved as %q, got %q", "Primary", state.Name.ValueString())
	}
}

func TestAssignSiteDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := SiteDataSourceModel{
		ID:   types.StringValue("preset"),
		Name: types.StringValue("preset"),
	}
	assignSiteDataSourceModel(&state, nil)
	if state.ID.ValueString() != "preset" || state.Name.ValueString() != "preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}
