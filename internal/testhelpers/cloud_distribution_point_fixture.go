// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// EnsurePrincipalCloudDistributionPoint makes the tenant's existing cloud
// distribution point the principal (master) DP and restores the original
// `master` flag on t.Cleanup. Policies that reference the "default" distribution
// point in their package configuration cannot be saved unless a principal DP is
// configured (the server otherwise rejects the write with 409 "Problem with
// distribution server").
//
// It NEVER creates or deletes the cloud distribution point: deleting one
// permanently wipes every package, in-house app, and eBook hosted in Jamf Cloud
// (see the cloud_distribution_point resource docs). It only PATCHes the `master`
// flag, which is non-destructive and reversible.
//
// The calling test is skipped when no cloud distribution point is configured
// (cdn_type NONE) — there is nothing to make principal, and standing one up
// would be destructive to tear down.
func EnsurePrincipalCloudDistributionPoint(t *testing.T) {
	t.Helper()
	client := pro.New(NewAcceptanceClient(t))
	ctx := context.Background()

	got, err := client.GetCloudDistributionPointV1(ctx)
	if err != nil {
		t.Fatalf("reading cloud distribution point: %v", err)
	}
	if got == nil || strings.EqualFold(got.CdnType, pro.CloudDistributionPointCdnTypeNone) {
		t.Skip("skipping: no cloud distribution point configured on tenant; cannot make one principal without a destructive create")
	}

	if got.Master != nil && *got.Master {
		return // already principal — nothing to change, nothing to restore
	}

	if _, err := client.UpdateCloudDistributionPointV1(ctx, cdpMasterPatch(got, true)); err != nil {
		t.Fatalf("setting cloud distribution point as principal: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.UpdateCloudDistributionPointV1(context.Background(), cdpMasterPatch(got, false))
	})
}

// cdpMasterPatch builds the minimal merge-patch that flips only the master flag.
// cdn_type and username are echoed from the current record (cdn_type is
// mandatory in every PATCH body); password is write-only, never returned, and
// not needed for JAMF_CLOUD, so it is left empty.
func cdpMasterPatch(current *pro.CloudDistributionPoint, master bool) *pro.CloudDistributionPoint {
	return &pro.CloudDistributionPoint{
		CdnType:  current.CdnType,
		Username: current.Username,
		Master:   &master,
	}
}

// RequireJCDSUploads skips the calling test unless the tenant can actually take
// a package upload to the Jamf Cloud Distribution Point.
//
// Every upload the provider performs ends in a verification poll that waits for
// JCDS to converge on the uploaded file's hash. On a tenant with no cloud
// distribution point that poll can never converge, so the test does not fail
// fast: it burns the resource's whole create timeout — 30 minutes apiece, two
// hours across the package suite in the 2026-09-03 pro lane — and then reports a
// timeout that reads like a provider defect rather than the estate condition it
// is.
//
// A tenant without a cloud distribution point is a legitimate acceptance estate,
// which is why this is a skip. Standing one up is not an option the suite has:
// creating a JCDS record is easy, but DELETING one permanently wipes every
// package, in-house app and eBook hosted in Jamf Cloud, so a fixture that
// created it could never tear it down (see EnsurePrincipalCloudDistributionPoint,
// which refuses the same thing for the same reason).
//
// Nothing here is inferred from an absence. Both endpoints answer positively
// about the tenant's configuration — cdnType names the CDN in use, "NONE" when
// there is none, and the upload-capability record states directly whether a
// direct upload is possible — so a skip always rests on the estate having said
// so. A read that FAILS is therefore a defect and fails the test, exactly as
// RequireAIGovernanceTool's catalogue read does: degrading a broken Pro path
// into a skip would empty the upload suite while the lane still reported green.
//
// The skip has a cost worth naming, because it is the shape this repo otherwise
// fights. On an estate with no cloud distribution point the lane goes green with
// every upload test unrun, and the helper is deliberately not named
// AccPreCheck*, so internal/conformance/acc_lanes_test.go does not demand a
// require token that would promote the skip to a lane failure. That is the right
// call — a tenant without a distribution point is a legitimate estate, not a
// missing secret — but it means a green pro lane says nothing about the upload
// path. TESTING.md §Tenant-prerequisite fixtures records the same caveat for a
// reader who only ever sees the lane result.
func RequireJCDSUploads(t *testing.T) {
	t.Helper()
	client := pro.New(NewAcceptanceClient(t))
	ctx := context.Background()

	cdp, err := client.GetCloudDistributionPointV1(ctx)
	if err != nil {
		t.Fatalf("reading cloud distribution point: %v", err)
	}
	switch {
	case cdp == nil || strings.EqualFold(cdp.CdnType, pro.CloudDistributionPointCdnTypeNone):
		t.Skip("skipping: no cloud distribution point configured on this tenant, so a package upload can never converge; configure Jamf Cloud (JCDS) in Settings → Server infrastructure → Cloud distribution point")
	case !strings.EqualFold(cdp.CdnType, pro.CloudDistributionPointCdnTypeJamfCloud):
		t.Skipf("skipping: this tenant's cloud distribution point is %s, and these tests upload to Jamf Cloud (JCDS)", cdp.CdnType)
	}

	capability, err := client.GetCloudDistributionPointUploadCapabilityV1(ctx)
	if err != nil {
		t.Fatalf("reading cloud distribution point upload capability: %v", err)
	}
	if !capability.DirectUploadCapable {
		t.Skip("skipping: this tenant's cloud distribution point does not accept direct uploads")
	}
}
