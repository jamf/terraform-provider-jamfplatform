// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// packageSourceAttribute is the one attribute that reads s3:// and azblob://
// sources, named in every diagnostic that turns one away.
const packageSourceAttribute = "package_file_source"

// packageFileSourceValidator checks the scheme and syntax of a
// package_file_source value.
type packageFileSourceValidator struct{}

// PackageFileSource returns the validator.String for package_file_source on
// jamfplatform_pro_package: a local path, or an http://, https://, s3:// or
// azblob:// source.
//
// Without it a value such as "gs://bucket/app.pkg" or "S3://bucket/app.pkg"
// matches no branch that fetches it, falls through to the local-path branch
// and fails during apply as a file that does not exist, which names the wrong
// problem. A malformed s3:// or azblob:// source is refused here for the same
// reason, with the form it should take.
//
// The diagnostic never echoes the value: a source can be a presigned URL, and
// its query string is a credential.
//
// Null and unknown values defer, per STYLE_GUIDE §Cross-field validation.
func PackageFileSource() validator.String {
	return packageFileSourceValidator{}
}

// Description returns a plain-text description of the validator.
func (packageFileSourceValidator) Description(_ context.Context) string {
	return "must be a local path or an http://, https://, s3:// or azblob:// source"
}

// MarkdownDescription returns the markdown description of the validator.
func (packageFileSourceValidator) MarkdownDescription(_ context.Context) string {
	return "must be a local path or an `http://`, `https://`, `s3://` or `azblob://` source"
}

// ValidateString implements validator.String.
func (packageFileSourceValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	ref, err := filesources.Parse(req.ConfigValue.ValueString())
	switch {
	case ref.Kind == filesources.KindUnsupported:
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported source scheme", fmt.Sprintf(
			"%s does not read %s:// sources. Give it a local path, or an http://, https://, s3:// or azblob:// source.",
			packageSourceAttribute, ref.Scheme))
	case !ref.LowerCaseScheme():
		resp.Diagnostics.AddAttributeError(req.Path, "Source scheme must be lower case", fmt.Sprintf(
			"Write the scheme %s:// in lower case. In any other spelling the value is read as a local path.", ref.Scheme))
	case err != nil:
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid cloud storage source", helpers.APIErrorDetail(err))
	}
}

// noCloudStorageSourceValidator turns away s3:// and azblob:// sources on an
// attribute that does not read them.
type noCloudStorageSourceValidator struct{}

// NoCloudStorageSource returns the validator.String for every upload-source
// attribute other than package_file_source. Those attributes read a local path
// or an http(s) URL only, so an s3:// or azblob:// value would be read as a
// local path and fail during apply as a missing file. The diagnostic names the
// attribute that does read them.
//
// Only the two cloud schemes are refused. Any other value keeps the behaviour
// the attribute had before the cloud schemes existed.
//
// Null and unknown values defer, per STYLE_GUIDE §Cross-field validation.
func NoCloudStorageSource() validator.String {
	return noCloudStorageSourceValidator{}
}

// Description returns a plain-text description of the validator.
func (noCloudStorageSourceValidator) Description(_ context.Context) string {
	return "must be a local path or an http(s):// URL, not an s3:// or azblob:// source"
}

// MarkdownDescription returns the markdown description of the validator.
func (noCloudStorageSourceValidator) MarkdownDescription(_ context.Context) string {
	return "must be a local path or an `http(s)://` URL, not an `s3://` or `azblob://` source"
}

// ValidateString implements validator.String.
func (noCloudStorageSourceValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	ref, _ := filesources.Parse(req.ConfigValue.ValueString())
	if !ref.Native() {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Cloud storage source not supported here", fmt.Sprintf(
		"Only %s on jamfplatform_pro_package reads s3:// and azblob:// sources. Give this attribute a local path or an http(s):// URL.",
		packageSourceAttribute))
}
