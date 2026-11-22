// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPackageFileSource(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		summary string
	}{
		{"absolute path", types.StringValue("/tmp/app.pkg"), ""},
		{"relative path", types.StringValue("build/app.pkg"), ""},
		{"windows path", types.StringValue(`C:\packages\app.pkg`), ""},
		{"empty", types.StringValue(""), ""},
		{"http", types.StringValue("http://example.com/app.pkg"), ""},
		{"https with signed query", types.StringValue("https://example.com/app.pkg?sig=secret"), ""},
		{"s3", types.StringValue("s3://bucket/releases/app.pkg"), ""},
		{"s3 with version", types.StringValue("s3://bucket/app.pkg?versionId=abc"), ""},
		{"azure", types.StringValue("azblob://account/pkgs/app.pkg"), ""},
		{"null defers", types.StringNull(), ""},
		{"unknown defers", types.StringUnknown(), ""},
		{"google cloud storage", types.StringValue("gs://bucket/app.pkg"), "Unsupported source scheme"},
		{"file scheme", types.StringValue("file:///tmp/app.pkg"), "Unsupported source scheme"},
		{"upper-case s3", types.StringValue("S3://bucket/app.pkg"), "Source scheme must be lower case"},
		{"upper-case https", types.StringValue("HTTPS://example.com/app.pkg"), "Source scheme must be lower case"},
		{"s3 without a key", types.StringValue("s3://bucket"), "Invalid cloud storage source"},
		{"s3 with a stray query", types.StringValue("s3://bucket/app.pkg?sig=secret"), "Invalid cloud storage source"},
		{"azure with a sas token", types.StringValue("azblob://account/pkgs/app.pkg?sig=secret"), "Invalid cloud storage source"},
		{"azure with a hostname", types.StringValue("azblob://account.blob.core.windows.net/pkgs/app.pkg"), "Invalid cloud storage source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			PackageFileSource().ValidateString(context.Background(), validator.StringRequest{Path: path.Root("package_file_source"), ConfigValue: tc.value}, resp)
			if tc.summary == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected error: %v", resp.Diagnostics)
				}
				return
			}
			if resp.Diagnostics.ErrorsCount() != 1 {
				t.Fatalf("want one error, got %v", resp.Diagnostics)
			}
			got := resp.Diagnostics[0]
			if got.Summary() != tc.summary {
				t.Errorf("summary = %q, want %q", got.Summary(), tc.summary)
			}
			if strings.Contains(got.Detail(), "secret") {
				t.Errorf("the diagnostic must not echo a query string: %q", got.Detail())
			}
		})
	}
}

func TestNoCloudStorageSource(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"local path", types.StringValue("/tmp/icon.png"), false},
		{"https", types.StringValue("https://example.com/icon.png"), false},
		{"upper-case https keeps its old behaviour", types.StringValue("HTTPS://example.com/icon.png"), false},
		{"another scheme keeps its old behaviour", types.StringValue("gs://bucket/icon.png"), false},
		{"empty", types.StringValue(""), false},
		{"null defers", types.StringNull(), false},
		{"unknown defers", types.StringUnknown(), false},
		{"s3", types.StringValue("s3://bucket/icon.png"), true},
		{"upper-case s3", types.StringValue("S3://bucket/icon.png"), true},
		{"malformed s3", types.StringValue("s3://bucket"), true},
		{"azure", types.StringValue("azblob://account/images/icon.png"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			NoCloudStorageSource().ValidateString(context.Background(), validator.StringRequest{Path: path.Root("icon_file_source"), ConfigValue: tc.value}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("error = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if !tc.wantErr {
				return
			}
			detail := resp.Diagnostics[0].Detail()
			if !strings.Contains(detail, "package_file_source") || !strings.Contains(detail, "jamfplatform_pro_package") {
				t.Errorf("the diagnostic must name the attribute that reads these sources: %q", detail)
			}
		})
	}
}
