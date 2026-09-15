// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignSearchDomainResourceModel copies the stored search domain into the resource
// model.
//
// It deliberately does not touch the model's ID: the CRUD handler stamps
// helpers.SingletonID, and an assigner that also wrote it would give two places for
// the value to come from. state_builders_test.go pins that.
func assignSearchDomainResourceModel(model *SearchDomainResourceModel, got *securitycloud.SearchDomain) {
	model.DomainName = types.StringValue(got.Suffix)
}

// assignSearchDomainDataSourceModel copies the stored search domain into the data
// source model.
func assignSearchDomainDataSourceModel(model *SearchDomainDataSourceModel, got *securitycloud.SearchDomain) {
	model.DomainName = types.StringValue(got.Suffix)
}
