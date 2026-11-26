// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"errors"
	"strings"
	"testing"
)

func TestParseClassifiesTheScheme(t *testing.T) {
	cases := []struct {
		src  string
		kind Kind
	}{
		{"/tmp/app.pkg", KindLocal},
		{"./relative/app.pkg", KindLocal},
		{"app.pkg", KindLocal},
		{"", KindLocal},
		{`C:\packages\app.pkg`, KindLocal},
		{"C://packages/app.pkg", KindLocal},
		{"http://example.com/app.pkg", KindHTTP},
		{"https://example.com/app.pkg?X-Amz-Signature=abc", KindHTTPS},
		{"HTTPS://example.com/app.pkg", KindHTTPS},
		{"s3://bucket/app.pkg", KindS3},
		{"S3://bucket/app.pkg", KindS3},
		{"azblob://account/pkgs/app.pkg", KindAzureBlob},
		{"gs://bucket/app.pkg", KindUnsupported},
		{"file:///tmp/app.pkg", KindUnsupported},
		{"ftp://example.com/app.pkg", KindUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			ref, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ref.Kind != tc.kind {
				t.Errorf("kind = %d, want %d", ref.Kind, tc.kind)
			}
			if got, want := ref.Native(), tc.kind == KindS3 || tc.kind == KindAzureBlob; got != want {
				t.Errorf("Native = %v, want %v", got, want)
			}
		})
	}
}

func TestParseReportsTheSchemeAsWritten(t *testing.T) {
	for src, lower := range map[string]bool{
		"s3://bucket/app.pkg":         true,
		"S3://bucket/app.pkg":         false,
		"Https://example.com/app.pkg": false,
		"/tmp/app.pkg":                true,
	} {
		ref, err := Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if ref.LowerCaseScheme() != lower {
			t.Errorf("%s: LowerCaseScheme = %v, want %v", src, ref.LowerCaseScheme(), lower)
		}
	}
}

func TestParseS3(t *testing.T) {
	cases := []struct {
		src                    string
		bucket, key, versionID string
		filename               string
	}{
		{"s3://bucket/app.pkg", "bucket", "app.pkg", "", "app.pkg"},
		{"s3://my.bucket-1/releases/2026/app.pkg", "my.bucket-1", "releases/2026/app.pkg", "", "app.pkg"},
		{"s3://bucket/app.pkg?versionId=3HL4kqtJlcpXroDTDmJ.rUAR", "bucket", "app.pkg", "3HL4kqtJlcpXroDTDmJ.rUAR", "app.pkg"},
		{"s3://bucket/My App #2 100%.pkg", "bucket", "My App #2 100%.pkg", "", "My App #2 100%.pkg"},
		{"s3://Legacy_Bucket/app.pkg", "Legacy_Bucket", "app.pkg", "", "app.pkg"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			ref, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ref.Bucket != tc.bucket || ref.Key != tc.key || ref.VersionID != tc.versionID {
				t.Errorf("got bucket=%q key=%q version=%q", ref.Bucket, ref.Key, ref.VersionID)
			}
			if ref.Filename() != tc.filename {
				t.Errorf("Filename = %q, want %q", ref.Filename(), tc.filename)
			}
			if ref.String() != tc.src {
				t.Errorf("String = %q, want the source back", ref.String())
			}
		})
	}
}

func TestParseS3Rejects(t *testing.T) {
	for _, src := range []string{
		"s3://",
		"s3://bucket",
		"s3://bucket/",
		"s3:///app.pkg",
		"s3://bucket/folder/",
		"s3://bad bucket/app.pkg",
		"s3://bucket/app.pkg?versionId=",
		"s3://bucket/app.pkg?versionId=a&partNumber=1",
		"s3://bucket/app.pkg?X-Amz-Signature=abc",
		"s3://bucket/?versionId=abc",
		"s3://bucket/app.pkg?versionId=abc#fragment",
		"s3://bucket/app.pkg\n",
		"s3://bucket/releases/..",
		"s3://bucket/.",
		"s3://AKIAEXAMPLE:s3cr3t@bucket/app.pkg",
	} {
		t.Run(src, func(t *testing.T) {
			ref, err := Parse(src)
			if _, ok := errors.AsType[*SyntaxError](err); !ok {
				t.Fatalf("want a SyntaxError, got %v", err)
			}
			if ref.Kind != KindS3 {
				t.Errorf("a malformed S3 source must still classify as S3, got %d", ref.Kind)
			}
			if strings.Contains(err.Error(), "s3cr3t") {
				t.Errorf("the error must not echo the source: %q", err)
			}
		})
	}
}

func TestParseAzureBlob(t *testing.T) {
	ref, err := Parse("azblob://neilstore01/pkgs/releases/My App.pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Account != "neilstore01" || ref.Container != "pkgs" || ref.Blob != "releases/My App.pkg" {
		t.Errorf("got account=%q container=%q blob=%q", ref.Account, ref.Container, ref.Blob)
	}
	if ref.Filename() != "My App.pkg" {
		t.Errorf("Filename = %q", ref.Filename())
	}
	if ref.String() != "azblob://neilstore01/pkgs/releases/My App.pkg" {
		t.Errorf("String = %q", ref.String())
	}
}

func TestParseAzureBlobRejects(t *testing.T) {
	for _, src := range []string{
		"azblob://",
		"azblob://account",
		"azblob://account/pkgs",
		"azblob://account/pkgs/",
		"azblob://account//app.pkg",
		"azblob://account/pkgs/folder/",
		"azblob://account.blob.core.windows.net/pkgs/app.pkg",
		"azblob://Account/pkgs/app.pkg",
		"azblob://account/pkgs/app.pkg?sv=2023&sig=secret",
		"azblob://account/pkgs/app.pkg?versionid=2026-11-22T00:00:00Z",
		"azblob://user:secret@account/pkgs/app.pkg",
		"azblob://account?sig=secret/pkgs/app.pkg",
		"azblob://account/pkgs/app.pkg\r\n",
		"azblob://account/pkgs/..",
	} {
		t.Run(src, func(t *testing.T) {
			ref, err := Parse(src)
			if _, ok := errors.AsType[*SyntaxError](err); !ok {
				t.Fatalf("want a SyntaxError, got %v", err)
			}
			if ref.Kind != KindAzureBlob {
				t.Errorf("a malformed Azure source must still classify as Azure, got %d", ref.Kind)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Errorf("the error must not echo the query: %q", err)
			}
		})
	}
}

func TestRefStringIsEmptyForSourcesThisPackageDoesNotRead(t *testing.T) {
	for _, src := range []string{"/tmp/app.pkg", "https://example.com/app.pkg?sig=secret", "gs://bucket/app.pkg"} {
		ref, err := Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if ref.String() != "" || ref.Filename() != "" {
			t.Errorf("%s: String=%q Filename=%q, want both empty", src, ref.String(), ref.Filename())
		}
	}
}
