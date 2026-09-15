// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildOnboardingInput converts the planned enabled flag and decoded onboarding
// item models into a full-replace PUT payload.
//
// Wire-probed 2026-06-11 (spike/MACOS_ONBOARDING_SPIKE.md): the /v1/onboarding PUT
// is full-replace — the body must carry the complete onboardingItems array, and an
// omitted item is dropped. Only entityId, selfServiceEntityType, and priority are
// sent; id and the entityName/scope/site echoes are server-derived (readOnly) and
// regenerated on each write, so they are left unset (omitempty). priority is stamped
// from the list index (1-based, wire-confirmed contiguous and accepted verbatim) so
// the user's onboarding_items order is the presentation order — the user never sets
// priority directly.
func buildOnboardingInput(enabled bool, items []onboardingItemModel) *pro.OnboardingConfiguration {
	out := make([]pro.OnboardingItem, 0, len(items))
	for idx := range items {
		priority := idx + 1
		out = append(out, pro.OnboardingItem{
			EntityID:              items[idx].EntityID.ValueString(),
			SelfServiceEntityType: items[idx].SelfServiceEntityType.ValueString(),
			Priority:              priority,
		})
	}
	return &pro.OnboardingConfiguration{
		Enabled:         enabled,
		OnboardingItems: out,
	}
}
