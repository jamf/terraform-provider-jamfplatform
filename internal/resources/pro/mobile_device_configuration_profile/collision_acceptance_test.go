// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_configuration_profile_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const collisionPlist = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1"><dict><key>PayloadUUID</key><string>%s</string><key>PayloadType</key><string>Configuration</string><key>PayloadOrganization</key><string>x</string><key>PayloadIdentifier</key><string>%s</string><key>PayloadDisplayName</key><string>p</string><key>PayloadDescription</key><string/><key>PayloadVersion</key><integer>1</integer><key>PayloadContent</key><array><dict><key>PayloadType</key><string>com.apple.applicationaccess</string><key>PayloadVersion</key><integer>1</integer><key>PayloadIdentifier</key><string>%s.r</string><key>PayloadUUID</key><string>%s</string><key>PayloadDisplayName</key><string>R</string><key>allowCamera</key><true/></dict></array></dict></plist>`

func createExistingProfile(t *testing.T, name string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	id := newUUID(t)
	payloads := proclassic.PayloadsXMLText(fmt.Sprintf(collisionPlist, newUUID(t), id, id, newUUID(t)))
	level := "Device Level"
	created, err := c.CreateMobileDeviceConfigurationProfileByID(ctx, "0", &proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{Name: &name, Level: &level, Payloads: &payloads},
	})
	if err != nil {
		t.Fatalf("creating mobile device configuration profile %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteMobileDeviceConfigurationProfileByID(ctx, strconv.Itoa(*created.ID)) })
}

func TestAccResource_MobileDeviceConfigurationProfile_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdcp-" + testhelpers.RunSuffix()
	payload := freshPayload(t, "profile_48.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingProfile(t, name) },
				Config:      configMinimal(strings.ToUpper(name), payload),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
			{
				Config:      configMinimal(" "+name+" ", payload),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}

func TestAccResource_MobileDeviceConfigurationProfile_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mine := "tf-acc-collide-mdcp-mine-" + suffix
	taken := "tf-acc-collide-mdcp-taken-" + suffix
	payload := freshPayload(t, "profile_48.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(mine, payload)},
			{
				PreConfig:   func() { createExistingProfile(t, taken) },
				Config:      configMinimal(taken, payload),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}

// twoProfilesConfig declares two profiles, each with its own payload since Jamf
// Pro keys a profile's identity on the payload UUID. The second depends on the
// first so the creates are sequential: Jamf's duplicate check is not atomic, and
// two concurrent creates of one name can both succeed, which would leave a test
// asserting on a race instead of on the refusal.
func twoProfilesConfig(nameOne, payloadOne, nameTwo, payloadTwo string) string {
	const address = `resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {`
	first := strings.Replace(configMinimal(nameOne, payloadOne), address,
		`resource "jamfplatform_pro_mobile_device_configuration_profile" "one" {`, 1)
	second := strings.Replace(configMinimal(nameTwo, payloadTwo), address,
		"resource \"jamfplatform_pro_mobile_device_configuration_profile\" \"two\" {\n  depends_on = [jamfplatform_pro_mobile_device_configuration_profile.one]\n", 1)
	return first + second
}

func TestAccResource_MobileDeviceConfigurationProfile_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix
	payloadOne := freshPayload(t, "profile_48.mobileconfig")
	payloadTwo := freshPayload(t, "profile_48.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: twoProfilesConfig(one, payloadOne, two, payloadTwo),
			},
			{
				Config:      twoProfilesConfig(one, payloadOne, one, payloadTwo),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_MobileDeviceConfigurationProfile_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()
	payloadOne := freshPayload(t, "profile_48.mobileconfig")
	payloadTwo := freshPayload(t, "profile_48.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      twoProfilesConfig(name, payloadOne, name, payloadTwo),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating\s+Jamf\s+Pro\s+mobile\s+device\s+configuration\s+profile.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
