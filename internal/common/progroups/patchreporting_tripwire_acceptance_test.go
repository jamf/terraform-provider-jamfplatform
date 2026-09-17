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

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAcceptance_ProGroups_PatchReportingStillRefused asserts a Jamf Pro DEFECT
// and therefore FAILS WHEN THE DEFECT IS FIXED. That is deliberate.
//
// progroups.ValidatePatchReportingCriteria refuses a patch-reporting criterion at
// plan time, so once that validator is in place no other test in the suite ever
// reaches the endpoint with one — every smart-group test passes precisely because
// the request is never made. A validator that goes on refusing something the
// server has started accepting would therefore be invisible: it would quietly
// block a configuration that works, and the whole suite would stay green.
//
// This test is the only thing that would notice, so it drives the request itself
// rather than going through Terraform, and it asserts the 400. When it fails,
// progroups.PatchReportingTripwireNote says what to remove.
//
// It mints its own fixtures because the criterion vocabulary is derived from the
// tenant's own patch titles: no patch software title, no criterion to send. It
// skips rather than infers when the tenant's title source publishes nothing,
// since an absent catalogue cannot distinguish "refused" from "no such criterion"
// — which is exactly the trap that made the first pass at this finding
// unprovable.
func TestAcceptance_ProGroups_PatchReportingStillRefused(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()

	base := testhelpers.NewAcceptanceClient(t)
	classic := proclassic.New(base)
	proClient := pro.New(base)

	title, titleID := configureAnyPatchSoftwareTitle(t, ctx, classic)
	t.Cleanup(func() {
		if err := classic.DeletePatchSoftwareTitleByID(ctx, titleID); err != nil {
			t.Logf("cleaning up patch software title %s: %v", titleID, err)
		}
	})

	criterionName := progroups.PatchReportingCriterionPrefix + title.appName
	groupID := createClassicSmartGroupWithCriterion(t, ctx, classic, criterionName, title.currentVersion)
	t.Cleanup(func() {
		if err := classic.DeleteComputerGroupByID(ctx, groupID); err != nil {
			t.Logf("cleaning up computer group %s: %v", groupID, err)
		}
	})

	stored, err := proClient.GetSmartComputerGroupV3(ctx, groupID)
	if err != nil {
		t.Fatalf("reading the classic-made patch group through the smart-group endpoint: %v", err)
	}
	if stored.Criteria == nil || len(*stored.Criteria) == 0 {
		t.Fatalf("the smart-group read returned no criteria for group %s, so the fixture did not take", groupID)
	}
	if got := (*stored.Criteria)[0].Name; got != criterionName {
		t.Fatalf("the smart-group read reported criterion %q, want %q", got, criterionName)
	}

	_, err = proClient.UpdateSmartComputerGroupV3(ctx, groupID, &pro.SmartComputerGroupV3{
		Name:     stored.Name,
		SiteID:   stored.SiteID,
		Criteria: stored.Criteria,
	})
	if err == nil {
		t.Fatalf("writing the group's own unmodified criteria back SUCCEEDED.\n\n%s", progroups.PatchReportingTripwireNote)
	}

	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("expected a Jamf API error refusing the patch reporting criterion, got %v", err)
	}
	if !apiErr.HasStatus(400) || !strings.Contains(err.Error(), criterionName) {
		t.Fatalf("the refusal changed shape, so the guard's justification needs re-probing: %v", err)
	}
	t.Logf("PI-1183 still present: the smart-group write refuses %q with %v", criterionName, apiErr.StatusCode)
}

// patchTitleFixture is the catalogue entry the tripwire builds its fixtures from.
type patchTitleFixture struct {
	appName        string
	nameID         string
	sourceID       int
	currentVersion string
}

// patchTitleSourceID is Jamf's own patch title source, which every tenant has.
// A tenant may also have external sources, but only the Jamf one is guaranteed,
// so the tripwire looks there and skips rather than hunting.
const patchTitleSourceID = 1

// configureAnyPatchSoftwareTitle configures the first catalogue title the tenant
// will actually accept, and returns it with its new id.
//
// It walks the catalogue rather than taking the first entry, because a title can
// be server-side corrupt in a way that is intrinsic to the title and not to the
// tenant: catalogue title 518 ("010 Editor") is a standing example, and it sorts
// first, so taking entry zero is the least robust choice available. A create that
// fails moves to the next title; the test only gives up when the catalogue is
// exhausted.
//
// Configuring a title is what makes Jamf Pro generate the per-title
// "Patch Reporting: <name>" criterion in the first place, which is why the
// fixture exists at all.
//
// The create goes through the classic endpoint because it is the only one that
// mints an id: the configurations surface takes the classic title id as input, so
// there is no modern create to use here. See
// internal/resources/pro/patch_software_title/crud.go for the full reasoning.
func configureAnyPatchSoftwareTitle(t *testing.T, ctx context.Context, classic *proclassic.Client) (patchTitleFixture, string) {
	t.Helper()

	titles, err := classic.ListPatchAvailableTitlesBySourceID(ctx, strconv.Itoa(patchTitleSourceID))
	if err != nil {
		t.Skipf("listing available patch titles on source %d: %v", patchTitleSourceID, err)
	}
	if titles == nil || titles.AvailableTitles == nil || titles.AvailableTitles.AvailableTitle == nil {
		t.Skipf("the tenant's patch title source %d publishes nothing, so there is no patch reporting criterion to send", patchTitleSourceID)
	}

	attempted := 0
	for _, at := range *titles.AvailableTitles.AvailableTitle {
		if at.AppName == nil || *at.AppName == "" || at.NameID == nil || *at.NameID == "" {
			continue
		}
		fixture := patchTitleFixture{
			appName:  *at.AppName,
			nameID:   *at.NameID,
			sourceID: patchTitleSourceID,
		}
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
			t.Logf("catalogue title %q (%s) could not be configured, trying the next one: %v", fixture.appName, fixture.nameID, err)
			continue
		}
		if created == nil || created.ID == nil {
			t.Logf("catalogue title %q (%s) was configured but reported no id, trying the next one", fixture.appName, fixture.nameID)
			continue
		}
		return fixture, strconv.Itoa(*created.ID)
	}

	if attempted == 0 {
		t.Skipf("the tenant's patch title source %d publishes no usable title", patchTitleSourceID)
	}
	t.Skipf("none of the %d catalogue titles on source %d could be configured, so there is no patch reporting criterion to send", attempted, patchTitleSourceID)
	return patchTitleFixture{}, ""
}

// createClassicSmartGroupWithCriterion creates a smart computer group carrying
// one criterion, through the classic endpoint.
//
// Classic is the point: it accepts the patch reporting criterion the smart-group
// endpoint refuses, which is both the workaround Jamf documents and the control
// that proves the criterion name is spelled correctly. Without it a 400 from the
// smart-group write would be indistinguishable from a typo.
func createClassicSmartGroupWithCriterion(t *testing.T, ctx context.Context, classic *proclassic.Client, criterionName, value string) string {
	t.Helper()

	groupName := acctest.RandomWithPrefix("tf-acc-patch-tripwire")
	isSmart := true
	priority := 0
	andOr := proclassic.CriterionAndOrAnd
	searchType := "is"

	created, err := classic.CreateComputerGroupByID(ctx, "0", &proclassic.ComputerGroupPost{
		Name:    &groupName,
		IsSmart: &isSmart,
		Criteria: &proclassic.ComputerGroupPostCriteria{
			Criterion: &[]proclassic.Criterion{{
				Name:       &criterionName,
				Priority:   &priority,
				AndOr:      &andOr,
				SearchType: &searchType,
				Value:      &value,
			}},
		},
	})
	if err != nil {
		t.Fatalf("creating the classic smart group fixture with criterion %q: %v", criterionName, err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("creating the classic smart group fixture returned no id")
	}
	return strconv.Itoa(*created.ID)
}
