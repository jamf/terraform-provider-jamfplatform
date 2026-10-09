// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// resourceAttributeNames is every attribute the resource schema declares, so a
// rename shows up as a test failure rather than as a silent state migration.
var resourceAttributeNames = []string{
	"id",
	"name",
	"internal_name",
	"connection_type",
	"hosting_region",
	"auth_method",
	"client_id",
	"client_secret",
	"client_secret_wo_version",
	"scopes",
	"pkce",
	"send_nonce",
	"sync_attributes_at_login",
	"omit_login_hint",
	"custom_username_claim_name",
	"username_domain",
	"attribute_map",
	"group_name_filter",
	"session_duration_minutes",
	"inactivity_timeout_minutes",
	"domains",
	"enabled_products",
	"enabled_environments",
	"enabled_product_names",
	"ticket_url",
	"consent_flow",
	"easy_config",
	"generic_oidc",
	"entra",
	"okta",
	"google_workspace",
	"timeouts",
}

// settingsBlocks is the four per-family blocks, exactly one of which may be set.
var settingsBlocks = []string{"generic_oidc", "entra", "okta", "google_workspace"}

// resourceSchema returns the resource schema.
func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	(&ConnectionResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestConnectionResource_Metadata(t *testing.T) {
	r := NewConnectionResource()
	var resp resource.MetadataResponse
	r.(*ConnectionResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_account_sso_connection" {
		t.Errorf("type name = %q, want %q", resp.TypeName, "jamfplatform_account_sso_connection")
	}
}

func TestConnectionResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range resourceAttributeNames {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(s.Attributes) != len(resourceAttributeNames) {
		t.Errorf("the schema declares %d attributes and this test knows about %d — one of them was added without a name here",
			len(s.Attributes), len(resourceAttributeNames))
	}
}

// TestConnectionResource_ImmutableAttributes pins the two attributes an update
// cannot change. Both are spec-derived rather than wire-verified — the write path
// is refused for every request, so the refusal was never observed — and both
// would otherwise plan an in-place change that could not succeed.
func TestConnectionResource_ImmutableAttributes(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"connection_type", "hosting_region"} {
		attribute, ok := s.Attributes[name].(rschema.StringAttribute)
		if !ok {
			t.Fatalf("%s is not a string attribute", name)
		}
		found := false
		for _, modifier := range attribute.PlanModifiers {
			if strings.Contains(modifier.Description(context.Background()), "destroy and recreate") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not force a replacement", name)
		}
	}
}

// TestConnectionResource_SecretIsWriteOnly pins the plaintext-secret rule. A
// secret held in state leaks to anyone who can read the state file, and marking
// it sensitive only redacts the printed plan.
func TestConnectionResource_SecretIsWriteOnly(t *testing.T) {
	s := resourceSchema(t)

	secret, ok := s.Attributes["client_secret"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("client_secret is not a string attribute")
	}
	if !secret.WriteOnly {
		t.Error("client_secret must be write-only so the plaintext never reaches state")
	}
	if !secret.Sensitive {
		t.Error("client_secret must be sensitive so it is redacted in printed output")
	}
	if secret.Computed {
		t.Error("client_secret must not be read back — Jamf never returns it")
	}

	if _, ok := s.Attributes["client_secret_wo_version"].(rschema.Int64Attribute); !ok {
		t.Error("client_secret needs its rotation companion — a write-only value alone gives Terraform nothing to act on")
	}
}

// TestConnectionResource_SettingsBlocksAreOptionalOnly pins
// STYLE_GUIDE §`SingleNestedAttribute` blocks. A block modelled as a typed
// pointer cannot be read back as computed: the framework has no way to express
// "unknown" in one, and the apply fails with a conversion error rather than
// anything a reader could act on.
func TestConnectionResource_SettingsBlocksAreOptionalOnly(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range append(settingsBlocks, "group_name_filter") {
		block, ok := s.Attributes[name].(rschema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is not a single nested attribute", name)
		}
		if !block.Optional {
			t.Errorf("%s must be optional", name)
		}
		if block.Computed {
			t.Errorf("%s must not be computed — a typed-pointer block cannot carry an unknown value", name)
		}
	}
}

// TestConnectionResource_ReadOnlyAttributes pins what Jamf owns. Each of these
// would otherwise invite an operator to set a value that is either derived,
// managed by Jamf, or the only half of an assignment that can be read.
func TestConnectionResource_ReadOnlyAttributes(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"id", "internal_name", "enabled_product_names", "ticket_url", "consent_flow", "easy_config"} {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("missing attribute %q", name)
		}
		if !attribute.IsComputed() {
			t.Errorf("%s must be read-only", name)
		}
		if attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%s must not be settable", name)
		}
	}
}

// TestConnectionResource_OktaAddressesAreReadOnly pins the four addresses Jamf
// works out from the org domain. Offering them would invite an operator to
// declare a value Jamf derives, and sending a stale copy back would be sending
// Jamf its own derivation as an instruction.
func TestConnectionResource_OktaAddressesAreReadOnly(t *testing.T) {
	s := resourceSchema(t)

	okta := s.Attributes["okta"].(rschema.SingleNestedAttribute)
	for _, name := range []string{"issuer_url", "authorization_endpoint", "token_endpoint", "jwks_uri"} {
		attribute, ok := okta.Attributes[name]
		if !ok {
			t.Fatalf("okta is missing %q", name)
		}
		if !attribute.IsComputed() || attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("okta.%s must be read-only — Jamf works it out from the domain", name)
		}
	}
	if !okta.Attributes["domain"].IsRequired() {
		t.Error("okta.domain must be required — it is the only part an operator sets")
	}
}

// TestConnectionResource_EntraBasicProfileIsReadOnly pins the one Entra option
// that is not a choice: the console renders it ticked and greyed out because it is
// always on, so offering an attribute an operator cannot change would be a lie.
func TestConnectionResource_EntraBasicProfileIsReadOnly(t *testing.T) {
	s := resourceSchema(t)

	entra := s.Attributes["entra"].(rschema.SingleNestedAttribute)
	basicProfile, ok := entra.Attributes["basic_profile"]
	if !ok {
		t.Fatal("entra is missing basic_profile")
	}
	if !basicProfile.IsComputed() || basicProfile.IsOptional() {
		t.Error("entra.basic_profile must be read-only — it is always on")
	}
}

// TestConnectionResource_EntraDomainsTakeNoFormatCheck pins a deliberate
// omission. Across the six real Entra connections read, these two hold an
// onmicrosoft host, a plain company domain, a bare tenant identifier and a full
// Microsoft sign-in address — so anything stricter than non-empty would refuse
// working configuration.
func TestConnectionResource_EntraDomainsTakeNoFormatCheck(t *testing.T) {
	s := resourceSchema(t)

	entra := s.Attributes["entra"].(rschema.SingleNestedAttribute)
	for _, name := range []string{"domain", "tenant_domain"} {
		attribute := entra.Attributes[name].(rschema.StringAttribute)
		for _, v := range attribute.Validators {
			if strings.Contains(v.Description(context.Background()), "bare domain") {
				t.Errorf("entra.%s carries a domain-shape check, which would refuse a real value", name)
			}
		}
	}
}

// TestConnectionResource_DomainsRequireAtLeastOne pins the one collection
// constraint Jamf does report, and reports by name: a connection cannot be
// created without a domain.
func TestConnectionResource_DomainsRequireAtLeastOne(t *testing.T) {
	s := resourceSchema(t)

	domains, ok := s.Attributes["domains"].(rschema.SetAttribute)
	if !ok {
		t.Fatal("domains is not a set attribute")
	}
	if !domains.Required {
		t.Error("domains must be required")
	}
	if len(domains.Validators) == 0 {
		t.Fatal("domains carries no validators")
	}
}

// TestConnectionResource_NoCallbackAddressAttribute pins the deliberate absence.
// The console shows a callback address and Jamf's own data does not carry it, so
// deriving one here would mean publishing a value that could silently go wrong and
// break sign-in for anyone who trusted it.
func TestConnectionResource_NoCallbackAddressAttribute(t *testing.T) {
	s := resourceSchema(t)

	for name := range s.Attributes {
		if strings.Contains(name, "callback") {
			t.Errorf("the schema declares %q; the callback address appears nowhere in Jamf's data and must not be invented", name)
		}
	}
}

// TestConnectionResource_IdentitySchema pins the import key. Unlike the sibling
// SSO domain construct, a connection is read by identifier and keeps it — and the
// stored name would be the wrong key anyway, since two connections can answer to
// the same one.
func TestConnectionResource_IdentitySchema(t *testing.T) {
	var resp resource.IdentitySchemaResponse
	(&ConnectionResource{}).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if len(resp.IdentitySchema.Attributes) != 1 {
		t.Fatalf("the identity declares %d attributes, want one", len(resp.IdentitySchema.Attributes))
	}
	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Error("the identity must be the connection identifier")
	}
}

// jargonPattern matches the plumbing vocabulary a user-facing description must
// not carry, per STYLE_GUIDE §User-facing descriptions are UI-aligned, not
// wire-aligned. It is deliberately narrower than the pre-commit grep: that one
// matches substrings and so fires on ordinary English, whereas this asserts on
// whole words a description has no business using.
var jargonPattern = regexp.MustCompile(`(?i)\b(api|wire|sdk|endpoints?|payload|http|json response|put|post|delete)\b`)

// jargonExemptions are the whole words the pattern above catches that are
// nonetheless the right words here. Each is the practitioner's own vocabulary
// rather than Jamf's plumbing: the three OpenID Connect addresses are called
// endpoints by every provider's own configuration screen and by the discovery
// document an operator copies them out of, and "API" appears only inside the
// console labels this provider is required to quote verbatim.
var jargonExemptions = []string{
	"endpoint",
	"endpoints",
	"api",
}

// TestConnectionResource_DescriptionsAvoidPlumbingVocabulary keeps the published
// documentation in the console's language. A description is read beside the Jamf
// Account console, so it should name what the operator sees rather than how the
// provider talks to Jamf.
func TestConnectionResource_DescriptionsAvoidPlumbingVocabulary(t *testing.T) {
	s := resourceSchema(t)

	checked := 0
	for name, attribute := range s.Attributes {
		description := attribute.GetMarkdownDescription()
		if description == "" {
			continue
		}
		checked++
		for _, match := range jargonPattern.FindAllString(description, -1) {
			if isExemptJargon(match) {
				continue
			}
			t.Errorf("the description of %q uses %q, which is plumbing vocabulary:\n%s", name, match, description)
		}
	}
	if checked == 0 {
		t.Fatal("no descriptions were checked")
	}
}

// isExemptJargon reports whether a matched word is one of the exemptions.
func isExemptJargon(match string) bool {
	lowered := strings.ToLower(match)
	return slices.Contains(jargonExemptions, lowered)
}

// TestConnectionResource_DescriptionsQuoteTheConsoleLabels pins
// STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI for every renamed
// attribute: the description has to lead with the exact console label, so a plan
// reviewer can match it against the screen.
func TestConnectionResource_DescriptionsQuoteTheConsoleLabels(t *testing.T) {
	s := resourceSchema(t)

	renamed := map[string]string{
		"hosting_region":             "Hosting region",
		"auth_method":                "Connection auth method",
		"pkce":                       "PKCE configuration",
		"sync_attributes_at_login":   "Sync at each login",
		"omit_login_hint":            "Omit `login_hint` IdP parameter",
		"session_duration_minutes":   "Session duration (minutes)",
		"inactivity_timeout_minutes": "Inactivity timeout (minutes)",
		"domains":                    "Associated Domains",
		"enabled_products":           "Applications",
	}

	for name, label := range renamed {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("missing attribute %q", name)
		}
		if !strings.Contains(attribute.GetMarkdownDescription(), "**\""+label+"\"**") {
			t.Errorf("the description of %q does not lead with the console label %q:\n%s",
				name, label, attribute.GetMarkdownDescription())
		}
	}
}

// TestConnectionResource_DescriptionStatesTheUpdateFault pins that the resource
// says plainly what an operator will otherwise discover by failing an apply, and
// that it scopes the fault to what is actually broken. Creating works, so a
// description claiming otherwise would talk practitioners out of using the
// resource for the one write it can do. Drop this assertion when Jamf fixes the
// endpoint — and the description with it.
func TestConnectionResource_DescriptionStatesTheUpdateFault(t *testing.T) {
	s := resourceSchema(t)

	description := s.MarkdownDescription
	for _, want := range []string{
		"cannot currently apply a change to an existing connection",
		"Creating, reading, listing and destroying all work",
	} {
		if !strings.Contains(description, want) {
			t.Errorf("the resource description does not mention %q", want)
		}
	}
	if strings.Contains(description, "unable to create") {
		t.Error("the resource description still claims creates are refused, which they are not")
	}
}

// TestConnectionResource_DescriptionStatesTheDriftBlindness pins the other thing
// an operator has to be told rather than find out: two collections are written
// and never read back, so Terraform cannot notice a change made outside it.
func TestConnectionResource_DescriptionStatesTheDriftBlindness(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"enabled_products", "enabled_environments"} {
		description := s.Attributes[name].GetMarkdownDescription()
		if !strings.Contains(description, "Configuration-authoritative") &&
			!strings.Contains(description, "configuration-authoritative") {
			t.Errorf("the description of %q does not say it is configuration-authoritative:\n%s", name, description)
		}
	}
	if !strings.Contains(s.Attributes["enabled_products"].GetMarkdownDescription(), "cannot recover this") {
		t.Error("the description of enabled_products does not say an import cannot recover it")
	}
}

// TestConnectionResource_ManagedAccountIsDocumentedAsUndetectable pins the one
// "cannot be managed" case this provider cannot enforce. A partner-managed
// account identifier is a write-only field of a write-only collection, so nothing
// this provider reads would reveal it — which makes the description the only
// place the trap can be recorded.
func TestConnectionResource_ManagedAccountIsDocumentedAsUndetectable(t *testing.T) {
	s := resourceSchema(t)

	products := s.Attributes["enabled_products"].(rschema.SetNestedAttribute)
	managed, ok := products.NestedObject.Attributes["managed_account_id"]
	if !ok {
		t.Fatal("enabled_products is missing managed_account_id")
	}
	description := managed.GetMarkdownDescription()
	if !strings.Contains(description, "Nothing Jamf Account returns reveals it") {
		t.Errorf("the description does not say Terraform cannot detect it:\n%s", description)
	}
}

// --- data source ---

// dataSourceAttributeNames is every attribute the singular data source declares.
// It is the resource's set less the four things that make no sense on a read: the
// write-only secret and its rotation companion, the two collections nothing reads
// back, and the resource's own split between a configured and a stored name.
var dataSourceAttributeNames = []string{
	"id",
	"name",
	"connection_type",
	"hosting_region",
	"auth_method",
	"client_id",
	"scopes",
	"pkce",
	"send_nonce",
	"sync_attributes_at_login",
	"omit_login_hint",
	"custom_username_claim_name",
	"username_domain",
	"attribute_map",
	"group_name_filter",
	"session_duration_minutes",
	"inactivity_timeout_minutes",
	"domains",
	"enabled_product_names",
	"ticket_url",
	"consent_flow",
	"easy_config",
	"generic_oidc",
	"entra",
	"okta",
	"google_workspace",
	"timeouts",
}

// dataSourceSchema returns the singular data source schema.
func dataSourceSchema(t *testing.T) dsschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	(&ConnectionDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestConnectionDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	(&ConnectionDataSource{}).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_account_sso_connection" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestConnectionDataSource_Schema(t *testing.T) {
	s := dataSourceSchema(t)

	for _, name := range dataSourceAttributeNames {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(s.Attributes) != len(dataSourceAttributeNames) {
		t.Errorf("the schema declares %d attributes and this test knows about %d",
			len(s.Attributes), len(dataSourceAttributeNames))
	}
}

// TestConnectionDataSource_CarriesNoSecret pins that the write-only secret is
// absent. Jamf never returns it, so an attribute for it would be permanently
// empty and would invite an operator to think otherwise.
func TestConnectionDataSource_CarriesNoSecret(t *testing.T) {
	s := dataSourceSchema(t)

	for _, name := range []string{"client_secret", "client_secret_wo_version", "enabled_products", "enabled_environments", "internal_name"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("the data source declares %q, which no read can supply", name)
		}
	}
}

// TestConnectionDataSource_LookupKeysAreExclusive pins that exactly one of the
// two keys is required. The identifier is exact but opaque; the name is what a
// practitioner knows but is not a unique key, so neither alone would do and both
// together would be ambiguous.
func TestConnectionDataSource_LookupKeysAreExclusive(t *testing.T) {
	validators := (&ConnectionDataSource{}).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("the data source declares %d configuration validators, want one", len(validators))
	}

	s := dataSourceSchema(t)
	for _, name := range []string{"id", "name"} {
		attribute := s.Attributes[name]
		if !attribute.IsOptional() || !attribute.IsComputed() {
			t.Errorf("%q must be optional and reported, so either can be the key", name)
		}
	}
}

// --- list resource ---

func TestConnectionListResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	(&ConnectionListResource{}).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_account_sso_connection" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

// TestConnectionListResource_TakesNoFilter pins that the list configuration holds
// nothing but the shared timeouts attribute, because Jamf exposes no search
// arguments on the connection collection.
func TestConnectionListResource_TakesNoFilter(t *testing.T) {
	var resp list.ListResourceSchemaResponse
	(&ConnectionListResource{}).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if _, ok := resp.Schema.Attributes["timeouts"]; !ok || len(resp.Schema.Attributes) != 1 {
		t.Errorf("the list configuration declares %v, want only timeouts", resp.Schema.Attributes)
	}
	for _, want := range []string{"admin-consent", "one extra read per connection"} {
		if !strings.Contains(resp.Schema.Description, want) {
			t.Errorf("the list description does not mention %q:\n%s", want, resp.Schema.Description)
		}
	}
}
