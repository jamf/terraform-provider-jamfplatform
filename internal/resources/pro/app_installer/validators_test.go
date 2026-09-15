// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// fakeTitleResolver is a testable titleCatalog. notFound forces the empty catalog
// Jamf Pro returns on a tenant with no titles; otherErr forces the read failure a
// transport error or a privilege gap produces. The catalog it serves holds one
// title, so an exact-name query resolves and an off-casing one does not.
type fakeTitleResolver struct {
	notFound bool
	otherErr error
	called   bool
}

func (f *fakeTitleResolver) Titles(ctx context.Context) ([]pro.AppTitle, error) {
	f.called = true
	if f.otherErr != nil {
		return nil, f.otherErr
	}
	if f.notFound {
		return []pro.AppTitle{}, nil
	}
	return []pro.AppTitle{{ID: "027", TitleName: "Adobe Lightroom Classic"}}, nil
}

func TestValidateAppTitleName_Found(t *testing.T) {
	r := &fakeTitleResolver{}
	diags := validateAppTitleName(context.Background(), r, types.StringValue("Adobe Lightroom Classic"), path.Root("app_title_name"))
	if diags.HasError() {
		t.Errorf("expected no error for a found title, got %v", diags)
	}
	if !r.called {
		t.Errorf("resolver should have been called")
	}
}

func TestValidateAppTitleName_NotFoundIsError(t *testing.T) {
	r := &fakeTitleResolver{notFound: true}
	diags := validateAppTitleName(context.Background(), r, types.StringValue("No Such App"), path.Root("app_title_name"))
	if !diags.HasError() {
		t.Errorf("expected an error for an unknown title")
	}
}

func TestValidateAppTitleName_TransportErrorIsWarning(t *testing.T) {
	r := &fakeTitleResolver{otherErr: errors.New("connection refused")}
	diags := validateAppTitleName(context.Background(), r, types.StringValue("Adobe Lightroom Classic"), path.Root("app_title_name"))
	if diags.HasError() {
		t.Errorf("transport error must downgrade to a warning, got error: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Errorf("expected exactly 1 warning, got %d", diags.WarningsCount())
	}
}

func TestValidateAppTitleName_SkipsUnknownAndNull(t *testing.T) {
	r := &fakeTitleResolver{}
	for _, v := range []types.String{types.StringUnknown(), types.StringNull(), types.StringValue("")} {
		diags := validateAppTitleName(context.Background(), r, v, path.Root("app_title_name"))
		if diags.HasError() || diags.WarningsCount() > 0 {
			t.Errorf("value %v must be skipped, got %v", v, diags)
		}
	}
	if r.called {
		t.Errorf("resolver must not be called for unknown/null/empty values")
	}
}

func TestValidateAppTitleName_NilResolver(t *testing.T) {
	diags := validateAppTitleName(context.Background(), nil, types.StringValue("Adobe Lightroom Classic"), path.Root("app_title_name"))
	if diags.HasError() || diags.WarningsCount() > 0 {
		t.Errorf("nil resolver must be a no-op, got %v", diags)
	}
}

func TestResolveAppTitleID_Found(t *testing.T) {
	r := &fakeTitleResolver{}
	id, diags := resolveAppTitleID(context.Background(), r, "Adobe Lightroom Classic")
	if diags.HasError() {
		t.Fatalf("expected no error, got %v", diags)
	}
	if id != "027" {
		t.Errorf("expected resolved id 027, got %q", id)
	}
}

func TestResolveAppTitleID_NotFoundIsError(t *testing.T) {
	r := &fakeTitleResolver{notFound: true}
	_, diags := resolveAppTitleID(context.Background(), r, "No Such App")
	if !diags.HasError() {
		t.Errorf("expected an error for an unknown title")
	}
}

func TestTitleNameForID(t *testing.T) {
	n := &fakeTitleResolver{}
	got, ok := titleNameForID(context.Background(), n, "027")
	if !ok || got != "Adobe Lightroom Classic" {
		t.Errorf("expected reverse-resolve to succeed, got %q ok=%v", got, ok)
	}

	// Catalog read failure → ok=false (caller preserves existing state).
	nErr := &fakeTitleResolver{otherErr: errors.New("boom")}
	if _, ok := titleNameForID(context.Background(), nErr, "027"); ok {
		t.Errorf("a failed catalog read must return ok=false")
	}

	// An id the catalog does not hold → ok=false, the same as a failed read: a
	// withdrawn title cannot be named.
	if _, ok := titleNameForID(context.Background(), n, "no-such-id"); ok {
		t.Errorf("an id absent from the catalog must return ok=false")
	}

	// Empty id / nil catalog → ok=false, no read.
	if _, ok := titleNameForID(context.Background(), n, ""); ok {
		t.Errorf("empty id must return ok=false")
	}
	if _, ok := titleNameForID(context.Background(), nil, "027"); ok {
		t.Errorf("nil catalog must return ok=false")
	}
}
