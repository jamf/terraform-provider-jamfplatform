// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package jamf_teacher_settings_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func teacherSafelistConfig(safelist string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_jamf_teacher_settings" "test" {
			timezone        = "Europe/London"
			safelisted_apps = %s
		}
	`, safelist)
}

// TestAccResource_ProJamfTeacherSettings_SafelistRoundTrip pins how the
// safelist set meets what Jamf Pro stores (wire-probed 2026-11-11). Identical
// entries collapse in Terraform before they reach Jamf Pro. The same bundle id
// under two names is stored as two entries, but the provider rejects it at
// plan time, mirroring the admin UI form's one entry per app. Bundle ids that
// differ only in case are two entries to Jamf Pro and to the set alike, so
// they round-trip and the next plan is empty.
func TestAccResource_ProJamfTeacherSettings_SafelistRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	restoreTeacherBaseline(t)

	caseOnly := `[
		{ name = "Example App", bundle_id = "com.example.tfacc.app" },
		{ name = "Example App", bundle_id = "COM.EXAMPLE.TFACC.APP" },
	]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkTeacherSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: teacherSafelistConfig(`[
					{ name = "Example One", bundle_id = "com.example.tfacc.app" },
					{ name = "Example Two", bundle_id = "com.example.tfacc.app" },
				]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Duplicate bundle_id within set`),
			},
			{
				Config: teacherSafelistConfig(caseOnly),
				Check:  resource.TestCheckResourceAttr(teacherResourceAddr, "safelisted_apps.#", "2"),
			},
			{
				Config: teacherSafelistConfig(caseOnly),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
