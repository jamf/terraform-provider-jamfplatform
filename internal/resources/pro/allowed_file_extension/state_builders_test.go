// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignAllowedFileExtensionResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := AllowedFileExtensionResourceModel{
		ID:        types.StringValue("42"),
		Extension: types.StringValue("jpg"),
	}
	ext := "png"
	api := &proclassic.AllowedFileExtension{ID: nil, Extension: &ext}

	assignAllowedFileExtensionResourceModel(&state, api)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected state.ID preserved as %q, got %q", "42", state.ID.ValueString())
	}
	if state.Extension.ValueString() != "png" {
		t.Errorf("expected Extension refreshed, got %q", state.Extension.ValueString())
	}
}

func TestAssignAllowedFileExtensionResourceModel_OverwritesIDWhenAPIPresent(t *testing.T) {
	state := AllowedFileExtensionResourceModel{ID: types.StringValue("placeholder")}
	id := 99
	ext := "tar.gz"
	api := &proclassic.AllowedFileExtension{ID: &id, Extension: &ext}

	assignAllowedFileExtensionResourceModel(&state, api)

	if state.ID.ValueString() != "99" {
		t.Errorf("expected state.ID overwritten to %q, got %q", "99", state.ID.ValueString())
	}
	if state.Extension.ValueString() != "tar.gz" {
		t.Errorf("expected Extension %q, got %q", "tar.gz", state.Extension.ValueString())
	}
}

func TestAssignAllowedFileExtensionResourceModel_NilAPIIsNoop(t *testing.T) {
	state := AllowedFileExtensionResourceModel{
		ID:        types.StringValue("7"),
		Extension: types.StringValue("keepme"),
	}
	assignAllowedFileExtensionResourceModel(&state, nil)
	if state.ID.ValueString() != "7" || state.Extension.ValueString() != "keepme" {
		t.Errorf("expected state unchanged, got id=%q ext=%q", state.ID.ValueString(), state.Extension.ValueString())
	}
}

func TestAssignAllowedFileExtensionDataSourceModel_PopulatesBoth(t *testing.T) {
	state := AllowedFileExtensionDataSourceModel{}
	id := 11
	ext := "csv"
	api := &proclassic.AllowedFileExtension{ID: &id, Extension: &ext}

	assignAllowedFileExtensionDataSourceModel(&state, api)

	if state.ID.ValueString() != "11" {
		t.Errorf("expected ID %q, got %q", "11", state.ID.ValueString())
	}
	if state.Extension.ValueString() != "csv" {
		t.Errorf("expected Extension %q, got %q", "csv", state.Extension.ValueString())
	}
}

func TestAssignAllowedFileExtensionDataSourceModel_PreservesSelectorOnNilAPIFields(t *testing.T) {
	// Caller looked the record up by extension; the SDK responded with a nil Extension
	// field. The DS must NOT overwrite the caller-supplied value with null. Same contract
	// as the resource model — symmetric guard.
	state := AllowedFileExtensionDataSourceModel{
		ID:        types.StringNull(),
		Extension: types.StringValue("csv"),
	}
	id := 7
	api := &proclassic.AllowedFileExtension{ID: &id, Extension: nil}

	assignAllowedFileExtensionDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID written, got %q", state.ID.ValueString())
	}
	if state.Extension.ValueString() != "csv" {
		t.Errorf("expected Extension preserved as %q, got %q", "csv", state.Extension.ValueString())
	}
}

func TestAssignAllowedFileExtensionDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := AllowedFileExtensionDataSourceModel{
		ID:        types.StringValue("preset"),
		Extension: types.StringValue("preset"),
	}
	assignAllowedFileExtensionDataSourceModel(&state, nil)
	if state.ID.ValueString() != "preset" || state.Extension.ValueString() != "preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}
