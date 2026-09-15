// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// This file uses Go 1.26's extended `new(v)` builtin, which allocates a
// pointer to the value `v` and returns the pointer — replacing the older
// two-step `x := v; &x` pattern. The expression `new(1)` returns `*int`
// pointing at `1`; `new("foo")` returns `*string` pointing at `"foo"`.
// `go fix` rewrites the older two-step form to this builtin on Go 1.26,
// so reviewing this file against an older Go expectation will flag valid
// code as broken — it is not. See the Go 1.26 release notes.

package directory_binding

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAssignDirectoryBindingResourceModel_ActiveDirectory mirrors the audit
// payload for ID 1 — every flat field plus a fully populated
// active_directory block.
func TestAssignDirectoryBindingResourceModel_ActiveDirectory(t *testing.T) {
	state := DirectoryBindingResourceModel{}
	in := &proclassic.DirectoryBinding{
		ID:         new(1),
		Name:       new("Active Directory"),
		Priority:   new(1),
		Domain:     new("test.com"),
		Username:   new("test"),
		ComputerOu: new("test"),
		Type:       new("Active Directory"),
		ActiveDirectory: &proclassic.DirectoryBindingActiveDirectory{
			Forest:              new(""),
			CacheLastUser:       new(true),
			RequireConfirmation: new(true),
			LocalHome:           new(true),
			UseUncPath:          new(true),
			MountStyle:          new("smb"),
			DefaultShell:        new("/bin/bash"),
			Uid:                 new("testuid"),
			UserGid:             new("testuser"),
			Gid:                 new("testgroup"),
			MultipleDomains:     new(true),
			PreferredDomain:     new("testserver"),
			AdminGroups:         new("testgroup1,testgroup2"),
		},
	}

	diags := assignDirectoryBindingResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "1" {
		t.Errorf("expected ID=1, got %s", state.ID.ValueString())
	}
	if state.Type.ValueString() != "Active Directory" {
		t.Errorf("expected Type=Active Directory, got %s", state.Type.ValueString())
	}
	if state.ActiveDirectory == nil {
		t.Fatalf("expected ActiveDirectory nested model, got nil")
	}
	if state.ActiveDirectory.AdminGroups.ValueString() != "testgroup1,testgroup2" {
		t.Errorf("AdminGroups did not round-trip: %s", state.ActiveDirectory.AdminGroups.ValueString())
	}
	if state.OpenDirectory != nil || state.Admitmac != nil || state.Centrify != nil {
		t.Errorf("non-AD nested blocks must remain nil")
	}
}

// TestAssignDirectoryBindingResourceModel_PowerBroker_NoBlockSurfaced
// confirms that the empty `<powerbroker_identity_services/>` wire element
// does not surface in TF state — the TF schema exposes no nested block for
// PowerBroker, so the `type` field on its own conveys the identity.
func TestAssignDirectoryBindingResourceModel_PowerBroker_NoBlockSurfaced(t *testing.T) {
	state := DirectoryBindingResourceModel{}
	in := &proclassic.DirectoryBinding{
		ID:                          new(62),
		Name:                        new("PowerBroker"),
		Type:                        new("PowerBroker Identity Services"),
		PowerbrokerIdentityServices: &proclassic.DirectoryBindingPowerbrokerIdentityServices{},
	}
	diags := assignDirectoryBindingResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ActiveDirectory != nil || state.OpenDirectory != nil || state.Admitmac != nil || state.Centrify != nil {
		t.Errorf("PowerBroker GET must not populate any TF nested block; the schema exposes none for this type")
	}
}

// TestAssignDirectoryBindingResourceModel_PasswordNotTouched verifies the
// state builder does not write state.Password. Password is a `WriteOnly`
// attribute — the framework excludes it from state regardless of what we
// assign — so this test pins the no-op contract: passing a state with a
// pre-populated Password (e.g. from an in-memory plan model) survives the
// builder unchanged.
func TestAssignDirectoryBindingResourceModel_PasswordNotTouched(t *testing.T) {
	state := DirectoryBindingResourceModel{
		Password: types.StringValue("hunter2"),
	}
	in := &proclassic.DirectoryBinding{
		ID:   new(1),
		Name: new("ad"),
		Type: new("Active Directory"),
	}
	diags := assignDirectoryBindingResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Password.ValueString() != "hunter2" {
		t.Errorf("state.Password must be preserved across reads; got %q (state builder must never touch it)", state.Password.ValueString())
	}
}

// TestAssignDirectoryBindingResourceModel_IDNotClobbered verifies that
// a transient GET response with a nil ID does not clobber a populated
// state.ID from Create.
func TestAssignDirectoryBindingResourceModel_IDNotClobbered(t *testing.T) {
	state := DirectoryBindingResourceModel{
		ID: types.StringValue("42"),
	}
	in := &proclassic.DirectoryBinding{
		Name: new("ad"),
		Type: new("Active Directory"),
	}
	diags := assignDirectoryBindingResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "42" {
		t.Errorf("nil API ID must not clobber existing state ID; got %s", state.ID.ValueString())
	}
}

// TestAssignDirectoryBindingResourceModel_NilSafe covers the contract
// that a nil binding response leaves state untouched.
func TestAssignDirectoryBindingResourceModel_NilSafe(t *testing.T) {
	state := DirectoryBindingResourceModel{ID: types.StringValue("1")}
	diags := assignDirectoryBindingResourceModel(&state, nil)
	if diags.HasError() {
		t.Errorf("nil-binding assigner must return clean diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "1" {
		t.Errorf("nil response must leave state untouched")
	}
}

// TestAssignDirectoryBindingResourceModel_ADmitMac_LocalHomeString
// pins the ADmitMac `local_home` decoder as string-valued — the AD field
// of the same name is bool-valued. This avoids the trap of generic
// codegen flattening both to a single shape.
func TestAssignDirectoryBindingResourceModel_ADmitMac_LocalHomeString(t *testing.T) {
	state := DirectoryBindingResourceModel{}
	in := &proclassic.DirectoryBinding{
		ID:   new(63),
		Name: new("admitmac"),
		Type: new("ADmitMac"),
		Admitmac: &proclassic.DirectoryBindingAdmitmac{
			LocalHome:         new("Local"),
			CachedCredentials: new(10),
		},
	}
	diags := assignDirectoryBindingResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Admitmac == nil {
		t.Fatalf("expected Admitmac nested model")
	}
	if state.Admitmac.HomeLocation.ValueString() != "Local" {
		t.Errorf("expected LocalHome='Local', got %s", state.Admitmac.HomeLocation.ValueString())
	}
	if state.Admitmac.CachedCredentials.ValueInt64() != 10 {
		t.Errorf("expected CachedCredentials=10, got %d", state.Admitmac.CachedCredentials.ValueInt64())
	}
}

// TestAssignDirectoryBindingDataSourceModel_BasicRoundTrip confirms the
// data source model populates from a GET response. The data source omits the
// WriteOnly `password` attribute (read-only context) and no longer surfaces
// the `password_sha256` sentinel (the wire value is a literal
// 20-asterisk redaction string with no drift-detection signal).
func TestAssignDirectoryBindingDataSourceModel_BasicRoundTrip(t *testing.T) {
	state := DirectoryBindingDataSourceModel{}
	in := &proclassic.DirectoryBinding{
		ID:   new(1),
		Name: new("ad"),
		Type: new("Active Directory"),
	}
	diags := assignDirectoryBindingDataSourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Name.ValueString() != "ad" {
		t.Errorf("data source Name must round-trip; got %q", state.Name.ValueString())
	}
}
