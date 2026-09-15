// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_title

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

const wantPluralTypeName = "jamfplatform_pro_app_installer_titles"

func TestAppInstallerTitlesDataSource_Metadata(t *testing.T) {
	d := NewAppInstallerTitlesDataSource()
	var resp datasource.MetadataResponse
	d.(*AppInstallerTitlesDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantPluralTypeName {
		t.Errorf("expected type name %q, got %q", wantPluralTypeName, resp.TypeName)
	}
}

func TestAppInstallerTitlesDataSource_Schema(t *testing.T) {
	d := NewAppInstallerTitlesDataSource()
	var resp datasource.SchemaResponse
	d.(*AppInstallerTitlesDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"id", "name_substring", "titles"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func TestFilterAndMapTitles_NoFilter(t *testing.T) {
	titles := []pro.AppTitle{
		{ID: "1", TitleName: "Adobe Lightroom Classic"},
		{ID: "2", TitleName: "Jamf Composer"},
	}
	got := FilterAndMapTitles(titles, types.StringNull())
	if len(got) != 2 {
		t.Fatalf("expected 2 titles, got %d", len(got))
	}
}

func TestFilterAndMapTitles_Substring(t *testing.T) {
	titles := []pro.AppTitle{
		{ID: "1", TitleName: "Adobe Lightroom Classic"},
		{ID: "2", TitleName: "Jamf Composer"},
	}
	got := FilterAndMapTitles(titles, types.StringValue("composer"))
	if len(got) != 1 || got[0].ID.ValueString() != "2" {
		t.Fatalf("expected only the composer title, got %+v", got)
	}
}

func TestFilterAndMapTitles_EmptyResultNotNil(t *testing.T) {
	got := FilterAndMapTitles(nil, types.StringNull())
	if got == nil {
		t.Fatalf("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d", len(got))
	}
}
