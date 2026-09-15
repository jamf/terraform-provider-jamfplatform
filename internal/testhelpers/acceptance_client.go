// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"errors"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// runSuffix computes a unique suffix (epoch timestamp) once for the entire test run.
var runSuffix = sync.OnceValue(func() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
})

// RunSuffix returns a unique suffix for the current test run, generated once
// from the epoch timestamp at the time of first call. All acceptance test
// resource names should include this suffix to avoid cross-run collisions.
func RunSuffix() string {
	return runSuffix()
}

// errAccCredsUnset marks the one client-construction failure that may legitimately
// skip a test: no credentials were configured at all.
//
// Every other failure means credentials WERE supplied and something rejected
// them — a bad secret, an expired integration, an edge or WAF block — and that
// must fail. The two used to be one branch, so a WAF block read as "no
// credentials configured" and every affected test self-skipped while the run
// reported success. That is the 2026-08-04 shape exactly, and it is why the
// sentinel exists rather than a string match on the message.
var errAccCredsUnset = errors.New("no acceptance credentials configured")

// initAcceptanceClient creates and validates the singleton acceptance client once.
var initAcceptanceClient = sync.OnceValues(func() (*jamfplatform.Client, error) {
	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")

	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("%w: set JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID and JAMFPLATFORM_CLIENT_SECRET", errAccCredsUnset)
	}

	environmentID := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID")
	tenantID := os.Getenv("JAMFPLATFORM_TENANT_ID")
	if environmentID != "" && tenantID != "" {
		return nil, fmt.Errorf("JAMFPLATFORM_ENVIRONMENT_ID and JAMFPLATFORM_TENANT_ID are both set; unset one so this client is scoped the same way as the provider under test")
	}

	var opts []jamfplatform.Option
	switch {
	case environmentID != "":
		opts = append(opts, jamfplatform.WithEnvironmentID(environmentID))
	case tenantID != "":
		opts = append(opts, jamfplatform.WithTenantID(tenantID))
	}

	c := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	return c, nil
})

// NewAcceptanceClient returns a live Jamf Platform API client for acceptance testing.
// It reads credentials from JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID, and
// JAMFPLATFORM_CLIENT_SECRET. The client is created once and reused across all
// tests in a run.
//
// Unset credentials skip — or fail, when JAMFPLATFORM_ACC_REQUIRE names the
// lane's credential set. Credentials that were supplied and REFUSED always fail,
// and hand back the report Jamf Support asks for: the timestamp, the instance
// URL and the runner's egress IP, which is gone with the runner and cannot be
// recovered afterwards. Before this split, both outcomes were one t.Skipf, so an
// edge block silently emptied the suite and the run went green.
func NewAcceptanceClient(t *testing.T) *jamfplatform.Client {
	t.Helper()

	c, err := initAcceptanceClient()
	switch {
	case errors.Is(err, errAccCredsUnset):
		SkipOrFailUnset(t, "AccPreCheck", err.Error())
	case err != nil:
		t.Fatal(CredentialRejectedMessage(os.Getenv("JAMFPLATFORM_BASE_URL"), err))
	}

	return c
}

// smartGroupFixtureOnce ensures the fixture smart group is created only once.
var smartGroupFixtureOnce sync.Once

// smartGroupID holds the ID of the fixture smart group.
var smartGroupID string

// smartGroupErr captures any error during fixture creation.
var smartGroupErr error

// smartGroupFixtureName returns the name used for the shared smart group fixture,
// incorporating the run-wide unique suffix.
func smartGroupFixtureName() string {
	return "tf-provider-test-fixture-" + RunSuffix()
}

// RequireSmartGroupFixture returns the ID of a smart device group for acceptance tests
// that need a live smart group to target, such as benchmarks and blueprints. If the group
// already exists in the tenant it is reused; otherwise a new one is created.
func RequireSmartGroupFixture(t *testing.T) string {
	t.Helper()

	c := NewAcceptanceClient(t)
	dgClient := devicegroups.New(c)

	smartGroupFixtureOnce.Do(func() {
		ctx := context.Background()

		groups, err := dgClient.ListDeviceGroups(ctx, nil, fmt.Sprintf("name==%q", smartGroupFixtureName()))
		if err != nil {
			smartGroupErr = fmt.Errorf("failed to look up fixture smart group: %w", err)
			return
		}

		for _, g := range groups {
			if g.Name == smartGroupFixtureName() {
				smartGroupID = g.ID
				return
			}
		}

		req := &devicegroups.DeviceGroupCreateRepresentationV1{
			Name:        smartGroupFixtureName(),
			Description: new("Terraform provider acceptance test fixture — safe to delete"),
			DeviceType:  devicegroups.DeviceTypeV1Computer,
			GroupType:   devicegroups.GroupTypeV1Smart,
			Criteria: &[]devicegroups.DeviceGroupCriteriaRepresentationV1{
				{
					Order:          0,
					AttributeName:  "Serial Number",
					Operator:       "LIKE",
					AttributeValue: "",
					JoinType:       devicegroups.JoinTypeV1And,
				},
			},
		}

		resp, err := dgClient.CreateDeviceGroup(ctx, req)
		if err != nil {
			smartGroupErr = fmt.Errorf("failed to create fixture smart group: %w", err)
			return
		}

		smartGroupID = resp.ID
	})

	if smartGroupErr != nil {
		t.Fatalf("Smart group fixture failed: %v", smartGroupErr)
	}

	return smartGroupID
}

// EnsureBenchmarkDeleted removes a CBEngine benchmark by title. It waits for the
// benchmark to reach a stable sync state (SYNCED or FAILED) before issuing the
// delete — deleting while still in PENDING causes the benchmark to get stuck in a
// DELETING state. After the delete is issued it polls until the benchmark disappears,
// re-issuing the delete every 20 seconds to unstick it if needed.
func EnsureBenchmarkDeleted(t *testing.T, c *jamfplatform.Client, ctx context.Context, title string) {
	t.Helper()
	cbClient := compliancebenchmarks.New(c)

	id, err := cbClient.ResolveBenchmarkIDByName(ctx, title)
	if err != nil {
		return
	}
	existing, err := cbClient.GetBenchmark(ctx, id)
	if err != nil {
		return
	}
	t.Logf("Cleaning up existing benchmark %q (ID: %s)", title, existing.BenchmarkID)

	waitForBenchmarkSyncState(t, cbClient, ctx, existing.BenchmarkID)

	if err := cbClient.DeleteBenchmark(ctx, existing.BenchmarkID); err != nil {
		t.Logf("Warning: failed to delete benchmark %q: %v", title, err)
		return
	}

	lastDelete := time.Now()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		_, lookupErr := cbClient.ResolveBenchmarkIDByName(ctx, title)
		if lookupErr != nil {
			t.Logf("Benchmark %q fully deleted", title)
			return
		}
		if time.Since(lastDelete) > 20*time.Second {
			lastDelete = time.Now()
			t.Logf("Retrying delete for stuck benchmark %q", title)
			_ = cbClient.DeleteBenchmark(ctx, existing.BenchmarkID)
		}
	}
	t.Logf("Warning: benchmark %q still exists after 2m — proceeding anyway", title)
}

// EnsureBenchmarkDeletedByID removes a CBEngine benchmark by ID. It waits for the
// benchmark to reach a stable sync state before deleting, then polls until the
// benchmark is fully removed. Re-issues the delete every 20 seconds to unstick it.
func EnsureBenchmarkDeletedByID(t *testing.T, c *jamfplatform.Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	cbClient := compliancebenchmarks.New(c)

	waitForBenchmarkSyncState(t, cbClient, ctx, benchmarkID)

	if err := cbClient.DeleteBenchmark(ctx, benchmarkID); err != nil {
		t.Logf("Warning: failed to delete benchmark %s: %v", benchmarkID, err)
		return
	}
	t.Logf("Delete issued for benchmark %s", benchmarkID)

	lastDelete := time.Now()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		if _, found := benchmarkSyncState(cbClient, ctx, benchmarkID); !found {
			t.Logf("Benchmark %s fully deleted", benchmarkID)
			return
		}
		if time.Since(lastDelete) > 20*time.Second {
			lastDelete = time.Now()
			t.Logf("Retrying delete for stuck benchmark %s", benchmarkID)
			_ = cbClient.DeleteBenchmark(ctx, benchmarkID)
		}
	}
	t.Logf("Warning: benchmark %s still present after 2m", benchmarkID)
}

// waitForBenchmarkSyncState polls until the benchmark reaches SYNCED or FAILED,
// or until a 2-minute timeout expires.
func waitForBenchmarkSyncState(t *testing.T, cbClient *compliancebenchmarks.Client, ctx context.Context, benchmarkID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		state, found := benchmarkSyncState(cbClient, ctx, benchmarkID)
		if !found {
			t.Logf("Benchmark %s not found in list, may already be deleted", benchmarkID)
			return
		}
		if state == compliancebenchmarks.BenchmarkV2SyncStateSynced || state == compliancebenchmarks.BenchmarkV2SyncStateFailed {
			t.Logf("Benchmark %s reached state %s", benchmarkID, state)
			return
		}
		t.Logf("Benchmark %s in state %q, waiting for SYNCED", benchmarkID, state)
		time.Sleep(3 * time.Second)
	}
	t.Logf("Warning: benchmark %s did not reach SYNCED after 2m — proceeding anyway", benchmarkID)
}

// benchmarkSyncState returns the sync state of a benchmark by ID from the list endpoint.
func benchmarkSyncState(cbClient *compliancebenchmarks.Client, ctx context.Context, benchmarkID string) (string, bool) {
	benchmarks, err := cbClient.ListBenchmarks(ctx)
	if err != nil {
		return "", false
	}
	for _, b := range benchmarks.Benchmarks {
		if b.ID == benchmarkID {
			return b.SyncState, true
		}
	}
	return "", false
}

// proGroupsReadableOnce caches the result of the privilege probe so we hit the
// Pro /v2/groups endpoint at most once per test run.
var proGroupsReadableOnce sync.Once

// proGroupsReadable holds the cached probe result.
var proGroupsReadable bool

// ProbeProGroupsReadable returns true if the acceptance API client can read from
// the Pro /v2/groups endpoint (i.e. has the Device groups Read permission wired up on
// the integration tenant). On 403 it returns false; on any other error
// (including network failure or the Pro endpoint not existing on the tenant)
// it also returns false — the probe is intentionally conservative so missing
// state never causes a downstream test to fail spuriously. The probe runs once
// per test run.
//
// Acceptance tests that assert jamf_pro_id is non-null on the device_group
// resource or data source should branch on this probe via SkipUnlessProGroupsReadable.
func ProbeProGroupsReadable(t *testing.T) bool {
	t.Helper()
	proGroupsReadableOnce.Do(func() {
		c, err := initAcceptanceClient()
		if err != nil {
			return
		}
		proClient := pro.New(c)
		_, err = proClient.ListGroupsV2(context.Background(), nil, "")
		if err == nil {
			proGroupsReadable = true
			return
		}
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(http.StatusForbidden) {
			t.Logf("Pro Device groups Read permission probe: 403 forbidden — jamf_pro_id assertions will be skipped")
			return
		}
		t.Logf("Pro Device groups Read permission probe: error (%v) — treating as not-readable; jamf_pro_id assertions will be skipped", err)
	})
	return proGroupsReadable
}

// SkipUnlessProGroupsReadable skips the calling test if the integration tenant's
// API client lacks the Pro Device groups Read permission (or the Pro endpoint is
// otherwise unreachable). Call this immediately after AccPreCheck in tests that
// hard-assert jamf_pro_id is set on device_group resources.
func SkipUnlessProGroupsReadable(t *testing.T) {
	t.Helper()
	if !ProbeProGroupsReadable(t) {
		t.Skip("Skipping: acceptance client lacks the Pro Device groups Read permission (or Pro endpoint unreachable); jamf_pro_id cannot be asserted")
	}
}

// CleanupSmartGroupFixture deletes the shared smart group fixture if it was created.
// Call this from TestMain after all tests in the package have completed.
func CleanupSmartGroupFixture() {
	if smartGroupID == "" {
		return
	}
	if c, err := initAcceptanceClient(); err == nil {
		dgClient := devicegroups.New(c)
		_ = dgClient.DeleteDeviceGroup(context.Background(), smartGroupID)
	}
}
