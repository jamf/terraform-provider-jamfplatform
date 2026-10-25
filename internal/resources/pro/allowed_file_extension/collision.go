// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_allowed_file_extension"

// allowedFileExtensionNamespace is the uniqueness rule for extensions. Jamf Pro
// refuses a duplicate with 409 "Duplicate extension" and compares the value
// without regard to case, surrounding spaces or accents (wire-probed 2026-10-25),
// although it stores the value exactly as entered.
func allowedFileExtensionNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.allowed-file-extensions",
		Product:   "Jamf Pro",
		KeyLabel:  "extension",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "allowed file extension",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListAllowedFileExtensions(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.AllowedFileExtensions))
				for _, e := range list.AllowedFileExtensions {
					if e.ID != nil && e.Extension != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*e.ID), Key: *e.Extension})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports an extension that another record already holds.
func (r *AllowedFileExtensionResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: allowedFileExtensionNamespace(r.client),
			Path:      path.Root("extension"),
		}},
	})
}
