// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// ComputerScopeOptions gates the handful of per-resource deltas in the
// otherwise-shared Jamf classic computer-scope block. Today the only axis is
// iBeacon support: policy and os_x_configuration_profile carry iBeacon
// limitations/exclusions; the macapplications endpoint silently drops them
// (wire-probed), so mac_application sets IncludeIbeacons=false to avoid a
// permadiff. See STYLE_GUIDE.md §Scope helper.
type ComputerScopeOptions struct {
	// IncludeIbeacons adds ibeacon_ids to the limitations and exclusions
	// sub-blocks. Resources whose endpoint ignores iBeacon scope must leave
	// this false so the attribute is absent rather than silently dropped.
	IncludeIbeacons bool
}

// ComputerScopeAttributes returns the attribute map for a computer-scoped
// classic resource's <scope> block (targets + limitations + exclusions),
// mirroring the Jamf Pro admin UI's Targets / Limitations / Exclusions tabs.
// The all-flags and every target category live inside the `targets`
// sub-block; each target category is a flat Set<String> of numeric Jamf Pro
// IDs and directory-service categories carry names. The caller wraps the map
// in its own schema.SingleNestedAttribute with a resource-specific description:
//
//	"scope": schema.SingleNestedAttribute{
//	    MarkdownDescription: "...",
//	    Optional:            true,
//	    Attributes:          scope.ComputerScopeAttributes(scope.ComputerScopeOptions{IncludeIbeacons: true}),
//	}
//
// Every attribute in the block — the per-category sets and the all-flags —
// follows per-category granular ownership: omitted means "not managed by
// Terraform" (preserved on the wire via read-merge-write in the consuming
// resource's update), declared means owned. The all-flags carry the
// value-discriminated AllFlagConflictsWith validators (relative paths, so they
// resolve against their siblings inside `targets`). See STYLE_GUIDE.md
// §Scope helper.
func ComputerScopeAttributes(opts ComputerScopeOptions) map[string]schema.Attribute {
	limitations := map[string]schema.Attribute{
		"network_segment_ids":                   IDSetAttribute("network segment"),
		"directory_service_or_local_user_names": NameSetAttribute("directory service or local user"),
		"directory_service_user_group_names":    NameSetAttribute("directory service user group"),
	}
	exclusions := map[string]schema.Attribute{
		"computer_ids":                          IDSetAttribute("computer"),
		"computer_group_ids":                    IDSetAttribute("computer group"),
		"building_ids":                          IDSetAttribute("building"),
		"department_ids":                        IDSetAttribute("department"),
		"user_ids":                              IDSetAttribute("user"),
		"user_group_ids":                        IDSetAttribute("user group"),
		"network_segment_ids":                   IDSetAttribute("network segment"),
		"directory_service_or_local_user_names": NameSetAttribute("directory service or local user"),
		"directory_service_user_group_names":    NameSetAttribute("directory service user group"),
	}
	if opts.IncludeIbeacons {
		limitations["ibeacon_ids"] = IDSetAttribute("iBeacon")
		exclusions["ibeacon_ids"] = IDSetAttribute("iBeacon")
	}

	targets := map[string]schema.Attribute{
		"all_computers": schema.BoolAttribute{
			MarkdownDescription: "Scope to every computer in the tenant. Forbids `computer_ids` and `computer_group_ids` when true. You can declare buildings, departments, users and user groups with it. " + OmitAllFlagNote("`computer_ids` or `computer_group_ids`"),
			Optional:            true,
			Validators: []validator.Bool{
				AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("computer_ids"),
					path.MatchRelative().AtParent().AtName("computer_group_ids"),
				),
			},
		},
		"all_jss_users": schema.BoolAttribute{
			MarkdownDescription: "Scope to every Jamf Pro user in the tenant. Matches the admin UI's \"All Users\" toggle. Forbids `user_ids` and `user_group_ids` when true. " + OmitAllFlagNote("`user_ids` or `user_group_ids`"),
			Optional:            true,
			Validators: []validator.Bool{
				AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("user_ids"),
					path.MatchRelative().AtParent().AtName("user_group_ids"),
				),
			},
		},
		"computer_ids":       IDSetAttribute("computer"),
		"computer_group_ids": IDSetAttribute("computer group"),
		"building_ids":       IDSetAttribute("building"),
		"department_ids":     IDSetAttribute("department"),
		"user_ids":           IDSetAttribute("user"),
		"user_group_ids":     IDSetAttribute("user group"),
	}

	return map[string]schema.Attribute{
		"targets": schema.SingleNestedAttribute{
			MarkdownDescription: "The audience the resource applies to. Mirrors the admin UI's Targets tab: set `all_computers` / `all_jss_users` for tenant-wide scope, or list specific IDs.",
			Optional:            true,
			Attributes:          targets,
		},
		"limitations": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope limitations narrow the audience after the targets resolve. `directory_service_or_local_user_names` and `directory_service_user_group_names` carry names (not IDs) because that is how Jamf Pro identifies these directory-service objects.",
			Optional:            true,
			Attributes:          limitations,
		},
		"exclusions": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope exclusions remove items that would otherwise be included by targets or limitations.",
			Optional:            true,
			Attributes:          exclusions,
		},
	}
}
