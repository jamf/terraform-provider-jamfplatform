// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package progroups_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// removalNote is printed when a canary fails, so the failure explains itself
// instead of reading as a regression in this provider.
const removalNote = "Jamf Pro ACCEPTED a criterion combination it used to refuse, so this defect looks fixed. " +
	"That is what this test exists to detect: the fallback write path is invisible to every other test in the suite, " +
	"because a group that no longer needs it simply takes the normal path and passes either way. " +
	"Check the remaining canaries, and once none of them fail, delete progroups.ModernWriteRefused and its call sites, " +
	"the FallbackWarning, the known-defect paragraphs in the two smart group descriptions, and this test."

// TestAcceptance_ProGroups_ModernWriteRefusalsStillPresent asserts Jamf Pro
// DEFECTS and therefore FAILS WHEN THEY ARE FIXED. That is deliberate.
//
// The provider writes a smart group through Jamf Pro's older group interface when
// the group endpoint refuses its criteria, which it does for combinations the
// admin UI offers and the older interface stores. Once that fallback is in place
// nothing else in the suite can tell whether it is still needed: a group that no
// longer trips the refusal just takes the normal path, and the tests pass either
// way. So the workaround would outlive the defect silently, and keep a group on a
// path whose operator checking is weaker, for no reason.
//
// Each subtest drives the group endpoint directly rather than going through
// Terraform, and asserts both that the write is refused and that
// progroups.ModernWriteRefused classifies the refusal — the second half matters
// as much as the first, because a refusal whose wording drifts would stop
// triggering the fallback and start surfacing as a raw apply failure.
//
// Subtests skip rather than infer when the tenant has nothing to build the
// criterion from. An absent extension attribute or patch title cannot
// distinguish "refused" from "no such criterion", which is the ambiguity that
// made the first pass at this finding unprovable.
func TestAcceptance_ProGroups_ModernWriteRefusalsStillPresent(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()

	base := testhelpers.NewAcceptanceClient(t)
	proClient := pro.New(base)
	classic := proclassic.New(base)

	t.Run("patch reporting criterion on a computer smart group", func(t *testing.T) {
		title, titleID := configureAnyPatchSoftwareTitle(t, ctx, classic)
		t.Cleanup(func() {
			if err := classic.DeletePatchSoftwareTitleByID(ctx, titleID); err != nil {
				t.Logf("cleaning up patch software title %s: %v", titleID, err)
			}
		})

		criterion := pro.ComputerSmartGroupCriteriaV2{
			Name:       progroups.PatchReportingCriterionPrefix + title.appName,
			SearchType: "is",
			Value:      "Latest Version",
			AndOr:      pro.ComputerSmartGroupCriteriaV2AndOrAnd,
			Priority:   0,
		}
		assertComputerCriterionRefused(t, ctx, proClient, criterion, "PI-1286 / PI-1183 / PI-626")
	})

	t.Run("has on a computer extension attribute", func(t *testing.T) {
		name := firstComputerExtensionAttributeName(t, ctx, proClient)
		criterion := pro.ComputerSmartGroupCriteriaV2{
			Name:       name,
			SearchType: "has",
			Value:      "canary",
			AndOr:      pro.ComputerSmartGroupCriteriaV2AndOrAnd,
			Priority:   0,
		}
		assertComputerCriterionRefused(t, ctx, proClient, criterion, "PI-1032")
	})

	t.Run("has on a mobile device extension attribute", func(t *testing.T) {
		name := firstMobileExtensionAttributeName(t, ctx, proClient)
		criteria := []pro.MobileDeviceSmartGroupCriteriaV2{{
			Name:       name,
			SearchType: "has",
			Value:      "canary",
			AndOr:      pro.MobileDeviceSmartGroupCriteriaV2AndOrAnd,
			Priority:   0,
		}}
		groupName := acctest.RandomWithPrefix("tf-acc-canary")
		site := progroups.NoSiteID
		_, err := proClient.CreateSmartMobileDeviceGroupV2(ctx, &pro.SmartGroupAssignmentV2{
			GroupName: groupName,
			SiteID:    &site,
			Criteria:  &criteria,
		}, true)
		assertRefused(t, err, name, "PI-1032, mobile — the ticket is filed against computer groups only")
	})
}

// assertComputerCriterionRefused attempts a computer smart group carrying one
// criterion and asserts the group endpoint refuses it.
func assertComputerCriterionRefused(t *testing.T, ctx context.Context, proClient *pro.Client, criterion pro.ComputerSmartGroupCriteriaV2, ticket string) {
	t.Helper()

	criteria := []pro.ComputerSmartGroupCriteriaV2{criterion}
	site := progroups.NoSiteID
	_, err := proClient.CreateSmartComputerGroupV3(ctx, &pro.SmartComputerGroupV3{
		Name:     acctest.RandomWithPrefix("tf-acc-canary"),
		SiteID:   &site,
		Criteria: &criteria,
	}, true)
	assertRefused(t, err, criterion.Name, ticket)
}

// assertRefused pins both halves of the contract: the write is refused, and
// progroups.ModernWriteRefused recognises the refusal.
//
// The second half is the one that rots quietly. If Jamf rewords the message, the
// classifier stops matching, the fallback stops engaging, and a configuration
// that worked yesterday fails at apply with a raw error — with every other test
// still green, because they never reach the endpoint with such a criterion.
func assertRefused(t *testing.T, err error, criterionName, ticket string) {
	t.Helper()

	if err == nil {
		t.Fatalf("the group endpoint accepted %q (%s).\n\n%s", criterionName, ticket, removalNote)
	}
	if !progroups.ModernWriteRefused(err) {
		t.Fatalf("the group endpoint refused %q (%s) but progroups.ModernWriteRefused did not recognise the refusal, "+
			"so the fallback would not engage and this would surface as a raw apply failure. Re-read the wording and "+
			"update the classifier: %v", criterionName, ticket, err)
	}
	if !strings.Contains(err.Error(), criterionName) {
		t.Errorf("the refusal for %q no longer names the criterion, which the diagnostics rely on: %v", criterionName, err)
	}
	t.Logf("%s still present: the group endpoint refuses %q", ticket, criterionName)
}

// firstComputerExtensionAttributeName returns any computer extension attribute's
// name, or skips.
func firstComputerExtensionAttributeName(t *testing.T, ctx context.Context, proClient *pro.Client) string {
	t.Helper()

	attrs, err := proClient.ListComputerExtensionAttributesV1(ctx, nil, "")
	if err != nil {
		t.Skipf("listing computer extension attributes: %v", err)
	}
	for _, a := range attrs {
		if a.Name != "" {
			return a.Name
		}
	}
	t.Skip("the tenant has no computer extension attribute, so there is no criterion to refuse")
	return ""
}

// firstMobileExtensionAttributeName returns any mobile device extension
// attribute's name, or skips.
func firstMobileExtensionAttributeName(t *testing.T, ctx context.Context, proClient *pro.Client) string {
	t.Helper()

	attrs, err := proClient.ListMobileDeviceExtensionAttributesV1(ctx, nil, "")
	if err != nil {
		t.Skipf("listing mobile device extension attributes: %v", err)
	}
	for _, a := range attrs {
		if a.Name != "" {
			return a.Name
		}
	}
	t.Skip("the tenant has no mobile device extension attribute, so there is no criterion to refuse")
	return ""
}

// patchTitleFixture is the catalogue entry the patch canary builds its fixture
// from.
type patchTitleFixture struct {
	appName        string
	nameID         string
	sourceID       int
	currentVersion string
}

// patchTitleSourceID is Jamf's own patch title source, which every tenant has.
const patchTitleSourceID = 1

// configureAnyPatchSoftwareTitle configures the first catalogue title the tenant
// will accept, and returns it with its new id.
//
// It walks the catalogue rather than taking the first entry, because a title can
// be server-side corrupt in a way intrinsic to the title and not the tenant:
// catalogue title 518 ("010 Editor") is a standing example and it sorts first, so
// entry zero is the least robust choice available.
//
// Configuring a title is what makes Jamf Pro generate the per-title
// "Patch Reporting: <name>" criterion, which is why the fixture exists at all.
// The create uses the older interface because it is the only one that mints an
// id — see internal/resources/pro/patch_software_title/crud.go.
func configureAnyPatchSoftwareTitle(t *testing.T, ctx context.Context, classic *proclassic.Client) (patchTitleFixture, string) {
	t.Helper()

	titles, err := classic.ListPatchAvailableTitlesBySourceID(ctx, strconv.Itoa(patchTitleSourceID))
	if err != nil {
		t.Skipf("listing available patch titles on source %d: %v", patchTitleSourceID, err)
	}
	if titles == nil || titles.AvailableTitles == nil || titles.AvailableTitles.AvailableTitle == nil {
		t.Skipf("the tenant's patch title source %d publishes nothing, so there is no patch reporting criterion to refuse", patchTitleSourceID)
	}

	attempted := 0
	for _, at := range *titles.AvailableTitles.AvailableTitle {
		if at.AppName == nil || *at.AppName == "" || at.NameID == nil || *at.NameID == "" {
			continue
		}
		fixture := patchTitleFixture{appName: *at.AppName, nameID: *at.NameID, sourceID: patchTitleSourceID}
		if at.CurrentVersion != nil {
			fixture.currentVersion = *at.CurrentVersion
		}

		attempted++
		created, err := classic.CreatePatchSoftwareTitleByID(ctx, "0", &proclassic.PatchSoftwareTitle{
			Name:     &fixture.appName,
			NameID:   &fixture.nameID,
			SourceID: &fixture.sourceID,
		})
		if err != nil {
			t.Logf("catalogue title %q (%s) could not be configured, trying the next: %v", fixture.appName, fixture.nameID, err)
			continue
		}
		if created == nil || created.ID == nil {
			t.Logf("catalogue title %q (%s) reported no id, trying the next", fixture.appName, fixture.nameID)
			continue
		}
		return fixture, strconv.Itoa(*created.ID)
	}

	if attempted == 0 {
		t.Skipf("the tenant's patch title source %d publishes no usable title", patchTitleSourceID)
	}
	t.Skipf("none of the %d catalogue titles on source %d could be configured", attempted, patchTitleSourceID)
	return patchTitleFixture{}, ""
}
