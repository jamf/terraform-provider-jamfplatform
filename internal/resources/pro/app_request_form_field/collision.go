// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_app_request_form_field"

// appRequestFormFieldNamespace is the strict uniqueness rule for App Request form
// field titles. Jamf Pro stores every variant of the name, so there is no server
// rule to mirror. The comparison folds case, trailing spaces and accents, so names
// a person reads as the same object are reported. The rule applies only when
// include_unenforced is set.
func appRequestFormFieldNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.app-request-form-fields",
		Product:   "Jamf Pro",
		KeyLabel:  "title",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "App Request form field",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListAppRequestFormInputFieldsV1(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.Results))
				for _, f := range list.Results {
					if f.ID != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*f.ID), Key: f.Title})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a title that another App Request form field already holds, when
// include_unenforced is set.
func (r *AppRequestFormFieldResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: appRequestFormFieldNamespace(r.client),
			Path:      path.Root("title"),
		}},
	})
}
