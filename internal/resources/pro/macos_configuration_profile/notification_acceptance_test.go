// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Coverage for the Self Service notification pairing Jamf Pro's classic API
// cannot store. See buildSelfServiceNotification for the wire record and Jamf
// PI-1662 for the defect.

package macos_configuration_profile_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_MacOSConfigurationProfile_NotificationCenterRefused pins the
// plan-time refusal. It protects practitioners from an apply that would report
// success having stored "Self Service", and it goes when PI-1662 is fixed.
func TestAccResource_MacOSConfigurationProfile_NotificationCenterRefused(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-mcp-notifloc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      notificationCenterConfig(name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Notification Center cannot be combined with display_notifications`),
			},
		},
	})
}

func notificationCenterConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads            = %q
  }
  self_service = {
    display_notifications = true
    notification_location = "Self Service and Notification Center"
  }
}
`, name, minimalPayload)
}

const minimalPayload = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>PayloadContent</key><array/><key>PayloadDisplayName</key><string>tf acc notifloc</string><key>PayloadIdentifier</key><string>com.example.tfacc.notifloc</string><key>PayloadType</key><string>Configuration</string><key>PayloadUUID</key><string>5E2C1A70-9999-4888-8777-666655554444</string><key>PayloadVersion</key><integer>1</integer></dict></plist>`

// TestAccResource_MacOSConfigurationProfile_NotificationCenterIsUnwritable is
// the canary. It goes round the provider and writes the pairing through the SDK
// directly, then asserts the server did NOT store it.
//
// A FAILURE HERE IS GOOD NEWS: it means PI-1662 is fixed. Delete
// notificationCenterUnwritableValidator, restore the plain projection in
// buildSelfServiceNotification, drop the refusal test above, and delete this
// file.
//
// The write goes through the SDK rather than raw HTTP for the reason in
// feedback_probe_through_the_sdk_not_curl: it is the path the provider takes,
// so a fix the SDK's marshalling cannot express is not one this provider could
// adopt.
func TestAccResource_MacOSConfigurationProfile_NotificationCenterIsUnwritable(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	name := "tf-acc-mcp-notifloc-canary-" + testhelpers.RunSuffix()

	const center = "Self Service and Notification Center"
	yes := true
	method := center
	subject := "canary subject"
	distribution := "Make Available in Self Service"
	payload := minimalPayload

	created, err := c.CreateOSXConfigurationProfileByID(ctx, "0", &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{
			Name:               &name,
			DistributionMethod: &distribution,
			Payloads:           (*proclassic.PayloadsXMLText)(&payload),
		},
		SelfService: &proclassic.OsXConfigurationProfileSelfService{
			Notification:        &proclassic.NotificationValue{Enabled: &yes, Method: &method},
			NotificationSubject: &subject,
		},
	})
	if err != nil {
		t.Fatalf("create canary profile: %s", err)
	}
	id := fmt.Sprintf("%d", derefInt(created.ID))
	t.Cleanup(func() {
		if err := c.DeleteOSXConfigurationProfileByID(ctx, id); err != nil {
			t.Logf("cleanup: delete profile %s: %s", id, err)
		}
	})

	got, err := c.GetOSXConfigurationProfileByID(ctx, id)
	if err != nil {
		t.Fatalf("read back canary profile %s: %s", id, err)
	}
	ss := got.SelfService
	if ss == nil || ss.Notification == nil {
		t.Fatalf("profile %s came back with no self_service notification at all", id)
	}
	// notification_subject is the control: it travels in the same block and
	// persists, so an empty one means the write never landed and the result
	// says nothing about the pairing.
	if strings.TrimSpace(derefString(ss.NotificationSubject)) == "" {
		t.Fatalf("notification_subject did not persist either — the write did not land, so this proves nothing")
	}

	enabled, location := ss.Notification.Enabled, derefString(ss.Notification.Method)
	if enabled != nil && *enabled && location == center {
		t.Fatalf(
			"Jamf Pro now stores the pairing (enabled=%v, location=%q).\n\n"+
				"This is PI-1662 landing, not a regression. Remove the workaround:\n"+
				"  1. delete notificationCenterUnwritableValidator and its wiring in resource.go\n"+
				"  2. restore the plain projection in buildSelfServiceNotification\n"+
				"  3. drop TestAccResource_MacOSConfigurationProfile_NotificationCenterRefused\n"+
				"  4. delete this file and re-declare the pairing in the omit-retains config\n"+
				"  5. close Jamf PI-1662",
			*enabled, location,
		)
	}
	t.Logf("still unwritable, as expected: enabled=%v location=%q", enabled, location)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
