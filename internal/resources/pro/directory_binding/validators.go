// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// typeBlockConfigValidator enforces the bidirectional cross-field rule
// between `type` and the four per-type nested blocks
// (`active_directory`, `open_directory`, `admitmac`, `centrify`) at plan
// time, before apply.
//
// The rule, derived from the §13.2 wire audit (see
// local-testing/directorybindings/AUDIT_FINDINGS.md):
//
//   - `type = "Active Directory"` → the `active_directory` nested block is
//     required and the other three must be absent.
//   - `type = "Open Directory"`   → `open_directory` only.
//   - `type = "ADmitMac"`         → `admitmac` only.
//   - `type = "Centrify"`         → `centrify` only.
//   - `type = "PowerBroker Identity Services"` → ALL four nested blocks
//     must be absent. PowerBroker carries no per-type fields in the wire,
//     so the TF schema exposes no nested block for it; the `type`
//     attribute on its own conveys the PowerBroker identity. The input
//     builder synthesises the empty SDK struct from `type` alone.
//
// The block is required, not merely permitted, because Jamf Pro returns the
// typed block populated for these four types whether or not one was sent, and
// the state builder keeps it. A block that is absent from the configuration then
// comes back non-null and Terraform fails the apply with "Provider produced
// inconsistent result after apply" on the block itself, on create and on an
// update that removes it. Wire-probed 2026-10-14. An empty block (`active_directory
// = {}`) satisfies the rule and takes Jamf Pro's defaults.
//
// A block whose value is unknown at plan time passes both checks: it counts as
// supplying the matching block and as not supplying any other. Terraform runs
// the validator again with known values when it re-plans during apply, so a
// block that resolves to the wrong shape still fails there. The check reads
// each attribute with GetAttribute rather than decoding the whole config,
// because a block built from a variable or another resource is unknown during
// validation and a struct decode fails on it.
//
// Off-the-shelf framework validators (`stringvalidator.ConflictsWith`)
// fire when the companion is set regardless of `type`'s value — that is
// the wrong shape here because the conflict is value-specific. Hence a
// custom resource.ConfigValidator. Mirrors `printer`'s
// `useGenericPPDConfigValidator`. See STYLE_GUIDE §Cross-field
// validation.
type typeBlockConfigValidator struct{}

// Description returns a plain-text description of the validator.
func (typeBlockConfigValidator) Description(context.Context) string {
	return "each value of `type` requires its own nested block, and PowerBroker takes none; a plan fails if the matching block is missing or another type's block is set"
}

// MarkdownDescription returns the markdown description.
func (v typeBlockConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
func (typeBlockConfigValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var bindingTypeValue types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("type"), &bindingTypeValue)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if bindingTypeValue.IsNull() || bindingTypeValue.IsUnknown() {
		return
	}

	bindingType := bindingTypeValue.ValueString()

	// allowedBlock names the one TF attribute that applies to the given
	// `type`. Empty string means "no block allowed" — used for PowerBroker.
	var allowedBlock string
	switch bindingType {
	case typeActiveDirectory:
		allowedBlock = "active_directory"
	case typeOpenDirectory:
		allowedBlock = "open_directory"
	case typeADmitMac:
		allowedBlock = "admitmac"
	case typeCentrify:
		allowedBlock = "centrify"
	case typePowerBroker:
		// All four blocks forbidden; allowedBlock stays "".
	default:
		// stringvalidator.OneOf on the schema rejects unknown values
		// at this same plan step, so we have nothing further to add.
		return
	}

	for _, name := range typedBlockNames {
		var block types.Object
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(name), &block)...)
		if resp.Diagnostics.HasError() {
			return
		}
		switch {
		case name == allowedBlock && block.IsNull():
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				fmt.Sprintf("%s required when type = %q", name, bindingType),
				fmt.Sprintf("`type = %q` needs the `%s` block. Jamf Pro returns the block with values filled in even when you leave it out, and the apply then fails with an inconsistent result. Add `%s = {}` to accept Jamf Pro's defaults.", bindingType, name, name),
			)
		case name != allowedBlock && !block.IsNull() && !block.IsUnknown():
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				fmt.Sprintf("%s forbidden when type = %q", name, bindingType),
				explainBlockForbidden(name, bindingType, allowedBlock),
			)
		}
	}
}

// typedBlockNames are the per-type nested blocks, one per directory type that
// carries type-specific configuration.
var typedBlockNames = []string{"active_directory", "open_directory", "admitmac", "centrify"}

// explainBlockForbidden produces the long-form diagnostic detail string
// used by typeBlockConfigValidator. Pulled out so the diagnostic message
// is consistent across the four "this block is wrong for that type"
// permutations.
func explainBlockForbidden(attemptedBlock, bindingType, allowedBlock string) string {
	if allowedBlock == "" {
		return fmt.Sprintf(
			"`%s` cannot be set when `type = %q`. Jamf Pro has no type-specific settings for PowerBroker, so it takes no nested block. Remove the `%s` block.",
			attemptedBlock, bindingType, attemptedBlock,
		)
	}
	return fmt.Sprintf(
		"`%s` cannot be set when `type = %q`, which uses the `%s` block. Change `type` to match the block you set, or remove the `%s` block.",
		attemptedBlock, bindingType, allowedBlock, attemptedBlock,
	)
}

// Compile-time interface assertion.
var _ resource.ConfigValidator = typeBlockConfigValidator{}
