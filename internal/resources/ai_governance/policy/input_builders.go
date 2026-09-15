// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"encoding/json"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildCreateRequest assembles the create payload.
func buildCreateRequest(model *policyModel) *aigovernance.CreatePolicyRequest {
	return &aigovernance.CreatePolicyRequest{
		Name:          model.Name.ValueString(),
		Description:   createDescription(model),
		ToolID:        model.ToolID.ValueString(),
		SchemaVersion: model.SchemaVersion.ValueString(),
		Settings:      settingsPayload(model),
	}
}

// buildUpdateRequest assembles the update payload.
//
// Name and description are optional on the wire and mean "leave unchanged" when omitted, but they
// are always sent: Terraform's model is the desired state in full, so a name or description the
// operator removed from the configuration must be written back, not silently preserved. The update
// is a PATCH, and probing it on 2026-08-30 established that an explicit blank is the only body it
// clears a description on — an absent key and a JSON null both preserve the stored value — so an
// unset description is sent as "". Schema version and settings are mandatory on every update —
// omitting either is refused — which makes settings a full replace.
func buildUpdateRequest(model *policyModel) *aigovernance.UpdatePolicyRequest {
	name := model.Name.ValueString()
	return &aigovernance.UpdatePolicyRequest{
		Name:          &name,
		Description:   updateDescription(model),
		SchemaVersion: model.SchemaVersion.ValueString(),
		Settings:      settingsPayload(model),
	}
}

// createDescription omits an unset description: a create has nothing to preserve.
func createDescription(model *policyModel) *string {
	if !helpers.IsConfiguredValue(model.Description) {
		return nil
	}
	value := model.Description.ValueString()
	return &value
}

// updateDescription always sends a value. Omitting the field means "leave unchanged" on this PATCH
// endpoint and so does a JSON null, and an explicit blank is the only body the platform clears on,
// so a description the operator removed from the configuration has to be sent as "" rather than
// dropped. Sending it back unchanged is not an option: `description` is Optional-only, so a planned
// null compared against a preserved string fails the apply with "inconsistent result after apply"
// and leaves the configuration un-appliable until a description is put back. The house
// Optional+Computed shape (STYLE_GUIDE §Full-replace endpoints) would also settle that, but it
// would give up the ability to clear the field, which this endpoint does support.
func updateDescription(model *policyModel) *string {
	value := ""
	if helpers.IsConfiguredValue(model.Description) {
		value = model.Description.ValueString()
	}
	return &value
}

// settingsPayload returns the settings body to send. The attribute is required and validated as a
// JSON object before this runs, so an unset value can only mean an empty settings object.
func settingsPayload(model *policyModel) json.RawMessage {
	if !helpers.IsConfiguredValue(model.SettingsJSON) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(model.SettingsJSON.ValueString())
}
