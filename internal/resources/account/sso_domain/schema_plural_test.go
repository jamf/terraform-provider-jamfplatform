// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

func TestDomainsDataSource_Metadata(t *testing.T) {
	d := NewDomainsDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*DomainsDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_account_sso_domains" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_account_sso_domains", resp.TypeName)
	}
}

func TestDomainsDataSource_Schema(t *testing.T) {
	d := NewDomainsDataSource()
	var resp datasource.SchemaResponse
	d.(*DomainsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "sso_domains", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	domains, ok := s.Attributes["sso_domains"].(dsschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("sso_domains must be a ListNestedAttribute, got %T", s.Attributes["sso_domains"])
	}
	if !s.Attributes["sso_domains"].IsComputed() {
		t.Error("sso_domains must be computed")
	}
	for _, name := range resourceAttributeNames {
		if name == "timeouts" {
			continue
		}
		if _, present := domains.NestedObject.Attributes[name]; !present {
			t.Errorf("sso_domains missing nested attribute %q", name)
		}
	}
}

// TestDomainsDataSource_CarriesNoAssignments pins the deliberate omission. The
// assignment lookup is keyed on one domain name at a time, so surfacing it here
// would cost one extra round trip per domain the organization holds.
func TestDomainsDataSource_CarriesNoAssignments(t *testing.T) {
	d := NewDomainsDataSource()
	var resp datasource.SchemaResponse
	d.(*DomainsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	domains, ok := resp.Schema.Attributes["sso_domains"].(dsschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("sso_domains must be a ListNestedAttribute, got %T", resp.Schema.Attributes["sso_domains"])
	}
	for _, name := range []string{"assigned_connections", "jamf_id_enabled"} {
		if _, present := domains.NestedObject.Attributes[name]; present {
			t.Errorf("sso_domains must not carry %q — it costs a round trip per domain", name)
		}
	}
}

// assertDocumentsEveryStatus checks a description names every verification status
// Jamf declares, so a value the SDK adds cannot go undocumented.
func assertDocumentsEveryStatus(t *testing.T, description string) {
	t.Helper()
	for _, value := range account.DomainStatusValues() {
		if !strings.Contains(description, value) {
			t.Errorf("description does not name the %q status:\n%s", value, description)
		}
	}
	for _, label := range verificationStatusUILabels {
		if !strings.Contains(description, label) {
			t.Errorf("description does not name the %q console label:\n%s", label, description)
		}
	}
}
