// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

func TestConfigureSecurityCloud_ScopeGate(t *testing.T) {
	tests := []struct {
		name    string
		opts    []jamfplatform.Option
		wantErr bool
	}{
		{
			name: "environment scope is allowed",
			opts: []jamfplatform.Option{jamfplatform.WithEnvironmentID("e-1")},
		},
		{
			name: "tenant scope is allowed",
			opts: []jamfplatform.Option{jamfplatform.WithTenantID("t-1")},
		},
		{
			name:    "organization scope is refused",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pd := New(jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret", tc.opts...))

			client, diags := ConfigureSecurityCloud(context.Background(), pd, "jamfplatform_security_cloud_dns_zone")

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

// TestConfigureSecurityCloud_NilProviderDataIsNotAnError covers the early
// framework lifecycle, where Configure is called with no provider data. The
// construct stays unconfigured until a later call supplies it; treating this as
// an error would fail every plan.
func TestConfigureSecurityCloud_NilProviderDataIsNotAnError(t *testing.T) {
	client, diags := ConfigureSecurityCloud(context.Background(), nil, "jamfplatform_security_cloud_dns_zone")
	if diags.HasError() {
		t.Errorf("nil provider data must not error: %v", diags)
	}
	if client != nil {
		t.Error("nil provider data must not yield a client")
	}
}

func TestConfigureSecurityCloud_WrongProviderDataType(t *testing.T) {
	_, diags := ConfigureSecurityCloud(context.Background(), "not a *Data", "jamfplatform_security_cloud_dns_zone")
	if !diags.HasError() {
		t.Fatal("a provider data value of the wrong type must error")
	}
	if !strings.Contains(diags.Errors()[0].Summary(), "Unexpected Configure Type") {
		t.Errorf("diagnostic summary = %q, want the unexpected-type summary", diags.Errors()[0].Summary())
	}
}

// TestConfigureSecurityCloud_DiagnosticNamesTheConstruct guards the argument that
// makes the gate useful: the refusal has to say which construct was refused and
// which scope to set, or a user with several products configured cannot tell what
// to change.
func TestConfigureSecurityCloud_DiagnosticNamesTheConstruct(t *testing.T) {
	pd := New(jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret"))

	_, diags := ConfigureSecurityCloud(context.Background(), pd, "jamfplatform_security_cloud_dns_zone")
	if !diags.HasError() {
		t.Fatal("organization scope must be refused")
	}
	err := diags.Errors()[0]
	if !strings.Contains(err.Summary(), "jamfplatform_security_cloud_dns_zone") {
		t.Errorf("summary %q does not name the construct", err.Summary())
	}
	for _, want := range []string{"environment_id", "tenant_id"} {
		if !strings.Contains(err.Detail(), want) {
			t.Errorf("detail does not mention %s:\n%s", want, err.Detail())
		}
	}
}
