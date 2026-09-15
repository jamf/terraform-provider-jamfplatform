// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// cdnTypeRequiredFields maps each non-JCDS cdn_type to the attributes the Jamf
// Pro admin UI marks "Required" for that content delivery network. Sourced from
// the admin UI (Settings → Server → Cloud distribution point) per cdn_type:
//
//	RACKSPACE_CLOUD_FILES — Username, API key            → username, password
//	AMAZON_S3             — Access key ID, Secret access key → username, password
//	AKAMAI                — Username, Password, Upload URL,
//	                        Directory, Download URL        → username, password,
//	                                                         upload_url, directory,
//	                                                         download_url
//
// JAMF_CLOUD requires no credential fields (only the optional `master` toggle),
// so it is absent from the map.
var cdnTypeRequiredFields = map[string][]string{
	pro.CloudDistributionPointCdnTypeRackspaceCloudFiles: {"username", "password"},
	pro.CloudDistributionPointCdnTypeAmazonS3:            {"username", "password"},
	pro.CloudDistributionPointCdnTypeAkamai:              {"username", "password", "upload_url", "directory", "download_url"},
}

// cdnTypeRequiredFieldsConfigValidator enforces the per-cdn_type required-field
// rules at plan time. The requirement is value-discriminated — which fields are
// mandatory depends on the *value* of cdn_type — which off-the-shelf
// AlsoRequires/ConflictsWith cannot express (they fire on presence, not value),
// so this is a custom config validator per STYLE_GUIDE §Cross-field validation.
type cdnTypeRequiredFieldsConfigValidator struct{}

// Description returns a plain-text description.
func (cdnTypeRequiredFieldsConfigValidator) Description(context.Context) string {
	return "each non-JCDS cdn_type requires its content delivery network credential/endpoint fields (username/password, plus upload_url/directory/download_url for AKAMAI)"
}

// MarkdownDescription returns the markdown description.
func (v cdnTypeRequiredFieldsConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource checks that the fields the admin UI marks Required for the
// selected cdn_type are supplied. It defers on unknown values (cdn_type itself,
// or any individual required field) so the resource stays usable from modules /
// variables — config validators run with unknowns for anything sourced from a
// variable, for_each, count, or another resource (STYLE_GUIDE §Config-time
// validators MUST defer on unknown values).
func (cdnTypeRequiredFieldsConfigValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	// Read each attribute the rule touches as its typed value via GetAttribute,
	// rather than decoding the whole model — this is the style-guide pattern that
	// guarantees a Go decode can never collapse an unknown to a zero value
	// (STYLE_GUIDE §369). All fields here are scalars, but GetAttribute keeps the
	// unknown/null distinction explicit and unambiguous.
	var data CloudDistributionPointResourceModel
	for p, target := range map[string]*types.String{
		"cdn_type":     &data.CdnType,
		"username":     &data.Username,
		"password":     &data.Password,
		"upload_url":   &data.UploadURL,
		"directory":    &data.Directory,
		"download_url": &data.DownloadURL,
		"private_key":  &data.PrivateKey,
	} {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(p), target)...)
	}
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("require_signed_urls"), &data.RequireSignedURLs)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateCdnTypeRequiredFields(data)...)
}

// validateCdnTypeRequiredFields is the pure check behind the config validator,
// kept separate so it can be unit-tested with model values directly (including
// unknowns) without constructing a live tfsdk.Config. It defers on every
// unknown it touches — cdn_type itself and each individual required field — so
// the rule never fires for values sourced from variables / for_each / other
// resources (STYLE_GUIDE §Config-time validators MUST defer on unknown values).
func validateCdnTypeRequiredFields(data CloudDistributionPointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if data.CdnType.IsUnknown() || data.CdnType.IsNull() {
		return diags
	}

	cdnType := data.CdnType.ValueString()
	required, ok := cdnTypeRequiredFields[cdnType]
	if !ok {
		return diags // JAMF_CLOUD or unrecognised — OneOf handles the latter.
	}

	values := map[string]types.String{
		"username":     data.Username,
		"password":     data.Password,
		"upload_url":   data.UploadURL,
		"directory":    data.Directory,
		"download_url": data.DownloadURL,
	}

	for _, field := range required {
		v := values[field]
		if v.IsUnknown() {
			continue // defer — value not resolvable yet
		}
		if v.IsNull() || v.ValueString() == "" {
			diags.Append(diag.NewAttributeErrorDiagnostic(
				path.Root(field),
				fmt.Sprintf("%s is required when cdn_type = %q", field, cdnType),
				fmt.Sprintf("The Jamf Pro cloud distribution point requires %q to be set when cdn_type is %q.", field, cdnType),
			))
		}
	}

	// AWS CloudFront signed-URL private key: required when cdn_type is AMAZON_S3
	// AND signed URLs are enabled. Defers on unknown require_signed_urls or
	// unknown private_key.
	if cdnType == pro.CloudDistributionPointCdnTypeAmazonS3 && !data.RequireSignedURLs.IsUnknown() {
		signedURLs := !data.RequireSignedURLs.IsNull() && data.RequireSignedURLs.ValueBool()
		if signedURLs && !data.PrivateKey.IsUnknown() && (data.PrivateKey.IsNull() || data.PrivateKey.ValueString() == "") {
			diags.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("private_key"),
				"private_key is required when cdn_type = \"AMAZON_S3\" and require_signed_urls = true",
				"AWS CloudFront signed URLs require the CloudFront private key (`.pem`/`.der`, base64-encoded). Set `private_key`, or disable `require_signed_urls`.",
			))
		}
	}
	return diags
}
