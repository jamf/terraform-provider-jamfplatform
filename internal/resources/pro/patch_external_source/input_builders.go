// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_external_source

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildPatchExternalSourceInput converts the Terraform plan model into an SDK
// PatchExternalSource payload. Name is required by the schema so we always send
// it as a non-nil pointer. ID is omitted on write — Create uses path id="0" and
// Update derives it from state. Optional/Optional+Computed fields that are
// null or unknown become nil pointers so the SDK's omitempty tag drops them from
// the wire (leaving the server to keep / default the value) — except `port`,
// whose clear token is derived from priorPort by portForWrite.
//
// priorPort is the port Terraform last recorded for this object: null on
// Create, the prior state's value on Update.
func buildPatchExternalSourceInput(plan PatchExternalSourceResourceModel, priorPort types.Int64) *proclassic.PatchExternalSource {
	return &proclassic.PatchExternalSource{
		Name:                         helpers.OptionalStringPointer(plan.Name),
		Enabled:                      helpers.OptionalBoolPointer(plan.Enabled),
		HostName:                     helpers.OptionalStringPointer(plan.HostName),
		Port:                         portForWrite(plan.Port, priorPort),
		SslEnabled:                   helpers.OptionalBoolPointer(plan.SslEnabled),
		CertificateValidationEnabled: helpers.OptionalBoolPointer(plan.CertificateValidationEnabled),
	}
}

// portForWrite encodes `port` for the classic /patchexternalsources PUT, which
// merges field by field: an omitted <port> keeps the stored value, so a config
// that drops the attribute has to send something that clears it (issue #384).
// The wire type is an integer, so the only empty the SDK can send is
// <port>0</port>; wire-probed 2026-09-06 on Jamf Pro 11.31.1 the server accepts
// 0 and stores it (GET echoes <port>0</port>, which the state builder already
// folds to null), does not default it from ssl_enabled, and refuses -1 with
// `409 remotePort - Port number is invalid`. A never-set port is stored
// differently — GET echoes an empty <port/> — and whether the patch service
// treats 0 and empty alike at run time could not be verified here, so 0 is
// sent only when there is a recorded value to clear: a null plan beside a
// null priorPort omits the element and leaves a fresh source's port empty as
// before. A configured port is sent as-is; unknown is omitted.
func portForWrite(planned, prior types.Int64) *int {
	if planned.IsNull() && helpers.IsConfiguredValue(prior) {
		return new(0)
	}
	return helpers.OptionalInt64Pointer(planned)
}
