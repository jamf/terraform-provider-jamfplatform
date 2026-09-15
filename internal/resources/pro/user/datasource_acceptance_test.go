// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package user_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// createInventoryUserFixture creates a throwaway Jamf Pro inventory user and
// registers its deletion, returning the username. The provider ships no user
// resource, so the data-source test self-provisions its subject this way rather
// than depending on a pre-existing tenant user named by an env var.
//
// Create and delete go through the CLASSIC /JSSResource/users API on purpose:
// the Pro v1 /users endpoint is broken on Jamf Pro 11.28 (POST 500s but still
// persists the record; DELETE 500s and is a silent no-op), whereas the classic
// endpoint works on every version (wire-probed). The data source under test
// still reads via Pro v1, which is unaffected.
func createInventoryUserFixture(t *testing.T) string {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	username := "tf-acc-user-" + testhelpers.RunSuffix()

	id, _, err := c.ApplyUser(ctx, &proclassic.UserPost{Name: &username})
	if err != nil {
		t.Fatalf("creating inventory user fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := c.DeleteUserByID(ctx, id); err != nil {
			t.Errorf("deleting inventory user fixture %s: %v", id, err)
			return
		}
		// Confirm the delete took effect with an authoritative GET — never trust
		// the delete status alone (the v1 delete is a silent no-op on 11.28).
		if _, err := c.GetUserByID(ctx, id); err == nil {
			t.Errorf("inventory user fixture %s still present after delete", id)
		} else if !helpers.IsNotFoundError(err) {
			t.Errorf("verifying inventory user fixture %s deletion: %v", id, err)
		}
	})
	return username
}

// TestAccDataSource_ProUser_ByUsernameThenID looks a user up by username, then
// re-reads the resolved ID through the same data source and asserts the records
// agree. Self-provisions its subject via createInventoryUserFixture.
func TestAccDataSource_ProUser_ByUsernameThenID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	username := createInventoryUserFixture(t)

	cfg := fmt.Sprintf(`
		data "jamfplatform_pro_user" "by_name" {
			username = %q
		}
		data "jamfplatform_pro_user" "by_id" {
			id = data.jamfplatform_pro_user.by_name.id
		}
	`, username)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.jamfplatform_pro_user.by_name", "id"),
				resource.TestCheckResourceAttr("data.jamfplatform_pro_user.by_name", "username", username),
				// The id-keyed read must resolve the same record.
				resource.TestCheckResourceAttrPair(
					"data.jamfplatform_pro_user.by_id", "username",
					"data.jamfplatform_pro_user.by_name", "username",
				),
				resource.TestCheckResourceAttrPair(
					"data.jamfplatform_pro_user.by_id", "id",
					"data.jamfplatform_pro_user.by_name", "id",
				),
			),
		}},
	})
}
