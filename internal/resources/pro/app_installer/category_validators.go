// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// selfServiceCategoryValidators guards self_service_settings.categories
// against a category_id listed twice.
//
// Jamf Pro keeps one entry per category and the last one it receives wins.
// Wire-probed on the EU sandbox, 2026-11-11, on create and update alike:
// featured true then false stored false, false then true stored true, and
// identical entries stored one. A set has no order, so which entry the
// provider sends last is not the order written, and a config with two
// entries for one id cannot match what Jamf Pro stores. With the provider
// built from main the create failed with "inconsistent result after apply".
func selfServiceCategoryValidators() []validator.Set {
	return []validator.Set{
		validators.UniqueStringFieldSetRule(validators.UniqueFieldRule{
			Field:   "category_id",
			Summary: "Self Service category listed more than once",
			Detail:  "Category %s is listed more than once. Jamf Pro keeps only one entry per category, and entries in a set have no order, so which featured value it keeps is unpredictable. List the category once, with the featured value you want.",
		}),
	}
}
