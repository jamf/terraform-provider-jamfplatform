// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// selfServiceCategoryValidators guards self_service.categories against the
// two shapes Jamf Pro rewrites on write: a category id listed twice (merged
// into one entry) and display_in = false (the entry is dropped). Both made
// the plan disagree with what Jamf Pro stores. With the provider built from
// main, 2026-11-11: two entries for one id applied and then planned the same
// change on every run, and an entry with display_in = false failed the apply
// with "inconsistent result after apply". See validators.SelfServiceCategories
// for the wire probe.
func selfServiceCategoryValidators() []validator.Set {
	return validators.SelfServiceCategories("display_in", "feature_in")
}
