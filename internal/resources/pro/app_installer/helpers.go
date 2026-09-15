// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// deployment_type and update_behavior vocabularies, taken from the SDK's own
// generated helpers rather than restated as literals, so the OneOf validators
// and the documented value lists cannot drift from the API — see
// STYLE_GUIDE §"Enum values and error codes come from the SDK, not from
// literals". Aliased here because both the schema and the acceptance fixtures
// name individual values.
const (
	deploymentTypeInstallAutomatically = pro.AppTitleDeploymentDeploymentTypeInstallAutomatically
	deploymentTypeSelfService          = pro.AppTitleDeploymentDeploymentTypeSelfService

	updateBehaviorAutomatic = pro.AppTitleDeploymentUpdateBehaviorAutomatic
	updateBehaviorManual    = pro.AppTitleDeploymentUpdateBehaviorManual
)

// boolPointerOrFalse returns the bool value, treating null/unknown as false.
// Used for the Self Service booleans, which the server defaults to false and so
// are always emitted.
func boolPointerOrFalse(value types.Bool) *bool {
	v := value.ValueBool()
	if !helpers.IsConfiguredValue(value) {
		v = false
	}
	return &v
}

// optionalInt64Pointer collapses null/unknown TF Int64 to nil so an unset field
// is dropped from the wire payload (the server rejects a non-positive value on
// any notification interval/delay that is present).
func optionalInt64Pointer(value types.Int64) *int64 {
	if !helpers.IsConfiguredValue(value) {
		return nil
	}
	return new(value.ValueInt64())
}
