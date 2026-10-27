// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_account"

// accountNamespace is the uniqueness rule for one key of a Jamf Pro administrator
// account. Jamf Pro refuses a second account whose username or email matches an
// existing one with 400 INVALID_REQUEST_PARAMETER_VALUE "Duplicated user account",
// and compares both without regard to case, trailing spaces or accents. A leading
// space keeps two values apart. The key picks the account field read from the list.
func accountNamespace(id, keyLabel string, key func(pro.UserAccount) *string, client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        id,
		Product:   "Jamf Pro",
		KeyLabel:  keyLabel,
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "administrator account",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListAccountsV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed))
				for _, a := range listed {
					if v := key(a); a.ID != nil && v != nil {
						entries = append(entries, collision.Entry{ID: *a.ID, Key: *v})
					}
				}
				return entries, nil
			},
		}},
	}
}

func usernameNamespace(client *pro.Client) collision.Namespace {
	return accountNamespace("pro.account-usernames", "username", func(a pro.UserAccount) *string { return a.Username }, client)
}

func emailNamespace(client *pro.Client) collision.Namespace {
	return accountNamespace("pro.account-emails", "email address", func(a pro.UserAccount) *string { return a.Email }, client)
}

// reportNameCollision reports a username or email address that another
// administrator account already holds. An empty email address is not checked.
func (r *AccountResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.proClient == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{
			{Namespace: usernameNamespace(r.proClient), Path: path.Root("username")},
			{Namespace: emailNamespace(r.proClient), Path: path.Root("email_address")},
		},
	})
}
