// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// selfServiceCategoryValidators guards self_service.self_service_categories
// against the two shapes Jamf Pro rewrites on write: a category id listed
// twice (merged into one entry) and display_in = false (the entry is
// dropped). Mobile device apps carry no feature flag. With the provider built
// from main, 2026-11-11: two entries for one id, display_in true and false,
// failed the apply with "inconsistent result after apply" and then planned
// the same change on every run, and display_in = false alone failed the same
// way. See validators.SelfServiceCategories for the wire probe.
func selfServiceCategoryValidators() []validator.Set {
	return validators.SelfServiceCategories("display_in", "")
}
