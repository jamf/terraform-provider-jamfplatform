// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// extractPatchPolicyID returns the assigned ID as a string from a Create/GET
// response. Create returns the ID at the top level (<patch_policy><id>); GET
// echoes the same. Prefer the top-level reading, fall back to general.id.
func extractPatchPolicyID(p *proclassic.PatchPolicy) string {
	if p == nil {
		return ""
	}
	if p.ID != nil {
		return strconv.Itoa(*p.ID)
	}
	if p.General != nil && p.General.ID != nil {
		return strconv.Itoa(*p.General.ID)
	}
	return ""
}

// int64ValueOrNull maps an SDK *int onto a Terraform Int64, null for nil. Used
// for the server-derived release_date (Computed-only).
func int64ValueOrNull(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}
