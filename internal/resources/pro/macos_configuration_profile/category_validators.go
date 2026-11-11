// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// selfServiceCategoryValidators guards self_service.categories against the
// shapes Jamf Pro rewrites on write. Wire-probed on the EU sandbox,
// 2026-11-11, on create and update alike: repeated entries for one id,
// identical or not, store one entry whose flags are the OR of the entries,
// and an entry with display_in = false is dropped whatever feature_in says.
// The list is stored sorted by id, which the state builder already aligns
// back to the written order. With the provider built from main, every one of
// those shapes failed the apply with "inconsistent result after apply"
// ("element 1 has vanished", or the next element shifting into the dropped
// one's index) and then planned the same change again.
func selfServiceCategoryValidators() []validator.List {
	return validators.SelfServiceCategoriesList("display_in", "feature_in")
}
