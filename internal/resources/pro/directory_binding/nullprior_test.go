// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/nullprior"
)

// TestDirectoryBindingResource_OptionalBlockLeavesPlanUnknownFromNullPrior
// exempts the four per-type blocks because none of them is ever null in state
// when it is next declared, so the omitted-to-declared transition the guard
// models cannot happen. Jamf Pro answers a binding of type Active Directory,
// Open Directory, ADmitMac or Centrify with that type's block populated whether
// or not one was sent, and the state builder keeps it. A binding created without
// its block therefore fails on the spot, with "Provider produced inconsistent
// result after apply" on the block itself, and never reaches a state with the
// block null. Changing `type` in place fails the same way, because Jamf Pro
// keeps the original type. Wire-probed 2026-10-14.
//
// If the block ever becomes null in state (a fix that hydrates it only when it
// is declared, say), remove the exemption: its leaves would then need
// UseNonNullStateForUnknown, and AssertNoPlanCollapse passes for this resource.
func TestDirectoryBindingResource_OptionalBlockLeavesPlanUnknownFromNullPrior(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewDirectoryBindingResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertOptionalBlockLeavesPlanUnknown(t, resp.Schema,
		nullprior.ExemptBlocks("the server populates the typed block whether or not it is sent", "active_directory", "open_directory", "admitmac", "centrify"))
}
