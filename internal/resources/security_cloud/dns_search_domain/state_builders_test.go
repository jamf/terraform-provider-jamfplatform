// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

func TestAssignSearchDomainResourceModel(t *testing.T) {
	cases := map[string]string{
		"ordinary":          "corp.example.com",
		"single label":      "corp",
		"case preserved":    "CORP.Example.COM",
		"trailing root dot": "example.com.",
		"empty":             "",
	}
	for name, suffix := range cases {
		t.Run(name, func(t *testing.T) {
			model := &SearchDomainResourceModel{}
			assignSearchDomainResourceModel(model, &securitycloud.SearchDomain{Suffix: suffix})
			if model.DomainName.ValueString() != suffix {
				t.Errorf("DomainName = %q, want %q", model.DomainName.ValueString(), suffix)
			}
			if model.DomainName.IsNull() {
				t.Error("DomainName must never be null; domain_name is Required")
			}
		})
	}
}

// TestAssignSearchDomainResourceModelLeavesIDAlone pins the division of labour: the
// CRUD handler stamps helpers.SingletonID, and an assigner that also wrote it would
// give the value two sources.
func TestAssignSearchDomainResourceModelLeavesIDAlone(t *testing.T) {
	model := &SearchDomainResourceModel{ID: types.StringValue("pre-existing")}
	assignSearchDomainResourceModel(model, &securitycloud.SearchDomain{Suffix: "corp.example.com"})
	if model.ID.ValueString() != "pre-existing" {
		t.Errorf("assigner clobbered ID: got %q", model.ID.ValueString())
	}
}

func TestAssignSearchDomainDataSourceModel(t *testing.T) {
	model := &SearchDomainDataSourceModel{ID: types.StringValue("pre-existing")}
	assignSearchDomainDataSourceModel(model, &securitycloud.SearchDomain{Suffix: "corp.example.com"})
	if model.DomainName.ValueString() != "corp.example.com" {
		t.Errorf("DomainName = %q", model.DomainName.ValueString())
	}
	if model.ID.ValueString() != "pre-existing" {
		t.Errorf("assigner clobbered ID: got %q", model.ID.ValueString())
	}
}
