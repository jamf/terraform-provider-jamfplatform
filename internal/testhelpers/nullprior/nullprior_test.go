// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package nullprior

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestUnmatchedExemptions keeps an exemption from outliving its block: a name
// that matches no top-level Optional single-nested block is reported.
func TestUnmatchedExemptions(t *testing.T) {
	t.Parallel()
	sch := schema.Schema{Attributes: map[string]schema.Attribute{
		"optional_block": schema.SingleNestedAttribute{Optional: true},
		"required_block": schema.SingleNestedAttribute{Required: true},
		"scalar":         schema.StringAttribute{Optional: true},
	}}
	exempt := map[string]bool{"optional_block": true, "required_block": true, "scalar": true, "renamed_block": true}

	got := unmatchedExemptions(sch, exempt)
	want := []string{"renamed_block", "required_block", "scalar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatchedExemptions = %v, want %v", got, want)
	}
}
