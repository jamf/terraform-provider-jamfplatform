// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// CriterionAttributes returns the resource-schema attribute map for a single
// classic smart-group / advanced-search criterion, shared by every ProClassic
// resource that exposes the classic `<criterion>` element: jamfplatform_pro_user_group,
// jamfplatform_pro_advanced_computer_search, and jamfplatform_pro_advanced_user_search.
// All three back onto the SDK's shared proclassic.Criterion type with identical
// fields, types, and null semantics — extracted per STYLE_GUIDE §Schemas 3-consumer
// rule.
//
// The caller wraps the returned map in its own collection attribute (a List for
// the ordered criteria these resources expose) with a resource-specific
// description:
//
//	"criteria": schema.ListNestedAttribute{
//	    MarkdownDescription: "...",
//	    Optional:            true,
//	    NestedObject:        schema.NestedAttributeObject{Attributes: criteria.CriterionAttributes(ValidOperators)},
//	}
//
// operators is the accepted search_type vocabulary — pass criteria.Operators for
// the full set (computer searches) or criteria.Without(...) for the user-attribute
// subset (user groups, user searches). Note this is intentionally NOT the
// device_group criterion shape: device_group is a Platform Services resource with
// divergent legacy attribute names (`order`/`criteria`/`operator`) and is not a
// consumer of this helper.
func CriterionAttributes(operators []string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"priority": schema.Int64Attribute{
			MarkdownDescription: "Evaluation order for the criterion. Defaults to the element index (zero-based) if omitted.",
			Optional:            true,
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Inventory attribute to evaluate (the criterion field name, e.g. `Username`, `Serial Number`, `Last Inventory Update`).",
			Required:            true,
		},
		"search_type": schema.StringAttribute{
			MarkdownDescription: Description(operators),
			Required:            true,
			Validators: []validator.String{
				stringvalidator.OneOf(operators...),
			},
		},
		"value": schema.StringAttribute{
			MarkdownDescription: "Comparison value for the operator.",
			Required:            true,
		},
		"and_or": schema.StringAttribute{
			MarkdownDescription: "How this criterion joins to the next. Valid values are `and` or `or`. Defaults to `and` if omitted.",
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(proclassic.CriterionAndOrAnd),
			Validators: []validator.String{
				stringvalidator.OneOf(proclassic.CriterionAndOrValues()...),
			},
		},
		"has_opening_parenthesis": schema.BoolAttribute{
			MarkdownDescription: "Whether the criterion begins a parenthetical grouping.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
		},
		"has_closing_parenthesis": schema.BoolAttribute{
			MarkdownDescription: "Whether the criterion ends a parenthetical grouping.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
		},
	}
}
