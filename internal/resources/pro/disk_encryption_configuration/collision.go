// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_disk_encryption_configuration"

// diskEncryptionConfigurationNamespace is the uniqueness rule for disk encryption configuration names. Jamf Pro refuses a
// duplicate with 409, and the Classic API trims the name and compares it without regard to case, leading and trailing spaces, or accents.
func diskEncryptionConfigurationNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.disk-encryption-configurations",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "disk encryption configuration",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListDiskEncryptionConfigurations(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed.DiskEncryptionConfigurations))
				for _, item := range listed.DiskEncryptionConfigurations {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a disk encryption configuration name that another disk encryption configuration already holds.
func (r *DiskEncryptionConfigurationResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: diskEncryptionConfigurationNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
