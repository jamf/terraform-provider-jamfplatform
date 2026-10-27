// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

func collisionBlock(mode types.String, include types.Bool) types.Object {
	return types.ObjectValueMust(
		map[string]attr.Type{"mode": types.StringType, "include_unenforced": types.BoolType},
		map[string]attr.Value{"mode": mode, "include_unenforced": include},
	)
}

func nullCollisionBlock() types.Object {
	return types.ObjectNull(map[string]attr.Type{"mode": types.StringType, "include_unenforced": types.BoolType})
}

func TestResolveNameCollisionCheck(t *testing.T) {
	tests := []struct {
		name     string
		block    types.Object
		mode     *string
		include  *string
		want     collision.Config
		wantWarn int
	}{
		{name: "default is error and strict off", block: nullCollisionBlock(), want: collision.Config{Mode: collision.ModeError}},
		{name: "attribute mode", block: collisionBlock(types.StringValue("warn"), types.BoolNull()), want: collision.Config{Mode: collision.ModeWarn}},
		{name: "attribute off", block: collisionBlock(types.StringValue("off"), types.BoolNull()), want: collision.Config{Mode: collision.ModeOff}},
		{name: "attribute include", block: collisionBlock(types.StringNull(), types.BoolValue(true)), want: collision.Config{Mode: collision.ModeError, IncludeUnenforced: true}},
		{name: "environment mode", block: nullCollisionBlock(), mode: new("warn"), want: collision.Config{Mode: collision.ModeWarn}},
		{name: "environment mode case and space tolerated", block: nullCollisionBlock(), mode: new(" OFF "), want: collision.Config{Mode: collision.ModeOff}},
		{name: "environment include", block: nullCollisionBlock(), include: new("true"), want: collision.Config{Mode: collision.ModeError, IncludeUnenforced: true}},
		{name: "attribute beats environment", block: collisionBlock(types.StringValue("off"), types.BoolValue(false)), mode: new("error"), include: new("true"), want: collision.Config{Mode: collision.ModeOff}},
		{name: "unknown attribute falls through to the environment", block: collisionBlock(types.StringUnknown(), types.BoolUnknown()), mode: new("warn"), want: collision.Config{Mode: collision.ModeWarn}},
		{name: "bad environment mode keeps the default and warns", block: nullCollisionBlock(), mode: new("loud"), want: collision.Config{Mode: collision.ModeError}, wantWarn: 1},
		{name: "bad environment bool keeps strict off and warns", block: nullCollisionBlock(), include: new("sure"), want: collision.Config{Mode: collision.ModeError}, wantWarn: 1},
		{name: "empty environment values are unset", block: nullCollisionBlock(), mode: new(""), include: new("  "), want: collision.Config{Mode: collision.ModeError}},
		{name: "attribute set suppresses the environment warning", block: collisionBlock(types.StringValue("warn"), types.BoolValue(true)), mode: new("loud"), include: new("sure"), want: collision.Config{Mode: collision.ModeWarn, IncludeUnenforced: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envNameCollisionMode, "")
			t.Setenv(envNameCollisionIncludeUnenforced, "")
			if tc.mode != nil {
				t.Setenv(envNameCollisionMode, *tc.mode)
			}
			if tc.include != nil {
				t.Setenv(envNameCollisionIncludeUnenforced, *tc.include)
			}
			got, warnings := resolveNameCollisionCheck(tc.block)
			if got != tc.want {
				t.Fatalf("config = %+v, want %+v", got, tc.want)
			}
			if len(warnings) != tc.wantWarn {
				t.Fatalf("warnings = %v, want %d", warnings, tc.wantWarn)
			}
			for _, w := range warnings {
				if !strings.Contains(w, "JAMFPLATFORM_NAME_COLLISION_CHECK") {
					t.Errorf("warning %q does not name the environment variable", w)
				}
			}
		})
	}
}
