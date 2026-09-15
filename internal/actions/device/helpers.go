// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package deviceactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	daSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"
	devSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// deviceAction shares Configure logic across device action implementations.
type deviceAction struct {
	devices *devSDK.Client
	actions *daSDK.Client
}

// secretAttrNote is appended to the description of every action attribute that
// carries a user-supplied secret (here, the Find My PIN).
//
// Such an attribute can be given NEITHER protection the framework offers:
//
//   - Sensitive does not exist on action schema attributes. The field is absent,
//     and IsSensitive() is hardcoded to return false ("action schema attributes
//     cannot be Sensitive").
//   - WriteOnly exists on action attributes and compiles, but setting it makes
//     the attribute impossible to use. Action config validation hardcodes
//     WriteOnlyAttributesAllowed: false (fwserver/server_validateactionconfig.go,
//     on the stated grounds that the capability "is only valid for resource
//     schemas"), while the shared SchemaValidate still applies the resource
//     write-only gate (fwserver/attribute_validation.go). Any non-null value for
//     a write-only action attribute therefore fails validation with "WriteOnly
//     Attribute Not Allowed".
//
// Note this is unconditional and version-independent: the capability is a
// hardcoded false, not a negotiated one, so no Terraform upgrade fixes it —
// despite the diagnostic blaming "Terraform 1.11 and later". Observed on
// framework v1.19.0 with Terraform v1.15.8. The framework is internally
// inconsistent here (the field is offered and documented but cannot be used),
// which is worth raising upstream.
//
// So the choice is an attribute that works with its value visible, or one nobody
// can use. We take the former and say so here. Do not re-add WriteOnly: it
// breaks every configuration that sets the attribute, and no device-less test
// catches it. TestActionAttributes_AreNotWriteOnly guards this.
const secretAttrNote = " This value appears in Terraform plan output and should be supplied from a variable or secret store rather than committed."

// deviceTargetAttributes returns the device_id / serial_number selector shared
// by every Platform device action.
//
// Exactly-one-of is enforced by deviceTargetConfigValidators rather than by
// per-attribute ConflictsWith, so that supplying NO identifier also fails at
// plan time instead of part-way through the apply.
func deviceTargetAttributes() map[string]actionschema.Attribute {
	return map[string]actionschema.Attribute{
		"device_id": actionschema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Jamf Pro Management ID. Set exactly one of this or `serial_number`.",
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
		"serial_number": actionschema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Device serial number (case-sensitive). Requires **Device Inventory API** access when set. Set exactly one of this or `device_id`.",
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
	}
}

// deviceTargetConfigValidators is the plan-time counterpart to
// deviceTargetAttributes: exactly one of device_id / serial_number selects the
// device.
func deviceTargetConfigValidators() []action.ConfigValidator {
	return []action.ConfigValidator{
		actionvalidator.ExactlyOneOf(
			path.MatchRoot("device_id"),
			path.MatchRoot("serial_number"),
		),
	}
}

// configure binds the provider-supplied client to the action.
func (a *deviceAction) configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *providerdata.Data, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	resp.Diagnostics.Append(pd.RequireScope("Jamf Platform device actions", providerdata.DeviceActionsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a.devices = devSDK.New(pd.Client)
	a.actions = daSDK.New(pd.Client)
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *deviceAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.actions != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Platform client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}

// resolveDeviceIdentifier ensures exactly one device identifier is provided and returns the device ID.
func (a *deviceAction) resolveDeviceIdentifier(ctx context.Context, resp *action.InvokeResponse, deviceIDAttr, serialNumberAttr types.String) (string, bool) {
	hasDeviceID := helpers.IsConfiguredValue(deviceIDAttr)
	hasSerial := helpers.IsConfiguredValue(serialNumberAttr)

	switch {
	case hasDeviceID && hasSerial:
		resp.Diagnostics.AddError(
			"Multiple Device Identifiers Provided",
			"Specify only one of device_id or serial_number when invoking this action.",
		)
		return "", false
	case hasDeviceID:
		return deviceIDAttr.ValueString(), true
	case hasSerial:
		serial := serialNumberAttr.ValueString()
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Resolving serial number %s", serial)})

		deviceID, err := a.devices.ResolveDeviceIDBySerialNumber(ctx, serial)
		if err != nil {
			resp.Diagnostics.AddError(
				"Device Lookup Failed",
				fmt.Sprintf("Unable to resolve serial number %s to a device ID: %s", serial, helpers.APIErrorDetail(err)),
			)
			return "", false
		}

		return deviceID, true
	default:
		resp.Diagnostics.AddError(
			"Missing Device Identifier",
			"Specify either device_id or serial_number to select the device.",
		)
		return "", false
	}
}
