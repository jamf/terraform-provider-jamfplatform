// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package macos_configuration_profile_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const collisionPlist = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1"><dict><key>PayloadUUID</key><string>%s</string><key>PayloadType</key><string>Configuration</string><key>PayloadOrganization</key><string>x</string><key>PayloadIdentifier</key><string>%s</string><key>PayloadDisplayName</key><string>p</string><key>PayloadDescription</key><string/><key>PayloadVersion</key><integer>1</integer><key>PayloadEnabled</key><true/><key>PayloadRemovalDisallowed</key><true/><key>PayloadScope</key><string>System</string><key>PayloadContent</key><array><dict><key>PayloadType</key><string>com.apple.dock</string><key>PayloadVersion</key><integer>1</integer><key>PayloadIdentifier</key><string>%s.dock</string><key>PayloadUUID</key><string>%s</string><key>PayloadDisplayName</key><string>Dock</string><key>autohide</key><true/></dict></array></dict></plist>`

func collisionUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating UUID: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func createExistingProfile(t *testing.T, name string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	id := collisionUUID(t)
	payloads := proclassic.PayloadsXMLText(fmt.Sprintf(collisionPlist, collisionUUID(t), id, id, collisionUUID(t)))
	level := "computer"
	created, err := c.CreateOSXConfigurationProfileByID(ctx, "0", &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{Name: &name, Level: &level, Payloads: &payloads},
	})
	if err != nil {
		t.Fatalf("creating macOS configuration profile %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteOSXConfigurationProfileByID(ctx, strconv.Itoa(*created.ID)) })
}

func TestAccResource_MacOSConfigurationProfile_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mcp-" + testhelpers.RunSuffix()
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")

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

func TestAccResource_MacOSConfigurationProfile_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mine := "tf-acc-collide-mcp-mine-" + suffix
	taken := "tf-acc-collide-mcp-taken-" + suffix
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")

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
