// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 is the one digest every distribution point reports, not a security control
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
)

// EnvCloudStorageS3Bucket names the bucket the cloud storage acceptance tests
// write their objects to and read them back from. Setting it says the run has
// such a bucket and AWS credentials on the default chain that can write to it.
const EnvCloudStorageS3Bucket = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_BUCKET"

// The optional settings of the cloud storage fixture. Each is read through
// AccEnv; only the bucket is required.
const (
	// EnvCloudStorageS3Prefix is the key prefix test objects are written
	// under. Defaults to "tf-acc/".
	EnvCloudStorageS3Prefix = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_PREFIX"
	// EnvCloudStorageS3Region is the Region the provider's client starts in.
	// Set it to a Region other than the bucket's to cover Region discovery.
	EnvCloudStorageS3Region = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_REGION"
	// EnvCloudStorageS3Endpoint points the tests at an S3-compatible store
	// such as MinIO. Setting it also turns on path-style addressing.
	EnvCloudStorageS3Endpoint = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_ENDPOINT"
	// EnvCloudStorageS3AssumeRoleARN is a role the ambient identity may
	// assume and that can read the bucket, for the assume_role test.
	EnvCloudStorageS3AssumeRoleARN = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_ASSUME_ROLE_ARN"
	// EnvCloudStorageS3AssumeRoleExternalID is the external ID that role's
	// trust policy asks for, if any.
	EnvCloudStorageS3AssumeRoleExternalID = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_ASSUME_ROLE_EXTERNAL_ID"
	// EnvCloudStorageS3ProfileA and EnvCloudStorageS3ProfileB are two named
	// AWS profiles that can each read the bucket, for the two-alias test.
	EnvCloudStorageS3ProfileA = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_PROFILE_A"
	EnvCloudStorageS3ProfileB = "JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_PROFILE_B"
)

// cloudStorageWriteTimeout bounds one write to the fixture bucket.
const cloudStorageWriteTimeout = 2 * time.Minute

// CloudStorageFixture is the bucket the cloud storage acceptance tests use,
// and the means to put objects in it.
//
// Objects are written with the AWS default credential chain, the same one the
// provider under test reads with when its file_sources.s3 block names no
// credential. The identity therefore needs to write and delete under the
// prefix as well as read, which is more than a provider user's identity needs.
//
// One type serves both stores. An S3 fixture has Bucket, Region and Endpoint;
// an Azure Blob Storage fixture has Account and Container. The Azure fixture
// writes with the Microsoft Entra credential the ARM_* variables or the Azure
// CLI give it, and the identity needs Storage Blob Data Contributor on the
// container.
type CloudStorageFixture struct {
	Bucket   string
	Prefix   string
	Region   string
	Endpoint string

	Account   string
	Container string

	sources *filesources.Resolver
}

// CloudStorageObject is one object a test wrote. Source is the object as a
// package_file_source value. VersionID is the version the write created, and
// is empty on a bucket without versioning. MD5 and SHA256 are the digests of
// the bytes written, in lower-case hex.
type CloudStorageObject struct {
	Source    string
	VersionID string
	MD5       string
	SHA256    string
	Size      int
}

// RequireCloudStorageFixture returns the fixture, or skips the test when no
// bucket is named.
//
// It is a skip everywhere, in a pipeline as well as locally, and the helper is
// deliberately not named AccPreCheck*, so internal/conformance/acc_lanes_test.go
// does not demand a require token that would promote the skip to a lane
// failure. A bucket is infrastructure outside Jamf that an estate may simply
// not have, like the cloud distribution point RequireDirectUploads looks for,
// and a run without one is a legitimate run. The cost is the one that helper
// records: a green lane says nothing about cloud storage sources unless the
// bucket was named, so check the skip count before trusting it.
//
// AWS credentials are not looked for. They come from the default credential
// chain, which has no single variable to check, and credentials that are
// present and refused must fail; the first write to the bucket reports them.
func RequireCloudStorageFixture(t *testing.T) *CloudStorageFixture {
	t.Helper()
	if AccEnv(EnvCloudStorageS3Bucket) == "" {
		t.Skipf("skipping: %s names no bucket to read a package file from", EnvCloudStorageS3Bucket)
	}
	fixture := &CloudStorageFixture{
		Bucket:   AccEnv(EnvCloudStorageS3Bucket),
		Prefix:   AccEnv(EnvCloudStorageS3Prefix),
		Region:   AccEnv(EnvCloudStorageS3Region),
		Endpoint: AccEnv(EnvCloudStorageS3Endpoint),
	}
	if fixture.Prefix == "" {
		fixture.Prefix = "tf-acc/"
	}
	fixture.sources = filesources.New(filesources.Config{
		UserAgent: "terraform-provider-jamfplatform/acceptance",
		S3: filesources.S3Config{
			Region:       fixture.Region,
			EndpointS3:   fixture.Endpoint,
			UsePathStyle: fixture.Endpoint != "",
		},
	}, nil)
	return fixture
}

// EnvCloudStorageAzureAccount and EnvCloudStorageAzureContainer name the
// storage account and container the Azure Blob Storage acceptance tests write
// to and read from. Setting both says the run has them and a Microsoft Entra
// credential that can write there.
const (
	EnvCloudStorageAzureAccount   = "JAMFPLATFORM_ACC_CLOUDSTORAGE_AZURE_ACCOUNT"
	EnvCloudStorageAzureContainer = "JAMFPLATFORM_ACC_CLOUDSTORAGE_AZURE_CONTAINER"
	// EnvCloudStorageAzurePrefix is the blob name prefix the tests write
	// under. Defaults to "tf-acc/".
	EnvCloudStorageAzurePrefix = "JAMFPLATFORM_ACC_CLOUDSTORAGE_AZURE_PREFIX"
)

// RequireAzureBlobFixture returns the Azure Blob Storage fixture, or skips the
// test when no account and container are named. It is a skip everywhere, for
// the reason RequireCloudStorageFixture gives.
func RequireAzureBlobFixture(t *testing.T) *CloudStorageFixture {
	t.Helper()
	fixture := &CloudStorageFixture{
		Account:   AccEnv(EnvCloudStorageAzureAccount),
		Container: AccEnv(EnvCloudStorageAzureContainer),
		Prefix:    AccEnv(EnvCloudStorageAzurePrefix),
	}
	if fixture.Account == "" || fixture.Container == "" {
		t.Skipf("skipping: %s and %s do not both name where to read a package file from", EnvCloudStorageAzureAccount, EnvCloudStorageAzureContainer)
	}
	if fixture.Prefix == "" {
		fixture.Prefix = "tf-acc/"
	}
	fixture.sources = filesources.New(filesources.Config{UserAgent: "terraform-provider-jamfplatform/acceptance"}, nil)
	return fixture
}

// Source returns the package_file_source value for an object name under the
// fixture's prefix. The object need not exist.
func (f *CloudStorageFixture) Source(name string) string {
	if f.Account != "" {
		return "azblob://" + f.Account + "/" + f.Container + "/" + f.Prefix + name
	}
	return "s3://" + f.Bucket + "/" + f.Prefix + name
}

// PutRandom writes size random bytes to the named object, replacing whatever
// is there, and removes the object when the test ends. Each call writes
// different bytes, so a second call under the same name is a changed object.
//
// A refused or failed write fails the test: by this point the bucket was
// declared, so the store turning the fixture away is a broken fixture, not a
// missing one.
func (f *CloudStorageFixture) PutRandom(t *testing.T, name string, size int) CloudStorageObject {
	t.Helper()
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("generating object bytes: %v", err)
	}
	return f.Put(t, name, body)
}

// Put writes body to the named object, as PutRandom does.
func (f *CloudStorageFixture) Put(t *testing.T, name string, body []byte) CloudStorageObject {
	t.Helper()
	source := f.Source(name)
	ctx, cancel := context.WithTimeout(context.Background(), cloudStorageWriteTimeout)
	defer cancel()
	version, err := "", error(nil)
	remove := f.sources.DeleteForAcceptance
	if f.Account != "" {
		err, remove = f.sources.PutBlobForAcceptance(ctx, source, body), f.sources.DeleteBlobForAcceptance
	} else {
		version, err = f.sources.PutForAcceptance(ctx, source, body)
	}
	if err != nil {
		t.Fatalf("writing the fixture object: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudStorageWriteTimeout)
		defer cancel()
		if err := remove(ctx, source); err != nil {
			t.Logf("removing the fixture object: %v", err)
		}
	})
	md5Sum := md5.Sum(body) //nolint:gosec // see the md5 import
	sha256Sum := sha256.Sum256(body)
	return CloudStorageObject{
		Source:    source,
		VersionID: version,
		MD5:       hex.EncodeToString(md5Sum[:]),
		SHA256:    hex.EncodeToString(sha256Sum[:]),
		Size:      len(body),
	}
}

// ProviderBlock returns a provider block whose file_sources.s3 block points at
// the fixture, with extra lines added inside the s3 block. alias is empty for
// the default provider. An Azure fixture returns no block at all:
// its tests run on the ambient credential, which is the case a user with the
// azurerm backend already configured is in.
func (f *CloudStorageFixture) ProviderBlock(alias string, extra ...string) string {
	if f.Account != "" {
		return ""
	}
	var lines []string
	if f.Region != "" {
		lines = append(lines, fmt.Sprintf("region = %q", f.Region))
	}
	if f.Endpoint != "" {
		lines = append(lines, fmt.Sprintf("endpoints = { s3 = %q }", f.Endpoint), "use_path_style = true")
	}
	lines = append(lines, extra...)
	aliasLine := ""
	if alias != "" {
		aliasLine = fmt.Sprintf("  alias = %q\n", alias)
	}
	return fmt.Sprintf("provider \"jamfplatform\" {\n%s  file_sources = {\n    s3 = {\n      %s\n    }\n  }\n}\n",
		aliasLine, strings.Join(lines, "\n      "))
}
