// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// UserScopeAttributes returns the attribute map for a user-based classic
// resource's <scope> block (targets + limitations + exclusions). This is the
// THIRD scope shape (distinct from ComputerScopeAttributes / MobileScopeAttributes):
// targets are Jamf Pro users and Jamf Pro user groups (id-keyed) plus the
// all_jss_users flag; limitations and exclusions carry directory-service (LDAP)
// user groups by NAME. The caller wraps the map in its own
// schema.SingleNestedAttribute with a resource-specific description:
//
//	"scope": schema.SingleNestedAttribute{
//	    MarkdownDescription: "...",
//	    Optional:            true,
//	    Attributes:          scope.UserScopeAttributes(),
//	}
//
// Every attribute in the block follows per-category granular ownership:
// omitted means "not managed by Terraform" (preserved on the wire via
// read-merge-write in the consuming resource's update), declared means owned.
// all_jss_users (inside the `targets` sub-block) is Optional-only and carries
// the value-discriminated AllFlagConflictsWith validator (relative paths, so
// they resolve against its siblings inside `targets`).
//
// Consumed by vpp_invitation and vpp_assignment. The build/flatten glue stays
// per-resource because VppInvitation* and VppAssignment* are distinct generated
// SDK structs with no shared interface (see STYLE_GUIDE.md §Scope helper).
func UserScopeAttributes() map[string]schema.Attribute {
	targets := map[string]schema.Attribute{
		"all_jss_users": schema.BoolAttribute{
			MarkdownDescription: "Target all Jamf Pro users. Forbids `jss_user_ids` and `jss_user_group_ids` when true. " + OmitAllFlagNote("`jss_user_ids` or `jss_user_group_ids`"),
			Optional:            true,
			Validators: []validator.Bool{
				AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("jss_user_ids"),
					path.MatchRelative().AtParent().AtName("jss_user_group_ids"),
				),
			},
		},
		"jss_user_ids":       IDSetAttribute("user"),
		"jss_user_group_ids": IDSetAttribute("user group"),
	}

	return map[string]schema.Attribute{
		"targets": schema.SingleNestedAttribute{
			MarkdownDescription: "The Jamf Pro users and user groups the assignment applies to. Mirrors the admin UI's Targets tab.",
			Optional:            true,
			Attributes:          targets,
		},
		"limitations": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope limitations.",
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"directory_service_user_group_names": NameSetAttribute("directory service (LDAP) user group"),
			},
		},
		"exclusions": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope exclusions.",
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"jss_user_ids":                       IDSetAttribute("user"),
				"jss_user_group_ids":                 IDSetAttribute("user group"),
				"directory_service_user_group_names": NameSetAttribute("directory service (LDAP) user group"),
			},
		},
	}
}
