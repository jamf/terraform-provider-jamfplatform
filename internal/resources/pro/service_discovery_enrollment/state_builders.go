// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package service_discovery_enrollment

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// modelFromWire flattens one SDK well-known setting into a row model. org_name is a
// read-only echo; nil/absent collapses to null.
func modelFromWire(w pro.WellKnownSetting) wellKnownSettingModel {
	return wellKnownSettingModel{
		ServerUUID:     types.StringValue(w.ServerUUID),
		EnrollmentType: types.StringValue(w.EnrollmentType),
		OrgName:        helpers.StringPointerValueOrNull(w.OrgName),
	}
}

// wellKnownSettingsListValue converts row models into a known types.List. A
// nil/empty slice yields a known EMPTY list (never null) so an explicit empty
// well_known_setting round-trips.
func wellKnownSettingsListValue(ctx context.Context, models []wellKnownSettingModel) (types.List, diag.Diagnostics) {
	if len(models) == 0 {
		return types.ListValueMust(wellKnownSettingObjectType(), []attr.Value{}), nil
	}
	return types.ListValueFrom(ctx, wellKnownSettingObjectType(), models)
}

// assignServiceDiscoveryEnrollmentResourceModel reconciles the GET response into the
// resource model.
//
// The endpoint is a server-keyed, by-key merge (the row set is fixed by the synced
// AxM orgs; the user only sets enrollment_type per known server_uuid). To keep the
// user-authored well_known_setting list stable, rows are emitted in the INCOMING
// plan/state order (the order the user wrote), each matched to its GET row by
// server_uuid:
//
//   - declared server_uuid present in GET → take the authoritative enrollment_type
//     and org_name from the wire.
//   - declared server_uuid ABSENT from GET → Jamf Pro silently dropped it (not a
//     synced AxM org). The row is retained with its planned enrollment_type and a
//     null org_name, and a warning is emitted — the server gives no error for an
//     unrecognized server_uuid, so this is the only signal the user gets.
//
// On first read / import (no prior list) every GET row is adopted in server order.
//
// The import signal is the prior list being null/unknown — NOT req.State.Raw.IsNull()
// in the CRUD handler. During the Read that follows `terraform import`, the framework
// has already seeded the singleton id into the state, so req.State.Raw is non-null; the
// framework tracks import via an internal private field, not via a null state (see
// terraform-plugin-framework server_readresource.go ImportBeforeReadKey). The prior
// well_known_setting list, however, is genuinely null on import (only the id was
// passed through), and always a known list after any Create/Update, so it is the
// reliable discriminator.
func assignServiceDiscoveryEnrollmentResourceModel(ctx context.Context, state *ServiceDiscoveryEnrollmentResourceModel, resp *pro.WellKnownSettingsResponse) diag.Diagnostics {
	var diags diag.Diagnostics

	wire := map[string]pro.WellKnownSetting{}
	var wireOrder []pro.WellKnownSetting
	if resp != nil {
		wireOrder = resp.WellKnownSettings
		for _, w := range resp.WellKnownSettings {
			wire[w.ServerUUID] = w
		}
	}

	adoptAll := state.WellKnownSetting.IsNull() || state.WellKnownSetting.IsUnknown()

	var models []wellKnownSettingModel
	if adoptAll {
		for _, w := range wireOrder {
			models = append(models, modelFromWire(w))
		}
	} else {
		prior, d := wellKnownSettingsFromList(ctx, state.WellKnownSetting)
		diags.Append(d...)
		for _, p := range prior {
			uuid := p.ServerUUID.ValueString()
			if w, ok := wire[uuid]; ok {
				models = append(models, modelFromWire(w))
				continue
			}
			diags.AddWarning(
				"Service discovery server UUID not recognized by Jamf Pro",
				fmt.Sprintf(
					"server_uuid %q does not match a synced Automated Device Enrollment (Apple Business/School Manager) "+
						"organization, so Jamf Pro ignored it. Verify the value against Settings > Automated Device Enrollment "+
						"> Server UUID. The row is kept in state but has no effect.",
					uuid,
				),
			)
			models = append(models, wellKnownSettingModel{
				ServerUUID:     p.ServerUUID,
				EnrollmentType: p.EnrollmentType,
				OrgName:        types.StringNull(),
			})
		}
	}

	list, d := wellKnownSettingsListValue(ctx, models)
	diags.Append(d...)
	state.WellKnownSetting = list
	return diags
}

// assignServiceDiscoveryEnrollmentDataSourceModel populates the data source model
// from a GET response. The data source surfaces every row Jamf Pro returns so users
// can discover the available server_uuids and current enrollment types.
func assignServiceDiscoveryEnrollmentDataSourceModel(ctx context.Context, state *ServiceDiscoveryEnrollmentDataSourceModel, resp *pro.WellKnownSettingsResponse) diag.Diagnostics {
	var models []wellKnownSettingModel
	if resp != nil {
		for _, w := range resp.WellKnownSettings {
			models = append(models, modelFromWire(w))
		}
	}
	list, diags := wellKnownSettingsListValue(ctx, models)
	state.WellKnownSetting = list
	return diags
}
