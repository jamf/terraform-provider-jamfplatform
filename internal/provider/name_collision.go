// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const (
	envNameCollisionMode              = "JAMFPLATFORM_NAME_COLLISION_CHECK_MODE"
	envNameCollisionIncludeUnenforced = "JAMFPLATFORM_NAME_COLLISION_CHECK_INCLUDE_UNENFORCED"
)

// defaultNameCollisionMode is the mode applied when neither the attribute nor the
// environment sets one. Jamf refuses the duplicate at apply time for every type
// the check covers by default, so reporting it at plan time only moves the failure.
const defaultNameCollisionMode = collision.ModeError

// nameCollisionCheckModel is the decoded name_collision_check block.
type nameCollisionCheckModel struct {
	Mode              types.String `tfsdk:"mode"`
	IncludeUnenforced types.Bool   `tfsdk:"include_unenforced"`
}

// resolveNameCollisionCheck resolves the name_collision_check setting. Each
// attribute inside the block takes precedence over its environment variable,
// which is then not consulted. An unusable environment value keeps the default
// and returns a warning for Configure to surface: the check is advisory about
// operator input, so a typo must neither fail the plan nor pass silently.
func resolveNameCollisionCheck(block types.Object) (cfg collision.Config, warnings []string) {
	cfg = collision.Config{Mode: defaultNameCollisionMode}

	var set nameCollisionCheckModel
	if !block.IsNull() && !block.IsUnknown() {
		_ = block.As(context.Background(), &set, basetypes.ObjectAsOptions{})
	}

	switch {
	case !set.Mode.IsNull() && !set.Mode.IsUnknown():
		cfg.Mode = collision.Mode(set.Mode.ValueString())
	default:
		if v, ok := os.LookupEnv(envNameCollisionMode); ok && strings.TrimSpace(v) != "" {
			mode := collision.Mode(strings.ToLower(strings.TrimSpace(v)))
			if slices.Contains(collision.Modes(), string(mode)) {
				cfg.Mode = mode
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"%s is set to %q, which is not a valid mode, so the default %q applies. Accepted values are %s. "+
						"The name_collision_check.mode attribute, when set, takes precedence over this variable.",
					envNameCollisionMode, v, defaultNameCollisionMode, strings.Join(collision.Modes(), ", ")))
			}
		}
	}

	switch {
	case !set.IncludeUnenforced.IsNull() && !set.IncludeUnenforced.IsUnknown():
		cfg.IncludeUnenforced = set.IncludeUnenforced.ValueBool()
	default:
		if v, ok := os.LookupEnv(envNameCollisionIncludeUnenforced); ok && strings.TrimSpace(v) != "" {
			parsed, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"%s is set to %q, which is not a valid boolean, so unenforced types stay unchecked. "+
						"Accepted values are those of Go's strconv.ParseBool. "+
						"The name_collision_check.include_unenforced attribute, when set, takes precedence over this variable.",
					envNameCollisionIncludeUnenforced, v))
			} else {
				cfg.IncludeUnenforced = parsed
			}
		}
	}
	return cfg, warnings
}
