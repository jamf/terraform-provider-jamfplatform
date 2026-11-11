// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
)

// externalRecipientsValidators rejects an email address listed twice in
// external_recipients.
//
// Wire-probed on the EU sandbox, 2026-11-11, on create and update alike:
// identical entries store one, which Terraform's own set collapse already
// covers. The same address with a different name answers HTTP 500 with an
// empty errors array, and so does the same address in a different case,
// whether the names match or not, so Jamf Pro compares addresses without
// regard to case. The same name with different addresses stores both. With
// the provider built from main the create surfaced that bare 500.
func externalRecipientsValidators() []validator.Set {
	return []validator.Set{
		validators.UniqueStringFieldSetRule(validators.UniqueFieldRule{
			Field:   "email",
			Key:     strings.ToLower,
			Summary: "External recipient listed more than once",
			Detail:  "%s is listed more than once. Jamf Pro compares email addresses without regard to case and refuses a notification that lists one twice, failing with a server error. List each address once.",
		}),
	}
}
