// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package securitycloudtenant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

const (
	tenantA = "fc75ff7c-cda5-484e-b652-59ca4a8b2010"
	tenantB = "2bbdc494-845f-4ef1-a186-4189e374dff5"
)

// connectors returns a lister reporting one connector per customer ID, and counts its calls.
func connectors(calls *int, customerIDs ...string) ConnectorLister {
	return func(context.Context) (*securitycloud.ConnectorPage, error) {
		*calls++
		page := &securitycloud.ConnectorPage{TotalCount: int64(len(customerIDs))}
		for _, id := range customerIDs {
			page.Results = append(page.Results, securitycloud.ConnectorConfig{CustomerID: id})
		}
		return page, nil
	}
}

// TestResolve_TenantScopeNeedsNoLookup pins that a tenant-scoped provider uses its own tenant ID
// and never reads UEM Connect, which a tenant-scoped integration may not be permitted to read.
func TestResolve_TenantScopeNeedsNoLookup(t *testing.T) {
	var calls int
	got, err := Resolve(context.Background(), tenantA, connectors(&calls, tenantB))
	if err != nil || got != tenantA {
		t.Fatalf("Resolve = %q, %v; want %q", got, err, tenantA)
	}
	if calls != 0 {
		t.Errorf("the connector list was read %d times under tenant scope, want 0", calls)
	}
}

// TestResolve_EnvironmentScopeReadsTheConnector covers the environment-scope source, including a
// connector repeated in the page, which is still one tenant.
func TestResolve_EnvironmentScopeReadsTheConnector(t *testing.T) {
	var calls int
	for _, ids := range [][]string{{tenantA}, {tenantA, tenantA}, {"", tenantA}} {
		got, err := Resolve(context.Background(), "", connectors(&calls, ids...))
		if err != nil || got != tenantA {
			t.Errorf("Resolve with connectors %v = %q, %v; want %q", ids, got, err, tenantA)
		}
	}
}

// TestResolve_NoConnectorIsErrNoSource pins the case wire-probed on 2026-09-30 with the connector
// deleted: an empty list, and no other source to fall back to.
func TestResolve_NoConnectorIsErrNoSource(t *testing.T) {
	var calls int
	for name, list := range map[string]ConnectorLister{
		"empty list":     connectors(&calls),
		"blank customer": connectors(&calls, ""),
		"nil page":       func(context.Context) (*securitycloud.ConnectorPage, error) { return nil, nil },
	} {
		if _, err := Resolve(context.Background(), "", list); !errors.Is(err, ErrNoSource) {
			t.Errorf("%s: err = %v, want ErrNoSource", name, err)
		}
	}
}

// TestResolve_DisagreeingConnectorsAreRefused pins that the resolver never picks one of two
// tenants: granting a gateway to a tenant nobody named is the failure this guards against.
func TestResolve_DisagreeingConnectorsAreRefused(t *testing.T) {
	var calls int
	_, err := Resolve(context.Background(), "", connectors(&calls, tenantA, tenantB))
	if err == nil || errors.Is(err, ErrNoSource) {
		t.Fatalf("err = %v, want a disagreement error", err)
	}
	if !strings.Contains(err.Error(), tenantA) || !strings.Contains(err.Error(), tenantB) {
		t.Errorf("error %q does not name both tenants", err)
	}
}

// TestResolve_ListFailureIsReported pins that a failed read, a missing UEM Connect permission for
// instance, surfaces instead of reading as "no connector".
func TestResolve_ListFailureIsReported(t *testing.T) {
	boom := errors.New("403 BAD_PERMISSIONS")
	_, err := Resolve(context.Background(), "", func(context.Context) (*securitycloud.ConnectorPage, error) { return nil, boom })
	if !errors.Is(err, boom) || errors.Is(err, ErrNoSource) {
		t.Fatalf("err = %v, want it to wrap the list failure", err)
	}
}

// TestAppendUnresolved pins that the error lands on tenant_ids and tells the reader where to find
// the ID, for both the no-source case and a failed lookup.
func TestAppendUnresolved(t *testing.T) {
	for name, err := range map[string]error{"no source": ErrNoSource, "lookup failed": errors.New("403 BAD_PERMISSIONS")} {
		var diags diag.Diagnostics
		AppendUnresolved(&diags, path.Root("tenant_ids"), err)
		if diags.ErrorsCount() != 1 {
			t.Fatalf("%s: %d errors, want 1", name, diags.ErrorsCount())
		}
		d := diags.Errors()[0]
		withPath, ok := d.(diag.DiagnosticWithPath)
		if !ok || !withPath.Path().Equal(path.Root("tenant_ids")) {
			t.Errorf("%s: error is not attributed to tenant_ids", name)
		}
		if !strings.Contains(d.Detail(), "Copy ID") {
			t.Errorf("%s: detail does not say where to find the ID: %q", name, d.Detail())
		}
		if name == "lookup failed" && !strings.Contains(d.Detail(), "403 BAD_PERMISSIONS") {
			t.Errorf("%s: detail drops the lookup error: %q", name, d.Detail())
		}
	}
}
