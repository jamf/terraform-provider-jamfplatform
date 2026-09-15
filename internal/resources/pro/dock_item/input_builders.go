// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildDockItemInput converts the Terraform plan model into the SDK DockItem
// payload used for both Create and Update. ID is omitted on write — Create
// uses path id="0" and Update derives it from state. Contents is omitted
// entirely on writes: the classic /dockitems endpoint ignores user-supplied
// PLIST and recomputes it server-side from name + type + path. SDK omitempty
// tags drop the field from the wire when nil.
func buildDockItemInput(plan DockItemResourceModel) *proclassic.DockItem {
	return &proclassic.DockItem{
		Name: helpers.OptionalStringPointer(plan.Name),
		Type: helpers.OptionalStringPointer(plan.Type),
		Path: helpers.OptionalStringPointer(plan.Path),
	}
}
