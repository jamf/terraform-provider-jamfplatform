// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package icon_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const (
	iconResourceAddress = "jamfplatform_pro_icon.test"

	// Real public URLs hosted by Jamf Concepts. The two icons differ in
	// content so switching between them exercises the URL→URL replacement
	// path. Both expose strong ETags + Last-Modified headers.
	urlJamfCLI = "https://concepts.jamf.com/icons/jamf-cli.png"
	urlGoSDK   = "https://concepts.jamf.com/icons/go-sdk.png"
)

// sourceHashRegex matches the canonical provider source_hash format.
var sourceHashRegex = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// fixturesDir returns the absolute path to the test_fixtures directory next
// to this test file.
func fixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("could not resolve caller path for fixture lookup")
	}
	return filepath.Join(filepath.Dir(file), "test_fixtures")
}

// fixturePath returns the absolute path to a file under test_fixtures.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(fixturesDir(t), name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %q not present at %q: %v", name, p, err)
	}
	return p
}

// fixtureBytes returns the bytes of a fixture file.
func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(fixturePath(t, name)) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("reading fixture %q: %v", name, err)
	}
	return b
}

// downloadToTempFile downloads url into a tempfile and returns the path.
// The tempfile is cleaned up at test end. Used to materialise URL bytes
// locally so subsequent steps can switch icon_file_source to a path with
// matching content.
func downloadToTempFile(t *testing.T, url string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("downloading %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body for %s: %v", url, err)
	}
	f, err := os.CreateTemp(t.TempDir(), "icon-*.png")
	if err != nil {
		t.Fatalf("creating tempfile: %v", err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		t.Fatalf("writing tempfile: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing tempfile: %v", err)
	}
	return f.Name()
}

// localConfig returns a config block with icon_file_source set to a path.
func localConfig(path string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_icon" "test" {
  icon_file_source = %q
}
`, path)
}

// urlConfig returns a config block with icon_file_source set to a URL.
func urlConfig(url string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_icon" "test" {
  icon_file_source = %q
}
`, url)
}

// unstableIconServer answers every request for one path with a different PNG,
// the way Apple's iTunes artwork CDN answers a fixed URL. The returned counter
// records how many times the provider fetched it, so a test can prove the
// provider read the source once rather than once per plan.
//
// The provider runs in this process, so a loopback server is reachable from it.
func unstableIconServer(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	images := [][]byte{fixtureBytes(t, "icon_a.png"), fixtureBytes(t, "icon_b.png")}

	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(images[int(n-1)%len(images)])
	}))
	t.Cleanup(server.Close)

	return server.URL + "/icon.png", &hits
}

// TestAccResource_Icon_LocalFile_BasicCRUD creates an icon from a local
// fixture and asserts the canonical source_hash format. The create plan must
// leave source_hash unknown: it is resolved in Create from the bytes uploaded,
// for every source and not only the unstable ones. The second step re-applies
// the same config and asserts an empty plan.
func TestAccResource_Icon_LocalFile_BasicCRUD(t *testing.T) {
	testhelpers.AccPreCheck(t)

	pathA := fixturePath(t, "icon_a.png")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: localConfig(pathA),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(iconResourceAddress, tfjsonpath.New("source_hash")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(iconResourceAddress, "id"),
					resource.TestCheckResourceAttrSet(iconResourceAddress, "url"),
					resource.TestMatchResourceAttr(iconResourceAddress, "source_hash", sourceHashRegex),
				),
			},
			{
				Config: localConfig(pathA),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_Icon_LocalFile_ContentChange_TriggersReplace switches
// from icon_a.png (red) to icon_b.png (green). Different bytes → provider
// must destroy-before-create and assign a new ID. The replacement plan leaves
// source_hash unknown, and the hash committed after it must differ from the one
// before, which is what proves the new bytes reached the tenant.
func TestAccResource_Icon_LocalFile_ContentChange_TriggersReplace(t *testing.T) {
	testhelpers.AccPreCheck(t)

	pathA := fixturePath(t, "icon_a.png")
	pathB := fixturePath(t, "icon_b.png")

	idCompare := statecheck.CompareValue(compare.ValuesDiffer())
	hashCompare := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: localConfig(pathA),
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
					hashCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("source_hash")),
				},
			},
			{
				Config: localConfig(pathB),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
						plancheck.ExpectUnknownValue(iconResourceAddress, tfjsonpath.New("source_hash")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
					hashCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("source_hash")),
				},
			},
		},
	})
}

// TestAccResource_Icon_LocalFile_SameContentDifferentPath_NoReplace
// switches the source path to a byte-identical copy. The provider must
// see the same hash and perform an in-place update (icon_file_source
// changes only); the ID and the hash must NOT change.
func TestAccResource_Icon_LocalFile_SameContentDifferentPath_NoReplace(t *testing.T) {
	testhelpers.AccPreCheck(t)

	pathA := fixturePath(t, "icon_a.png")
	pathACopy := fixturePath(t, "icon_a_copy.png")

	idCompare := statecheck.CompareValue(compare.ValuesSame())
	hashCompare := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: localConfig(pathA),
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
					hashCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("source_hash")),
				},
			},
			{
				Config: localConfig(pathACopy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
					hashCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("source_hash")),
				},
			},
		},
	})
}

// TestAccResource_Icon_URL_BasicCRUD creates an icon from the real
// concepts.jamf.com URL, asserts state, and re-applies for an empty plan.
// Requires network access during apply.
func TestAccResource_Icon_URL_BasicCRUD(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: urlConfig(urlJamfCLI),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(iconResourceAddress, tfjsonpath.New("source_hash")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(iconResourceAddress, "id"),
					resource.TestCheckResourceAttrSet(iconResourceAddress, "url"),
					resource.TestMatchResourceAttr(iconResourceAddress, "source_hash", sourceHashRegex),
				),
			},
			{
				Config: urlConfig(urlJamfCLI),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_Icon_URL_UnstableContent_AppliesOnTheFirstRun is the
// acceptance cover for issue #373, where five of fifteen iTunes artwork icons
// failed a first apply with "Provider produced inconsistent final plan" on
// source_hash and all five succeeded on a retry.
//
// The condition is a URL whose bytes differ between two requests, which the
// iTunes artwork CDN does and which no public URL can be relied on to do on
// demand, so the test serves it. The apply has to succeed on the first run, the
// re-apply has to plan nothing, and the provider has to have read the URL
// exactly once across both: before the fix it read it on every plan, so the
// count is the assertion that would still catch a plan-time fetch reintroduced
// somewhere the hash comparison no longer visits.
func TestAccResource_Icon_URL_UnstableContent_AppliesOnTheFirstRun(t *testing.T) {
	testhelpers.AccPreCheck(t)

	url, hits := unstableIconServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: urlConfig(url),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(iconResourceAddress, tfjsonpath.New("source_hash")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(iconResourceAddress, "id"),
					resource.TestMatchResourceAttr(iconResourceAddress, "source_hash", sourceHashRegex),
				),
			},
			{
				Config: urlConfig(url),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})

	if got := hits.Load(); got != 1 {
		t.Fatalf("the provider fetched the icon URL %d times, want 1: only the upload in Create may read it, or an unstable source plans one hash and applies another", got)
	}
}

// TestAccResource_Icon_URL_ChangeBetweenURLs_TriggersReplace creates from
// the jamf-cli URL and then switches to the go-sdk URL. A changed URL string
// → destroy-before-create. ID changes.
func TestAccResource_Icon_URL_ChangeBetweenURLs_TriggersReplace(t *testing.T) {
	testhelpers.AccPreCheck(t)

	idCompare := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: urlConfig(urlJamfCLI),
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
				},
			},
			{
				Config: urlConfig(urlGoSDK),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
				},
			},
		},
	})
}

// TestAccResource_Icon_URL_SwitchToLocalSameBytes_NoReplace downloads the
// jamf-cli URL to a tempfile, creates from URL, then switches to that
// tempfile (byte-identical to what was served). Hashes match → in-place
// update only; ID and source_hash unchanged.
//
// It also pins that the hash Create recorded is the hash of what it uploaded:
// the local file holds the bytes the URL served, so a Create that recorded
// anything else would make this step a replacement.
func TestAccResource_Icon_URL_SwitchToLocalSameBytes_NoReplace(t *testing.T) {
	testhelpers.AccPreCheck(t)

	localCopyOfURLBytes := downloadToTempFile(t, urlJamfCLI)

	idCompare := statecheck.CompareValue(compare.ValuesSame())
	hashCompare := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: urlConfig(urlJamfCLI),
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
					hashCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("source_hash")),
				},
			},
			{
				Config: localConfig(localCopyOfURLBytes),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
					hashCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("source_hash")),
				},
			},
		},
	})
}

// TestAccResource_Icon_URL_SwitchToLocalDifferentBytes_TriggersReplace
// creates from the jamf-cli URL then switches to icon_b.png on disk.
// Hashes differ → destroy-before-create. ID changes.
func TestAccResource_Icon_URL_SwitchToLocalDifferentBytes_TriggersReplace(t *testing.T) {
	testhelpers.AccPreCheck(t)

	pathB := fixturePath(t, "icon_b.png")

	idCompare := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: urlConfig(urlJamfCLI),
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
				},
			},
			{
				Config: localConfig(pathB),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idCompare.AddStateValue(iconResourceAddress, tfjsonpath.New("id")),
				},
			},
		},
	})
}

// TestAccResource_Icon_Import_NoReplace_AfterMatchingLocalConfig pre-creates
// an icon via the SDK, imports it by ID, then applies a config whose
// icon_file_source points at the SERVER-SERVED bytes (downloaded from the
// icon's CDN URL after the import step uploads it). Expected outcome:
// in-place update (icon_file_source goes null → local path) with NO
// replacement, because ModifyPlan hashes the local file and finds what import
// stored: both derive from the SAME server-served bytes.
//
// Both steps assert source_hash against the hash of the downloaded
// server-served bytes, which the test computes itself. That pins the hash to a
// concrete value rather than only to the two steps agreeing with each other, so
// an Update that recomputed and overwrote the imported hash fails here even
// though the plan is still an in-place update.
//
// Why this matters: Jamf Pro re-encodes uploaded PNGs server-side
// (different zlib compression and/or metadata). The bytes served back
// from /icon/{id} or the CDN URL are NOT byte-identical to the original
// upload. Therefore the documented import workflow is:
//
//  1. terraform import jamfplatform_pro_icon.x <id>
//  2. Download the icon from state.url (NOT your original upload file)
//  3. Set icon_file_source to that downloaded copy
//
// This test exercises that contract.
func TestAccResource_Icon_Import_NoReplace_AfterMatchingLocalConfig(t *testing.T) {
	testhelpers.AccPreCheck(t)

	bytesA := fixtureBytes(t, "icon_a.png")

	apiClient := testhelpers.NewAcceptanceClient(t)
	proClient := pro.New(apiClient)

	uploadResp, err := proClient.UploadIconV1(context.Background(), "icon_a.png", bytes.NewReader(bytesA))
	if err != nil {
		t.Fatalf("pre-creating icon via SDK: %v", err)
	}
	preCreatedID := strconv.Itoa(uploadResp.ID)
	t.Logf("pre-created icon id=%s url=%s", preCreatedID, uploadResp.URL)

	// Download the SERVER-SERVED bytes to a tempfile. This emulates the
	// user-facing workflow ("curl -o ./icon.png $url") and ensures the
	// local file we point at in step 2 has the exact bytes Jamf stored.
	serverBytesPath := downloadToTempFile(t, uploadResp.URL)

	serverBytes, err := os.ReadFile(serverBytesPath) //nolint:gosec // test tempfile
	if err != nil {
		t.Fatalf("reading the downloaded icon %q: %v", serverBytesPath, err)
	}
	serverBytesHash := files.ComputeContentSHA256(serverBytes)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: import the pre-existing icon. Read's import path
			// downloads the icon bytes via state.url and stores canonical
			// "sha256:<hex>" source_hash. ImportStatePersist=true so the
			// next step inherits this state. ImportStateVerify=false because
			// icon_file_source is null immediately after import and is only
			// populated by step 2.
			{
				Config:             localConfig(serverBytesPath),
				ResourceName:       iconResourceAddress,
				ImportState:        true,
				ImportStateId:      preCreatedID,
				ImportStatePersist: true,
				ImportStateVerify:  false,
				// An import step never reads Check or ConfigStateChecks, so what
				// import committed is asserted through ImportStateCheck. The hash
				// is pinned to the downloaded server-served bytes rather than only
				// to the canonical format, since the test has those bytes.
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					s := states[0]
					if s.Attributes["id"] != preCreatedID {
						return fmt.Errorf("imported id = %q, want %q", s.Attributes["id"], preCreatedID)
					}
					if got := s.Attributes["source_hash"]; got != serverBytesHash {
						return fmt.Errorf("imported source_hash = %q, want the server-served bytes' %q", got, serverBytesHash)
					}
					return nil
				},
			},
			// Step 2: apply config with icon_file_source pointing at the
			// server-served bytes (the user's downloaded copy). ModifyPlan
			// hashes the local file and finds the import-path hash, so the
			// plan is an in-place update rather than a replacement and the
			// hash stays known. ID must remain preCreatedID.
			{
				Config: localConfig(serverBytesPath),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(iconResourceAddress, tfjsonpath.New("source_hash"), knownvalue.StringRegexp(sourceHashRegex)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(iconResourceAddress, "id", preCreatedID),
					resource.TestCheckResourceAttr(iconResourceAddress, "source_hash", serverBytesHash),
				),
			},
		},
	})
}

// TestAccResource_Icon_Import_Replace_WhenLocalIsOriginalUpload demonstrates
// the OTHER side of the server-side-transform reality: if a user imports an
// icon and then points icon_file_source at the ORIGINAL upload file (the
// bytes they had on disk before Jamf re-encoded them), the local hash and
// the import-path hash differ, so the plan correctly shows a REPLACEMENT.
//
// This is expected and correct behaviour — the test exists to document the
// failure mode so a future contributor doesn't try to "fix" it.
func TestAccResource_Icon_Import_Replace_WhenLocalIsOriginalUpload(t *testing.T) {
	testhelpers.AccPreCheck(t)

	pathA := fixturePath(t, "icon_a.png")
	bytesA := fixtureBytes(t, "icon_a.png")

	apiClient := testhelpers.NewAcceptanceClient(t)
	proClient := pro.New(apiClient)

	uploadResp, err := proClient.UploadIconV1(context.Background(), "icon_a.png", bytes.NewReader(bytesA))
	if err != nil {
		t.Fatalf("pre-creating icon via SDK: %v", err)
	}
	preCreatedID := strconv.Itoa(uploadResp.ID)
	t.Logf("pre-created icon id=%s url=%s (Jamf re-encodes; URL bytes != local bytes)", preCreatedID, uploadResp.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             localConfig(pathA),
				ResourceName:       iconResourceAddress,
				ImportState:        true,
				ImportStateId:      preCreatedID,
				ImportStatePersist: true,
				ImportStateVerify:  false,
				// An import step never reads Check, so these run as an
				// ImportStateCheck. The hash is matched against the canonical
				// format only: this test deliberately never downloads the
				// server-served bytes, because pointing at the original upload
				// instead is the whole point of the replacement it asserts.
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					s := states[0]
					if s.Attributes["id"] != preCreatedID {
						return fmt.Errorf("imported id = %q, want %q", s.Attributes["id"], preCreatedID)
					}
					if got := s.Attributes["source_hash"]; !sourceHashRegex.MatchString(got) {
						return fmt.Errorf("imported source_hash = %q, want the canonical %s format", got, sourceHashRegex)
					}
					return nil
				},
			},
			{
				Config: localConfig(pathA),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(iconResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}
