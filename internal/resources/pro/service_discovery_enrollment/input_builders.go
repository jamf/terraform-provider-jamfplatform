// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package service_discovery_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// wellKnownSettingsFromList decodes the well_known_setting types.List into row
// models. Returns an empty (non-nil) slice when the list is null or unknown.
func wellKnownSettingsFromList(ctx context.Context, list types.List) ([]wellKnownSettingModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return []wellKnownSettingModel{}, diags
	}
	out := make([]wellKnownSettingModel, 0, len(list.Elements()))
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out, diags
}

// buildServiceDiscoveryEnrollmentInput projects the declared rows into the SDK PUT
// payload.
//
// The PUT is a by-key MERGE (wire-probed 2026-06-13, spike/SERVICE_DISCOVERY_
// ENROLLMENT_SPIKE.md): only the rows sent are touched; an omitted row's value is
// preserved server-side, and an empty array is an accepted no-op. The provider
// therefore sends exactly the rows the user declares. org_name is a read-only echo
// (the server returns the canonical AxM org name and ignores any value sent), so it
// is never included in the request.
func buildServiceDiscoveryEnrollmentInput(models []wellKnownSettingModel) *pro.WellKnownSettingsRequest {
	out := make([]pro.WellKnownSetting, 0, len(models))
	for _, m := range models {
		out = append(out, pro.WellKnownSetting{
			ServerUUID:     m.ServerUUID.ValueString(),
			EnrollmentType: m.EnrollmentType.ValueString(),
		})
	}
	return &pro.WellKnownSettingsRequest{WellKnownSettings: out}
}
