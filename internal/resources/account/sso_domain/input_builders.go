// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// buildDomainRequest converts the Terraform plan model into the claim payload.
//
// The payload carries the domain name and nothing else, and there is no update
// counterpart: Jamf Account exposes no way to modify a claim once it is made, so
// every attribute on this resource is RequiresReplace and this builder serves
// only Create.
func buildDomainRequest(plan DomainResourceModel) *account.DomainRequest {
	return &account.DomainRequest{
		Domain: plan.Domain.ValueString(),
	}
}
