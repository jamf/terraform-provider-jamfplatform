// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildSiteInput converts the Terraform plan model into an SDK Site payload.
// Name is required by the schema so we always send it as a non-nil pointer.
// ID is omitted on write — Create uses path id="0" and Update derives it from state.
func buildSiteInput(plan SiteResourceModel) *proclassic.Site {
	return &proclassic.Site{
		Name: helpers.OptionalStringPointer(plan.Name),
	}
}
