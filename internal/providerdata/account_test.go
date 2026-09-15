// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// TestConfigureAccount_ScopeGate pins the gate the whole jamfplatform_account_*
// family depends on, and it runs in the opposite direction to every other
// family's: organization scope is the only one that reaches /sso/v1, and
// environment and tenant scope are both refused. Environment scope is refused
// deliberately rather than incidentally — it is untested, not ruled out — so
// widening the allowed list is a fresh wire probe, and this test is what makes
// the one-token widening fail loudly instead of silently sending
// X-Environment-Id to a namespace that has never answered it.
func TestConfigureAccount_ScopeGate(t *testing.T) {
	tests := []struct {
		name    string
		opts    []jamfplatform.Option
		wantErr bool
	}{
		{
			name: "organization scope is allowed",
		},
		{
			name:    "environment scope is refused",
			opts:    []jamfplatform.Option{jamfplatform.WithEnvironmentID("e-1")},
			wantErr: true,
		},
		{
			name:    "tenant scope is refused",
			opts:    []jamfplatform.Option{jamfplatform.WithTenantID("t-1")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pd := New(jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret", tc.opts...))

			client, diags := ConfigureAccount(context.Background(), pd, "jamfplatform_account_sso_domain")

			if got := diags.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (diagnostics: %v)", got, tc.wantErr, diags)
			}
			if tc.wantErr {
				if client != nil {
					t.Error("a refused scope must not yield a client")
				}
				return
			}
			if client == nil {
				t.Error("an allowed scope must yield a client")
			}
		})
	}
}

// TestConfigureAccount_NilProviderDataIsNotAnError covers the early framework
// lifecycle, where Configure is called with no provider data. The construct
// stays unconfigured until a later call supplies it; treating this as an error
// would fail every plan. Every Account call site appends these diagnostics and
// then checks HasError, so an error here surfaces on all of them.
func TestConfigureAccount_NilProviderDataIsNotAnError(t *testing.T) {
	client, diags := ConfigureAccount(context.Background(), nil, "jamfplatform_account_sso_domain")
	if diags.HasError() {
		t.Errorf("nil provider data must not error: %v", diags)
	}
	if client != nil {
		t.Error("nil provider data must not yield a client")
	}
}

func TestConfigureAccount_WrongProviderDataType(t *testing.T) {
	_, diags := ConfigureAccount(context.Background(), "not a *Data", "jamfplatform_account_sso_domain")
	if !diags.HasError() {
		t.Fatal("a provider data value of the wrong type must error")
	}
	if !strings.Contains(diags.Errors()[0].Summary(), "Unexpected Configure Type") {
		t.Errorf("diagnostic summary = %q, want the unexpected-type summary", diags.Errors()[0].Summary())
	}
}

// TestConfigureAccount_DiagnosticNamesTheConstruct guards the argument that
// makes the gate useful: the refusal has to say which construct was refused and
// what to change, or a user with several products configured cannot tell what to
// do. For this family the remedy is an unsetting rather than a setting, and it
// has to name the environment variables too — the scope is resolved from
// JAMFPLATFORM_ENVIRONMENT_ID / JAMFPLATFORM_TENANT_ID whenever the provider
// block sets neither attribute, which is the likeliest way to reach this
// diagnostic.
func TestConfigureAccount_DiagnosticNamesTheConstruct(t *testing.T) {
	pd := New(jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret", jamfplatform.WithEnvironmentID("e-1")))

	_, diags := ConfigureAccount(context.Background(), pd, "jamfplatform_account_sso_domain")
	if !diags.HasError() {
		t.Fatal("environment scope must be refused")
	}
	err := diags.Errors()[0]
	if !strings.Contains(err.Summary(), "jamfplatform_account_sso_domain") {
		t.Errorf("summary %q does not name the construct", err.Summary())
	}
	for _, want := range []string{
		"an organization-scoped integration",
		"`environment_id`",
		"`tenant_id`",
		"`JAMFPLATFORM_ENVIRONMENT_ID`",
		"`JAMFPLATFORM_TENANT_ID`",
	} {
		if !strings.Contains(err.Detail(), want) {
			t.Errorf("detail does not mention %s:\n%s", want, err.Detail())
		}
	}
}
