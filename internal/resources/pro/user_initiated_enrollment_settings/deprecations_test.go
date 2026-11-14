// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestResource_SignQuickAddAndDeveloperCertificateAreDeprecated(t *testing.T) {
	t.Parallel()
	resp := &resource.SchemaResponse{}
	NewUserInitiatedEnrollmentSettingsResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", resp.Diagnostics)
	}
	cases := map[string]string{
		"sign_quickadd_package": signQuickAddPackageDeprecation,
		"developer_certificate": developerCertificateDeprecation,
	}
	for name, want := range cases {
		attr := resp.Schema.Attributes[name]
		if attr == nil {
			t.Fatalf("%s missing from schema", name)
		}
		if attr.GetDeprecationMessage() != want {
			t.Errorf("%s DeprecationMessage = %q", name, attr.GetDeprecationMessage())
		}
		if !strings.HasPrefix(attr.GetMarkdownDescription(), "~> **Deprecated.**") {
			t.Errorf("%s description must open with the deprecation callout", name)
		}
	}
}
