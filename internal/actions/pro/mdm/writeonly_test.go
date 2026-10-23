// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"maps"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// allMdmActions is every action this package registers.
func allMdmActions() map[string]action.Action {
	all := map[string]action.Action{
		"flush_mdm_commands": NewFlushMdmCommandsAction(),
		"renew_mdm_profile":  NewRenewMdmProfileAction(),
	}
	maps.Copy(all, batchActions())
	maps.Copy(all, singleActions())
	return all
}

// TestActionAttributes_AreNotWriteOnly is a regression guard with teeth.
//
// WriteOnly exists on action attributes and compiles, but the framework hardcodes
// WriteOnlyAttributesAllowed: false for action config validation while still
// applying the resource write-only gate, so ANY non-null value fails with
// "WriteOnly Attribute Not Allowed" — unconditionally, on every Terraform
// version. That breaks silently: nothing fails until someone actually assigns the
// attribute, and the acceptance tests that would notice are device-gated and skip
// in CI. So the invariant is asserted here, in a unit test that always runs.
//
// If a future attribute genuinely needs write-only semantics, verify against a
// real Terraform binary first: an action attribute can be neither WriteOnly nor
// Sensitive, so a user-supplied secret can only be disclosed as plan-visible.
func TestActionAttributes_AreNotWriteOnly(t *testing.T) {
	for name, a := range allMdmActions() {
		var resp action.SchemaResponse
		a.Schema(context.Background(), action.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: schema diagnostics: %v", name, resp.Diagnostics)
		}
		for attrName, attr := range resp.Schema.Attributes {
			if attr.IsWriteOnly() {
				t.Errorf("%s: attribute %q is WriteOnly; Terraform rejects any value set for a write-only action attribute, so the attribute cannot be used at all", name, attrName)
			}
		}
	}
}
