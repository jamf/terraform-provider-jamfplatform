// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package jamf_connect_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// jamfConnectAccVersion is a Jamf Connect version known-valid against the test
// tenant's release catalog. The server validates version against Jamf's hosted
// catalog (an arbitrary semver is rejected), so if this ages out, set it to a
// version currently offered in Settings → Jamf apps → Jamf Connect.
const jamfConnectAccVersion = "2.45.1"

// connectProfileName is the display name of the throwaway macOS configuration
// profile the test creates (and adopts). Unique enough to match back from the
// Jamf Connect list.
const connectProfileName = "tf-acc-jamf-connect"

// connectPayload is a minimal, synthetic com.jamf.connect.login mobileconfig.
// A configuration profile carrying this payload auto-links into Jamf Connect,
// which is what makes it adoptable. No org-identifying data.
const connectPayload = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadContent</key>
			<dict>
				<key>com.jamf.connect.login</key>
				<dict>
					<key>Forced</key>
					<array>
						<dict>
							<key>mcx_preference_settings</key>
							<dict>
								<key>OIDCProvider</key>
								<string>Okta</string>
							</dict>
						</dict>
					</array>
				</dict>
			</dict>
			<key>PayloadType</key>
			<string>com.apple.ManagedClient.preferences</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadScope</key>
	<string>System</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`

func jamfConnectClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

// findLinkedByName returns the Jamf Connect-linked profile with the given
// display name, or nil.
func findLinkedByName(t *testing.T, name string) *pro.LinkedConnectProfile {
	t.Helper()
	profiles, err := jamfConnectClient(t).ListJamfConnectConfigProfilesV1(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("listing Jamf Connect config profiles: %v", err)
	}
	for i := range profiles {
		if profiles[i].ProfileName != nil && *profiles[i].ProfileName == name {
			return &profiles[i]
		}
	}
	return nil
}

// configProfileOnly is the throwaway macOS configuration profile that carries
// the Jamf Connect payload (and therefore auto-links into Jamf Connect).
func configProfileOnly() string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "connect" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
}
`, connectProfileName, connectPayload)
}

// configWithResource adds the jamf_connect resource (keyed by the profile's id)
// plus data sources looking the same profile up by id and by name.
func configWithResource(deploymentType, versionLine string) string {
	return configProfileOnly() + fmt.Sprintf(`
resource "jamfplatform_pro_jamf_connect" "test" {
  profile_id           = jamfplatform_pro_macos_configuration_profile.connect.id
  auto_deployment_type = %q
%s
}

data "jamfplatform_pro_jamf_connect" "by_id" {
  profile_id = jamfplatform_pro_jamf_connect.test.profile_id
}

data "jamfplatform_pro_jamf_connect" "by_name" {
  profile_name = jamfplatform_pro_macos_configuration_profile.connect.general.name
  depends_on   = [jamfplatform_pro_jamf_connect.test]
}
`, deploymentType, versionLine)
}

// TestAccResource_ProJamfConnect exercises the full adoption lifecycle: adopt a
// Connect-linked profile by profile_id and set a deployment type + version,
// update across deployment types (including clearing the version on NONE),
// read both data sources, import by profile_id, and finally prove the
// state-only delete leaves Jamf Connect (and its applied settings) on the
// profile. CheckDestroy confirms the underlying profile teardown.
func TestAccResource_ProJamfConnect(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const rsrc = "jamfplatform_pro_jamf_connect.test"
	versionLine := fmt.Sprintf("  version = %q", jamfConnectAccVersion)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkProfileDestroyed(t),
		Steps: []resource.TestStep{
			{
				// Adopt + set PATCH_UPDATES with a version; verify the resource
				// and both data-source lookups.
				Config: configWithResource("PATCH_UPDATES", versionLine),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rsrc, "auto_deployment_type", "PATCH_UPDATES"),
					resource.TestCheckResourceAttr(rsrc, "version", jamfConnectAccVersion),
					resource.TestCheckResourceAttrSet(rsrc, "profile_id"),
					resource.TestCheckResourceAttrSet(rsrc, "config_profile_uuid"),
					resource.TestCheckResourceAttrSet(rsrc, "id"),
					resource.TestCheckResourceAttr(rsrc, "profile_name", connectProfileName),
					resource.TestCheckResourceAttrSet(rsrc, "scope_description"),
					resource.TestCheckResourceAttrSet(rsrc, "site_id"),
					// Data source (by profile_id) matches the resource.
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_jamf_connect.by_id", "config_profile_uuid", rsrc, "config_profile_uuid"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_jamf_connect.by_id", "auto_deployment_type", "PATCH_UPDATES"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_jamf_connect.by_id", "version", jamfConnectAccVersion),
					// Data source (by profile_name) resolves the same profile.
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_jamf_connect.by_name", "profile_id", rsrc, "profile_id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_jamf_connect.by_name", "profile_name", connectProfileName),
				),
			},
			{
				// Update to NONE: the version must clear.
				Config: configWithResource("NONE", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rsrc, "auto_deployment_type", "NONE"),
					resource.TestCheckNoResourceAttr(rsrc, "version"),
				),
			},
			{
				// Re-enable with MINOR_AND_PATCH_UPDATES (third enum value).
				Config: configWithResource("MINOR_AND_PATCH_UPDATES", versionLine),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rsrc, "auto_deployment_type", "MINOR_AND_PATCH_UPDATES"),
					resource.TestCheckResourceAttr(rsrc, "version", jamfConnectAccVersion),
				),
			},
			{
				// Import by profile_id.
				ResourceName:            rsrc,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				// Remove the resource (and data sources), keeping the profile.
				// The state-only delete must leave Jamf Connect on the profile
				// with its last-applied settings intact.
				Config: configProfileOnly(),
				Check: func(_ *terraform.State) error {
					p := findLinkedByName(t, connectProfileName)
					if p == nil {
						return fmt.Errorf("Jamf Connect link was removed from the profile after destroying the resource; the delete should be a no-op")
					}
					if p.AutoDeploymentType == nil || *p.AutoDeploymentType != "MINOR_AND_PATCH_UPDATES" {
						return fmt.Errorf("deployment settings were reset on resource destroy; autoDeploymentType = %v, want MINOR_AND_PATCH_UPDATES", p.AutoDeploymentType)
					}
					return nil
				},
			},
		},
	})
}

// TestAccResource_ProJamfConnect_Validation covers the plan-time version ↔
// auto_deployment_type config validator (no profile needed — the validator
// fires before any API call).
func TestAccResource_ProJamfConnect_Validation(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_jamf_connect" "test" {
  profile_id           = 1
  auto_deployment_type = "NONE"
  version              = "2.45.1"
}
`,
				ExpectError: regexp.MustCompile(`(?s)ignores the version`),
			},
			{
				Config: `
resource "jamfplatform_pro_jamf_connect" "test" {
  profile_id           = 1
  auto_deployment_type = "PATCH_UPDATES"
}
`,
				ExpectError: regexp.MustCompile(`(?s)version must be set`),
			},
		},
	})
}

// checkProfileDestroyed is the CheckDestroy: once the throwaway macOS profile
// is destroyed it no longer carries a Jamf Connect payload, so it drops out of
// the Jamf Connect list.
func checkProfileDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if p := findLinkedByName(t, connectProfileName); p != nil {
			return fmt.Errorf("configuration profile %q still linked to Jamf Connect after destroy", connectProfileName)
		}
		return nil
	}
}
