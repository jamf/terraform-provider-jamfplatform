// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

func TestBuildCreateRequest(t *testing.T) {
	pinned := buildCreateRequest(PlatformEnvironmentResourceModel{
		Name: types.StringValue("n"), Type: types.StringValue("PRODUCTION"), Region: types.StringValue("APAC"),
	})
	if pinned.Name != "n" || pinned.Label != "PRODUCTION" || pinned.MetaRegion == nil || *pinned.MetaRegion != "APAC" {
		t.Errorf("request = %+v", pinned)
	}

	global := buildCreateRequest(PlatformEnvironmentResourceModel{
		Name: types.StringValue("n"), Type: types.StringValue("BETA"), Region: types.StringNull(),
	})
	if global.MetaRegion != nil {
		t.Errorf("a null region must be left out of the request, got %q", *global.MetaRegion)
	}
}

func TestBuildUpdateRequestCarriesNameAndTypeOnly(t *testing.T) {
	got := buildUpdateRequest(PlatformEnvironmentResourceModel{
		Name: types.StringValue("n"), Type: types.StringValue("NFR"), Region: types.StringValue("EU"),
	})
	if got.Name != "n" || got.Label != "NFR" {
		t.Errorf("request = %+v", got)
	}
}

func TestAssignResourceModel(t *testing.T) {
	label, region := "TRIAL", "FEDRAMP"
	var state PlatformEnvironmentResourceModel
	assignResourceModel(&state, &account.Environment{ID: "e", Name: "n", Label: &label, MetaRegion: &region})
	if state.ID.ValueString() != "e" || state.Name.ValueString() != "n" || state.Type.ValueString() != "TRIAL" || state.Region.ValueString() != "FEDRAMP" {
		t.Errorf("state = %+v", state)
	}

	assignResourceModel(&state, &account.Environment{ID: "e", Name: "n"})
	if !state.Type.IsNull() || !state.Region.IsNull() {
		t.Errorf("absent label and region must be null, got %+v", state)
	}
}

func TestBuildResultRow(t *testing.T) {
	label := "SANDBOX"
	row := buildResultRow(account.Environment{ID: "e", Name: "n", Label: &label})
	if row.ID.ValueString() != "e" || row.Type.ValueString() != "SANDBOX" || !row.Region.IsNull() {
		t.Errorf("row = %+v", row)
	}
}

func TestDescribeTenant(t *testing.T) {
	name, product := "Corp", "pro"
	if got := describeTenant(account.Tenant{ID: "t-1", Name: &name, Product: &product}); got != "Corp (t-1, pro)" {
		t.Errorf("got %q", got)
	}
	if got := describeTenant(account.Tenant{ID: "t-2"}); got != "(unnamed) (t-2)" {
		t.Errorf("got %q", got)
	}
}

func TestAppendHeldTenantsDiagnostics_CapsTheListing(t *testing.T) {
	env := "env-1"
	tenants := make([]account.Tenant, 0, 13)
	for i := range 13 {
		tenants = append(tenants, account.Tenant{ID: fmt.Sprintf("t-%d", i), EnvironmentID: &env})
	}
	var diags diag.Diagnostics
	if !appendHeldTenantsDiagnostics(&diags, "n", tenants) {
		t.Fatal("expected a refusal")
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "and 3 more") || !strings.Contains(detail, "13 tenant(s)") {
		t.Errorf("detail %q does not summarise the overflow", detail)
	}
	if appendHeldTenantsDiagnostics(&diags, "n", nil) {
		t.Error("an empty tenant list must not refuse")
	}
}
