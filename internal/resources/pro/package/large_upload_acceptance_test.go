// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// The large-file scenarios move 300 MB and 2 GB through a cloud distribution
// point, so they are opt-in. Set JAMFPLATFORM_ACC_PRO_PACKAGE_LARGE_UPLOAD=1 to
// run them; the default lane never uploads that much. The tenant must have a
// cloud distribution point. Set JAMFPLATFORM_ACC_PRO_PACKAGE_LARGE_UPLOAD_MIB to change the large size from
// 2048 MiB, for a link too slow to push 2 GB through the gateway before it
// answers 502. Whether Jamf Pro hashes a 2 GB package depends on the
// distribution point, so the assertions hold for both outcomes: a verified
// package carries a hash and an unverified one carries none.

package pkg_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const largeUploadEnvVar = "JAMFPLATFORM_ACC_PRO_PACKAGE_LARGE_UPLOAD"

const largeUploadMiBEnvVar = "JAMFPLATFORM_ACC_PRO_PACKAGE_LARGE_UPLOAD_MIB"

const smallUploadBytes = 300 << 20

func largeUploadBytes() int64 {
	if v, err := strconv.ParseInt(os.Getenv(largeUploadMiBEnvVar), 10, 64); err == nil && v > 0 {
		return v << 20
	}
	return 2 << 30
}

func requireLargeUploads(t *testing.T) {
	t.Helper()
	testhelpers.AccPreCheck(t)
	if os.Getenv(largeUploadEnvVar) != "1" {
		t.Skipf("%s is not 1; skipping the 300 MB and 2 GB upload scenarios", largeUploadEnvVar)
	}
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	cdp, err := c.GetCloudDistributionPointV1(context.Background())
	if err != nil || cdp == nil || cdp.CdnType == pro.CloudDistributionPointCdnTypeNone {
		t.Skip("the tenant has no cloud distribution point, so a package binary cannot be uploaded")
	}
}

func writeRandomFile(t *testing.T, path string, size int64, seed byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var key [32]byte
	key[0] = seed
	rng := rand.NewChaCha8(key)
	buf := make([]byte, 1<<20)
	for written := int64(0); written < size; {
		_, _ = rng.Read(buf)
		n := min(size-written, int64(len(buf)))
		if _, err := f.Write(buf[:n]); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		written += n
	}
}

func logStep(t *testing.T, name string, started *time.Time) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[packageResourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", packageResourceAddr)
		}
		t.Logf("SCENARIO %-44s apply %-10s size=%s sha3=%q sha256=%q md5=%q",
			name, time.Since(*started).Round(time.Second),
			rs.Primary.Attributes["size"], rs.Primary.Attributes["sha3_512"], rs.Primary.Attributes["sha256"], rs.Primary.Attributes["md5"])
		return nil
	}
}

func checkRecordMatches(t *testing.T, wantSize int64) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[packageResourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", packageResourceAddr)
		}
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		p, err := c.GetPackageV1(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("fetching package %s: %w", rs.Primary.ID, err)
		}
		if got := helpers.DerefString(p.Size); got != strconv.FormatInt(wantSize, 10) {
			return fmt.Errorf("size on the server is %q, want %d", got, wantSize)
		}
		serverHashed := helpers.DerefString(p.Sha3512) != "" || helpers.DerefString(p.Sha256) != "" || helpers.DerefString(p.Md5) != ""
		stateHashed := rs.Primary.Attributes["sha3_512"] != "" || rs.Primary.Attributes["sha256"] != "" || rs.Primary.Attributes["md5"] != ""
		if serverHashed != stateHashed {
			return fmt.Errorf("state holds a hash: %v, server holds a hash: %v; state must record what the server holds", stateHashed, serverHashed)
		}
		return nil
	}
}

// largeLabel names the large size in the scenario log, which follows the MiB
// environment variable and not a fixed figure.
func largeLabel() string {
	mib := largeUploadBytes() / (1024 * 1024)
	if mib%1024 == 0 {
		return fmt.Sprintf("%d GiB", mib/1024)
	}
	return fmt.Sprintf("%d MiB", mib)
}

func TestAccResource_ProPackage_LargeUploadScenarios(t *testing.T) {
	requireLargeUploads(t)

	dir := t.TempDir()
	small := filepath.Join(dir, "small.bin")
	smallOther := filepath.Join(dir, "small-other.bin")
	large := filepath.Join(dir, "large.bin")
	largeSameSize := filepath.Join(dir, "large-same-size.bin")
	display := "tf-acc-large-" + testhelpers.RunSuffix()
	fileName := display + ".pkg"

	var started time.Time
	step := func(name, src string, size int64, prepare func()) resource.TestStep {
		return resource.TestStep{
			PreConfig: func() {
				if prepare != nil {
					prepare()
				}
				started = time.Now()
			},
			Config: hclLocalUpload(display, fileName, src, "large upload scenarios"),
			Check: resource.ComposeAggregateTestCheckFunc(
				logStep(t, name, &started),
				checkRecordMatches(t, size),
			),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
			},
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			step("create 300 MB", small, smallUploadBytes, func() {
				writeRandomFile(t, small, smallUploadBytes, 1)
				writeRandomFile(t, smallOther, smallUploadBytes, 2)
				writeRandomFile(t, large, largeUploadBytes(), 3)
				writeRandomFile(t, largeSameSize, largeUploadBytes(), 4)
			}),
			step("replace 300 MB with "+largeLabel(), large, largeUploadBytes(), nil),
			step("replace "+largeLabel()+" with "+largeLabel()+" of identical size", largeSameSize, largeUploadBytes(), nil),
			step("replace "+largeLabel()+" with 300 MB", smallOther, smallUploadBytes, nil),
		},
	})
}

func TestAccResource_ProPackage_LargeUploadCreate(t *testing.T) {
	requireLargeUploads(t)

	dir := t.TempDir()
	large := filepath.Join(dir, "large.bin")
	display := "tf-acc-large-create-" + testhelpers.RunSuffix()
	fileName := display + ".pkg"

	var started time.Time
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					writeRandomFile(t, large, largeUploadBytes(), 5)
					started = time.Now()
				},
				Config: hclLocalUpload(display, fileName, large, "large create"),
				Check: resource.ComposeAggregateTestCheckFunc(
					logStep(t, "create "+largeLabel(), &started),
					checkRecordMatches(t, largeUploadBytes()),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
