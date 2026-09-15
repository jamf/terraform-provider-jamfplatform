// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package location

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildCreateInput converts the Terraform plan + a TrimSpaced base64 service
// token string into the SDK POST payload. The plaintext token string lives on
// the config object (`WriteOnly`), so callers pre-trim it from the config and
// pass the resulting string in here. The wire field `serviceToken` is a plain
// `string` (Required) — Apple's `.vpptoken` file is already base64-encoded on
// disk, so the provider does NOT base64-decode the value before sending.
func buildCreateInput(plan VolumePurchasingLocationResourceModel, trimmedToken string) *pro.VolumePurchasingLocationPost {
	out := &pro.VolumePurchasingLocationPost{
		Name:         helpers.OptionalStringPointer(plan.Name),
		ServiceToken: trimmedToken,
	}
	if b := helpers.OptionalBoolPointer(plan.AutomaticallyPopulatePurchasedContent); b != nil {
		out.AutomaticallyPopulatePurchasedContent = b
	}
	if b := helpers.OptionalBoolPointer(plan.SendNotificationWhenNoLongerAssigned); b != nil {
		out.SendNotificationWhenNoLongerAssigned = b
	}
	if b := helpers.OptionalBoolPointer(plan.AutoRegisterManagedUsers); b != nil {
		out.AutoRegisterManagedUsers = b
	}
	if s := helpers.OptionalStringPointer(plan.SiteID); s != nil {
		out.SiteID = s
	}
	return out
}

// buildMetadataPatch converts the Terraform plan into the SDK PATCH payload
// used for metadata-only Updates (no token rotation). `ServiceToken` is left
// as a nil `*string` so the JSON `omitempty` tag drops the field from the
// outbound request — Jamf Pro treats a missing `serviceToken` as "leave the
// stored token alone".
func buildMetadataPatch(plan VolumePurchasingLocationResourceModel) *pro.VolumePurchasingLocationPatch {
	out := &pro.VolumePurchasingLocationPatch{
		Name: helpers.OptionalStringPointer(plan.Name),
	}
	if b := helpers.OptionalBoolPointer(plan.AutomaticallyPopulatePurchasedContent); b != nil {
		out.AutomaticallyPopulatePurchasedContent = b
	}
	if b := helpers.OptionalBoolPointer(plan.SendNotificationWhenNoLongerAssigned); b != nil {
		out.SendNotificationWhenNoLongerAssigned = b
	}
	if b := helpers.OptionalBoolPointer(plan.AutoRegisterManagedUsers); b != nil {
		out.AutoRegisterManagedUsers = b
	}
	if s := helpers.OptionalStringPointer(plan.SiteID); s != nil {
		out.SiteID = s
	}
	return out
}

// buildTokenRotationPatch converts the Terraform plan + a TrimSpaced base64
// service token string into the SDK PATCH payload used when the user bumps
// `service_token_wo_version`. Carries the new token alongside the full
// metadata payload so the rotation is atomic.
func buildTokenRotationPatch(plan VolumePurchasingLocationResourceModel, trimmedToken string) *pro.VolumePurchasingLocationPatch {
	out := buildMetadataPatch(plan)
	out.ServiceToken = &trimmedToken
	return out
}
