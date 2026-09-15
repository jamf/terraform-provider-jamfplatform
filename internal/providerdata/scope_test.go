// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// TestNewDerivesScopeFromClient covers the round-trip New relies on: the scope
// option the SDK client was built with must come back out of Client.Scope() as
// the matching ScopeKind, so the gate can never disagree with the header the
// client actually sends.
func TestNewDerivesScopeFromClient(t *testing.T) {
	tests := []struct {
		name string
		opts []jamfplatform.Option
		want ScopeKind
	}{
		{
			name: "no scope option is organization scope",
			want: ScopeOrganization,
		},
		{
			name: "environment option",
			opts: []jamfplatform.Option{jamfplatform.WithEnvironmentID("e-1")},
			want: ScopeEnvironment,
		},
		{
			name: "tenant option",
			opts: []jamfplatform.Option{jamfplatform.WithTenantID("t-1")},
			want: ScopeTenant,
		},
		{
			name: "an empty ID is not a scope",
			opts: []jamfplatform.Option{jamfplatform.WithTenantID("")},
			want: ScopeOrganization,
		},
		{
			name: "both options set follows the SDK, where environment wins",
			opts: []jamfplatform.Option{jamfplatform.WithTenantID("t-1"), jamfplatform.WithEnvironmentID("e-1")},
			want: ScopeEnvironment,
		},
		{
			name: "both options set, reversed order, still environment",
			opts: []jamfplatform.Option{jamfplatform.WithEnvironmentID("e-1"), jamfplatform.WithTenantID("t-1")},
			want: ScopeEnvironment,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret", tc.opts...)
			if got := New(c).Scope(); got != tc.want {
				t.Errorf("derived scope: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNewNilClient guards the nil path: scopeFromClient must not dereference a
// nil client, and the result has to be the scope that gates everything rather
// than one that passes.
func TestNewNilClient(t *testing.T) {
	if got := New(nil).Scope(); got != ScopeOrganization {
		t.Errorf("nil client: got %v, want ScopeOrganization", got)
	}
}

// TestRequireScope covers the gate every construct's Configure runs through:
// the configured scope must appear in the allowed set, and an organization-scoped
// credential is rejected everywhere until the account-level constructs exist.
func TestRequireScope(t *testing.T) {
	tests := []struct {
		name       string
		configured ScopeKind
		allowed    []ScopeKind
		wantError  bool
	}{
		{
			name:       "tenant satisfies tenant-or-environment",
			configured: ScopeTenant,
			allowed:    []ScopeKind{ScopeEnvironment, ScopeTenant},
		},
		{
			name:       "environment satisfies tenant-or-environment",
			configured: ScopeEnvironment,
			allowed:    []ScopeKind{ScopeEnvironment, ScopeTenant},
		},
		{
			name:       "organization satisfies neither",
			configured: ScopeOrganization,
			allowed:    []ScopeKind{ScopeEnvironment, ScopeTenant},
			wantError:  true,
		},
		{
			name:       "tenant does not satisfy environment-only",
			configured: ScopeTenant,
			allowed:    []ScopeKind{ScopeEnvironment},
			wantError:  true,
		},
		{
			name:       "environment satisfies environment-only",
			configured: ScopeEnvironment,
			allowed:    []ScopeKind{ScopeEnvironment},
		},
		{
			name:       "organization satisfies organization-only",
			configured: ScopeOrganization,
			allowed:    []ScopeKind{ScopeOrganization},
		},
		{
			name:       "environment does not satisfy organization-only",
			configured: ScopeEnvironment,
			allowed:    []ScopeKind{ScopeOrganization},
			wantError:  true,
		},
		{
			name:       "tenant does not satisfy organization-only",
			configured: ScopeTenant,
			allowed:    []ScopeKind{ScopeOrganization},
			wantError:  true,
		},
		{
			name:       "an empty allowed set gates nothing",
			configured: ScopeOrganization,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pd := &Data{scope: tc.configured}
			diags := pd.RequireScope("jamfplatform_pro_test", tc.allowed...)
			if got := diags.HasError(); got != tc.wantError {
				t.Errorf("HasError: got %v, want %v (%v)", got, tc.wantError, diags)
			}
		})
	}
}

// TestRequireScope_NilReceiver verifies the gate is permissive for a Data value
// that was never configured, matching ImpactCache: the framework's early-lifecycle
// nil ProviderData is the caller's guard, not this one's error.
func TestRequireScope_NilReceiver(t *testing.T) {
	var pd *Data
	if diags := pd.RequireScope("jamfplatform_pro_test", ScopeTenant); diags.HasError() {
		t.Errorf("expected no diagnostics from a nil receiver, got %v", diags)
	}
	if got := pd.Scope(); got != ScopeOrganization {
		t.Errorf("Scope on nil receiver: got %v, want ScopeOrganization", got)
	}
}

// TestRequireScope_DiagnosticIsActionable pins the parts of the message a user
// needs: which construct, which scope it wants, which one is configured, and the
// 403 that follows from pointing the right header at the wrong credential.
func TestRequireScope_DiagnosticIsActionable(t *testing.T) {
	pd := &Data{scope: ScopeOrganization}
	diags := pd.RequireScope("jamfplatform_blueprints_blueprint", ScopeEnvironment)
	if !diags.HasError() {
		t.Fatal("expected an error for an organization-scoped credential")
	}
	err := diags.Errors()[0]
	if !strings.Contains(err.Summary(), "jamfplatform_blueprints_blueprint") {
		t.Errorf("summary does not name the construct: %s", err.Summary())
	}
	for _, want := range []string{
		"an environment-scoped integration",
		"an organization-scoped integration",
		"`environment_id`",
		"OWNERSHIP_FORBIDDEN",
	} {
		if !strings.Contains(err.Detail(), want) {
			t.Errorf("detail does not mention %q: %s", want, err.Detail())
		}
	}
	if strings.Contains(err.Detail(), "`tenant_id` in the provider block") {
		t.Errorf("environment-only requirement should not suggest tenant_id: %s", err.Detail())
	}
}

// TestRequireScope_OrganizationOnlyRemedyNamesTheEnvironment pins the remedy for
// the organization-only allowed set, which the jamfplatform_account_* family is
// the first to use. The scope is resolved from `JAMFPLATFORM_ENVIRONMENT_ID` /
// `JAMFPLATFORM_TENANT_ID` whenever the provider block sets neither attribute,
// with no warning on that path — so a remedy naming only the provider block
// tells the most likely reader, a CI runner exporting the preferred environment
// variable, to remove a line that is not in their configuration.
// TestRequireScope_RemedyNamesTheSwapNotJustTheAddition pins the one thing every
// reader of this diagnostic needs and the earlier wording withheld.
//
// Reaching the refusal means carrying a scope the construct rejects, so the
// input that selected it is already set. `environment_id` and `tenant_id` are
// mutually exclusive, so a remedy that said only "set `environment_id`" traded
// this error for `Conflicting API Integration Scope` on the next plan — a
// deterministic second cycle for 100% of readers. The remedy has to name what to
// remove as well as what to add.
func TestRequireScope_RemedyNamesTheSwapNotJustTheAddition(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured ScopeKind
		allowed    []ScopeKind
		wantRemove string
		wantAdd    string
	}{
		{"tenant aimed at an environment-only family", ScopeTenant, []ScopeKind{ScopeEnvironment},
			"`tenant_id`", "`environment_id`"},
		{"environment aimed at a tenant-only family", ScopeEnvironment, []ScopeKind{ScopeTenant},
			"`environment_id`", "`tenant_id`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scopeRemedy(tc.configured, tc.allowed)
			if !strings.Contains(got, "Replace "+tc.wantRemove+" with "+tc.wantAdd) {
				t.Errorf("remedy does not name the swap %s -> %s, got:\n%s", tc.wantRemove, tc.wantAdd, got)
			}
			if !strings.Contains(got, "mutually exclusive") {
				t.Errorf("remedy does not say the two cannot both be set, got:\n%s", got)
			}
		})
	}
}

// TestRequireScope_OrganizationRemedyDoesNotNameASwap covers the branch where
// there is nothing to remove: an organization-scoped provider has neither
// attribute set, so "replace X with Y" would name an attribute the operator
// never wrote.
func TestRequireScope_OrganizationRemedyDoesNotNameASwap(t *testing.T) {
	got := scopeRemedy(ScopeOrganization, []ScopeKind{ScopeEnvironment})
	if strings.Contains(got, "Replace ") {
		t.Errorf("organization scope has no attribute to replace, got:\n%s", got)
	}
	if !strings.Contains(got, "Set `environment_id`") {
		t.Errorf("remedy should still say which attribute to set, got:\n%s", got)
	}
}

func TestRequireScope_OrganizationOnlyRemedyNamesTheEnvironment(t *testing.T) {
	for _, configured := range []ScopeKind{ScopeEnvironment, ScopeTenant} {
		t.Run(configured.String(), func(t *testing.T) {
			pd := &Data{scope: configured}
			diags := pd.RequireScope("jamfplatform_account_sso_domain", ScopeOrganization)
			if !diags.HasError() {
				t.Fatalf("expected an error for a %s-scoped credential", configured)
			}
			detail := diags.Errors()[0].Detail()
			for _, want := range []string{
				"`environment_id`",
				"`tenant_id`",
				"`JAMFPLATFORM_ENVIRONMENT_ID`",
				"`JAMFPLATFORM_TENANT_ID`",
				"scoped from the access token alone",
			} {
				if !strings.Contains(detail, want) {
					t.Errorf("detail does not mention %q: %s", want, detail)
				}
			}
		})
	}
}

// TestScopeDescriptionDoesNotAssertAnAttributeIsSet guards the other half of the
// same argument: the description cannot claim `environment_id` or `tenant_id`
// "is set", because the value may have come from the environment instead, and
// nothing in the diagnostic can tell which source selected it.
func TestScopeDescriptionDoesNotAssertAnAttributeIsSet(t *testing.T) {
	tests := map[ScopeKind][]string{
		ScopeEnvironment:  {"an environment-scoped integration", "`environment_id`", "`JAMFPLATFORM_ENVIRONMENT_ID`"},
		ScopeTenant:       {"a tenant-scoped integration", "`tenant_id`", "`JAMFPLATFORM_TENANT_ID`"},
		ScopeOrganization: {"an organization-scoped integration", "`environment_id`", "`tenant_id`", "`JAMFPLATFORM_*`"},
	}
	for kind, wants := range tests {
		got := scopeDescription(kind)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("scopeDescription(%v) = %q, want it to mention %q", kind, got, want)
			}
		}
		if strings.Contains(got, "is set") {
			t.Errorf("scopeDescription(%v) = %q, must not assert an attribute is set", kind, got)
		}
	}
}

// TestScopeRequirementPhrasing guards the generated phrasing, which is assembled
// rather than written out and so is easy to break into "a tenant-scoped or
// credential".
func TestScopeRequirementPhrasing(t *testing.T) {
	tests := []struct {
		allowed []ScopeKind
		want    string
	}{
		{[]ScopeKind{ScopeEnvironment}, "an environment-scoped integration"},
		{[]ScopeKind{ScopeTenant}, "a tenant-scoped integration"},
		{[]ScopeKind{ScopeEnvironment, ScopeTenant}, "an environment-scoped or tenant-scoped integration"},
		{[]ScopeKind{ScopeTenant, ScopeEnvironment}, "a tenant-scoped or environment-scoped integration"},
	}
	for _, tc := range tests {
		if got := scopeRequirement(tc.allowed); got != tc.want {
			t.Errorf("scopeRequirement(%v): got %q, want %q", tc.allowed, got, tc.want)
		}
	}
}

// TestScopeKindString pins the names that reach the tflog field and the
// diagnostics, including the zero value.
func TestScopeKindString(t *testing.T) {
	tests := map[ScopeKind]string{
		ScopeOrganization: "organization",
		ScopeTenant:       "tenant",
		ScopeEnvironment:  "environment",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("ScopeKind(%d).String(): got %q, want %q", kind, got, want)
		}
	}
}
