// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildAppRequestFormFieldInput converts the Terraform plan model into an SDK
// AppRequestFormInputField payload. Title and Priority are Required (always sent). The
// endpoint is a full-replace PUT, so Description is sent as a nil pointer when omitted
// (the server clears it) and as the literal value otherwise (an empty string round-trips
// faithfully). ID is omitted — the server mints it on create and the path carries it on
// update.
func buildAppRequestFormFieldInput(plan AppRequestFormFieldResourceModel) *pro.AppRequestFormInputField {
	return &pro.AppRequestFormInputField{
		Title:       plan.Title.ValueString(),
		Priority:    int(plan.Priority.ValueInt64()),
		Description: helpers.OptionalStringPointer(plan.Description),
	}
}
