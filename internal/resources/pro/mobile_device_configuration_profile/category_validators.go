// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// selfServiceCategoryValidators guards self_service.categories against a
// category id listed twice. This resource exposes no display or feature flag
// (the write always sends display_in = true, and the read echoes only id and
// name), so a repeated id is the only shape a configuration can reach that
// Jamf Pro rewrites. Wire-probed on the EU sandbox, 2026-11-11, on create and
// update alike: repeated entries for one id store one, and an entry sent with
// display_in = false is dropped, as on the other five classic resources.
// With the provider built from main, two entries for one id, identical or
// not, failed the apply with "inconsistent result after apply" and planned
// the same change again.
func selfServiceCategoryValidators() []validator.List {
	return validators.SelfServiceCategoriesList("", "")
}
