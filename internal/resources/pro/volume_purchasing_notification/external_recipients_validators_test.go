// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/setvalidation"
)

func runRecipientValidators(t *testing.T, elems ...[2]string) []string {
	t.Helper()
	var resp resource.SchemaResponse
	NewVolumePurchasingNotificationResource().(*VolumePurchasingNotificationResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	values := make([]map[string]attr.Value, 0, len(elems))
	for _, e := range elems {
		values = append(values, map[string]attr.Value{"email": types.StringValue(e[0]), "name": types.StringValue(e[1])})
	}
	diags := setvalidation.Run(t, resp.Schema, []string{"external_recipients"}, values...)
	for _, d := range diags.Errors() {
		if !strings.Contains(d.Detail(), "without regard to case") {
			t.Errorf("detail does not say how Jamf Pro compares: %q", d.Detail())
		}
	}
	return setvalidation.Summaries(diags)
}

func TestExternalRecipients_Wired(t *testing.T) {
	want := []string{"External recipient listed more than once"}
	cases := map[string][][2]string{
		"same email, different name": {{"a@example.com", "A"}, {"a@example.com", "B"}},
		"email differs only in case": {{"a@example.com", "A"}, {"A@Example.com", "A"}},
	}
	for name, elems := range cases {
		t.Run(name, func(t *testing.T) {
			if got := runRecipientValidators(t, elems...); !slices.Equal(got, want) {
				t.Errorf("errors = %q, want %q", got, want)
			}
		})
	}
	t.Run("same name, different email", func(t *testing.T) {
		if got := runRecipientValidators(t, [2]string{"a@example.com", "A"}, [2]string{"b@example.com", "A"}); len(got) != 0 {
			t.Errorf("valid set rejected: %q", got)
		}
	})
}
