// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package prestageskip

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// PaneBool returns the attribute for one pane in the skip_setup_items block. Jamf Pro returns
// every pane on every read, so a refreshed state never holds a null pane. A null prior therefore
// means the attribute is missing from the state, either because the block is new or because an
// older provider wrote the state before the pane existed or was renamed. Such a pane plans as
// unknown, so apply stores the value Jamf Pro holds. Carrying the null instead makes Terraform
// reject the apply with "Provider produced inconsistent result after apply" after a plan made
// with -refresh=false. A non-null prior is carried, which keeps a refreshed plan empty.
func PaneBool(markdownDescription string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: markdownDescription,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
	}
}
