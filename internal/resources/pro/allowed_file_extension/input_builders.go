// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildAllowedFileExtensionInput converts the Terraform plan model into an SDK
// AllowedFileExtension payload. Extension (the wire `extension`) is required by the
// schema so we always send it as a non-nil pointer. ID is omitted on write — Create uses
// path id="0" and the server mints the real ID.
func buildAllowedFileExtensionInput(plan AllowedFileExtensionResourceModel) *proclassic.AllowedFileExtension {
	return &proclassic.AllowedFileExtension{
		Extension: helpers.OptionalStringPointer(plan.Extension),
	}
}
