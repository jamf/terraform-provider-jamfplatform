// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// applyPolicyToState copies a fetched policy onto the model, leaving the operator-facing `publish`
// flag and the timeouts block alone.
//
// The settings string is written as the platform returned it. The framework then applies the
// attribute's JSON semantic equality against the prior value, so a policy authored with
// `jsonencode` — which sorts keys — does not drift against a payload the platform stores in the
// order it was first written.
//
// The description is reconciled rather than copied. Clearing one means sending "" — the only body
// this PATCH endpoint clears on — and the platform then stores and returns that blank verbatim, so
// a straight copy would put "" into state where the configuration says nothing and fail the apply
// with "inconsistent result after apply". A blank therefore reads back as null, except where the
// operator wrote `description = ""` themselves, which is preserved so that spelling settles too.
func applyPolicyToState(model *policyModel, detail *aigovernance.PolicyDetail) error {
	settings, err := renderSettings(detail.Settings)
	if err != nil {
		return err
	}

	model.ID = types.StringValue(detail.ID)
	model.Name = types.StringValue(detail.Name)
	model.Description = helpers.ReconcileOptionalStringPointer(detail.Description, model.Description)
	model.ToolID = types.StringValue(detail.ToolID)
	model.SchemaVersion = types.StringValue(detail.SchemaVersion)
	model.SettingsJSON = settings
	model.PublishedVersion = optionalInt64(detail.CurrentVersionNumber)
	model.HasDraft = types.BoolValue(detail.HasDraft)
	model.SchemaDrift = types.BoolValue(detail.SchemaDrift)
	model.CreatedAt = types.StringValue(detail.CreatedAt.UTC().Format(time.RFC3339))
	model.UpdatedAt = types.StringValue(detail.UpdatedAt.UTC().Format(time.RFC3339))
	return nil
}

// renderSettings turns the stored settings into the attribute's string form, rejecting a body that
// is not a JSON object so a surprise from the platform surfaces as an error rather than as state
// Terraform can never reconcile.
func renderSettings(raw json.RawMessage) (jsonObjectValue, error) {
	if len(raw) == 0 {
		return newJSONObjectValue("{}"), nil
	}
	decoded, err := decodeJSON(string(raw))
	if err != nil {
		return newJSONObjectNull(), fmt.Errorf("decode stored settings: %w", err)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return newJSONObjectNull(), fmt.Errorf("stored settings are not a JSON object")
	}
	return newJSONObjectValue(string(raw)), nil
}

// resolvePublish keeps the operator's publish flag across a refresh. It is a behaviour flag with no
// wire field, so nothing in a fetched policy can restore it — and on import there is no prior value
// at all, where it has to land on the schema's default or the first plan after an import would show
// a change to an attribute the operator never wrote.
func resolvePublish(prior types.Bool) types.Bool {
	if prior.IsNull() || prior.IsUnknown() {
		return types.BoolValue(true)
	}
	return prior
}

// optionalInt64 maps a nullable wire integer onto a Terraform value.
func optionalInt64(value *int) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*value))
}
