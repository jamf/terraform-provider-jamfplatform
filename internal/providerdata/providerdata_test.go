// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// atFloorVersion is the tenant version the fake fetchers report: exactly the
// provider floor, so the floor advisory does NOT fire and a test asserting zero
// warnings is asserting something real.
//
// Bound to the constant rather than written out, because these fixtures were
// previously hardcoded to the floor's then-current value and every bump silently
// pushed the fake tenant BELOW the floor — turning "no warnings at floor" into a
// failing test that looked like a regression in the code under test rather than a
// stale fixture. Deriving it means the next bump needs no edit here.
const atFloorVersion = ProviderMinJamfProVersion

// newFakeClient builds a non-network jamfplatform.Client suitable for tests that
// exercise ConfigurePro. The version fetcher is overridden so no HTTP call is made;
// the client itself is only needed because ConfigurePro constructs pro.New(pd.Client)
// to return to callers.
func newFakeClient() *jamfplatform.Client {
	return jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret")
}

// countSeverity returns the number of diagnostics at the given severity.
func countSeverity(d diag.Diagnostics, sev diag.Severity) int {
	n := 0
	for _, e := range d {
		if e.Severity() == sev {
			n++
		}
	}
	return n
}

func TestConfigurePro_NilProviderData(t *testing.T) {
	client, diags := ConfigurePro(context.Background(), nil, "", "jamfplatform_pro_test")
	if client != nil {
		t.Errorf("expected nil client for nil providerData, got %v", client)
	}
	if diags.HasError() {
		t.Errorf("expected no diagnostics for nil providerData, got %v", diags)
	}
}

func TestConfigurePro_WrongType(t *testing.T) {
	client, diags := ConfigurePro(context.Background(), "not a Data value", "", "jamfplatform_pro_test")
	if client != nil {
		t.Errorf("expected nil client for wrong providerData type, got %v", client)
	}
	if !diags.HasError() {
		t.Fatalf("expected error diagnostic for wrong providerData type, got %v", diags)
	}
	if !strings.Contains(diags[0].Summary(), "Unexpected Configure Type") {
		t.Errorf("expected 'Unexpected Configure Type' summary, got %q", diags[0].Summary())
	}
}

func TestConfigurePro_HappyPath_NoMinVer(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return atFloorVersion, nil
		},
	}
	client, diags := ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_test")
	if client == nil {
		t.Fatal("expected non-nil client on happy path")
	}
	if diags.HasError() {
		t.Errorf("expected no errors, got %v", diags)
	}
	if countSeverity(diags, diag.SeverityWarning) != 0 {
		t.Errorf("expected no warnings at floor, got %v", diags)
	}
}

func TestConfigurePro_MinVerGate_Satisfied(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return atFloorVersion, nil
		},
	}
	_, diags := ConfigurePro(context.Background(), pd, "11.5.0", "jamfplatform_pro_test")
	if diags.HasError() {
		t.Errorf("expected no error when tenant >= minVer, got %v", diags)
	}
}

func TestConfigurePro_MinVerGate_Failed(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return "10.0.0", nil
		},
	}
	_, diags := ConfigurePro(context.Background(), pd, "11.5.0", "jamfplatform_pro_test")
	if !diags.HasError() {
		t.Fatalf("expected error when tenant < minVer, got %v", diags)
	}
}

// TestConfigurePro_FloorWarning_EmittedOnce verifies the provider-floor advisory
// warning fires at most once per Data value even with many Pro Configure calls.
func TestConfigurePro_FloorWarning_EmittedOnce(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return "10.0.0", nil
		},
	}
	_, diags1 := ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_test_a")
	if got := countSeverity(diags1, diag.SeverityWarning); got != 1 {
		t.Fatalf("first Configure: expected 1 warning, got %d (%v)", got, diags1)
	}

	for i := range 4 {
		_, diagsN := ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_test_more")
		if got := countSeverity(diagsN, diag.SeverityWarning); got != 0 {
			t.Errorf("subsequent Configure #%d: expected 0 warnings, got %d (%v)", i+2, got, diagsN)
		}
	}
}

// TestConfigurePro_FetchError_NotCached verifies a transient fetch error in the
// first Configure does not poison subsequent Configures — they retry the fetch.
func TestConfigurePro_FetchError_NotCached(t *testing.T) {
	calls := 0
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("transient network error")
			}
			return atFloorVersion, nil
		},
	}
	// First call: empty minVer → fetch error is swallowed, client returned.
	client, diags := ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_test_a")
	if client == nil {
		t.Fatal("expected client even when fetch errors with empty minVer")
	}
	if diags.HasError() {
		t.Errorf("expected no error diagnostics with empty minVer + fetch failure, got %v", diags)
	}

	// Second call: non-empty minVer → must retry the fetch and succeed.
	_, diags2 := ConfigurePro(context.Background(), pd, "11.0.0", "jamfplatform_pro_test_b")
	if diags2.HasError() {
		t.Errorf("expected retry to succeed on second Configure, got %v", diags2)
	}
	if calls < 2 {
		t.Errorf("expected versionFetcher to be retried, only called %d times", calls)
	}
}

// TestConfigurePro_FetchError_HardWithMinVer verifies a fetch failure surfaces as
// a Configure-time error when the resource declares a non-empty minVer.
func TestConfigurePro_FetchError_HardWithMinVer(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return "", errors.New("503 service unavailable")
		},
	}
	client, diags := ConfigurePro(context.Background(), pd, "11.5.0", "jamfplatform_pro_test")
	if client != nil {
		t.Errorf("expected nil client when fetch fails with non-empty minVer, got %v", client)
	}
	if !diags.HasError() {
		t.Fatalf("expected error diagnostic, got %v", diags)
	}
	if !strings.Contains(diags[0].Summary(), "Failed to read Jamf Pro tenant version") {
		t.Errorf("expected 'Failed to read Jamf Pro tenant version' summary, got %q", diags[0].Summary())
	}
}

// TestConfigurePro_ShortCircuit_EmptyMinVerAfterFloorHandled verifies that once the
// provider-floor advisory has been considered for a Data value, subsequent Configure
// calls with empty minVer skip the version fetch entirely. This keeps acceptance-test
// noise low when many empty-minVer Pro resources reuse the same Data.
func TestConfigurePro_ShortCircuit_EmptyMinVerAfterFloorHandled(t *testing.T) {
	calls := 0
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			calls++
			return atFloorVersion, nil
		},
	}
	// First Configure: fetches and considers floor (no warning at floor).
	_, diags := ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_test_a")
	if diags.HasError() {
		t.Fatalf("unexpected error on first Configure: %v", diags)
	}
	if calls != 1 {
		t.Fatalf("first Configure: expected 1 fetch, got %d", calls)
	}

	// Subsequent empty-minVer Configures must not fetch.
	for range 5 {
		_, _ = ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_test_more")
	}
	if calls != 1 {
		t.Errorf("expected 1 total fetch after 6 empty-minVer Configures, got %d", calls)
	}

	// A Configure with non-empty minVer must still consult the cached version (no new
	// fetch — cached value is reused, gate evaluated against it).
	_, diags = ConfigurePro(context.Background(), pd, "11.0.0", "jamfplatform_pro_test_gated")
	if diags.HasError() {
		t.Errorf("non-empty minVer Configure unexpectedly errored: %v", diags)
	}
	if calls != 1 {
		t.Errorf("expected fetch count unchanged after non-empty minVer Configure (cache hit), got %d", calls)
	}
}

// ProClassic Configure tests are intentionally narrower than the Pro suite:
// ConfigureProClassic and ConfigurePro share the configureSub core, so the
// version-cache, fetch-retry, fast-path-short-circuit, and gate-satisfied paths
// are already exercised through the Pro tests above. The tests below cover the
// proclassic-specific wiring (factory choice, returned type) and the
// cross-client invariants we need to keep (shared one-shot floor warning).

func TestConfigureProClassic_NilProviderData(t *testing.T) {
	client, diags := ConfigureProClassic(context.Background(), nil, "", "jamfplatform_pro_test")
	if client != nil {
		t.Errorf("expected nil client for nil providerData, got %v", client)
	}
	if diags.HasError() {
		t.Errorf("expected no diagnostics for nil providerData, got %v", diags)
	}
}

func TestConfigureProClassic_WrongType(t *testing.T) {
	client, diags := ConfigureProClassic(context.Background(), "not a Data value", "", "jamfplatform_pro_test")
	if client != nil {
		t.Errorf("expected nil client for wrong providerData type, got %v", client)
	}
	if !diags.HasError() {
		t.Fatalf("expected error diagnostic for wrong providerData type, got %v", diags)
	}
	if !strings.Contains(diags[0].Summary(), "Unexpected Configure Type") {
		t.Errorf("expected 'Unexpected Configure Type' summary, got %q", diags[0].Summary())
	}
}

func TestConfigureProClassic_HappyPath_NoMinVer(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return atFloorVersion, nil
		},
	}
	client, diags := ConfigureProClassic(context.Background(), pd, "", "jamfplatform_pro_test")
	if client == nil {
		t.Fatal("expected non-nil proclassic client on happy path")
	}
	if diags.HasError() {
		t.Errorf("expected no errors, got %v", diags)
	}
}

func TestConfigureProClassic_MinVerGate_Failed(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return "10.0.0", nil
		},
	}
	_, diags := ConfigureProClassic(context.Background(), pd, "11.5.0", "jamfplatform_pro_test")
	if !diags.HasError() {
		t.Fatalf("expected error when tenant < minVer, got %v", diags)
	}
}

// TestConfigureProClassic_SharesFloorStateWithPro verifies that ConfigurePro and
// ConfigureProClassic share the one-shot floor-warning machinery on the same
// Data value. A config that mixes both kinds of resources must only see one
// warning regardless of order.
func TestConfigureProClassic_SharesFloorStateWithPro(t *testing.T) {
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			return "10.0.0", nil
		},
	}
	_, diags1 := ConfigurePro(context.Background(), pd, "", "jamfplatform_pro_a")
	if got := countSeverity(diags1, diag.SeverityWarning); got != 1 {
		t.Fatalf("first (Pro) Configure: expected 1 warning, got %d (%v)", got, diags1)
	}
	_, diags2 := ConfigureProClassic(context.Background(), pd, "", "jamfplatform_pro_b")
	if got := countSeverity(diags2, diag.SeverityWarning); got != 0 {
		t.Errorf("subsequent (ProClassic) Configure: expected 0 warnings, got %d (%v)", got, diags2)
	}
}

// TestFiredOnce_NamespacedLatches verifies that distinct keys each fire exactly
// once and that repeat keys are suppressed thereafter. Used by cross-API bridging
// paths to avoid duplicate warnings per provider invocation.
func TestFiredOnce_NamespacedLatches(t *testing.T) {
	pd := &Data{}
	if !pd.FiredOnce("foo") {
		t.Error("first call for key 'foo' should return true")
	}
	if pd.FiredOnce("foo") {
		t.Error("second call for key 'foo' should return false")
	}
	if !pd.FiredOnce("bar") {
		t.Error("first call for distinct key 'bar' should return true")
	}
	if pd.FiredOnce("bar") {
		t.Error("second call for key 'bar' should return false")
	}
	if pd.FiredOnce("foo") {
		t.Error("third call for key 'foo' should still return false")
	}
}

// TestGetJamfProVersion_CachesSuccess verifies successful fetches are not re-issued.
func TestGetJamfProVersion_CachesSuccess(t *testing.T) {
	calls := 0
	pd := &Data{Client: newFakeClient(), scope: ScopeTenant,
		versionFetcher: func(_ context.Context) (string, error) {
			calls++
			return atFloorVersion, nil
		},
	}
	for i := range 3 {
		v, err := pd.GetJamfProVersion(context.Background())
		if err != nil {
			t.Fatalf("call %d: unexpected error %v", i, err)
		}
		if v != atFloorVersion {
			t.Errorf("call %d: got %q, want %q", i, v, atFloorVersion)
		}
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 fetch for cached success, got %d", calls)
	}
}

// TestEnrollmentWriteLock_StableIdentity verifies the shared enrollment write lock
// is the same *sync.Mutex instance on every call for a given Data value, so the
// user-initiated-enrollment-settings (/v4) and re-enrollment-settings (/v1) resources
// — which receive the same *Data by pointer at Configure — serialize against one lock.
func TestEnrollmentWriteLock_StableIdentity(t *testing.T) {
	pd := New(newFakeClient())
	a := pd.EnrollmentWriteLock()
	b := pd.EnrollmentWriteLock()
	if a == nil {
		t.Fatal("EnrollmentWriteLock returned nil")
	}
	if a != b {
		t.Errorf("expected the same mutex instance across calls, got %p and %p", a, b)
	}
	// Sanity: the returned lock is usable and starts unlocked.
	if !a.TryLock() {
		t.Fatal("fresh enrollment write lock should be acquirable")
	}
	a.Unlock()

	// Distinct Data values must not share a lock.
	if other := New(newFakeClient()).EnrollmentWriteLock(); other == a {
		t.Error("distinct Data values unexpectedly share one enrollment write lock")
	}
}

// The patch source cache is the second provider-instance cache in this file (the
// Jamf Pro version being the first) and follows the same two rules: a success is
// read once and shared, an error is not cached. Both catalogue reads are
// injected, so none of these tests needs a client or a mock server.

// TestPatchSourceCache_ReadsOnce verifies the snapshot is read once per cache
// and shared. Without it a configuration with N patch software title data
// sources pays 2N identical catalogue requests on every plan and again on every
// apply.
func TestPatchSourceCache_ReadsOnce(t *testing.T) {
	name := "Jamf"
	id := 1
	calls := 0
	cache := &PatchSourceCache{
		read: func(_ context.Context, _ *proclassic.Client) (PatchSourceCatalogues, error) {
			calls++
			return PatchSourceCatalogues{Internal: []proclassic.IDName{{ID: &id, Name: &name}}}, nil
		},
	}

	for i := range 3 {
		got, err := cache.Catalogues(context.Background())
		if err != nil {
			t.Fatalf("call %d: unexpected error %v", i, err)
		}
		if len(got.Internal) != 1 || got.Internal[0].Name == nil || *got.Internal[0].Name != "Jamf" {
			t.Fatalf("call %d: expected the cached snapshot, got %+v", i, got)
		}
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 catalogue read for a cached success, got %d", calls)
	}
}

// TestPatchSourceCache_ErrorIsNotCached verifies a failed read is retried by the
// next caller. A transient blip or a momentary privilege problem on the first
// title must not blank every later title's source_id for the rest of the run.
func TestPatchSourceCache_ErrorIsNotCached(t *testing.T) {
	name := "Jamf"
	id := 1
	calls := 0
	cache := &PatchSourceCache{
		read: func(_ context.Context, _ *proclassic.Client) (PatchSourceCatalogues, error) {
			calls++
			if calls == 1 {
				return PatchSourceCatalogues{}, errors.New("transient 503")
			}
			return PatchSourceCatalogues{External: []proclassic.IDName{{ID: &id, Name: &name}}}, nil
		},
	}

	if _, err := cache.Catalogues(context.Background()); err == nil {
		t.Fatal("expected the first read's error to surface")
	}

	got, err := cache.Catalogues(context.Background())
	if err != nil {
		t.Fatalf("expected the second read to retry and succeed, got %v", err)
	}
	if len(got.External) != 1 {
		t.Errorf("expected the retried snapshot, got %+v", got)
	}
	if calls != 2 {
		t.Errorf("expected the failed read to be retried (2 reads), got %d", calls)
	}
}

// TestPatchSourceCache_NilReceiver verifies a construct that never received a
// cache reports an unresolved id rather than panicking mid-plan. Resolving a
// patch source name is best-effort everywhere except import.
func TestPatchSourceCache_NilReceiver(t *testing.T) {
	got, err := (*PatchSourceCache)(nil).Catalogues(context.Background())
	if err == nil {
		t.Fatal("expected an error from a nil cache")
	}
	if len(got.Internal) != 0 || len(got.External) != 0 {
		t.Errorf("expected an empty snapshot from a nil cache, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected the error to say the provider is not configured, got %q", err)
	}
}

// TestConfigurePatchSources_NilProviderData verifies the nil ProviderData the
// framework passes during early lifecycle yields a nil cache rather than a
// diagnostic — the same contract ConfigureImpact keeps.
func TestConfigurePatchSources_NilProviderData(t *testing.T) {
	noRead := func(context.Context, *proclassic.Client) (PatchSourceCatalogues, error) {
		t.Fatal("the read must not be invoked while configuring")
		return PatchSourceCatalogues{}, nil
	}
	if got := ConfigurePatchSources(nil, noRead); got != nil {
		t.Errorf("expected a nil cache for nil providerData, got %v", got)
	}
	if got := ConfigurePatchSources("not a Data value", noRead); got != nil {
		t.Errorf("expected a nil cache for a wrong providerData type, got %v", got)
	}
}

// TestConfigurePatchSources_StableIdentity verifies every construct configured
// from one provider instance shares one cache — the whole point of hanging it
// off Data — and that distinct Data values do not share one. The read is also
// asserted to receive a client, since Data builds the classic client itself
// rather than taking one from the caller.
func TestConfigurePatchSources_StableIdentity(t *testing.T) {
	var gotClient *proclassic.Client
	read := func(_ context.Context, c *proclassic.Client) (PatchSourceCatalogues, error) {
		gotClient = c
		return PatchSourceCatalogues{}, nil
	}

	pd := New(newFakeClient())
	a := ConfigurePatchSources(pd, read)
	b := ConfigurePatchSources(pd, read)
	if a == nil {
		t.Fatal("expected a cache for a *Data providerData")
	}
	if a != b {
		t.Errorf("expected one shared cache per provider instance, got %p and %p", a, b)
	}
	if other := ConfigurePatchSources(New(newFakeClient()), read); other == a {
		t.Error("distinct Data values unexpectedly share one patch source cache")
	}

	if _, err := a.Catalogues(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotClient == nil {
		t.Error("the cache must hand its own classic client to the read")
	}
}

// The App Catalog title cache is the third provider-instance cache in this file
// and keeps the same two rules as the patch source cache: a success is read once
// and shared, an error is not cached. The catalog read is injected, so none of
// these tests needs a client or a mock server.

// TestAppTitleCatalogCache_ReadsOnce verifies the snapshot is read once per cache
// and shared. Without it every App Installer in a configuration resolved its own
// title name and reverse-resolved its own title id, so a 50-resource workspace
// paid 100 catalog requests on top of its 50 deployment reads on every plan.
func TestAppTitleCatalogCache_ReadsOnce(t *testing.T) {
	calls := 0
	cache := &AppTitleCatalogCache{
		read: func(_ context.Context, _ *pro.Client) ([]pro.AppTitle, error) {
			calls++
			return []pro.AppTitle{{ID: "Composer", TitleName: "Jamf Composer"}}, nil
		},
	}

	for i := range 3 {
		got, err := cache.Titles(context.Background())
		if err != nil {
			t.Fatalf("call %d: unexpected error %v", i, err)
		}
		if len(got) != 1 || got[0].TitleName != "Jamf Composer" {
			t.Fatalf("call %d: expected the cached snapshot, got %+v", i, got)
		}
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 catalog read for a cached success, got %d", calls)
	}
}

// TestAppTitleCatalogCache_ErrorIsNotCached verifies a failed read is retried by
// the next caller. A transient blip or a momentary privilege problem on the first
// deployment must not blank every later deployment's app_title_name for the rest
// of the run.
func TestAppTitleCatalogCache_ErrorIsNotCached(t *testing.T) {
	calls := 0
	cache := &AppTitleCatalogCache{
		read: func(_ context.Context, _ *pro.Client) ([]pro.AppTitle, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("transient 503")
			}
			return []pro.AppTitle{{ID: "7F0", TitleName: "Jamf Sync"}}, nil
		},
	}

	if _, err := cache.Titles(context.Background()); err == nil {
		t.Fatal("expected the first read's error to surface")
	}

	got, err := cache.Titles(context.Background())
	if err != nil {
		t.Fatalf("expected the second read to retry and succeed, got %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected the retried snapshot, got %+v", got)
	}
	if calls != 2 {
		t.Errorf("expected the failed read to be retried (2 reads), got %d", calls)
	}
}

// TestAppTitleCatalogCache_NilReceiver verifies a construct that never received a
// cache reports the failure rather than panicking mid-plan. Resolving a title name
// is a plan-time preflight and a best-effort refresh everywhere except import.
func TestAppTitleCatalogCache_NilReceiver(t *testing.T) {
	got, err := (*AppTitleCatalogCache)(nil).Titles(context.Background())
	if err == nil {
		t.Fatal("expected an error from a nil cache")
	}
	if len(got) != 0 {
		t.Errorf("expected an empty snapshot from a nil cache, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected the error to say the provider is not configured, got %q", err)
	}
}

// TestConfigureAppTitleCatalog_NilProviderData verifies the nil ProviderData the
// framework passes during early lifecycle yields a nil cache rather than a
// diagnostic — the same contract ConfigurePatchSources keeps.
func TestConfigureAppTitleCatalog_NilProviderData(t *testing.T) {
	noRead := func(context.Context, *pro.Client) ([]pro.AppTitle, error) {
		t.Fatal("the read must not be invoked while configuring")
		return nil, nil
	}
	if got := ConfigureAppTitleCatalog(nil, noRead); got != nil {
		t.Errorf("expected a nil cache for nil providerData, got %v", got)
	}
	if got := ConfigureAppTitleCatalog("not a Data value", noRead); got != nil {
		t.Errorf("expected a nil cache for a wrong providerData type, got %v", got)
	}
}

// TestConfigureAppTitleCatalog_StableIdentity verifies every construct configured
// from one provider instance shares one cache — the whole point of hanging it off
// Data — and that distinct Data values do not share one. The read is also asserted
// to receive a client, since Data builds the Pro client itself rather than taking
// one from the caller.
func TestConfigureAppTitleCatalog_StableIdentity(t *testing.T) {
	var gotClient *pro.Client
	read := func(_ context.Context, c *pro.Client) ([]pro.AppTitle, error) {
		gotClient = c
		return nil, nil
	}

	pd := New(newFakeClient())
	a := ConfigureAppTitleCatalog(pd, read)
	b := ConfigureAppTitleCatalog(pd, read)
	if a == nil {
		t.Fatal("expected a cache for a *Data providerData")
	}
	if a != b {
		t.Errorf("expected one shared cache per provider instance, got %p and %p", a, b)
	}
	if other := ConfigureAppTitleCatalog(New(newFakeClient()), read); other == a {
		t.Error("distinct Data values unexpectedly share one App Catalog title cache")
	}

	if _, err := a.Titles(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotClient == nil {
		t.Error("the cache must hand its own Pro client to the read")
	}
}
